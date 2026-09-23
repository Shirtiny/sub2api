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
    expect(JSON.parse(sessionStorage.getItem('register_data') || '{}')).toHaveProperty('waitlist_registration', true)
  })
  it('offers a direct sign-in path before an existing user starts registration', async () => {
    query.waitlist = '1'
    sessionStorage.setItem('register_data', JSON.stringify({ password: 'stale-password' }))
    render(); await flushPromises()
    const login = page.findAll('button').find(button => button.text() === 'auth.waitlistExistingAccountLink')!
    await login.trigger('click')
    expect(push).toHaveBeenCalledWith('/login')
    expect(sessionStorage.getItem('register_data')).toBeNull()
    expect(register).not.toHaveBeenCalled()
  })
  it.each(['WAITLIST_SIGN_IN_REQUIRED', 'EMAIL_EXISTS'])('recovers direct signup errors with a sign-in action: %s', async (reason) => {
    settings.mockResolvedValue({ registration_enabled: true, email_verify_enabled: false, invitation_code_enabled: false })
    register.mockRejectedValueOnce({ reason })
    render(); await flushPromises()
    await page.get('#email').setValue('existing@example.com')
    await page.get('#password').setValue('new-password')
    await page.get('form').trigger('submit'); await flushPromises()
    expect(page.find('form').exists()).toBe(false)
    expect(page.get('[role="status"]').text()).toContain('auth.registrationSignInTitle')
    expect(page.text()).not.toContain('auth.registrationDisabled')
    expect(showError).not.toHaveBeenCalled()
    await page.get('[role="status"] button').trigger('click')
    expect(push).toHaveBeenCalledWith('/login')
    expect(sessionStorage.getItem('register_data')).toBeNull()
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
