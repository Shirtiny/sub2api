import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SubscriptionsView from '../SubscriptionsView.vue'

const routerPush = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())
const getMySubscriptions = vi.hoisted(() => vi.fn())

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: routerPush }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => params?.days ? `${params.days} days` : key }),
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError }),
}))

vi.mock('@/api/subscriptions', () => ({
  default: {
    getMySubscriptions,
  },
}))

describe('SubscriptionsView renewal routing', () => {
  beforeEach(() => {
    routerPush.mockReset()
    showError.mockReset()
    getMySubscriptions.mockReset()
  })

  it('shows the multiplier on custom subscription cards', async () => {
    getMySubscriptions.mockResolvedValue([
      {
        id: 44,
        group_id: 99,
        status: 'active',
        expires_at: '2099-01-01T00:00:00Z',
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
  })

  it('shows the multiplier badge even when the custom group name already includes it', async () => {
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
          name: '[3x]Starter#7',
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
    expect(wrapper.text()).not.toContain('[3x]Starter#7')
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

    await wrapper.find('button').trigger('click')

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
})
