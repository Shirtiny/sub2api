import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent } from 'vue'
import HomeWaitlistDialog from '../HomeWaitlistDialog.vue'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'

const state = vi.hoisted(() => ({
  locale: 'zh', join: vi.fn(), reset: vi.fn(), app: { cachedPublicSettings: { turnstile_enabled: false, turnstile_site_key: '' } }
}))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => {
    let value: unknown = state.locale === 'zh' ? zh : en
    for (const part of key.split('.')) {
      value = value && typeof value === 'object' ? (value as Record<string, unknown>)[part] : undefined
    }
    return typeof value === 'string' ? value : key
  } })
}))
vi.mock('@/api/waitlist', () => ({ joinWaitlist: state.join }))
vi.mock('@/stores', () => ({ useAppStore: () => state.app }))
const Challenge = defineComponent({
  name: 'TurnstileWidget', props: ['siteKey', 'theme'], emits: ['verify', 'expire', 'error'],
  setup(_, { expose }) { expose({ reset: state.reset }); return {} }, template: '<div class="challenge" />'
})
let page: VueWrapper
function render(locale = 'zh') {
  state.locale = locale
  page = mount(HomeWaitlistDialog, { props: { isDark: true }, attachTo: document.body,
    global: { stubs: { teleport: true, TurnstileWidget: Challenge } }
  })
  return page
}
beforeEach(() => {
  vi.clearAllMocks()
  state.app.cachedPublicSettings = { turnstile_enabled: false, turnstile_site_key: '' }
  state.join.mockResolvedValue(undefined)
})
afterEach(() => { page?.unmount(); document.body.innerHTML = '' })

describe('waiting-list dialog', () => {
  it.each([
    ['zh', '加入候补名单'],
    ['en', 'Join waiting list'],
  ])('localizes the submit button (%s)', (locale, label) => {
    render(locale)
    expect(page.get('button[type="submit"]').text()).toBe(label)
  })
  it.each(['zh', 'en'])('keeps the success icon beside localized copy and the close action below (%s)', async locale => {
    render(locale)
    await page.get('input').setValue('person@example.com')
    await page.get('form').trigger('submit')
    await flushPromises()
    const status = page.get('[role="status"]')
    expect(status.classes()).toEqual(expect.arrayContaining(['flex', 'items-center', 'gap-3']))
    expect(status.element.children).toHaveLength(2)
    expect(status.get('[aria-hidden="true"]').classes()).toContain('shrink-0')
    expect(status.get('p').text()).toBe((locale === 'zh' ? zh : en).home.landing.waitlist.success)
    expect(status.find('button').exists()).toBe(false)
    const closeButton = page.get('.modal-body button')
    expect(status.element.nextElementSibling).toBe(closeButton.element)
    await closeButton.trigger('click')
    expect(page.emitted('close')).toHaveLength(1)
  })
  it('focuses a required email input and rejects invalid syntax without a request', async () => {
    render(); await flushPromises()
    const input = page.get('input')
    expect(input.attributes('type')).toBe('email')
    expect(input.attributes('required')).toBeDefined()
    expect(document.activeElement).toBe(input.element)
    await input.setValue('not-an-email')
    await page.get('form').trigger('submit')
    expect(state.join).not.toHaveBeenCalled()
    expect(page.get('[role="alert"]').text()).toContain('格式正确')
  })
  it('normalizes an email and shows success only after the server confirms it', async () => {
    let resolve!: () => void
    state.join.mockReturnValue(new Promise<void>(done => { resolve = done }))
    render()
    await page.get('input').setValue(' Person+launch@Example.com ')
    await page.get('form').trigger('submit')
    await page.get('form').trigger('submit')
    expect(state.join).toHaveBeenCalledOnce()
    expect(state.join).toHaveBeenCalledWith('person+launch@example.com', '')
    expect(page.find('[role="status"]').exists()).toBe(false)
    expect(page.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    resolve(); await flushPromises()
    expect(page.get('[role="status"]').text()).toContain('已加入候补名单')
    expect(page.find('input').exists()).toBe(false)
    await page.get('.modal-body button').trigger('click')
    expect(page.emitted('close')).toHaveLength(1)
  })
  it('keeps the form and email on a server error so the visitor can retry', async () => {
    state.join.mockRejectedValue({ status: 503 })
    render(); await page.get('input').setValue('a@example.com')
    await page.get('form').trigger('submit'); await flushPromises()
    expect(page.find('[role="status"]').exists()).toBe(false)
    expect(page.get('[role="alert"]').text()).toContain('暂时无法提交')
    expect((page.get('input').element as HTMLInputElement).value).toBe('a@example.com')
    expect(page.get('button[type="submit"]').attributes('disabled')).toBeUndefined()
  })
  it('explains that an application survives an email failure and can be retried', async () => {
    state.join.mockRejectedValueOnce({ status: 503, reason: 'WAITLIST_CONFIRMATION_FAILED' })
    render(); await page.get('input').setValue('a@example.com')
    await page.get('form').trigger('submit'); await flushPromises()
    expect(page.get('[role="alert"]').text()).toContain('申请已保存')
    expect(page.get('[role="alert"]').text()).toContain('确认邮件')
    expect(page.find('[role="status"]').exists()).toBe(false)
    await page.get('form').trigger('submit'); await flushPromises()
    expect(page.get('[role="status"]').text()).toContain('已加入候补名单')
  })
  it('shows a useful rate-limit message in English', async () => {
    state.join.mockRejectedValue({ status: 429 })
    render('en'); await page.get('input').setValue('a@example.com')
    await page.get('form').trigger('submit'); await flushPromises()
    expect(page.get('[role="alert"]').text()).toContain('Too many submissions')
  })
  it('requires and submits the configured challenge, resetting it on failure', async () => {
    state.app.cachedPublicSettings = { turnstile_enabled: true, turnstile_site_key: 'site-key' }
    state.join.mockRejectedValue({ status: 400 })
    render(); await page.get('input').setValue('a@example.com')
    await page.get('form').trigger('submit')
    expect(state.join).not.toHaveBeenCalled()
    const challenge = page.getComponent(Challenge)
    expect(challenge.props('theme')).toBe('dark')
    challenge.vm.$emit('verify', 'proof')
    await page.get('form').trigger('submit'); await flushPromises()
    expect(state.join).toHaveBeenCalledWith('a@example.com', 'proof')
    expect(state.reset).toHaveBeenCalledOnce()
    expect(page.get('button[type="submit"]').attributes('disabled')).toBeDefined()
  })
})
