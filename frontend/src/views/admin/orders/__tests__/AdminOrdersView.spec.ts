import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import AdminOrdersView from '../AdminOrdersView.vue'
import AdminOrderDetail from '@/components/admin/payment/AdminOrderDetail.vue'
import AdminPresaleOfflineDialog from '@/components/admin/payment/AdminPresaleOfflineDialog.vue'
import AdminRefundDialog from '@/components/admin/payment/AdminRefundDialog.vue'
import type { AdminPaymentOrder } from '@/api/admin/payment'
const mocks = vi.hoisted(() => ({ list: vi.fn(), detail: vi.fn(), refund: vi.fn(), offline: vi.fn(), error: vi.fn() }))
vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/payment', () => {
  const api = { getOrders: mocks.list, getOrder: mocks.detail, refundOrder: mocks.refund, processPresaleOffline: mocks.offline }
  return { adminPaymentAPI: api, default: api }
})
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.error, showSuccess: vi.fn() }) }))
enableAutoUnmount(afterEach)
const order = (id: number): AdminPaymentOrder => ({ id, user_id: id, order_type: 'subscription', status: 'COMPLETED', amount: 100, pay_amount: 101, fee_rate: 1, payment_type: 'alipay', out_trade_no: `order-${id}`, created_at: '', expires_at: '', refund_amount: 0 })
const response = (id: number) => ({ data: { order: { ...order(id), user_name: `Buyer ${id}` }, summary: { currency: 'USD', plan_name: `Plan ${id}` }, auditLogs: [{ id, action: 'ORDER_CREATED' }] } })
function deferred<T>() { let resolve!: (value: T) => void; let reject!: (reason: unknown) => void; const promise = new Promise<T>((a, b) => { resolve = a; reject = b }); return { promise, resolve, reject } }
function render() {
 return mount(AdminOrdersView, { global: { stubs: {
   AppLayout: { template: '<main><slot /></main>' },
   OrderTable: { props: ['orders'], template: '<div><div v-for="row in orders" :key="row.id" :data-order="row.id"><slot name="actions" :row="row" /></div></div>' },
   AdminOrderDetail: true, AdminRefundDialog: true, AdminPresaleOfflineDialog: true, Select: true, Pagination: true, Icon: true
 } } })
}
beforeEach(() => { vi.clearAllMocks(); mocks.list.mockResolvedValue({ data: { items: [order(1), order(2)], total: 2 } }); mocks.detail.mockImplementation((id: number) => Promise.resolve(response(id))); mocks.refund.mockResolvedValue({}) })

describe('administrator order detail loading', () => {
  it('loads typed detail, summary and audit data through the shared dialog', async () => {
    const w = render(); await flushPromises()
    await w.get('[data-order="1"] button').trigger('click'); await flushPromises()
    expect(mocks.detail).toHaveBeenCalledWith(1)
    expect(w.getComponent(AdminOrderDetail).props()).toMatchObject({ show: true, loading: false, error: false, order: { id: 1, user_name: 'Buyer 1' }, summary: { plan_name: 'Plan 1' }, auditLogs: [{ id: 1 }] })
  })
  it('ignores an earlier response after opening another order', async () => {
    const first = deferred<ReturnType<typeof response>>()
    mocks.detail.mockImplementation((id: number) => id === 1 ? first.promise : Promise.resolve(response(id)))
    const w = render(); await flushPromises()
    await w.get('[data-order="1"] button').trigger('click')
    expect(w.getComponent(AdminOrderDetail).props('loading')).toBe(true)
    await w.get('[data-order="2"] button').trigger('click'); await flushPromises()
    first.resolve(response(1)); await flushPromises()
    expect(w.getComponent(AdminOrderDetail).props('order')?.id).toBe(2)
    expect(w.getComponent(AdminOrderDetail).props('summary')?.plan_name).toBe('Plan 2')
  })
  it('ignores responses after closing', async () => {
    const pending = deferred<ReturnType<typeof response>>()
    mocks.detail.mockReturnValue(pending.promise)
    const w = render(); await flushPromises()
    await w.get('[data-order="1"] button').trigger('click')
    w.getComponent(AdminOrderDetail).vm.$emit('close'); await flushPromises()
    pending.resolve(response(1)); await flushPromises()
    expect(w.getComponent(AdminOrderDetail).props()).toMatchObject({ show: false, loading: false, summary: null })
    w.unmount()
  })
  it('ignores late failures for another order and after unmount', async () => {
    const first = deferred<ReturnType<typeof response>>()
    mocks.detail.mockImplementation((id: number) => id === 1 ? first.promise : Promise.resolve(response(id)))
    const w = render(); await flushPromises()
    await w.get('[data-order="1"] button').trigger('click')
    await w.get('[data-order="2"] button').trigger('click'); await flushPromises()
    first.reject(new Error('old failure')); await flushPromises()
    expect(w.getComponent(AdminOrderDetail).props()).toMatchObject({ order: { id: 2 }, error: false, loading: false })
    const next = deferred<ReturnType<typeof response>>()
    mocks.detail.mockReturnValue(next.promise)
    await w.get('[data-order="1"] button').trigger('click'); w.unmount()
    next.reject(new Error('after unmount')); await flushPromises()
    expect(mocks.error).not.toHaveBeenCalled()
  })
  it('rejects a mismatched detail response instead of showing the wrong purchase', async () => {
    mocks.detail.mockResolvedValue(response(2))
    const w = render(); await flushPromises()
    await w.get('[data-order="1"] button').trigger('click'); await flushPromises()
    expect(w.getComponent(AdminOrderDetail).props()).toMatchObject({ error: true, order: { id: 1 }, summary: null, auditLogs: [] })
  })
  it('offers retry instead of silently showing incomplete details', async () => {
    mocks.detail.mockRejectedValueOnce(new Error('offline'))
    const w = render(); await flushPromises()
    await w.get('[data-order="1"] button').trigger('click'); await flushPromises()
    expect(w.getComponent(AdminOrderDetail).props()).toMatchObject({ error: true, loading: false, summary: null })
    w.getComponent(AdminOrderDetail).vm.$emit('reload'); await flushPromises()
    expect(w.getComponent(AdminOrderDetail).props()).toMatchObject({ error: false, loading: false, summary: { plan_name: 'Plan 1' } })
  })
  it('keeps the refund target independent from a delayed order-detail request', async () => {
    const first = deferred<ReturnType<typeof response>>()
    mocks.detail.mockReturnValue(first.promise)
    const w = render(); await flushPromises()
    await w.get('[data-order="1"] button').trigger('click')
    const refundButton = w.findAll('[data-order="2"] button').find(button => button.text().includes('payment.admin.refund'))!
    await refundButton.trigger('click')
    first.resolve(response(1)); await flushPromises()
    expect(w.getComponent(AdminRefundDialog).props('order')?.id).toBe(2)
    w.getComponent(AdminRefundDialog).vm.$emit('confirm', { amount: 80, reason: 'test', deduct_balance: false, force: false }); await flushPromises()
    expect(mocks.refund).toHaveBeenCalledWith(2, expect.objectContaining({ amount: 80 }))
  })
})


describe('administrator offline presale actions', () => {
  const presale = { ...order(1), paid_at: '2026-09-25T10:00:00Z', updated_at: '2026-09-25T10:00:00.123456Z', presale_starts_at: '2026-10-01T00:00:00+08:00' }
  const request = { mode: 'cancel' as const, amount: 0, reference: '', reason: 'support', confirmed: true, expected_updated_at: presale.updated_at }
  beforeEach(() => {
    mocks.list.mockResolvedValue({ data: { items: [presale, order(2)], total: 2 } })
    mocks.detail.mockResolvedValue({ ...response(1), data: { ...response(1).data, order: presale } })
    mocks.offline.mockResolvedValue({ data: { status: 'PRESALE_CANCELLED', affiliate_pending: false } })
  })
  it('only offers offline handling for paid presales and loads fresh details', async () => {
    const w = render(); await flushPromises()
    expect(w.findAll('[data-testid="offline-open"]')).toHaveLength(1)
    await w.get('[data-testid="offline-open"]').trigger('click'); await flushPromises()
    expect(mocks.detail).toHaveBeenCalledWith(1)
    expect(w.getComponent(AdminPresaleOfflineDialog).props()).toMatchObject({ show: true, order: { id: 1, updated_at: presale.updated_at } })
    w.getComponent(AdminPresaleOfflineDialog).vm.$emit('confirm', request); await flushPromises()
    expect(mocks.offline).toHaveBeenCalledWith(1, request)
    expect(mocks.refund).not.toHaveBeenCalled()
    expect(w.getComponent(AdminPresaleOfflineDialog).props('show')).toBe(false)
  })
  it('rejects wrong or unversioned orders without opening a dangerous dialog', async () => {
    mocks.detail.mockResolvedValue(response(2))
    const w = render(); await flushPromises()
    await w.get('[data-testid="offline-open"]').trigger('click'); await flushPromises()
    expect(w.getComponent(AdminPresaleOfflineDialog).props('show')).toBe(false)
    expect(mocks.error).toHaveBeenCalled()
  })
  it('blocks duplicate submissions and retains the exact request for bookkeeping retries', async () => {
    const pending = deferred<{ data: { status: string; affiliate_pending: boolean } }>()
    mocks.offline.mockReturnValueOnce(pending.promise)
    const w = render(); await flushPromises()
    await w.get('[data-testid="offline-open"]').trigger('click'); await flushPromises()
    const dialog = w.getComponent(AdminPresaleOfflineDialog)
    dialog.vm.$emit('confirm', request); dialog.vm.$emit('confirm', request)
    await flushPromises(); expect(mocks.offline).toHaveBeenCalledTimes(1)
    pending.resolve({ data: { status: 'REFUNDED', affiliate_pending: true } }); await flushPromises()
    expect(dialog.props('retryOnly')).toBe(true)
    dialog.vm.$emit('confirm', { ...request, amount: 999 }); await flushPromises()
    expect(mocks.offline).toHaveBeenLastCalledWith(1, request)
  })
  it('closes a stale form on server rejection and requires fresh review', async () => {
    mocks.offline.mockRejectedValue(new Error('stale'))
    const w = render(); await flushPromises()
    await w.get('[data-testid="offline-open"]').trigger('click'); await flushPromises()
    w.getComponent(AdminPresaleOfflineDialog).vm.$emit('confirm', request); await flushPromises()
    expect(w.getComponent(AdminPresaleOfflineDialog).props('show')).toBe(false)
    expect(mocks.error).toHaveBeenCalled()
  })
})
