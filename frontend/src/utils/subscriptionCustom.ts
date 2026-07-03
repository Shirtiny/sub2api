import type { UserSubscription } from '@/types'

function hasActiveVirtualCustomEntitlement(subscription: UserSubscription | null | undefined): boolean {
  const expiresAt = subscription?.custom_expires_at
  if (!expiresAt) return true
  const time = Date.parse(expiresAt)
  return Number.isNaN(time) || time > Date.now()
}

export function subscriptionCustomMultiplier(subscription: UserSubscription | null | undefined): number | null {
  if (!hasActiveVirtualCustomEntitlement(subscription)) return null
  const raw = subscription?.custom_multiplier ?? subscription?.group?.custom_multiplier
  const multiplier = Number(raw)
  return Number.isFinite(multiplier) && multiplier >= 1 ? multiplier : null
}

export function subscriptionCustomSourcePlanId(subscription: UserSubscription | null | undefined): number | null {
  if (!hasActiveVirtualCustomEntitlement(subscription)) return null
  const raw = subscription?.custom_source_plan_id ?? subscription?.group?.custom_source_plan_id
  const id = Number(raw)
  return Number.isFinite(id) && id > 0 ? id : null
}

export function subscriptionCustomSourceGroupId(subscription: UserSubscription | null | undefined): number | null {
  if (!hasActiveVirtualCustomEntitlement(subscription)) return null
  const raw = subscription?.custom_source_group_id ?? subscription?.group?.custom_source_group_id
  const id = Number(raw)
  return Number.isFinite(id) && id > 0 ? id : null
}

export function subscriptionCustomDisplayName(subscription: UserSubscription | null | undefined): string {
  const displayName = hasActiveVirtualCustomEntitlement(subscription) ? subscription?.custom_display_name?.trim() : ''
  if (displayName) return displayName
  return subscription?.group?.name?.trim() || ''
}

export function isCustomSubscription(subscription: UserSubscription | null | undefined): boolean {
  return subscriptionCustomMultiplier(subscription) != null && subscriptionCustomSourcePlanId(subscription) != null
}

export function isCustomSubscriptionForPlan(subscription: UserSubscription | null | undefined, planId: number | null | undefined): boolean {
  if (!planId) return false
  return subscriptionCustomSourcePlanId(subscription) === planId && subscriptionCustomMultiplier(subscription) != null
}
