import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import MyPresales from '../MyPresales.vue'
const mocks = vi.hoisted(() => ({ mine: vi.fn(), quote: vi.fn(), refund: vi.fn(), refresh: vi.fn(), error: vi.fn(), success: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: { value: 'zh' } }) }))
vi.mock('@/api/presale', () => ({ presaleAPI: { mine: mocks.mine, refundQuote: mocks.quote } }))
vi.mock('@/api/payment', () => ({ paymentAPI: { requestRefund: mocks.refund } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.error, showSuccess: mocks.success }) }))
vi.mock('@/stores/subscriptions', () => ({ useSubscriptionStore: () => ({ fetchActiveSubscriptions: mocks.refresh }) }))
const order = {
  id: 7, status: 'COMPLETED', pay_amount: 100, currency: 'CNY', presale_plan_name: '小杯',
  presale_starts_at: '2026-10-01T00:00:00+08:00', presale_expires_at: '2026-11-01T00:00:00+08:00',
}
const render = () => mount(MyPresales, { global: { stubs: { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' }, BaseDialog: { props: ['show'], template: '<div v-if="show" role="dialog"><slot /><slot name="footer" /></div>' } } } })
beforeEach(() => {
  vi.clearAllMocks()
  vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-09-24T12:00:00Z'))
  mocks.mine.mockResolvedValue({ data: [order] })
  mocks.quote.mockResolvedValue({ data: { gateway_amount: 80, refund_amount: 80, currency: 'CNY', fee_percent: 20 } })
  mocks.refund.mockResolvedValue({}); mocks.refresh.mockResolvedValue(undefined)
})
afterEach(() => vi.restoreAllMocks())
describe('presale reservation cards', () => {
  it('prioritizes the plan, one status, and dates without repeating policy paragraphs', async () => {
    const w = render(); await flushPromises()
    expect(w.get('.reservation-card h3').text()).toBe('小杯')
    expect(w.findAll('.reservation-status')).toHaveLength(1)
    expect(w.get('.reservation-card').classes()).toContain('reservation-arriving')
    expect(w.get('.reservation-status').attributes('data-status')).toBe('pending')
    expect(w.get('.reservation-start').text()).toBe('2026.10.0100:00')
    expect(w.get('.reservation-end').text()).toBe('2026.11.0100:00')
    expect(w.get('.reservation-end').attributes('datetime')).toBe(order.presale_expires_at)
    expect(w.find('.reservation-multiplier').exists()).toBe(false)
    expect(w.text()).not.toContain('presale.myPresalesCopy')
    expect(w.text()).not.toContain('presale.termCopy')
    expect(w.get('a[href="/presale"]').text()).toBe('presale.reservation.browse')
    expect(w.get('.reservation-card a[href="/orders"]').text()).toBe('presale.viewOrders')
    w.unmount()
  })
  it('uses Beijing dates even when the API returns UTC, including across years', async () => {
    mocks.mine.mockResolvedValue({ data: [{ ...order, presale_starts_at: '2026-12-31T16:00:00Z', presale_expires_at: '2027-01-31T16:00:00Z' }] })
    const w = render(); await flushPromises()
    expect(w.get('.reservation-start').text()).toBe('2027.01.0100:00')
    expect(w.get('.reservation-end').text()).toBe('2027.02.0100:00')
    w.unmount()
  })
  it('omits timezone captions and preserves multiplier and renewal information', async () => {
    mocks.mine.mockResolvedValue({ data: [order, { ...order, id: 8, subscription_multiplier: 2, presale_renewal: true }] })
    const w = render(); await flushPromises()
    expect(w.findAll('.reservation-card')).toHaveLength(2)
    expect(w.find('.reservation-timezone').exists()).toBe(false)
    expect(w.text()).not.toContain('presale.reservation.timezone')
    expect(w.get('#presale-pending-content').classes()).toContain('grid')
    expect(w.get('.reservation-multiplier').text()).toBe('2×')
    expect(w.findAll('.reservation-card')[1].text()).toContain('presale.reservation.renewal')
    w.unmount()
  })
  it.each([
    [{ presale_activated_at: '2026-10-01T00:00:00+08:00' }, 'active', true],
    [{ status: 'REFUND_REQUESTED' }, 'refundPending', false],
    [{ status: 'REFUNDED' }, 'refunded', false],
    [{ status: 'FAILED', paid_at: '2026-09-24T00:00:00Z' }, 'activationIssue', true],
    [{ presale_activated_at: '2026-08-01T00:00:00Z', presale_expires_at: '2026-09-01T00:00:00Z' }, 'expired', true],
  ])('keeps lifecycle state %s accessible in the right section', async (overrides, status, hasRefundAction) => {
    mocks.mine.mockResolvedValue({ data: [{ ...order, ...overrides }] })
    const w = render(); await flushPromises()
    if (status !== 'activationIssue') {
      expect(w.find('[data-presale-section="pending"]').exists()).toBe(false)
      expect(w.find('.reservation-card').exists()).toBe(false)
      await w.get('[data-testid="presale-records-toggle"]').trigger('click')
    }
    expect(w.get('.reservation-status').attributes('data-status')).toBe(status)
    expect(w.get('.reservation-status').text()).toBe(`presale.${status}`)
    expect(w.get('.reservation-card').classes()).not.toContain('reservation-arriving')
    expect(w.find('article button').exists()).toBe(hasRefundAction)
    w.unmount()
  })
  it('does not invent dates when the API has missing or invalid values', async () => {
    mocks.mine.mockResolvedValue({ data: [{ ...order, presale_starts_at: undefined, presale_expires_at: 'invalid' }] })
    const w = render(); await flushPromises()
    expect(w.get('.reservation-start').text()).toBe('——')
    expect(w.get('.reservation-end').text()).toBe('——')
    w.unmount()
  })
  it('shows only unactivated reservations by default and keeps records out of the pending section', async () => {
    mocks.mine.mockResolvedValue({ data: [
      order,
      { ...order, id: 8, presale_activated_at: '2026-10-01T00:00:00+08:00' },
      { ...order, id: 9, status: 'REFUNDED' },
      { ...order, id: 10, status: 'REFUND_REQUESTED' },
      { ...order, id: 11, status: 'FAILED', paid_at: '2026-09-24T00:00:00Z' },
    ] })
    const w = render(); await flushPromises()
    const pending = w.get('[data-presale-section="pending"]')
    expect(pending.get('header').text().replace(/\s/g, '')).toBe('presale.reservation.pendingTitlepresale.reservation.pendingHintpresale.reservation.browse')
    expect(pending.findAll('.reservation-status').map(status => status.attributes('data-status'))).toEqual(['pending', 'activationIssue'])
    expect(w.findAll('.reservation-card')).toHaveLength(2)
    const toggle = w.get('[data-testid="presale-records-toggle"]')
    expect(toggle.text()).toBe('presale.reservation.records')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    await toggle.trigger('click')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    const records = w.get('[data-presale-section="records"]')
    expect(records.findAll('.reservation-status').map(status => status.attributes('data-status'))).toEqual(['active', 'refunded', 'refundPending'])
    // Activated orders still have their original cancellation/refund flow.
    await records.get('article button').trigger('click'); await flushPromises()
    expect(mocks.quote).toHaveBeenCalledWith(8)
    expect(w.find('[role=dialog]').exists()).toBe(true)
    expect(mocks.refund).not.toHaveBeenCalled()
    w.unmount()
  })
})
describe('presale refund review', () => {
  it.each(['preparation', 'unused_days'])('shows the fee deduction without a misleading preparation-period label for %s', async (policy) => {
    mocks.quote.mockResolvedValue({ data: { gateway_amount: 64, refund_amount: 64, currency: 'CNY', fee_percent: 20, policy } })
    const w = render(); await flushPromises()
    await w.get('article button').trigger('click'); await flushPromises()
    expect(w.get('[role=dialog]').text()).toContain('64.00')
    expect(w.get('[role=dialog]').text()).toContain('presale.refundFeeApplied')
    expect(w.get('[role=dialog]').text()).not.toContain('presale.prepareCopy')
    w.unmount()
  })
  it('does not show a fee deduction for a full refund', async () => {
    mocks.quote.mockResolvedValue({ data: { gateway_amount: 100, refund_amount: 100, currency: 'CNY', fee_percent: 0, policy: 'full' } })
    const w = render(); await flushPromises()
    await w.get('article button').trigger('click'); await flushPromises()
    expect(w.get('[role=dialog]').text()).not.toContain('presale.refundFeeApplied')
    w.unmount()
  })
  it('submits only after reviewing a quote and carries the confirmed amount', async () => {
    const w = render(); await flushPromises()
    await w.get('article button').trigger('click'); await flushPromises()
    expect(mocks.refund).not.toHaveBeenCalled(); expect(w.get('[role=dialog]').text()).toContain('80.00')
    expect(w.get('[role=dialog] .btn-primary').attributes('disabled')).toBeDefined()
    await w.get('#presale-refund-reason').setValue('  使用计划有变，暂时不需要订阅  ')
    expect(w.get('[role=dialog] .btn-primary').attributes('disabled')).toBeUndefined()
    await w.get('#presale-refund-form').trigger('submit'); await flushPromises()
    expect(mocks.refund).toHaveBeenCalledWith(7, { reason: '使用计划有变，暂时不需要订阅', expected_refund_amount: 80 })
    expect(w.emitted('refunded')).toHaveLength(1)
    expect(mocks.refresh).toHaveBeenCalledWith(true); w.unmount()
  })
  it.each(['', ' \n\t　', '退'.repeat(501), '🌙'.repeat(501)])('blocks blank or oversized reasons before submitting (%s)', async reason => {
    const w = render(); await flushPromises(); await w.get('article button').trigger('click'); await flushPromises()
    await w.get('#presale-refund-reason').setValue(reason)
    expect(w.get('[role=dialog] .btn-primary').attributes('disabled')).toBeDefined()
    await w.get('#presale-refund-form').trigger('submit'); await flushPromises()
    expect(mocks.refund).not.toHaveBeenCalled(); w.unmount()
  })
  it('allows 500 Unicode characters and clears the draft when another review is opened', async () => {
    const w = render(); await flushPromises(); await w.get('article button').trigger('click'); await flushPromises()
    await w.get('#presale-refund-reason').setValue('🌙'.repeat(500))
    expect(w.get('#presale-refund-reason-count').text()).toBe('500/500')
    expect(w.get('[role=dialog] .btn-primary').attributes('disabled')).toBeUndefined()
    await w.get('[role=dialog] .btn-secondary').trigger('click')
    await w.get('article button').trigger('click'); await flushPromises()
    expect((w.get('#presale-refund-reason').element as HTMLTextAreaElement).value).toBe('')
    expect(w.get('[role=dialog] .btn-primary').attributes('disabled')).toBeDefined(); w.unmount()
  })
  it.each([true, false])('shows coupon forfeiture only when the server quote says a coupon was applied (%s)', async couponApplied => {
    mocks.quote.mockResolvedValue({ data: { gateway_amount: 60, refund_amount: 100, currency: 'CNY', fee_percent: 0, coupon_applied: couponApplied } })
    const w = render(); await flushPromises(); await w.get('article button').trigger('click'); await flushPromises()
    expect(w.find('[data-test="refund-coupon-notice"]').exists()).toBe(couponApplied)
    if (couponApplied) expect(w.get('[data-test="refund-coupon-notice"]').text()).toBe('presale.refundCouponNotice')
    expect(w.get('[role=dialog]').text()).toContain('60.00'); w.unmount()
  })
  it('keeps the reason after an API failure and does not send a duplicate request', async () => {
    let reject!: (error: unknown) => void
    mocks.refund.mockReturnValue(new Promise((_, fail) => { reject = fail }))
    const w = render(); await flushPromises(); await w.get('article button').trigger('click'); await flushPromises()
    await w.get('#presale-refund-reason').setValue('稍后再订阅')
    await w.get('#presale-refund-form').trigger('submit')
    await w.get('#presale-refund-form').trigger('submit')
    expect(mocks.refund).toHaveBeenCalledTimes(1)
    expect(w.get('#presale-refund-reason').attributes('disabled')).toBeDefined()
    reject(new Error('network')); await flushPromises()
    expect(mocks.error).toHaveBeenCalled()
    expect((w.get('#presale-refund-reason').element as HTMLTextAreaElement).value).toBe('稍后再订阅')
    expect(w.get('[role=dialog] .btn-primary').attributes('disabled')).toBeUndefined(); w.unmount()
  })
  it('does not open a confirmation or send a refund when quoting is disallowed', async () => {
    mocks.quote.mockRejectedValue({ reason: 'PRESALE_REFUND_LAST_WEEK', message: 'Final week' })
    const w = render(); await flushPromises(); await w.get('article button').trigger('click'); await flushPromises()
    expect(mocks.error).toHaveBeenCalled(); expect(w.find('[role=dialog]').exists()).toBe(false); expect(mocks.refund).not.toHaveBeenCalled(); w.unmount()
  })
})
