import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import RegisterView from '../RegisterView.vue'

const { push, register, settings, query, showError } = vi.hoisted(() => ({
  push: vi.fn(), register: vi.fn(), settings: vi.fn(), showError: vi.fn(), query: {} as Record<string, string>
}))
vi.mock('vue-router', () => ({ useRoute: () => ({ query }), useRouter: () => ({ push }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key, locale: { value: 'zh' } })
}))
vi.mock('@/stores', () => ({
  useAuthStore: () => ({ register }),
  useAppStore: () => ({ showError, showSuccess: vi.fn() })
}))
vi.mock('@/api/auth', async () => ({
  ...await vi.importActual<typeof import('@/api/auth')>('@/api/auth'),
  getPublicSettings: settings
}))
let page: VueWrapper
function render() {
  page = mount(RegisterView, { global: { stubs: {
    AuthLayout: { template: '<main><slot /></main>' },
    RouterLink: { props: ['to'], template: '<a :data-to="JSON.stringify(to)"><slot /></a>' },
    Icon: true, TurnstileWidget: true, LoginAgreementPrompt: true,
    LinuxDoOAuthSection: true, OidcOAuthSection: true, WechatOAuthSection: true, EmailOAuthButtons: true
  } } })
}
beforeEach(() => {
  vi.clearAllMocks(); sessionStorage.clear(); localStorage.clear()
  for (const key of Object.keys(query)) delete query[key]
  settings.mockResolvedValue({ registration_enabled: false, email_verify_enabled: false, invitation_code_enabled: true })
})
afterEach(() => { page?.unmount() })
describe('approved waiting-list signup entry', () => {
  it('keeps the ordinary closed-registration screen with a separate approval entry', async () => {
    render(); await flushPromises()
    expect(page.find('form').exists()).toBe(false)
    expect(page.text()).toContain('auth.registrationDisabled')
    expect(page.text()).toContain('auth.waitlistRegistrationLink')
    expect(page.get('a[data-to]').attributes('data-to')).toContain('waitlist')
  })
  it('always routes approved-entry signup through mailbox verification, not direct signup', async () => {
    query.waitlist = '1'
    render(); await flushPromises()
    expect(page.text()).toContain('auth.waitlistRegistrationHint')
    expect(page.find('#invitation_code').exists()).toBe(false)
    await page.get('#email').setValue('approved@example.com')
    await page.get('#password').setValue('test-password')
    await page.get('form').trigger('submit'); await flushPromises()
    expect(showError).not.toHaveBeenCalled()
    expect(register).not.toHaveBeenCalled()
    expect(push).toHaveBeenCalledWith('/email-verify')
    expect(JSON.parse(sessionStorage.getItem('register_data') || '{}')).toMatchObject({ email: 'approved@example.com' })
  })
  it('retains normal open signup behavior without a waitlist entry', async () => {
    settings.mockResolvedValue({ registration_enabled: true, email_verify_enabled: false, invitation_code_enabled: false })
    register.mockResolvedValue(undefined)
    render(); await flushPromises()
    await page.get('#email').setValue('normal@example.com')
    await page.get('#password').setValue('test-password')
    await page.get('form').trigger('submit'); await flushPromises()
    expect(register).toHaveBeenCalledOnce()
    expect(push).toHaveBeenCalledWith('/dashboard')
  })
  it('still respects email-domain restrictions in approval mode', async () => {
    query.waitlist = '1'
    settings.mockResolvedValue({ registration_enabled: false, email_verify_enabled: false, registration_email_suffix_whitelist: ['allowed.example'] })
    render(); await flushPromises()
    await page.get('#email').setValue('person@blocked.example')
    await page.get('#password').setValue('test-password')
    await page.get('form').trigger('submit'); await flushPromises()
    expect(showError).toHaveBeenCalled()
    expect(push).not.toHaveBeenCalled()
    expect(register).not.toHaveBeenCalled()
  })
})
