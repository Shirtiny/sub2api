import { describe, expect, it } from 'vitest'
import { formatPresaleDate, matchesPresaleCheckout, presaleStatus } from '../presale'
import type { PaymentOrder } from '@/types/payment'
describe('presale dates', () => {
  it('matches only the server-confirmed plan and business month', () => {
    const order = { order_type: 'subscription', plan_id: 7, presale_starts_at: '2026-09-30T16:00:00Z' } as PaymentOrder
    expect(matchesPresaleCheckout(order, 7, '2026-10')).toBe(true)
    expect(matchesPresaleCheckout(order, 8, '2026-10')).toBe(false)
    expect(matchesPresaleCheckout(order, 7, '2026-09')).toBe(false)
    expect(matchesPresaleCheckout(order, 7, undefined)).toBe(false)
    expect(matchesPresaleCheckout({ ...order, order_type: 'balance' }, 7, '2026-10')).toBe(false)
    expect(matchesPresaleCheckout({ ...order, presale_starts_at: undefined }, 7, '2026-10')).toBe(false)
  })
  it('uses the business timezone, not the browser timezone', () => {
    expect(formatPresaleDate('2026-09-30T16:00:00Z', 'zh')).toContain('2026/10/01 00:00')
    expect(formatPresaleDate('invalid', 'zh')).toBe('—')
  })
  it('requires the persisted activation marker; never treats payment as activation', () => {
    const o = { status: 'COMPLETED', presale_starts_at: '2026-10-01T00:00:00+08:00', presale_expires_at: '2026-11-01T00:00:00+08:00' } as PaymentOrder
    const now = Date.parse('2026-10-02T00:00:00Z')
    expect(presaleStatus(o, Date.parse(o.presale_starts_at!) - 1000)).toBe('pending')
    expect(presaleStatus(o,now)).toBe('activationIssue')
    expect(presaleStatus({ ...o, presale_activated_at: o.presale_starts_at },now)).toBe('active')
    expect(presaleStatus({ ...o, status: 'PARTIALLY_REFUNDED' },now)).toBe('refunded')
    expect(presaleStatus({ ...o, status: 'REFUND_REQUESTED' },now)).toBe('refundPending')
  })
})
