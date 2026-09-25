import type { PaymentOrder } from '@/types/payment'

export function matchesPresaleCheckout(order: PaymentOrder, planId: number, month: string | undefined): boolean {
  if (order.order_type !== 'subscription' || order.plan_id !== planId || !month || !/^\d{4}-\d{2}$/.test(month)) return false
  // Compare instants: the API may serialize the UTC+8 start in UTC instead.
  const startsAt = Date.parse(order.presale_starts_at || '')
  return Number.isFinite(startsAt) && startsAt === Date.parse(`${month}-01T00:00:00+08:00`)
}

export function formatPresaleDate(value: string | undefined, locale: string, monthOnly = false): string {
  if (!value) return '—'
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) return '—'
  return new Intl.DateTimeFormat(locale.startsWith('zh') ? 'zh-CN' : 'en-GB', {
    timeZone: 'Asia/Shanghai', year: 'numeric', month: monthOnly ? 'long' : '2-digit',
    ...(monthOnly ? {} : { day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' as const }),
  }).format(date)
}
export function presaleStatus(order: PaymentOrder, now = Date.now()): string {
  if (order.status === 'PRESALE_CANCELLED') return 'cancelled'
  if (['REFUNDED', 'PARTIALLY_REFUNDED'].includes(order.status)) return 'refunded'
  if (['REFUND_REQUESTED', 'REFUNDING', 'REFUND_FAILED'].includes(order.status)) return 'refundPending'
  if (!order.presale_activated_at && (order.status === 'FAILED' || (order.presale_starts_at && Date.parse(order.presale_starts_at) + 120000 <= now))) return 'activationIssue'
  if (order.presale_expires_at && Date.parse(order.presale_expires_at) <= now) return 'expired'
  return order.presale_activated_at ? 'active' : 'pending'
}
