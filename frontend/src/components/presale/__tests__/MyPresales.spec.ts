import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import MyPresales from '../MyPresales.vue'
const mocks = vi.hoisted(() => ({ mine: vi.fn(), quote: vi.fn(), refund: vi.fn(), refresh: vi.fn(), error: vi.fn(), success: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: { value: 'zh' } }) }))
vi.mock('@/api/presale', () => ({ presaleAPI: { mine: mocks.mine, refundQuote: mocks.quote } }))
vi.mock('@/api/payment', () => ({ paymentAPI: { requestRefund: mocks.refund } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.error, showSuccess: mocks.success }) }))
vi.mock('@/stores/subscriptions', () => ({ useSubscriptionStore: () => ({ fetchActiveSubscriptions: mocks.refresh }) }))
const order = { id: 7, status: 'COMPLETED', pay_amount: 100, currency: 'CNY', presale_starts_at: '2026-10-01T00:00:00+08:00' }
const render = () => mount(MyPresales, { global: { stubs: { PresaleOrderTerm: true, OrderStatusBadge: true, RouterLink: { template: '<a><slot /></a>' }, BaseDialog: { props: ['show'], template: '<div v-if="show" role="dialog"><slot /><slot name="footer" /></div>' } } } })
beforeEach(() => { vi.clearAllMocks(); mocks.mine.mockResolvedValue({ data: [order] }); mocks.quote.mockResolvedValue({ data: { gateway_amount: 80, refund_amount: 80, currency: 'CNY', fee_percent: 20 } }); mocks.refund.mockResolvedValue({}); mocks.refresh.mockResolvedValue(undefined) })
describe('presale refund review', () => {
  it('submits only after reviewing a quote and carries the confirmed amount', async () => {
    const w = render(); await flushPromises()
    await w.get('article button').trigger('click'); await flushPromises()
    expect(mocks.refund).not.toHaveBeenCalled(); expect(w.get('[role=dialog]').text()).toContain('80.00')
    await w.get('[role=dialog] .btn-primary').trigger('click'); await flushPromises()
    expect(mocks.refund).toHaveBeenCalledWith(7, { reason: 'presale.requestRefund', expected_refund_amount: 80 })
    expect(mocks.refresh).toHaveBeenCalledWith(true); w.unmount()
  })
  it('does not open a confirmation or send a refund when quoting is disallowed', async () => {
    mocks.quote.mockRejectedValue({ reason: 'PRESALE_REFUND_LAST_WEEK', message: 'Final week' })
    const w = render(); await flushPromises(); await w.get('article button').trigger('click'); await flushPromises()
    expect(mocks.error).toHaveBeenCalled(); expect(w.find('[role=dialog]').exists()).toBe(false); expect(mocks.refund).not.toHaveBeenCalled(); w.unmount()
  })
})
