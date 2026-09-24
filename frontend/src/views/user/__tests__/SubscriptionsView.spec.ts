import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SubscriptionsView from '../SubscriptionsView.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import MyPresales from '@/components/presale/MyPresales.vue'
import { formatDateOnly } from '@/utils/format'

const routerPush = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())
const showSuccess = vi.hoisted(() => vi.fn())
const getMySubscriptions = vi.hoisted(() => vi.fn())
const earlyResetSubscription = vi.hoisted(() => vi.fn())
const resetSubscriptionQuota = vi.hoisted(() => vi.fn())
const invalidateCache = vi.hoisted(() => vi.fn())
const syncActiveSubscription = vi.hoisted(() => vi.fn())

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: routerPush }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (key === 'userSubscriptions.earlyResetErrors.EARLY_RESET_WOULD_EXPIRE') {
          return '本次提前重置会使订阅立即过期，如需放弃套餐请联系客服'
        }
        return params?.days ? `${params.days} days` : key
      },
    }),
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('@/stores/subscriptions', () => ({
  useSubscriptionStore: () => ({ invalidateCache, syncActiveSubscription }),
}))

vi.mock('@/api/subscriptions', () => ({
  default: {
    getMySubscriptions,
    earlyResetSubscription,
    resetSubscriptionQuota,
    resetQuota: resetSubscriptionQuota,
  },
}))

describe('SubscriptionsView renewal routing', () => {
  beforeEach(() => {
    routerPush.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    getMySubscriptions.mockReset()
    earlyResetSubscription.mockReset()
    resetSubscriptionQuota.mockReset()
    invalidateCache.mockReset()
    syncActiveSubscription.mockReset()
  })

  it('shows the multiplier on custom subscription cards', async () => {
    getMySubscriptions.mockResolvedValue([
      {
        id: 44,
        group_id: 99,
        status: 'active',
        expires_at: '2099-01-01T00:00:00Z',
        custom_expires_at: '2098-12-15T00:00:00Z',
        custom_multiplier: 3,
        custom_source_plan_id: 7,
        daily_usage_usd: 0,
        weekly_usage_usd: 0,
        monthly_usage_usd: 0,
        group: {
          id: 99,
          name: 'Starter-custom-user',
          platform: 'openai',
          description: '',
          is_custom_subscription_group: true,
          custom_source_plan_id: 7,
          custom_source_group_id: 3,
          custom_multiplier: 3,
        },
      },
    ])

    const wrapper = shallowMount(SubscriptionsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
        },
      },
    })
    await flushPromises()

    const multiplier = wrapper.find('[data-testid="subscription-custom-multiplier"]')
    expect(multiplier.exists()).toBe(true)
    expect(multiplier.text()).toContain('3x')
    expect(wrapper.text()).toContain(formatDateOnly('2098-12-15T00:00:00Z'))
  })

  it('shows the multiplier badge with the custom group suffix name', async () => {
    getMySubscriptions.mockResolvedValue([
      {
        id: 46,
        user_id: 7,
        group_id: 100,
        status: 'active',
        expires_at: '2099-01-01T00:00:00Z',
        daily_usage_usd: 0,
        weekly_usage_usd: 0,
        monthly_usage_usd: 0,
        group: {
          id: 100,
          name: 'Starter-3x',
          platform: 'openai',
          description: '',
          is_custom_subscription_group: true,
          custom_source_plan_id: 7,
          custom_source_group_id: 3,
          custom_multiplier: 3,
        },
      },
    ])

    const wrapper = shallowMount(SubscriptionsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
        },
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('Starter')
    expect(wrapper.text()).toContain('Starter-3x')
    expect(wrapper.find('[data-testid="subscription-custom-multiplier"]').text()).toContain('3x')
  })

  it('does not show a multiplier on normal subscription cards', async () => {
    getMySubscriptions.mockResolvedValue([
      {
        id: 45,
        group_id: 3,
        status: 'active',
        expires_at: '2099-01-01T00:00:00Z',
        daily_usage_usd: 0,
        weekly_usage_usd: 0,
        monthly_usage_usd: 0,
        group: {
          id: 3,
          name: 'Starter',
          platform: 'openai',
          description: '',
          is_custom_subscription_group: false,
          custom_multiplier: 3,
        },
      },
    ])

    const wrapper = shallowMount(SubscriptionsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
        },
      },
    })
    await flushPromises()

    expect(wrapper.find('[data-testid="subscription-custom-multiplier"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('3x')
  })

  it('routes custom subscription renewals by source plan and multiplier', async () => {
    getMySubscriptions.mockResolvedValue([
      {
        id: 1,
        group_id: 99,
        status: 'active',
        expires_at: '2099-01-01T00:00:00Z',
        daily_usage_usd: 0,
        weekly_usage_usd: 0,
        monthly_usage_usd: 0,
        group: {
          id: 99,
          name: 'Custom Pro-user',
          platform: 'openai',
          description: '',
          is_custom_subscription_group: true,
          custom_source_plan_id: 7,
          custom_source_group_id: 3,
          custom_multiplier: 4,
        },
      },
    ])

    const wrapper = shallowMount(SubscriptionsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
        },
      },
    })
    await flushPromises()

    await wrapper.find('[data-testid="subscription-renew"]').trigger('click')

    expect(routerPush).toHaveBeenCalledWith({
      path: '/purchase',
      query: {
        tab: 'subscription',
        plan: '7',
        group: '3',
        multiplier: '4',
      },
    })
  })

  it('confirms an enabled early reset and refreshes the card state', async () => {
    getMySubscriptions.mockResolvedValue([
      {
        id: 8,
        user_id: 7,
        group_id: 3,
        status: 'active',
        expires_at: '2099-01-20T00:00:00Z',
        early_reset_enabled: true,
        early_reset_duration_days: 5,
        daily_usage_usd: 10,
        weekly_usage_usd: 20,
        monthly_usage_usd: 30,
        group: {
          id: 3,
          name: 'Starter',
          platform: 'openai',
          description: '',
        },
      },
    ])
    earlyResetSubscription.mockResolvedValue({
      id: 8,
      user_id: 7,
      group_id: 3,
      status: 'active',
      expires_at: '2099-01-15T00:00:00Z',
      early_reset_enabled: true,
      early_reset_duration_days: 5,
      daily_usage_usd: 0,
      weekly_usage_usd: 0,
      monthly_usage_usd: 0,
      group: {
        id: 3,
        name: 'Starter',
        platform: 'openai',
        description: '',
      },
    })

    const wrapper = shallowMount(SubscriptionsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
        },
      },
    })
    await flushPromises()

    await wrapper.find('[data-testid="subscription-early-reset"]').trigger('click')
    const dialog = wrapper.findComponent(ConfirmDialog)
    expect(dialog.props('show')).toBe(true)
    dialog.vm.$emit('confirm')
    await flushPromises()

    expect(earlyResetSubscription).toHaveBeenCalledWith(8, expect.any(String))
    expect(showSuccess).toHaveBeenCalled()
    expect(syncActiveSubscription).toHaveBeenCalledWith(expect.objectContaining({ id: 8 }))
    expect(wrapper.text()).toContain(formatDateOnly('2099-01-15T00:00:00Z'))
  })

  it('shows quota expiry instead of an automatic reset for limited quota', async () => {
    getMySubscriptions.mockResolvedValue([
      {
        id: 9,
        user_id: 7,
        group_id: 3,
        status: 'active',
        starts_at: '2020-12-01T00:00:00Z',
        expires_at: '2099-01-20T00:00:00Z',
        early_reset_enabled: true,
        early_reset_duration_days: 31,
        daily_usage_usd: 0,
        weekly_usage_usd: 0,
        monthly_usage_usd: 100,
        daily_window_start: null,
        weekly_window_start: null,
        monthly_window_start: '2098-12-01T00:00:00Z',
        group: {
          id: 3,
          name: 'Limited',
          platform: 'openai',
          description: '',
          monthly_limit_usd: 230,
        },
      },
    ])

    const wrapper = shallowMount(SubscriptionsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
        },
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('userSubscriptions.quotaEndsIn')
    expect(wrapper.text()).not.toContain('userSubscriptions.resetIn')
  })

  it('localizes an early reset error from its backend reason code', async () => {
    getMySubscriptions.mockResolvedValue([
      {
        id: 8,
        user_id: 7,
        group_id: 3,
        status: 'active',
        expires_at: '2099-01-20T00:00:00Z',
        early_reset_enabled: true,
        early_reset_duration_days: 5,
        daily_usage_usd: 10,
        weekly_usage_usd: 20,
        monthly_usage_usd: 30,
        group: {
          id: 3,
          name: 'Starter',
          platform: 'openai',
          description: '',
        },
      },
    ])
    earlyResetSubscription.mockRejectedValue({
      reason: 'EARLY_RESET_WOULD_EXPIRE',
      message: 'early reset would expire the subscription',
    })

    const wrapper = shallowMount(SubscriptionsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
        },
      },
    })
    await flushPromises()

    await wrapper.find('[data-testid="subscription-early-reset"]').trigger('click')
    wrapper.findComponent(ConfirmDialog).vm.$emit('confirm')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('本次提前重置会使订阅立即过期，如需放弃套餐请联系客服')
  })

  it('shows and consumes a user quota reset allowance', async () => {
    const now = new Date()
    const startsAt = new Date(now.getTime() - 24 * 60 * 60 * 1000).toISOString()
    const expiresAt = new Date(now.getTime() + 24 * 60 * 60 * 1000).toISOString()
    getMySubscriptions.mockResolvedValue([
      {
        id: 17,
        user_id: 7,
        group_id: 3,
        status: 'active',
        starts_at: startsAt,
        expires_at: expiresAt,
        reset_count: 2,
        daily_usage_usd: 10,
        weekly_usage_usd: 20,
        monthly_usage_usd: 30,
        group: { id: 3, name: 'Weekly', platform: 'openai', weekly_limit_usd: 80 }
      }
    ])
    resetSubscriptionQuota.mockResolvedValue({
      id: 17,
      user_id: 7,
      group_id: 3,
      status: 'active',
      starts_at: startsAt,
      expires_at: expiresAt,
      reset_count: 1,
      daily_usage_usd: 0,
      weekly_usage_usd: 0,
      monthly_usage_usd: 30,
      group: { id: 3, name: 'Weekly', platform: 'openai', weekly_limit_usd: 80 }
    })

    const wrapper = shallowMount(SubscriptionsView, {
      global: {
        stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true }
      }
    })
    await flushPromises()

    await wrapper.find('[data-testid="subscription-quota-reset"]').trigger('click')
    const dialogs = wrapper.findAllComponents(ConfirmDialog)
    const dialog = dialogs.find(item => item.props('show'))
    expect(dialog).toBeDefined()
    dialog?.vm.$emit('confirm')
    await flushPromises()

    expect(resetSubscriptionQuota).toHaveBeenCalledWith(17, expect.any(String))
    expect(syncActiveSubscription).toHaveBeenCalledWith(expect.objectContaining({ id: 17, reset_count: 1 }))
    expect(showSuccess).toHaveBeenCalled()
  })

  it('does not expose user reset for early-reset, one-day, or future subscriptions', async () => {
    const now = new Date()
    const starts = new Date(now.getTime() - 12 * 60 * 60 * 1000).toISOString()
    const expires = new Date(now.getTime() + 12 * 60 * 60 * 1000).toISOString()
    const futureStarts = new Date(now.getTime() + 12 * 60 * 60 * 1000).toISOString()
    const futureExpires = new Date(now.getTime() + 48 * 60 * 60 * 1000).toISOString()
    getMySubscriptions.mockResolvedValue([
      {
        id: 18, group_id: 3, status: 'active', starts_at: starts, expires_at: '2099-01-20T00:00:00Z',
        reset_count: 2, early_reset_enabled: true, daily_usage_usd: 0, weekly_usage_usd: 0, monthly_usage_usd: 0,
        group: { id: 3, name: 'Early', platform: 'openai', weekly_limit_usd: 80 }
      },
      {
        id: 19, group_id: 3, status: 'active', starts_at: starts, expires_at: expires,
        reset_count: 2, daily_usage_usd: 0, weekly_usage_usd: 0, monthly_usage_usd: 0,
        group: { id: 3, name: 'Day', platform: 'openai', daily_limit_usd: 80 }
      },
      {
        id: 20, group_id: 3, status: 'active', starts_at: futureStarts, expires_at: futureExpires,
        reset_count: 2, daily_usage_usd: 0, weekly_usage_usd: 0, monthly_usage_usd: 0,
        group: { id: 3, name: 'Future', platform: 'openai', weekly_limit_usd: 80 }
      }
    ])

    const wrapper = shallowMount(SubscriptionsView, {
      global: {
        stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true }
      }
    })
    await flushPromises()
    expect(wrapper.findAll('[data-testid="subscription-quota-reset"]')).toHaveLength(0)
  })
})

describe('subscription lifecycle sections', () => {
  const render = () => shallowMount(SubscriptionsView, {
    global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } },
  })
  const active = {
    id: 1, group_id: 3, status: 'active', starts_at: '2020-01-01T00:00:00Z',
    expires_at: '2099-01-01T00:00:00Z', reset_count: 2,
    daily_usage_usd: 0, weekly_usage_usd: 10, monthly_usage_usd: 0,
    group: { id: 3, name: 'Current plan', platform: 'openai', weekly_limit_usd: 80 },
  }
  beforeEach(() => { vi.clearAllMocks(); getMySubscriptions.mockReset() })

  it('puts current subscriptions first, presales second, and other subscriptions last', async () => {
    getMySubscriptions.mockResolvedValue([
      active,
      { ...active, id: 2, status: 'expired' },
      { ...active, id: 3, starts_at: '2098-01-01T00:00:00Z' },
      { ...active, id: 4, expires_at: '2020-02-01T00:00:00Z' },
      { ...active, id: 5, status: 'revoked' },
    ])
    const w = render(); await flushPromises()
    const regions = w.findAll('[data-subscription-section], my-presales-stub')
    expect(regions.map(region => region.attributes('data-subscription-section') || 'presales')).toEqual(['active', 'presales', 'other'])
    expect(w.get('[data-subscription-section="active"] > header').text().replace(/\s/g, '')).toBe('userSubscriptions.activeSectionuserSubscriptions.activeSectionHint')
    expect(w.get('[data-subscription-section="active"]').findAll('[data-subscription-id]').map(card => card.attributes('data-subscription-id'))).toEqual(['1'])
    const other = w.get('[data-subscription-section="other"]')
    expect(other.get('button').text()).toBe('userSubscriptions.otherSection')
    expect(other.find('[data-subscription-id]').exists()).toBe(false)
    await other.get('button').trigger('click')
    expect(other.findAll('[data-subscription-id]')).toHaveLength(4)
    expect(other.text()).toContain('userSubscriptions.status.pending')
    expect(other.text()).toContain('userSubscriptions.status.expired')
    expect(other.find('[data-testid="subscription-renew"]').exists()).toBe(false)
    expect(other.find('[data-testid="subscription-quota-reset"]').exists()).toBe(false)
    w.unmount()
  })

  it('keeps a compact active empty state above pending reservations', async () => {
    getMySubscriptions.mockResolvedValue([])
    const w = render(); await flushPromises()
    expect(w.get('[data-subscription-section="active"]').text()).toContain('userSubscriptions.noActiveSubscriptions')
    expect(w.text()).not.toContain('userSubscriptions.noActiveSubscriptionsDesc')
    expect(w.findAll('[data-subscription-section], my-presales-stub').map(region => region.attributes('data-subscription-section') || 'presales')).toEqual(['active', 'presales'])
    w.unmount()
  })

  it('refreshes the current subscriptions after a presale refund cancels an active term', async () => {
    getMySubscriptions.mockResolvedValueOnce([active]).mockResolvedValueOnce([{ ...active, status: 'expired' }])
    const w = render(); await flushPromises()
    expect(w.get('[data-subscription-section="active"]').findAll('[data-subscription-id]')).toHaveLength(1)
    w.findComponent(MyPresales).vm.$emit('refunded'); await flushPromises()
    expect(getMySubscriptions).toHaveBeenCalledTimes(2)
    expect(w.get('[data-subscription-section="active"]').find('[data-subscription-id]').exists()).toBe(false)
    expect(w.find('[data-subscription-section="other"]').exists()).toBe(true)
    w.unmount()
  })

  it('does not misrepresent a failed request as having no active subscriptions', async () => {
    const log = vi.spyOn(console, 'error').mockImplementation(() => {})
    getMySubscriptions.mockRejectedValueOnce(new Error('unavailable')).mockResolvedValueOnce([active])
    const w = render(); await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('userSubscriptions.failedToLoad')
    expect(w.text()).not.toContain('userSubscriptions.noActiveSubscriptions')
    await w.get('[role="alert"] button').trigger('click'); await flushPromises()
    expect(w.find('[role="alert"]').exists()).toBe(false)
    expect(w.find('[data-subscription-id="1"]').exists()).toBe(true)
    w.unmount(); log.mockRestore()
  })
})
