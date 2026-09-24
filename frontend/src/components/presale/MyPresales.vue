<template>
  <div v-if="orders.length || loadError" class="space-y-6">
    <p v-if="loadError" class="text-sm text-red-500" role="alert">{{ t('presale.failed') }} <button class="underline" @click="load">{{ t('presale.retry') }}</button></p>
    <section v-for="section in sections" :key="section.key" :data-presale-section="section.key" :aria-labelledby="`presale-${section.key}-title`" class="space-y-4 border-t border-stroke-default pt-6">
      <header class="flex flex-wrap items-center justify-between gap-3">
        <div v-if="section.key === 'pending'" class="flex items-center gap-3">
          <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary-500/10 text-primary-700 dark:text-primary-300"><Icon name="clock" size="md" aria-hidden="true" /></span>
          <div>
            <h2 id="presale-pending-title" class="text-lg font-semibold text-content-primary">{{ t('presale.reservation.pendingTitle') }}</h2>
            <p class="mt-0.5 text-xs text-content-tertiary">{{ t('presale.reservation.pendingHint') }}</p>
          </div>
        </div>
        <h2 v-else id="presale-records-title" class="min-w-0 flex-1">
          <button data-testid="presale-records-toggle" class="flex w-full items-center gap-2.5 text-sm font-medium text-content-secondary" :aria-expanded="showRecords" aria-controls="presale-records-content" @click="showRecords = !showRecords">
            {{ t('presale.reservation.records') }}
            <Icon name="chevronDown" size="sm" class="transition-transform" :class="{ 'rotate-180': showRecords }" aria-hidden="true" />
          </button>
        </h2>
        <RouterLink v-if="section.key === 'pending' || !pendingCards.length" to="/presale" class="inline-flex items-center gap-2 text-xs font-medium text-content-secondary transition-colors hover:text-content-primary">
          {{ t('presale.reservation.browse') }}<Icon name="arrowRight" size="sm" aria-hidden="true" />
        </RouterLink>
      </header>
      <div v-if="section.key === 'pending' || showRecords" :id="`presale-${section.key}-content`" class="space-y-3">
        <p class="reservation-timezone text-[11px] text-content-tertiary">{{ t('presale.reservation.timezone') }}</p>
        <div class="grid gap-4 xl:grid-cols-2">
          <article v-for="card in section.cards" :key="card.order.id" class="reservation-card min-w-0 overflow-hidden rounded-2xl border border-stroke-default bg-surface-card" :class="{ 'reservation-arriving': card.status === 'pending' }">
            <header class="flex items-start justify-between gap-4 px-5 pb-4 pt-5">
              <div class="min-w-0">
                <div class="flex items-center gap-2.5">
                  <h3 class="truncate text-xl font-semibold tracking-tight text-content-primary" :title="card.order.presale_plan_name">{{ card.order.presale_plan_name || t('presale.nav') }}</h3>
                  <span v-if="(card.order.subscription_multiplier || 1) > 1" class="reservation-multiplier shrink-0 rounded-md bg-surface-hover px-1.5 py-0.5 text-xs font-medium text-content-secondary">{{ card.order.subscription_multiplier }}×</span>
                </div>
                <p v-if="card.order.presale_renewal" class="mt-1 text-xs text-content-tertiary">{{ t('presale.reservation.renewal') }}</p>
              </div>
              <span class="reservation-status inline-flex max-w-[60%] shrink-0 items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium" :class="statusClass(card.status)" :data-status="card.status">
                <span class="reservation-status-dot h-1.5 w-1.5 shrink-0 rounded-full bg-current" aria-hidden="true" />{{ t(`presale.${card.status}`) }}
              </span>
            </header>
            <div class="reservation-dates mx-5 flex items-center gap-3 border-y border-stroke-default py-4 sm:gap-5">
              <div class="min-w-0 flex-1">
                <p class="text-[11px] font-medium text-content-tertiary">{{ t('presale.reservation.starts') }}</p>
                <time class="reservation-start mt-2 block" :datetime="card.order.presale_starts_at">
                  <span class="block whitespace-nowrap text-lg font-medium tabular-nums tracking-tight text-content-primary sm:text-xl">{{ card.start.date }}</span>
                  <span class="mt-1 block text-xs tabular-nums text-content-secondary">{{ card.start.time }}</span>
                </time>
              </div>
              <div class="reservation-connector flex shrink-0 items-center text-content-tertiary" aria-hidden="true">
                <span class="hidden h-px w-6 bg-stroke-default sm:block" /><Icon name="arrowRight" size="sm" />
              </div>
              <div class="min-w-0 flex-1 text-right">
                <p class="text-[11px] font-medium text-content-tertiary">{{ t('presale.reservation.ends') }}</p>
                <time class="reservation-end mt-2 block" :datetime="card.order.presale_expires_at">
                  <span class="block whitespace-nowrap text-lg font-medium tabular-nums tracking-tight text-content-primary sm:text-xl">{{ card.end.date }}</span>
                  <span class="mt-1 block text-xs tabular-nums text-content-secondary">{{ card.end.time }}</span>
                </time>
              </div>
            </div>
            <footer class="flex flex-wrap items-center justify-between gap-x-5 gap-y-3 px-5 py-4 text-xs">
              <div class="flex flex-wrap items-center gap-2 text-content-tertiary">
                <span>{{ t('payment.orders.payAmount') }} <strong class="font-medium text-content-secondary">{{ formatPaymentAmount(card.order.pay_amount, card.order.currency, locale) }}</strong></span>
                <span aria-hidden="true">·</span><span class="tabular-nums">#{{ card.order.id }}</span>
              </div>
              <div class="flex items-center gap-4">
                <RouterLink to="/orders" class="text-content-secondary transition-colors hover:text-content-primary">{{ t('presale.viewOrders') }}</RouterLink>
                <button v-if="card.order.status === 'COMPLETED' || (card.order.status === 'FAILED' && card.order.paid_at)" class="text-content-tertiary transition-colors hover:text-content-primary disabled:opacity-50" :disabled="busy" @click="openRefund(card.order)">{{ t('presale.requestRefund') }}</button>
              </div>
            </footer>
          </article>
        </div>
      </div>
    </section>
    <BaseDialog :show="!!refundOrder" :title="t('presale.refundTitle')" @close="!busy && (refundOrder = null)">
      <div v-if="quote" class="space-y-4"><p class="text-sm leading-relaxed text-content-secondary">{{ t('presale.refundNotice') }}</p><div class="rounded-lg bg-surface-hover p-4"><p class="text-xs text-content-tertiary">{{ t('presale.refundAmount') }}</p><strong class="mt-2 block text-xl text-content-primary">{{ formatPaymentAmount(quote.gateway_amount, quote.currency) }}</strong><p v-if="quote.fee_percent" class="mt-2 text-xs text-content-secondary">{{ t('presale.refundFeeApplied', { percent: quote.fee_percent }) }}</p></div></div>
      <template #footer><button class="btn btn-secondary" :disabled="busy" @click="refundOrder = null">{{ t('common.cancel') }}</button><button class="btn btn-primary ml-3" :disabled="busy || !quote" @click="requestRefund">{{ t('presale.refundConfirm') }}</button></template>
    </BaseDialog>
  </div>
</template>
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useSubscriptionStore } from '@/stores/subscriptions'
import { presaleAPI, type PresaleRefundQuote } from '@/api/presale'
import { paymentAPI } from '@/api/payment'
import type { PaymentOrder } from '@/types/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatPaymentAmount } from '@/components/payment/currency'
import { presaleStatus } from '@/utils/presale'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
const { t, locale } = useI18n(), app = useAppStore(), subscriptions = useSubscriptionStore()
const emit = defineEmits<{ refunded: [] }>()
const orders = ref<PaymentOrder[]>([]), loadError = ref(false), busy = ref(false)
const showRecords = ref(false)
const refundOrder = ref<PaymentOrder | null>(null), quote = ref<PresaleRefundQuote | null>(null)
const dateFormatter = new Intl.DateTimeFormat('en-GB', {
  timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit',
  hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
})
function dateParts(value?: string) {
  const date = new Date(value || '')
  if (!Number.isFinite(date.getTime())) return { date: '—', time: '—' }
  const parts = Object.fromEntries(dateFormatter.formatToParts(date).map(part => [part.type, part.value]))
  return { date: `${parts.year}.${parts.month}.${parts.day}`, time: `${parts.hour}:${parts.minute}` }
}
const cards = computed(() => orders.value.map(order => ({
  order, status: presaleStatus(order), start: dateParts(order.presale_starts_at), end: dateParts(order.presale_expires_at),
})))
const pendingCards = computed(() => cards.value.filter(card => card.status === 'pending' || card.status === 'activationIssue'))
const sections = computed(() => [
  { key: 'pending', cards: pendingCards.value },
  { key: 'records', cards: cards.value.filter(card => card.status !== 'pending' && card.status !== 'activationIssue') },
].filter(section => section.cards.length))
function statusClass(status: string): string {
  switch (status) {
    case 'pending': return 'bg-primary-500/10 text-primary-700 dark:text-primary-300'
    case 'active': return 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
    case 'refundPending': return 'bg-amber-500/10 text-amber-700 dark:text-amber-300'
    case 'activationIssue': return 'bg-red-500/10 text-red-700 dark:text-red-300'
    default: return 'bg-surface-hover text-content-tertiary'
  }
}
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
    emit('refunded')
    await load()
    void subscriptions.fetchActiveSubscriptions(true)
  } catch (err) { app.showError(extractI18nErrorMessage(err, t, 'presale.errors', t('common.error'))) }
  finally { busy.value = false }
}
onMounted(load)
</script>

<style scoped>
/* Decorative arrival cues only; dates and controls never move, and no progress
   is implied before the subscription actually activates. */
.reservation-arriving {
  position: relative;
  isolation: isolate;
}

.reservation-arriving::before,
.reservation-arriving::after {
  content: '';
  position: absolute;
  pointer-events: none;
}

.reservation-arriving::before {
  z-index: -1;
  inset: 0;
  background: radial-gradient(ellipse at 85% 0%, rgb(var(--color-primary-300) / .17), transparent 65%);
  animation: arrival-warmth 6s ease-in-out infinite;
}

.reservation-arriving::after {
  top: 0;
  left: 0;
  width: 36%;
  height: 1px;
  background: linear-gradient(90deg, transparent, rgb(var(--color-primary-text-300, var(--color-primary-300)) / .8), transparent);
  box-shadow: 0 0 8px rgb(var(--color-primary-300) / .25);
  animation: arrival-glint 7s cubic-bezier(.4, 0, .2, 1) -1.4s infinite;
}

.reservation-arriving .reservation-status-dot {
  position: relative;
}

.reservation-arriving .reservation-status-dot::after {
  content: '';
  position: absolute;
  inset: -4px;
  border: 1px solid currentColor;
  border-radius: 50%;
  pointer-events: none;
  animation: arrival-pulse 4s ease-out infinite;
}

@keyframes arrival-warmth {
  0%, 100% { opacity: .3; }
  50% { opacity: .9; }
}

@keyframes arrival-glint {
  0% { transform: translateX(-110%); opacity: 0; }
  15%, 65% { opacity: .85; }
  85%, 100% { transform: translateX(310%); opacity: 0; }
}

@keyframes arrival-pulse {
  0% { transform: scale(.45); opacity: 0; }
  20% { opacity: .5; }
  80%, 100% { transform: scale(1.2); opacity: 0; }
}

@media (prefers-reduced-motion: reduce) {
  .reservation-arriving::before { animation: none; opacity: .4; }
  .reservation-arriving::after,
  .reservation-arriving .reservation-status-dot::after { animation: none; opacity: 0; }
}
</style>
