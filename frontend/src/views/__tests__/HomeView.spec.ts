import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, nextTick, reactive } from 'vue'
import type { PublicSettings } from '@/types'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'
import HomeView from '../HomeView.vue'
import homeSource from '../HomeView.vue?raw'
import Icon from '@/components/icons/Icon.vue'

const state = vi.hoisted(() => ({
  locale: 'zh' as 'zh' | 'en',
  app: {
    cachedPublicSettings: null as Partial<PublicSettings> | null,
    siteName: 'Sub2API',
    siteLogo: '',
    docUrl: '',
    publicSettingsLoaded: true,
    fetchPublicSettings: vi.fn()
  },
  auth: { isAuthenticated: false, isAdmin: false, checkAuth: vi.fn() }
}))

// Vitest uses the runtime-only i18n build; resolve the real locale strings here.
// The isolated browser checks exercise the actual JIT-enabled application build.
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({
    t: (key: string) => {
      let value: unknown = state.locale === 'zh' ? zh : en
      for (const part of key.split('.')) {
        value = value && typeof value === 'object' ? (value as Record<string, unknown>)[part] : undefined
      }
      return typeof value === 'string' ? value : key
    }
  })
}))

vi.mock('@/stores', async () => {
  const { reactive } = await import('vue')
  return {
    useAppStore: () => reactive(state.app),
    useAuthStore: () => reactive(state.auth)
  }
})

const RouterLinkStub = defineComponent({
  props: { to: { type: String, required: true } },
  template: '<a :href="to"><slot /></a>'
})

let wrapper: VueWrapper | undefined

function render(locale: 'zh' | 'en' = 'zh') {
  state.locale = locale
  wrapper = mount(HomeView, {
    global: {
      stubs: { RouterLink: RouterLinkStub, LocaleSwitcher: true }
    }
  })
  return wrapper
}

beforeEach(() => {
  vi.clearAllMocks()
  Object.assign(state.app, {
    cachedPublicSettings: null, siteName: 'Sub2API', siteLogo: '',
    docUrl: '', publicSettingsLoaded: true
  })
  Object.assign(state.auth, { isAuthenticated: false, isAdmin: false })
  document.documentElement.classList.remove('dark')
  localStorage.clear()
})

afterEach(() => {
  wrapper?.unmount()
  document.documentElement.classList.remove('dark')
  localStorage.clear()
})

describe('visitor-first homepage', () => {
  it('shows model availability in its own slide immediately after the everyday features', () => {
    const page = render()
    const models = page.find('#supported-models')
    expect(models.element.closest('[data-home-slide]')?.previousElementSibling?.getAttribute('data-home-slide')).toBe('possibilities')
    expect(models.findAll('.model-name').map(name => name.text())).toEqual(['ChatGPT'])
    expect(models.findAll('[data-model]')).toHaveLength(1)
    expect(models.find('[data-model]').attributes('data-model')).toBe('chatgpt')
    expect(models.find('.models-description, button').exists()).toBe(false)
  })

  it('gives each section a complete slide and a labelled chapter control', () => {
    const page = render()
    expect(page.findAll('[data-home-slide]').map(slide => slide.attributes('data-home-slide'))).toEqual([
      'welcome', 'possibilities', 'supported-models', 'billing', 'questions'
    ])
    expect(page.findAll('.chapter-stop')).toHaveLength(5)
    expect(page.find('.chapter-nav').attributes('aria-label')).toBe('首页章节')
    for (const button of page.findAll('.chapter-stop')) {
      expect(page.find(`#${button.attributes('aria-controls')}`).exists()).toBe(true)
    }
    expect(page.find('[data-home-slide="questions"] .home-footer').exists()).toBe(true)
    expect(page.find('[data-home-slide="next-step"]').exists()).toBe(false)
  })

  it('stages headings, copy and cards individually without nested reveal transforms', () => {
    const page = render()
    for (const slide of page.findAll('[data-home-slide]')) {
      expect(slide.findAll('[data-reveal]').length).toBeGreaterThanOrEqual(4)
    }
    expect(page.findAll('[data-reveal] [data-reveal]')).toHaveLength(0)
    expect(page.findAll('.feature').map(card => (card.element as HTMLElement).style.getPropertyValue('--reveal-delay'))).toEqual([
      '300ms', '430ms', '560ms'
    ])
    expect(page.findAll('.model-group-stage').map(card => (card.element as HTMLElement).style.getPropertyValue('--reveal-delay'))).toEqual([
      '300ms', '420ms', '540ms', '660ms'
    ])
  })

  it('explains usage enforcement, permitted clients and request controls without duplicating billing', () => {
    const page = render()
    const items = page.findAll('#questions article')
    expect(items.map(item => item.find('h3').text())).toEqual(['使用规范', '客户端限制', '请求管控'])
    expect(page.find('#questions details').exists()).toBe(false)
    expect(page.find('.policy-warning').text()).toBe('违规封禁 · 不予退款')
    expect(page.findAll('.policy-clients img')).toHaveLength(2)
    expect(items[0].text()).toContain('封禁账号且不予退款')
    expect(items[1].text()).toContain('Codex 或 Pi')
    expect(items[2].find('.request-summary').text()).toContain('接入文档的格式要求')
    expect(items[2].findAll('dt').map(item => item.text())).toEqual(['普通请求内容', 'Cyber 及违规请求内容'])
    expect(items[2].findAll('dd').map(item => item.text())).toEqual(['仅保留 1 天', '永久保留'])
    expect(page.find('#questions .policy-billing-options').exists()).toBe(false)
  })

  it('moves the simplified first-sip guide ahead of the usage policies', () => {
    const page = render()
    const guide = page.find('#questions #getting-started')
    expect(guide.text()).toContain('Your first sip')
    expect(guide.find('.quick-guide-title').text()).toContain('接入指南')
    expect(guide.find('.quick-guide-header').element.nextElementSibling).toBe(guide.find('.quick-steps').element)
    expect(guide.element.nextElementSibling).toBe(page.find('.policy-notice').element)
    expect(guide.findAll('.quick-steps li')).toHaveLength(3)
    expect(guide.text()).toContain('接入 Codex 或 Pi')
    expect(guide.find('a').attributes('href')).toBe('/login')
    expect(page.findAll('#getting-started')).toHaveLength(1)
    expect(page.find('[data-home-slide="getting-started"]').exists()).toBe(false)
  })

  it('dedicates the former guide slide to detailed subscription and balance information', () => {
    const page = render()
    const billing = page.find('[data-home-slide="billing"]')
    expect(billing.element.previousElementSibling).toBe(page.find('[data-home-slide="supported-models"]').element)
    expect(billing.element.nextElementSibling).toBe(page.find('[data-home-slide="questions"]').element)
    expect(billing.findAll('article')).toHaveLength(2)
    expect(billing.findAll('.billing-facts > div')).toHaveLength(4)
    expect(billing.text()).toContain('每次限购一个月')
    expect(billing.text()).toContain('月初开始')
    expect(billing.text()).toContain('当月月底结束')
    expect(billing.text()).toContain('余额支持即时充值')
    expect(billing.text()).toContain('随时使用')
    expect(billing.text()).not.toMatch(/Astra|0\.4|0\.5|倍率/)
    expect(billing.find('a').attributes('href')).toBe('/login')
  })

  it.each(['zh', 'en'] as const)('uses matching highlights, captions and two facts for both billing plans (%s)', locale => {
    const page = render(locale)
    const balance = page.find('[aria-labelledby="billing-balance-title"]')
    expect(balance.find('.billing-metric strong').text()).toBe(locale === 'zh' ? '按需' : 'Flexible')
    expect(balance.find('.billing-metric span').text()).toBe(locale === 'zh' ? '充值' : 'top-ups')
    expect(balance.find('.billing-caption').text()).toBe(locale === 'zh' ? '即时充值，随时使用' : 'Top up anytime. Use as needed.')
    expect(balance.findAll('.billing-facts dt').map(label => label.text())).toEqual(locale === 'zh' ? ['充值方式', '使用方式'] : ['Top-ups', 'Usage'])
    expect(balance.text()).not.toMatch(/Astra|0\.4|0\.5|×|倍率|multiplier|Group rate/)
    const subscription = page.find('[aria-labelledby="billing-subscription-title"]')
    expect(subscription.find('.billing-metric strong').text()).toBe(locale === 'zh' ? '按月' : 'Monthly')
    expect(subscription.find('.billing-metric span').text()).toBe(locale === 'zh' ? '订阅' : 'plan')
    expect(subscription.find('.billing-caption').text()).toBe(locale === 'zh' ? '每次限购一个月' : 'One month per purchase')
    expect(subscription.findAll('.billing-facts dt').map(label => label.text())).toEqual(locale === 'zh' ? ['获取方式', '使用周期'] : ['Availability', 'Service period'])
    expect(subscription.text()).not.toMatch(/价格优势|订阅价格更优惠|Pricing|lower price/)
    for (const card of [subscription, balance]) {
      expect(card.findAll('.billing-facts > div')).toHaveLength(2)
      expect([...card.element.children].map(child => child.className || child.tagName))
        .toEqual(['billing-option-heading', 'H3', 'billing-metric', 'billing-caption', 'billing-facts'])
    }
  })

  it.each(['zh', 'en'] as const)('states one-day audit retention and permanent exceptions explicitly (%s)', locale => {
    const page = render(locale)
    const rules = page.find('.policy-requests').text()
    expect(rules).toContain(locale === 'zh' ? '仅用于审计' : 'only for auditing')
    const periods = page.findAll('.request-retention > div')
    expect(periods).toHaveLength(2)
    expect(periods[0].find('dt').text()).toContain(locale === 'zh' ? '普通请求内容' : 'Ordinary request content')
    expect(periods[0].find('dd').text()).toBe(locale === 'zh' ? '仅保留 1 天' : '1 day only')
    expect(periods[1].find('dt').text()).toContain('Cyber')
    expect(periods[1].find('dt').text()).toContain(locale === 'zh' ? '违规请求内容' : 'rule-violating request content')
    expect(periods[1].find('dd').text()).toBe(locale === 'zh' ? '永久保留' : 'Retained permanently')
    expect(rules).not.toMatch(/7\s*(天|days)/)
    expect(page.text()).not.toContain('home.landing.')
  })

  it('uses the original feature cards as the only SVG selectors in one shared presentation', async () => {
    const page = render()
    const section = page.find('#possibilities')
    expect(section.findAll('.feature')).toHaveLength(3)
    expect(section.find('.feature-grid').element.nextElementSibling).toBe(section.find('.feature-presentation').element)
    expect(section.find('.feature-grid').element.parentElement).toBe(section.find('.feature-carousel').element)
    expect(section.findAll('.feature-selector')).toHaveLength(3)
    expect(section.find('.feature-grid p').exists()).toBe(false)
    expect(section.find('.feature-presentation .astra-signature').text()).toBe('ASTRA')
    expect(section.findAll('.feature-description')).toHaveLength(3)
    expect(section.find('.feature-controls, .feature-selectors').exists()).toBe(false)
    await section.findAll('.feature-selector')[2].trigger('click')
    expect(section.find('.feature-frame.is-current').attributes('data-feature')).toBe('trust')
    expect(section.findAll('.feature')[2].attributes('data-selected')).toBe('true')
    expect(section.findAll('svg.feature-scene')).toHaveLength(3)
    expect(section.find('.feature-presentation').attributes('data-reveal')).toBeDefined()
    expect(page.find('[data-home-slide="possibilities"]').element.nextElementSibling)
      .toBe(page.find('[data-home-slide="supported-models"]').element)
  })


  it('keeps the closing content and entry action below pricing without a separate chapter', () => {
    const page = render()
    const callout = page.find('#questions .closing')
    expect(callout.exists()).toBe(true)
    expect(callout.element.tagName).toBe('DIV')
    expect(page.find('#questions .policy-grid').element.nextElementSibling).toBe(callout.element)
    expect(callout.text()).toContain('Make room for an idea')
    expect(callout.find('#closing-title').text()).toBe('下一杯，留给你的新想法。')
    expect(callout.find('.cafe-button-primary').attributes('href')).toBe('/login')
    expect(page.find('#questions').element.nextElementSibling).toBe(page.find('.home-footer').element)
    expect(page.find('[aria-controls="next-step"]').exists()).toBe(false)
  })

  it('uses chip, lightning and verified shield icons for the three feature cards', () => {
    const page = render()
    expect(page.findAll('.feature').map(card => card.findComponent(Icon).props('name')))
      .toEqual(['cpu', 'bolt', 'shield'])
  })

  it('links the client cards to their official websites in a safe new tab', () => {
    const links = render().findAll('.policy-clients a')
    expect(links.map(link => link.attributes('href'))).toEqual(['https://openai.com/codex/', 'https://pi.dev/'])
    expect(links.map(link => link.text())).toEqual(['Codex', 'Pi'])
    for (const link of links) {
      expect(link.attributes('target')).toBe('_blank')
      expect(link.attributes('rel')).toBe('noopener noreferrer')
    }
  })

  it('keeps both client buttons in equal-width columns with centered content', () => {
    const group = homeSource.match(/\.policy-clients\s*\{([^}]+)\}/)?.[1]
    const button = homeSource.match(/\.policy-clients a\s*\{([^}]+)\}/)?.[1]
    expect(group).toContain('display: grid;')
    expect(group).toContain('grid-template-columns: repeat(2, minmax(0, 1fr));')
    expect(group).toContain('width: fit-content;')
    expect(group).toContain('max-width: 100%;')
    expect(button).toContain('justify-content: center;')
    expect(button).toContain('align-items: center;')
  })

  it('renders public sections without personal data or invented statistics', () => {
    const page = render()
    expect(page.find('h1').text()).toContain('让工作，多些从容。')
    expect(page.find('h1').text()).toContain('让灵感，高效落地。')
    expect(page.find('.hero-subtitle').exists()).toBe(false)
    expect(page.find('.hero-description').exists()).toBe(false)
    expect(page.findAll('.feature')).toHaveLength(3)
    expect(page.findAll('.quick-steps li')).toHaveLength(3)
    expect(page.findAll('#questions article')).toHaveLength(3)
    expect(page.text()).not.toContain('浏览首页需要登录吗？')
    expect(page.find('aside').exists()).toBe(false)
    expect(page.text()).not.toMatch(/账户余额|升级套餐|总请求数|平均耗时|免费试用|最新公告/)
    expect(page.find('.hero-actions a').attributes('href')).toBe('/login')
    expect(page.find('.header-entry').attributes('href')).toBe('/login')
    expect(page.find('.home-header nav').exists()).toBe(false)
    expect(page.find('.home-header a[href^="#"]').exists()).toBe(false)
    expect(state.app.fetchPublicSettings).not.toHaveBeenCalled()
    // Authentication restoration belongs to the router, not a public landing component.
    expect(state.auth.checkAuth).not.toHaveBeenCalled()
  })

  it.each([
    [false, '/dashboard'],
    [true, '/admin/dashboard']
  ])('sends an authenticated visitor (admin=%s) to %s', (isAdmin, path) => {
    Object.assign(state.auth, { isAuthenticated: true, isAdmin })
    const page = render()
    expect(page.find('.header-entry').attributes('href')).toBe(path)
    for (const link of page.findAll('.cafe-button-primary')) {
      expect(link.attributes('href')).toBe(path)
      expect(link.text()).toContain('进入控制台')
    }
    expect(page.find('a[href="/login"]').exists()).toBe(false)
  })

  it('uses the shared Sub2API primary-button treatment for homepage entry actions', () => {
    const page = render()
    const buttons = page.findAll('.cafe-button-primary')
    expect(buttons).toHaveLength(2)
    for (const button of buttons) {
      expect(button.classes()).toEqual(expect.arrayContaining(['btn', 'btn-primary']))
    }
  })

  it('uses configured branding and only exposes configured public links', () => {
    state.app.cachedPublicSettings = {
      site_name: 'My cafe', site_logo: '/custom-logo.png', site_subtitle: 'Custom welcome',
      doc_url: 'https://docs.example.com/guide', login_agreement_enabled: true,
      login_agreement_documents: [{ id: 'terms', title: 'Terms', content_md: 'Terms content' }]
    }
    const page = render()
    expect(page.find('.brand-name').text()).toBe('My cafe')
    expect(page.find('.brand').attributes('aria-label')).toBe('My cafe')
    expect(page.find('.brand img').exists()).toBe(false)
    expect(page.find('.home-footer').text()).toContain('My cafe')
    expect(page.find('.hero-subtitle').exists()).toBe(false)
    expect(page.find('.hero-description').exists()).toBe(false)
    expect(page.find('a[href="/legal/terms"]').text()).toBe('Terms')
    for (const link of page.findAll('a[target="_blank"]:not(.policy-clients a)')) {
      expect(link.attributes('href')).toBe('https://docs.example.com/guide')
      expect(link.attributes('rel')).toBe('noopener noreferrer')
    }
    expect(page.findAll('a[target="_blank"]:not(.policy-clients a)')).toHaveLength(4)
    expect(page.find('.quick-guide-header a').attributes('href')).toBe('https://docs.example.com/guide')
  })

  it('keeps both tools below the hero, separate from the stable site wordmark', () => {
    const page = render()
    expect(page.find('.hero').element.nextElementSibling?.classList.contains('tool-note')).toBe(true)
    expect(page.find('.tool-note').attributes('aria-label')).toBe('创作工具')
    expect(page.findAll('.tool-mark')).toHaveLength(2)
    expect(page.find('.tool-note').text()).toContain('Codex')
    expect(page.find('.tool-note').text()).toContain('pi')
    expect(page.find('.tool-icon-codex').attributes('src')).toContain('codex-mark.svg')
    expect(page.find('.tool-icon-pi').attributes('src')).toContain('pi-mark.svg')
    expect(page.find('.home-brand').text()).not.toMatch(/Codex|\bpi\b/)
    expect(page.find('.brand-tools').attributes('aria-label')).toBe('暂停 Codex / pi 图标轮换')
    expect(page.find('.brand-tools img, .brand-signature, .brand-note, .brand-rule').exists()).toBe(false)
    expect(page.find('.brand-tools svg animate').exists()).toBe(true)
    expect(page.findAll('.pi-tile')).toHaveLength(3)
  })

  it('anchors the coffee story to the configured Café Shop letters without a separate rotating logo', () => {
    state.app.cachedPublicSettings = { site_name: 'Café Shop' }
    const page = render()
    expect(page.find('.brand').attributes('aria-label')).toBe('Café Shop')
    expect(page.find('.story-stage .accent-steam').exists()).toBe(true)
    expect(page.find('.story-stage .accent-steam animate').exists()).toBe(true)
    expect(page.find('.story-umbrella, .story-drip, .p-to-pi, .pi-runoff').exists()).toBe(false)
    expect(page.find('.tool-stage, .brand-note, .brand-rule').exists()).toBe(false)
  })

  it('has no empty docs links or disabled legal documents', () => {
    state.app.cachedPublicSettings = {
      login_agreement_enabled: false,
      login_agreement_documents: [{ id: 'terms', title: 'Terms', content_md: '' }]
    }
    const page = render()
    expect(page.find('a[target="_blank"]:not(.policy-clients a)').exists()).toBe(false)
    expect(page.find('a[href="/legal/terms"]').exists()).toBe(false)
    for (const link of page.findAll('a[href^="#"]')) {
      expect(page.find(link.attributes('href')).exists()).toBe(true)
    }
  })

  it('does not render a logo and rejects unsafe documentation URLs', () => {
    state.app.cachedPublicSettings = { site_logo: 'javascript:alert(1)', doc_url: 'javascript:alert(1)' }
    const page = render()
    expect(page.find('.brand img').exists()).toBe(false)
    expect(page.find('a[target="_blank"]:not(.policy-clients a)').exists()).toBe(false)
  })

  it('fetches public settings when not already initialized', () => {
    state.app.publicSettingsLoaded = false
    render()
    expect(state.app.fetchPublicSettings).toHaveBeenCalledOnce()
  })

  it('uses the shared store name before public settings are loaded', () => {
    state.app.siteName = 'Configured gateway'
    const page = render()
    expect(page.find('.brand-name').text()).toBe('Configured gateway')
    expect(page.find('.home-footer').text()).toContain('Configured gateway')
    expect(page.text()).not.toContain('Cafe Code Work')
  })

  it('updates the header and footer when the backend site name becomes available', async () => {
    const page = render()
    expect(page.find('.brand-name').text()).toBe('Sub2API')
    reactive(state.app).cachedPublicSettings = { site_name: '后台配置的名称' }
    await nextTick()
    expect(page.find('.brand-name').text()).toBe('后台配置的名称')
    expect(page.find('.home-footer').text()).toContain('后台配置的名称')
    expect(page.find('.brand img').exists()).toBe(false)
  })

  it('falls back to the product name instead of a hard-coded cafe brand', () => {
    state.app.siteName = ''
    const page = render()
    expect(page.find('.brand-name').text()).toBe('Sub2API')
    expect(page.text()).not.toContain('Cafe Code Work')
  })

  it('preserves a custom HTML homepage instead of rendering the landing page', () => {
    state.app.cachedPublicSettings = { home_content: '<section id="custom-home">Custom page</section>' }
    const page = render()
    expect(page.find('#custom-home').text()).toBe('Custom page')
    expect(page.find('.cafe-home').exists()).toBe(false)
  })

  it('preserves a custom iframe homepage with an accessible title', () => {
    state.app.cachedPublicSettings = { site_name: 'Custom site', home_content: '  https://example.com/home  ' }
    const page = render()
    expect(page.find('iframe').attributes('src')).toBe('https://example.com/home')
    expect(page.find('iframe').attributes('title')).toBe('Custom site')
    expect(page.find('.cafe-home').exists()).toBe(false)
  })

  it('respects the existing theme and persists the theme toggle', async () => {
    document.documentElement.classList.add('dark')
    const page = render()
    expect(page.find('.theme-toggle').attributes('aria-label')).toBe('切换到浅色模式')
    await page.find('.theme-toggle').trigger('click')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(localStorage.getItem('theme')).toBe('light')
    await page.find('.theme-toggle').trigger('click')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(localStorage.getItem('theme')).toBe('dark')
  })

  it('does not force a light page back into dark mode', () => {
    const page = render()
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(page.find('.theme-toggle').attributes('aria-label')).toBe('切换到深色模式')
  })

  it('renders all landing copy in English without unresolved translation keys', async () => {
    const page = render('en')
    await nextTick()
    expect(page.find('h1').text()).toContain('Room for inspiration.')
    expect(page.find('h1').text()).toContain('AI for your everyday.')
    expect(page.text()).not.toContain('home.landing.')
    expect(page.text()).not.toMatch(/[\u4e00-\u9fff]/)
  })
})
