import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ActivityManagementDialog from '../ActivityManagementDialog.vue'
const mocks = vi.hoisted(() => ({ list: vi.fn(), create: vi.fn(), update: vi.fn(), error: vi.fn() }))
vi.mock('@/api/admin/payment', () => ({ adminPaymentAPI: { getActivities: mocks.list, createActivity: mocks.create, updateActivity: mocks.update } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.error, showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: { value: 'zh' } }) }))
beforeEach(() => { vi.clearAllMocks(); mocks.list.mockResolvedValue({ data: [] }); mocks.create.mockResolvedValue({ data: {} }) })
describe('promotion activity management', () => {
  it('configures fixed CNY gifts on presale plans with explicit Beijing dates', async () => {
    const plans = [{ id: 1, name: 'Presale', presale_enabled: true }, { id: 2, name: 'Legacy', presale_enabled: false }]
    const w = mount(ActivityManagementDialog, { props: { show: false, plans: plans as never }, global: { stubs: { BaseDialog: { template: '<div><slot/></div>' }, ConfirmDialog: true, Icon: true } } })
    await w.setProps({ show: true }); await flushPromises()
    await w.get('select').setValue('presale_balance')
    expect(w.text()).toContain('Presale'); expect(w.text()).not.toContain('Legacy')
    await w.get('input[maxlength="100"]').setValue('September gifts')
    const dates = w.findAll('input[type="datetime-local"]')
    await dates[0].setValue('2026-09-25T00:00'); await dates[1].setValue('2026-09-29T00:00')
    await w.get('input[type="checkbox"][value="1"]').setValue(true)
    await w.get('input[step="0.01"]').setValue('10.50')
    await w.get('form').trigger('submit'); await flushPromises()
    expect(mocks.create).toHaveBeenCalledWith(expect.objectContaining({ type: 'presale_balance', bonus_currency: 'CNY', starts_at: '2026-09-24T16:00:00.000Z', ends_at: '2026-09-28T16:00:00.000Z', max_uses_per_user: 1, plan_bonuses: [{ plan_id: 1, bonus_days: 0, bonus_balance: 10.5 }] }))
    expect(mocks.error).not.toHaveBeenCalled(); w.unmount()
  })
})
