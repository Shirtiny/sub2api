import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'
import PresaleView from '../PresaleView.vue'
import HomeHeader from '@/components/home/HomeHeader.vue'
import Select from '@/components/common/Select.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { UserSubscription } from '@/types'
import { PAYMENT_RECOVERY_STORAGE_KEY } from '@/components/payment/paymentFlow'
enableAutoUnmount(afterEach)
const mocks = vi.hoisted(() => ({ catalog: vi.fn(), quote: vi.fn(), push: vi.fn(), replace: vi.fn(), fetchSubscriptions: vi.fn(), activeSubscriptions: [] as UserSubscription[], auth: { isAuthenticated: false, user: null as { id: number } | null }, route: { query: {} as Record<string,string>, hash: '' }, locale: 'zh' }))
vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ locale: { value: mocks.locale }, t: (key: string, args: Record<string,unknown> = {}) => {
  let value: unknown = mocks.locale === 'zh' ? zh : en
  for (const part of key.split('.')) value = value && typeof value === 'object' ? (value as Record<string,unknown>)[part] : undefined
  return typeof value === 'string' ? value.replace(/\{(\w+)\}/g, (_, name) => String(args[name] ?? `{${name}}`)) : key
} }) }))
vi.mock('@/api/presale' , () => ({ presaleAPI: { catalog: mocks.catalog, quote: mocks.quote } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ siteName: 'Café Shop', docUrl: 'https://docs.example.com/guide', publicSettingsLoaded: true }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => reactive(mocks.auth) }))
vi.mock('@/stores/subscriptions', () => ({ useSubscriptionStore: () => ({ activeSubscriptions: mocks.activeSubscriptions, fetchActiveSubscriptions: mocks.fetchSubscriptions }) }))
vi.mock('vue-router', () => ({ useRoute: () => mocks.route, useRouter: () => ({ push: mocks.push, replace: mocks.replace }) }))
vi.mock('@/views/user/PaymentView.vue', () => ({ default: { props: ['presalePlanId','presaleMonth','presaleMultiplier'], template: '<div class="checkout-fixture" :data-multiplier="presaleMultiplier">{{ presalePlanId }} / {{ presaleMonth }}</div>' } }))
const fixture = () => ({ enabled: true, server_time: '2026-09-23T00:00:00Z', period: { month: '2026-10', timezone: 'Asia/Shanghai', starts_at: '2026-10-01T00:00:00+08:00', expires_at: '2026-11-01T00:00:00+08:00', full_refund_before: '2026-09-28T00:00:00+08:00' }, plans: [{ id: 12, group_id: 3, group_platform: 'openai', name: 'Astra Monthly', description: 'Focus for a month', price: 300, concurrency: 2, features: ['Codex + Pi'], presale_enabled: true, presale_reset_cards: 2 }] })
function render(locale = 'zh') {
  mocks.locale = locale
  return mount(PresaleView, { global: { stubs: { RouterLink: { props: ['to'], template: '<a :href="typeof to === \'string\' ? to : to.path"><slot /></a>' }, LocaleSwitcher: true, Icon: true, BaseDialog: { name: 'BaseDialog', props: ['show'], template: '<div v-if="show"><slot /></div>' } } } })
}
beforeEach(() => { vi.clearAllMocks(); localStorage.removeItem(PAYMENT_RECOVERY_STORAGE_KEY); mocks.activeSubscriptions.length = 0; mocks.fetchSubscriptions.mockResolvedValue([]); mocks.auth.isAuthenticated = false; mocks.auth.user = null; mocks.route.query = {}; mocks.route.hash = ''; mocks.catalog.mockResolvedValue({ data: fixture() }); mocks.quote.mockReset().mockResolvedValue({ data: { ...fixture().period, plan_id: 12, renewal: false } }) })
describe('presale landing', () => {
  it('keeps reservation disabled until the authoritative check finishes, including login-return links', async () => {
    mocks.auth.isAuthenticated = true
    mocks.route.query = { plan: '12' }
    let finish!: (value: unknown) => void
    mocks.quote.mockImplementation(() => new Promise(resolve => { finish = resolve }))
    const w = render(); await flushPromises()
    expect(mocks.quote).toHaveBeenCalledWith(12)
    expect(w.get('.plan-buy').attributes('disabled')).toBeDefined()
    expect(w.get('.plan-buy').attributes('aria-busy')).toBe('true')
    expect(w.get('.plan-buy').attributes('aria-label')).toBe(zh.common.loading)
    expect(w.get('.plan-buy').classes()).toContain('is-checking')
    expect(w.get('.plan-buy-label').attributes('aria-hidden')).toBe('true')
    expect(w.get('.plan-buy-loader').attributes('aria-hidden')).toBe('true')
    expect(w.getComponent(LoadingSpinner).props()).toMatchObject({
      variant: 'steam', color: 'current', overlay: true, decorative: true
    })
    expect(w.text()).not.toContain('正在确认预订状态')
    await w.get('.plan-buy').trigger('click')
    expect(w.find('.checkout-fixture').exists()).toBe(false)
    expect(mocks.replace).not.toHaveBeenCalled()
    finish({ data: { ...fixture().period, plan_id: 12 } }); await flushPromises()
    expect(w.get('.plan-buy').attributes('disabled')).toBeUndefined()
    expect(w.get('.plan-buy').attributes('aria-busy')).toBeUndefined()
    await vi.waitFor(() => expect(w.find('.plan-buy-loader').exists()).toBe(false))
    expect(w.get('.plan-buy').text()).toBe('预定')
    expect(w.find('.checkout-fixture').exists()).toBe(true)
    w.unmount()
  })
  it('uses a graphic-only loading state with an accessible name in English', async () => {
    mocks.auth.isAuthenticated = true
    mocks.quote.mockReturnValue(new Promise(() => {}))
    const w = render('en'); await flushPromises()
    expect(w.get('.plan-buy').attributes('aria-label')).toBe(en.common.loading)
    expect(w.get('.plan-buy').attributes('aria-busy')).toBe('true')
    expect(w.getComponent(LoadingSpinner).props('variant')).toBe('steam')
    expect(w.text()).not.toContain('Checking reservation status')
    expect(w.find('.plan-availability').exists()).toBe(false)
    w.unmount()
  })
  it.each([
    ['COMPLETED', 'reserved', '/subscriptions', '本期已预订'],
    ['PAID', 'reserved', '/subscriptions', '本期已预订'],
    ['RECHARGING', 'reserved', '/subscriptions', '本期已预订'],
    ['FAILED', 'reserved', '/subscriptions', '本期已预订'],
    ['PENDING', 'pending', '/orders', '本期订单待付款'],
    ['REFUND_REQUESTED', 'refund', '/subscriptions', '本期退款待处理'],
    ['REFUNDING', 'refund', '/subscriptions', '本期退款待处理'],
    ['REFUND_FAILED', 'refund', '/subscriptions', '本期退款待处理'],
    [undefined, 'existing', '/orders', '本期已有订单'],
  ])('shows %s on the card before a click instead of opening an invalid checkout', async (status, kind, path, title) => {
    mocks.auth.isAuthenticated = true
    mocks.route.query = { plan: '12' }
    mocks.quote.mockRejectedValue({ reason: 'PRESALE_ALREADY_RESERVED', metadata: { order_status: status } })
    const w = render(); await flushPromises()
    expect(w.get('.plan-availability').attributes('data-state')).toBe(kind)
    expect(w.get('.plan-availability strong').text()).toBe(title)
    expect(w.get('.plan-buy').element.tagName).toBe('A')
    expect(w.get('.plan-buy').attributes('href')).toBe(path)
    expect(w.find('.checkout-fixture').exists()).toBe(false)
    expect(mocks.replace).not.toHaveBeenCalled()
    w.unmount()
  })
  it('shows a readable English reservation state', async () => {
    mocks.auth.isAuthenticated = true
    mocks.quote.mockRejectedValue({ reason: 'PRESALE_ALREADY_RESERVED', metadata: { order_status: 'COMPLETED' } })
    const w = render('en'); await flushPromises()
    expect(w.get('.plan-availability').text()).toContain('Already reserved')
    expect(w.get('.plan-availability').text()).toContain('October 2026')
    expect(w.get('.plan-buy').text()).toBe(en.presale.viewSubscriptions)
    w.unmount()
  })
  it.each(['PRESALE_COVERAGE_OVERLAP', 'PRESALE_LEGACY_SUBSCRIPTION', 'PRESALE_NOT_AVAILABLE', 'PRESALE_RENEWAL_TOO_EARLY'])('explains %s inline without offering checkout', async reason => {
    mocks.auth.isAuthenticated = true
    mocks.quote.mockRejectedValue({ reason })
    const w = render(); await flushPromises()
    expect(w.get('.plan-availability').attributes('data-state')).toBe('blocked')
    expect(w.get('.plan-buy').attributes('disabled')).toBeDefined()
    expect(w.get('.plan-availability p').text()).toBe(zh.presale.errors[reason as keyof typeof zh.presale.errors])
    w.unmount()
  })
  it.each(['zh', 'en'])('shows the renewal opening date before checkout in %s and refreshes eligibility on return', async locale => {
    mocks.auth.isAuthenticated = true
    mocks.route.query = { plan: '12' }
    mocks.quote.mockRejectedValue({ reason: 'PRESALE_RENEWAL_TOO_EARLY', metadata: { renewal_opens_at: '2026-09-15T00:00:00+08:00' } })
    const w = render(locale); await flushPromises()
    expect(w.get('.plan-availability').attributes('data-state')).toBe('blocked')
    expect(w.get('.plan-availability p').text()).toContain('14')
    expect(w.get('.plan-availability p').text()).toContain(locale === 'zh' ? '2026/09/15 00:00' : '15/09/2026, 00:00')
    expect(w.get('.plan-buy').attributes('disabled')).toBeDefined()
    await w.get('.plan-buy').trigger('click')
    expect(w.find('.checkout-fixture').exists()).toBe(false)
    expect(mocks.replace).not.toHaveBeenCalled()
    mocks.quote.mockResolvedValue({ data: { ...fixture().period, plan_id: 12, renewal: true } })
    window.dispatchEvent(new Event('focus')); await flushPromises()
    expect(w.find('.plan-availability').exists()).toBe(false)
    expect(w.get('.plan-buy').attributes('disabled')).toBeUndefined()
    await w.get('.plan-buy').trigger('click'); await flushPromises()
    expect(w.find('.checkout-fixture').exists()).toBe(true)
    w.unmount()
  })
  it('does not treat a failed eligibility request as permission to reserve and allows retry', async () => {
    mocks.auth.isAuthenticated = true
    mocks.quote.mockRejectedValueOnce(new Error('offline'))
    const w = render(); await flushPromises()
    expect(w.get('.plan-availability').attributes('data-state')).toBe('error')
    expect(w.get('.plan-buy').text()).toBe('重新检查')
    expect(w.find('.checkout-fixture').exists()).toBe(false)
    await w.get('.plan-buy').trigger('click'); await flushPromises()
    expect(w.find('.plan-availability').exists()).toBe(false)
    expect(w.get('.plan-buy').text()).toBe('预定')
    w.unmount()
  })
  it('refuses a quote for a different month and refreshes the catalog on retry', async () => {
    mocks.auth.isAuthenticated = true
    mocks.route.query = { plan: '12' }
    mocks.quote.mockResolvedValueOnce({ data: { ...fixture().period, month: '2026-11' } })
    const w = render(); await flushPromises()
    expect(w.find('.checkout-fixture').exists()).toBe(false)
    expect(w.get('.plan-availability p').text()).toContain('月份已经变化')
    await w.get('.plan-buy').trigger('click'); await flushPromises()
    expect(mocks.catalog).toHaveBeenCalledTimes(2)
    expect(w.find('.checkout-fixture').exists()).toBe(true)
    w.unmount()
  })
  it('checks all plans including siblings in the same group without blocking other groups', async () => {
    mocks.auth.isAuthenticated = true
    const data = fixture()
    data.plans.push({ ...data.plans[0], id: 13 }, { ...data.plans[0], id: 14, group_id: 4 })
    mocks.catalog.mockResolvedValue({ data })
    mocks.quote.mockImplementation((id: number) => id === 14
      ? Promise.resolve({ data: { ...data.period, plan_id: id } })
      : Promise.reject({ reason: 'PRESALE_ALREADY_RESERVED', metadata: { order_status: 'COMPLETED' } }))
    const w = render(); await flushPromises()
    expect(mocks.quote.mock.calls.map(args => args[0])).toEqual([12, 13, 14])
    expect(w.findAll('.plan-availability')).toHaveLength(2)
    expect(w.findAll('.plan-buy').map(button => button.element.tagName)).toEqual(['A', 'A', 'BUTTON'])
    w.unmount()
  })
  it('refreshes after checkout closes so a newly created order immediately replaces Reserve', async () => {
    mocks.auth.isAuthenticated = true
    const w = render(); await flushPromises()
    await w.get('.plan-buy').trigger('click')
    mocks.quote.mockRejectedValue({ reason: 'PRESALE_ALREADY_RESERVED', metadata: { order_status: 'PENDING' } })
    w.getComponent({ name: 'BaseDialog' }).vm.$emit('close'); await flushPromises()
    expect(w.find('.checkout-fixture').exists()).toBe(false)
    expect(w.get('.plan-availability').attributes('data-state')).toBe('pending')
    expect(mocks.quote).toHaveBeenCalledTimes(2)
    w.unmount()
  })
  it('refreshes after returning from another tab and ignores an older in-flight result', async () => {
    mocks.auth.isAuthenticated = true
    let finishOld!: (value: unknown) => void
    mocks.quote.mockImplementationOnce(() => new Promise(resolve => { finishOld = resolve }))
    const w = render(); await flushPromises()
    mocks.quote.mockRejectedValue({ reason: 'PRESALE_ALREADY_RESERVED', metadata: { order_status: 'COMPLETED' } })
    window.dispatchEvent(new Event('focus')); await flushPromises()
    expect(w.get('.plan-availability').attributes('data-state')).toBe('reserved')
    finishOld({ data: { ...fixture().period } }); await flushPromises()
    expect(w.get('.plan-availability').attributes('data-state')).toBe('reserved')
    w.unmount()
    window.dispatchEvent(new Event('focus')); await flushPromises()
    expect(mocks.quote).toHaveBeenCalledTimes(2)
  })
  it('does not check personal eligibility for logged-out visitors', async () => {
    const w = render(); await flushPromises()
    expect(mocks.quote).not.toHaveBeenCalled()
    w.unmount()
  })
  it('discards a previous account check after switching users', async () => {
    mocks.auth.isAuthenticated = true; mocks.auth.user = { id: 1 }
    let rejectOld!: (reason: unknown) => void
    mocks.quote.mockImplementationOnce(() => new Promise((_, reject) => { rejectOld = reject }))
    const w = render(); await flushPromises()
    reactive(mocks.auth).user = { id: 2 }; await flushPromises()
    expect(w.get('.plan-buy').text()).toBe('预定')
    rejectOld({ reason: 'PRESALE_ALREADY_RESERVED', metadata: { order_status: 'COMPLETED' } }); await flushPromises()
    expect(w.find('.plan-availability').exists()).toBe(false)
    expect(w.get('.plan-buy').attributes('disabled')).toBeUndefined()
    expect(mocks.quote).toHaveBeenCalledTimes(2)
    w.unmount()
  })
  it('removes personal reservation state on logout without issuing another private request', async () => {
    mocks.auth.isAuthenticated = true
    mocks.quote.mockRejectedValue({ reason: 'PRESALE_ALREADY_RESERVED', metadata: { order_status: 'COMPLETED' } })
    const w = render(); await flushPromises()
    expect(w.find('.plan-availability').exists()).toBe(true)
    reactive(mocks.auth).isAuthenticated = false; await flushPromises()
    expect(w.find('.plan-availability').exists()).toBe(false)
    expect(w.get('.plan-buy').text()).toBe('登录并预订')
    expect(mocks.quote).toHaveBeenCalledTimes(1)
    w.unmount()
  })
  it('keeps multiplier controls disabled on an already reserved plan', async () => {
    mocks.auth.isAuthenticated = true
    mocks.catalog.mockResolvedValue({ data: customFixture() })
    mocks.quote.mockRejectedValue({ reason: 'PRESALE_ALREADY_RESERVED', metadata: { order_status: 'COMPLETED' } })
    const w = render(); await flushPromises()
    expect(w.getComponent(Select).props('disabled')).toBe(true)
    expect(w.find('.checkout-fixture').exists()).toBe(false)
    w.unmount()
  })
  it('preserves provider-return handling rather than treating it as a new reservation', async () => {
    mocks.auth.isAuthenticated = true
    mocks.route.query = { plan: '12', wechat_resume_token: 'server-validated-in-checkout' }
    mocks.quote.mockRejectedValue({ reason: 'PRESALE_ALREADY_RESERVED', metadata: { order_status: 'PENDING' } })
    const w = render(); await flushPromises()
    expect(w.find('.checkout-fixture').exists()).toBe(true)
    expect(w.get('.plan-availability').attributes('data-state')).toBe('pending')
    w.unmount()
  })
  it.each([
    [123, 12, 'subscription', '2099-01-01T00:00:00Z', true],
    [999, 12, 'subscription', '2099-01-01T00:00:00Z', false],
    [123, 99, 'subscription', '2099-01-01T00:00:00Z', false],
    [123, 12, 'balance', '2099-01-01T00:00:00Z', false],
    [123, 12, 'subscription', '2000-01-01T00:00:00Z', false],
  ])('only resumes recovery for the server-confirmed pending order and plan: %s/%s/%s/%s', async (orderId, planId, orderType, expiresAt, resumable) => {
    mocks.auth.isAuthenticated = true
    localStorage.setItem(PAYMENT_RECOVERY_STORAGE_KEY, JSON.stringify({
      orderId, orderType, expiresAt, amount: 300, qrCode: 'weixin://test', paymentType: 'wxpay', payUrl: '',
      clientSecret: '', payAmount: 300, paymentMode: 'qrcode', resumeToken: 'test-only', createdAt: Date.now(),
    }))
    mocks.quote.mockRejectedValue({ reason: 'PRESALE_ALREADY_RESERVED', metadata: { order_status: 'PENDING', order_id: '123', order_plan_id: String(planId) } })
    const w = render(); await flushPromises()
    if (resumable) {
      expect(w.get('.plan-buy').text()).toBe('继续付款')
      await w.get('.plan-buy').trigger('click'); await flushPromises()
      expect(w.find('.checkout-fixture').exists()).toBe(true)
    } else {
      expect(w.get('.plan-buy').attributes('href')).toBe('/orders')
      expect(w.find('.checkout-fixture').exists()).toBe(false)
    }
    w.unmount()
  })
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
  it.each([[2, 4], [4, 2]])('lets an active %sx subscriber reserve %sx for next month', async (current, selected) => {
    mocks.auth.isAuthenticated = true
    mocks.activeSubscriptions.push({ status: 'active', expires_at: '2099-01-01T00:00:00Z', custom_source_plan_id: 12, custom_multiplier: current } as UserSubscription)
    mocks.catalog.mockResolvedValue({ data: customFixture() })
    const w = render(); await flushPromises()
    const select = w.getComponent(Select)
    expect(select.props('disabled')).toBe(false)
    expect(select.props('modelValue')).toBe(current)
    expect(select.props('options')).toEqual([2, 3, 4].map(value => ({ value, label: `${value}x` })))
    select.vm.$emit('update:modelValue', selected); await flushPromises()
    expect(w.get('.plan-price strong').text()).toBe(`￥${selected}`)
    expect(w.get('.plan-quotas').text()).toContain(`$${selected * 10}`)
    await w.get('.plan-buy').trigger('click')
    expect(w.get('.checkout-fixture').attributes('data-multiplier')).toBe(String(selected))
    expect(mocks.replace).toHaveBeenCalledWith({ path: '/presale', query: { plan: '12', multiplier: String(selected) }, hash: '' })
    expect(mocks.activeSubscriptions[0].custom_multiplier).toBe(current)
    w.unmount()
  })
  it('uses the current plan choices when a previous multiplier is no longer sold', async () => {
    mocks.auth.isAuthenticated = true
    mocks.activeSubscriptions.push({ status: 'active', expires_at: '2099-01-01T00:00:00Z', custom_source_plan_id: 12, custom_multiplier: 5 } as UserSubscription)
    mocks.route.query = { plan: '12' }
    mocks.catalog.mockResolvedValue({ data: customFixture() })
    const w = render(); await flushPromises()
    expect(w.getComponent(Select).props('disabled')).toBe(false)
    expect(w.getComponent(Select).props('options')).toEqual([2, 3, 4].map(value => ({ value, label: `${value}x` })))
    expect(w.get('.checkout-fixture').attributes('data-multiplier')).toBe('4')
    expect(w.get('.plan-price strong').text()).toBe('￥4')
    w.unmount()
  })
  it('uses 1x for the next term when customization has been disabled', async () => {
    mocks.auth.isAuthenticated = true
    mocks.activeSubscriptions.push({ status: 'active', expires_at: '2099-01-01T00:00:00Z', custom_source_plan_id: 12, custom_multiplier: 3 } as UserSubscription)
    mocks.route.query = { plan: '12', multiplier: '3' }
    const w = render(); await flushPromises()
    expect(w.findComponent(Select).exists()).toBe(false)
    expect(w.get('.checkout-fixture').attributes('data-multiplier')).toBe('1')
    expect(w.get('.plan-price strong').text()).toBe('￥300')
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
    expect(w.text()).not.toContain('北京时间'); expect(w.findAll('.presale-plan')).toHaveLength(1)
    expect(w.get('.plan-price strong').text()).toBe('￥300')
    expect(w.find('.old-price').exists()).toBe(false)
    expect(w.find('.checkout-fixture').exists()).toBe(false)
    await w.get('.plan-buy').trigger('click')
    expect(mocks.push).toHaveBeenCalledWith({ path: '/login', query: { redirect: '/presale?plan=12' } })
    w.unmount()
  })
  it.each(['zh', 'en'])('uses concise subscription copy without timezone captions or fixed reset gifts in %s', async locale => {
    const w = render(locale); await flushPromises()
    const copy = locale === 'zh' ? zh.presale : en.presale
    expect(w.get('.plans-section h2').text()).toBe(copy.plansTitle)
    expect(w.get('.plans-section .eyebrow').text()).toBe('Subscription plans')
    expect(w.find('.timezone').exists()).toBe(false)
    expect(w.text()).not.toMatch(/UTC\+8|Beijing time|北京时间/)
    expect(w.findAll('.plan-features li')).toHaveLength(2) // Concurrency + configured feature, even with old reset metadata.
    const benefits = w.findAll('.benefit-item')
    expect(benefits[0].text()).toContain(locale === 'zh' ? '套餐随官方发放重置卡' : 'when the official provider issues them')
    expect(benefits[2].text()).toContain(locale === 'zh' ? '提供按天退款' : 'Daily prorated refunds')
    expect(benefits[2].text()).not.toContain('20%')
    expect(w.get('.policy-section').text()).toContain('20%') // Detailed policy remains unchanged.
    expect(w.get('footer').text()).toBe('Café Shop')
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
