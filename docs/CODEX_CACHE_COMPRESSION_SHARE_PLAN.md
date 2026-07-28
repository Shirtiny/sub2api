# Codex 缓存命中率：压缩占比展示方案

日期：2026-07-07  
范围：sub2api 缓存命中率统计 / dashboard 轻量增强

## 背景

排查 Aether 线上请求体捕获后发现，Codex Pro 号池里一部分低缓存命中请求不是网关格式转换导致，而是 Codex 客户端在同一个 session 内发出的不同类型 Responses 请求。

这些请求仍然是：

```text
/v1/responses -> /v1/responses
OpenAI Responses 格式
非 chat/responses 转换
```

其中比较影响命中率的后台请求类型是：

```text
x-codex-turn-metadata.request_kind = memory
x-codex-turn-metadata.request_kind = compaction
```

这两类请求通常表现为：

- tools 为空或明显变化；
- parallel_tool_calls 变化；
- input 结构和普通 turn 不同；
- cache_read 很低，常见 0 / 1920 / 4992 / 5888；
- 对整体 miss 贡献高于它们的请求数量占比。

因此希望在 sub2api 的缓存命中率统计里，简单显示一个指标，用来解释“有多少 miss 是压缩类请求贡献的”。

## 产品展示要求

保持简单，只新增一个展示项：

```text
压缩占比：X%
```

建议放在缓存命中率旁边，例如：

```text
缓存命中率：91.2%
压缩占比：14.5%
```

不需要在页面上展开 memory / compaction / turn / unknown 多个分桶。

## 指标口径

### 压缩类请求定义

只按 Codex header 判断：

```text
compression_like = request_kind in ('memory', 'compaction')
```

其中 `request_kind` 来自入站请求头：

```text
x-codex-turn-metadata
```

该 header 是 JSON，字段示例：

```json
{"request_kind":"memory"}
```

或者：

```json
{"request_kind":"compaction"}
```

### 展示指标公式

页面只展示：

```text
压缩占比 = 压缩类请求 miss_tokens / 全部请求 miss_tokens
```

其中 `miss_tokens` 必须沿用当前缓存命中率统计里的同一套 token 口径，不要另起一套算法。

如果当前统计口径是：

```text
context_tokens = input_tokens + cache_read_tokens + cache_creation_tokens + cache_creation_5m_tokens + cache_creation_1h_tokens
hit_tokens     = cache_read_tokens
miss_tokens    = context_tokens - hit_tokens
```

则压缩占比为：

```sql
SUM(miss_tokens) FILTER (WHERE codex_request_kind IN ('memory', 'compaction'))
/
SUM(miss_tokens)
```

如果现有 dashboard 已经封装了 miss token 计算，应直接复用现有计算结果。

## 落库方案

### 最小新增字段

在 `usage_logs` 增加一个字段：

```sql
ALTER TABLE usage_logs
ADD COLUMN codex_request_kind VARCHAR(32);
```

字段含义：

```text
Codex 请求类型，来自 x-codex-turn-metadata.request_kind。
常见值：turn / memory / compaction。
为空表示请求没有携带该字段或不是 Codex 请求。
```

建议索引可选：

```sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_codex_request_kind_created_at
ON usage_logs (codex_request_kind, created_at);
```

如果 dashboard 查询窗口主要按 `created_at` 过滤，且数据量当前可接受，也可以先不加索引，后续再观察。

## 写入逻辑

在处理 OpenAI Responses 请求时，从入站 header 提取：

```text
x-codex-turn-metadata
```

解析 JSON，读取：

```text
request_kind
```

然后写入 `usage_logs.codex_request_kind`。

伪代码：

```go
func extractCodexRequestKind(headerValue string) string {
    headerValue = strings.TrimSpace(headerValue)
    if headerValue == "" {
        return ""
    }

    var payload map[string]any
    if err := json.Unmarshal([]byte(headerValue), &payload); err != nil {
        return ""
    }

    kind, _ := payload["request_kind"].(string)
    kind = strings.ToLower(strings.TrimSpace(kind))

    switch kind {
    case "turn", "memory", "compaction":
        return kind
    default:
        return ""
    }
}
```

注意：

- 不需要保存完整 `x-codex-turn-metadata`；
- 不保存 prompt 正文；
- 不保存 workspace 路径等隐私信息；
- 只保存低基数字段 `request_kind`。

## Dashboard 查询示例

示例 SQL 仅表达口径，实际应复用现有缓存命中率查询中的 token 计算。

```sql
WITH usage_window AS (
  SELECT
    *,
    (
      COALESCE(input_tokens, 0)
      + COALESCE(cache_read_tokens, 0)
      + COALESCE(cache_creation_tokens, 0)
      + COALESCE(cache_creation_5m_tokens, 0)
      + COALESCE(cache_creation_1h_tokens, 0)
    ) AS context_tokens
  FROM usage_logs
  WHERE created_at >= $1
    AND created_at < $2
), tokenized AS (
  SELECT
    *,
    GREATEST(context_tokens - COALESCE(cache_read_tokens, 0), 0) AS miss_tokens
  FROM usage_window
)
SELECT
  CASE
    WHEN SUM(miss_tokens) = 0 THEN 0
    ELSE
      SUM(miss_tokens) FILTER (
        WHERE codex_request_kind IN ('memory', 'compaction')
      )::numeric
      / SUM(miss_tokens)
  END AS compression_miss_share
FROM tokenized;
```

前端显示：

```text
压缩占比：round(compression_miss_share * 100, 1)%
```

## 当前线上样本参考

Aether 捕获窗口：`2026-07-07 09:28+08 ~ 调查时刻`  
过滤：`Codex Pro + OpenAI Responses + hit < 20%`

低命中 miss 中：

```text
memory      miss ≈ 1,167,639
compaction  miss ≈ 1,107,170
合计        miss ≈ 2,274,809
```

全量 Codex Pro Responses 样本中：

```text
memory      ctx_share ≈ 0.94%, miss_share ≈ 7.46%
compaction  ctx_share ≈ 0.87%, miss_share ≈ 7.07%
合计        ctx_share ≈ 1.81%, miss_share ≈ 14.53%
```

说明：压缩类请求数量/上下文占比不高，但 miss 贡献明显偏高，所以用“压缩占比”最能解释缓存命中率被拉低的原因。

## 验收标准

1. 新请求写入 `usage_logs.codex_request_kind`。
2. `memory` 和 `compaction` 能被归为压缩类。
3. 缓存命中率页面只新增一个指标：

   ```text
   压缩占比：X%
   ```

4. 不展示复杂分桶。
5. 不保存完整请求体、不保存完整 `x-codex-turn-metadata`。
6. 历史数据该字段为空时，压缩占比按 0 或“无数据”处理，避免影响旧数据查询。

## 后续可选增强

如果后续还需要进一步解释低命中，可再考虑记录：

- tools_len
- input_len
- body_shape_hash

但当前需求不需要，先保持简单。
