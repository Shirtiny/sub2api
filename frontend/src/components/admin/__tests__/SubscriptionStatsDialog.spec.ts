import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'

import SubscriptionStatsDialog from '../SubscriptionStatsDialog.vue'
import type { SubscriptionStats, SubscriptionUsageSeries } from '@/types'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false }, title: { type: String, default: '' } },
  setup: (props, { slots }) => () =>
    props.show
      ? h('div', { 'data-test': 'base-dialog' }, [
          h('h2', { 'data-test': 'dialog-title' }, props.title),
          slots.default?.(),
          slots.footer?.()
        ])
      : null
})

const SelectStub = defineComponent({
  name: 'SelectStub',
  props: {
    modelValue: { type: [String, Number, Boolean, null], default: null },
    options: { type: Array as () => Array<{ value: unknown; label: string }>, default: () => [] }
  },
  emits: ['update:modelValue', 'change'],
  setup(props, { emit }) {
    const onChange = (event: Event) => {
      const raw = (event.target as HTMLSelectElement).value
      const option = props.options.find((item) => String(item.value ?? '') === raw)
      const value = option ? option.value : raw
      emit('update:modelValue', value)
      emit('change', value, option ?? null)
    }
    return () =>
      h(
        'select',
        { value: String(props.modelValue ?? ''), onChange, 'data-test': 'horizon-select' },
        props.options.map((option) =>
          h('option', { value: option.value as string | number, key: String(option.value) }, option.label)
        )
      )
  }
})

const { getStats, getUsageSeries } = vi.hoisted(() => ({
  getStats: vi.fn(),
  getUsageSeries: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    subscriptions: { getStats, getUsageSeries }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (!params) return key
        return `${key}:${Object.values(params).join(',')}`
      }
    })
  }
})

const statsFixture: SubscriptionStats = {
  generated_at: '2026-07-21T22:10:00+08:00',
  horizon_days: 7,
  totals: {
    active_subscriptions: 112,
    active_users: 98,
    daily_limited_subscriptions: 7,
    weekly_limited_subscriptions: 105,
    remaining_today_usd: 330.12,
    remaining_week_usd: 12665.4,
    horizon_capacity_usd: 38537
  },
  plans: [
    {
      group_id: 4,
      group_name: 'Latte (拿铁)',
      platform: 'openai',
      subscriptions: 54,
      users: 54,
      daily_limit_usd: 0,
      weekly_limit_usd: 300,
      monthly_limit_usd: 0,
      daily_quota_usd: 0,
      daily_used_usd: 0,
      remaining_today_usd: 0,
      weekly_quota_usd: 16200,
      weekly_used_usd: 6863.12,
      remaining_week_usd: 9336.88,
      monthly_quota_usd: 0,
      monthly_used_usd: 0,
      remaining_month_usd: 0,
      used_usd: 6863.12,
      quota_usd: 16200,
      usage_ratio: 0.4237,
      horizon_capacity_usd: 24336.88
    },
    {
      group_id: 5,
      group_name: 'Specialty (精选)',
      platform: 'openai',
      subscriptions: 6,
      users: 6,
      daily_limit_usd: 150,
      weekly_limit_usd: 0,
      monthly_limit_usd: 0,
      daily_quota_usd: 900,
      daily_used_usd: 629.88,
      remaining_today_usd: 270.12,
      weekly_quota_usd: 0,
      weekly_used_usd: 0,
      remaining_week_usd: 0,
      monthly_quota_usd: 0,
      monthly_used_usd: 0,
      remaining_month_usd: 0,
      used_usd: 629.88,
      quota_usd: 900,
      usage_ratio: 0.6998,
      horizon_capacity_usd: 4771
    },
    {
      group_id: 13,
      group_name: 'Shaken Tea（冰摇茶）',
      platform: 'openai',
      subscriptions: 3,
      users: 3,
      daily_limit_usd: 0,
      weekly_limit_usd: 0,
      monthly_limit_usd: 220,
      daily_quota_usd: 0,
      daily_used_usd: 0,
      remaining_today_usd: 0,
      weekly_quota_usd: 0,
      weekly_used_usd: 0,
      remaining_week_usd: 0,
      monthly_quota_usd: 660,
      monthly_used_usd: 132.5,
      remaining_month_usd: 527.5,
      used_usd: 132.5,
      quota_usd: 660,
      usage_ratio: 0.2008,
      horizon_capacity_usd: 660
    }
  ],
  ranking: {
    daily: [
      {
        subscription_id: 888,
        user_id: 424,
        username: 'daily-top',
        email: 'daily@example.com',
        group_id: 5,
        group_name: 'Specialty (精选)',
        limit_usd: 150,
        used_usd: 149.3,
        remaining_usd: 0.7,
        usage_ratio: 0.9953,
        window_start: '2026-07-21T00:00:00+08:00',
        window_resets_at: '2026-07-22T00:00:00+08:00',
        expires_at: '2026-08-18T00:00:00+08:00'
      }
    ],
    weekly: [
      {
        subscription_id: 847,
        user_id: 309,
        username: 'poco',
        email: 'poco@example.com',
        group_id: 4,
        group_name: 'Latte (拿铁)',
        limit_usd: 300,
        used_usd: 300.27,
        remaining_usd: 0,
        usage_ratio: 1.0009,
        window_start: '2026-07-18T14:00:00+08:00',
        window_resets_at: '2026-07-25T14:00:00+08:00',
        expires_at: '2026-08-05T00:00:00+08:00'
      }
    ]
  }
}

const seriesFixture: SubscriptionUsageSeries = {
  subscription_id: 847,
  user_id: 309,
  username: 'poco',
  group_id: 4,
  group_name: 'Latte (拿铁)',
  starts_at: '2026-07-06T00:00:00+08:00',
  expires_at: '2026-08-05T00:00:00+08:00',
  daily_limit_usd: 0,
  weekly_limit_usd: 300,
  monthly_limit_usd: 0,
  data_from: '2026-07-18',
  data_complete: false,
  daily: [
    {
      date: '2026-07-18',
      cost_usd: 120.35,
      requests: 3564,
      limit_usd: 42.857142,
      limit_is_derived: true,
      usage_ratio: 2.808
    },
    {
      date: '2026-07-19',
      cost_usd: 20.1,
      requests: 900,
      limit_usd: 42.857142,
      limit_is_derived: true,
      usage_ratio: 0.469
    }
  ],
  weekly: [
    {
      week_start: '2026-07-18',
      week_end: '2026-07-24',
      cost_usd: 300.27,
      requests: 9012,
      limit_usd: 300,
      limit_is_derived: false,
      usage_ratio: 1.0009
    }
  ],
  cycle: {
    start: '2026-07-06',
    end: '2026-08-05',
    cost_usd: 300.27,
    quota_usd: 1200,
    usage_ratio: 0.2502,
    windows_elapsed: 4,
    window_kind: 'weekly'
  }
}

const mountDialog = async (show = true) => {
  const wrapper = mount(SubscriptionStatsDialog, {
    props: { show },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Select: SelectStub,
        Icon: true
      }
    }
  })
  await flushPromises()
  return wrapper
}

describe('SubscriptionStatsDialog overview', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getStats.mockResolvedValue(statsFixture)
    getUsageSeries.mockResolvedValue(seriesFixture)
  })

  it('does not fetch while the dialog stays closed', async () => {
    await mountDialog(false)
    expect(getStats).not.toHaveBeenCalled()
  })

  it('fetches with the default 7 day horizon when opened', async () => {
    await mountDialog()
    expect(getStats).toHaveBeenCalledWith({ horizon_days: 7 }, expect.anything())
  })

  it('renders the three summary cards', async () => {
    const wrapper = await mountDialog()

    expect(wrapper.find('[data-test="card-today"]').text()).toBe('$330.12')
    expect(wrapper.find('[data-test="card-week"]').text()).toBe('$12665.40')
    expect(wrapper.find('[data-test="card-horizon"]').text()).toBe('$38537.00')
  })

  it('refetches when the horizon changes', async () => {
    const wrapper = await mountDialog()
    getStats.mockClear()

    await wrapper.find('[data-test="horizon-select"]').setValue('14')
    await flushPromises()

    expect(getStats).toHaveBeenCalledWith({ horizon_days: 14 }, expect.anything())
  })

  it('shows a dash instead of zero when the plan has no limit for that window', async () => {
    const wrapper = await mountDialog()
    const rows = wrapper.findAll('tbody tr')

    // Latte 只有周限：今日剩余、本月剩余列应为 —
    expect(rows[0].findAll('td')[2].text()).toBe('—')
    expect(rows[0].findAll('td')[3].text()).toBe('$9336.88')
    expect(rows[0].findAll('td')[4].text()).toBe('—')

    // Specialty 只有日限
    expect(rows[1].findAll('td')[2].text()).toBe('$270.12')
    expect(rows[1].findAll('td')[3].text()).toBe('—')
    expect(rows[1].findAll('td')[4].text()).toBe('—')

    // Shaken Tea 只有月限：本月剩余列必须有值
    expect(rows[2].findAll('td')[2].text()).toBe('—')
    expect(rows[2].findAll('td')[3].text()).toBe('—')
    expect(rows[2].findAll('td')[4].text()).toBe('$527.50')
  })

  it('renders the backend-provided used / usage ratio instead of deriving them', async () => {
    const wrapper = await mountDialog()
    const rows = wrapper.findAll('tbody tr')

    // 最后两列固定读 used_usd / usage_ratio，不再按 weekly>daily 猜主窗口。
    expect(rows[0].findAll('td')[6].text()).toBe('$6863.12')
    expect(rows[0].findAll('td')[7].text()).toBe('42%')

    // 月限套餐若靠客户端推导会拿到 0，这里必须是后端给的值。
    expect(rows[2].findAll('td')[6].text()).toBe('$132.50')
    expect(rows[2].findAll('td')[7].text()).toBe('20%')
  })

  it('reports a usage ratio above 100% without clamping the number', async () => {
    const overspent: SubscriptionStats = {
      ...statsFixture,
      ranking: {
        daily: [],
        weekly: [
          {
            ...statsFixture.ranking.weekly[0],
            used_usd: 355.03,
            limit_usd: 300,
            remaining_usd: 0,
            usage_ratio: 1.1834
          }
        ]
      }
    }
    getStats.mockResolvedValue(overspent)
    const wrapper = await mountDialog()

    const row = wrapper.find('[data-test="ranking-row"]')
    expect(row.text()).toContain('118%')

    // 条宽仍然 clamp 到 100%，否则会把容器撑破。
    const bar = row.find('.h-1\\.5.rounded-full.transition-all')
    expect(bar.attributes('style')).toContain('width: 100%')
  })

  it('shows a placeholder instead of a countdown when the window awaits its lazy reset', async () => {
    const pending: SubscriptionStats = {
      ...statsFixture,
      ranking: {
        daily: [],
        weekly: [
          {
            ...statsFixture.ranking.weekly[0],
            used_usd: 0,
            remaining_usd: 300,
            usage_ratio: 0,
            window_start: null,
            window_resets_at: null
          }
        ]
      }
    }
    getStats.mockResolvedValue(pending)
    const wrapper = await mountDialog()

    const row = wrapper.find('[data-test="ranking-row"]')
    expect(row.text()).toContain('admin.subscriptions.stats.pendingReset')
    expect(row.text()).not.toContain('NaN')
    // 负数倒计时会走 resetIn* 文案，出现即说明没走占位分支。
    expect(row.text()).not.toContain('admin.subscriptions.resetIn')
  })

  it('labels a limited quota countdown as quota expiry', async () => {
    const limited: SubscriptionStats = {
      ...statsFixture,
      ranking: {
        daily: [],
        weekly: [
          {
            ...statsFixture.ranking.weekly[0],
            limited_quota: true,
            window_resets_at: '2099-01-20T00:00:00Z'
          }
        ]
      }
    }
    getStats.mockResolvedValue(limited)
    const wrapper = await mountDialog()

    const row = wrapper.find('[data-test="ranking-row"]')
    expect(row.text()).toContain('admin.subscriptions.quotaEndsInDaysHours')
    expect(row.text()).not.toContain('admin.subscriptions.resetInDaysHours')
  })

  it('defaults to the weekly ranking tab and switches to daily on click', async () => {
    const wrapper = await mountDialog()

    expect(wrapper.findAll('[data-test="ranking-row"]')).toHaveLength(1)
    expect(wrapper.find('[data-test="ranking-row"]').text()).toContain('poco')

    await wrapper.find('[data-test="ranking-tab-daily"]').trigger('click')

    expect(wrapper.find('[data-test="ranking-row"]').text()).toContain('daily-top')
  })

  it('preserves the backend ranking order without re-sorting', async () => {
    const reordered: SubscriptionStats = {
      ...statsFixture,
      ranking: {
        daily: [],
        weekly: [
          { ...statsFixture.ranking.weekly[0], subscription_id: 1, username: 'first', usage_ratio: 0.1 },
          { ...statsFixture.ranking.weekly[0], subscription_id: 2, username: 'second', usage_ratio: 0.9 }
        ]
      }
    }
    getStats.mockResolvedValue(reordered)
    const wrapper = await mountDialog()

    const rows = wrapper.findAll('[data-test="ranking-row"]')
    expect(rows[0].text()).toContain('first')
    expect(rows[1].text()).toContain('second')
  })

  it('shows an error banner when the stats request fails', async () => {
    getStats.mockRejectedValueOnce(new Error('boom'))
    const wrapper = await mountDialog()

    expect(wrapper.text()).toContain('admin.subscriptions.stats.loadFailed')
  })
})

describe('SubscriptionStatsDialog usage series detail', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getStats.mockResolvedValue(statsFixture)
    getUsageSeries.mockResolvedValue(seriesFixture)
  })

  const openDetail = async (wrapper: Awaited<ReturnType<typeof mountDialog>>) => {
    await wrapper.find('[data-test="ranking-row"]').trigger('click')
    await flushPromises()
  }

  it('loads the series for the clicked subscription', async () => {
    const wrapper = await mountDialog()
    await openDetail(wrapper)

    expect(getUsageSeries).toHaveBeenCalledWith(847, expect.anything())
  })

  it('warns that history is truncated when data_complete is false', async () => {
    const wrapper = await mountDialog()
    await openDetail(wrapper)

    const warning = wrapper.find('[data-test="incomplete-warning"]')
    expect(warning.exists()).toBe(true)
    expect(warning.text()).toContain('admin.subscriptions.stats.historyTruncated')
    expect(warning.text()).toContain('2026-07-18')
  })

  it('omits the warning when the history covers the whole cycle', async () => {
    getUsageSeries.mockResolvedValue({ ...seriesFixture, data_complete: true })
    const wrapper = await mountDialog()
    await openDetail(wrapper)

    expect(wrapper.find('[data-test="incomplete-warning"]').exists()).toBe(false)
  })

  it('flags derived denominators on daily points but not on real weekly limits', async () => {
    const wrapper = await mountDialog()
    await openDetail(wrapper)

    const dailyPoints = wrapper.findAll('[data-test="daily-point"]')
    expect(dailyPoints).toHaveLength(2)
    expect(dailyPoints[0].find('[data-test="derived-badge"]').exists()).toBe(true)

    const weeklyPoints = wrapper.findAll('[data-test="weekly-point"]')
    expect(weeklyPoints).toHaveLength(1)
    expect(weeklyPoints[0].find('[data-test="derived-badge"]').exists()).toBe(false)
  })

  it('renders the whole-cycle block', async () => {
    const wrapper = await mountDialog()
    await openDetail(wrapper)

    const text = wrapper.text()
    expect(text).toContain('admin.subscriptions.stats.cycleUsage')
    expect(text).toContain('$300.27')
    expect(text).toContain('$1200.00')
    expect(text).toContain('25%')
  })

  it('still renders the cycle block when the subscription has zero usage', async () => {
    getUsageSeries.mockResolvedValue({
      ...seriesFixture,
      daily: [],
      weekly: [],
      cycle: {
        start: '2026-07-06',
        end: '2026-08-05',
        cost_usd: 0,
        quota_usd: 1200,
        usage_ratio: 0,
        windows_elapsed: 4,
        window_kind: 'weekly'
      }
    })
    const wrapper = await mountDialog()
    await openDetail(wrapper)

    // 零用量的 cycle 是有意义的（使用率 0%），不能被真值判断藏掉。
    expect(wrapper.text()).toContain('admin.subscriptions.stats.cycleUsage')
    expect(wrapper.text()).toContain('$1200.00')
    expect(wrapper.text()).toContain('0%')
  })

  it('hides the cycle block only when the backend reports no windowed limit at all', async () => {
    getUsageSeries.mockResolvedValue({ ...seriesFixture, cycle: null })
    const wrapper = await mountDialog()
    await openDetail(wrapper)

    expect(wrapper.text()).not.toContain('admin.subscriptions.stats.cycleUsage')
  })

  it('returns to the overview and drops the loaded series', async () => {
    const wrapper = await mountDialog()
    await openDetail(wrapper)

    await wrapper.find('[data-test="detail-back"]').trigger('click')
    await flushPromises()

    expect(wrapper.vm.view).toBe('overview')
    expect(wrapper.vm.series).toBeNull()
    expect(wrapper.findAll('[data-test="ranking-row"]').length).toBeGreaterThan(0)
  })

  it('shows an error banner when the series request fails', async () => {
    getUsageSeries.mockRejectedValueOnce(new Error('boom'))
    const wrapper = await mountDialog()
    await openDetail(wrapper)

    expect(wrapper.text()).toContain('admin.subscriptions.stats.seriesLoadFailed')
  })
})
