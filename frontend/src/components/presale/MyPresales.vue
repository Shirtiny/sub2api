<template>
  <section v-if="orders.length || loadError" class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3"><h2 class="text-lg font-medium text-content-primary">{{ t('presale.myPresales') }}</h2><RouterLink to="/presale" class="btn btn-secondary text-xs">{{ t('presale.nav') }} →</RouterLink></div>
    <p class="text-xs text-content-tertiary">{{ t('presale.myPresalesCopy') }}</p>
    <p v-if="loadError" class="text-sm text-red-500">{{ t('presale.failed') }} <button class="underline" @click="load">{{ t('presale.retry') }}</button></p>
    <div class="grid gap-4 md:grid-cols-2">
      <article v-for="order in orders" :key="order.id" class="card space-y-3 p-4">
        <PresaleOrderTerm :order="order" />
        <div class="flex items-center justify-between gap-3 text-xs text-content-tertiary"><span>#{{ order.id }} · {{ formatPaymentAmount(order.pay_amount, order.currency) }}</span><OrderStatusBadge :status="order.status" /></div>
        <div class="flex justify-end gap-4"><RouterLink to="/orders" class="text-xs text-primary-600 dark:text-primary-400">{{ t('presale.viewOrders') }}</RouterLink><button v-if="order.status === 'COMPLETED' || (order.status === 'FAILED' && order.paid_at)" class="text-xs text-content-secondary" :disabled="busy" @click="openRefund(order)">{{ t('presale.requestRefund') }}</button></div>
      </article>
    </div>
    <BaseDialog :show="!!refundOrder" :title="t('presale.refundTitle')" @close="!busy && (refundOrder = null)">
      <div v-if="quote" class="space-y-4"><p class="text-sm leading-relaxed text-content-secondary">{{ t('presale.refundNotice') }}</p><div class="rounded-lg bg-surface-hover p-4"><p class="text-xs text-content-tertiary">{{ t('presale.refundAmount') }}</p><strong class="mt-2 block text-xl text-content-primary">{{ formatPaymentAmount(quote.gateway_amount, quote.currency) }}</strong><p v-if="quote.fee_percent" class="mt-2 text-xs text-content-secondary">{{ t('presale.prepareCopy') }}</p></div></div>
      <template #footer><button class="btn btn-secondary" :disabled="busy" @click="refundOrder = null">{{ t('common.cancel') }}</button><button class="btn btn-primary ml-3" :disabled="busy || !quote" @click="requestRefund">{{ t('presale.refundConfirm') }}</button></template>
    </BaseDialog>
  </section>
</template>
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useSubscriptionStore } from '@/stores/subscriptions'
import { presaleAPI, type PresaleRefundQuote } from '@/api/presale'
import { paymentAPI } from '@/api/payment'
import type { PaymentOrder } from '@/types/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatPaymentAmount } from '@/components/payment/currency'
import BaseDialog from '@/components/common/BaseDialog.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'
import PresaleOrderTerm from './PresaleOrderTerm.vue'
const { t } = useI18n(), app = useAppStore(), subscriptions = useSubscriptionStore()
const orders = ref<PaymentOrder[]>([]), loadError = ref(false), busy = ref(false)
const refundOrder = ref<PaymentOrder | null>(null), quote = ref<PresaleRefundQuote | null>(null)
async function load() {
  loadError.value = false
  try { orders.value = (await presaleAPI.mine()).data }
  catch { loadError.value = true }
}
async function openRefund(order: PaymentOrder) {
  busy.value = true
  try { quote.value = (await presaleAPI.refundQuote(order.id)).data; refundOrder.value = order }
  catch (err) { app.showError(extractI18nErrorMessage(err, t, 'presale.errors', t('common.error'))) }
  finally { busy.value = false }
}
async function requestRefund() {
  if (!refundOrder.value || !quote.value || busy.value) return
  busy.value = true
  try {
    await paymentAPI.requestRefund(refundOrder.value.id, { reason: t('presale.requestRefund'), expected_refund_amount: quote.value.gateway_amount })
    refundOrder.value = null
    app.showSuccess(t('presale.refundSubmitted'))
    await load()
    void subscriptions.fetchActiveSubscriptions(true)
  } catch (err) { app.showError(extractI18nErrorMessage(err, t, 'presale.errors', t('common.error'))) }
  finally { busy.value = false }
}
onMounted(load)
</script>
