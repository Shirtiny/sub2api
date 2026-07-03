<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading" class="flex justify-center py-12">
        <div
          class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
        ></div>
      </div>

      <template v-else-if="detail">
        <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <div class="card p-5">
            <p class="flex items-center gap-1.5 text-sm text-content-tertiary">
              <Icon name="dollar" size="sm" class="text-primary-500" />
              {{ t('affiliate.stats.rebateRate') }}
            </p>
            <p class="mt-2 text-2xl font-semibold text-primary-600 dark:text-primary-600">
              {{ formattedRebateRate }}<span class="ml-0.5 text-base font-medium">%</span>
            </p>
            <p class="mt-1 text-xs text-content-tertiary">
              {{ t('affiliate.stats.rebateRateHint') }}
            </p>
            <p v-if="detail.can_invite" class="mt-1 text-xs font-medium text-primary-600 dark:text-primary-600">
              {{ t('affiliate.stats.rebateCap', { amount: formatPoints(rebatePerInviteeCap) }) }}
            </p>
          </div>
          <div class="card relative p-5">
            <p class="text-sm text-content-tertiary">{{ t('affiliate.stats.invitedUsers') }}</p>
            <p class="mt-2 text-2xl font-semibold text-content-primary">
              {{ formatCount(detail.aff_count) }}
            </p>
            <p class="absolute bottom-3 right-4 text-xs text-content-tertiary">
              {{ inviteLimitLabel }}
            </p>
          </div>
          <div class="card p-5">
            <p class="text-sm text-content-tertiary">{{ t('affiliate.stats.availablePoints') }}</p>
            <p class="mt-2 text-2xl font-semibold text-emerald-600 dark:text-emerald-400">
              <button
                type="button"
                class="underline decoration-emerald-400 underline-offset-4 transition-colors hover:text-emerald-700 dark:decoration-emerald-500 dark:hover:text-emerald-300"
                @click="openLedgerDialog"
              >
                {{ formatPoints(availableRebatePoints) }}
              </button>
            </p>
            <p class="mt-1 text-xs text-content-tertiary">
              {{ t('affiliate.stats.pointsHint') }}
            </p>
          </div>
        </div>

        <div class="card p-6">
          <h3 class="text-base font-semibold text-content-primary">{{ t('affiliate.title') }}</h3>
          <p class="mt-1 text-sm text-content-tertiary">{{ t('affiliate.description') }}</p>

          <div
            v-if="detail.can_invite"
            class="mt-5 grid gap-4 md:grid-cols-2"
          >
            <div class="space-y-2">
              <p class="text-sm font-medium text-content-secondary">{{ t('affiliate.yourCode') }}</p>
              <div class="flex items-center gap-2 rounded-xl border border-stroke-default bg-surface-secondary px-3 py-2">
                <code class="flex-1 truncate text-sm font-semibold text-content-primary">{{ detail.aff_code }}</code>
                <button class="btn btn-secondary btn-sm" @click="copyCode">
                  <Icon name="copy" size="sm" />
                  <span>{{ t('affiliate.copyCode') }}</span>
                </button>
              </div>
            </div>

            <div class="space-y-2">
              <p class="text-sm font-medium text-content-secondary">{{ t('affiliate.inviteLink') }}</p>
              <div class="flex items-center gap-2 rounded-xl border border-stroke-default bg-surface-secondary px-3 py-2">
                <code class="flex-1 truncate text-sm text-content-secondary">{{ inviteLink }}</code>
                <button class="btn btn-secondary btn-sm" @click="copyInviteLink">
                  <Icon name="copy" size="sm" />
                  <span>{{ t('affiliate.copyLink') }}</span>
                </button>
              </div>
            </div>
          </div>
          <div
            v-else
            class="mt-5 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-700 dark:border-amber-900/40 dark:bg-amber-900/20 dark:text-amber-300"
          >
            {{ t('affiliate.unavailable') }}
          </div>

          <div class="mt-5 rounded-xl border border-stroke-brand bg-primary-50 p-4 dark:bg-primary-400/20">
            <p class="text-sm font-medium text-content-primary">{{ t('affiliate.tips.title') }}</p>
            <ol class="mt-2 list-decimal space-y-1 pl-5 text-sm text-content-secondary">
              <li>{{ t('affiliate.tips.line1') }}</li>
              <li>{{ t('affiliate.tips.line2', { rate: `${formattedRebateRate}%` }) }}</li>
              <li>{{ t('affiliate.tips.line3') }}</li>
              <li>{{ t('affiliate.tips.line4') }}</li>
              <li v-if="frozenRebatePoints > 0">{{ t('affiliate.tips.line5') }}</li>
            </ol>
          </div>
        </div>

        <div class="card p-6">
          <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h3 class="text-base font-semibold text-content-primary">{{ t('affiliate.redeem.title') }}</h3>
              <p class="mt-1 text-sm text-content-tertiary">{{ t('affiliate.redeem.description') }}</p>
            </div>
            <div class="flex flex-wrap gap-2 sm:justify-end">
              <button
                class="btn btn-primary"
                :disabled="redeeming"
                @click="openRedeemDialog"
              >
                <Icon v-if="redeeming" name="refresh" size="sm" class="animate-spin" />
                <Icon v-else name="dollar" size="sm" />
                <span>{{ t('affiliate.redeem.button') }}</span>
              </button>
              <span class="inline-flex cursor-not-allowed" :title="t('affiliate.redeem.withdrawUnavailable')">
                <button
                  type="button"
                  class="btn border border-gray-200 bg-gray-100 text-gray-400 shadow-none hover:bg-gray-100 hover:text-gray-400 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-500"
                  disabled
                >
                  <Icon name="creditCard" size="sm" />
                  <span>{{ t('affiliate.redeem.withdrawButton') }}</span>
                </button>
              </span>
            </div>
          </div>
          <p v-if="availableRebatePoints <= 0" class="mt-3 text-sm text-amber-600 dark:text-amber-400">
            {{ t('affiliate.redeem.empty') }}
          </p>
        </div>

        <div class="card p-6">
          <h3 class="text-base font-semibold text-content-primary">{{ t('affiliate.invitees.title') }}</h3>
          <div v-if="detail.invitees.length === 0" class="mt-4 rounded-xl border border-dashed border-stroke-strong p-6 text-center text-sm text-content-tertiary">
            {{ t('affiliate.invitees.empty') }}
          </div>
          <div v-else class="mt-4 overflow-x-auto">
            <table class="w-full min-w-[560px] text-left text-sm">
              <thead>
                <tr class="border-b border-stroke-default text-content-tertiary">
                  <th class="px-3 py-2 font-medium">{{ t('affiliate.invitees.columns.email') }}</th>
                  <th class="px-3 py-2 font-medium">{{ t('affiliate.invitees.columns.username') }}</th>
                  <th class="px-3 py-2 font-medium text-right">{{ t('affiliate.invitees.columns.rebate') }}</th>
                  <th class="px-3 py-2 font-medium">{{ t('affiliate.invitees.columns.joinedAt') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="item in detail.invitees"
                  :key="item.user_id"
                  class="border-b border-stroke-subtle last:border-b-0"
                >
                  <td class="px-3 py-3 text-content-primary">{{ formatPrivateAccount(item.email || item.username) }}</td>
                  <td class="px-3 py-3 text-content-secondary">{{ item.username || '-' }}</td>
                  <td class="px-3 py-3 text-right font-medium text-emerald-600 dark:text-emerald-400">
                    {{ formatPoints(inviteeRebatePoints(item)) }}
                  </td>
                  <td class="px-3 py-3 text-content-secondary">{{ formatDateTime(item.created_at) || '-' }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </template>
    </div>

    <BaseDialog
      :show="ledgerDialog"
      :title="t('affiliate.ledger.title')"
      width="extra-wide"
      @close="ledgerDialog = false"
    >
      <div class="space-y-4">
        <div v-if="ledgerLoading" class="flex justify-center py-8">
          <div class="h-6 w-6 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
        </div>
        <div v-else-if="ledgerRecords.length === 0" class="rounded-xl border border-dashed border-stroke-strong p-8 text-center text-sm text-content-tertiary">
          {{ t('affiliate.ledger.empty') }}
        </div>
        <div v-else class="overflow-x-auto rounded-xl border border-stroke-default">
          <table class="w-full min-w-[720px] text-left text-sm">
            <thead class="bg-surface-secondary text-content-tertiary">
              <tr>
                <th class="px-3 py-2 font-medium">{{ t('affiliate.ledger.columns.time') }}</th>
                <th class="px-3 py-2 font-medium">{{ t('affiliate.ledger.columns.action') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('affiliate.ledger.columns.amount') }}</th>
                <th class="px-3 py-2 font-medium">{{ t('affiliate.ledger.columns.source') }}</th>
                <th class="px-3 py-2 font-medium">{{ t('affiliate.ledger.columns.group') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('affiliate.ledger.columns.availableAfter') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-stroke-subtle">
              <tr v-for="item in ledgerRecords" :key="item.ledger_id">
                <td class="px-3 py-3 text-content-secondary">{{ formatDateTime(item.created_at) || '-' }}</td>
                <td class="px-3 py-3 text-content-primary">{{ formatLedgerAction(item) }}</td>
                <td class="px-3 py-3 text-right font-semibold" :class="ledgerAmountClass(item.action)">
                  {{ formatSignedLedgerAmount(item) }}
                </td>
                <td class="px-3 py-3 text-content-secondary">{{ formatLedgerSource(item) }}</td>
                <td class="px-3 py-3 text-content-secondary">{{ formatLedgerGroup(item) }}</td>
                <td class="px-3 py-3 text-right text-content-secondary">{{ formatNullablePoints(item.available_points_after) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <Pagination
          v-if="ledgerPagination.total > 0"
          :page="ledgerPagination.page"
          :total="ledgerPagination.total"
          :page-size="ledgerPagination.page_size"
          @update:page="handleLedgerPageChange"
          @update:pageSize="handleLedgerPageSizeChange"
        />
      </div>
    </BaseDialog>

    <BaseDialog
      :show="redeemDialog"
      :title="t('affiliate.redeem.modalTitle')"
      width="normal"
      @close="closeRedeemDialog"
    >
      <div class="space-y-5">
        <div class="rounded-xl border border-emerald-200 bg-emerald-50 p-4 dark:border-emerald-900/40 dark:bg-emerald-900/20">
          <p class="text-sm text-emerald-700 dark:text-emerald-300">
            {{ t('affiliate.redeem.available', { points: formatPoints(availableRebatePoints) }) }}
          </p>
        </div>

        <div>
          <p class="mb-2 text-sm font-medium text-content-secondary">
            {{ t('affiliate.redeem.targetLabel') }}
          </p>
          <div class="grid gap-3 sm:grid-cols-2">
            <label class="flex cursor-pointer gap-3 rounded-xl border p-3 dark:border-dark-700" :class="redeemForm.target_type === 'balance' ? 'border-primary-300 bg-primary-50 dark:border-primary-700 dark:bg-primary-900/20' : 'border-gray-200'">
              <input v-model="redeemForm.target_type" type="radio" value="balance" class="mt-1" />
              <span>
                <span class="block text-sm font-medium text-content-primary">{{ t('affiliate.redeem.balanceTarget') }}</span>
                <span class="mt-1 block text-xs text-content-tertiary">{{ t('affiliate.redeem.balanceTargetHint') }}</span>
              </span>
            </label>
            <label class="flex cursor-pointer gap-3 rounded-xl border p-3 dark:border-dark-700" :class="redeemForm.target_type === 'subscription' ? 'border-primary-300 bg-primary-50 dark:border-primary-700 dark:bg-primary-900/20' : 'border-gray-200'">
              <input v-model="redeemForm.target_type" type="radio" value="subscription" class="mt-1" />
              <span>
                <span class="block text-sm font-medium text-content-primary">{{ t('affiliate.redeem.subscriptionTarget') }}</span>
                <span class="mt-1 block text-xs text-content-tertiary">{{ t('affiliate.redeem.subscriptionHint') }}</span>
              </span>
            </label>
          </div>
        </div>

        <div v-if="redeemForm.target_type === 'balance'">
          <label class="mb-2 block text-sm font-medium text-content-secondary">
            {{ t('affiliate.redeem.pointsLabel') }}
          </label>
          <div class="flex gap-2">
            <div class="relative flex-1">
              <span class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-content-tertiary">¥</span>
              <input
                v-model.number="redeemForm.points"
                type="number"
                min="0"
                step="0.1"
                class="input w-full pl-7"
                :max="maxBalanceRedeemPoints"
                :placeholder="t('affiliate.redeem.pointsPlaceholder')"
              />
            </div>
            <button type="button" class="btn btn-secondary whitespace-nowrap" @click="fillMaxBalanceRedeemPoints">
              {{ t('affiliate.redeem.maxButton') }}
            </button>
          </div>
          <p class="mt-2 text-xs text-content-tertiary">
            {{ t('affiliate.redeem.balanceEstimate', { balance: formatCurrency(estimatedBalanceCredit) }) }}
          </p>
        </div>

        <div v-if="redeemForm.target_type === 'subscription'" class="space-y-3">
          <div v-if="redemptionContextLoading" class="flex items-center gap-2 text-sm text-content-tertiary">
            <Icon name="refresh" size="sm" class="animate-spin" />
            {{ t('common.loading') }}
          </div>
          <template v-else>
            <div v-if="subscriptionRedeemOptions.length === 0" class="rounded-xl border border-amber-200 bg-amber-50 p-3 text-sm text-amber-700 dark:border-amber-900/40 dark:bg-amber-900/20 dark:text-amber-300">
              {{ t('affiliate.redeem.noSubscriptions') }}
            </div>
            <div v-else>
              <label class="mb-2 block text-sm font-medium text-content-secondary">
                {{ t('affiliate.redeem.subscriptionLabel') }}
              </label>
              <Select
                v-model="redeemForm.plan_id"
                :options="subscriptionRedeemSelectOptions"
                :placeholder="t('common.selectOption')"
                class="w-full"
              />
              <p v-if="selectedSubscriptionOption" class="mt-2 text-xs text-content-tertiary">
                {{ selectedSubscriptionDetail }}
              </p>
            </div>
          </template>
        </div>
      </div>

      <template #footer>
        <button type="button" class="btn btn-secondary" :disabled="redeeming" @click="closeRedeemDialog">
          {{ t('common.cancel') }}
        </button>
        <button type="button" class="btn btn-primary" :disabled="!canSubmitRedemption" @click="redeemPoints">
          <Icon v-if="redeeming" name="refresh" size="sm" class="animate-spin" />
          <span>{{ redeeming ? t('affiliate.redeem.redeeming') : t('affiliate.redeem.confirm') }}</span>
        </button>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import userAPI, { type AffiliateRedeemTargetType } from '@/api/user'
import { paymentAPI } from '@/api/payment'
import subscriptionsAPI from '@/api/subscriptions'
import type { AffiliateInvitee, AffiliateLedgerRecord, SubscriptionPlan, UserAffiliateDetail, UserSubscription } from '@/types'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { useClipboard } from '@/composables/useClipboard'
import { formatCurrency, formatDateTime } from '@/utils/format'
import { extractApiErrorMessage } from '@/utils/apiError'

interface SubscriptionRedeemOption {
  group_id: number
  plan_id?: number
  group_name: string
  expires_at?: string | null
  plan_price?: number
  validity_days?: number
}

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()
const { copyToClipboard } = useClipboard()

const loading = ref(true)
const redeeming = ref(false)
const detail = ref<UserAffiliateDetail | null>(null)
const ledgerDialog = ref(false)
const ledgerLoading = ref(false)
const ledgerRecords = ref<AffiliateLedgerRecord[]>([])
const ledgerPagination = reactive({ page: 1, page_size: 10, total: 0 })
const redeemDialog = ref(false)
const redemptionContextLoading = ref(false)
const redemptionContextLoaded = ref(false)
const activeSubscriptions = ref<UserSubscription[]>([])
const subscriptionPlans = ref<SubscriptionPlan[]>([])
const rechargeMultiplier = ref<number | null>(null)
const redeemForm = reactive<{
  target_type: AffiliateRedeemTargetType
  points: number | null
  group_id: number | null
  plan_id: number | null
}>({
  target_type: 'balance',
  points: null,
  group_id: null,
  plan_id: null,
})

const inviteLink = computed(() => {
  if (!detail.value?.can_invite || !detail.value.aff_code) return ''
  if (typeof window === 'undefined') return `/register?aff=${encodeURIComponent(detail.value.aff_code)}`
  return `${window.location.origin}/register?aff=${encodeURIComponent(detail.value.aff_code)}`
})

// Rebate rate is a percentage in the range [0, 100]; backend already clamps it.
// We trim trailing zeros (e.g. 20.00 → "20", 12.50 → "12.5") for a cleaner UI.
const formattedRebateRate = computed(() => {
  const v = detail.value?.effective_rebate_rate_percent ?? 0
  const rounded = Math.round(v * 100) / 100
  return Number.isInteger(rounded) ? String(rounded) : rounded.toString()
})

const affiliatePointsEpsilon = 0.00000001
const availableRebatePoints = computed(() => detail.value?.available_points ?? detail.value?.available_rebate_points ?? detail.value?.aff_quota ?? 0)
const frozenRebatePoints = computed(() => detail.value?.frozen_rebate_points ?? detail.value?.aff_frozen_quota ?? 0)
const rebatePerInviteeCap = computed(() => {
  const detailCap = detail.value?.affiliate_rebate_per_invitee_cap
  if (Number(detailCap) > 0) return Number(detailCap)
  return Number(appStore.cachedPublicSettings?.affiliate_rebate_per_invitee_cap || 0)
})

const inviteLimitLabel = computed(() => {
  const limit = detail.value?.effective_invite_limit ?? 0
  return limit > 0
    ? t('affiliate.stats.inviteLimit', { count: formatCount(limit) })
    : t('affiliate.stats.inviteLimitUnavailable')
})

const subscriptionRedeemOptions = computed<SubscriptionRedeemOption[]>(() => {
  return subscriptionPlans.value
    .filter((plan) => plan.for_sale !== false)
    .map((plan) => {
      const customSubscription = activeSubscriptions.value.find((item) =>
        item.group?.is_custom_subscription_group === true &&
        item.group?.custom_source_plan_id === plan.id,
      )
      const subscription = customSubscription || activeSubscriptions.value.find((item) => item.group_id === plan.group_id)
      const multiplier = customSubscription?.group?.custom_multiplier && customSubscription.group.custom_multiplier >= 1
        ? customSubscription.group.custom_multiplier
        : 1
      const groupId = customSubscription?.group_id || plan.group_id
      return {
        group_id: groupId,
        plan_id: plan.id,
        group_name: customSubscription?.group?.name || plan.group_name || subscription?.group?.name || t('affiliate.redeem.unknownGroup', { id: groupId }),
        expires_at: subscription?.expires_at,
        plan_price: Math.round((Number(plan.price) || 0) * multiplier * 100) / 100,
        validity_days: plan.validity_days,
      }
    })
})

const subscriptionRedeemSelectOptions = computed(() => {
  return subscriptionRedeemOptions.value.map((option) => ({
    value: option.plan_id ?? null,
    label: formatSubscriptionOption(option),
  }))
})

const selectedSubscriptionOption = computed(() => {
  if (!redeemForm.plan_id) return null
  return subscriptionRedeemOptions.value.find((option) => option.plan_id === redeemForm.plan_id) ?? null
})

const subscriptionRedeemCost = computed(() => {
  const price = selectedSubscriptionOption.value?.plan_price ?? 0
  return Math.max(0, Number(price) || 0)
})

const selectedSubscriptionDetail = computed(() => {
  if (!selectedSubscriptionOption.value) return ''
  if (subscriptionRedeemCost.value > availableRebatePoints.value) {
    return t('affiliate.redeem.insufficientPoints')
  }
  return t('affiliate.redeem.remainingPoints', {
    points: formatPoints(availableRebatePoints.value - subscriptionRedeemCost.value),
  })
})

const maxBalanceRedeemPoints = computed(() => Math.min(availableRebatePoints.value, 100))
const redeemPointsValue = computed(() => Math.max(0, Number(redeemForm.points) || 0))
const redemptionRequiredPoints = computed(() => {
  if (redeemForm.target_type === 'subscription') return subscriptionRedeemCost.value
  return redeemPointsValue.value
})
const estimatedBalanceCredit = computed(() => redeemPointsValue.value * (rechargeMultiplier.value ?? 1))
const canSubmitRedemption = computed(() => {
  const required = redemptionRequiredPoints.value
  const available = availableRebatePoints.value
  if (redeeming.value || required <= 0 || required - available > affiliatePointsEpsilon) return false
  if (redeemForm.target_type === 'balance' && required - maxBalanceRedeemPoints.value > affiliatePointsEpsilon) return false
  if (redeemForm.target_type === 'subscription') return !!selectedSubscriptionOption.value?.plan_id
  return true
})

function formatCount(value: number): string {
  return value.toLocaleString()
}

function formatPoints(value: number | null | undefined): string {
  const amount = Number(value || 0)
  const formatted = new Intl.NumberFormat(undefined, {
    minimumFractionDigits: amount > 0 && amount < 0.01 ? 6 : 2,
    maximumFractionDigits: amount > 0 && amount < 0.01 ? 6 : 2,
  }).format(amount)
  return `¥${formatted}`
}

function formatNullablePoints(value: number | null | undefined): string {
  if (value === null || value === undefined) return '-'
  return formatPoints(value)
}

function isLedgerDebit(action: string): boolean {
  return action === 'transfer' || action === 'transfer_subscription' || action === 'clawback'
}

function ledgerAmountClass(action: string): string {
  return isLedgerDebit(action)
    ? 'text-rose-600 dark:text-rose-400'
    : 'text-emerald-600 dark:text-emerald-400'
}

function formatSignedLedgerAmount(item: AffiliateLedgerRecord): string {
  const prefix = isLedgerDebit(item.action) ? '-' : '+'
  return `${prefix}${formatPoints(item.amount)}`
}

function formatLedgerAction(item: AffiliateLedgerRecord): string {
  if (item.action === 'accrue') {
    if (item.frozen_until) return t('affiliate.ledger.actions.accrueFrozen')
    return t('affiliate.ledger.actions.accrue')
  }
  if (item.action === 'transfer_subscription' || (item.action === 'transfer' && item.subscription_group_id)) {
    return t('affiliate.ledger.actions.transferSubscription')
  }
  if (item.action === 'transfer') return t('affiliate.ledger.actions.transferBalance')
  if (item.action === 'clawback') return t('affiliate.ledger.actions.clawback')
  return item.action || '-'
}

function formatPrivateAccount(value: string | null | undefined): string {
  const account = String(value || '').trim()
  if (!account) return '-'
  if (account.includes('***')) return account
  const atIndex = account.indexOf('@')
  if (atIndex > 0) {
    const local = account.slice(0, atIndex)
    const domain = account.slice(atIndex)
    if (local.length <= 2) return `${local.slice(0, 1)}***${domain}`
    return `${local.slice(0, 2)}***${domain}`
  }
  if (account.length <= 2) return `${account.slice(0, 1)}***`
  return `${account.slice(0, 2)}***`
}

function formatLedgerSource(item: AffiliateLedgerRecord): string {
  return formatPrivateAccount(item.source_user_email || item.source_username)
}

function formatLedgerOrder(item: AffiliateLedgerRecord): string {
  if (!item.source_order_id && !item.out_trade_no) return '-'
  const id = item.source_order_id ? `#${item.source_order_id}` : ''
  return [id, item.out_trade_no].filter(Boolean).join(' · ')
}

function formatLedgerGroup(item: AffiliateLedgerRecord): string {
  if (!item.subscription_group_id && !item.subscription_group_name) return '-'
  return item.subscription_group_name || t('affiliate.redeem.unknownGroup', { id: item.subscription_group_id })
}

function formatEstimatedDays(value: number): string {
  if (value < 0.1) {
    return '<0.1'
  }
  const rounded = Math.round(value * 10) / 10
  return Number.isInteger(rounded) ? String(rounded) : rounded.toString()
}

function floorAffiliateRedeemPoints(value: number): number {
  return Math.floor((value + 1e-9) * 10) / 10
}

function fillMaxBalanceRedeemPoints(): void {
  redeemForm.points = floorAffiliateRedeemPoints(maxBalanceRedeemPoints.value)
}

function inviteeRebatePoints(item: AffiliateInvitee): number {
  return item.total_rebate_points ?? item.total_rebate ?? 0
}

function formatSubscriptionOption(option: SubscriptionRedeemOption): string {
  const suffix = option.plan_price && option.validity_days
    ? ` · ${formatCurrency(option.plan_price, 'CNY')} / ${formatCount(option.validity_days)}${t('affiliate.redeem.daysUnit')}`
    : ''
  return `${option.group_name}${suffix}`
}

async function ensureAffiliateEnabled(): Promise<boolean> {
  const settings = await appStore.fetchPublicSettings(!appStore.publicSettingsLoaded)
  const enabled = settings?.affiliate_enabled === true || appStore.cachedPublicSettings?.affiliate_enabled === true
  if (!enabled) {
    await router.replace('/dashboard')
    return false
  }
  return true
}

async function loadAffiliateDetail(silent = false): Promise<void> {
  if (!silent) {
    loading.value = true
  }
  try {
    detail.value = await userAPI.getAffiliateDetail()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('affiliate.loadFailed')))
  } finally {
    if (!silent) {
      loading.value = false
    }
  }
}

async function openLedgerDialog(): Promise<void> {
  ledgerDialog.value = true
  ledgerPagination.page = 1
  await loadLedgerRecords()
}

async function loadLedgerRecords(): Promise<void> {
  ledgerLoading.value = true
  try {
    const resp = await userAPI.getAffiliateLedger({
      page: ledgerPagination.page,
      page_size: ledgerPagination.page_size,
      sort_by: 'created_at',
      sort_order: 'desc',
    })
    ledgerRecords.value = resp.items || []
    ledgerPagination.page = resp.page
    ledgerPagination.page_size = resp.page_size
    ledgerPagination.total = resp.total
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('affiliate.ledger.loadFailed')))
  } finally {
    ledgerLoading.value = false
  }
}

function handleLedgerPageChange(page: number): void {
  ledgerPagination.page = page
  void loadLedgerRecords()
}

function handleLedgerPageSizeChange(pageSize: number): void {
  ledgerPagination.page = 1
  ledgerPagination.page_size = pageSize
  void loadLedgerRecords()
}

async function loadRedemptionContext(): Promise<void> {
  if (redemptionContextLoaded.value || redemptionContextLoading.value) return
  redemptionContextLoading.value = true
  try {
    const [checkoutResult, subscriptionsResult] = await Promise.allSettled([
      paymentAPI.getCheckoutInfo(),
      subscriptionsAPI.getActiveSubscriptions(),
    ])
    if (checkoutResult.status === 'fulfilled') {
      rechargeMultiplier.value = checkoutResult.value.data.balance_recharge_multiplier || 1
      subscriptionPlans.value = checkoutResult.value.data.plans || []
    }
    if (subscriptionsResult.status === 'fulfilled') {
      activeSubscriptions.value = subscriptionsResult.value || []
    }
    redemptionContextLoaded.value = true
  } finally {
    redemptionContextLoading.value = false
  }
}

async function openRedeemDialog(): Promise<void> {
  redeemForm.target_type = 'balance'
  redeemForm.points = null
  redeemForm.group_id = null
  redeemForm.plan_id = null
  redeemDialog.value = true
  await loadRedemptionContext()
}

function closeRedeemDialog(): void {
  if (redeeming.value) return
  redeemDialog.value = false
}

async function copyCode(): Promise<void> {
  if (!detail.value?.can_invite || !detail.value.aff_code) return
  await copyToClipboard(detail.value.aff_code, t('affiliate.codeCopied'))
}

async function copyInviteLink(): Promise<void> {
  if (!inviteLink.value) return
  await copyToClipboard(inviteLink.value, t('affiliate.linkCopied'))
}

async function redeemPoints(): Promise<void> {
  if (!canSubmitRedemption.value) return
  redeeming.value = true
  try {
    const resp = await userAPI.redeemAffiliatePoints({
      target_type: redeemForm.target_type,
      points: redemptionRequiredPoints.value,
      group_id: redeemForm.target_type === 'subscription' ? selectedSubscriptionOption.value?.group_id : undefined,
      plan_id: redeemForm.target_type === 'subscription' ? selectedSubscriptionOption.value?.plan_id : undefined,
    })
    if (redeemForm.target_type === 'subscription') {
      appStore.showSuccess(t('affiliate.redeem.subscriptionSuccess', {
        points: formatPoints(resp.redeemed_points),
        days: formatEstimatedDays(resp.transferred_days ?? 0),
        group: resp.group_name || selectedSubscriptionOption.value?.group_name || '-',
      }))
    } else {
      appStore.showSuccess(t('affiliate.redeem.balanceSuccess', {
        points: formatPoints(resp.redeemed_points),
        balance: formatCurrency(resp.credited_balance ?? 0),
      }))
    }
    redeemDialog.value = false
    await Promise.all([
      loadAffiliateDetail(true),
      authStore.refreshUser().catch(() => undefined),
      subscriptionsAPI.getActiveSubscriptions().then((items) => {
        activeSubscriptions.value = items
      }).catch(() => undefined),
    ])
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('affiliate.redeem.failed')))
  } finally {
    redeeming.value = false
  }
}

onMounted(async () => {
  if (await ensureAffiliateEnabled()) {
    await loadAffiliateDetail()
  }
})
</script>
