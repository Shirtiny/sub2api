<template>
  <BaseDialog :show="show" :title="t('payment.admin.orderDetail')" width="extra-wide" @close="emit('close')">
    <div class="admin-order-detail space-y-5" :aria-busy="loading">
      <div v-if="loading" class="flex items-center gap-2 text-xs text-content-tertiary" role="status">
        <LoadingSpinner variant="steam" size="sm" color="current" decorative />{{ t('common.loading') }}
      </div>
      <div v-if="error" class="flex items-center justify-between gap-3 rounded-xl border border-status-warning/30 bg-status-warning/5 p-3 text-sm text-content-secondary" role="alert">
        {{ t('adminOrderDetail.failedToLoad') }}<button class="shrink-0 underline" :disabled="loading" @click="emit('reload')">{{ t('adminOrderDetail.retry') }}</button>
      </div>

      <template v-if="order">
        <section class="order-summary">
          <div class="min-w-0 flex-1">
            <div class="mb-3 flex flex-wrap items-center gap-2 text-xs text-content-secondary">
              <span class="rounded-md bg-primary-500/10 px-2 py-1 text-primary-700 dark:text-primary-300">{{ purchaseKind }}</span>
              <span v-if="order.presale_starts_at">{{ t(order.presale_renewal ? 'adminOrderDetail.renewal' : 'adminOrderDetail.newReservation') }}</span>
              <span class="font-mono">#{{ order.id }}</span>
            </div>
            <h2 class="break-words text-xl font-semibold tracking-tight text-content-primary sm:text-2xl" data-testid="product-title">
              {{ productName }}<span v-if="order.order_type === 'subscription' && (order.subscription_multiplier || 0) > 1" class="ml-2 text-lg text-content-secondary">× {{ order.subscription_multiplier }}</span>
            </h2>
            <p v-if="order.order_type === 'subscription'" class="mt-1.5 text-xs text-content-tertiary">{{ nameSourceLabel }}</p>
            <p class="mt-3 break-all text-sm text-content-secondary">{{ order.user_email || order.user_name || t('adminOrderDetail.notRecorded') }} · #{{ order.user_id }}</p>
          </div>
          <div class="summary-payment">
            <OrderStatusBadge :status="order.status" />
            <p class="mb-1 mt-4 text-xs text-content-tertiary">{{ t(order.paid_at ? 'adminOrderDetail.paid' : 'adminOrderDetail.payable') }}</p>
            <p class="text-2xl font-semibold tabular-nums text-content-primary">{{ money(order.pay_amount) }} <span class="text-xs font-normal text-content-tertiary">{{ currency }}</span></p>
            <p class="mt-2 text-xs text-content-secondary">{{ t(`payment.methods.${order.payment_type}`, order.payment_type) }}<span v-if="summary?.provider_name"> · {{ summary.provider_name }}</span></p>
          </div>
        </section>

        <div v-if="order.presale_starts_at" class="term-strip">
          <div class="flex flex-wrap items-center gap-2 text-sm font-medium text-primary-700 dark:text-primary-300">
            <Icon name="clock" size="sm" />{{ t(`presale.${presaleStatus(order)}`) }}
          </div>
          <div><p>{{ t('adminOrderDetail.starts') }}</p><strong>{{ presaleDate(order.presale_starts_at) }}</strong></div>
          <div><p>{{ t('adminOrderDetail.ends') }}</p><strong>{{ presaleDate(order.presale_expires_at) }}</strong></div>
        </div>

        <div class="detail-grid">
          <section v-for="section in sections" :key="section.key" class="detail-section" :data-section="section.key">
            <h3>{{ t(`adminOrderDetail.${section.key}`) }}</h3>
            <dl class="detail-fields">
              <div v-for="row in section.rows" :key="row.label" :class="{ 'field-wide': row.wide }">
                <dt>{{ row.label }}</dt><dd :class="{ 'font-mono text-xs': row.mono }">{{ row.value }}</dd>
              </div>
            </dl>
            <p v-if="section.key === 'source'" class="mt-4 text-xs leading-relaxed text-content-tertiary">{{ t('adminOrderDetail.sourceHint') }}</p>
          </section>
        </div>

        <section v-if="order.failed_reason" class="rounded-xl border border-status-error/25 bg-status-error/5 p-4">
          <h3 class="text-sm font-medium text-status-error">{{ t('adminOrderDetail.failureReason') }}</h3>
          <p class="mt-2 whitespace-pre-wrap break-all text-sm text-content-secondary">{{ order.failed_reason }}</p>
        </section>

        <section v-if="refundRows.length" class="detail-section" data-section="refund">
          <h3>{{ t('payment.admin.refundInfo') }}</h3>
          <dl class="detail-fields"><div v-for="row in refundRows" :key="row.label" :class="{ 'field-wide': row.wide }"><dt>{{ row.label }}</dt><dd>{{ row.value }}</dd></div></dl>
        </section>

        <section class="detail-section" data-section="lifecycle">
          <h3>{{ t('adminOrderDetail.lifecycle') }}</h3>
          <ol class="timeline-grid">
            <li v-for="event in timeline" :key="event.label"><span class="timeline-dot" /><p class="text-xs text-content-tertiary">{{ event.label }}</p><time class="mt-1.5 block text-sm tabular-nums text-content-secondary" :datetime="event.at">{{ date(event.at) }}</time></li>
          </ol>
        </section>

        <details class="detail-section" data-section="audit">
          <summary class="cursor-pointer text-sm font-medium text-content-primary">{{ t('adminOrderDetail.audit') }} <span class="ml-1 font-normal text-content-tertiary">{{ auditLogs.length }}</span></summary>
          <p v-if="!auditLogs.length" class="mt-4 text-sm text-content-tertiary">{{ t('adminOrderDetail.auditEmpty') }}</p>
          <ol v-else class="mt-4 max-h-80 space-y-3 overflow-y-auto">
            <li v-for="log in auditLogs" :key="log.id" class="rounded-lg bg-surface-secondary p-3">
              <div class="flex flex-wrap items-center justify-between gap-2 text-xs"><strong class="font-medium text-content-secondary">{{ t(`adminOrderDetail.actions.${log.action}`, log.action) }}</strong><time class="text-content-tertiary">{{ date(log.created_at) }}</time></div>
              <p v-if="log.operator" class="mt-1 text-xs text-content-tertiary">{{ t('payment.admin.operator') }} · {{ log.operator }}</p>
              <details v-if="log.detail" class="mt-2 text-xs text-content-tertiary"><summary class="cursor-pointer">{{ t('adminOrderDetail.rawDetail') }}</summary><pre class="mt-2 whitespace-pre-wrap break-all leading-relaxed">{{ auditDetail(log.detail) }}</pre></details>
            </li>
          </ol>
        </details>
      </template>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'
import { formatPaymentAmount, normalizePaymentCurrency } from '@/components/payment/currency'
import { formatPresaleDate, presaleStatus } from '@/utils/presale'
import type { AdminPaymentOrder, AdminOrderSummary, PaymentAuditLog } from '@/api/admin/payment'

const props = withDefaults(defineProps<{
  show: boolean
  order: AdminPaymentOrder | null
  summary?: AdminOrderSummary | null
  auditLogs?: PaymentAuditLog[]
  loading?: boolean
  error?: boolean
}>(), { summary: null, auditLogs: () => [], loading: false, error: false })
const emit = defineEmits<{ close: []; reload: [] }>()
const { t, locale } = useI18n()
const currency = computed(() => normalizePaymentCurrency(props.summary?.currency || props.order?.currency))
const money = (amount: number, unit = currency.value) => formatPaymentAmount(amount, unit, locale.value)
const presaleDate = (value?: string) => formatPresaleDate(value, locale.value)
const unknown = () => t('adminOrderDetail.notRecorded')
const purchaseKind = computed(() => t(`adminOrderDetail.${props.order?.presale_starts_at ? 'presale' : props.order?.order_type === 'subscription' ? 'subscription' : 'balance'}`))
const productName = computed(() => props.order?.order_type === 'balance'
  ? t('adminOrderDetail.balance')
  : props.order?.presale_plan_name?.trim() || props.summary?.plan_name || (props.order?.plan_id ? t('adminOrderDetail.planFallback', { id: props.order.plan_id }) : t('adminOrderDetail.subscription')))
const nameSourceLabel = computed(() => props.order?.presale_plan_name?.trim() || props.summary?.plan_name_source === 'snapshot'
  ? t('adminOrderDetail.snapshotName') : props.summary?.plan_name_source === 'current' ? t('adminOrderDetail.currentName') : t('adminOrderDetail.missingName'))

function date(value?: string): string {
  if (!value || !Number.isFinite(Date.parse(value))) return unknown()
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}
function referringPage(value?: string): URL | null {
  try {
    const parsed = new URL(value || '')
    return ['http:', 'https:'].includes(parsed.protocol) ? parsed : null
  } catch { return null }
}
interface DetailRow { label: string; value: string | number; wide?: boolean; mono?: boolean }
function row(key: string, value?: string | number | null, options: Pick<DetailRow, 'wide' | 'mono'> = {}): DetailRow {
  return { label: t(`adminOrderDetail.${key}`), value: value === undefined || value === null || value === '' ? unknown() : value, ...options }
}
const sections = computed(() => {
  const o = props.order
  if (!o) return []
  const s = props.summary
  const sourceGroup = o.subscription_source_group_id ?? o.subscription_group_id
  const purchase = o.order_type === 'subscription' ? [
    row('planId', o.plan_id),
    row('group', [s?.group_name, sourceGroup ? `#${sourceGroup}` : ''].filter(Boolean).join(' · ')),
    row('multiplier', o.subscription_multiplier != null ? `${o.subscription_multiplier}×` : undefined),
    row('concurrency', o.subscription_concurrency),
    row('duration', o.subscription_days != null ? t('adminOrderDetail.days', { count: o.subscription_days }) : undefined),
    ...(o.subscription_bonus_days ? [row('bonusDays', t('adminOrderDetail.days', { count: o.subscription_bonus_days }))] : []),
    ...(o.subscription_source_price != null ? [row('unitPrice', money(o.subscription_source_price))] : []),
    ...(o.subscription_group_id && o.subscription_group_id !== sourceGroup ? [row('deliveredGroup', o.subscription_group_id)] : []),
    ...(o.presale_starts_at ? [row('activated', o.presale_activated_at ? formatPresaleDate(o.presale_activated_at, locale.value) : t('adminOrderDetail.pendingActivation'))] : []),
    ...(o.presale_subscription_id ? [row('subscription', `#${o.presale_subscription_id}`)] : []),
    ...(o.subscription_early_reset_enabled ? [row('earlyReset', t('adminOrderDetail.resetDeduction', { count: o.subscription_early_reset_duration_days }))] : []),
    ...(o.presale_reset_cards ? [row('legacyResetCards', o.presale_reset_cards)] : []),
  ] : [row('creditedAmount', money(o.amount, 'USD'))]
  const referrer = referringPage(o.src_url)
  const path = referrer?.pathname.replace(/\/$/, '')
  const entry = !referrer || !path ? unknown() : t(`adminOrderDetail.${path === '/presale' ? 'presaleEntry' : path === '/purchase' ? 'balanceEntry' : 'otherEntry'}`)
  const paymentSource = recordedPaymentSource()
  return [
    { key: 'purchase', rows: purchase },
    { key: 'source', rows: [row('entry', entry), row('clientIp', o.client_ip, { mono: true }), row('sourceHost', o.src_host, { wide: true }), row('sourcePage', referrer ? referrer.origin + referrer.pathname : undefined, { wide: true, mono: true }), ...(paymentSource ? [row('paymentSource', t(`adminOrderDetail.sources.${paymentSource}`, paymentSource), { wide: true })] : [])] },
    { key: 'buyer', rows: [row('userId', `#${o.user_id}`), row('userName', o.user_name), row('email', o.user_email, { wide: true }), ...(o.user_notes ? [row('notes', o.user_notes, { wide: true })] : [])] },
    { key: 'payment', rows: [
      row(o.order_type === 'balance' ? 'creditedAmount' : 'orderAmount', money(o.amount, o.order_type === 'balance' ? 'USD' : currency.value)),
      row(o.paid_at ? 'paid' : 'payable', money(o.pay_amount)),
      row('method', t(`payment.methods.${o.payment_type}`, o.payment_type)),
      row('feeRate', `${o.fee_rate}%`),
      row('paymentExpires', date(o.expires_at)), row('currency', currency.value),
      row('provider', s?.provider_name), row('providerKey', o.provider_key), row('providerId', o.provider_instance_id),
      row('mode', s?.payment_mode ? t(`adminOrderDetail.modes.${s.payment_mode}`, s.payment_mode) : undefined),
      ...(o.cafe_coupon_code ? [row('coupon', o.cafe_coupon_code)] : []),
      ...(o.cafe_coupon_discount ? [row('discount', money(o.cafe_coupon_discount))] : []),
      row('orderNo', o.out_trade_no, { wide: true, mono: true }), row('tradeNo', o.payment_trade_no, { wide: true, mono: true }),
    ] },
  ]
})
const timeline = computed(() => {
  const o = props.order
  if (!o) return []
  return [
    { label: t('adminOrderDetail.created'), at: o.created_at },
    { label: t('adminOrderDetail.paidAt'), at: o.paid_at },
    { label: t('adminOrderDetail.completed'), at: o.completed_at },
    { label: t('adminOrderDetail.failed'), at: o.failed_at },
    { label: t('adminOrderDetail.refundAt'), at: o.refund_at },
  ].filter((event): event is { label: string; at: string } => !!event.at)
    .sort((a, b) => Date.parse(a.at) - Date.parse(b.at))
})
const refundRows = computed(() => {
  const o = props.order
  if (!o) return []
  const rows: DetailRow[] = []
  for (const log of props.auditLogs) {
    if (log.action !== 'PRESALE_OFFLINE_REFUND' || !log.detail) continue
    try {
      const detail: unknown = JSON.parse(log.detail)
      if (!detail || typeof detail !== 'object') continue
      if ('amount' in detail && typeof detail.amount === 'number' && Number.isFinite(detail.amount)) rows.push(row('offlineAmount', money(detail.amount, currency.value)))
      if ('reference' in detail && typeof detail.reference === 'string') rows.push(row('offlineReference', detail.reference))
    } catch { /* Ignore malformed legacy audit data. */ }
  }
  if (o.refund_amount) rows.push(row('refundLedger', money(o.refund_amount, o.order_type === 'balance' ? 'USD' : currency.value)))
  if (o.refund_requested_at) rows.push({ label: t('payment.admin.refundRequestedAt'), value: date(o.refund_requested_at) })
  if (o.refund_requested_by) rows.push({ label: t('payment.admin.refundRequestedBy'), value: o.refund_requested_by })
  if (o.refund_request_reason) rows.push({ label: t('payment.admin.refundRequestReason'), value: o.refund_request_reason, wide: true })
  if (o.refund_reason) rows.push({ label: t('payment.admin.refundReason'), value: o.refund_reason, wide: true })
  return rows
})
function auditDetail(value: string): string {
  try { return JSON.stringify(JSON.parse(value), null, 2) } catch { return value }
}
function recordedPaymentSource(): string | undefined {
  for (const log of props.auditLogs) {
    if (log.action !== 'ORDER_CREATED' || !log.detail) continue
    try {
      const detail: unknown = JSON.parse(log.detail)
      if (detail && typeof detail === 'object' && 'paymentSource' in detail && typeof detail.paymentSource === 'string') return detail.paymentSource
    } catch { /* Legacy free-text audit details are not purchase-source evidence. */ }
  }
  return undefined
}
</script>

<style scoped>
.order-summary { display: flex; flex-wrap: wrap; gap: 24px; padding: 22px; border: 1px solid rgb(var(--color-stroke-brand) / .5); border-radius: 16px; background: linear-gradient(125deg, rgb(var(--color-primary-500) / .09), transparent); }
.summary-payment { flex-shrink: 0; text-align: right; }
.term-strip { display: grid; grid-template-columns: auto 1fr 1fr; align-items: center; gap: 24px; border-block: 1px solid rgb(var(--color-stroke-default)); padding: 18px 4px; }
.term-strip p { font-size: 11px; color: rgb(var(--color-content-tertiary)); margin-bottom: 6px; }
.term-strip strong { font-size: 13px; font-weight: 500; color: rgb(var(--color-content-primary)); font-variant-numeric: tabular-nums; }
.detail-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.detail-section { min-width: 0; border: 1px solid rgb(var(--color-stroke-default)); border-radius: 12px; padding: 18px; }
.detail-section h3 { font-size: 13px; font-weight: 600; color: rgb(var(--color-content-primary)); margin-bottom: 18px; }
.detail-fields { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 18px 16px; }
.detail-fields .field-wide { grid-column: 1 / -1; }
.detail-fields dt { font-size: 11px; color: rgb(var(--color-content-tertiary)); margin-bottom: 6px; }
.detail-fields dd { font-size: 13px; color: rgb(var(--color-content-secondary)); overflow-wrap: anywhere; white-space: pre-wrap; }
.timeline-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 20px; }
.timeline-grid li { position: relative; padding-left: 15px; }
.timeline-dot { position: absolute; left: 0; top: 5px; width: 5px; height: 5px; border-radius: 50%; background: rgb(var(--color-primary-text-500)); }
@media (max-width: 640px) {
  .order-summary { padding: 18px; gap: 18px; }
  .summary-payment { width: 100%; text-align: left; border-top: 1px solid rgb(var(--color-stroke-default)); padding-top: 16px; }
  .term-strip { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
  .term-strip > div:first-child { grid-column: 1 / -1; }
  .detail-grid { grid-template-columns: 1fr; }
  .timeline-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
</style>
