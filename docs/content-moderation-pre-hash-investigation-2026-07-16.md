# 内容审计前置哈希比对未拦截关键词命中的调查与修复

- 记录日期：2026-07-16
- 时间口径：本文业务时间均为 UTC+8（Asia/Shanghai）
- 文档性质：线上调查记录 + 未提交改动说明；不包含提示词正文、用户邮箱、API Key 或 OAuth 凭据（受影响用户以 `user_id` 引用）
- 调查范围：admin/risk-control 风控中心「内容审计设置 - 启用前置哈希比对」开关的实际生效范围

## 1. 记录时的线上版本

| 组件 | 版本 / 提交 | 线上镜像 digest | 状态 |
|---|---|---|---|
| Sub2API | `cafecode-v0.0.26` / `a24f0ee8` | `sha256:cbbbead44e18ec0dbdfe0637ebeb46726570ea52fcea79604734a40417b717a5` | healthy |

## 2. 报告的现象

风控中心开关「启用前置哈希比对」说明文案为：

> 异步审核命中过的输入哈希会被前置拦截；该拦截不发送邮件，也不累计封禁次数。

但审核记录里对同一用户发出了 180 余封邮件，且这些记录的「输入摘要」完全相同，看起来该开关未生效。

## 3. 线上证据

### 3.1 配置侧确认开关确实是开的

`settings.content_moderation_config` 关键字段（其余字段与 API Key 略）：

| 字段 | 值 |
|---|---|
| `enabled` | `true` |
| `mode` | `pre_block` |
| `pre_hash_check_enabled` | **`true`** |
| `email_on_hit` | `true` |
| `auto_ban_enabled` | `false` |
| `ban_threshold` | `4` |
| `keyword_blocking_mode` | `keyword_and_api` |
| `worker_count` | `2` |

### 3.2 涉事记录全部是关键词命中，不是异步审核命中

`content_moderation_logs` 中 `user_id=191`，全部 196 条均为 `action='keyword_block'`、`matched_keyword='破解版'`、`input_excerpt` 只有 **1 种**（md5 相同）：

| `email_sent` | 条数 | `violation_count` 区间 | 时间范围 |
|---|---:|---|---|
| `t` | 188 | 1 → 188 | 12:55:40 – 12:58:36 |
| `f` | 8 | 189 → 196 | 12:58:30 – 12:58:36 |

即约 3 分钟内 188 封邮件，违规计数一路累加到 196。（末尾 8 条未发信的原因未在本次调查中定位，不影响本文结论。）

### 3.3 对照组：审核 API 路径上该功能是正常的

| `action` | 条数 | 其中发信 |
|---|---:|---:|
| `block`（审核 API 命中） | 20 | 20 |
| `hash_block`（前置哈希拦截） | 26 | **0** |

Redis `content_moderation:flagged_hashes` 为 `set` 类型、成员数 `10`。

也就是说：20 次 API 命中去重后产生 10 个哈希，据此拦下 26 次重复且一封邮件都没发——**前置哈希比对在它覆盖的路径上完全正常**。问题只在于它从未覆盖关键词路径。

## 4. 根因

两处代码共同导致关键词路径完全在该开关的作用范围之外。

### 4.1 关键词命中从不写入哈希

`backend/internal/service/content_moderation.go` 关键词拦截分支的落库调用是：

```go
s.enqueueRecord(input, cfg, log, hashText, false, true)  // recordHash=false, applySideEffects=true
```

对比审核 API 命中路径传的是 `flagged, flagged`（命中即记哈希）。因此关键词命中既不写哈希（下次比对必然落空），又每次都执行副作用（发邮件 + 累计违规）。

### 4.2 哈希比对排在关键词检查之后

`checkInternal` 的原始顺序为：关键词检查 → `keyword_only` 提前返回 → 哈希比对 → 采样 → 审核 API。

即便 4.1 修好，关键词命中也会先短路返回并再次发信，哈希比对没有机会执行。另外 `keyword_only` 模式下的提前返回使哈希比对在该模式下完全不可达。

### 4.3 结论

文案里「异步审核命中过的」这个限定语在字面上是准确的——关键词命中本就不经过审核 API。这是**功能覆盖范围**与**管理员预期**之间的落差，不是单纯的实现 bug。

## 5. 修复方案

核心判断：**关键词匹配是本地的、廉价的，真正需要去重的是通知，而不是拦截决策本身**。因此不把重复的关键词命中改写成 `hash_block`（那会把一次关键词命中谎报成哈希命中，并丢失归因），而是保留 `keyword_block` 并抑制其副作用。

修复后的行为：

| 场景 | 动作 | 邮件 | 违规计数 | 审核 API |
|---|---|---|---|---|
| 首次关键词命中 | `keyword_block` | 发 | +1 | 不调用 |
| 重复相同输入 | `keyword_block`（保留 `matched_keyword`） | **不发** | **不加** | 不调用 |
| 首次 API 命中 | `block` | 发 | +1 | 调用 |
| 重复相同输入（拦截模式） | `hash_block` | 不发 | 不加 | **跳过** |
| 重复相同输入（观察模式） | 放行 | 不发 | 不加 | **跳过** |

## 6. 代码 review 发现的问题及处理

| # | 问题 | 处理 |
|---|---|---|
| 1 | 回归测试的 `EmailSent` / `ViolationCount` 断言是空断言（`emailService` 为 nil、输入未设 `UserID`，导致每条日志都提前返回，断言恒真） | 设 `UserID`，改断言 `logs[0].ViolationCount==1` / `logs[1]==0`；已用变异测试验证其会失败 |
| 2 | 哈希分支返回 `Blocked:true` 不分模式，观察模式会硬拦真实流量；关键词哈希入集后影响面被放大 | 观察模式下只跳过审核 API、不拦截 |
| 3 | 哈希经共享异步队列落盘，而 worker 同步阻塞在 SMTP 上（线上 `worker_count=2`），突发时仍会发出大量重复邮件 | 改为在判定点同步写入，脱离该队列 |
| 4 | Redis 集合无 TTL 无上限，本次改动使写入量提升一个数量级 | `SET` 换为按过期时间打分的 `ZSET`，TTL 取 `HitRetentionDays`（默认 180 天），写入时 `ZREMRANGEBYSCORE` 剪枝 |
| 5 | 重复记录丢失关键词归因（`matched_keyword` 为空、分类变 `hash`） | 由第 5 节的设计一并解决：重复仍是 `keyword_block` |
| 6 | 重复命中把输入的 SHA-256 拼进拦截文案返回给终端用户 | 关键词重复不再走该分支；API 命中重复的该行为**未改动**（见第 9 节） |

顺带清理：`recordHash` 参数原本穿过 `enqueueRecord` / `persistContentModerationLog` / task 三层，哈希改为判定点写入后已删除。

## 7. 涉及文件

| 文件 | 改动 |
|---|---|
| `backend/internal/service/content_moderation.go` | 检查流程重构；新增 `recordFlaggedInputHash`；删除 `recordHash` 管道 |
| `backend/internal/repository/content_moderation_hash_cache.go` | `SET` → `ZSET`（带 TTL 与剪枝），换 key |
| `backend/internal/service/content_moderation_test.go` | 重写关键词重复回归测试；新增观察模式测试 |
| `backend/internal/repository/content_moderation_hash_cache_test.go` | 新增（miniredis，5 个用例） |
| `frontend/src/i18n/locales/zh.ts` / `en.ts` | 开关说明文案 |

## 8. 验证情况

- `go build ./...`、`go vet ./internal/...` 干净
- `go test ./internal/service/`（43s）、`go test -tags unit ./internal/repository/`（2.3s）全过
- 前端 `RiskControlView.spec.ts` 5/5
- 变异验证：把副作用抑制改回 `true` → 关键词重复测试失败；去掉观察模式门禁 → 观察模式测试失败；还原后全过

注意：`miniredis` 的 `FastForward` 不会推进 Go 的 `time.Now()`，而过期判断依赖 score 里的墙钟时间。缓存测试因此改用直接种入过期 score 来复现老化条目，不要用 `FastForward` 重写这些用例。

## 9. 部署注意事项与未决项

1. **Redis key 迁移**：新键为 `content_moderation:flagged_hashes:v2`（`ZSET`）。旧键 `content_moderation:flagged_hashes` 是 `SET`，类型不兼容、同名会 `WRONGTYPE`，故只能换名。线上旧键现有 10 个哈希会被弃用，代价是它们下次命中时各多发一封邮件。后台「清空哈希」已会一并删除旧键，也可手动 `DEL`。
2. **未改动项**：审核 API 命中的重复（`hash_block`）仍会把哈希以 `拦截文案（hash: …）` 形式返回给终端用户。该格式看似有意为之（便于用户报给客服，对应后台「删除哈希」操作），故保留。
3. **语义扩大**：「不累计封禁次数」现在也适用于关键词命中，即同一段文本重复 200 次只记 1 次违规，自动封禁不再被此类重复触发。线上 `auto_ban_enabled=false`，当前不影响实际封禁。
4. **本记录时改动尚未提交**：工作区当时有 111 个文件未提交，且 `content_moderation.go` 内本修复与另一套未提交的 `cacheOnly`（配置快照）重构交错在同一文件，无法单独 `git add`。`cafecode-v*` tag 会触发 `custom-prod-image.yml` 构建并发布生产镜像、覆盖 `latest`，故未 push / 未 tag。建议先单独提交 `cacheOnly` 那套，本修复即可干净成独立 commit。
5. **线上遗留数据未处理**：`user_id=191` 的 196 条记录与 Redis 中既有的 10 个哈希均保持原样。
