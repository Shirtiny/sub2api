/**
 * 格式化缓存 token 数量（1K/1M 缩写）
 */
export function formatCacheTokens(tokens: number): string {
  if (tokens >= 1000000) return `${(tokens / 1000000).toFixed(1)}M`
  if (tokens >= 1000) return `${(tokens / 1000).toFixed(1)}K`
  return tokens.toLocaleString()
}

interface CacheHitRateUsage {
  input_tokens?: number | null
  cache_creation_tokens?: number | null
  cache_read_tokens?: number | null
}

/**
 * Format cache hit rate against all prompt-side tokens:
 * input + cache creation + cache read.
 */
export function formatCacheHitRate(log: CacheHitRateUsage): string {
  const promptTokens = (log.input_tokens || 0) + (log.cache_creation_tokens || 0) + (log.cache_read_tokens || 0)
  if (promptTokens <= 0) return '0.0%'
  return `${(((log.cache_read_tokens || 0) / promptTokens) * 100).toFixed(1)}%`
}

/**
 * 自适应精度格式化倍率（确保小数值如 0.001 不被截断）
 */
export function formatMultiplier(val: number): string {
  if (val >= 0.01) return val.toFixed(2)
  if (val >= 0.001) return val.toFixed(3)
  if (val >= 0.0001) return val.toFixed(4)
  return val.toPrecision(2)
}
