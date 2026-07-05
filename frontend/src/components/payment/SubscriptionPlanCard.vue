<template>
  <div
    :class="[
      'group relative flex flex-col overflow-hidden rounded-2xl border transition-colors',
      borderClass,
      'bg-surface-card',
    ]"
  >
    <!-- Colored top accent bar -->
    <div :class="['h-1.5', accentClass]" />

    <div class="flex flex-1 flex-col p-4">
      <!-- Header: name + badge + price -->
      <div class="mb-3 flex items-start justify-between gap-2">
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <h3 class="truncate text-base font-bold text-content-primary">{{ plan.name }}</h3>
            <span :class="['shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium', badgeLightClass]">
              {{ pLabel }}
            </span>
            <span
              v-if="isRenewal && effectiveMultiplier > 1"
              class="inline-flex shrink-0 items-center rounded-full bg-[#F5C66B]/15 px-2 py-0.5 text-[11px] font-bold text-[#3D2E2A] dark:bg-[#F5C66B]/10 dark:text-[#F5C66B]"
            >
              {{ effectiveMultiplier }}x
            </span>
          </div>
          <p v-if="plan.description" class="mt-0.5 text-xs leading-relaxed text-content-tertiary line-clamp-2">
            {{ plan.description }}
          </p>
        </div>
        <div class="shrink-0 text-right">
          <div v-if="couponDisplayPrice != null" class="flex items-baseline justify-end gap-1.5">
            <span class="text-xs text-gray-400 line-through dark:text-dark-500">&yen;{{ formatPlanAmount(effectivePrice) }}</span>
            <span class="text-xs text-content-tertiary">&yen;</span>
            <span class="text-2xl font-extrabold tracking-tight text-[#3D2E2A] dark:text-[#F5C66B]">{{ formatPlanAmount(couponDisplayPrice) }}</span>
          </div>
          <div v-else class="flex items-baseline justify-end gap-1">
            <span class="text-xs text-content-tertiary">&yen;</span>
            <span :class="['text-2xl font-extrabold tracking-tight', textClass]">{{ formatPlanAmount(effectivePrice) }}</span>
          </div>
          <span class="text-[11px] text-content-tertiary">/ {{ validitySuffix }}</span>
          <div v-if="effectiveOriginalPrice" class="mt-0.5 flex items-center justify-end gap-1.5">
            <span class="text-xs text-gray-400 line-through dark:text-dark-500">&yen;{{ formatPlanAmount(effectiveOriginalPrice) }}</span>
            <span :class="['rounded px-1 py-0.5 text-[10px] font-semibold', discountClass]">{{ discountText }}</span>
          </div>
        </div>
      </div>

      <!-- Group quota info (compact) -->
      <div class="mb-3 grid grid-cols-2 gap-x-3 gap-y-1 rounded-lg bg-gray-50 px-3 py-2 text-xs dark:bg-dark-700/50">
        <div class="flex items-center justify-between">
          <span class="text-content-tertiary">{{ t('payment.planCard.rate') }}</span>
          <span data-testid="plan-rate-display" class="font-medium text-content-secondary">{{ rateDisplay }}</span>
        </div>
        <div v-if="effectiveDailyLimit != null" class="flex items-center justify-between">
          <span class="text-content-tertiary">{{ t('payment.planCard.dailyLimit') }}</span>
          <span class="font-medium text-content-secondary">${{ effectiveDailyLimit }}</span>
        </div>
        <div v-if="effectiveWeeklyLimit != null" class="flex items-center justify-between">
          <span class="text-content-tertiary">{{ t('payment.planCard.weeklyLimit') }}</span>
          <span class="font-medium text-content-secondary">${{ effectiveWeeklyLimit }}</span>
        </div>
        <div v-if="effectiveMonthlyLimit != null" class="flex items-center justify-between">
          <span class="text-content-tertiary">{{ t('payment.planCard.monthlyLimit') }}</span>
          <span class="font-medium text-content-secondary">${{ effectiveMonthlyLimit }}</span>
        </div>
        <div v-if="effectiveDailyLimit == null && effectiveWeeklyLimit == null && effectiveMonthlyLimit == null" class="flex items-center justify-between">
          <span class="text-content-tertiary">{{ t('payment.planCard.quota') }}</span>
          <span class="font-medium text-content-secondary">{{ t('payment.planCard.unlimited') }}</span>
        </div>
        <div v-if="modelScopeLabels.length > 0" class="col-span-2 flex items-center justify-between">
          <span class="text-content-tertiary">{{ t('payment.planCard.models') }}</span>
          <div class="flex flex-wrap justify-end gap-1">
            <span v-for="scope in modelScopeLabels" :key="scope"
              class="rounded bg-gray-200/80 px-1.5 py-0.5 text-[10px] font-medium text-gray-600 dark:bg-dark-600 dark:text-gray-300">
              {{ scope }}
            </span>
          </div>
        </div>
      </div>

      <!-- Features list (compact) -->
      <div v-if="plan.features.length > 0" class="mb-3 space-y-1">
        <div v-for="feature in plan.features" :key="feature" class="flex items-start gap-1.5">
          <svg :class="['mt-0.5 h-3.5 w-3.5 flex-shrink-0', iconClass]" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
          </svg>
          <span class="text-xs text-gray-600 dark:text-gray-300">{{ feature }}</span>
        </div>
      </div>

      <div class="flex-1" />

      <div class="flex items-center gap-2">
        <div v-if="showMultiplierSelector" class="min-w-[88px]">
          <span class="sr-only">{{ t('payment.planCard.multiplier') }}</span>
          <Select
            :model-value="selectedMultiplier"
            :options="multiplierSelectOptions"
            :placeholder="t('payment.planCard.multiplier')"
            :searchable="false"
            :clearable="false"
            @update:model-value="handleMultiplierUpdate"
          />
        </div>
        <!-- Subscribe Button -->
        <button
          type="button"
          :class="['flex-1 rounded-xl py-2.5 text-sm font-semibold transition-all active:scale-[0.98]', btnClass]"
          @click="emit('select', plan, effectiveMultiplier)"
        >
          {{ isRenewal ? t('payment.renewNow') : t('payment.subscribeNow') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import type { SubscriptionPlan } from '@/types/payment'
import type { UserSubscription } from '@/types'
import { isCustomSubscriptionForPlan, subscriptionCustomMultiplier } from '@/utils/subscriptionCustom'
import {
  platformAccentBarClass,
  platformBadgeLightClass,
  platformBorderClass,
  platformTextClass,
  platformIconClass,
  platformButtonClass,
  platformDiscountClass,
  platformLabel,
} from '@/utils/platformColors'

const props = defineProps<{ plan: SubscriptionPlan; activeSubscriptions?: UserSubscription[]; couponPayAmount?: number | null }>()
const emit = defineEmits<{
  select: [plan: SubscriptionPlan, multiplier: number]
  'multiplier-change': [plan: SubscriptionPlan, multiplier: number]
}>()
const { t } = useI18n()

const platform = computed(() => props.plan.group_platform || '')

function multiplierMin(plan: SubscriptionPlan): number {
  return Math.max(1, Number(plan.custom_multiplier_min ?? 1))
}

function multiplierMax(plan: SubscriptionPlan): number {
  return Math.max(multiplierMin(plan), Number(plan.custom_multiplier_max || multiplierMin(plan)))
}

function roundAmount(value: number): number {
  return Math.round(value * 100) / 100
}

function formatPlanAmount(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(Number(value))) return ''
  const rounded = roundAmount(Number(value))
  if (Number.isInteger(rounded)) return String(rounded)
  return rounded.toFixed(2)
}

function multiplyLimit(value: number | null | undefined, multiplier: number): number | null {
  if (value == null) return null
  return roundAmount(value * multiplier)
}

function isSubscriptionCurrentlyActive(subscription: UserSubscription): boolean {
  if (subscription.status !== 'active') return false
  if (!subscription.expires_at) return true
  const expiresAt = Date.parse(subscription.expires_at)
  return Number.isFinite(expiresAt) && expiresAt > Date.now()
}

const selectedMultiplier = ref(multiplierMin(props.plan))

watch(() => props.plan.id, () => {
  selectedMultiplier.value = multiplierMin(props.plan)
})

const customRenewalSubscription = computed(() =>
  props.activeSubscriptions?.find(s =>
    isSubscriptionCurrentlyActive(s)
      && isCustomSubscriptionForPlan(s, props.plan.id),
  ) ?? null,
)

const normalRenewalSubscription = computed(() => {
  if (props.plan.custom_multiplier_enabled === true) return null
  return props.activeSubscriptions?.find(s =>
    isSubscriptionCurrentlyActive(s)
      && s.group_id === props.plan.group_id
      && s.group?.is_custom_subscription_group !== true,
  ) ?? null
})

const isRenewal = computed(() => !!customRenewalSubscription.value || !!normalRenewalSubscription.value)
const showMultiplierSelector = computed(() => props.plan.custom_multiplier_enabled === true && !isRenewal.value)
const multiplierOptions = computed(() => {
  if (!showMultiplierSelector.value) return []
  const min = multiplierMin(props.plan)
  const max = multiplierMax(props.plan)
  return Array.from({ length: max - min + 1 }, (_, index) => min + index)
})
const multiplierSelectOptions = computed<SelectOption[]>(() =>
  multiplierOptions.value.map(value => ({ value, label: `${value}x` })),
)

function handleMultiplierUpdate(value: string | number | boolean | null): void {
  const nextMultiplier = Number(value)
  if (!Number.isFinite(nextMultiplier)) return
  if (!multiplierOptions.value.includes(nextMultiplier)) return
  selectedMultiplier.value = nextMultiplier
}

const effectiveMultiplier = computed(() => {
  const customMultiplier = subscriptionCustomMultiplier(customRenewalSubscription.value)
  if (customMultiplier && customMultiplier >= 1) return customMultiplier
  if (showMultiplierSelector.value) return selectedMultiplier.value
  return 1
})

watch(effectiveMultiplier, multiplier => {
  emit('multiplier-change', props.plan, multiplier)
}, { immediate: true })

// Derived color classes from central config
const accentClass = computed(() => platformAccentBarClass(platform.value))
const borderClass = computed(() => platformBorderClass(platform.value))
const badgeLightClass = computed(() => platformBadgeLightClass(platform.value))
const textClass = computed(() => platformTextClass(platform.value))
const iconClass = computed(() => platformIconClass(platform.value))
const btnClass = computed(() => platformButtonClass(platform.value))
const discountClass = computed(() => platformDiscountClass(platform.value))
const pLabel = computed(() => platformLabel(platform.value))

const effectivePrice = computed(() => roundAmount(props.plan.price * effectiveMultiplier.value))
const couponDisplayPrice = computed(() => {
  if (props.couponPayAmount == null) return null
  const value = Number(props.couponPayAmount)
  if (!Number.isFinite(value)) return null
  const rounded = roundAmount(Math.max(0, value))
  return rounded < effectivePrice.value ? rounded : null
})
const effectiveOriginalPrice = computed(() => props.plan.original_price ? roundAmount(props.plan.original_price * effectiveMultiplier.value) : null)
const effectiveDailyLimit = computed(() => multiplyLimit(props.plan.daily_limit_usd, effectiveMultiplier.value))
const effectiveWeeklyLimit = computed(() => multiplyLimit(props.plan.weekly_limit_usd, effectiveMultiplier.value))
const effectiveMonthlyLimit = computed(() => multiplyLimit(props.plan.monthly_limit_usd, effectiveMultiplier.value))

const discountText = computed(() => {
  if (!effectiveOriginalPrice.value || effectiveOriginalPrice.value <= 0) return ''
  const pct = Math.round((1 - effectivePrice.value / effectiveOriginalPrice.value) * 100)
  return pct > 0 ? `-${pct}%` : ''
})

const rateDisplay = computed(() => {
  const rate = props.plan.rate_multiplier ?? 1
  return `${Number(rate.toPrecision(10))}x`
})

const MODEL_SCOPE_LABELS: Record<string, string> = {
  claude: 'Claude',
  gemini_text: 'Gemini',
  gemini_image: 'Imagen',
}

const modelScopeLabels = computed(() => {
  if (platform.value !== 'antigravity') return []
  const scopes = props.plan.supported_model_scopes
  if (!scopes || scopes.length === 0) return []
  return scopes.map(s => MODEL_SCOPE_LABELS[s] || s)
})

const validitySuffix = computed(() => {
  const u = props.plan.validity_unit || 'day'
  if (u === 'month') return t('payment.perMonth')
  if (u === 'year') return t('payment.perYear')
  return `${props.plan.validity_days}${t('payment.days')}`
})
</script>
