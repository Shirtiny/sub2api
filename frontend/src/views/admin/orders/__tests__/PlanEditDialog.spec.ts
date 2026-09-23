import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import PlanEditDialog from '../PlanEditDialog.vue'
import type { SubscriptionPlan } from '@/types/payment'
const mocks = vi.hoisted(() => ({ create: vi.fn(), update: vi.fn(), error: vi.fn(), success: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/payment', () => ({ adminPaymentAPI: { createPlan: mocks.create, updatePlan: mocks.update } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.error, showSuccess: mocks.success }) }))
const plan = { id: 7, group_id: 3, name: 'Monthly', description: 'Monthly service', price: 100, validity_days: 30, validity_unit: 'days', concurrency: 2, for_sale: true, sort_order: 0, features: ['Native models'], presale_enabled: true, presale_visible: true, presale_badge: 'Next month', presale_reset_cards: 2 } satisfies SubscriptionPlan
const render = () => mount(PlanEditDialog, { props: { show: false, plan, groups: [] }, global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }, Select: true, Icon: true, GroupBadge: true, RouterLink: true } } })
beforeEach(() => { vi.clearAllMocks(); mocks.update.mockResolvedValue({ data: plan }) })
describe('presale admin configuration', () => {
  it('loads and persists presale fields while preserving regular plan fields', async () => {
    const w = render(); await w.setProps({ show: true }); await flushPromises()
    const checks = w.findAll('fieldset input[type=checkbox]')
    expect(checks).toHaveLength(2); expect((checks[0].element as HTMLInputElement).checked).toBe(true)
    await checks[1].setValue(false)
    await w.get('form').trigger('submit'); await flushPromises()
    expect(mocks.update).toHaveBeenCalledWith(7, expect.objectContaining({ presale_enabled: true, presale_visible: false, presale_badge: 'Next month', presale_reset_cards: 2, validity_days: 30, concurrency: 2, features: 'Native models' }))
    w.unmount()
  })
})
