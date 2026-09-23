import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent } from 'vue'
import LoginView from '../LoginView.vue'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'

const state = vi.hoisted(() => ({
  locale: 'zh' as 'zh' | 'en',
  login: vi.fn(), login2FA: vi.fn(), push: vi.fn(),
  showError: vi.fn(), showSuccess: vi.fn(), showWarning: vi.fn(),
  setError: vi.fn(), setVerifying: vi.fn()
}))

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => {
    let value: unknown = state.locale === 'zh' ? zh : en
    for (const part of key.split('.')) {
      value = value && typeof value === 'object' ? (value as Record<string, unknown>)[part] : undefined
    }
    return typeof value === 'string' ? value : key
  } })
}))
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: state.push, currentRoute: { value: { query: {} } } })
}))
vi.mock('@/stores', () => ({
  useAuthStore: () => ({ login: state.login, login2FA: state.login2FA }),
  useAppStore: () => ({ showError: state.showError, showSuccess: state.showSuccess, showWarning: state.showWarning })
}))
vi.mock('@/api/auth', () => ({
  getPublicSettings: vi.fn().mockResolvedValue({ turnstile_enabled: false, login_agreement_enabled: false }),
  isTotp2FARequired: (response: { requires_2fa?: boolean }) => response?.requires_2fa === true,
  isWeChatWebOAuthEnabled: () => false
}))

const TotpModal = defineComponent({
  name: 'TotpLoginModal',
  props: ['tempToken', 'userEmailMasked'],
  emits: ['verify', 'cancel'],
  setup(_, { expose }) {
    expose({ setError: state.setError, setVerifying: state.setVerifying })
  },
  template: '<div class="totp-modal" />'
})

let page: VueWrapper | undefined
async function render(locale: 'zh' | 'en') {
  state.locale = locale
  page = mount(LoginView, {
    global: { stubs: {
      AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
      RouterLink: { template: '<a><slot /></a>' },
      Icon: true,
      TotpLoginModal: TotpModal
    } }
  })
  await flushPromises()
  await page.get('#email').setValue('person@example.com')
  await page.get('#password').setValue('test-password')
  return page
}

beforeEach(() => {
  vi.clearAllMocks()
  state.login.mockReset()
  state.login2FA.mockReset()
  localStorage.clear()
  sessionStorage.clear()
})
afterEach(() => page?.unmount())

const unavailable = { status: 403, code: 403, reason: 'USER_NOT_ACTIVE', message: 'user is not active' }
const locales = [
  ['zh', '暂无访问权限'],
  ['en', 'Access is not available at the moment.']
] as const

describe('login access-unavailable feedback', () => {
  it.each(locales)('shows neutral localized feedback without granting access (%s)', async (locale, message) => {
    state.login.mockRejectedValue(unavailable)
    const view = await render(locale)
    await view.get('form').trigger('submit')
    await flushPromises()
    expect(state.showError).toHaveBeenCalledOnce()
    expect(state.showError).toHaveBeenCalledWith(message)
    expect(state.showSuccess).not.toHaveBeenCalled()
    expect(state.push).not.toHaveBeenCalled()
    expect(state.login2FA).not.toHaveBeenCalled()
    expect(view.get('button[type="submit"]').attributes('disabled')).toBeUndefined()
  })

  it.each(locales)('uses the same feedback if access becomes unavailable during 2FA (%s)', async (locale, message) => {
    state.login.mockResolvedValue({ requires_2fa: true, temp_token: 'test-session', user_email_masked: 'p***@example.com' })
    state.login2FA.mockRejectedValue(unavailable)
    const view = await render(locale)
    await view.get('form').trigger('submit')
    await flushPromises()
    view.getComponent(TotpModal).vm.$emit('verify', '123456')
    await flushPromises()
    expect(state.login2FA).toHaveBeenCalledWith('test-session', '123456')
    expect(state.setError).toHaveBeenCalledWith(message)
    expect(state.setVerifying).toHaveBeenLastCalledWith(false)
    expect(state.showSuccess).not.toHaveBeenCalled()
    expect(state.push).not.toHaveBeenCalled()
  })

  it('keeps credential failures distinct from unavailable access', async () => {
    state.login.mockRejectedValue({ status: 401, reason: 'INVALID_CREDENTIALS', message: 'Invalid credentials' })
    const view = await render('zh')
    await view.get('form').trigger('submit')
    await flushPromises()
    expect(state.showError).toHaveBeenCalledOnce()
    expect(state.showError).toHaveBeenCalledWith('Invalid credentials')
    expect(state.push).not.toHaveBeenCalled()
  })
})
