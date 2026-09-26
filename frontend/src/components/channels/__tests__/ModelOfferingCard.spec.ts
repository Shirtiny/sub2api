import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { createI18n } from 'vue-i18n'
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string, params: Record<string, string> = {}) => {
    const value = key.split('.').reduce<unknown>((item, part) => (item as Record<string, unknown>)?.[part], zh)
    return typeof value === 'string' ? value.replace(/\{(\w+)\}/g, (match, name) => params[name] ?? match) : key
  } })
}))
import ModelOfferingCard from '../ModelOfferingCard.vue'
import zh from '@/i18n/locales/zh'
import type { UserSupportedModel } from '@/api/channels'

const copy = vi.hoisted(() => vi.fn())
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copied: ref(false), copyToClipboard: copy }) }))
const pricing = { billing_mode: 'token' as const, input_price: 2e-6, output_price: 10e-6, cache_read_price: .2e-6, cache_write_price: 2.5e-6, image_output_price: null, per_request_price: null, intervals: [] }
const mounted: ReturnType<typeof mount>[] = []
function render(model: UserSupportedModel) {
  const wrapper = mount(ModelOfferingCard, { props: { model }, global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh } })], stubs: { teleport: true } } })
  mounted.push(wrapper)
  return wrapper
}
describe('ModelOfferingCard', () => {
  beforeEach(() => copy.mockClear())
  afterEach(() => { mounted.splice(0).forEach(wrapper => wrapper.unmount()) })
  it('copies exactly the model ID and shows original and discounted prices without group-rate conflation', async () => {
    const wrapper = render({ name: 'gpt-example/astra', platform: 'openai', pricing: { ...pricing, price_multiplier: .1 } })
    expect(wrapper.find('.copy-caption').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('点击复制模型 ID')
    expect(wrapper.find('.model-rate').text()).toBe('0.1×')
    expect(wrapper.findAll('dd').map(e => e.text())).toEqual(['$0.2', '$1'])
    expect(wrapper.findAll('s').map(e => e.text())).toEqual(['$2', '$10'])
    await wrapper.find('.model-copy').trigger('click')
    expect(copy).toHaveBeenCalledTimes(1)
    expect(copy).toHaveBeenCalledWith('gpt-example/astra')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(wrapper.find('.price-details').text()).toBe('查看详情')
    await wrapper.find('.price-details').trigger('click')
    expect(wrapper.find('.cache-prices').text()).toContain('$0.02')
  })
  it('defaults existing records to 1x and supports markups', () => {
    const legacy = render({ name: 'old', platform: 'openai', pricing })
    expect(legacy.find('.model-rate').text()).toBe('1×')
    expect(legacy.findAll('s')).toHaveLength(0)
    const markup = render({ name: 'new', platform: 'openai', pricing: { ...pricing, price_multiplier: 5 } })
    expect(markup.findAll('dd').map(e => e.text())).toEqual(['$10', '$50'])
  })
  it('shows image per-request prices and tier details, not the image token rate', async () => {
    const wrapper = render({ name: 'image', platform: 'openai', pricing: { ...pricing, billing_mode: 'image', price_multiplier: 5, per_request_price: .04, image_output_price: 9e-6,
      intervals: [{ min_tokens: 0, max_tokens: null, tier_label: 'HD', input_price: null, output_price: null, cache_read_price: null, cache_write_price: null, per_request_price: .08 }] } })
    expect(wrapper.find('dd').text()).toBe('$0.2')
    expect(wrapper.find('.price-details').text()).toBe('查看详情')
    await wrapper.find('.price-details').trigger('click')
    expect(wrapper.find('.tier-table').text()).toContain('$0.4')
    expect(wrapper.find('.tier-table').text()).toContain('HD')
    expect(wrapper.text()).toContain('USD / 次')
  })
  it.each([
    [0, 272000, '272k 及以下'],
    [272000, null, '超过 272k'],
    [128000, 272000, '超过 128k，至 272k'],
    [0, null, '全部上下文'],
    [0, 500, '500 及以下'],
    [0, 272001, '272.001k 及以下']
  ])('renders readable exact context bounds (%s, %s]', async (min, max, label) => {
    const wrapper = render({ name: 'tiered-model', platform: 'openai', pricing: { ...pricing,
      intervals: [{ ...pricing, min_tokens: min as number, max_tokens: max as number | null }] } })
    await wrapper.find('.price-details').trigger('click')
    expect(wrapper.findAll('.tier-table tbody th')[1].text()).toBe(label)
  })
  it('opens a separate dialog for long tier lists instead of an inline expansion', async () => {
    const intervals = Array.from({ length: 25 }, (_, index) => ({
      min_tokens: index * 1000, max_tokens: (index + 1) * 1000, tier_label: '',
      input_price: 2e-6, output_price: 10e-6, cache_read_price: .2e-6, cache_write_price: 2.5e-6, per_request_price: null
    }))
    const wrapper = render({ name: 'tiered-model', platform: 'openai', pricing: { ...pricing, intervals } })
    expect(wrapper.find('details').exists()).toBe(false)
    expect(wrapper.find('.tier-table').exists()).toBe(false)
    expect(wrapper.find('.price-details').attributes('aria-haspopup')).toBe('dialog')
    await wrapper.find('.price-details').trigger('click')
    expect(wrapper.find('.price-details').attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(true)
    expect(wrapper.findAll('.tier-table tbody tr')).toHaveLength(26)
    await wrapper.find('button[aria-label="Close modal"]').trigger('click')
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(wrapper.find('.price-details').attributes('aria-expanded')).toBe('false')
  })
  it('does not invent a price or multiplier for an unpriced model', () => {
    const wrapper = render({ name: 'unknown', platform: 'openai', pricing: null })
    expect(wrapper.text()).toContain('未配置定价')
    expect(wrapper.find('.model-rate').exists()).toBe(false)
  })
})
