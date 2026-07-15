import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'

import SubscriptionsView from '../SubscriptionsView.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

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

// 该视图挂了两个 ConfirmDialog（单个重置和批量重置），只有打开的那个会渲染内容。
const bulkResetDialog = (wrapper: Awaited<ReturnType<typeof mountView>>) => {
  const dialog = wrapper.findAllComponents(ConfirmDialog).find((item) => item.props('show'))
  if (!dialog) throw new Error('bulk reset dialog is not open')
  return dialog
}

// 顺序与 bulkResetWindows 一致：每日、每周、每月。
const windowCheckboxes = (wrapper: Awaited<ReturnType<typeof mountView>>) =>
  bulkResetDialog(wrapper).findAll('input[type="checkbox"]')

// t() 在测试里回显 key，所以按文案定位比按样式类稳。
const confirmButton = (wrapper: Awaited<ReturnType<typeof mountView>>) => {
  const button = bulkResetDialog(wrapper)
    .findAll('button')
    .find((item) => item.text() === 'admin.subscriptions.bulkResetQuota')
  if (!button) throw new Error('confirm button not found')
  return button
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

// 上面的用例直接读写内部状态，不会发现勾选框没渲染或 v-model 接错；
// 下面的用例只通过界面操作，覆盖勾选框到请求体之间的接线。
describe('SubscriptionsView bulk reset quota dialog wiring', () => {
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

  it('renders one checkbox per window with monthly unticked', async () => {
    const wrapper = await mountView()
    await openBulkResetDialog(wrapper)

    const checked = windowCheckboxes(wrapper).map(
      (box) => (box.element as HTMLInputElement).checked
    )

    expect(checked).toEqual([true, true, false])
  })

  it('sends the monthly window once its checkbox is ticked', async () => {
    const wrapper = await mountView()
    await openBulkResetDialog(wrapper)

    await windowCheckboxes(wrapper)[2].setValue(true)
    await confirmButton(wrapper).trigger('click')
    await flushPromises()

    expect(bulkResetQuota).toHaveBeenCalledWith({ daily: true, weekly: true, monthly: true })
  })

  it('drops a window once its checkbox is unticked', async () => {
    const wrapper = await mountView()
    await openBulkResetDialog(wrapper)

    await windowCheckboxes(wrapper)[0].setValue(false)
    await confirmButton(wrapper).trigger('click')
    await flushPromises()

    expect(bulkResetQuota).toHaveBeenCalledWith({ daily: false, weekly: true, monthly: false })
  })

  it('disables the confirm button and sends nothing when every window is unticked', async () => {
    const wrapper = await mountView()
    await openBulkResetDialog(wrapper)

    const boxes = windowCheckboxes(wrapper)
    await boxes[0].setValue(false)
    await boxes[1].setValue(false)

    expect(confirmButton(wrapper).attributes('disabled')).toBeDefined()

    await confirmButton(wrapper).trigger('click')
    await flushPromises()

    expect(bulkResetQuota).not.toHaveBeenCalled()
  })
})
