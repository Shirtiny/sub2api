import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import AdminOrderDetail from '../AdminOrderDetail.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'
import type { AdminPaymentOrder, AdminOrderSummary, PaymentAuditLog } from '@/api/admin/payment'

const state = vi.hoisted(() => ({ locale: 'zh' }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ locale: { value: state.locale }, t: (key: string, args: Record<string, unknown> | string = {}) => {
  let value: unknown = state.locale === 'zh' ? zh : en
  for (const part of key.split('.')) value = value && typeof value === 'object' ? (value as Record<string, unknown>)[part] : undefined
  if (typeof value !== 'string') return typeof args === 'string' ? args : key
  return value.replace(/\{(\w+)\}/g, (_, name) => typeof args === 'object' ? String(args[name] ?? '') : '')
} }) }))
enableAutoUnmount(afterEach)
beforeEach(() => { state.locale = 'zh' })
const order: AdminPaymentOrder = {
  id: 9, user_id: 7, user_email: 'buyer@example.test', user_name: 'Buyer', amount: 200, pay_amount: 202, fee_rate: 1,
  payment_type: 'alipay', out_trade_no: 'order-9', payment_trade_no: 'trade-9', provider_key: 'alipay', provider_instance_id: '4',
  status: 'COMPLETED', order_type: 'subscription', created_at: '2026-09-24T12:00:00Z', expires_at: '2026-09-24T12:30:00Z', paid_at: '2026-09-24T12:01:00Z', refund_amount: 0,
  plan_id: 2, presale_plan_name: '原始小杯', presale_starts_at: '2099-09-30T16:00:00Z', presale_expires_at: '2099-10-31T16:00:00Z', presale_renewal: true,
  subscription_source_group_id: 3, subscription_group_id: 3, subscription_multiplier: 2, subscription_concurrency: 4, subscription_days: 31, subscription_source_price: 100,
  src_host: 'cafeshop.ai', src_url: 'https://cafeshop.ai/presale?token=never-show#secret', client_ip: '192.0.2.1'
}
const summary: AdminOrderSummary = { currency: 'CNY', plan_name: 'New name', plan_name_source: 'current', group_name: 'Astra', provider_name: '主渠道', payment_mode: 'redirect' }
function render(overrides: Partial<AdminPaymentOrder> = {}, meta: AdminOrderSummary | null = summary, auditLogs: PaymentAuditLog[] = []) {
  return mount(AdminOrderDetail, { props: { show: true, order: { ...order, ...overrides }, summary: meta, auditLogs }, global: { stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' } } } })
}

describe('AdminOrderDetail', () => {
  it('prioritizes snapshotted product, multiplier, customer and scheduled term', () => {
    const w = render()
    expect(w.get('[data-testid="product-title"]').text()).toBe('原始小杯× 2')
    expect(w.get('.order-summary').text()).toContain('订阅预售')
    expect(w.get('.order-summary').text()).toContain('续订')
    expect(w.get('.order-summary').text()).toContain('下单时名称')
    expect(w.get('.order-summary').text()).toContain('buyer@example.test')
    expect(w.get('.term-strip').text()).toContain('2099/10/01 00:00')
    expect(w.get('.term-strip').text()).toContain('2099/11/01 00:00')
    expect(w.get('.term-strip').text()).toContain('待生效')
    expect(w.get('[data-section="purchase"]').text()).toContain('31 天')
    expect(w.get('[data-section="purchase"]').text()).toContain('Astra · #3')
    expect(w.get('[data-section="purchase"]').text()).toContain('并发数4')
    expect(w.text()).not.toContain('New name')
  })
  it('separates request origin from provider and strips URL credentials, query and fragment', () => {
    const w = render({ src_url: 'https://user:password@cafeshop.ai/presale?token=never-show#secret' })
    const origin = w.get('[data-section="source"]')
    expect(origin.text()).toContain('预售落地页')
    expect(origin.text()).toContain('https://cafeshop.ai/presale')
    expect(origin.text()).toContain('192.0.2.1')
    expect(w.text()).not.toMatch(/password|never-show|#secret/)
    expect(w.get('[data-section="payment"]').text()).toContain('主渠道')
    expect(w.get('[data-section="payment"]').text()).toContain('trade-9')
  })
  it.each([undefined, '', 'javascript:alert(1)', 'bad url'])('does not invent a source for %s', src_url => {
    const w = render({ src_url })
    expect(w.get('[data-section="source"]').text()).not.toContain('预售落地页')
    expect(w.get('[data-section="source"]').text()).toContain('来源页面未记录')
  })
  it('does not infer the exact entry page from an origin-only referrer', () => {
    const w = render({ src_url: 'https://cafeshop.ai/' })
    expect(w.get('[data-section="source"]').text()).toContain('访问入口未记录')
    expect(w.get('[data-section="source"]').text()).toContain('https://cafeshop.ai/')
  })
  it('distinguishes payment deadline from subscription expiry and orders events chronologically', () => {
    const w = render({ completed_at: '2026-09-24T12:02:00Z' })
    expect(w.get('[data-section="payment"]').text()).toContain('支付截止')
    expect(w.get('[data-section="lifecycle"]').text()).not.toContain('支付截止')
    expect(w.findAll('[data-section="lifecycle"] time').map(event => event.attributes('datetime'))).toEqual([order.created_at, order.paid_at, '2026-09-24T12:02:00Z'])
  })
  it('identifies current legacy names and gracefully handles deleted plans', async () => {
    const w = render({ presale_plan_name: undefined, presale_starts_at: undefined })
    expect(w.get('[data-testid="product-title"]').text()).toContain('New name')
    expect(w.get('.order-summary').text()).toContain('当前套餐名称')
    expect(w.find('.term-strip').exists()).toBe(false)
    await w.setProps({ summary: { currency: 'CNY' } })
    expect(w.get('[data-testid="product-title"]').text()).toContain('套餐 #2')
    expect(w.text()).toContain('名称未记录或关联项已删除')
  })
  it('distinguishes USD balance credit from the recorded payment currency and unpaid amount', () => {
    const w = render({ order_type: 'balance', amount: 20, pay_amount: 15, currency: 'EUR', paid_at: undefined, status: 'PENDING', presale_starts_at: undefined, presale_plan_name: undefined }, { currency: 'EUR' })
    expect(w.get('[data-testid="product-title"]').text()).toBe('余额充值')
    expect(w.get('[data-section="purchase"]').text()).toContain('$20.00')
    expect(w.get('.summary-payment').text()).toContain('€15.00')
    expect(w.get('.summary-payment').text()).toContain('EUR')
    expect(w.get('.summary-payment').text()).toContain('应付金额')
    expect(w.get('[data-section="purchase"]').text()).not.toContain('套餐倍数')
  })
  it('retains failure, coupon, refund and audit information safely', () => {
    const w = render({ status: 'REFUND_REQUESTED', failed_reason: 'Review required', cafe_coupon_code: 'CAFE10', cafe_coupon_discount: 10, refund_amount: 150, refund_requested_by: 'user:7', refund_requested_at: '2026-09-25T00:00:00Z', refund_request_reason: '<img src=x onerror=alert(1)>', user_notes: '<script>bad()</script>' }, summary, [
      { id: 1, action: 'ORDER_CREATED', operator: 'user:7', created_at: order.created_at, detail: '{"paymentSource":"wechat_in_app_resume"}' },
      { id: 2, action: 'LEGACY_ACTION', created_at: order.created_at, detail: 'Legacy text' }
    ])
    expect(w.get('[data-section="payment"]').text()).toContain('CAFE10')
    expect(w.get('[data-section="source"]').text()).toContain('微信内继续支付')
    expect(w.get('[data-section="refund"]').text()).toContain('user:7')
    expect(w.text()).toContain('Review required')
    expect(w.get('[data-section="audit"]').text()).toContain('创建订单')
    expect(w.get('[data-section="audit"]').text()).toContain('LEGACY_ACTION')
    expect(w.find('img,script').exists()).toBe(false)
  })
  it('exposes loading, retry and close without adding payment actions', async () => {
    const w = render()
    await w.setProps({ loading: true })
    expect(w.get('.admin-order-detail').attributes('aria-busy')).toBe('true')
    await w.setProps({ loading: false, error: true })
    await w.get('[role="alert"] button').trigger('click')
    expect(w.emitted('reload')).toHaveLength(1)
    w.getComponent(BaseDialog).vm.$emit('close')
    expect(w.emitted('close')).toHaveLength(1)
    expect(w.findAll('button')).toHaveLength(1)
  })
  it('shows initial loading and retryable errors before any order is available', async () => {
    const w = render()
    await w.setProps({ order: null, loading: true })
    expect(w.get('[role="status"]').text()).toContain(zh.common.loading)
    expect(w.get('.admin-order-detail').attributes('aria-busy')).toBe('true')
    expect(w.find('.order-summary').exists()).toBe(false)
    await w.setProps({ loading: false, error: true })
    expect(w.get('[role="alert"]').text()).toContain(zh.adminOrderDetail.failedToLoad)
    await w.get('[role="alert"] button').trigger('click')
    expect(w.emitted('reload')).toHaveLength(1)
    await w.setProps({ loading: true })
    expect(w.get('[role="alert"] button').attributes('disabled')).toBeDefined()
    await w.setProps({ loading: false, error: false, order })
    expect(w.find('[role="alert"]').exists()).toBe(false)
    expect(w.get('.order-summary').text()).toContain(order.user_email)
  })
  it('matches English section semantics', () => {
    state.locale = 'en'
    const w = render()
    expect(w.text()).toContain('Subscription presale')
    expect(w.text()).toContain('Purchase origin')
    expect(w.text()).toContain('Name at purchase')
    expect(w.text()).toContain('Payment channel (current name)')
    expect(w.text()).not.toMatch(/adminOrderDetail\./)
  })
})


it('shows real offline money separately from the accounting amount and localizes cancellation', () => {
  const w = render({ status: 'PARTIALLY_REFUNDED', refund_amount: 80 }, summary, [{ id: 1, action: 'PRESALE_OFFLINE_REFUND', operator: 'admin:9', created_at: '', detail: JSON.stringify({ amount: 80.8, reference: 'receipt-xyz' }) }])
  const refunds = w.get('[data-section="refund"]').text()
  expect(refunds).toContain('线下实退金额')
  expect(refunds).toContain('80.80')
  expect(refunds).toContain('receipt-xyz')
  expect(render({ status: 'PRESALE_CANCELLED' }).text()).toContain('预订已取消')
})
