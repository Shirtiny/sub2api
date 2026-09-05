<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex justify-center py-12">
        <div
          class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
        ></div>
      </div>

      <!-- Empty State -->
      <div v-else-if="subscriptions.length === 0" class="card p-12 text-center">
        <div
          class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-surface-hover"
        >
          <Icon name="creditCard" size="xl" class="text-gray-400" />
        </div>
        <h3 class="mb-2 text-lg font-semibold text-content-primary">
          {{ t('userSubscriptions.noActiveSubscriptions') }}
        </h3>
        <p class="text-content-tertiary">
          {{ t('userSubscriptions.noActiveSubscriptionsDesc') }}
        </p>
      </div>

      <!-- Subscriptions Grid -->
      <div v-else class="grid gap-6 lg:grid-cols-2">
        <div
          v-for="subscription in subscriptions"
          :key="subscription.id"
          class="overflow-hidden rounded-2xl border bg-surface-card"
          :class="platformBorderClass(subscription.group?.platform || '')"
        >
          <!-- Header -->
          <div
            class="flex items-center justify-between border-b border-gray-100 p-4 dark:border-dark-700"
          >
            <div class="flex min-w-0 flex-1 items-center gap-3">
              <div :class="['h-1.5 w-1.5 shrink-0 rounded-full', platformAccentDotClass(subscription.group?.platform || '')]" />
              <div class="min-w-0">
                <div class="flex min-w-0 items-center gap-2">
                  <h3 class="truncate font-semibold text-content-primary">
                    {{ subscriptionCustomPlanName(subscription) || `Group #${subscription.group_id}` }}
                  </h3>
                  <span :class="['rounded-md border px-2 py-0.5 text-[11px] font-medium', platformBadgeClass(subscription.group?.platform || '')]">
                    {{ platformLabel(subscription.group?.platform || '') }}
                  </span>
                  <span
                    v-if="shouldShowCustomMultiplierBadge(subscription)"
                    data-testid="subscription-custom-multiplier"
                    class="inline-flex shrink-0 items-center rounded-full bg-[#F5C66B]/15 px-2 py-0.5 text-[11px] font-bold text-[#3D2E2A] dark:bg-[#F5C66B]/10 dark:text-[#F5C66B]"
                  >
                    {{ subscriptionCustomMultiplier(subscription) }}x
                  </span>
                </div>
                <p v-if="subscription.group?.description" class="mt-0.5 text-xs text-content-tertiary">
                  {{ subscription.group.description }}
                </p>
              </div>
            </div>
            <div class="flex flex-wrap items-center justify-end gap-2">
              <span
                :class="[
                  'rounded-full px-2 py-0.5 text-xs font-medium',
                  subscription.status === 'active'
                    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
                    : subscription.status === 'expired'
                      ? 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-400'
                      : 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
                ]"
              >
                {{ t(`userSubscriptions.status.${subscription.status}`) }}
              </span>
              <button
                v-if="canShowQuotaReset(subscription)"
                data-testid="subscription-quota-reset"
                :disabled="quotaResetting"
                class="rounded-lg border border-orange-300 px-3 py-1.5 text-xs font-semibold text-orange-700 transition-colors hover:bg-orange-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-orange-700 dark:text-orange-300 dark:hover:bg-orange-950/30"
                @click="openQuotaResetDialog(subscription)"
              >
                {{ t('userSubscriptions.resetQuota') }}
                <span class="ml-1 tabular-nums">({{ subscription.reset_count }})</span>
              </button>
              <button
                v-if="subscription.status === 'active' && subscription.early_reset_enabled && (subscription.early_reset_duration_days || 0) > 0"
                data-testid="subscription-early-reset"
                :disabled="earlyResetting && earlyResetTarget?.id === subscription.id"
                class="rounded-lg border border-red-300 px-3 py-1.5 text-xs font-semibold text-red-600 transition-colors hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-red-700 dark:text-red-400 dark:hover:bg-red-950/30"
                @click="openEarlyResetDialog(subscription)"
              >
                {{ t('userSubscriptions.earlyReset') }}
              </button>
              <button
                v-if="subscription.status === 'active'"
                data-testid="subscription-renew"
                :class="['rounded-lg px-3 py-1.5 text-xs font-semibold text-white transition-colors', platformButtonClass(subscription.group?.platform || '')]"
                @click="router.push({ path: '/purchase', query: renewalQuery(subscription) })"
              >
                {{ t('payment.renewNow') }}
              </button>
            </div>
          </div>

          <!-- Usage Progress -->
          <div class="space-y-4 p-4">
            <!-- Expiration Info -->
            <div v-if="subscriptionEffectiveExpiresAt(subscription)" class="flex items-center justify-between text-sm">
              <span class="text-content-tertiary">{{
                t('userSubscriptions.expires')
              }}</span>
              <span :class="getExpirationClass(subscriptionEffectiveExpiresAt(subscription)!)">
                {{ formatExpirationDate(subscriptionEffectiveExpiresAt(subscription)!) }}
              </span>
            </div>
            <div v-else class="flex items-center justify-between text-sm">
              <span class="text-content-tertiary">{{
                t('userSubscriptions.expires')
              }}</span>
              <span class="text-content-secondary">{{
                t('userSubscriptions.noExpiration')
              }}</span>
            </div>

            <!-- Daily Usage -->
            <div v-if="subscription.group?.daily_limit_usd" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-content-secondary">
                  {{ t('userSubscriptions.daily') }}
                </span>
                <span class="text-sm text-content-tertiary">
                  ${{ (subscription.daily_usage_usd || 0).toFixed(2) }} / ${{
                    subscription.group.daily_limit_usd.toFixed(2)
                  }}
                </span>
              </div>
              <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="
                    getProgressBarClass(
                      subscription.daily_usage_usd,
                      subscription.group.daily_limit_usd
                    )
                  "
                  :style="{
                    width: getProgressWidth(
                      subscription.daily_usage_usd,
                      subscription.group.daily_limit_usd
                    )
                  }"
                ></div>
              </div>
              <p
                v-if="subscription.daily_window_start"
                class="text-xs text-content-tertiary"
              >
                {{ formatDailyUsageWindow(subscription) }}
              </p>
            </div>

            <!-- Weekly Usage -->
            <div v-if="subscription.group?.weekly_limit_usd" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-content-secondary">
                  {{ t('userSubscriptions.weekly') }}
                </span>
                <span class="text-sm text-content-tertiary">
                  ${{ (subscription.weekly_usage_usd || 0).toFixed(2) }} / ${{
                    subscription.group.weekly_limit_usd.toFixed(2)
                  }}
                </span>
              </div>
              <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="
                    getProgressBarClass(
                      subscription.weekly_usage_usd,
                      subscription.group.weekly_limit_usd
                    )
                  "
                  :style="{
                    width: getProgressWidth(
                      subscription.weekly_usage_usd,
                      subscription.group.weekly_limit_usd
                    )
                  }"
                ></div>
              </div>
              <p
                v-if="subscription.weekly_window_start"
                class="text-xs text-content-tertiary"
              >
                {{ formatUsageWindow(subscription, subscription.weekly_window_start, 168) }}
              </p>
            </div>

            <!-- Monthly Usage -->
            <div v-if="subscription.group?.monthly_limit_usd" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-content-secondary">
                  {{ t('userSubscriptions.monthly') }}
                </span>
                <span class="text-sm text-content-tertiary">
                  ${{ (subscription.monthly_usage_usd || 0).toFixed(2) }} / ${{
                    subscription.group.monthly_limit_usd.toFixed(2)
                  }}
                </span>
              </div>
              <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="
                    getProgressBarClass(
                      subscription.monthly_usage_usd,
                      subscription.group.monthly_limit_usd
                    )
                  "
                  :style="{
                    width: getProgressWidth(
                      subscription.monthly_usage_usd,
                      subscription.group.monthly_limit_usd
                    )
                  }"
                ></div>
              </div>
              <p
                v-if="subscription.monthly_window_start"
                class="text-xs text-content-tertiary"
              >
                {{ formatUsageWindow(subscription, subscription.monthly_window_start, 720) }}
              </p>
            </div>

            <!-- No limits configured - Unlimited badge -->
            <div
              v-if="
                !subscription.group?.daily_limit_usd &&
                !subscription.group?.weekly_limit_usd &&
                !subscription.group?.monthly_limit_usd
              "
              class="flex items-center justify-center rounded-xl bg-gradient-to-r from-emerald-50 to-teal-50 py-6 dark:from-emerald-900/20 dark:to-teal-900/20"
            >
              <div class="flex items-center gap-3">
                <span class="text-4xl text-emerald-600 dark:text-emerald-400">∞</span>
                <div>
                  <p class="text-sm font-medium text-emerald-700 dark:text-emerald-300">
                    {{ t('userSubscriptions.unlimited') }}
                  </p>
                  <p class="text-xs text-emerald-600/70 dark:text-emerald-400/70">
                    {{ t('userSubscriptions.unlimitedDesc') }}
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
    <ConfirmDialog
      :show="earlyResetTarget !== null"
      :title="t('userSubscriptions.earlyResetConfirmTitle')"
      :message="earlyResetConfirmMessage"
      :confirm-text="earlyResetting ? t('userSubscriptions.earlyResetting') : t('userSubscriptions.earlyResetConfirm')"
      :confirm-disabled="earlyResetting"
      danger
      @cancel="closeEarlyResetDialog"
      @confirm="confirmEarlyReset"
    />
    <ConfirmDialog
      data-testid="subscription-quota-reset-dialog"
      :show="quotaResetTarget !== null"
      :title="t('userSubscriptions.resetQuotaConfirmTitle')"
      :message="quotaResetConfirmMessage"
      :confirm-text="quotaResetting ? t('userSubscriptions.resetQuotaResetting') : t('userSubscriptions.resetQuotaConfirm')"
      :confirm-disabled="quotaResetting"
      :danger="true"
      @cancel="closeQuotaResetDialog"
      @confirm="confirmQuotaReset"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { useSubscriptionStore } from '@/stores/subscriptions'
import subscriptionsAPI from '@/api/subscriptions'
import type { UserSubscription } from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { formatDateOnly } from '@/utils/format'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { createIdempotencyKey } from '@/utils/idempotency'
import { platformBorderClass, platformBadgeClass, platformButtonClass, platformLabel } from '@/utils/platformColors'
import { getRemainingDurationParts, isOneTimeDailyQuota, type RemainingDurationParts } from '@/utils/subscriptionQuota'
import { subscriptionCustomMultiplier, subscriptionCustomPlanName, subscriptionCustomSourceGroupId, subscriptionCustomSourcePlanId, subscriptionEffectiveExpiresAt } from '@/utils/subscriptionCustom'

function platformAccentDotClass(p: string): string {
  switch (p) {
    case 'anthropic': return 'bg-orange-500'
    case 'openai': return 'bg-emerald-500'
    case 'antigravity': return 'bg-purple-500'
    case 'gemini': return 'bg-blue-500'
    default: return 'bg-gray-400'
  }
}

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()
const subscriptionStore = useSubscriptionStore()

const subscriptions = ref<UserSubscription[]>([])
const loading = ref(true)
const earlyResetTarget = ref<UserSubscription | null>(null)
const earlyResetting = ref(false)
const earlyResetIdempotencyKey = ref<string | null>(null)
const quotaResetTarget = ref<UserSubscription | null>(null)
const quotaResetting = ref(false)
const quotaResetIdempotencyKey = ref<string | null>(null)
const earlyResetConfirmMessage = computed(() => {
  const target = earlyResetTarget.value
  if (!target) return ''
  return t('userSubscriptions.earlyResetConfirmMessage', {
    name: subscriptionCustomPlanName(target) || `Group #${target.group_id}`,
    days: target.early_reset_duration_days || 0
  })
})

const quotaResetConfirmMessage = computed(() => {
  const target = quotaResetTarget.value
  if (!target) return ''
  return t('userSubscriptions.resetQuotaConfirmMessage', {
    name: subscriptionCustomPlanName(target) || `Group #${target.group_id}`,
    remaining: Math.max((target.reset_count || 0) - 1, 0)
  })
})

async function loadSubscriptions() {
  try {
    loading.value = true
    subscriptions.value = await subscriptionsAPI.getMySubscriptions()
  } catch (error) {
    console.error('Failed to load subscriptions:', error)
    appStore.showError(t('userSubscriptions.failedToLoad'))
  } finally {
    loading.value = false
  }
}

function openEarlyResetDialog(subscription: UserSubscription) {
  earlyResetTarget.value = subscription
  earlyResetIdempotencyKey.value = createIdempotencyKey('subscription-early-reset')
}

function closeEarlyResetDialog() {
  if (!earlyResetting.value) {
    earlyResetTarget.value = null
    earlyResetIdempotencyKey.value = null
  }
}

async function confirmEarlyReset() {
  const target = earlyResetTarget.value
  if (!target || earlyResetting.value) return
  earlyResetting.value = true
  try {
    const idempotencyKey =
      earlyResetIdempotencyKey.value ||
      createIdempotencyKey('subscription-early-reset')
    earlyResetIdempotencyKey.value = idempotencyKey
    const updated = await subscriptionsAPI.earlyResetSubscription(target.id, idempotencyKey)
    const index = subscriptions.value.findIndex(subscription => subscription.id === updated.id)
    if (index >= 0) {
      subscriptions.value[index] = updated
    }
    subscriptionStore.syncActiveSubscription(updated)
    appStore.showSuccess(t('userSubscriptions.earlyResetSuccess'))
    earlyResetTarget.value = null
    earlyResetIdempotencyKey.value = null
  } catch (error: unknown) {
    appStore.showError(
      extractI18nErrorMessage(
        error,
        t,
        'userSubscriptions.earlyResetErrors',
        t('userSubscriptions.earlyResetFailed'),
      ),
    )
  } finally {
    earlyResetting.value = false
  }
}

function canShowQuotaReset(subscription: UserSubscription): boolean {
  const count = subscription.reset_count || 0
  const now = Date.now()
  const startsAt = Date.parse(subscription.starts_at)
  const expiresAt = subscription.expires_at ? Date.parse(subscription.expires_at) : Number.POSITIVE_INFINITY
  // The API can return a future subscription with status=active.  Keep the
  // action hidden until the same temporal checks enforced by the backend pass.
  if ((Number.isFinite(startsAt) && startsAt > now) || (Number.isFinite(expiresAt) && expiresAt <= now)) {
    return false
  }
  const group = subscription.group
  const hasLimitMetadata = Boolean(
    group && (
      Object.prototype.hasOwnProperty.call(group, 'subscription_type') ||
      Object.prototype.hasOwnProperty.call(group, 'daily_limit_usd') ||
      Object.prototype.hasOwnProperty.call(group, 'weekly_limit_usd') ||
      Object.prototype.hasOwnProperty.call(group, 'monthly_limit_usd')
    )
  )
  const hasQuotaWindow = !group || !hasLimitMetadata || Boolean(
    (group.daily_limit_usd && group.daily_limit_usd > 0) ||
    (group.weekly_limit_usd && group.weekly_limit_usd > 0)
  )
  // Treat omitted limit metadata as unknown (some legacy responses omit it),
  // but suppress the action when the server explicitly reports no daily or
  // weekly window.
  return (
    subscription.status === 'active' &&
    (!group || !group.subscription_type || group.subscription_type === 'subscription') &&
    !subscription.early_reset_enabled &&
    (subscription.early_reset_duration_days || 0) === 0 &&
    !isOneTimeDailyQuota(subscription) &&
    count > 0 &&
    hasQuotaWindow
  )
}

function openQuotaResetDialog(subscription: UserSubscription) {
  if (!canShowQuotaReset(subscription)) return
  quotaResetTarget.value = subscription
  quotaResetIdempotencyKey.value = createIdempotencyKey('subscription-quota-reset')
}

function closeQuotaResetDialog() {
  if (!quotaResetting.value) {
    quotaResetTarget.value = null
    quotaResetIdempotencyKey.value = null
  }
}

async function confirmQuotaReset() {
  const target = quotaResetTarget.value
  if (!target || quotaResetting.value) return
  quotaResetting.value = true
  try {
    const idempotencyKey =
      quotaResetIdempotencyKey.value || createIdempotencyKey('subscription-quota-reset')
    quotaResetIdempotencyKey.value = idempotencyKey
    const reset = subscriptionsAPI.resetSubscriptionQuota || subscriptionsAPI.resetQuota
    if (!reset) throw new Error('Subscription quota reset is unavailable')
    const updated = await reset(target.id, idempotencyKey)
    const index = subscriptions.value.findIndex(subscription => subscription.id === updated.id)
    if (index >= 0) subscriptions.value[index] = updated
    subscriptionStore.syncActiveSubscription(updated)
    appStore.showSuccess(t('userSubscriptions.resetQuotaSuccess'))
    quotaResetTarget.value = null
    quotaResetIdempotencyKey.value = null
  } catch (error: unknown) {
    appStore.showError(
      extractI18nErrorMessage(
        error,
        t,
        'userSubscriptions.resetQuotaErrors',
        t('userSubscriptions.resetQuotaFailed'),
      ),
    )
  } finally {
    quotaResetting.value = false
  }
}

function shouldShowCustomMultiplierBadge(subscription: UserSubscription): boolean {
  return subscriptionCustomMultiplier(subscription) != null && subscriptionCustomSourcePlanId(subscription) != null
}

function renewalQuery(subscription: UserSubscription): Record<string, string> {
  const query: Record<string, string> = { tab: 'subscription' }
  const sourcePlanId = subscriptionCustomSourcePlanId(subscription)
  if (sourcePlanId) {
    query.plan = String(sourcePlanId)
    const sourceGroupId = subscriptionCustomSourceGroupId(subscription)
    if (sourceGroupId) {
      query.group = String(sourceGroupId)
    }
    const multiplier = subscriptionCustomMultiplier(subscription)
    if (multiplier && multiplier >= 1) {
      query.multiplier = String(multiplier)
    }
    return query
  }
  query.group = String(subscription.group_id)
  return query
}

function getProgressWidth(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return '0%'
  const percentage = Math.min(((used || 0) / limit) * 100, 100)
  return `${percentage}%`
}

function getProgressBarClass(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return 'bg-gray-400'
  const percentage = ((used || 0) / limit) * 100
  if (percentage >= 90) return 'bg-red-500'
  if (percentage >= 70) return 'bg-orange-500'
  return 'bg-green-500'
}

function formatExpirationDate(expiresAt: string): string {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))

  if (days < 0) {
    return t('userSubscriptions.status.expired')
  }

  const dateStr = formatDateOnly(expires)

  if (days === 0) {
    return `${dateStr} (${t('common.today')})`
  }
  if (days === 1) {
    return `${dateStr} (${t('common.tomorrow')})`
  }

  return t('userSubscriptions.daysRemaining', { days }) + ` (${dateStr})`
}

function getExpirationClass(expiresAt: string): string {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))

  if (days <= 0) return 'text-red-600 dark:text-red-400 font-medium'
  if (days <= 3) return 'text-red-600 dark:text-red-400'
  if (days <= 7) return 'text-orange-600 dark:text-orange-400'
  return 'text-content-secondary'
}

function formatDurationParts(parts: RemainingDurationParts): string {
  if (parts.days > 0) {
    return `${parts.days}d ${parts.hours}h`
  }

  if (parts.hours > 0) {
    return `${parts.hours}h ${parts.minutes}m`
  }

  return `${parts.minutes}m`
}

function formatDailyUsageWindow(subscription: UserSubscription): string {
  const effectiveExpiresAt = subscriptionEffectiveExpiresAt(subscription)
  if ((subscription.early_reset_enabled || isOneTimeDailyQuota(subscription)) && effectiveExpiresAt) {
    const parts = getRemainingDurationParts(effectiveExpiresAt)
    if (!parts) return t('userSubscriptions.windowNotActive')
    return t('userSubscriptions.quotaEndsIn', { time: formatDurationParts(parts) })
  }

  return t('userSubscriptions.resetIn', {
    time: formatResetTime(subscription.daily_window_start, 24)
  })
}

function formatUsageWindow(
  subscription: UserSubscription,
  windowStart: string | null,
  windowHours: number
): string {
  const effectiveExpiresAt = subscriptionEffectiveExpiresAt(subscription)
  if (subscription.early_reset_enabled && effectiveExpiresAt) {
    const parts = getRemainingDurationParts(effectiveExpiresAt)
    if (!parts) return t('userSubscriptions.windowNotActive')
    return t('userSubscriptions.quotaEndsIn', { time: formatDurationParts(parts) })
  }

  return t('userSubscriptions.resetIn', {
    time: formatResetTime(windowStart, windowHours)
  })
}

function formatResetTime(windowStart: string | null, windowHours: number): string {
  if (!windowStart) return t('userSubscriptions.windowNotActive')

  const start = new Date(windowStart)
  const end = new Date(start.getTime() + windowHours * 60 * 60 * 1000)
  const parts = getRemainingDurationParts(end)

  return parts ? formatDurationParts(parts) : t('userSubscriptions.windowNotActive')
}

onMounted(() => {
  loadSubscriptions()
})
</script>
