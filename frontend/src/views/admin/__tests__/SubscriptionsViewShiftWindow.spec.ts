import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'

import SubscriptionsView from '../SubscriptionsView.vue'

const AppLayoutStub = defineComponent({
  name: 'AppLayout',
  setup: (_props, { slots }) => () => h('div', slots.default?.())
})

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  setup: (props, { slots }) => () =>
    props.show ? h('div', { 'data-test': 'base-dialog' }, [slots.default?.(), slots.footer?.()]) : null
})

const { bulkShiftWindow, list, getAll, searchUsers, showError, showSuccess } = vi.hoisted(() => ({
  bulkShiftWindow: vi.fn(),
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
      bulkShiftWindow,
      bulkResetQuota: vi.fn(),
      getStats: vi.fn(),
      getUsageSeries: vi.fn(),
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
    useI18n: () => ({
      // 回显 key 便于定位元素；带参数时把参数值拼在后面，让断言能看到插值。
      t: (key: string, params?: Record<string, unknown>) => {
        if (!params) return key
        return `${key}:${Object.values(params).join(',')}`
      }
    })
  }
})

const mountView = async () => {
  const wrapper = mount(SubscriptionsView, {
    global: {
      stubs: {
        Icon: true,
        teleport: true,
        Teleport: true,
        AppLayout: AppLayoutStub,
        BaseDialog: BaseDialogStub,
        SubscriptionStatsDialog: true,
        RouterLink: true
      }
    }
  })
  await flushPromises()
  return wrapper
}

type ViewWrapper = Awaited<ReturnType<typeof mountView>>

const openShiftDialog = async (wrapper: ViewWrapper) => {
  wrapper.vm.openShiftWindowDialog()
  await flushPromises()
}

const groupFixture = {
  id: 4,
  name: 'Latte (拿铁)',
  description: null,
  platform: 'openai',
  subscription_type: 'subscription',
  status: 'active',
  rate_multiplier: 1
}

describe('SubscriptionsView shift reset window', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: vi.fn().mockReturnValue({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })
    })
    vi.clearAllMocks()
    list.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    getAll.mockResolvedValue([groupFixture])
    searchUsers.mockResolvedValue([])
    bulkShiftWindow.mockResolvedValue({ matched: 105, updated: 103, skipped_future: 2, dry_run: false })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('does not preview or submit while the offset is still zero', async () => {
    const wrapper = await mountView()
    await openShiftDialog(wrapper)

    expect(bulkShiftWindow).not.toHaveBeenCalled()
    expect(wrapper.vm.canSubmitShiftWindow).toBe(false)

    await wrapper.vm.confirmShiftWindow()
    expect(bulkShiftWindow).not.toHaveBeenCalled()
  })

  it('blocks submitting when no window is selected', async () => {
    const wrapper = await mountView()
    await openShiftDialog(wrapper)

    wrapper.vm.shiftWindowForm.offset_hours = 14
    wrapper.vm.shiftWindowSelection.daily = false
    wrapper.vm.shiftWindowSelection.weekly = false
    wrapper.vm.shiftWindowSelection.monthly = false
    await flushPromises()

    expect(wrapper.vm.canSubmitShiftWindow).toBe(false)

    await wrapper.vm.confirmShiftWindow()
    expect(bulkShiftWindow).not.toHaveBeenCalled()
  })

  it('rejects an offset beyond the 720 hour bound', async () => {
    const wrapper = await mountView()
    await openShiftDialog(wrapper)

    wrapper.vm.shiftWindowForm.offset_hours = 721
    await flushPromises()

    expect(wrapper.vm.canSubmitShiftWindow).toBe(false)

    await wrapper.vm.confirmShiftWindow()
    expect(bulkShiftWindow).not.toHaveBeenCalled()
  })

  it('accepts a negative offset', async () => {
    const wrapper = await mountView()
    await openShiftDialog(wrapper)

    wrapper.vm.shiftWindowForm.offset_hours = -6
    await flushPromises()
    await wrapper.vm.confirmShiftWindow()

    expect(bulkShiftWindow).toHaveBeenCalledWith(
      expect.objectContaining({ offset_hours: -6, dry_run: false })
    )
  })

  it('previews with dry_run when the dialog opens with a valid offset', async () => {
    const wrapper = await mountView()
    wrapper.vm.shiftWindowForm.offset_hours = 14
    await flushPromises()

    await openShiftDialog(wrapper)

    expect(bulkShiftWindow).toHaveBeenCalledWith(
      expect.objectContaining({ dry_run: true, offset_hours: 14 })
    )
    expect(wrapper.vm.shiftPreview).toEqual({
      matched: 105,
      updated: 103,
      skipped_future: 2,
      dry_run: false
    })
  })

  it('debounces the preview when the offset changes', async () => {
    vi.useFakeTimers()
    const wrapper = mount(SubscriptionsView, {
      global: {
        stubs: {
          Icon: true,
          teleport: true,
          Teleport: true,
          AppLayout: AppLayoutStub,
          BaseDialog: BaseDialogStub,
          SubscriptionStatsDialog: true,
          RouterLink: true
        }
      }
    })
    await vi.runOnlyPendingTimersAsync()

    wrapper.vm.showShiftWindowDialog = true
    wrapper.vm.shiftWindowForm.offset_hours = 5
    wrapper.vm.scheduleShiftPreview()
    wrapper.vm.shiftWindowForm.offset_hours = 8
    wrapper.vm.scheduleShiftPreview()

    expect(bulkShiftWindow).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(300)

    const dryRunCalls = bulkShiftWindow.mock.calls.filter((call) => call[0].dry_run === true)
    expect(dryRunCalls).toHaveLength(1)
    expect(dryRunCalls[0][0]).toEqual(expect.objectContaining({ offset_hours: 8 }))
  })

  it('sends only the windows that are ticked', async () => {
    const wrapper = await mountView()
    await openShiftDialog(wrapper)

    wrapper.vm.shiftWindowForm.offset_hours = 14
    wrapper.vm.shiftWindowSelection.daily = true
    wrapper.vm.shiftWindowSelection.weekly = false
    wrapper.vm.shiftWindowSelection.monthly = true
    await flushPromises()

    await wrapper.vm.confirmShiftWindow()

    expect(bulkShiftWindow).toHaveBeenCalledWith(
      expect.objectContaining({ daily: true, weekly: false, monthly: true })
    )
  })

  it('mirrors the active page filters into the request scope', async () => {
    const wrapper = await mountView()
    await openShiftDialog(wrapper)

    wrapper.vm.shiftWindowForm.offset_hours = 14
    wrapper.vm.filters.status = 'active'
    wrapper.vm.filters.group_id = '4'
    wrapper.vm.filters.platform = 'openai'
    wrapper.vm.filters.user_id = 309
    await flushPromises()

    await wrapper.vm.confirmShiftWindow()

    expect(bulkShiftWindow).toHaveBeenCalledWith(
      expect.objectContaining({
        filters: { status: 'active', group_id: 4, platform: 'openai', user_id: 309 }
      })
    )
  })

  it('omits filter keys that are not set on the page', async () => {
    const wrapper = await mountView()
    await openShiftDialog(wrapper)

    wrapper.vm.shiftWindowForm.offset_hours = 14
    wrapper.vm.filters.status = 'active'
    wrapper.vm.filters.group_id = ''
    wrapper.vm.filters.platform = ''
    wrapper.vm.filters.user_id = null
    await flushPromises()

    await wrapper.vm.confirmShiftWindow()

    const submitCall = bulkShiftWindow.mock.calls.find((call) => call[0].dry_run === false)
    expect(submitCall?.[0].filters).toEqual({ status: 'active' })
  })

  it('reports the updated count and reloads the table after applying', async () => {
    const wrapper = await mountView()
    await openShiftDialog(wrapper)

    wrapper.vm.shiftWindowForm.offset_hours = 14
    await flushPromises()
    list.mockClear()

    await wrapper.vm.confirmShiftWindow()
    await flushPromises()

    expect(showSuccess).toHaveBeenCalledWith(
      expect.stringContaining('admin.subscriptions.shiftWindowSuccess')
    )
    expect(showSuccess.mock.calls[0][0]).toContain('103')
    expect(list).toHaveBeenCalled()
    expect(wrapper.vm.showShiftWindowDialog).toBe(false)
  })

  it('surfaces an error toast and keeps the dialog open when the request fails', async () => {
    bulkShiftWindow.mockRejectedValueOnce(new Error('boom'))
    const wrapper = await mountView()
    await openShiftDialog(wrapper)

    wrapper.vm.shiftWindowForm.offset_hours = 14
    await flushPromises()

    await wrapper.vm.confirmShiftWindow()
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.subscriptions.shiftWindowFailed')
    expect(wrapper.vm.showShiftWindowDialog).toBe(true)
  })
})

// 上面的用例直接读写内部状态，不会发现按钮/勾选框没渲染或 v-model 接错；
// 下面的用例只通过界面操作，覆盖控件到请求体之间的接线。
describe('SubscriptionsView shift reset window dialog wiring', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: vi.fn().mockReturnValue({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() })
    })
    vi.clearAllMocks()
    list.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    getAll.mockResolvedValue([groupFixture])
    searchUsers.mockResolvedValue([])
    bulkShiftWindow.mockResolvedValue({ matched: 105, updated: 103, skipped_future: 2, dry_run: false })
  })

  const shiftDialog = (wrapper: ViewWrapper) => {
    const dialog = wrapper.findAll('[data-test="base-dialog"]').at(0)
    if (!dialog) throw new Error('shift window dialog is not open')
    return dialog
  }

  it('renders one checkbox per window with only the weekly window ticked', async () => {
    const wrapper = await mountView()
    await openShiftDialog(wrapper)

    const checked = shiftDialog(wrapper)
      .findAll('input[type="checkbox"]')
      .map((box) => (box.element as HTMLInputElement).checked)

    expect(checked).toEqual([false, true, false])
  })

  it('keeps the submit button disabled until the offset is valid', async () => {
    const wrapper = await mountView()
    await openShiftDialog(wrapper)

    const submit = shiftDialog(wrapper)
      .findAll('button')
      .find((button) => button.text() === 'admin.subscriptions.shiftWindowSubmit')
    expect(submit?.attributes('disabled')).toBeDefined()

    await shiftDialog(wrapper).find('input[type="number"]').setValue(14)

    const enabled = shiftDialog(wrapper)
      .findAll('button')
      .find((button) => button.text() === 'admin.subscriptions.shiftWindowSubmit')
    expect(enabled?.attributes('disabled')).toBeUndefined()
  })

  it('renders the dry-run preview counts', async () => {
    const wrapper = await mountView()
    wrapper.vm.shiftWindowForm.offset_hours = 14
    await flushPromises()
    await openShiftDialog(wrapper)

    const preview = wrapper.find('[data-test="shift-preview"]')
    expect(preview.text()).toContain('105')
    expect(preview.text()).toContain('2')
  })

  it('submits from the footer button', async () => {
    const wrapper = await mountView()
    await openShiftDialog(wrapper)
    await shiftDialog(wrapper).find('input[type="number"]').setValue(14)

    const submit = shiftDialog(wrapper)
      .findAll('button')
      .find((button) => button.text() === 'admin.subscriptions.shiftWindowSubmit')
    await submit?.trigger('click')
    await flushPromises()

    expect(bulkShiftWindow).toHaveBeenCalledWith(
      expect.objectContaining({ dry_run: false, offset_hours: 14, weekly: true })
    )
  })
})
