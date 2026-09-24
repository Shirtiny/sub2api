import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'
import PresaleView from '../PresaleView.vue'
import HomeHeader from '@/components/home/HomeHeader.vue'
import Select from '@/components/common/Select.vue'
import type { UserSubscription } from '@/types'
const mocks = vi.hoisted(() => ({ catalog: vi.fn(), push: vi.fn(), replace: vi.fn(), fetchSubscriptions: vi.fn(), activeSubscriptions: [] as UserSubscription[], auth: { isAuthenticated: false }, route: { query: {} as Record<string,string>, hash: '' }, locale: 'zh' }))
vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ locale: { value: mocks.locale }, t: (key: string, args: Record<string,unknown> = {}) => {
  let value: unknown = mocks.locale === 'zh' ? zh : en
  for (const part of key.split('.')) value = value && typeof value === 'object' ? (value as Record<string,unknown>)[part] : undefined
  return typeof value === 'string' ? value.replace(/\{(\w+)\}/g, (_, name) => String(args[name] ?? `{${name}}`)) : key
} }) }))
vi.mock('@/api/presale' , () => ({ presaleAPI: { catalog: mocks.catalog } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ siteName: 'Café Shop', docUrl: 'https://docs.example.com/guide', publicSettingsLoaded: true }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => mocks.auth }))
vi.mock('@/stores/subscriptions', () => ({ useSubscriptionStore: () => ({ activeSubscriptions: mocks.activeSubscriptions, fetchActiveSubscriptions: mocks.fetchSubscriptions }) }))
vi.mock('vue-router', () => ({ useRoute: () => mocks.route, useRouter: () => ({ push: mocks.push, replace: mocks.replace }) }))
vi.mock('@/views/user/PaymentView.vue', () => ({ default: { props: ['presalePlanId','presaleMonth','presaleMultiplier'], template: '<div class="checkout-fixture" :data-multiplier="presaleMultiplier">{{ presalePlanId }} / {{ presaleMonth }}</div>' } }))
const fixture = () => ({ enabled: true, server_time: '2026-09-23T00:00:00Z', period: { month: '2026-10', timezone: 'Asia/Shanghai', starts_at: '2026-10-01T00:00:00+08:00', expires_at: '2026-11-01T00:00:00+08:00', full_refund_before: '2026-09-28T00:00:00+08:00' }, plans: [{ id: 12, group_id: 3, group_platform: 'openai', name: 'Astra Monthly', description: 'Focus for a month', price: 300, concurrency: 2, features: ['Codex + Pi'], presale_enabled: true, presale_reset_cards: 2 }] })
function render(locale = 'zh') {
  mocks.locale = locale
  return mount(PresaleView, { global: { stubs: { RouterLink: { props: ['to'], template: '<a :href="typeof to === \'string\' ? to : to.path"><slot /></a>' }, LocaleSwitcher: true, Icon: true, BaseDialog: { name: 'BaseDialog', props: ['show'], template: '<div v-if="show"><slot /></div>' } } } })
}
beforeEach(() => { vi.clearAllMocks(); mocks.activeSubscriptions.length = 0; mocks.fetchSubscriptions.mockResolvedValue([]); mocks.auth.isAuthenticated = false; mocks.route.query = {}; mocks.route.hash = ''; mocks.catalog.mockResolvedValue({ data: fixture() }) })
describe('presale landing', () => {
  it.each([['zh', '预定'], ['en', 'Reserve']])('uses the concise reservation label in %s', async (locale, label) => {
    mocks.auth.isAuthenticated = true
    const w = render(locale); await flushPromises()
    expect(w.get('.plan-buy').text()).toBe(label)
    w.unmount()
  })
  it('preserves the plans anchor when opening and closing checkout', async () => {
    mocks.auth.isAuthenticated = true
    mocks.route.hash = '#presale-plans'
    const w = render(); await flushPromises()
    await w.get('.plan-buy').trigger('click')
    expect(mocks.replace).toHaveBeenCalledWith({ path: '/presale', query: { plan: '12' }, hash: '#presale-plans' })
    w.getComponent({ name: 'BaseDialog' }).vm.$emit('close'); await flushPromises()
    expect(mocks.replace).toHaveBeenLastCalledWith({ path: '/presale', hash: '#presale-plans' })
    w.unmount()
  })
  it.each(['zh', 'en'])('discloses the daily refund fee and final-week exclusion in %s', async (locale) => {
    const w = render(locale); await flushPromises()
    const policy = w.get('.policy-details > div:nth-child(3)').text()
    expect(policy).toContain('20%')
    expect(policy).toContain(locale === 'zh' ? '剩余完整天数' : 'remaining whole days')
    expect(policy).toContain(locale === 'zh' ? '7 天' : 'seven days or fewer')
    w.unmount()
  })
  function customFixture() {
    const data = fixture()
    return { ...data, plans: [{ ...data.plans[0], price: 1, original_price: 690, daily_limit_usd: 10, weekly_limit_usd: 420, monthly_limit_usd: 0, custom_multiplier_enabled: true, custom_multiplier_min: 2, custom_multiplier_max: 4 }] }
  }
  it('selects configured integer multipliers on the card and scales prices and quotas before login', async () => {
    mocks.catalog.mockResolvedValue({ data: customFixture() })
    const w = render(); await flushPromises()
    const select = w.getComponent(Select)
    expect(select.props('options')).toEqual([2, 3, 4].map(value => ({ value, label: `${value}x` })))
    expect(select.props('modelValue')).toBe(2)
    expect(w.get('.plan-price strong').text()).toBe('￥2')
    expect(w.find('input[type="number"]').exists()).toBe(false)
    select.vm.$emit('update:modelValue', 3); await flushPromises()
    expect(w.get('.plan-price strong').text()).toBe('￥3')
    expect(w.get('.old-price').text()).toBe('￥2,070')
    expect(w.get('.plan-quotas').text()).toContain('$30')
    expect(w.get('.plan-quotas').text()).toContain('$1,260')
    expect(w.findAll('.plan-quotas div')).toHaveLength(2)
    await w.get('.plan-buy').trigger('click')
    expect(mocks.push).toHaveBeenCalledWith({ path: '/login', query: { redirect: '/presale?plan=12&multiplier=3' } })
    expect(mocks.fetchSubscriptions).not.toHaveBeenCalled()
    w.unmount()
  })
  it('carries the selected multiplier into checkout and the shareable route', async () => {
    mocks.auth.isAuthenticated = true
    mocks.catalog.mockResolvedValue({ data: customFixture() })
    const w = render(); await flushPromises()
    w.getComponent(Select).vm.$emit('update:modelValue', 4); await flushPromises()
    await w.get('.plan-buy').trigger('click')
    expect(w.get('.checkout-fixture').attributes('data-multiplier')).toBe('4')
    expect(mocks.replace).toHaveBeenCalledWith({ path: '/presale', query: { plan: '12', multiplier: '4' }, hash: '' })
    w.unmount()
  })
  it.each([['3', '3'], ['99', '4'], ['0', '2'], ['invalid', '2']])('restores a login return multiplier %s as %s within configured bounds', async (requested, expected) => {
    mocks.auth.isAuthenticated = true
    mocks.route.query = { plan: '12', multiplier: requested }
    mocks.catalog.mockResolvedValue({ data: customFixture() })
    const w = render(); await flushPromises()
    expect(w.get('.checkout-fixture').attributes('data-multiplier')).toBe(expected)
    expect(w.getComponent(Select).props('modelValue')).toBe(Number(expected))
    w.unmount()
  })
  it('keeps different card selections independent and ignores unsupported choices', async () => {
    const data = customFixture()
    data.plans.push({ ...data.plans[0], id: 13 })
    mocks.catalog.mockResolvedValue({ data })
    const w = render(); await flushPromises()
    const selects = w.findAllComponents(Select)
    selects[0].vm.$emit('update:modelValue', 4); await flushPromises()
    selects[1].vm.$emit('update:modelValue', 99); await flushPromises()
    expect(w.findAll('.plan-price strong').map(p => p.text())).toEqual(['￥4', '￥2'])
    w.unmount()
  })
  it('does not apply a route multiplier when the plan has customization disabled', async () => {
    mocks.auth.isAuthenticated = true
    mocks.route.query = { plan: '12', multiplier: '4' }
    const w = render(); await flushPromises()
    expect(w.findComponent(Select).exists()).toBe(false)
    expect(w.get('.checkout-fixture').attributes('data-multiplier')).toBe('1')
    expect(w.get('.plan-price strong').text()).toBe('￥300')
    w.unmount()
  })
  it('retains the existing custom subscription multiplier for a renewal', async () => {
    mocks.auth.isAuthenticated = true
    mocks.activeSubscriptions.push({ status: 'active', expires_at: '2099-01-01T00:00:00Z', custom_source_plan_id: 12, custom_multiplier: 5 } as UserSubscription)
    mocks.route.query = { plan: '12' }
    mocks.catalog.mockResolvedValue({ data: customFixture() })
    const w = render(); await flushPromises()
    expect(w.getComponent(Select).props('disabled')).toBe(true)
    expect(w.getComponent(Select).props('options')).toEqual([{ value: 5, label: '5x' }])
    expect(w.get('.checkout-fixture').attributes('data-multiplier')).toBe('5')
    expect(w.get('.plan-price strong').text()).toBe('￥5')
    w.unmount()
  })
  it.each(['zh', 'en'])('marks both plan prices in renminbi without a checkout footnote in %s', async (locale) => {
    const data = fixture()
    mocks.catalog.mockResolvedValue({ data: { ...data, plans: [{ ...data.plans[0], price: 1, original_price: 690 }] } })
    const w = render(locale); await flushPromises()
    expect(w.get('.plan-price .old-price').text()).toBe('￥690')
    expect(w.get('.plan-price strong').text()).toBe('￥1')
    expect(w.find('.price-note').exists()).toBe(false)
    expect(w.text()).not.toContain('金额币种与支付渠道费用，以结算页为准。')
    expect(w.text()).not.toContain('Payment currency and any channel fees are confirmed at checkout.')
    w.unmount()
  })
  it.each([
    ['01', 'jan'], ['02', 'feb'], ['03', 'mar'], ['04', 'apr'],
    ['05', 'may'], ['06', 'jun'], ['07', 'jul'], ['08', 'aug'],
    ['09', 'sep'], ['10', 'oct'], ['11', 'nov'], ['12', 'dec'],
  ])('labels month %s with its lowercase English abbreviation %s', async (month, abbreviation) => {
    const data = fixture()
    data.period.month = `2027-${month}`
    data.period.starts_at = `2027-${month}-01T00:00:00+08:00`
    data.period.expires_at = new Date(Date.UTC(2027, Number(month), 1, -8)).toISOString()
    mocks.catalog.mockResolvedValue({ data })
    const w = render(); await flushPromises()
    expect(w.get('.ticket-date strong').text()).toBe(month)
    expect(w.get('.ticket-month-abbr').text()).toBe(abbreviation)
    expect(w.get('.ticket-kicker').text()).toBe('NEXT / 2027')
    w.unmount()
  })
  it('keeps the English month mark in either locale across a UTC year boundary', async () => {
    const data = fixture()
    data.period.month = '2027-01'
    data.period.starts_at = '2026-12-31T16:00:00Z'
    data.period.expires_at = '2027-01-31T16:00:00Z'
    mocks.catalog.mockResolvedValue({ data })
    for (const locale of ['zh', 'en']) {
      const w = render(locale); await flushPromises()
      expect(w.get('.ticket-date strong').text()).toBe('01')
      expect(w.get('.ticket-month-abbr').text()).toBe('jan')
      expect(w.get('.ticket-month').text()).toBe(locale === 'zh' ? '2027年1月' : 'January 2027')
      w.unmount()
    }
  })
  it('reuses the homepage header, branding, documentation and theme controls', async () => {
    localStorage.setItem('theme', 'dark')
    const w = render(); await flushPromises()
    expect(w.findComponent(HomeHeader).exists()).toBe(true)
    expect(w.find('.presale-header, .wordmark').exists()).toBe(false)
    expect(w.get('.home-header .brand').attributes('aria-label')).toBe('Café Shop')
    expect(w.get('.header-menu a[href="/presale"]').text()).toBe('订阅预售')
    expect(w.get('.header-menu a[href="https://docs.example.com/guide"]').exists()).toBe(true)
    expect(w.get('.header-entry').attributes('href')).toBe('/login')
    await w.get('.theme-toggle').trigger('click')
    expect(localStorage.getItem('theme')).toBe('light')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    w.unmount(); localStorage.removeItem('theme')
  })
  it('renders published plans and explicit dates/refund rules without login', async () => {
    const w = render(); await flushPromises()
    expect(w.text()).toContain('Astra Monthly'); expect(w.text()).toContain('20%')
    expect(w.text()).toContain('2026/10/01'); expect(w.text()).toContain('2026/11/01')
    expect(w.text()).toContain('北京时间'); expect(w.findAll('.presale-plan')).toHaveLength(1)
    expect(w.get('.plan-price strong').text()).toBe('￥300')
    expect(w.find('.old-price').exists()).toBe(false)
    expect(w.find('.checkout-fixture').exists()).toBe(false)
    await w.get('.plan-buy').trigger('click')
    expect(mocks.push).toHaveBeenCalledWith({ path: '/login', query: { redirect: '/presale?plan=12' } })
    w.unmount()
  })
  it('opens existing checkout for authenticated users and restores a login return', async () => {
    mocks.auth.isAuthenticated = true; mocks.route.query = { plan: '12' }
    const w = render('en'); await flushPromises()
    expect(w.get('.checkout-fixture').text()).toBe('12 / 2026-10')
    expect(w.get('.header-entry').attributes('href')).toBe('/dashboard')
    expect(w.text()).toContain('unconditional full refund')
    expect(w.text()).toContain('seven days or fewer')
    w.unmount()
  })
  it('does not render purchasable plans when payment or presale is closed', async () => {
    mocks.catalog.mockResolvedValue({ data: { ...fixture(), enabled: false } })
    const w = render(); await flushPromises(); expect(w.find('.plan-buy').exists()).toBe(false); expect(w.text()).toContain('尚未开放'); w.unmount()
  })
  it('shows a recoverable error rather than a fake open sale', async () => {
    mocks.catalog.mockRejectedValue(new Error('offline'))
    const w = render(); await flushPromises(); expect(w.find('[role="alert"]').exists()).toBe(true); expect(w.find('.plan-buy').exists()).toBe(false)
    expect(w.get('.ticket-month-abbr').text()).toBe('—')
    mocks.catalog.mockResolvedValue({ data: fixture() }); await w.get('[role="alert"] button').trigger('click'); await flushPromises(); expect(w.find('.plan-buy').exists()).toBe(true); w.unmount()
  })
})
