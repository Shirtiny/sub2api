import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'
import PresalePurchaseSuccess from '../PresalePurchaseSuccess.vue'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'
import type { PaymentOrder } from '@/types/payment'

const currentLocale = ref('zh')
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    locale: currentLocale,
    t: (key: string) => {
      const messages = currentLocale.value === 'zh' ? zh : en
      return key.split('.').reduce<unknown>((value, part) => (value as Record<string, unknown>)?.[part], messages) ?? key
    },
  }),
}))

const order: PaymentOrder = {
  id: 42, user_id: 9, order_type: 'subscription', status: 'COMPLETED',
  amount: 139, pay_amount: 100.22, cafe_coupon_discount: 41.70, fee_rate: 3,
  currency: 'CNY', payment_type: 'alipay', out_trade_no: 'sub2_20260925_confirmation',
  created_at: '2026-09-25T00:00:00Z', expires_at: '2026-09-25T00:30:00Z', refund_amount: 0,
  presale_plan_name: '小杯', presale_starts_at: '2026-09-30T16:00:00Z', presale_expires_at: '2026-10-31T16:00:00Z',
}
const render = (overrides: Partial<PaymentOrder> = {}, locale = 'zh') => {
  currentLocale.value = locale
  return mount(PresalePurchaseSuccess, { props: { order: { ...order, ...overrides } } })
}
beforeEach(() => { vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-09-25T12:00:00Z')) })
afterEach(() => vi.restoreAllMocks())

describe('PresalePurchaseSuccess', () => {
  it('prioritizes the plan, actual payment and Beijing dates, with details collapsed', async () => {
    const w = render()
    expect(w.get('h2').text()).toBe('预售购买成功')
    expect(w.get('h3').text()).toBe('小杯')
    expect(w.get('[data-test="paid-amount"]').text()).toMatch(/[¥￥]100.22/)
    expect(w.get('.reservation-state').attributes('data-state')).toBe('pending')
    expect(w.findAll('time').map(time => time.text())).toEqual(['2026/10/01 00:00', '2026/11/01 00:00'])
    expect(w.get('details').attributes('open')).toBeUndefined()
    expect(w.get('details').text()).toContain(order.out_trade_no)
    expect(w.get('details').text()).toContain('41.70')
    expect(w.get('details').text()).toContain('2.92')
    expect(w.find('.reservation-multiplier').exists()).toBe(false)
    expect(w.find('[data-test="purchase-bonus"]').exists()).toBe(false)
    expect(w.text()).not.toContain('不含')
    expect(w.text()).not.toContain('UTC+8')
    await w.get('.btn-primary').trigger('click')
    expect(w.emitted('viewSubscriptions')).toHaveLength(1)
    await w.get('.confirmation-secondary').trigger('click')
    expect(w.emitted('done')).toHaveLength(1)
    w.unmount()
  })

  it('preserves renewal, multiplier and the actual extended expiry', () => {
    const w = render({ presale_renewal: true, subscription_multiplier: 3, presale_expires_at: '2026-11-03T16:00:00Z' })
    expect(w.text()).toContain('本次为续费预订')
    expect(w.get('.reservation-multiplier').text()).toBe('3×')
    expect(w.findAll('time')[1].text()).toBe('2026/11/04 00:00')
    w.unmount()
  })

  it.each([
    [{ presale_balance_bonus_currency: 'CNY' as const, presale_balance_bonus_face_amount: 30, presale_balance_bonus_amount: 4.2 }, '30.00'],
    [{ presale_balance_bonus_amount: 6 }, '$6.00 USD'],
  ])('shows only the snapshotted gift, without guessing its face value', (bonus, expected) => {
    const w = render(bonus)
    expect(w.get('[data-test="purchase-bonus"]').text()).toContain(expected)
    w.unmount()
  })

  it('does not mislabel ledger dollars as CNY when the face value is missing', () => {
    const w = render({ presale_balance_bonus_currency: 'CNY', presale_balance_bonus_amount: 4.2 })
    expect(w.find('[data-test="purchase-bonus"]').exists()).toBe(false)
    w.unmount()
  })

  it.each([
    [{ presale_activated_at: '2026-10-01T00:00:00+08:00' }, '2026-10-02T00:00:00Z', 'active', '订阅已生效'],
    [{ presale_activated_at: '2026-10-01T00:00:00+08:00' }, '2026-11-02T00:00:00Z', 'expired', '本期订阅已结束'],
    [{}, '2026-10-02T00:00:00Z', 'activationIssue', '尚未生效，请联系支持'],
  ])('does not promise future activation for status %s', (overrides, now, state, copy) => {
    vi.spyOn(Date, 'now').mockReturnValue(Date.parse(now))
    const w = render(overrides)
    expect(w.get('.reservation-state').attributes('data-state')).toBe(state)
    expect(w.get('.confirmation-intro').text()).toContain(copy)
    w.unmount()
  })

  it('supports English and server currency, with no fabricated missing dates', () => {
    const w = render({ currency: 'USD', presale_plan_name: undefined, presale_expires_at: undefined }, 'en')
    expect(w.get('h2').text()).toBe(en.presale.purchased)
    expect(w.get('h3').text()).toBe(en.presale.nav)
    expect(w.get('[data-test="paid-amount"]').text()).toBe('$100.22')
    expect(w.findAll('time')[0].text()).toBe('01/10/2026, 00:00')
    expect(w.findAll('time')[1].text()).toBe('—')
    w.unmount()
  })
})
