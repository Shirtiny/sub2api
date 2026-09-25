import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import AdminPresaleOfflineDialog from '../AdminPresaleOfflineDialog.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { AdminPaymentOrder } from '@/api/admin/payment'

vi.mock('vue-i18n', async () => { const { ref } = await import('vue'); return { useI18n: () => ({ t: (key: string) => key, locale: ref('zh') }) } })
enableAutoUnmount(afterEach)
const order: AdminPaymentOrder = { id: 3, user_id: 7, user_email: 'buyer@example.test', status: 'COMPLETED', order_type: 'subscription', amount: 100, pay_amount: 101, fee_rate: 1, payment_type: 'alipay', out_trade_no: 'order-3', created_at: '', expires_at: '', refund_amount: 0, paid_at: '2026-09-25T10:00:00Z', updated_at: '2026-09-25T10:01:00.123456Z', presale_starts_at: '2026-10-01T00:00:00+08:00', presale_expires_at: '2026-11-01T00:00:00+08:00', presale_plan_name: 'Small' }
function render(overrides: Partial<AdminPaymentOrder> = {}, currency = 'CNY') {
 return mount(AdminPresaleOfflineDialog, { props: { show: true, order: { ...order, ...overrides }, summary: { currency }, submitting: false }, global: { stubs: { BaseDialog: { props: ['show'], template: '<main v-if="show"><slot /><slot name="footer" /></main>' }, OrderStatusBadge: true } } })
}
async function confirm(w: ReturnType<typeof render>) {
 await w.get('[data-testid="offline-reason"]').setValue('  customer request  ')
 await w.get('[data-testid="offline-confirm"]').setValue(true)
}
describe('offline presale handling', () => {
 it('separates cancellation from refund and requires explicit confirmation', async () => {
  const w = render()
  expect(w.text()).toContain('buyer@example.test')
  expect(w.text()).toContain('2026/10/01')
  expect(w.find('[data-testid="offline-amount"]').exists()).toBe(false)
  expect(w.get('[data-testid="offline-submit"]').attributes('disabled')).toBeDefined()
  await confirm(w)
  expect(w.get('[data-testid="offline-submit"]').attributes('disabled')).toBeUndefined()
  await w.get('form').trigger('submit')
  expect(w.emitted('confirm')?.[0]?.[0]).toEqual({ mode: 'cancel', amount: 0, reference: '', reason: 'customer request', confirmed: true, expected_updated_at: order.updated_at })
 })
 it('requires actual money, reference, reason and currency precision for offline refunds', async () => {
  const w = render()
  await w.get('[data-testid="offline-mode"]').setValue('refund')
  await confirm(w)
  for (const value of ['', '0', '-1', '102', '10.001']) {
   await w.get('[data-testid="offline-amount"]').setValue(value)
   await w.get('form').trigger('submit')
  }
  expect(w.emitted('confirm')).toBeUndefined()
  await w.get('[data-testid="offline-amount"]').setValue('80.50')
  await w.get('form').trigger('submit')
  expect(w.emitted('confirm')).toBeUndefined()
  await w.get('[data-testid="offline-reference"]').setValue(' receipt-123 ')
  await w.get('form').trigger('submit')
  expect(w.emitted('confirm')?.[0]?.[0]).toMatchObject({ mode: 'refund', amount: 80.5, reference: 'receipt-123' })
 })
 it('uses whole amounts for JPY', async () => {
  const w = render({ status: 'PRESALE_CANCELLED' }, 'JPY')
  await confirm(w)
  await w.get('[data-testid="offline-reference"]').setValue('receipt')
  await w.get('[data-testid="offline-amount"]').setValue('10.5')
  expect(w.get('[data-testid="offline-amount"]').attributes('step')).toBe('1')
  await w.get('form').trigger('submit')
  expect(w.emitted('confirm')).toBeUndefined()
  await w.get('[data-testid="offline-amount"]').setValue('10')
  await w.get('form').trigger('submit')
  expect(w.emitted('confirm')?.[0]?.[0]).toMatchObject({ amount: 10 })
 })
 it.each(['PRESALE_CANCELLED', 'REFUND_REQUESTED'] as const)('does not cancel again for %s', status => {
  const w = render({ status })
  expect(w.findAll('option').map(o => o.attributes('value'))).toEqual(['refund'])
 })
 it('changing the action clears consent and never sends hidden refund fields when cancelling', async () => {
  const w = render()
  await w.get('[data-testid="offline-mode"]').setValue('refund')
  await w.get('[data-testid="offline-amount"]').setValue('80')
  await w.get('[data-testid="offline-reference"]').setValue('receipt')
  await confirm(w)
  await w.get('[data-testid="offline-mode"]').setValue('cancel')
  await w.get('form').trigger('submit')
  expect(w.emitted('confirm')).toBeUndefined()
  await w.get('[data-testid="offline-confirm"]').setValue(true)
  await w.get('form').trigger('submit')
  expect(w.emitted('confirm')?.[0]?.[0]).toMatchObject({ mode: 'cancel', amount: 0, reference: '' })
 })
 it('blocks duplicate submission and closing while submitting', async () => {
  const w = render(); await confirm(w); await w.setProps({ submitting: true })
  await w.get('form').trigger('submit')
  w.getComponent(BaseDialog).vm.$emit('close')
  expect(w.emitted('confirm')).toBeUndefined(); expect(w.emitted('cancel')).toBeUndefined()
 })
 it('requires a fresh server version and clears previous fields on reopening', async () => {
  const w = render({ updated_at: undefined }); await confirm(w)
  await w.get('form').trigger('submit'); expect(w.emitted('confirm')).toBeUndefined()
  await w.setProps({ show: false }); await w.setProps({ show: true, order: { ...order, id: 4 } })
  expect((w.get('[data-testid="offline-reason"]').element as HTMLTextAreaElement).value).toBe('')
  expect((w.get('[data-testid="offline-confirm"]').element as HTMLInputElement).checked).toBe(false)
 })
})
