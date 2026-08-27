import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import UserDashboardStats from '../UserDashboardStats.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key === 'dashboard.platformOther' ? 'Historical / unclassified' : key,
    }),
  }
})

describe('UserDashboardStats platform breakdown', () => {
  it('shows historical token and request usage even when its cost is zero', () => {
    const wrapper = mount(UserDashboardStats, {
      props: {
        stats: {
          total_requests: 8,
          total_tokens: 800,
          total_actual_cost: 0,
          today_actual_cost: 0,
          by_platform: [],
        } as any,
        balance: 0,
        isSimple: false,
      },
      global: {
        stubs: { Icon: true },
      },
    })

    expect(wrapper.text()).toContain('Historical / unclassified')
    expect(wrapper.text()).toContain('800')
    expect(wrapper.text()).toContain('8')
  })
})
