# Codex Pro Prompt Cache 调查记录（2026-07-07）

> 本文只记录本次调查的事实、依据、样本范围和边界。本文不包含优化方案、实施建议或生产变更步骤。

## 1. 调查背景

2026-07-06 至 2026-07-07 期间，观察到 Codex Pro 号池的 OpenAI Responses 请求存在聚合缓存命中率低于单条高命中样本观感的情况。用户侧关注点包括：

- 单条记录中大量请求命中率约 98%，但聚合命中率被拉低。
- 低命中请求是否来自 mini、小样本、压缩请求或尾部请求。
- `prompt_cache_key` 是否存在接入问题。
- `client_metadata` / `turn_id` 变化是否会影响 prompt cache。
- 官方文档对 prompt cache prefix、`prompt_cache_retention`、`24h` / `in_memory` 的说明。

## 2. 数据来源与范围

### 2.1 Aether 侧

使用 Aether 生产库中的只读数据：

- `usage`
- `usage_http_audits`
- `usage_body_blobs`

请求体捕获已在线上开启，调查使用的是 gzip 压缩后的 `provider_request_body` / `request_body` 捕获内容。分析输出只使用字段名、hash、结构、token 数、时间、命中率等派生信息，未记录 prompt 正文。

主要过滤条件：

```text
provider_name = 'Codex Pro'
api_format = 'openai:responses'
endpoint_api_format = 'openai:responses'
created_at >= 2026-07-07 09:28:00 +08
input_tokens > 0
```

部分统计排除了：

- `memory`
- `compaction`
- mini 模型
- 小上下文请求
- body capture 截断占位对象

### 2.2 sub2api 侧

调查中确认这些请求从 sub2api 进入 Aether。sub2api `usage_logs` 中没有保存完整请求体或 header；追溯时通过以下维度做近似匹配：

- cafecode 用户标识 / uid
- Aether 完成时间与 sub2api `created_at`
- `sub2api.input_tokens + sub2api.cache_read_tokens == Aether.input_tokens`
- endpoint/model/account/channel 等字段

先前抽样中，Codex Pro Responses 低命中样本可与 sub2api 记录精确匹配，说明这些请求链路为：

```text
client -> sub2api -> Aether -> Codex Pro upstream
```

## 3. 官方文档依据

### 3.1 Prompt Caching 官方说明

OpenAI Prompt Caching 文档说明：

- 缓存命中依赖 prompt 初始部分的 exact prefix match。
- 静态内容应放在 prompt 前部，变量内容放在后部。
- tools、images、structured output schema 等也会参与可缓存内容。
- 请求路由基于 prompt 初始 prefix 的 hash；该 hash 通常使用前 256 tokens，具体长度随模型变化。
- `prompt_cache_key` 会与 prefix hash 组合，用于影响路由和提高命中率。
- 相同 `prefix + prompt_cache_key` 的请求速率过高时，约超过 15 requests/min，可能 overflow 到其他机器，降低缓存效果。
- Prompt cache 对 1024 tokens 以上的 prompt 自动启用。

来源：

- https://developers.openai.com/api/docs/guides/prompt-caching.md

### 3.2 `prompt_cache_retention` 官方说明

官方文档说明 `prompt_cache_retention` 可用于配置 prompt cache retention policy。

Responses API 参数文档中，该字段枚举值包括：

```text
in_memory
24h
```

Prompt Caching 文档说明：

- `in_memory`：保存在易失 GPU memory 中，通常 5 至 10 分钟不活跃后过期，最长可到 1 小时。
- `24h`：Extended Prompt Caching，缓存前缀最长可保持 24 小时。
- 对 `gpt-5.5`、`gpt-5.5-pro` 和 future models，官方说明 only `24h` is supported。
- 对同时支持 `in_memory` 与 `24h` 的旧模型，默认值取决于组织的数据保留策略：
  - 非 ZDR 组织默认 `24h`。
  - ZDR 组织不指定时默认 `in_memory`。

来源：

- https://developers.openai.com/api/docs/guides/prompt-caching.md
- https://developers.openai.com/api/docs/api-reference/responses/create#responses-create-prompt_cache_retention

### 3.3 `client_metadata` / `turn_id` 的官方覆盖情况

在本次查阅的官方 Prompt Caching 文档和 Responses create API 文档中，未看到公开说明称：

```text
client_metadata.turn_id
client_metadata.x-codex-turn-metadata
HTTP header x-codex-turn-metadata
```

会参与 prompt cache prefix 或 prompt cache lookup。

Responses create 官方公开参数中可见 `metadata`，但本次未在公开文档中确认 `client_metadata` 是通用公开 Responses API 参数。

## 4. 请求体结构观察

### 4.1 `prompt_cache_key`

在有效 `provider_request_body` 样本中：

```text
有效样本数：2595
prompt_cache_key 存在：2595 / 2595
prompt_cache_key == client_session_affinity.session_key：2494 / 2595
```

观察到的 `prompt_cache_key` 多数等于 Codex session id，且与 Aether `client_session_affinity.session_key` 对齐。

### 4.2 `prompt_cache_retention`

在同一批有效 `provider_request_body` 样本中：

```text
prompt_cache_retention 出现次数：0 / 2595
```

进一步比较 `request_body` 与 `provider_request_body`：

```text
request_body 中 prompt_cache_retention 出现次数：0
provider_request_body 中 prompt_cache_retention 出现次数：0
```

也就是说，本次样本中没有看到客户端传入该字段，也没有看到 Aether 转发该字段。

### 4.3 Aether Codex special body edits 中的字段处理

Aether 代码中存在 Codex Responses special body edits 逻辑，文件：

```text
crates/aether-ai-formats/src/formats/openai/responses/codex.rs
```

其中 `CODEX_OPENAI_RESPONSES_UNSUPPORTED_BODY_FIELDS` 包含：

```text
prompt_cache_retention
previous_response_id
metadata
user
...
```

本次查看的 Codex Pro endpoint `body_rules` 未包含 `prompt_cache_retention` 相关规则。

### 4.4 `client_metadata` 字段

非压缩样本中，`client_metadata` 常见字段包括：

```text
x-codex-window-id
x-codex-installation-id
x-codex-turn-metadata
thread_id
turn_id
session_id
x-openai-subagent
x-codex-parent-thread-id
```

其中 `turn_id` 与 `x-codex-turn-metadata` 随 turn 变化频繁。

`client_metadata.x-codex-turn-metadata` 与 HTTP header `x-codex-turn-metadata` 的一致性观察：

```text
body 有 x-codex-turn-metadata：1922
header 有 x-codex-turn-metadata：2288
body/header 同时存在：1848
同时存在且内容相等：1848
diff：0
```

## 5. 低命中定义与样本统计

本次按用户修正后的口径，将低命中定义为：

```text
cache hit rate < 20%
```

在排除 body capture 截断占位后，有效样本统计：

```text
有效样本：2595
low<20 总数：216
非压缩 low<20：197
非压缩、非 mini、大上下文 low<20：156
非压缩、非 mini、大上下文 low<20 miss tokens：9,511,452
```

## 6. 按官方因素映射低命中原因

对 `low<20`、非压缩、非 mini、大上下文样本，按官方文档可解释的因素进行近似归类：

| 归类 | count | miss tokens | 占比 |
|---|---:|---:|---:|
| input 仅尾部追加，前部 prefix 近似稳定 | 84 | 5,640,090 | 59.30% |
| 捕获窗口内无同 pck/key/model 前序请求 | 62 | 3,220,296 | 33.86% |
| input prefix 发生变化 | 4 | 265,392 | 2.79% |
| tools 发生变化 | 5 | 257,489 | 2.71% |
| same prompt 但前序请求仍在进行中 | 1 | 128,185 | 1.35% |

说明：

- “input 仅尾部追加”表示 `instructions/tools/text` 等前置结构相同，前一请求的 `input[]` 是后一请求的前缀，后一请求只在尾部追加新 input 项。
- 这类结构按官方文档属于较符合 prompt caching best practice 的结构，因为变量内容位于尾部，前部静态内容保持一致。
- 该归类不是 OpenAI 内部 cache key 的真实判定，只是基于捕获请求体的近似结构分析。

## 7. `input` 尾部追加样本的命中情况

对所有非压缩、同 `pck/key/model` 的相邻请求，如果满足：

```text
instructions/tools/text 相同
前一请求 input[] 是后一请求 input[] 的前缀
后一请求只在 input[] 尾部追加内容
```

统计结果：

```text
suffix-only pairs：2367
总 ctx：194,031,168
总 miss：15,353,152
miss_rate：7.91%
```

命中率分布：

| 命中率区间 | 数量 |
|---|---:|
| >=90% | 1811 |
| 80%~90% | 227 |
| 50%~80% | 170 |
| 20%~50% | 66 |
| <20% | 93 |

`low<20` 的 suffix-only pairs 中：

```text
low suffix-only pairs：93
miss tokens：5,679,405
reasoning same：88
reasoning changed：5
client_metadata same：64
client_metadata changed：29
```

该统计显示：

- 大多数 suffix-only pair 是高命中。
- 仍存在少量 suffix-only pair 出现低命中。
- 这些低命中并不完全伴随 `client_metadata` 变化，因为其中 `client_metadata same` 的样本更多。

## 8. `client_metadata` / `turn_id` 与命中的关系

仅 `client_metadata` 变化、其他近似 prompt 结构保持一致的相邻 pair：

```text
metadata-only changed pairs：258
low<20：24
```

命中率分布：

| 命中率区间 | 数量 |
|---|---:|
| >=90% | 148 |
| 80%~90% | 39 |
| 50%~80% | 26 |
| 20%~50% | 21 |
| <20% | 24 |

按变化字段：

| 变化字段 | pair 数 | low<20 | miss_rate |
|---|---:|---:|---:|
| `turn_id,x-codex-turn-metadata` | 157 | 15 | 26.55% |
| `x-codex-turn-metadata` | 101 | 9 | 14.32% |

边界说明：

- `turn_id` 变化是正常 turn 行为。
- 官方文档未确认 `turn_id` / `client_metadata` 参与 prompt cache prefix。
- 样本中大量 `client_metadata` 变化的 pair 仍保持高命中。
- 因此本次调查不能将 `turn_id` 变化认定为低命中的确定原因。

## 9. 官方 15 req/min overflow 因素核查

官方文档提到，相同 `prefix + prompt_cache_key` 组合超过约 15 requests/min 时可能 overflow。

本次按多种近似 prefix 粒度统计 60 秒内同组前序请求数：

```text
pck + base + model + key：max prev60 = 9
pck + base + model：max prev60 = 9
pck + base_no_text + model + key：max prev60 = 9
pck + first1 + model + key：max prev60 = 9
pck + model + key：max prev60 = 9
```

在这些近似口径下：

```text
超过 15 req/min 的请求数：0
```

边界说明：

- 这里的 prefix 是基于请求体字段的近似 hash，不是 OpenAI 内部真实 prefix hash。
- 在本次样本窗口中，未观察到符合官方 15 req/min overflow 描述的明显证据。

## 10. 捕获窗口内首次出现样本

在 `low<20`、非压缩、非 mini、大上下文样本中，有一类为捕获窗口内无同 `pck/key/model` 前序请求：

```text
数量：62
miss tokens：3,220,296
```

进一步回看 2026-07-06 00:00:00 +08 之后的 `usage` 记录：

| 情况 | 数量 | miss tokens |
|---|---:|---:|
| lookback 内无同 session/key/model 前序 | 44 | 1,655,650 |
| lookback 内存在前序高命中 | 11 | 892,855 |
| lookback 内存在前序低命中 | 6 | 654,001 |
| lookback 内存在前序中等命中 | 1 | 17,790 |

边界说明：

- 捕获窗口内首次出现不等同于真实首次出现。
- 对捕获开启前的请求，缺少完整请求体，无法判断 prefix 是否相同。

## 11. 请求结构突变与格式转换核查

本次捕获样本中，Codex Pro 低命中样本主要为：

```text
api_format = openai:responses
endpoint_api_format = openai:responses
```

此前对代表性样本比较 `request_body` 与 `provider_request_body`，未发现 Aether 将 chat 转 responses 的格式转换。结构差异主要来自客户端/sub2api 输入的 Responses 请求本身，Aether 侧主要进行字段清理、默认值设置、工具字段兼容处理等。

sub2api 侧也未观察到保存请求体的证据，且相关账号类型为 `openai/apikey`，不属于 OAuth Codex transform 路径。sub2api 代码中 `applyCodexClientMetadata` 仅出现在 OAuth 相关路径，且为追加 installation id，不覆盖已有 `client_metadata`。

## 12. 已确认事实摘要

1. 本次有效 Codex Pro Responses 样本中，`prompt_cache_key` 全部存在。
2. 大多数 `prompt_cache_key` 与 `client_session_affinity.session_key` 一致。
3. 本次样本中没有观察到 `prompt_cache_retention` 被发送到 provider request。
4. 官方文档确认 prompt cache 依赖 prompt 初始 prefix exact match，`prompt_cache_key` 影响路由。
5. 官方文档确认 `prompt_cache_retention` 支持 `in_memory` 与 `24h`，并说明 `gpt-5.5` 只支持 `24h`。
6. 官方文档未确认 `client_metadata.turn_id` 或 `x-codex-turn-metadata` 参与 prompt cache prefix。
7. `client_metadata` 变化与低命中存在相关性，但大量 `client_metadata` 变化样本仍高命中，不能据此确认因果。
8. 本次近似统计未发现超过官方约 15 req/min overflow 阈值的证据。
9. 低命中大头中，最多的是“input 尾部追加、前部结构近似稳定”的样本；这类样本多数仍高命中，但少数 miss 很大。
10. 部分低命中样本在捕获窗口内无同 `pck/key/model` 前序请求；捕获窗口外是否存在相同 prefix 无法完全确认。

## 13. 边界与不确定性

- 本次无法访问 OpenAI 内部真实 prompt prefix hash、cache lookup 机器、cache eviction 事件或 overflow 事件。
- 本次 prefix 判断基于请求体结构和 hash，是外部近似分析。
- 请求体捕获有大小限制，超限样本只保留截断占位，已在部分统计中排除。
- 公开文档没有覆盖 Codex 私有 metadata/header 的全部内部语义。
- `gpt-5.5` 不传 `prompt_cache_retention` 时的实际默认行为，根据官方文档语义推断应与只支持 `24h` 有关，但本次无法直接从 OpenAI 返回中确认内部 retention policy。
- 本文没有记录 prompt 正文、工具正文或用户内容。
