import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => {
    const value = key.split('.').reduce<unknown>((item, part) => (item as Record<string, unknown>)?.[part], zh)
    return typeof value === 'string' ? value : key
  } })
}))
import AvailableChannelsView from '../AvailableChannelsView.vue'
import ModelOfferingCard from '@/components/channels/ModelOfferingCard.vue'
import zh from '@/i18n/locales/zh'

const { getAvailable, getRates } = vi.hoisted(() => ({ getAvailable: vi.fn(), getRates: vi.fn() }))
vi.mock('@/api/channels', () => ({ default: { getAvailable } }))
vi.mock('@/api/groups', () => ({ default: { getUserGroupRates: getRates } }))
const channels = [{ name: 'The Main Offerings', description: '', platforms: [{ platform: 'openai', groups: [{ id: 1, name: 'Astra', rate_multiplier: .5 }], supported_models: [
  { name: 'gpt-6/luna', platform: 'openai', pricing: null }, { name: 'gpt-5.6/terra', platform: 'openai', pricing: null }
] }] }]
function render() { return shallowMount(AvailableChannelsView, { global: { plugins: [createI18n({ legacy: false, locale: 'zh', messages: { zh } })], stubs: { AppLayout: { template: '<main><slot /></main>' } } } }) }
describe('AvailableChannels collection', () => {
  beforeEach(() => { getAvailable.mockReset().mockResolvedValue(channels); getRates.mockReset().mockResolvedValue({ 1: .4 }) })
  it('shows one collection, not a crowded channel table, with effective group rates', async () => {
    const wrapper = render(); await flushPromises()
    expect(wrapper.find('.collection-intro').exists()).toBe(false)
    expect(wrapper.find('.collection-footnote').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('展示价格已含模型倍率')
    expect(wrapper.text()).not.toContain('THE MODEL COLLECTION')
    expect(wrapper.text()).not.toContain('为工作，选一份智能。')
    expect(wrapper.findAll('.channel-heading')).toHaveLength(1)
    expect(wrapper.find('h2').text()).toBe('The Main Offerings')
    expect(wrapper.find('table').exists()).toBe(false)
    expect(wrapper.findAllComponents(ModelOfferingCard)).toHaveLength(2)
    expect(wrapper.find('.group-pill').text()).toContain('0.4×')
  })
  it('filters individual models rather than keeping every model in a matching platform', async () => {
    const wrapper = render(); await flushPromises()
    await wrapper.find('input').setValue('TERRA')
    expect(wrapper.findAllComponents(ModelOfferingCard)).toHaveLength(1)
    expect(wrapper.findComponent(ModelOfferingCard).props('model').name).toBe('gpt-5.6/terra')
    await wrapper.find('input').setValue('not-a-model')
    expect(wrapper.text()).toContain('没有匹配的模型')
    await wrapper.find('input').setValue('Astra')
    expect(wrapper.findAllComponents(ModelOfferingCard)).toHaveLength(2)
  })
  it('renders a recoverable failure and an honest empty state', async () => {
    getAvailable.mockRejectedValueOnce(new Error('oops'))
    const wrapper = render(); await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    getAvailable.mockResolvedValueOnce([])
    await wrapper.find('[role="alert"] button').trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('暂无可用渠道')
  })
  it('aborts a pending request on unmount', async () => {
    getAvailable.mockReturnValue(new Promise(() => {}))
    const wrapper = render(); const signal = getAvailable.mock.calls[0][0].signal
    wrapper.unmount(); expect(signal.aborted).toBe(true)
  })
})
