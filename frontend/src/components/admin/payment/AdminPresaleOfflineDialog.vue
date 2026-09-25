<template>
  <BaseDialog :show="show" :title="t('presale.offline.title')" @close="close">
    <form id="presale-offline-form" class="space-y-5" @submit.prevent="submit">
      <div v-if="order" class="rounded-xl border border-stroke bg-surface-secondary p-4 text-sm">
        <div class="flex justify-between gap-3"><span class="font-mono">#{{ order.id }}</span><OrderStatusBadge :status="order.status" /></div>
        <p class="mt-3 font-medium text-content-primary">{{ order.presale_plan_name || summary?.plan_name || t('adminOrderDetail.planFallback', { id: order.plan_id }) }} <span class="ml-2 font-normal text-content-secondary">× {{ order.subscription_multiplier || 1 }}</span></p>
        <p class="mt-1 break-all text-content-secondary">{{ order.user_email || `#${order.user_id}` }}</p>
        <div class="mt-4 grid grid-cols-2 gap-4 text-content-secondary">
          <div><p class="text-xs">{{ t('adminOrderDetail.starts') }}</p><p class="mt-1 tabular-nums">{{ formatPresaleDate(order.presale_starts_at, locale) }}</p></div>
          <div><p class="text-xs">{{ t('adminOrderDetail.ends') }}</p><p class="mt-1 tabular-nums">{{ formatPresaleDate(order.presale_expires_at, locale) }}</p></div>
        </div>
        <p class="mt-4 flex justify-between"><span>{{ t('adminOrderDetail.paid') }}</span><strong>{{ formatPaymentAmount(order.pay_amount, currency) }}</strong></p>
      </div>
      <fieldset :disabled="submitting || retryOnly" class="space-y-4">
        <legend class="sr-only">{{ t('presale.offline.title') }}</legend>
        <label class="block text-sm text-content-secondary">{{ t('presale.offline.action') }}
          <select v-model="mode" class="input mt-2" data-testid="offline-mode">
            <option v-if="canCancel" value="cancel">{{ t('presale.offline.cancel') }}</option>
            <option value="refund">{{ t('presale.offline.refund') }}</option>
          </select>
        </label>
        <p class="rounded-lg bg-amber-50 p-3 text-sm leading-relaxed text-amber-800 dark:bg-amber-900/20 dark:text-amber-200">{{ t(`presale.offline.${mode}Notice`) }}</p>
        <template v-if="mode === 'refund'">
          <label class="block text-sm text-content-secondary">{{ t('presale.offline.amount') }} · {{ currency }}
            <input v-model="amount" type="number" :step="step" :min="step" :max="order?.pay_amount" required class="input mt-2" data-testid="offline-amount" />
          </label>
          <label class="block text-sm text-content-secondary">{{ t('presale.offline.reference') }}
            <input v-model="reference" maxlength="200" required class="input mt-2" data-testid="offline-reference" />
          </label>
        </template>
        <label class="block text-sm text-content-secondary">{{ t('presale.offline.reason') }}
          <textarea v-model="reason" rows="2" maxlength="500" required class="input mt-2" data-testid="offline-reason" />
        </label>
        <label class="flex items-start gap-3 text-sm leading-relaxed text-content-secondary">
          <input v-model="confirmed" type="checkbox" class="mt-1" data-testid="offline-confirm" />
          <span>{{ t(`presale.offline.${mode}Confirm`) }}</span>
        </label>
      </fieldset>
      <p v-if="retryOnly" role="alert" class="text-sm text-amber-700 dark:text-amber-300">{{ t('presale.offline.affiliatePending') }}</p>
    </form>
    <template #footer>
      <button class="btn btn-secondary" :disabled="submitting" @click="close">{{ t('common.close') }}</button>
      <button class="btn btn-primary" form="presale-offline-form" type="submit" :disabled="submitting || (!retryOnly && !valid)" data-testid="offline-submit">
        {{ submitting ? t('common.processing') : retryOnly ? t('presale.offline.retry') : t('presale.offline.submit') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AdminOrderSummary, AdminPaymentOrder, PresaleOfflineRequest } from '@/api/admin/payment'
import BaseDialog from '@/components/common/BaseDialog.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'
import { formatPaymentAmount, normalizePaymentCurrency } from '@/components/payment/currency'
import { formatPresaleDate } from '@/utils/presale'

const props = defineProps<{ show: boolean; order: AdminPaymentOrder | null; summary: AdminOrderSummary | null; submitting: boolean; retryOnly?: boolean }>()
const emit = defineEmits<{ cancel: []; confirm: [data: PresaleOfflineRequest] }>()
const { t, locale } = useI18n()
const mode = ref<'cancel' | 'refund'>('cancel')
const amount = ref<string | number>('')
const reason = ref('')
const reference = ref('')
const confirmed = ref(false)
const canCancel = computed(() => !!props.order && ['COMPLETED', 'FAILED'].includes(props.order.status))
const currency = computed(() => normalizePaymentCurrency(props.summary?.currency || props.order?.currency))
const digits = computed(() => new Intl.NumberFormat('en', { style: 'currency', currency: currency.value }).resolvedOptions().maximumFractionDigits ?? 2)
const step = computed(() => 10 ** -digits.value)
const valid = computed(() => {
  if (!props.order?.updated_at || !confirmed.value || !reason.value.trim() || reason.value.trim().length > 500) return false
  if (mode.value === 'cancel') return canCancel.value
  const value = Number(amount.value)
  return Number.isFinite(value) && value > 0 && value <= props.order.pay_amount && value === Number(value.toFixed(digits.value)) && !!reference.value.trim() && reference.value.trim().length <= 200
})
watch(() => [props.show, props.order?.id], () => {
  mode.value = canCancel.value ? 'cancel' : 'refund'
  amount.value = ''; reason.value = ''; reference.value = ''; confirmed.value = false
}, { immediate: true })
watch(mode, () => { confirmed.value = false })
function close() { if (!props.submitting) emit('cancel') }
function submit() {
  if (props.submitting || !valid.value || !props.order?.updated_at) return
  emit('confirm', { mode: mode.value, amount: mode.value === 'refund' ? Number(amount.value) : 0,
    reason: reason.value.trim(), reference: mode.value === 'refund' ? reference.value.trim() : '', confirmed: true, expected_updated_at: props.order.updated_at })
}
</script>
