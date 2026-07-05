import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import UsageStatsCards from '../UsageStatsCards.vue'

const messages: Record<string, string> = {
  'usage.totalRequests': 'Total Requests',
  'usage.inSelectedRange': 'In selected range',
  'usage.totalTokens': 'Total Tokens',
  'usage.in': 'In',
  'usage.out': 'Out',
  'usage.cacheHit': 'Cache Hit',
  'usage.cacheCreate': 'Cache Create',
  'usage.cacheHitRate': 'Cache Hit Rate',
  'usage.balanceGroup': 'Balance',
  'usage.subscriptionGroup': 'Subscription',
  'usage.unknownGroup': 'Unknown group',
  'usage.noCacheHitRecords': 'No records',
  'usage.totalCost': 'Total Cost',
  'usage.accountCost': 'Account cost',
  'usage.standardCost': 'Standard cost',
  'usage.avgDuration': 'Average Duration',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

describe('admin UsageStatsCards', () => {
  it('renders cache hit rates by filtered usage billing group', () => {
    const wrapper = mount(UsageStatsCards, {
      props: {
        stats: {
          total_requests: 2,
          total_input_tokens: 25,
          total_output_tokens: 0,
          total_cache_tokens: 125,
          total_cache_creation_tokens: 0,
          total_cache_read_tokens: 125,
          total_tokens: 150,
          total_cost: 0,
          total_actual_cost: 0,
          total_account_cost: 0,
          average_duration_ms: 0,
          cache_by_group_type: [
            {
              group_type: 'standard',
              requests: 1,
              input_tokens: 20,
              cache_creation_tokens: 0,
              cache_read_tokens: 80,
              total_input_tokens: 100,
              hit_rate: 80,
            },
            {
              group_type: 'subscription',
              requests: 1,
              input_tokens: 5,
              cache_creation_tokens: 0,
              cache_read_tokens: 45,
              total_input_tokens: 50,
              hit_rate: 90,
            },
          ],
        },
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    const text = wrapper.text()
    expect(text).toContain('Cache Hit 125')
    expect(text).toContain('Cache Create 0')
    expect(text).toContain('Cache Hit Rate')
    expect(text).toContain('Subscription:')
    expect(text).toContain('90.0%')
    expect(text).toContain('Balance:')
    expect(text).toContain('80.0%')
    expect(text).not.toContain('No records')
  })

  it('renders no records when every cache group has no prompt tokens', () => {
    const wrapper = mount(UsageStatsCards, {
      props: {
        stats: {
          total_requests: 0,
          total_input_tokens: 0,
          total_output_tokens: 0,
          total_cache_tokens: 0,
          total_cache_creation_tokens: 0,
          total_cache_read_tokens: 0,
          total_tokens: 0,
          total_cost: 0,
          total_actual_cost: 0,
          total_account_cost: 0,
          average_duration_ms: 0,
          cache_by_group_type: [
            {
              group_type: 'standard',
              requests: 1,
              input_tokens: 0,
              cache_creation_tokens: 0,
              cache_read_tokens: 0,
              total_input_tokens: 0,
              hit_rate: 0,
            },
          ],
        },
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    const text = wrapper.text()
    expect(text).toContain('Cache Hit Rate')
    expect(text).toContain('No records')
    expect(text).not.toContain('Balance:')
    expect(text).not.toContain('Subscription:')
  })
})
