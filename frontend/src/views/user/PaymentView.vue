<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-20">
        <div class="h-8 w-8 animate-spin rounded-full border-4 border-primary-500 border-t-transparent"></div>
      </div>
      <template v-else>
        <!-- Usage warning is always visible after checkout data loads. -->
        <div class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm font-medium leading-6 text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300">
          {{ t('payment.usagePolicyWarning') }}
        </div>
        <!-- Tab Switcher (hide during payment and subscription confirm) -->
        <div v-if="tabs.length > 1 && paymentPhase === 'select' && !selectedPlan" class="flex space-x-1 rounded-xl bg-gray-100 p-1 dark:bg-dark-800">
          <button v-for="tab in tabs" :key="tab.key"
            class="flex-1 rounded-lg px-4 py-2.5 text-sm font-medium transition-all"
            :class="activeTab === tab.key ? 'bg-white text-gray-900 shadow dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-300'"
            @click="activeTab = tab.key">{{ tab.label }}</button>
        </div>
        <!-- Payment in progress (shared by recharge and subscription) -->
        <template v-if="paymentPhase === 'paying'">
          <PaymentStatusPanel
            :order-id="paymentState.orderId"
            :qr-code="paymentState.qrCode"
            :expires-at="paymentState.expiresAt"
            :payment-type="paymentState.paymentType"
            :pay-url="paymentState.payUrl"
            :order-type="paymentState.orderType"
            :currency="paymentState.currency || selectedCurrency"
            @done="onPaymentDone"
            @success="onPaymentSuccess"
            @settled="onPaymentSettled"
          />
        </template>
        <!-- Tab content (select phase) -->
        <template v-else>
          <!-- Top-up Tab -->
          <template v-if="activeTab === 'recharge'">
            <!-- Recharge Account Card -->
            <div class="card p-5">
              <p class="text-xs font-medium text-gray-400 dark:text-gray-500">{{ t('payment.rechargeAccount') }}</p>
              <p class="mt-1 text-base font-semibold text-content-primary">{{ user?.username || '' }}</p>
              <p class="mt-0.5 text-sm font-medium text-green-600 dark:text-green-400">{{ t('payment.currentBalance') }}: {{ user?.balance?.toFixed(2) || '0.00' }}</p>
            </div>
            <div v-if="enabledMethods.length === 0" class="card py-16 text-center">
              <p class="text-gray-500 dark:text-gray-400">{{ t('payment.notAvailable') }}</p>
            </div>
            <template v-else>
            <div class="card p-6">
              <AmountInput
                v-model="amount"
                :amounts="RECHARGE_QUICK_AMOUNTS"
                :min="globalMinAmount"
                :max="globalMaxAmount"
              />
              <p v-if="amountError" class="mt-2 text-xs text-amber-600 dark:text-amber-300">{{ amountError }}</p>
            </div>
            <div v-if="enabledMethods.length >= 1" class="card p-6">
              <PaymentMethodSelector
                :methods="methodOptions"
                :selected="selectedMethod"
                @select="selectedMethod = $event"
              />
            </div>
            <div v-if="validAmount > 0" class="card p-6">
              <div class="space-y-2 text-sm">
                <div class="flex justify-between gap-3">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('payment.paymentAmount') }}</span>
                  <span class="text-right text-content-primary">{{ formatSelectedPaymentAmount(validAmount) }}</span>
                </div>
                <div v-if="rechargeCouponDiscountAmount != null" class="flex justify-between gap-3">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('payment.cafeCoupon.discountLabel') }}</span>
                  <span class="font-semibold text-[#3D2E2A] dark:text-[#F5C66B]">-{{ formatSelectedPaymentAmount(rechargeCouponDiscountAmount) }}</span>
                </div>
                <div v-if="feeRate > 0" class="flex justify-between">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('payment.fee') }} ({{ feeRate }}%)</span>
                  <span class="text-content-primary">{{ formatSelectedPaymentAmount(feeAmount) }}</span>
                </div>
                <div v-if="feeRate > 0" class="flex justify-between border-t border-gray-200 pt-2 dark:border-dark-600">
                  <span class="font-medium text-content-secondary">{{ t('payment.actualPay') }}</span>
                  <span class="text-lg font-bold text-primary-600 dark:text-primary-400">{{ formatSelectedPaymentAmount(rechargeTotalAmount) }}</span>
                </div>
                <div v-if="balanceRechargeMultiplier !== 1" class="flex justify-between" :class="{ 'border-t border-gray-200 pt-2 dark:border-dark-600': feeRate <= 0 }">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('payment.creditedBalance') }}</span>
                  <span class="text-content-primary">${{ creditedAmount.toFixed(2) }}</span>
                </div>
                <p v-if="balanceRechargeMultiplier !== 1" class="border-t border-gray-200 pt-2 text-xs text-gray-500 dark:border-dark-600 dark:text-gray-400">
                  {{ t('payment.rechargeRatePreview', { usd: balanceRechargeMultiplier.toFixed(2) }) }}
                </p>
              </div>
            </div>
            <div v-if="validAmount > 0" class="card p-4">
              <label for="cafe-coupon-recharge" class="input-label">{{ t('payment.cafeCoupon.label') }}</label>
              <div class="relative mt-1">
                <input
                  id="cafe-coupon-recharge"
                  v-model="cafeCouponCode"
                  type="text"
                  name="cafe_coupon_code"
                  autocomplete="off"
                  :placeholder="t('payment.cafeCoupon.placeholder')"
                  :disabled="submitting"
                  class="input pr-10 font-mono uppercase tracking-wide"
                  @input="scheduleCafeCouponAutoPreview"
                  @blur="previewCafeCoupon"
                />
                <span v-if="previewingCafeCoupon" class="absolute inset-y-0 right-3 flex items-center">
                  <span class="h-4 w-4 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></span>
                </span>
              </div>
              <p v-if="cafeCouponError" class="mt-2 text-xs text-red-600 dark:text-red-400">{{ cafeCouponError }}</p>
              <p v-else-if="currentOrderPayAmountBelowMinimum" class="mt-2 text-xs text-red-600 dark:text-red-400">
                {{ minimumPayAmountMessage }}
              </p>
              <p v-else-if="cafeCouponApplied" class="mt-2 text-xs text-emerald-600 dark:text-emerald-400">
                {{ cafeCouponAppliedText }}
              </p>
              <p v-else class="input-hint">{{ t('payment.cafeCoupon.hint') }}</p>
            </div>
            <button :class="['btn w-full py-3 text-base font-medium', paymentButtonClass]" :disabled="!canSubmit || submitting" @click="handleSubmitRecharge">
              <span v-if="submitting" class="flex items-center justify-center gap-2">
                <span class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
                {{ t('common.processing') }}
              </span>
              <span v-else>{{ t('payment.createOrder') }} {{ formatSelectedPaymentAmount(rechargeButtonAmount) }}</span>
            </button>
            </template>
          </template>
          <!-- Subscribe Tab -->
          <template v-else-if="activeTab === 'subscription'">
            <!-- Subscription confirm (inline, replaces plan list) -->
            <template v-if="selectedPlan">
              <div class="card p-5">
                <!-- Header: platform badge + plan name -->
                <div class="mb-3 flex flex-wrap items-center gap-2">
                  <span :class="['rounded-md border px-2 py-0.5 text-xs font-medium', planBadgeClass]">
                    {{ platformLabel(selectedPlan.group_platform || '') }}
                  </span>
                  <h3 class="text-lg font-bold text-content-primary">{{ selectedPlan.name }}</h3>
                  <span
                    v-if="effectiveSelectedMultiplier > 1"
                    class="inline-flex items-center rounded-full bg-[#F5C66B]/15 px-2.5 py-1 text-sm font-bold text-[#3D2E2A] dark:bg-[#F5C66B]/10 dark:text-[#F5C66B]"
                  >
                    {{ effectiveSelectedMultiplier }}x
                  </span>
                </div>
                <!-- Price -->
                <div class="flex flex-wrap items-baseline gap-2">
                  <template v-if="effectiveSelectedCouponPrice != null">
                    <span :class="['text-xl font-semibold line-through decoration-2', planTextClass]">
                      {{ formatSelectedPaymentAmount(effectiveSelectedPlanPrice) }}
                    </span>
                    <span class="text-3xl font-bold text-[#3D2E2A] dark:text-[#F5C66B]">
                      {{ formatSelectedPaymentAmount(effectiveSelectedCouponPrice) }}
                    </span>
                  </template>
                  <template v-else>
                    <span v-if="effectiveSelectedOriginalPrice" class="text-sm text-gray-400 line-through dark:text-gray-500">
                      {{ formatSelectedPaymentAmount(effectiveSelectedOriginalPrice) }}
                    </span>
                    <span :class="['text-3xl font-bold', planTextClass]">{{ formatSelectedPaymentAmount(effectiveSelectedPlanPrice) }}</span>
                  </template>
                  <span class="text-sm text-gray-500 dark:text-gray-400">/ {{ planValiditySuffix }}</span>
                </div>
                <!-- Description -->
                <p v-if="selectedPlan.description" class="mt-2 text-sm leading-relaxed text-gray-500 dark:text-gray-400">
                  {{ selectedPlan.description }}
                </p>
                <!-- Rate + Limits grid -->
                <div class="mt-3 grid grid-cols-2 gap-3">
                  <div>
                    <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.rate') }}</span>
                    <div class="flex items-baseline">
                      <span :class="['text-lg font-bold', planTextClass]">{{ selectedPlanRateDisplay }}</span>
                    </div>
                  </div>
                  <div v-if="effectiveSelectedDailyLimit != null">
                    <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.dailyLimit') }}</span>
                    <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">${{ effectiveSelectedDailyLimit }}</div>
                  </div>
                  <div v-if="effectiveSelectedWeeklyLimit != null">
                    <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.weeklyLimit') }}</span>
                    <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">${{ effectiveSelectedWeeklyLimit }}</div>
                  </div>
                  <div v-if="effectiveSelectedMonthlyLimit != null">
                    <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.monthlyLimit') }}</span>
                    <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">${{ effectiveSelectedMonthlyLimit }}</div>
                  </div>
                  <div v-if="effectiveSelectedDailyLimit == null && effectiveSelectedWeeklyLimit == null && effectiveSelectedMonthlyLimit == null">
                    <span class="text-xs text-gray-400 dark:text-gray-500">{{ t('payment.planCard.quota') }}</span>
                    <div class="text-lg font-semibold text-gray-800 dark:text-gray-200">{{ t('payment.planCard.unlimited') }}</div>
                  </div>
                </div>
                <p
                  v-if="selectedMultiplierConflictsActiveCustom"
                  class="mt-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:border-amber-900/40 dark:bg-amber-900/20 dark:text-amber-300"
                >
                  {{ t('payment.customMultiplierConflict', { current: `${selectedActiveCustomMultiplier}x`, selected: `${effectiveSelectedMultiplier}x` }) }}
                </p>
              </div>
              <div v-if="enabledMethods.length >= 1" class="card p-6">
                <PaymentMethodSelector
                  :methods="subMethodOptions"
                  :selected="selectedMethod"
                  @select="selectedMethod = $event"
                />
              </div>
              <div v-if="effectiveSelectedPlanPrice > 0 && (feeRate > 0 || subscriptionCouponPayableAmount != null)" class="card p-6">
                <div class="space-y-2 text-sm">
                  <div class="flex justify-between gap-3">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('payment.orders.subscriptionAmount') }}</span>
                    <span class="text-right text-content-primary">{{ formatSelectedPaymentAmount(effectiveSelectedPlanPrice) }}</span>
                  </div>
                  <div v-if="subscriptionCouponDiscountAmount != null" class="flex justify-between gap-3">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('payment.cafeCoupon.discountLabel') }}</span>
                    <span class="font-semibold text-[#3D2E2A] dark:text-[#F5C66B]">-{{ formatSelectedPaymentAmount(subscriptionCouponDiscountAmount) }}</span>
                  </div>
                  <div v-if="feeRate > 0" class="flex justify-between">
                    <span class="text-gray-500 dark:text-gray-400">{{ t('payment.fee') }} ({{ feeRate }}%)</span>
                    <span class="text-content-primary">{{ formatSelectedPaymentAmount(subFeeAmount) }}</span>
                  </div>
                  <div class="flex justify-between border-t border-gray-200 pt-2 dark:border-dark-600">
                    <span class="font-medium text-content-secondary">{{ t('payment.actualPay') }}</span>
                    <span class="text-lg font-bold text-primary-600 dark:text-primary-400">{{ formatSelectedPaymentAmount(subTotalAmount) }}</span>
                  </div>
                </div>
              </div>
              <div class="card p-4">
                <label for="cafe-coupon-subscription" class="input-label">{{ t('payment.cafeCoupon.label') }}</label>
                <div class="relative mt-1">
                  <input
                    id="cafe-coupon-subscription"
                    v-model="cafeCouponCode"
                    type="text"
                    name="cafe_coupon_code"
                    autocomplete="off"
                    :placeholder="t('payment.cafeCoupon.placeholder')"
                    :disabled="submitting"
                    class="input pr-10 font-mono uppercase tracking-wide"
                    @input="scheduleCafeCouponAutoPreview"
                    @blur="previewCafeCoupon"
                  />
                  <span v-if="previewingCafeCoupon" class="absolute inset-y-0 right-3 flex items-center">
                    <span class="h-4 w-4 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></span>
                  </span>
                </div>
                <p v-if="cafeCouponError" class="mt-2 text-xs text-red-600 dark:text-red-400">{{ cafeCouponError }}</p>
                <p v-else-if="currentOrderPayAmountBelowMinimum" class="mt-2 text-xs text-red-600 dark:text-red-400">
                  {{ minimumPayAmountMessage }}
                </p>
                <p v-else-if="cafeCouponApplied" class="mt-2 text-xs text-emerald-600 dark:text-emerald-400">
                  {{ cafeCouponAppliedText }}
                </p>
                <p v-else class="input-hint">{{ t('payment.cafeCoupon.hint') }}</p>
              </div>
              <button :class="['btn w-full py-3 text-base font-medium', paymentButtonClass]" :disabled="!canSubmitSubscription || submitting" @click="confirmSubscribe">
                <span v-if="submitting" class="flex items-center justify-center gap-2">
                  <span class="h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
                  {{ t('common.processing') }}
                </span>
                <span v-else>{{ t('payment.createOrder') }} {{ formatSelectedPaymentAmount(subscriptionButtonAmount) }}</span>
              </button>
              <button class="btn btn-secondary w-full" @click="selectedPlan = null">{{ t('common.cancel') }}</button>
            </template>
            <!-- Plan list -->
            <template v-else>
              <div v-if="checkout.plans.length === 0" class="card py-16 text-center">
                <Icon name="gift" size="xl" class="mx-auto mb-3 text-gray-300 dark:text-dark-600" />
                <p class="text-gray-500 dark:text-gray-400">{{ t('payment.noPlans') }}</p>
              </div>
              <div v-else :class="planGridClass">
                <SubscriptionPlanCard
                  v-for="plan in checkout.plans"
                  :key="plan.id"
                  :plan="plan"
                  :active-subscriptions="activeSubscriptions"
                  :coupon-pay-amount="planCardCouponPayAmount(plan)"
                  @select="selectPlan"
                  @multiplier-change="onPlanCardMultiplierChange"
                />
              </div>
              <!-- Active subscriptions (compact, below plan list) -->
              <div v-if="activeSubscriptions.length > 0">
                <p class="mb-2 text-xs font-medium text-gray-400 dark:text-gray-500">{{ t('payment.activeSubscription') }}</p>
                <div class="space-y-2">
                  <div v-for="sub in activeSubscriptions" :key="sub.id"
                    class="flex items-center gap-3 rounded-xl border border-gray-100 bg-white px-3 py-2 dark:border-dark-700 dark:bg-dark-800">
                    <div :class="['h-6 w-1 shrink-0 rounded-full', platformAccentBarClass(sub.group?.platform || '')]" />
                    <div class="min-w-0 flex-1">
                      <div class="flex items-center gap-1.5">
                        <span class="truncate text-xs font-semibold text-content-primary">{{ sub.group?.name || t('payment.groupFallback', { id: sub.group_id }) }}</span>
                        <span :class="['shrink-0 rounded-full px-1.5 py-0.5 text-[9px] font-medium', platformBadgeLightClass(sub.group?.platform || '')]">{{ platformLabel(sub.group?.platform || '') }}</span>
                      </div>
                      <div class="flex flex-wrap gap-x-3 text-[11px] text-gray-400 dark:text-gray-500">
                        <span>{{ t('payment.planCard.rate') }}: {{ activeSubscriptionRateDisplay(sub) }}</span>
                        <span v-if="sub.group?.daily_limit_usd == null && sub.group?.weekly_limit_usd == null && sub.group?.monthly_limit_usd == null">{{ t('payment.planCard.quota') }}: {{ t('payment.planCard.unlimited') }}</span>
                        <span v-if="sub.expires_at">{{ t('userSubscriptions.daysRemaining', { days: getDaysRemaining(sub.expires_at) }) }}</span>
                        <span v-else>{{ t('userSubscriptions.noExpiration') }}</span>
                      </div>
                    </div>
                    <span class="badge badge-success shrink-0 text-[10px]">{{ t('userSubscriptions.status.active') }}</span>
                  </div>
                </div>
              </div>
            </template>
          </template>
        </template>
        <div v-if="(checkout.help_text || checkout.help_image_url) && paymentPhase === 'select' && !selectedPlan" class="card p-4">
          <div class="flex flex-col items-center gap-3">
            <img v-if="checkout.help_image_url" :src="checkout.help_image_url" alt=""
              class="h-40 max-w-full cursor-pointer rounded-lg object-contain transition-opacity hover:opacity-80"
              @click="previewImage = checkout.help_image_url" />
            <p v-if="checkout.help_text" class="text-center text-sm text-gray-500 dark:text-gray-400">{{ checkout.help_text }}</p>
          </div>
        </div>
      </template>
    </div>
    <!-- Renewal Plan Selection Modal -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="showRenewalModal" class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" @click.self="closeRenewalModal">
          <div class="relative w-full max-w-lg rounded-2xl border border-gray-200 bg-white p-6 shadow-2xl dark:border-dark-700 dark:bg-dark-900">
            <!-- Close button -->
            <button class="absolute right-4 top-4 rounded-lg p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700 dark:hover:text-gray-200" @click="closeRenewalModal">
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
            </button>
            <h3 class="mb-4 text-lg font-semibold text-content-primary">{{ t('payment.selectPlan') }}</h3>
            <div class="space-y-4">
              <SubscriptionPlanCard
                v-for="plan in renewalPlans"
                :key="plan.id"
                :plan="plan"
                :active-subscriptions="activeSubscriptions"
                :coupon-pay-amount="planCardCouponPayAmount(plan)"
                @select="selectPlanFromModal"
                @multiplier-change="onPlanCardMultiplierChange"
              />
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
    <!-- Image Preview Overlay -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="previewImage" class="fixed inset-0 z-[60] flex items-center justify-center bg-black/70 backdrop-blur-sm" @click="previewImage = ''">
          <img :src="previewImage" alt="" class="max-h-[85vh] max-w-[90vw] rounded-xl object-contain shadow-2xl" />
        </div>
      </Transition>
    </Teleport>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { usePaymentStore } from '@/stores/payment'
import { useSubscriptionStore } from '@/stores/subscriptions'
import { useAppStore } from '@/stores'
import { createPaymentOrderIdempotencyKey, paymentAPI } from '@/api/payment'
import { extractApiErrorMessage, extractI18nErrorMessage } from '@/utils/apiError'
import { isMobileDevice } from '@/utils/device'
import type { SubscriptionPlan, CheckoutInfoResponse, CreateOrderResult, OrderType, CafeCouponSummary } from '@/types/payment'
import type { UserSubscription } from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import AmountInput from '@/components/payment/AmountInput.vue'
import PaymentMethodSelector from '@/components/payment/PaymentMethodSelector.vue'
import { METHOD_ORDER, getPaymentPopupFeatures } from '@/components/payment/providerConfig'
import {
  PAYMENT_RECOVERY_STORAGE_KEY,
  buildCreateOrderPayload,
  clearPaymentRecoverySnapshot,
  decidePaymentLaunch,
  getVisibleMethods,
  normalizeVisibleMethod,
  readPaymentRecoverySnapshot,
  type PaymentRecoverySnapshot,
  writePaymentRecoverySnapshot,
} from '@/components/payment/paymentFlow'
import { platformAccentBarClass, platformBadgeLightClass, platformBadgeClass, platformTextClass, platformLabel } from '@/utils/platformColors'
import { isCustomSubscriptionForPlan, subscriptionCustomMultiplier, subscriptionCustomSourcePlanId } from '@/utils/subscriptionCustom'
import SubscriptionPlanCard from '@/components/payment/SubscriptionPlanCard.vue'
import PaymentStatusPanel from '@/components/payment/PaymentStatusPanel.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatPaymentAmount, normalizePaymentCurrency } from '@/components/payment/currency'
import type { PaymentMethodOption } from '@/components/payment/PaymentMethodSelector.vue'
import { buildPaymentErrorToastMessage, describePaymentScenarioError } from './paymentUx'
import { hasWechatResumeQuery, parseWechatResumeRoute, stripWechatResumeQuery } from './paymentWechatResume'

const RECHARGE_QUICK_AMOUNTS = [20, 50, 100, 200, 500]
const MIN_ACTUAL_PAYMENT_AMOUNT = 1

const i18n = useI18n()
const { t } = i18n
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const paymentStore = usePaymentStore()
const subscriptionStore = useSubscriptionStore()
const appStore = useAppStore()

const user = computed(() => authStore.user)
const activeSubscriptions = computed(() => subscriptionStore.activeSubscriptions)

function getDaysRemaining(expiresAt: string): number {
  const diff = new Date(expiresAt).getTime() - Date.now()
  return Math.max(0, Math.ceil(diff / (1000 * 60 * 60 * 24)))
}

const loading = ref(true)
const submitting = ref(false)
const createOrderIdempotencyKey = ref('')
const errorMessage = ref('')
const errorHintMessage = ref('')
const activeTab = ref<'recharge' | 'subscription'>('subscription')
const amount = ref<number | null>(null)
const selectedMethod = ref('')
const selectedPlan = ref<SubscriptionPlan | null>(null)
const selectedSubscriptionMultiplier = ref(1)
const previewImage = ref('')
const cafeCouponCode = ref('')
const cafeCouponAppliedCode = ref('')
const cafeCouponAppliedContextKey = ref('')
const cafeCouponPreviewingCode = ref('')
const cafeCouponPreviewingContextKey = ref('')
const cafeCouponMessage = ref('')
const cafeCouponError = ref('')
const cafeCouponDiscountAmount = ref<number | null>(null)
const cafeCouponPayableAmount = ref<number | null>(null)
const cafeCouponAppliedCoupon = ref<CafeCouponSummary | null>(null)
const cafeCouponInfoCode = ref('')
const cafeCouponInfoCoupon = ref<CafeCouponSummary | null>(null)
const cafeCouponInfoLoadingCode = ref('')
const previewingCafeCoupon = ref(false)
const planCardMultipliers = ref<Record<number, number>>({})
let cafeCouponPreviewTimer: ReturnType<typeof setTimeout> | null = null
let cafeCouponPreviewPromise: Promise<void> | null = null
let cafeCouponPreviewSeq = 0
let cafeCouponInfoSeq = 0

const paymentPhase = ref<'select' | 'paying'>('select')

interface CreateOrderOptions {
  openid?: string
  wechatResumeToken?: string
  paymentType?: string
  isResume?: boolean
  mobileQrFallbackAttempted?: boolean
}


interface WeixinJSBridgeLike {
  invoke(
    action: string,
    payload: Record<string, unknown>,
    callback: (result: Record<string, unknown>) => void,
  ): void
}

function emptyPaymentState(): PaymentRecoverySnapshot {
  return {
    orderId: 0,
    amount: 0,
    qrCode: '',
    expiresAt: '',
    paymentType: '',
    payUrl: '',
    outTradeNo: '',
    clientSecret: '',
    intentId: '',
    currency: '',
    countryCode: '',
    paymentEnv: '',
    payAmount: 0,
    orderType: '',
    paymentMode: '',
    resumeToken: '',
    multiplier: undefined,
    createdAt: 0,
  }
}

function getWeixinJSBridge(): WeixinJSBridgeLike | undefined {
  return (window as Window & { WeixinJSBridge?: WeixinJSBridgeLike }).WeixinJSBridge
}

function waitForWeixinJSBridge(timeoutMs = 4000): Promise<WeixinJSBridgeLike | null> {
  const existing = getWeixinJSBridge()
  if (existing) return Promise.resolve(existing)

  return new Promise((resolve) => {
    let settled = false
    const finish = (bridge: WeixinJSBridgeLike | null) => {
      if (settled) return
      settled = true
      document.removeEventListener('WeixinJSBridgeReady', handleReady)
      document.removeEventListener('onWeixinJSBridgeReady', handleReady)
      window.clearTimeout(timer)
      resolve(bridge)
    }
    const handleReady = () => finish(getWeixinJSBridge() ?? null)
    const timer = window.setTimeout(() => finish(getWeixinJSBridge() ?? null), timeoutMs)
    document.addEventListener('WeixinJSBridgeReady', handleReady, false)
    document.addEventListener('onWeixinJSBridgeReady', handleReady, false)
  })
}

async function invokeWechatJsapiPayment(payload: Record<string, unknown>): Promise<Record<string, unknown>> {
  const bridge = await waitForWeixinJSBridge()
  if (!bridge) {
    throw new Error('WECHAT_JSAPI_UNAVAILABLE')
  }
  return new Promise((resolve) => {
    bridge.invoke('getBrandWCPayRequest', payload, (result) => resolve(result || {}))
  })
}

const paymentState = ref<PaymentRecoverySnapshot>(emptyPaymentState())

function nextCreateOrderIdempotencyKey(): string {
  if (!createOrderIdempotencyKey.value) {
    createOrderIdempotencyKey.value = createPaymentOrderIdempotencyKey()
  }
  return createOrderIdempotencyKey.value
}

function resetCreateOrderIdempotencyKey() {
  createOrderIdempotencyKey.value = ''
}

function persistRecoverySnapshot(snapshot: PaymentRecoverySnapshot) {
  if (typeof window === 'undefined' || !snapshot.orderId) return
  writePaymentRecoverySnapshot(window.localStorage, snapshot, PAYMENT_RECOVERY_STORAGE_KEY)
}

function removeRecoverySnapshot() {
  if (typeof window === 'undefined') return
  clearPaymentRecoverySnapshot(window.localStorage, PAYMENT_RECOVERY_STORAGE_KEY)
}

function resetPayment() {
  paymentPhase.value = 'select'
  paymentState.value = emptyPaymentState()
  resetCreateOrderIdempotencyKey()
  removeRecoverySnapshot()
}

async function redirectToPaymentResult(state: PaymentRecoverySnapshot): Promise<void> {
  const query: Record<string, string | undefined> = {}
  if (state.orderId > 0) {
    query.order_id = String(state.orderId)
  }
  if (state.outTradeNo) {
    query.out_trade_no = state.outTradeNo
  }
  if (state.resumeToken) {
    query.resume_token = state.resumeToken
  }
  await router.push({
    path: '/payment/result',
    query,
  })
}

function buildWechatOAuthAuthorizeUrl(authorizeUrl: string): string {
  return authorizeUrl.trim()
}


function onPaymentDone() {
  const wasSubscription = paymentState.value.orderType === 'subscription'
  resetPayment()
  selectedPlan.value = null
  if (wasSubscription) {
    subscriptionStore.fetchActiveSubscriptions(true).catch(() => {})
  }
}

function onPaymentSuccess() {
  removeRecoverySnapshot()
  authStore.refreshUser()
  if (paymentState.value.orderType === 'subscription') {
    subscriptionStore.fetchActiveSubscriptions(true).catch(() => {})
  }
}

function onPaymentSettled() {
  removeRecoverySnapshot()
}

// All checkout data from single API call
const checkout = ref<CheckoutInfoResponse>({
  methods: {}, global_min: 0, global_max: 0,
  plans: [], balance_disabled: false, balance_recharge_multiplier: 1, recharge_fee_rate: 0, help_text: '', help_image_url: '', stripe_publishable_key: '',
})

const tabs = computed(() => {
  const result: { key: 'recharge' | 'subscription'; label: string }[] = []
  result.push({ key: 'subscription', label: t('payment.tabSubscribe') })
  if (!checkout.value.balance_disabled) result.push({ key: 'recharge', label: t('payment.tabTopUp') })
  return result
})

const defaultRechargeAmount = RECHARGE_QUICK_AMOUNTS[0] ?? 0
const visibleMethods = computed(() => getVisibleMethods(checkout.value.methods))
const enabledMethods = computed(() => Object.keys(visibleMethods.value))
const validAmount = computed(() => amount.value ?? 0)
const balanceRechargeMultiplier = computed(() => {
  const multiplier = checkout.value.balance_recharge_multiplier
  return multiplier > 0 ? multiplier : 1
})
const creditedAmount = computed(() => Math.round((validAmount.value * balanceRechargeMultiplier.value) * 100) / 100)

function roundPaymentAmount(value: number): number {
  return Math.round(value * 100) / 100
}

function multiplyPlanLimit(value: number | null | undefined, multiplier: number): number | null {
  if (value == null) return null
  return roundPaymentAmount(value * multiplier)
}

function isSubscriptionCurrentlyActive(subscription: UserSubscription): boolean {
  if (subscription.status !== 'active') return false
  if (!subscription.expires_at) return true
  const expiresAt = Date.parse(subscription.expires_at)
  return Number.isFinite(expiresAt) && expiresAt > Date.now()
}

function activeCustomSubscriptionForPlan(plan: SubscriptionPlan | null | undefined) {
  if (!plan) return null
  return activeSubscriptions.value.find(sub =>
    isSubscriptionCurrentlyActive(sub)
      && isCustomSubscriptionForPlan(sub, plan.id),
  ) ?? null
}

function activeSubscriptionMultiplierForPlan(plan: SubscriptionPlan | null | undefined): number | null {
  return subscriptionCustomMultiplier(activeCustomSubscriptionForPlan(plan))
}

function defaultCustomMultiplierForPlan(plan: SubscriptionPlan | null | undefined): number {
  if (!plan?.custom_multiplier_enabled) return 1
  return Math.max(1, plan.custom_multiplier_min ?? 1)
}

function maxCustomMultiplierForPlan(plan: SubscriptionPlan | null | undefined): number {
  const min = defaultCustomMultiplierForPlan(plan)
  if (!plan?.custom_multiplier_enabled) return min
  return Math.max(min, Number(plan.custom_multiplier_max || min))
}

function clampCustomMultiplierForPlan(plan: SubscriptionPlan | null | undefined, multiplier: number): number {
  const fallback = activeSubscriptionMultiplierForPlan(plan) ?? defaultCustomMultiplierForPlan(plan)
  const parsed = Math.trunc(Number(multiplier || fallback || 1))
  if (!plan?.custom_multiplier_enabled) {
    return Math.max(1, Number.isFinite(parsed) ? parsed : fallback)
  }
  const min = defaultCustomMultiplierForPlan(plan)
  const max = maxCustomMultiplierForPlan(plan)
  const safe = Number.isFinite(parsed) ? parsed : min
  return Math.min(max, Math.max(min, safe))
}

function effectivePlanMultiplier(plan: SubscriptionPlan | null | undefined): number {
  if (!plan) return 1
  const renewalMultiplier = activeSubscriptionMultiplierForPlan(plan)
  if (plan.custom_multiplier_enabled || renewalMultiplier != null) {
    return clampCustomMultiplierForPlan(plan, selectedSubscriptionMultiplier.value || renewalMultiplier || defaultCustomMultiplierForPlan(plan))
  }
  return 1
}

const effectiveSelectedMultiplier = computed(() => effectivePlanMultiplier(selectedPlan.value))
const effectiveSelectedPlanPrice = computed(() => roundPaymentAmount((selectedPlan.value?.price ?? 0) * effectiveSelectedMultiplier.value))
const effectiveSelectedOriginalPrice = computed(() => {
  if (!selectedPlan.value?.original_price) return null
  return roundPaymentAmount(selectedPlan.value.original_price * effectiveSelectedMultiplier.value)
})
function selectedSubscriptionCouponPayableEstimate(): number | null {
  if (currentOrderType.value !== 'subscription') return null
  if (cafeCouponApplied.value && cafeCouponPayableAmount.value != null) {
    return roundPaymentAmount(cafeCouponPayableAmount.value)
  }
  const coupon = planCardCafeCoupon()
  if (!coupon) return null
  return localCafeCouponPayAmount(effectiveSelectedPlanPrice.value, coupon)
}

const effectiveSelectedCouponPrice = computed(() => selectedSubscriptionCouponPayableEstimate())
const effectiveSelectedDailyLimit = computed(() => multiplyPlanLimit(selectedPlan.value?.daily_limit_usd, effectiveSelectedMultiplier.value))
const effectiveSelectedWeeklyLimit = computed(() => multiplyPlanLimit(selectedPlan.value?.weekly_limit_usd, effectiveSelectedMultiplier.value))
const effectiveSelectedMonthlyLimit = computed(() => multiplyPlanLimit(selectedPlan.value?.monthly_limit_usd, effectiveSelectedMultiplier.value))
const selectedActiveCustomMultiplier = computed(() => activeSubscriptionMultiplierForPlan(selectedPlan.value))
const selectedMultiplierConflictsActiveCustom = computed(() => {
  const activeMultiplier = selectedActiveCustomMultiplier.value
  return activeMultiplier != null && effectiveSelectedMultiplier.value !== activeMultiplier
})

const selectedPlanRateDisplay = computed(() => {
  return formatBillingRateMultiplier(selectedPlan.value?.rate_multiplier)
})

function activeSubscriptionRateDisplay(subscription: UserSubscription): string {
  return formatBillingRateMultiplier(subscription.group?.rate_multiplier)
}

function formatBillingRateMultiplier(rate: number | null | undefined): string {
  const numericRate = rate ?? 1
  return `${Number(numericRate.toPrecision(10))}x`
}

// Adaptive grid: center single card, 2-col for 2 plans, 3-col for 3+
const planGridClass = computed(() => {
  const n = checkout.value.plans.length
  if (n <= 2) return 'grid grid-cols-1 gap-5 sm:grid-cols-2'
  return 'grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3'
})

// Check if an amount fits a method's [min, max]. 0 = no limit.
function amountFitsMethod(amt: number, methodType: string): boolean {
  if (amt <= 0) return true
  const ml = visibleMethods.value[methodType]
  if (!ml) return false
  if (ml.single_min > 0 && amt < ml.single_min) return false
  if (ml.single_max > 0 && amt > ml.single_max) return false
  return true
}

// Visible methods decide the amount range shown to users.
const globalMinAmount = computed(() => {
  const limits = Object.values(visibleMethods.value)
  if (limits.length === 0) return 0
  if (limits.some(limit => limit.single_min <= 0)) return 0
  return Math.min(...limits.map(limit => limit.single_min))
})
const globalMaxAmount = computed(() => {
  const limits = Object.values(visibleMethods.value)
  if (limits.length === 0) return 0
  if (limits.some(limit => limit.single_max <= 0)) return 0
  return Math.max(...limits.map(limit => limit.single_max))
})

// Selected method's limits (for validation and error messages)
const selectedLimit = computed(() => visibleMethods.value[selectedMethod.value])
const selectedCurrency = computed(() => normalizePaymentCurrency(selectedLimit.value?.currency))
const localeCode = computed(() => {
  const raw = i18n.locale as unknown
  if (typeof raw === 'string') return raw
  if (raw && typeof raw === 'object' && 'value' in raw) {
    return String((raw as { value?: string }).value || '')
  }
  return undefined
})

function formatSelectedPaymentAmount(value: number): string {
  return formatPaymentAmount(value, selectedCurrency.value, localeCode.value)
}

const minimumPayAmountMessage = computed(() =>
  t('payment.actualPayTooLow', { min: formatSelectedPaymentAmount(MIN_ACTUAL_PAYMENT_AMOUNT) })
)

function isPayAmountBelowMinimum(value: number): boolean {
  return value > 0 && value < MIN_ACTUAL_PAYMENT_AMOUNT
}

function currencyFractionDigits(currency: string): number {
  try {
    return new Intl.NumberFormat(undefined, { style: 'currency', currency }).resolvedOptions().maximumFractionDigits ?? 2
  } catch {
    return 2
  }
}

function roundUpToCurrency(value: number, currency: string): number {
  const scale = 10 ** currencyFractionDigits(currency)
  return Math.ceil(value * scale - Number.EPSILON) / scale
}

function calculateFeeAdjustedPayAmount(baseAmount: number, rate: number, currency = selectedCurrency.value): number {
  if (baseAmount <= 0 || rate <= 0) return baseAmount
  const scale = 10 ** currencyFractionDigits(currency)
  return Math.round((baseAmount + roundUpToCurrency((baseAmount * rate) / 100, currency)) * scale) / scale
}

const methodOptions = computed<PaymentMethodOption[]>(() =>
  enabledMethods.value.map((type) => {
    const ml = visibleMethods.value[type]
    const methodCurrency = normalizePaymentCurrency(ml?.currency)
    const payableAmount = cafeCouponApplied.value && cafeCouponPayableAmount.value != null
      ? calculateFeeAdjustedPayAmount(cafeCouponPayableAmount.value, feeRate.value, methodCurrency)
      : calculateFeeAdjustedPayAmount(validAmount.value, feeRate.value, methodCurrency)
    return {
      type,
      fee_rate: ml?.fee_rate ?? 0,
      available: ml?.available !== false && !isPayAmountBelowMinimum(payableAmount) && amountFitsMethod(payableAmount, type),
    }
  })
)

const feeRate = computed(() => checkout.value?.recharge_fee_rate ?? 0)
function cafeCouponCurrentContextKey(code = normalizedCafeCouponCode.value): string {
  const normalizedCode = code.trim().toUpperCase()
  if (!normalizedCode) return ''
  const orderType = currentOrderType.value
  const planId = orderType === 'subscription' ? currentPlanId.value ?? 0 : 0
  const multiplier = orderType === 'subscription' ? effectiveSelectedMultiplier.value : 0
  return [normalizedCode, orderType, planId, multiplier, roundPaymentAmount(currentOrderAmount.value)].join(':')
}
const cafeCouponApplied = computed(() => {
  const contextKey = cafeCouponCurrentContextKey()
  return contextKey !== ''
    && cafeCouponAppliedCode.value === normalizedCafeCouponCode.value
    && cafeCouponAppliedContextKey.value === contextKey
})
const rechargeCouponPayableAmount = computed(() => {
  if (!cafeCouponApplied.value || cafeCouponPayableAmount.value == null) return null
  if (currentOrderType.value !== 'balance') return null
  return roundPaymentAmount(cafeCouponPayableAmount.value)
})
const rechargeCouponDiscountAmount = computed(() => {
  if (rechargeCouponPayableAmount.value == null) return null
  if (cafeCouponDiscountAmount.value != null) return roundPaymentAmount(cafeCouponDiscountAmount.value)
  return roundPaymentAmount(Math.max(0, validAmount.value - rechargeCouponPayableAmount.value))
})
const rechargeFeeBaseAmount = computed(() => rechargeCouponPayableAmount.value ?? validAmount.value)
const feeAmount = computed(() =>
  feeRate.value > 0 && rechargeFeeBaseAmount.value > 0
    ? roundUpToCurrency((rechargeFeeBaseAmount.value * feeRate.value) / 100, selectedCurrency.value)
    : 0
)
const totalAmount = computed(() => calculateFeeAdjustedPayAmount(validAmount.value, feeRate.value))
const rechargeTotalAmount = computed(() => calculateFeeAdjustedPayAmount(rechargeFeeBaseAmount.value, feeRate.value))
const rechargeButtonAmount = computed(() => rechargeTotalAmount.value)

function effectiveRechargeMethodAmount(): number {
  return rechargeTotalAmount.value
}

const rechargePayAmountBelowMinimum = computed(() =>
  validAmount.value > 0 && isPayAmountBelowMinimum(effectiveRechargeMethodAmount())
)

const amountError = computed(() => {
  if (validAmount.value <= 0) return ''
  const methodAmount = effectiveRechargeMethodAmount()
  if (rechargePayAmountBelowMinimum.value) return minimumPayAmountMessage.value
  if (!enabledMethods.value.some((m) => amountFitsMethod(methodAmount, m))) {
    return t('payment.amountNoMethod')
  }
  const ml = selectedLimit.value
  if (ml) {
    if (ml.single_min > 0 && methodAmount < ml.single_min) return t('payment.amountTooLow', { min: formatSelectedPaymentAmount(ml.single_min) })
    if (ml.single_max > 0 && methodAmount > ml.single_max) return t('payment.amountTooHigh', { max: formatSelectedPaymentAmount(ml.single_max) })
  }
  return ''
})

const canSubmit = computed(() =>
  validAmount.value > 0
    && !rechargePayAmountBelowMinimum.value
    && amountFitsMethod(effectiveRechargeMethodAmount(), selectedMethod.value)
    && selectedLimit.value?.available !== false
)

// Subscription-specific: method options based on fee-adjusted payable amount.
const subMethodOptions = computed<PaymentMethodOption[]>(() => {
  const planPrice = effectiveSelectedPlanPrice.value
  return enabledMethods.value.map((type) => {
    const ml = visibleMethods.value[type]
    const methodCurrency = normalizePaymentCurrency(ml?.currency)
    const couponPayable = selectedSubscriptionCouponPayableEstimate()
    const payableAmount = couponPayable != null
      ? calculateFeeAdjustedPayAmount(couponPayable, feeRate.value, methodCurrency)
      : calculateFeeAdjustedPayAmount(planPrice, feeRate.value, methodCurrency)
    return {
      type,
      fee_rate: ml?.fee_rate ?? 0,
      available: ml?.available !== false && !isPayAmountBelowMinimum(payableAmount) && amountFitsMethod(payableAmount, type),
    }
  })
})

const subscriptionCouponPayableAmount = computed(() => selectedSubscriptionCouponPayableEstimate())
const subscriptionCouponDiscountAmount = computed(() => {
  if (subscriptionCouponPayableAmount.value == null) return null
  if (cafeCouponDiscountAmount.value != null) return roundPaymentAmount(cafeCouponDiscountAmount.value)
  return roundPaymentAmount(Math.max(0, effectiveSelectedPlanPrice.value - subscriptionCouponPayableAmount.value))
})
const subscriptionFeeBaseAmount = computed(() => subscriptionCouponPayableAmount.value ?? effectiveSelectedPlanPrice.value)
const subFeeAmount = computed(() => {
  const price = subscriptionFeeBaseAmount.value
  if (feeRate.value <= 0 || price <= 0) return 0
  return roundUpToCurrency((price * feeRate.value) / 100, selectedCurrency.value)
})

const subTotalAmount = computed(() => calculateFeeAdjustedPayAmount(subscriptionFeeBaseAmount.value, feeRate.value))
const subscriptionButtonAmount = computed(() => subTotalAmount.value)
const cafeCouponDisplayPayableAmount = computed(() => {
  if (!cafeCouponApplied.value || cafeCouponPayableAmount.value == null) return null
  return currentOrderType.value === 'subscription' ? subscriptionButtonAmount.value : rechargeButtonAmount.value
})
const cafeCouponAppliedText = computed(() => {
  const coupon = cafeCouponAppliedCoupon.value
  if (coupon?.type === 'discount') {
    return t('payment.cafeCoupon.appliedDiscount', {
      value: Number(coupon.value || 0).toLocaleString(undefined, { maximumFractionDigits: 2 }),
    })
  }
  if (cafeCouponDiscountAmount.value != null) {
    return t('payment.cafeCoupon.appliedCash', {
      amount: formatSelectedPaymentAmount(cafeCouponDiscountAmount.value),
    })
  }
  return t('payment.cafeCoupon.applied')
})

function effectiveSubscriptionMethodAmount(): number {
  return subTotalAmount.value
}

const subscriptionPayAmountBelowMinimum = computed(() =>
  selectedPlan.value !== null && isPayAmountBelowMinimum(effectiveSubscriptionMethodAmount())
)

const canSubmitSubscription = computed(() =>
  selectedPlan.value !== null
    && !selectedMultiplierConflictsActiveCustom.value
    && !subscriptionPayAmountBelowMinimum.value
    && amountFitsMethod(effectiveSubscriptionMethodAmount(), selectedMethod.value)
    && selectedLimit.value?.available !== false
)

const normalizedCafeCouponCode = computed(() => cafeCouponCode.value.trim().toUpperCase())
const currentOrderType = computed<OrderType>(() => activeTab.value === 'subscription' && selectedPlan.value ? 'subscription' : 'balance')
const currentOrderAmount = computed(() => currentOrderType.value === 'subscription' ? effectiveSelectedPlanPrice.value : validAmount.value)
const currentPlanId = computed(() => currentOrderType.value === 'subscription' ? selectedPlan.value?.id : undefined)
const currentOrderPayAmountBelowMinimum = computed(() =>
  currentOrderType.value === 'subscription' ? subscriptionPayAmountBelowMinimum.value : rechargePayAmountBelowMinimum.value
)

function shouldPreviewCafeCouponForCurrentContext(): boolean {
  if (activeTab.value === 'subscription') {
    return selectedPlan.value != null && effectiveSelectedPlanPrice.value > 0
  }
  return validAmount.value > 0
}

watch(normalizedCafeCouponCode, () => {
  resetCreateOrderIdempotencyKey()
  resetCafeCouponInfoState()
  loadCafeCouponInfoForPlanCards().catch(() => {})
})

watch([activeTab, () => selectedPlan.value?.id, () => checkout.value.plans.length, () => activeSubscriptions.value.length], () => {
  loadCafeCouponInfoForPlanCards().catch(() => {})
})

function resetCafeCouponState(keepError = false) {
  cafeCouponAppliedCode.value = ''
  cafeCouponAppliedContextKey.value = ''
  cafeCouponPreviewingCode.value = ''
  cafeCouponPreviewingContextKey.value = ''
  cafeCouponMessage.value = ''
  cafeCouponDiscountAmount.value = null
  cafeCouponPayableAmount.value = null
  cafeCouponAppliedCoupon.value = null
  if (!keepError) {
    cafeCouponError.value = ''
  }
}

function resetCafeCouponInfoState(): void {
  cafeCouponInfoCode.value = ''
  cafeCouponInfoCoupon.value = null
  cafeCouponInfoLoadingCode.value = ''
}

function rememberCafeCouponInfo(coupon: CafeCouponSummary | null | undefined): void {
  const code = (coupon?.code || '').trim().toUpperCase()
  if (!code) return
  cafeCouponInfoCode.value = code
  cafeCouponInfoCoupon.value = coupon ?? null
}

function planCardMultiplierForPlan(plan: SubscriptionPlan): number {
  const renewalMultiplier = activeSubscriptionMultiplierForPlan(plan)
  if (renewalMultiplier != null) return renewalMultiplier
  const tracked = Number(planCardMultipliers.value[plan.id])
  if (Number.isFinite(tracked) && tracked >= 1) return tracked
  return defaultCustomMultiplierForPlan(plan)
}

function planCardCafeCoupon(): CafeCouponSummary | null {
  const code = normalizedCafeCouponCode.value
  if (!code) return null
  const appliedCode = (cafeCouponAppliedCoupon.value?.code || '').trim().toUpperCase()
  if (appliedCode === code) return cafeCouponAppliedCoupon.value
  if (cafeCouponInfoCode.value === code) return cafeCouponInfoCoupon.value
  return null
}

function localCafeCouponPayAmount(amount: number, coupon: CafeCouponSummary | null): number | null {
  if (!coupon || amount <= 0) return null
  const value = Number(coupon.value)
  if (!Number.isFinite(value) || value <= 0) return null
  let discount = 0
  if (coupon.type === 'discount') {
    discount = amount * Math.min(value, 100) / 100
  } else {
    discount = value
  }
  discount = roundPaymentAmount(Math.max(0, discount))
  const maxDiscount = Math.max(0, amount - 0.01)
  const payable = roundPaymentAmount(amount - Math.min(discount, maxDiscount))
  return payable < amount ? payable : null
}

function planCardCouponPayAmount(plan: SubscriptionPlan): number | null {
  const code = normalizedCafeCouponCode.value
  if (!code) return null
  const coupon = planCardCafeCoupon()
  if (!coupon) return null
  const multiplier = planCardMultiplierForPlan(plan)
  const amount = roundPaymentAmount(plan.price * multiplier)
  return localCafeCouponPayAmount(amount, coupon)
}

async function loadCafeCouponInfoForPlanCards(): Promise<void> {
  const code = normalizedCafeCouponCode.value
  if (!code || activeTab.value !== 'subscription' || selectedPlan.value) return
  if (cafeCouponInfoCode.value === code && cafeCouponInfoCoupon.value) return
  if (cafeCouponInfoLoadingCode.value === code) return

  const seq = ++cafeCouponInfoSeq
  cafeCouponInfoLoadingCode.value = code
  try {
    const response = await paymentStore.getCafeCouponInfo({ code })
    if (seq !== cafeCouponInfoSeq || normalizedCafeCouponCode.value !== code) return
    if (response.valid === false || !response.coupon) {
      if (cafeCouponInfoCode.value === code) resetCafeCouponInfoState()
      return
    }
    rememberCafeCouponInfo(response.coupon)
  } catch {
    if (seq === cafeCouponInfoSeq && normalizedCafeCouponCode.value === code && cafeCouponInfoCode.value === code) {
      resetCafeCouponInfoState()
    }
  } finally {
    if (seq === cafeCouponInfoSeq && cafeCouponInfoLoadingCode.value === code) {
      cafeCouponInfoLoadingCode.value = ''
    }
  }
}

function onPlanCardMultiplierChange(plan: SubscriptionPlan, multiplier: number): void {
  const safeMultiplier = Number(multiplier)
  if (!Number.isFinite(safeMultiplier) || safeMultiplier < 1) return
  planCardMultipliers.value = { ...planCardMultipliers.value, [plan.id]: safeMultiplier }
}

async function previewCafeCoupon() {
  const code = normalizedCafeCouponCode.value
  if (!code) {
    resetCafeCouponState()
    return
  }
  if (currentOrderAmount.value <= 0) return

  const requestOrderType = currentOrderType.value
  const requestAmount = roundPaymentAmount(currentOrderAmount.value)
  const requestPlanId = currentPlanId.value
  const requestMultiplier = requestOrderType === 'subscription' ? effectiveSelectedMultiplier.value : undefined
  const requestContextKey = cafeCouponCurrentContextKey(code)
  if (requestContextKey && requestContextKey === cafeCouponAppliedContextKey.value) return
  if (requestContextKey && requestContextKey === cafeCouponPreviewingContextKey.value && cafeCouponPreviewPromise) {
    await cafeCouponPreviewPromise
    return
  }

  if (cafeCouponPreviewTimer) {
    clearTimeout(cafeCouponPreviewTimer)
    cafeCouponPreviewTimer = null
  }
  const seq = ++cafeCouponPreviewSeq
  previewingCafeCoupon.value = true
  cafeCouponPreviewingCode.value = code
  cafeCouponPreviewingContextKey.value = requestContextKey
  cafeCouponError.value = ''
  cafeCouponPreviewPromise = (async () => {
    try {
      const response = await paymentStore.previewCafeCoupon({
        code,
        amount: requestAmount,
        order_type: requestOrderType,
        plan_id: requestPlanId,
        multiplier: requestMultiplier,
      })
      if (seq !== cafeCouponPreviewSeq || cafeCouponCurrentContextKey(code) !== requestContextKey) return
      if (response.valid === false) {
        cafeCouponAppliedCode.value = ''
        cafeCouponAppliedContextKey.value = ''
        cafeCouponMessage.value = ''
        cafeCouponDiscountAmount.value = null
        cafeCouponPayableAmount.value = null
        cafeCouponAppliedCoupon.value = null
        if (cafeCouponInfoCode.value === code) resetCafeCouponInfoState()
        cafeCouponError.value = response.message || t('payment.cafeCoupon.invalid')
        return
      }
      cafeCouponAppliedCode.value = (response.coupon?.code || code).trim().toUpperCase()
      cafeCouponAppliedContextKey.value = requestContextKey
      cafeCouponMessage.value = response.message || ''
      cafeCouponDiscountAmount.value = Number.isFinite(response.discount_amount) ? response.discount_amount : null
      cafeCouponPayableAmount.value = Number.isFinite(response.pay_amount) ? response.pay_amount : null
      cafeCouponAppliedCoupon.value = response.coupon ?? null
      rememberCafeCouponInfo(response.coupon)
    } catch (error: unknown) {
      if (seq !== cafeCouponPreviewSeq || cafeCouponCurrentContextKey(code) !== requestContextKey) return
      console.error('Failed to preview Cafe coupon:', error)
      resetCafeCouponState(true)
      if (cafeCouponInfoCode.value === code) resetCafeCouponInfoState()
      cafeCouponError.value = extractI18nErrorMessage(error, t, 'payment.cafeCoupon.errors', extractApiErrorMessage(error, t('payment.cafeCoupon.invalid')))
    } finally {
      if (seq === cafeCouponPreviewSeq) {
        previewingCafeCoupon.value = false
        cafeCouponPreviewingCode.value = ''
        cafeCouponPreviewingContextKey.value = ''
        cafeCouponPreviewPromise = null
      }
    }
  })()
  await cafeCouponPreviewPromise
}

function scheduleCafeCouponAutoPreview() {
  resetCafeCouponState()
  const code = normalizedCafeCouponCode.value
  if (!code) return
  if (cafeCouponPreviewTimer) clearTimeout(cafeCouponPreviewTimer)
  cafeCouponPreviewTimer = setTimeout(() => {
    previewCafeCoupon().catch(() => {})
  }, code.length >= 8 ? 450 : 900)
}

watch([currentOrderAmount, currentOrderType, currentPlanId, selectedMethod, effectiveSelectedMultiplier], () => {
  resetCreateOrderIdempotencyKey()
  if (!normalizedCafeCouponCode.value) return
  resetCafeCouponState()
  if (shouldPreviewCafeCouponForCurrentContext()) {
    scheduleCafeCouponAutoPreview()
  } else {
    loadCafeCouponInfoForPlanCards().catch(() => {})
  }
})

watch(() => route.query.cafe_coupon_code, () => {
  applyRouteCafeCouponCode()
})

onBeforeUnmount(() => {
  if (cafeCouponPreviewTimer) {
    clearTimeout(cafeCouponPreviewTimer)
  }
})

// Auto-switch to first available method when current selection can't handle the payable amount
watch(() => [validAmount.value, selectedMethod.value, selectedPlan.value?.id, effectiveSelectedMultiplier.value, cafeCouponApplied.value, cafeCouponPayableAmount.value] as const, () => {
  const methodAmount = currentOrderType.value === 'subscription'
    ? effectiveSubscriptionMethodAmount()
    : effectiveRechargeMethodAmount()
  if (methodAmount <= 0 || amountFitsMethod(methodAmount, selectedMethod.value)) return
  const available = enabledMethods.value.find((m) => amountFitsMethod(methodAmount, m))
  if (available) selectedMethod.value = available
})

// Payment button class: follows selected payment method color
const paymentButtonClass = computed(() => {
  const m = selectedMethod.value
  if (!m) return 'btn-primary'
  if (m.includes('alipay')) return 'btn-alipay'
  if (m.includes('wxpay')) return 'btn-wxpay'
  if (m === 'stripe') return 'btn-stripe'
  if (m === 'airwallex') return 'btn-airwallex'
  return 'btn-primary'
})

// Subscription confirm: platform accent colors (clean card, no gradient)
const planBadgeClass = computed(() => platformBadgeClass(selectedPlan.value?.group_platform || ''))
const planTextClass = computed(() => platformTextClass(selectedPlan.value?.group_platform || ''))

// Renewal modal state
const showRenewalModal = ref(false)
const renewGroupId = ref<number | null>(null)
const renewalPlans = computed(() => {
  if (renewGroupId.value == null) return []
  return checkout.value.plans.filter(p => p.group_id === renewGroupId.value)
})

const planValiditySuffix = computed(() => {
  if (!selectedPlan.value) return ''
  const u = selectedPlan.value.validity_unit || 'day'
  if (u === 'month') return t('payment.perMonth')
  if (u === 'year') return t('payment.perYear')
  return `${selectedPlan.value.validity_days}${t('payment.days')}`
})


function firstRouteQueryString(value: unknown): string {
  if (Array.isArray(value)) return typeof value[0] === 'string' ? value[0] : ''
  return typeof value === 'string' ? value : ''
}

function positiveRouteNumber(value: unknown): number | null {
  const parsed = Number(firstRouteQueryString(value))
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null
}

function routeSubscriptionPlanForGroup(groupId: number): SubscriptionPlan | null {
  const customSubscription = activeSubscriptions.value.find(sub =>
    sub.group_id === groupId
      && isSubscriptionCurrentlyActive(sub)
      && subscriptionCustomSourcePlanId(sub) != null,
  )
  const customSourcePlanId = subscriptionCustomSourcePlanId(customSubscription)
  if (customSourcePlanId) {
    return checkout.value.plans.find(plan => plan.id === customSourcePlanId) ?? null
  }
  const groupPlans = checkout.value.plans.filter(plan => plan.group_id === groupId)
  if (groupPlans.length === 1) return groupPlans[0]
  if (groupPlans.length > 1) {
    renewGroupId.value = groupId
    showRenewalModal.value = true
  }
  return null
}

function initialMultiplierForPlan(plan: SubscriptionPlan | null | undefined, multiplier = 1, preferActive = true): number {
  const renewalMultiplier = activeSubscriptionMultiplierForPlan(plan)
  if (preferActive && renewalMultiplier != null) return renewalMultiplier
  if (plan?.custom_multiplier_enabled || renewalMultiplier != null) {
    return clampCustomMultiplierForPlan(plan, multiplier || renewalMultiplier || 1)
  }
  return 1
}

function selectPlan(plan: SubscriptionPlan, multiplier = 1) {
  selectedPlan.value = plan
  selectedSubscriptionMultiplier.value = initialMultiplierForPlan(plan, multiplier, false)
  errorMessage.value = ''
}

function selectPlanFromModal(plan: SubscriptionPlan, multiplier = 1) {
  showRenewalModal.value = false
  renewGroupId.value = null
  selectedPlan.value = plan
  selectedSubscriptionMultiplier.value = initialMultiplierForPlan(plan, multiplier, false)
  errorMessage.value = ''
}

function closeRenewalModal() {
  showRenewalModal.value = false
  renewGroupId.value = null
}

async function ensureCafeCouponReady(): Promise<boolean> {
  if (!normalizedCafeCouponCode.value) return true
  if (cafeCouponError.value && !cafeCouponApplied.value) return false
  await previewCafeCoupon()
  return cafeCouponApplied.value && !cafeCouponError.value
}

function cafeCouponCodeForOrder(isResume = false): string | undefined {
  if (cafeCouponApplied.value) return normalizedCafeCouponCode.value
  return isResume && normalizedCafeCouponCode.value ? normalizedCafeCouponCode.value : undefined
}

async function handleSubmitRecharge() {
  if (!canSubmit.value || submitting.value) return
  if (!await ensureCafeCouponReady()) return
  await createOrder(validAmount.value, 'balance')
}

async function confirmSubscribe() {
  if (!canSubmitSubscription.value || submitting.value) return
  const plan = selectedPlan.value
  if (!plan) return
  if (!await ensureCafeCouponReady()) return
  await createOrder(effectiveSelectedPlanPrice.value, 'subscription', plan.id)
}

async function createOrder(orderAmount: number, orderType: OrderType, planId?: number, options: CreateOrderOptions = {}) {
  submitting.value = true
  errorMessage.value = ''
  errorHintMessage.value = ''
  const requestType = normalizeVisibleMethod(options.paymentType || selectedMethod.value) || options.paymentType || selectedMethod.value
  try {
    const payload = buildCreateOrderPayload({
      amount: orderAmount,
      paymentType: requestType,
      orderType,
      planId,
      origin: typeof window !== 'undefined' ? window.location.origin : '',
      isMobile: isMobileDevice(),
      isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
      forceQRCode: !!(checkout.value.alipay_force_qrcode && normalizeVisibleMethod(requestType) === 'alipay'),
      cafeCouponCode: cafeCouponCodeForOrder(options.isResume === true),
      multiplier: orderType === 'subscription' ? effectiveSelectedMultiplier.value : undefined,
    })
    if (options.openid) {
      payload.openid = options.openid
    }
    if (options.wechatResumeToken) {
      payload.wechat_resume_token = options.wechatResumeToken
    }

    const idempotencyKey = nextCreateOrderIdempotencyKey()
    const result = await paymentStore.createOrder(payload, idempotencyKey) as CreateOrderResult & { resume_token?: string }
    const openWindow = (url: string) => {
      const win = window.open(url, 'paymentPopup', getPaymentPopupFeatures())
      if (!win || win.closed) {
        window.location.href = url
      }
    }
    const visibleMethod = normalizeVisibleMethod(requestType) || requestType
    // When user clicks the dedicated Stripe button, leave method blank so the
    // landing page renders Stripe's full Payment Element (card/link/alipay/wxpay).
    const stripeMethod = visibleMethod === 'stripe'
      ? ''
      : visibleMethod === 'wxpay' ? 'wechat_pay' : 'alipay'
    const stripeRouteUrl = result.client_secret && visibleMethod !== 'airwallex'
      ? router.resolve({
        path: '/payment/stripe',
        query: {
          order_id: String(result.order_id),
          client_secret: result.client_secret,
          method: stripeMethod || undefined,
          resume_token: result.resume_token || undefined,
        },
      }).href
      : ''
    const airwallexRouteUrl = result.client_secret && result.intent_id
      ? router.resolve({
        path: '/payment/airwallex',
        query: {
          order_id: String(result.order_id),
          out_trade_no: result.out_trade_no || undefined,
          resume_token: result.resume_token || undefined,
        },
      }).href
      : ''
    const decision = decidePaymentLaunch(result, {
      visibleMethod,
      orderType,
      isMobile: isMobileDevice(),
      isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
      forceQRCode: !!(checkout.value.alipay_force_qrcode && visibleMethod === 'alipay'),
      multiplier: orderType === 'subscription' ? effectiveSelectedMultiplier.value : undefined,
      stripePopupUrl: stripeRouteUrl,
      stripeRouteUrl,
      airwallexRouteUrl,
    })

    if (decision.kind === 'wechat_oauth' && decision.oauth?.authorize_url) {
      window.location.href = buildWechatOAuthAuthorizeUrl(decision.oauth.authorize_url)
      return
    }

    if (decision.kind === 'unhandled') {
      applyScenarioError({ reason: 'UNHANDLED_PAYMENT_SCENARIO' }, visibleMethod)
      return
    }

    if (decision.kind === 'completed') {
      await redirectToPaymentResult(decision.paymentState)
      return
    }

    paymentState.value = decision.paymentState
    paymentPhase.value = 'paying'
    persistRecoverySnapshot(decision.recovery)

    if (decision.kind === 'stripe_popup') {
      openWindow(decision.paymentState.payUrl)
      return
    }
    if (decision.kind === 'stripe_route') {
      window.location.href = decision.paymentState.payUrl
      return
    }
    if (decision.kind === 'airwallex_route') {
      window.location.href = decision.paymentState.payUrl
      return
    }
    if (decision.kind === 'wechat_jsapi' && decision.jsapi) {
      try {
        const jsapiResult = await invokeWechatJsapiPayment(decision.jsapi as Record<string, unknown>)
        const errMsg = String(jsapiResult.err_msg || '').toLowerCase()
        if (errMsg.includes('cancel')) {
          appStore.showInfo(t('payment.qr.cancelled'))
          resetPayment()
        } else if (errMsg && !errMsg.includes('ok')) {
          resetPayment()
          const fallbackApplied = await attemptMobileQrFallback(
            { reason: 'WECHAT_JSAPI_FAILED', message: errMsg },
            {
              orderAmount,
              orderType,
              planId,
              paymentType: visibleMethod,
              attempted: options.mobileQrFallbackAttempted === true,
            },
          )
          if (!fallbackApplied) {
            applyScenarioError({ reason: 'WECHAT_JSAPI_FAILED', message: errMsg }, visibleMethod)
          }
        } else {
          const resultState = { ...decision.paymentState }
          resetPayment()
          await redirectToPaymentResult(resultState)
        }
      } catch (err: unknown) {
        resetPayment()
        const fallbackApplied = await attemptMobileQrFallback(err, {
          orderAmount,
          orderType,
          planId,
          paymentType: visibleMethod,
          attempted: options.mobileQrFallbackAttempted === true,
        })
        if (!fallbackApplied) {
          throw err
        }
      }
      return
    }
    if (decision.kind === 'redirect_waiting' && decision.paymentState.payUrl) {
      if (isMobileDevice()) {
        window.location.href = decision.paymentState.payUrl
        return
      }
      openWindow(decision.paymentState.payUrl)
    }
  } catch (err: unknown) {
    const apiErr = err as Record<string, unknown>
    if (apiErr.reason === 'TOO_MANY_PENDING') {
      const metadata = apiErr.metadata as Record<string, unknown> | undefined
      errorMessage.value = t('payment.errors.tooManyPending', { max: metadata?.max || '' })
      errorHintMessage.value = ''
    } else if (apiErr.reason === 'CANCEL_RATE_LIMITED') {
      errorMessage.value = t('payment.errors.cancelRateLimited')
      errorHintMessage.value = ''
    } else if (await attemptMobileQrFallback(err, {
      orderAmount,
      orderType,
      planId,
      paymentType: requestType,
      attempted: options.mobileQrFallbackAttempted === true,
    })) {
      return
    } else {
      const handled = applyScenarioError(
        err,
        normalizeVisibleMethod(options.paymentType || selectedMethod.value) || selectedMethod.value,
      )
      if (!handled) {
        errorMessage.value = extractI18nErrorMessage(err, t, 'payment.errors', extractApiErrorMessage(err, t('payment.result.failed')))
        errorHintMessage.value = ''
      }
      if (handled) {
        return
      }
    }
    appStore.showError(buildPaymentErrorToastMessage(errorMessage.value, errorHintMessage.value))
  } finally {
    submitting.value = false
  }
}

interface MobileQrFallbackContext {
  orderAmount: number
  orderType: OrderType
  planId?: number
  paymentType: string
  attempted: boolean
}

function shouldFallbackToDesktopQr(err: unknown, paymentMethod: string, attempted: boolean): boolean {
  if (attempted || !isMobileDevice()) {
    return false
  }

  const normalizedMethod = normalizeVisibleMethod(paymentMethod) || paymentMethod
  const reason = typeof err === 'object' && err && 'reason' in err && typeof err.reason === 'string'
    ? err.reason
    : ''
  const message = err instanceof Error
    ? err.message
    : (typeof err === 'object' && err && 'message' in err && typeof err.message === 'string'
      ? err.message
      : '')
  const normalizedMessage = message.toLowerCase()

  if (normalizedMethod === 'wxpay') {
    return reason === 'WECHAT_H5_NOT_AUTHORIZED'
      || reason === 'WECHAT_PAYMENT_MP_NOT_CONFIGURED'
      || reason === 'WECHAT_JSAPI_FAILED'
      || reason === 'PAYMENT_GATEWAY_ERROR'
      || reason === 'UNHANDLED_PAYMENT_SCENARIO'
      || normalizedMessage.includes('weixinjsbridge is unavailable')
      || normalizedMessage.includes('wechat_jsapi_unavailable')
  }

  if (normalizedMethod === 'alipay') {
    return reason === 'PAYMENT_GATEWAY_ERROR' || reason === 'UNHANDLED_PAYMENT_SCENARIO'
  }

  return false
}

async function attemptMobileQrFallback(err: unknown, context: MobileQrFallbackContext): Promise<boolean> {
  if (!shouldFallbackToDesktopQr(err, context.paymentType, context.attempted)) {
    return false
  }

  try {
    const visibleMethod = normalizeVisibleMethod(context.paymentType) || context.paymentType
    const payload = buildCreateOrderPayload({
      amount: context.orderAmount,
      paymentType: visibleMethod,
      orderType: context.orderType,
      planId: context.planId,
      origin: typeof window !== 'undefined' ? window.location.origin : '',
      isMobile: false,
      isWechatBrowser: false,
      cafeCouponCode: cafeCouponCodeForOrder(false),
      multiplier: context.orderType === 'subscription' ? effectiveSelectedMultiplier.value : undefined,
    })
    const result = await paymentStore.createOrder(payload) as CreateOrderResult & { resume_token?: string }
    const stripeMethod = visibleMethod === 'wxpay' ? 'wechat_pay' : 'alipay'
    const stripeRouteUrl = result.client_secret
      ? router.resolve({
        path: '/payment/stripe',
        query: {
          order_id: String(result.order_id),
          client_secret: result.client_secret,
          method: stripeMethod,
          resume_token: result.resume_token || undefined,
        },
      }).href
      : ''
    const decision = decidePaymentLaunch(result, {
      visibleMethod,
      orderType: context.orderType,
      isMobile: false,
      isWechatBrowser: false,
      stripePopupUrl: stripeRouteUrl,
      stripeRouteUrl,
      multiplier: context.orderType === 'subscription' ? effectiveSelectedMultiplier.value : undefined,
    })

    if (decision.kind !== 'qr_waiting' || !decision.paymentState.qrCode) {
      return false
    }

    errorMessage.value = ''
    errorHintMessage.value = ''
    paymentState.value = decision.paymentState
    paymentPhase.value = 'paying'
    persistRecoverySnapshot(decision.recovery)
    appStore.showWarning(t('payment.errors.mobilePaymentFallbackToQr'))
    return true
  } catch {
    return false
  }
}

function applyScenarioError(err: unknown, paymentMethod: string): boolean {
  const descriptor = describePaymentScenarioError(err, {
    paymentMethod,
    isMobile: isMobileDevice(),
    isWechatBrowser: typeof window !== 'undefined' && /MicroMessenger/i.test(window.navigator.userAgent),
  })
  if (!descriptor) {
    errorMessage.value = ''
    errorHintMessage.value = ''
    return false
  }
  errorMessage.value = t(descriptor.messageKey)
  errorHintMessage.value = descriptor.hintKey ? t(descriptor.hintKey) : ''
  appStore.showError(buildPaymentErrorToastMessage(errorMessage.value, errorHintMessage.value))
  return true
}

function routeCafeCouponCode(): string {
  if (hasWechatResumeQuery(route.query)) return ''
  const value = route.query.cafe_coupon_code
  return (Array.isArray(value) ? value[0] : value)?.toString().trim().toUpperCase() || ''
}

function applyRouteCafeCouponCode() {
  const code = routeCafeCouponCode()
  if (!code) return
  const codeChanged = code !== normalizedCafeCouponCode.value
  if (codeChanged) {
    cafeCouponCode.value = code
    resetCafeCouponState()
  }
  if (!cafeCouponApplied.value && !previewingCafeCoupon.value && shouldPreviewCafeCouponForCurrentContext()) {
    previewCafeCoupon().catch(() => {})
  } else if (!codeChanged) {
    loadCafeCouponInfoForPlanCards().catch(() => {})
  }
}

async function resumeWechatPaymentFromQuery() {
  const resume = parseWechatResumeRoute(route.query, checkout.value.plans, validAmount.value)
  if (!resume) {
    return
  }

  selectedMethod.value = resume.paymentType
  if (resume.orderType === 'balance' && resume.orderAmount > 0) {
    amount.value = resume.orderAmount
  }
  if (resume.orderType === 'subscription' && resume.planId) {
    selectedPlan.value = checkout.value.plans.find(plan => plan.id === resume.planId) ?? null
    selectedSubscriptionMultiplier.value = initialMultiplierForPlan(selectedPlan.value, resume.multiplier || 1, false)
  }

  if (resume.cafeCouponCode) {
    cafeCouponCode.value = resume.cafeCouponCode
  }

  await router.replace({ path: route.path, query: stripWechatResumeQuery(route.query) })

  if (resume.wechatResumeToken) {
    await createOrder(0, resume.orderType, resume.planId, {
      wechatResumeToken: resume.wechatResumeToken,
      paymentType: resume.paymentType,
      isResume: true,
    })
    return
  }

  if (resume.orderAmount > 0 && resume.openid) {
    await createOrder(resume.orderAmount, resume.orderType, resume.planId, {
      openid: resume.openid,
      paymentType: resume.paymentType,
      isResume: true,
    })
  }
}

onMounted(async () => {
  try {
    const res = await paymentAPI.getCheckoutInfo()
    checkout.value = res.data
    if (enabledMethods.value.length) {
      const order: readonly string[] = METHOD_ORDER
      const sorted = [...enabledMethods.value].sort((a, b) => {
        const ai = order.indexOf(a)
        const bi = order.indexOf(b)
        return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi)
      })
      selectedMethod.value = sorted[0]
    }
    if (typeof window !== 'undefined') {
      if (hasWechatResumeQuery(route.query)) {
        removeRecoverySnapshot()
      }
      const routeResumeToken = typeof route.query.resume_token === 'string'
        ? route.query.resume_token
        : typeof route.query.wechat_resume_token === 'string'
          ? route.query.wechat_resume_token
          : undefined
      const restored = readPaymentRecoverySnapshot(
        window.localStorage.getItem(PAYMENT_RECOVERY_STORAGE_KEY),
        { resumeToken: routeResumeToken },
      )
      if (restored) {
        paymentState.value = restored
        paymentPhase.value = 'paying'
        const restoredMethod = normalizeVisibleMethod(restored.paymentType)
        if (restoredMethod) {
          selectedMethod.value = restoredMethod
        }
      } else {
        removeRecoverySnapshot()
      }
    }
    await resumeWechatPaymentFromQuery()
    const routeTab = firstRouteQueryString(route.query.tab)
    if (routeTab === 'recharge' && !checkout.value.balance_disabled) {
      activeTab.value = 'recharge'
    } else if (routeTab === 'subscription' || checkout.value.balance_disabled) {
      activeTab.value = 'subscription'
    }
    applyRouteCafeCouponCode()
    if (currentOrderType.value === 'balance' && amount.value == null && defaultRechargeAmount > 0) {
      amount.value = defaultRechargeAmount
    }
    // Handle renewal navigation: ?tab=subscription&plan=7 or ?tab=subscription&group=123
    let activeSubscriptionsFetchedForRoute = false
    if (route.query.tab === 'subscription' && (route.query.plan || route.query.group)) {
      await subscriptionStore.fetchActiveSubscriptions().catch(() => {})
      activeSubscriptionsFetchedForRoute = true
    }
    if (route.query.tab === 'subscription') {
      activeTab.value = 'subscription'
      const routeMultiplier = positiveRouteNumber(route.query.multiplier)
      const multiplier = routeMultiplier ?? 1
      const planId = positiveRouteNumber(route.query.plan)
      const groupId = positiveRouteNumber(route.query.group)
      const plan = planId
        ? (checkout.value.plans.find(p => p.id === planId) ?? null)
        : groupId != null ? routeSubscriptionPlanForGroup(groupId) : null
      if (plan) {
        selectedPlan.value = plan
        selectedSubscriptionMultiplier.value = initialMultiplierForPlan(plan, multiplier, routeMultiplier == null)
      }
    }
    if (!activeSubscriptionsFetchedForRoute) {
      subscriptionStore.fetchActiveSubscriptions().catch(() => {})
    }
  } catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
  finally { loading.value = false }
})
</script>
