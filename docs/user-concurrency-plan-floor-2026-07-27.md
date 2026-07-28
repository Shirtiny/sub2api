# 用户并发生效口径修正：套餐并发从"覆盖"改为"下限"，并让套餐编辑回填已有订阅

- 记录日期：2026-07-28（改动落地于 2026-07-27）
- 时间口径：本文业务时间均为 UTC+8（Asia/Shanghai）
- 文档性质：线上问题调查 + 代码改动说明 + 生产配置与数据修正记录；不包含用户邮箱、API Key 或 OAuth 凭据（受影响用户以 `user_id` 引用）
- 涉及页面：`/admin/users`（并发数列）、`/admin/orders` 套餐编辑、用户端个人资料与兑换页

| 组件 | 版本 / 提交 | 状态 |
|---|---|---|
| Sub2API（口径修正） | `cafecode-v0.0.36` / `154e8fdaa` | 已部署，镜像 `sha256:2ea87465…` |
| Sub2API（套餐编辑回填） | `cafecode-v0.0.37` / `9300fa947` | **已打标签，未部署** |

---

## 1. 报告的现象

管理端 `/admin/users` 中，`user_id=43` 的并发列显示 `3 / 4`，而管理员在用户编辑弹窗里设置的并发是 32。反复改成 16 再改回 32（`redeem_codes` 中留下两条 `admin_concurrency` ±16 记录，17:20:49 与 17:20:54）后，生效值始终是 4。

结论先行：**不是缓存、不是统计误差，是设计上的"套餐并发覆盖用户并发"。**

## 2. 根因

### 2.1 旧规则：套餐一旦生效就完全替换基础并发

`service.User.EffectiveConcurrencyAt` 的旧实现是：只要存在任一生效中的套餐并发权益，就取这些权益的最大值返回，**完全忽略 `users.concurrency`**；只有全部权益过期后才回落到基础值。套餐编辑页的提示文案也是这么写的（"套餐有效期内的最大并发请求数……套餐到期后恢复用户的基础并发数"）。

用户 43 的线上数据：

| 项 | 值 | 来源 |
|---|---|---|
| 基础并发 | 32 | `users.concurrency` |
| 套餐权益 | 4 | `subscription_concurrency_entitlements` id=56，订阅 812（group 4 / Latte），窗口 2026-06-29 → 07-29 |
| 生效值 | 4 | `EffectiveConcurrencyAt` 取套餐值 |
| 实时占用 | 4 槽 + 1 排队 | Redis `concurrency:user:43` zset 长度 4、`concurrency:wait:43`=1 |

网关限流与管理端展示走的是同一个函数（`api_key_auth.go` / `user_handler.go`），所以两边一致，被忽略的就是管理员设置的 32。

### 2.2 连带影响：加并发的手段全部失效

管理员手动调整（`admin_service.go`）和并发兑换码（`redeem_service.go`）都只写 `users.concurrency`。在旧规则下，**只要用户有生效套餐，这两种加并发手段对他完全无效**，且用户端和管理端都看不出来。

## 3. 决策

生效并发改为 **`max(用户基础并发, 所有生效中的套餐权益)`**。

- 套餐权益从"上限/覆盖值"变为"下限/保底值"：仍能把默认 1 并发的新用户抬到套餐档位，但不再压低管理员或兑换码给出的更高值。
- 备选方案（新增 `users.concurrency_override` 字段做无条件覆盖）因需要动 schema + 迁移 + 管理端表单 + DTO，未采用。

## 4. 改动 A：`cafecode-v0.0.36`（口径修正）

| 文件 | 改动 |
|---|---|
| `backend/internal/service/user.go` | `EffectiveConcurrencyAt` 把 `u.Concurrency` 纳入取最大值；注释写明"权益是下限不是上限"及其原因 |
| `backend/internal/repository/user_repo.go` | 管理端列表按并发排序的 SQL 表达式由 `COALESCE(NULLIF(plan,0), base)` 改为 `GREATEST(plan, COALESCE(base,0))` |
| `frontend/src/i18n/locales/{zh,en}.ts` | 套餐编辑页并发提示改为"保底并发数；用户基础并发更高时按基础并发生效" |
| `frontend/src/types/index.ts` | `base_concurrency` / `effective_concurrency` 的注释由"fallback"改为"floor / max" |
| 测试 | `user_plan_concurrency_test.go` 新增 base 32 + 套餐 4 → 32；`user_repo_plan_concurrency_unit_test.go` 补 base 高于套餐时的生效值与排序断言 |

### 4.1 排序 SQL 是独立实现，容易漏

管理端列表按"并发数"表头排序时**不走 Go 的 `EffectiveConcurrencyAt`**，而是在 `userEffectiveConcurrencyOrder` 里用一段等价的 SQL 表达式重算。首次改动只改了 Go 侧，会导致列表显示 32 却按 4 排序，27 个被加过并发的用户全部堆到降序末尾。

原有测试无法发现这个问题：两个 fixture 的基础并发都低于套餐值。已补断言。另外该测试只跑 SQLite（`MAX` 分支），Postgres 的 `GREATEST` 分支是拿生产库执行同构表达式验证的。

**后续改动注意：并发生效口径有两处实现，改一处必须同步另一处。**

## 5. 改动 B：`cafecode-v0.0.37`（套餐编辑回填已有订阅）

### 5.1 问题：权益是购买时的快照

`subscription_concurrency_entitlements` 的并发值是**订阅创建/续期时拍的快照**，而 `UpdatePlan` 只写 `subscription_plans`，不碰任何已有权益行。2026-07-27 17:35–18:10 期间套餐并发被调高（Latte 4→8、Specialty 4→16、Frappé 4→8）后，线上出现：

| 套餐 | 套餐现值 | 权益快照 | 生效订阅数 |
|---|---|---|---|
| Latte (拿铁) | 8 | 4 | 21 |
| Specialty (精选) | 16 | 4 | 4 |
| Americano / Instant / Shaken Tea | 4 | 4 | 32 |

即 25 个已购用户仍按 4 并发跑，要等续期才会拿到新值。

### 5.2 改动

`UpdatePlan` 保存后，若并发值发生变化，调用新增的 `raiseActiveConcurrencyEntitlements`（`payment_config_plans.go`）：

- 范围：该套餐 group 下、订阅 `active` 且未过期、权益 `expires_at > now()` 的行。
- **只升不降**：`ConcurrencyLT(plan.Concurrency)`，快照已 ≥ 新值的一律不动（更高值来自管理员刻意授予或更贵的购买，套餐编辑不得回收）。
- **包含未开始的 term**：提前续费会把新一期权益排在当前期之后（`planConcurrencyWindow`）。若只处理"当前生效中"的权益，这类排队 term 会在接管时把用户打回旧值，且以后再改套餐也永远修不到它。
- 失效受影响用户的 auth 缓存：`PaymentConfigService` 新增 `authCacheInvalidator`（wire 注入），否则新值要等 auth 快照过期（L2 300s）才生效。
- 失败只记日志不返回错误：套餐本身已持久化，不能让编辑显得失败。

测试：`payment_config_plan_concurrency_test.go` 覆盖抬升、跳过更高值、跳过已过期、跳过其他 group、抬升未开始的 term、并发值无变化时不动且不失效缓存。

### 5.3 未覆盖场景

- **新建套餐**不会回填该 group 已有订阅（新套餐不应追溯升级存量）。
- **把套餐迁到另一个 group** 不触发传播（只有并发值变化才触发）。
- 一个 group 下存在多个套餐时，各套餐的编辑各自按"只升不降"生效，等价于取最大值。

## 6. 生产配置调整

### 6.1 认证来源默认并发 5 → 4

`auth_source_default_{email,google,github,oidc,wechat,dingtalk}_concurrency` 由 5 改为 4（18:13）。

**这 6 个渠道的 `grant_on_signup` 与 `grant_on_first_bind` 全为 `false`，该值从未生效过**，新用户拿的一直是 `default_concurrency`=4。此项属口径清理，无行为变化。

### 6.2 关闭 LinuxDo 首绑加并发

`auth_source_default_linuxdo_concurrency` 由 4 改为 **0**（18:16）。

- LinuxDo 是唯一 `grant_on_first_bind=true` 的渠道，其首绑逻辑走的是 `AddConcurrency`（`auth_oauth_first_bind.go`），即**累加**基础并发 +4，而不是设置。旧规则下这个 +4 被套餐值吃掉、不可见；改成 `max()` 后它会变成真实加量。
- 之所以不是关 `grant_on_first_bind`：该开关同时管着 LinuxDo 首绑的 **+10 余额**，必须保留。代码里 `if providerDefaults.Concurrency != 0` 的守卫使并发值 0 即跳过，余额分支不受影响。
- **坑**：管理后台该输入框是 `min="1"`、placeholder `5`（`SettingsView.vue`）。从设置页保存"认证来源默认值"有可能把 0 顶回非 0，重新打开加并发逻辑。改这块优先走数据库，或先确认表单不会覆盖。
- 另注：`parseProviderDefaultGrantSettings` 在键缺失/解析失败时回落到常量 `defaultAuthSourceConcurrency = 5`，所以是把值设为 `0`，**不能删键**。

## 7. 生产数据修正

| 时间 | 操作 | 行数 |
|---|---|---|
| 18:22:08 | 生效中权益快照回填到套餐现值（只升不降） | 25 |
| 18:31:11 | 补回填未开始的排队 term（user_id=115，Latte，窗口 07-30 → 08-29） | 1 |

第二批是 review 改动 B 时发现第一批的 `starts_at <= now()` 条件漏掉了排队 term，随即用同一规则补齐。修正后校验：未过期权益中 `快照 <> 套餐值` 的行数为 **0**。

缓存处理：直接改库不会触发应用的 auth 缓存失效，因此按 API Key 哈希删除了受影响用户的 `apikey:auth:*` 条目（键格式 `apikey:auth:<alg>:<hash>`，`alg` 取 `lookup-sha256` 或 `key_hash_alg`）。实际命中 3 条，其余本无缓存。L1 为进程内 15s，L2 为 Redis 300s，故最迟十几秒内全量按新值生效，无需重启。

## 8. 影响面

改动 A 部署后，上限上升的用户（基础并发高于套餐权益）：

| 基础并发 | 原生效值 | 新生效值 | 人数 |
|---|---|---|---|
| 8 | 4 | 8 | 20 |
| 16 | 4 | 16 | 6 |
| 32 | 4 | 32 | 1 |

**这 27 人的基础并发经确认保持不动，不做回收。** 其来源混杂：仅 4 人有 LinuxDo 首绑记录，13 人有管理员/兑换调整记录，10 人无任何痕迹（早期默认值遗留，无审计可查）。

生效验证：部署后用户 43 的 `concurrency:user:43` 实时占用达到 7，突破旧上限 4。

另有 381 个用户无生效套餐，生效值等于基础并发，与本次口径无关；其中 35 人基础并发高于 4。

## 9. 后续注意

1. **`cafecode-v0.0.37` 尚未部署**。在部署前再次修改套餐并发，已有订阅仍不会自动跟上，需要手工回填（SQL 见第 7 节口径：`expires_at > now()` + 订阅 active + `p.concurrency > e.concurrency`）。
2. 套餐并发一度全为 4、与 `default_concurrency` 相同，那种状态下套餐并发是空转参数（生效值恒等于基础并发）。当前 Latte 8 / Specialty 16 / Frappé 8 已形成档位。
3. 并发生效口径有 Go 与 SQL 两处实现（第 4.1 节），改动需同步。
4. 后台"认证来源默认值"表单的 `min="1"` 与 LinuxDo 并发 0 的冲突（第 6.2 节）尚未从代码层面修掉。

## 10. 相关文件

- 后端：`internal/service/user.go`、`internal/service/payment_config_plans.go`、`internal/service/payment_config_service.go`、`internal/service/wire.go`、`internal/repository/user_repo.go`、`internal/service/auth_oauth_first_bind.go`（未改，仅涉及）
- 前端：`i18n/locales/{zh,en}.ts`、`types/index.ts`、`views/admin/UsersView.vue`（未改，仅涉及）、`views/admin/SettingsView.vue`（未改，见第 6.2 节坑）
- 测试：`internal/service/user_plan_concurrency_test.go`、`internal/service/payment_config_plan_concurrency_test.go`、`internal/repository/user_repo_plan_concurrency_unit_test.go`
- 容器更新记录：`容器更新历史.md`（v0.0.36 更新后验证一节，含回退 digest 与命令）
