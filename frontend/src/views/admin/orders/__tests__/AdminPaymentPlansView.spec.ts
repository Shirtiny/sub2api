import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import AdminPaymentPlansView from '../AdminPaymentPlansView.vue'

const mocks = vi.hoisted(() => ({ list: vi.fn(), update: vi.fn() }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/api/admin/payment', () => ({ adminPaymentAPI: { getPlans: mocks.list, updatePlan: mocks.update } }))
vi.mock('@/api/admin', () => ({ default: { groups: { getAll: async () => [] } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))
enableAutoUnmount(afterEach)
beforeEach(() => {
  vi.clearAllMocks()
  mocks.list.mockResolvedValue({ data: [
    { id: 1, presale_enabled: true, for_sale: true, presale_visible: false },
    { id: 2, presale_enabled: true, for_sale: false, presale_visible: true },
    { id: 3, presale_enabled: false, for_sale: true, presale_visible: true }
  ] })
  mocks.update.mockResolvedValue({})
})

describe('admin presale publication status', () => {
  it('uses only presale and sale status, including after the sale toggle', async () => {
    const wrapper = mount(AdminPaymentPlansView, { global: { stubs: {
      AppLayout: { template: '<main><slot /></main>' },
      DataTable: { props: ['data'], template: '<div><div v-for="row in data" :key="row.id" class="plan-row"><slot name="cell-presale_enabled" :row="row" /><slot name="cell-for_sale" :row="row" :value="row.for_sale" /></div></div>' },
      PlanEditDialog: true, ActivityManagementDialog: true, ActivityRecordsDialog: true, ConfirmDialog: true, Icon: true
    } } })
    await flushPromises()
    const rows = wrapper.findAll('.plan-row')
    expect(rows.map(row => row.text())).toEqual(['presale.nav', 'presale.admin.unpublished', 'presale.admin.unpublished'])
    await rows[0].get('button').trigger('click'); await flushPromises()
    expect(mocks.update).toHaveBeenCalledWith(1, { for_sale: false })
    expect(rows[0].text()).toBe('presale.admin.unpublished')
    await rows[1].get('button').trigger('click'); await flushPromises()
    expect(mocks.update).toHaveBeenCalledWith(2, { for_sale: true })
    expect(rows[1].text()).toBe('presale.nav')
  })
})
