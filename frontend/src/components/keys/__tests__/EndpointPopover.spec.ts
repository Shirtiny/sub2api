import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const copyToClipboard = vi.fn().mockResolvedValue(true)
const fetchMock = vi.fn()

const messages: Record<string, string> = {
  'keys.endpoints.title': 'API 端点',
  'keys.endpoints.default': '默认',
  'keys.endpoints.copied': '已复制',
  'keys.endpoints.copiedHint': '已复制到剪贴板',
  'keys.endpoints.clickToCopy': '点击可复制此端点',
  'keys.endpoints.latency': '延迟',
  'keys.endpoints.checkingLatency': '正在检测延迟',
  'keys.endpoints.latencyUnavailable': '延迟检测失败',
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => messages[key] ?? key,
  }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard,
  }),
}))

import EndpointPopover from '../EndpointPopover.vue'

describe('EndpointPopover', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    copyToClipboard.mockResolvedValue(true)
    fetchMock.mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue({ status: 'ok' }),
    })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('将说明提示渲染到 URL 上方而不是旧的 title 图标上', () => {
    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: 'https://default.example.com/v1',
        customEndpoints: [
          {
            name: '备用线路',
            endpoint: 'https://backup.example.com/v1',
            description: '自定义说明',
          },
        ],
      },
    })

    expect(wrapper.text()).toContain('自定义说明')
    expect(wrapper.text()).toContain('点击可复制此端点')
    expect(wrapper.find('[role="button"]').attributes('title')).toBeUndefined()
    expect(wrapper.find('[title="自定义说明"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('使用主题语义颜色并保持复制提示高对比可读', () => {
    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: 'https://default.example.com/v1',
        customEndpoints: [],
      },
    })

    const tooltip = wrapper.find('[role="tooltip"]')
    expect(tooltip.classes()).toEqual(expect.arrayContaining([
      'border-stroke-default',
      'bg-surface-card',
      'ring-stroke-subtle/70',
    ]))
    const legacySlateClasses = tooltip.classes().filter((className) =>
      /^(?:dark:)?(?:border|bg|ring|text)-slate-/.test(className),
    )
    expect(legacySlateClasses).toEqual([])

    const hint = tooltip.findAll('p').at(-1)
    expect(hint?.classes()).toContain('text-content-primary')
    expect(hint?.classes()).toContain('font-medium')
    wrapper.unmount()
  })

  it('点击 URL 后会复制并切换为已复制提示', async () => {
    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: 'https://default.example.com/v1',
        customEndpoints: [],
      },
    })

    await wrapper.find('[role="button"]').trigger('click')
    await flushPromises()

    expect(copyToClipboard).toHaveBeenCalledWith('https://default.example.com/v1', '已复制')
    expect(wrapper.text()).toContain('已复制到剪贴板')
    const copiedButton = wrapper.find('button[aria-label="已复制到剪贴板"]')
    expect(copiedButton.exists()).toBe(true)
    expect(copiedButton.classes()).toContain('text-primary-500')
    expect(copiedButton.classes().some((className) => className.includes('emerald'))).toBe(false)
    wrapper.unmount()
  })

  it('进入页面后仅在 0、30、90 秒检测三次端点延迟', async () => {
    vi.useFakeTimers()
    let now = 0
    vi.spyOn(performance, 'now').mockImplementation(() => {
      now += 25
      return now
    })

    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: 'https://default.example.com/v1',
        customEndpoints: [
          {
            name: '备用线路',
            endpoint: 'https://backup.example.com',
            description: '',
          },
        ],
      },
    })

    await flushPromises()
    expect(fetchMock).toHaveBeenCalledTimes(2)
    const initialPingUrls = fetchMock.mock.calls.map(([url]) => {
      const parsed = new URL(String(url))
      expect(parsed.searchParams.has('_ts')).toBe(true)
      return `${parsed.origin}${parsed.pathname}`
    })
    expect(initialPingUrls).toEqual(expect.arrayContaining([
      'https://default.example.com/v1/ping',
      'https://backup.example.com/v1/ping',
    ]))
    expect(wrapper.text()).toMatch(/\d+ms/)
    expect(wrapper.find('a[title="测速"]').exists()).toBe(false)

    const latencyText = wrapper.find('span[aria-live="polite"]')
    expect(latencyText.classes().some(className => className.startsWith('min-w-'))).toBe(false)
    expect(latencyText.classes()).toEqual(expect.arrayContaining([
      'text-emerald-600',
      'dark:text-emerald-400',
    ]))

    await vi.advanceTimersByTimeAsync(29_999)
    expect(fetchMock).toHaveBeenCalledTimes(2)

    await vi.advanceTimersByTimeAsync(1)
    await flushPromises()
    expect(fetchMock).toHaveBeenCalledTimes(4)

    await vi.advanceTimersByTimeAsync(59_999)
    expect(fetchMock).toHaveBeenCalledTimes(4)

    await vi.advanceTimersByTimeAsync(1)
    await flushPromises()
    expect(fetchMock).toHaveBeenCalledTimes(6)

    await vi.advanceTimersByTimeAsync(120_000)
    expect(fetchMock).toHaveBeenCalledTimes(6)
    wrapper.unmount()
  })

  it('colors endpoint latency by response time', async () => {
    const latencies: Record<string, number> = {
      'fast.example.com': 100,
      'medium.example.com': 300,
      'slow.example.com': 600,
    }
    const getEntriesByName = vi.fn((input: string, _entryType?: string) => {
      const latency = latencies[new URL(input).hostname]
      return [{
        requestStart: 100,
        responseStart: 100 + latency,
      } as PerformanceResourceTiming]
    })
    vi.stubGlobal('performance', {
      now: vi.fn(() => 1_000),
      getEntriesByName,
    })
    fetchMock.mockResolvedValue({
      ok: true,
      json: vi.fn().mockResolvedValue({ status: 'ok' }),
    })

    const wrapper = mount(EndpointPopover, {
      props: {
        apiBaseUrl: 'https://fast.example.com/v1',
        customEndpoints: [
          { name: 'Medium', endpoint: 'https://medium.example.com/v1', description: '' },
          { name: 'Slow', endpoint: 'https://slow.example.com/v1', description: '' },
        ],
      },
    })

    await flushPromises()

    const latencyTexts = wrapper.findAll('span[aria-live="polite"]')
    expect(latencyTexts.map(item => item.text())).toEqual(['100ms', '300ms', '600ms'])
    expect(latencyTexts[0].classes()).toEqual(expect.arrayContaining([
      'text-emerald-600',
      'dark:text-emerald-400',
    ]))
    expect(latencyTexts[1].classes()).toEqual(expect.arrayContaining([
      'text-amber-600',
      'dark:text-amber-400',
    ]))
    expect(latencyTexts[2].classes()).toEqual(expect.arrayContaining([
      'text-red-600',
      'dark:text-red-400',
    ]))
    expect(getEntriesByName).toHaveBeenCalledTimes(3)
    expect(getEntriesByName.mock.calls.every(([, type]) => type === 'resource')).toBe(true)
    wrapper.unmount()
  })
})
