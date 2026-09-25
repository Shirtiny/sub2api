<template>
  <section class="reservation-confirmation" data-test="presale-success">
    <header class="confirmation-heading" role="status">
      <svg class="confirmation-seal" viewBox="0 0 48 48" fill="none" aria-hidden="true">
        <circle cx="24" cy="24" r="21" stroke="currentColor" stroke-opacity=".25" />
        <circle cx="24" cy="24" r="16" fill="currentColor" fill-opacity=".06" />
        <path class="confirmation-check" d="m16 24 5.5 5.5L32 19" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
      <div>
        <p class="confirmation-eyebrow" aria-hidden="true">Reserved for you</p>
        <h2>{{ t('presale.purchased') }}</h2>
        <p class="confirmation-intro">{{ statusCopy }}</p>
      </div>
    </header>

    <div class="confirmation-reservation">
      <div class="reservation-summary">
        <div class="reservation-plan">
          <p class="confirmation-label">{{ t(order.presale_renewal ? 'presale.renewal' : 'presale.term') }}</p>
          <div class="reservation-plan-name">
            <h3>{{ order.presale_plan_name || t('presale.nav') }}</h3>
            <span v-if="(order.subscription_multiplier || 1) > 1" class="reservation-multiplier">{{ order.subscription_multiplier }}×</span>
          </div>
          <span class="reservation-state" :data-state="status"><span aria-hidden="true"></span>{{ t(`presale.${status}`) }}</span>
        </div>
        <div class="reservation-payment">
          <p class="confirmation-label">{{ t('payment.orders.payAmount') }}</p>
          <strong data-test="paid-amount">{{ money(order.pay_amount) }}</strong>
        </div>
      </div>

      <dl class="reservation-dates">
        <div v-for="date in dates" :key="date.label">
          <dt><span aria-hidden="true"></span>{{ date.label }}</dt>
          <dd><time :datetime="date.value">{{ formatPresaleDate(date.value, locale) }}</time></dd>
        </div>
      </dl>

      <div v-if="bonus" class="reservation-gift" data-test="purchase-bonus">
        <Icon name="gift" size="sm" aria-hidden="true" />
        <span>{{ t('presale.gift.eligible') }}</span>
        <strong>{{ bonus }}</strong>
      </div>
    </div>

    <div class="confirmation-actions">
      <button type="button" class="confirmation-secondary" @click="emit('done')">{{ secondaryLabel || t('common.close') }}</button>
      <button type="button" class="btn btn-primary" @click="emit('viewSubscriptions')">
        {{ t('payment.result.viewSubscriptions') }}<Icon name="arrowRight" size="sm" aria-hidden="true" />
      </button>
    </div>

    <details class="confirmation-details">
      <summary>{{ t('presale.confirmation.details') }}<Icon name="chevronDown" size="sm" aria-hidden="true" /></summary>
      <dl>
        <div><dt>{{ t('payment.orders.orderId') }}</dt><dd>#{{ order.id }}</dd></div>
        <div v-if="order.out_trade_no"><dt>{{ t('payment.orders.orderNo') }}</dt><dd class="order-number">{{ order.out_trade_no }}</dd></div>
        <div><dt>{{ t('payment.orders.subscriptionAmount') }}</dt><dd>{{ money(order.amount) }}</dd></div>
        <div v-if="discount > 0"><dt>{{ t('payment.cafeCoupon.discountLabel') }}</dt><dd>−{{ money(discount) }}</dd></div>
        <div v-if="order.fee_rate > 0"><dt>{{ t('payment.orders.fee') }} · {{ order.fee_rate }}%</dt><dd>{{ money(fee) }}</dd></div>
        <div><dt>{{ t('payment.orders.paymentMethod') }}</dt><dd>{{ t(paymentMethodI18nKey(order.payment_type), normalizePaymentMethodForDisplay(order.payment_type) || order.payment_type) }}</dd></div>
      </dl>
    </details>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { formatPaymentAmount } from '@/components/payment/currency'
import type { PaymentOrder } from '@/types/payment'
import { formatPresaleDate, presaleStatus } from '@/utils/presale'
import { normalizePaymentMethodForDisplay, paymentMethodI18nKey } from '@/views/user/paymentUx'

const props = defineProps<{ order: PaymentOrder; currency?: string; secondaryLabel?: string }>()
const emit = defineEmits<{ done: []; viewSubscriptions: [] }>()
const { t, locale } = useI18n()
const status = computed(() => presaleStatus(props.order))
const statusCopy = computed(() => {
  if (status.value === 'pending') return t('presale.confirmation.scheduled')
  if (status.value === 'active') return t('presale.confirmation.active')
  if (status.value === 'expired') return t('presale.confirmation.expired')
  return t(`presale.${status.value}`)
})
const dates = computed(() => [
  { label: t('presale.reservation.starts'), value: props.order.presale_starts_at },
  { label: t('presale.reservation.ends'), value: props.order.presale_expires_at },
])
const money = (amount: number) => formatPaymentAmount(amount, props.order.currency || props.currency, locale.value)
const discount = computed(() => Math.max(0, Number(props.order.cafe_coupon_discount) || 0))
const fee = computed(() => Math.round((props.order.pay_amount - Math.max(0, props.order.amount - discount.value)) * 100) / 100)
const bonus = computed(() => {
  // Show the purchased benefit, not the current campaign or a guessed conversion.
  const order = props.order
  if (!(Number(order.presale_balance_bonus_amount) > 0)) return ''
  if (order.presale_balance_bonus_currency === 'CNY') {
    if (!(Number(order.presale_balance_bonus_face_amount) > 0)) return ''
    return formatPaymentAmount(order.presale_balance_bonus_face_amount!, 'CNY', locale.value)
  }
  return `${formatPaymentAmount(order.presale_balance_bonus_amount!, 'USD', locale.value)} USD`
})
</script>

<style scoped>
.reservation-confirmation {
  --confirmation-ink: #392f29;
  --confirmation-muted: #74685d;
  --confirmation-accent: #876337;
  --confirmation-line: rgb(114 88 58 / 17%);
  color: var(--confirmation-ink);
  width: 100%;
  max-width: 760px;
  margin-inline: auto;
  padding: clamp(20px, 4vw, 36px);
  border: 1px solid var(--confirmation-line);
  border-radius: 20px;
  background: radial-gradient(ellipse at 100% 0, rgb(204 165 103 / 12%), transparent 55%), #fcfaf6;
  box-shadow: 0 16px 48px -30px rgb(38 28 17 / 20%);
  animation: confirmation-arrive .5s ease-out both;
}
.dark .reservation-confirmation {
  --confirmation-ink: #f3e9d9;
  --confirmation-muted: #b5a591;
  --confirmation-accent: #e3c591;
  --confirmation-line: rgb(219 190 146 / 16%);
  background: radial-gradient(ellipse at 100% 0, rgb(180 140 78 / 9%), transparent 55%), #201c18;
  box-shadow: 0 20px 56px -28px rgb(0 0 0 / 40%);
}
.confirmation-heading { display: flex; align-items: flex-start; gap: 18px; }
.confirmation-seal { flex: 0 0 48px; width: 48px; color: var(--confirmation-accent); margin-top: 3px; }
.confirmation-check { stroke-dasharray: 25; stroke-dashoffset: 0; animation: confirmation-check .65s .15s ease-out both; }
.confirmation-eyebrow { font: italic 17px/1.4 Georgia, 'Times New Roman', serif; color: var(--confirmation-accent); }
.confirmation-heading h2 { font-size: clamp(22px, 3vw, 26px); line-height: 1.5; font-weight: 500; letter-spacing: .025em; margin-top: 6px; }
.confirmation-intro { margin-top: 8px; color: var(--confirmation-muted); font-size: 13px; line-height: 1.8; }
.confirmation-reservation { margin-top: 30px; padding-block: 26px; border-block: 1px solid var(--confirmation-line); }
.reservation-summary { display: flex; align-items: flex-start; justify-content: space-between; gap: 22px; }
.confirmation-label { color: var(--confirmation-muted); font-size: 11px; line-height: 1.6; }
.reservation-plan { min-width: 0; }
.reservation-plan-name { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; margin-top: 8px; }
.reservation-plan-name h3 { font-size: 27px; font-weight: 500; line-height: 1.4; overflow-wrap: anywhere; }
.reservation-multiplier { padding: 2px 8px; border: 1px solid var(--confirmation-line); border-radius: 6px; font-size: 12px; color: var(--confirmation-accent); }
.reservation-state { display: inline-flex; align-items: center; gap: 7px; margin-top: 10px; font-size: 11px; color: var(--confirmation-accent); }
.reservation-state > span { width: 5px; height: 5px; border-radius: 50%; background: currentColor; box-shadow: 0 0 0 3px rgb(172 143 97 / 10%); }
.reservation-state[data-state='activationIssue'] { color: #d27657; }
.reservation-payment { flex-shrink: 0; text-align: right; }
.reservation-payment strong { display: block; margin-top: 8px; font-size: 26px; font-weight: 400; letter-spacing: -.04em; font-variant-numeric: tabular-nums; }
.reservation-dates { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; margin-top: 30px; }
.reservation-dates > div { min-width: 0; }
.reservation-dates dt { display: flex; align-items: center; gap: 8px; font-size: 11px; color: var(--confirmation-muted); }
.reservation-dates dt > span { width: 6px; height: 6px; border: 1px solid var(--confirmation-accent); border-radius: 50%; }
.reservation-dates > div:first-child dt > span { background: var(--confirmation-accent); }
.reservation-dates dt::after { content: ''; height: 1px; flex: 1; margin-left: 4px; background: var(--confirmation-line); }
.reservation-dates dd { margin-top: 10px; font-size: 16px; line-height: 1.7; font-variant-numeric: tabular-nums; }
.reservation-gift { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; margin-top: 22px; font-size: 12px; color: var(--confirmation-muted); }
.reservation-gift svg, .reservation-gift strong { color: var(--confirmation-accent); }
.reservation-gift strong { font-weight: 500; }
.confirmation-actions { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-top: 24px; }
.confirmation-actions .btn { gap: 18px; padding: 12px 20px; font-size: 13px; }
.confirmation-secondary { color: var(--confirmation-muted); font-size: 12px; padding: 10px 0; transition: color .2s; }
.confirmation-secondary:hover { color: var(--confirmation-ink); }
.confirmation-details { margin-top: 24px; padding-top: 16px; border-top: 1px solid var(--confirmation-line); color: var(--confirmation-muted); font-size: 11px; }
.confirmation-details summary { display: flex; align-items: center; justify-content: space-between; cursor: pointer; list-style: none; padding-block: 4px; }
.confirmation-details summary::-webkit-details-marker { display: none; }
.confirmation-details summary svg { transition: transform .2s; }
.confirmation-details[open] summary svg { transform: rotate(180deg); }
.confirmation-details dl { display: grid; gap: 12px; padding-top: 18px; }
.confirmation-details dl > div { display: flex; justify-content: space-between; gap: 20px; }
.confirmation-details dt { flex-shrink: 0; }
.confirmation-details dd { text-align: right; overflow-wrap: anywhere; min-width: 0; font-variant-numeric: tabular-nums; }
.order-number { font-family: ui-monospace, monospace; }
.confirmation-secondary:focus-visible, .confirmation-details summary:focus-visible { outline: 2px solid var(--confirmation-accent); outline-offset: 4px; border-radius: 3px; }
@keyframes confirmation-arrive { from { opacity: 0; transform: translateY(8px); } to { opacity: 1; transform: translateY(0); } }
@keyframes confirmation-check { from { stroke-dashoffset: 25; } to { stroke-dashoffset: 0; } }
@media (max-width: 480px) {
  .confirmation-heading { gap: 12px; }
  .confirmation-seal { flex-basis: 38px; width: 38px; }
  .confirmation-eyebrow { font-size: 15px; }
  .confirmation-reservation { margin-top: 24px; padding-block: 22px; }
  .reservation-summary { gap: 12px; flex-wrap: wrap; }
  .reservation-plan-name h3 { font-size: 23px; }
  .reservation-payment strong { font-size: 23px; }
  .reservation-dates { gap: 16px; }
  .reservation-dates dd { font-size: 14px; }
  .confirmation-actions { flex-direction: column-reverse; gap: 6px; }
  .confirmation-actions .btn { width: 100%; }
  .confirmation-details { margin-top: 12px; }
}
@media (prefers-reduced-motion: reduce) {
  .reservation-confirmation, .confirmation-check { animation: none; }
  .confirmation-details summary svg, .confirmation-secondary { transition: none; }
}
</style>
