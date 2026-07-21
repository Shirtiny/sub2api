import type { UserSubscription } from '@/types'

const ONE_DAY_MS = 24 * 60 * 60 * 1000

export interface RemainingDurationParts {
  days: number
  hours: number
  minutes: number
}

export function isOneTimeDailyQuota(
  subscription: Pick<UserSubscription, 'starts_at' | 'expires_at'>
): boolean {
  if (!subscription.starts_at || !subscription.expires_at) return false

  const startsAt = new Date(subscription.starts_at).getTime()
  const expiresAt = new Date(subscription.expires_at).getTime()

  if (!Number.isFinite(startsAt) || !Number.isFinite(expiresAt)) return false

  return expiresAt <= startsAt + ONE_DAY_MS
}

/**
 * 用量比值对应的进度条配色，是使用率配色的唯一事实源。
 * 阈值：>=90% 红、>=70% 橙、其余绿。
 *
 * 注意 ratio 可以大于 1：管理员中途重置配额后，汇总表记的是实际花费、
 * 订阅计数器记的是当前窗口计费额，两者本就会背离。
 */
export function ratioToneClass(ratio: number | null | undefined): string {
  const value = ratio ?? 0
  if (value >= 0.9) return 'bg-red-500'
  if (value >= 0.7) return 'bg-orange-500'
  return 'bg-green-500'
}

export function getRemainingDurationParts(
  targetAt: Date | string,
  now: Date = new Date()
): RemainingDurationParts | null {
  const targetTime = targetAt instanceof Date ? targetAt.getTime() : new Date(targetAt).getTime()
  const nowTime = now.getTime()

  if (!Number.isFinite(targetTime) || !Number.isFinite(nowTime)) return null

  const diffMs = targetTime - nowTime
  if (diffMs <= 0) return null

  const totalMinutes = Math.floor(diffMs / (1000 * 60))
  const days = Math.floor(totalMinutes / (24 * 60))
  const hours = Math.floor((totalMinutes % (24 * 60)) / 60)
  const minutes = totalMinutes % 60

  return { days, hours, minutes }
}
