import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import AdminCafeCampaigns from '../AdminCafeCampaigns.vue'
import AdminOrderDetail from '../AdminOrderDetail.vue'
import Pagination from '@/components/common/Pagination.vue'
import { cafeCampaignAPI } from '@/api/admin/cafeCampaigns'
import { adminPaymentAPI } from '@/api/admin/payment'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'
const mocks = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn() }))
vi.mock('@/api/admin/cafeCampaigns', () => ({ cafeCampaignAPI: { list: vi.fn(), create: vi.fn(), setEnabled: vi.fn(), uses: vi.fn() } }))
vi.mock('@/api/admin/payment', () => ({ adminPaymentAPI: { getOrder: vi.fn() } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: mocks.success, showError: mocks.error }) }))
vi.mock('@/i18n', () => ({ i18n: { global: { t: (key: string) => key } } }))
vi.mock('vue-i18n', async () => { const { ref } = await import('vue'); return { useI18n: () => ({ locale: ref('zh'), t: (key: string, args: Record<string, unknown> = {}) => {
 let value: unknown = zh
 for (const part of key.split('.')) value = value && typeof value === 'object' ? (value as Record<string, unknown>)[part] : undefined
 return typeof value === 'string' ? value.replace(/\{(\w+)\}/g, (_, n) => String(args[n] ?? '')) : key
 } }) } })
enableAutoUnmount(afterEach)
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals() })
const row = { id: 1, code: 'CAFE-PUBLIC-SEP40', name: 'September', discount_percent: 40, enabled: false, starts_at: '2026-09-24T16:00:00Z', expires_at: '2026-09-30T16:00:00Z', updated_at: '2026-09-25T00:00:00Z', created_at: '', created_by: 99 }
function render(realOrderDetail = false) {
 return mount(AdminCafeCampaigns, { global: { stubs: {
 BaseDialog: { props: ['show', 'title'], template: '<div v-if="show" :data-title="title"><slot /><slot name="footer" /></div>' },
 DataTable: { props: ['columns', 'data'], template: '<table><tr v-for="row in data" :key="row.id"><td v-for="col in columns" :key="col.key"><slot :name="`cell-${col.key}`" :row="row" :value="row[col.key]" /></td></tr></table>' },
 Pagination: true, OrderStatusBadge: true, AdminOrderDetail: !realOrderDetail, Icon: true, LoadingSpinner: true,
 } } })
}
function click(w: ReturnType<typeof render>, label: string) { const button = w.findAll('button').find(b => b.text() === label); expect(button).toBeDefined(); return button!.trigger('click') }
beforeEach(() => {
 vi.clearAllMocks(); vi.mocked(cafeCampaignAPI.list).mockResolvedValue({ data: { items: [row], total: 1 } } as never)
 vi.mocked(cafeCampaignAPI.create).mockResolvedValue({ data: row } as never)
 vi.mocked(cafeCampaignAPI.setEnabled).mockResolvedValue({ data: { ...row, enabled: true } } as never)
 vi.mocked(cafeCampaignAPI.uses).mockResolvedValue({ data: { items: [], total: 0 } } as never)
})
describe('shared café campaign administration', () => {
 it('insets campaign explanations and actions without breaking table scrolling', async () => {
  const w = render(); await flushPromises()
  expect(w.get('[data-test="campaign-toolbar"]').classes()).toEqual(expect.arrayContaining(['px-4', 'py-5', 'sm:px-6']))
  expect(w.get('[data-test="campaign-policy"]').classes()).toEqual(expect.arrayContaining(['px-4', 'py-4', 'sm:px-6', 'leading-relaxed']))
 })
 it('reserves an unbroken code prefix while allowing the suffix input to shrink', async () => {
  const w = render(); await flushPromises(); await click(w, '创建通用券')
  expect(w.get('[data-test="campaign-code-prefix"]').classes()).toEqual(expect.arrayContaining(['shrink-0', 'whitespace-nowrap']))
  expect(w.get('[data-test="campaign-code"]').classes()).toEqual(expect.arrayContaining(['min-w-0', 'flex-1', 'w-0']))
 })
 it.each([true, false])('shows exactly one result toast when copying on secure context=%s', async secure => {
  vi.useFakeTimers()
  const writeText = vi.fn().mockResolvedValue(undefined)
  const execCommand = vi.fn().mockReturnValue(true)
  vi.stubGlobal('isSecureContext', secure)
  vi.stubGlobal('navigator', { clipboard: { writeText } })
  Object.defineProperty(document, 'execCommand', { configurable: true, value: execCommand })
  try {
   const w = render(); await flushPromises(); await click(w, row.code); await flushPromises()
   expect(mocks.success).toHaveBeenCalledTimes(1)
   expect(mocks.success).toHaveBeenCalledWith(zh.cafeCampaign.copied)
   expect(mocks.error).not.toHaveBeenCalled()
   if (secure) {
    expect(writeText).toHaveBeenCalledTimes(1); expect(writeText).toHaveBeenCalledWith(row.code)
   } else {
    expect(execCommand).toHaveBeenCalledTimes(1); expect(execCommand).toHaveBeenCalledWith('copy')
   }
   vi.runOnlyPendingTimers()
  } finally { Reflect.deleteProperty(document, 'execCommand') }
 })
 it('does not announce success when copying fails', async () => {
  vi.stubGlobal('isSecureContext', false)
  Object.defineProperty(document, 'execCommand', { configurable: true, value: vi.fn().mockReturnValue(false) })
  try {
   const w = render(); await flushPromises(); await click(w, row.code); await flushPromises()
   expect(mocks.success).not.toHaveBeenCalled()
   expect(mocks.error).toHaveBeenCalledTimes(1); expect(mocks.error).toHaveBeenCalledWith('common.copyFailed')
  } finally { Reflect.deleteProperty(document, 'execCommand') }
 })
 it('preserves the constrained flex scrolling chain and fixed-size pagination', async () => {
  const w = render(); await flushPromises()
  expect(w.get('[data-test="campaign-layout"]').classes()).toEqual(expect.arrayContaining(['flex', 'flex-col', 'min-h-0', 'flex-1']))
  expect(w.getComponent(Pagination).classes()).toContain('shrink-0')
  expect(w.getComponent(Pagination).props()).toMatchObject({ pageSize: 20, showPageSizeSelector: false })
  w.getComponent(Pagination).vm.$emit('update:page', 2); await flushPromises()
  expect(cafeCampaignAPI.list).toHaveBeenLastCalledWith(2)
 })
 it('shows 40% off, once-per-account policy and accurate inclusive end-date boundary', async () => {
  const w = render(); await flushPromises()
  expect(w.text()).toContain('减免 40%'); expect(w.text()).toContain('2026/10/01 00:00'); expect(w.text()).toContain('截止时刻不含')
  expect(w.text()).toContain('每个账号最多成功使用一次'); expect(w.text()).toContain('不恢复次数')
 })
 it('creates paused codes with 40 percent off and explicit business dates, not browser UTC conversion', async () => {
  vi.useFakeTimers({ toFake: ['Date'] }); vi.setSystemTime(new Date('2026-09-25T00:00:00Z'))
  const w = render(); await flushPromises(); await click(w, '创建通用券')
  expect((w.get('[data-test="campaign-start"]').element as HTMLInputElement).value).toBe('2026-09-25')
  expect((w.get('[data-test="campaign-end"]').element as HTMLInputElement).value).toBe('2026-09-30')
  expect(w.text()).toContain('实际支付原价的 60%')
  await w.get('[data-test="campaign-name"]').setValue('September'); await w.get('[data-test="campaign-code"]').setValue('sep40')
  await w.get('form').trigger('submit'); await flushPromises()
  expect(cafeCampaignAPI.create).toHaveBeenCalledWith({ name: 'September', code: 'SEP40', discount_percent: 40, start_date: '2026-09-25', end_date: '2026-09-30', enabled: false })
 })
 it.each([0, -40, 100, 40.5])('blocks invalid discount %s before sending', async (discount) => {
  const w = render(); await flushPromises(); await click(w, '创建通用券')
  await w.get('[data-test="campaign-name"]').setValue('test'); await w.get('[data-test="campaign-code"]').setValue('SEP40'); await w.get('[data-test="campaign-discount"]').setValue(discount)
  await w.get('form').trigger('submit'); expect(cafeCampaignAPI.create).not.toHaveBeenCalled()
 })
 it.each(['券', '🌙'])('accepts 100 Unicode characters, not 100 bytes or UTF-16 units (%s)', async char => {
  const w = render(); await flushPromises(); await click(w, '创建通用券')
  await w.get('[data-test="campaign-code"]').setValue('UNICODE')
  await w.get('[data-test="campaign-name"]').setValue(char.repeat(101))
  expect(w.get('[data-test="campaign-name"]').attributes('aria-invalid')).toBe('true')
  expect(w.text()).toContain('101/100')
  await w.get('form').trigger('submit'); expect(cafeCampaignAPI.create).not.toHaveBeenCalled()
  await w.get('[data-test="campaign-name"]').setValue(char.repeat(100))
  expect(w.get('[data-test="campaign-name"]').attributes('aria-invalid')).toBeUndefined()
  await w.get('form').trigger('submit'); await flushPromises()
  expect(cafeCampaignAPI.create).toHaveBeenCalledWith(expect.objectContaining({ name: char.repeat(100) }))
 })
 it('requires confirmation and uses server version when enabling', async () => {
  const w = render(); await flushPromises(); await click(w, '启用')
  expect(cafeCampaignAPI.setEnabled).not.toHaveBeenCalled()
  await w.get('[data-test="campaign-toggle-confirm"]').trigger('click'); await flushPromises()
  expect(cafeCampaignAPI.setEnabled).toHaveBeenCalledWith(row, true)
 })
 it('does not hide stale-server failures or pretend the code was enabled', async () => {
  vi.mocked(cafeCampaignAPI.setEnabled).mockRejectedValue(new Error('stale'))
  const w = render(); await flushPromises(); await click(w, '启用'); await w.get('[data-test="campaign-toggle-confirm"]').trigger('click'); await flushPromises()
  expect(mocks.error).toHaveBeenCalled(); expect(mocks.success).not.toHaveBeenCalled()
 })
 it('opens the actual associated order through the shared administrator detail', async () => {
  vi.mocked(cafeCampaignAPI.uses).mockResolvedValue({ data: { items: [{ user_id: 7, user_email: 'buyer@example.test', order_id: 9, order_status: 'REFUNDED', used_at: '2026-09-25T01:00:00Z' }], total: 1 } } as never)
  vi.mocked(adminPaymentAPI.getOrder).mockResolvedValue({ data: { order: { id: 9 }, summary: { currency: 'CNY' }, auditLogs: [] } } as never)
  const w = render(); await flushPromises(); await click(w, '使用记录'); await flushPromises()
  expect(w.text()).toContain('已使用'); expect(w.text()).toContain('buyer@example.test')
  await click(w, '#9'); await flushPromises()
  expect(adminPaymentAPI.getOrder).toHaveBeenCalledWith(9)
  expect(w.getComponent(AdminOrderDetail).props('order')).toMatchObject({ id: 9 })
 })
 it('renders an actual error and retries when the initial associated-order request fails', async () => {
  vi.mocked(cafeCampaignAPI.uses).mockResolvedValue({ data: { items: [{ user_id: 7, user_email: 'buyer@example.test', order_id: 9, order_status: 'COMPLETED', used_at: '2026-09-25T01:00:00Z' }], total: 1 } } as never)
  vi.mocked(adminPaymentAPI.getOrder).mockRejectedValueOnce(new Error('network unavailable')).mockResolvedValueOnce({ data: { order: { id: 9, user_id: 7, user_email: 'buyer@example.test', order_type: 'balance', status: 'COMPLETED', payment_type: 'alipay', amount: 100, pay_amount: 60, fee_rate: 0, cafe_coupon_discount: 40 }, summary: { currency: 'CNY' }, auditLogs: [] } } as never)
  const w = render(true); await flushPromises(); await click(w, '使用记录'); await flushPromises(); await click(w, '#9'); await flushPromises()
  expect(w.get('[role="alert"]').text()).toContain(zh.adminOrderDetail.failedToLoad)
  expect(w.find('.order-summary').exists()).toBe(false)
  await w.get('[role="alert"] button').trigger('click'); await flushPromises()
  expect(adminPaymentAPI.getOrder).toHaveBeenCalledTimes(2)
  expect(w.find('[role="alert"]').exists()).toBe(false)
  expect(w.get('.order-summary').text()).toContain('buyer@example.test')
 })
 it('keeps Chinese and English campaign error keys aligned in all checkout namespaces', () => {
  expect(Object.keys(zh.cafeCampaign.errors)).toEqual(Object.keys(en.cafeCampaign.errors))
  for (const key of Object.keys(zh.cafeCampaign.errors) as (keyof typeof zh.cafeCampaign.errors)[]) {
   expect(zh.payment.cafeCoupon.errors[key]).toBe(zh.cafeCampaign.errors[key]); expect(en.presale.errors[key]).toBe(en.cafeCampaign.errors[key])
  }
 })
})
