import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'

import SubscriptionsView from '../SubscriptionsView.vue'

const AppLayoutStub = defineComponent({
  name: 'AppLayout',
  setup: (_props, { slots }) => () => h('div', slots.default?.())
})

const { bulkResetQuota, list, getAll, searchUsers, showError, showSuccess } = vi.hoisted(() => ({
  bulkResetQuota: vi.fn(),
  list: vi.fn(),
  getAll: vi.fn(),
  searchUsers: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    subscriptions: {
      list,
      bulkResetQuota,
      assign: vi.fn(),
      extend: vi.fn(),
      resetQuota: vi.fn(),
      revoke: vi.fn()
    },
    groups: { getAll },
    usage: { searchUsers }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const mountView = async () => {
  const wrapper = mount(SubscriptionsView, {
    global: {
      stubs: { Icon: true, teleport: true, AppLayout: AppLayoutStub, RouterLink: true }
    }
  })
  await flushPromises()
  return wrapper
}

const openBulkResetDialog = async (wrapper: Awaited<ReturnType<typeof mountView>>) => {
  wrapper.vm.showBulkResetQuotaConfirm = true
  await flushPromises()
}

describe('SubscriptionsView bulk reset quota', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: vi.fn().mockReturnValue({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })
    })
    vi.clearAllMocks()
    list.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    getAll.mockResolvedValue([])
    searchUsers.mockResolvedValue([])
    bulkResetQuota.mockResolvedValue({ count: 3 })
  })

  it('excludes the monthly window by default', async () => {
    const wrapper = await mountView()
    await openBulkResetDialog(wrapper)

    await wrapper.vm.confirmBulkResetQuota()

    expect(bulkResetQuota).toHaveBeenCalledWith({ daily: true, weekly: true, monthly: false })
  })

  it('includes the monthly window once it is selected', async () => {
    const wrapper = await mountView()
    await openBulkResetDialog(wrapper)

    wrapper.vm.bulkResetSelection.monthly = true
    await wrapper.vm.confirmBulkResetQuota()

    expect(bulkResetQuota).toHaveBeenCalledWith({ daily: true, weekly: true, monthly: true })
  })

  it('sends only the selected window', async () => {
    const wrapper = await mountView()
    await openBulkResetDialog(wrapper)

    wrapper.vm.bulkResetSelection.daily = false
    wrapper.vm.bulkResetSelection.weekly = false
    wrapper.vm.bulkResetSelection.monthly = true
    await wrapper.vm.confirmBulkResetQuota()

    expect(bulkResetQuota).toHaveBeenCalledWith({ daily: false, weekly: false, monthly: true })
  })

  it('does not call the API when no window is selected', async () => {
    const wrapper = await mountView()
    await openBulkResetDialog(wrapper)

    wrapper.vm.bulkResetSelection.daily = false
    wrapper.vm.bulkResetSelection.weekly = false
    wrapper.vm.bulkResetSelection.monthly = false
    await wrapper.vm.confirmBulkResetQuota()

    expect(bulkResetQuota).not.toHaveBeenCalled()
  })
})
