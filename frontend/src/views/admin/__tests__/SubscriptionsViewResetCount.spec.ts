import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'

import SubscriptionsView from '../SubscriptionsView.vue'

const AppLayoutStub = defineComponent({
  name: 'AppLayout',
  setup: (_props, { slots }) => () => h('div', slots.default?.())
})

const { list, getAll, searchUsers, getPlans, setResetCount, bulkSetResetCount, showError, showSuccess } = vi.hoisted(() => ({
  list: vi.fn(),
  getAll: vi.fn(),
  searchUsers: vi.fn(),
  getPlans: vi.fn(),
  setResetCount: vi.fn(),
  bulkSetResetCount: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    subscriptions: {
      list,
      setResetCount,
      bulkSetResetCount,
      bulkResetQuota: vi.fn(),
      bulkShiftWindow: vi.fn(),
      getStats: vi.fn(),
      getUsageSeries: vi.fn(),
      assign: vi.fn(),
      extend: vi.fn(),
      resetQuota: vi.fn(),
      revoke: vi.fn()
    },
    groups: { getAll },
    payment: { getPlans },
    usage: { searchUsers }
  }
}))

vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess }) }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${Object.values(params).join(',')}` : key })
  }
})

describe('SubscriptionsView reset count controls', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    list.mockResolvedValue({
      items: [{
        id: 11,
        user_id: 5,
        group_id: 2,
        status: 'active',
        starts_at: '2026-01-01T00:00:00Z',
        expires_at: '2099-01-01T00:00:00Z',
        reset_count: 1,
        daily_usage_usd: 1,
        weekly_usage_usd: 2,
        monthly_usage_usd: 3,
        group: { id: 2, name: 'Weekly', platform: 'openai', weekly_limit_usd: 80 }
      }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    getAll.mockResolvedValue([])
    searchUsers.mockResolvedValue([])
    getPlans.mockResolvedValue({ data: [] })
    setResetCount.mockResolvedValue({ reset_count: 4, id: 11 })
    bulkSetResetCount.mockResolvedValue({ updated: 1, skipped: 0, count: 1 })
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: vi.fn().mockReturnValue({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })
    })
  })

  it('sets a single subscription count', async () => {
    const wrapper = mount(SubscriptionsView, {
      global: { stubs: { Icon: true, teleport: true, AppLayout: AppLayoutStub, RouterLink: true } }
    })
    await flushPromises()
    wrapper.vm.openResetCountDialog(wrapper.vm.subscriptions[0])
    wrapper.vm.resetCountForm.count = 4
    await wrapper.vm.confirmSetResetCount()
    expect(setResetCount).toHaveBeenCalledWith(11, { count: 4 })
    expect(showSuccess).toHaveBeenCalled()
  })

  it('sends selected IDs for bulk count updates', async () => {
    const wrapper = mount(SubscriptionsView, {
      global: { stubs: { Icon: true, teleport: true, AppLayout: AppLayoutStub, RouterLink: true } }
    })
    await flushPromises()
    wrapper.vm.selectResetCountRow(11)
    wrapper.vm.bulkResetCountForm.count = 3
    await wrapper.vm.confirmBulkSetResetCount()
    expect(bulkSetResetCount).toHaveBeenCalledWith({ count: 3, subscription_ids: [11] })
  })
})
