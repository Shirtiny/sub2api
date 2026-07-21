/**
 * 生成一次性幂等键，用于写操作的 `Idempotency-Key` 请求头。
 *
 * 后端只从请求头取 key，不传就完全不去重。必须在**用户每次主动触发写操作时**
 * 现场生成一个新 key：在模块或组件初始化时生成一次复用，会让同一个 key 在后端
 * 幂等 TTL（24 小时）内一直命中缓存，用户的第二次合法操作会被静默吞掉。
 *
 * @param prefix 操作名前缀，便于在后端日志里区分来源
 */
export function createIdempotencyKey(prefix: string): string {
  const random =
    typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(36).slice(2)}`
  return `${prefix}-${random}`
}
