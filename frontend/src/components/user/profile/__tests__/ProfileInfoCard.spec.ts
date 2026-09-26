import { enableAutoUnmount, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ProfileInfoCard from '@/components/user/profile/ProfileInfoCard.vue'
import type { User } from '@/types'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'

enableAutoUnmount(afterEach)
let testLocale: 'en' | 'zh' = 'en'
afterEach(() => { testLocale = 'en'; vi.useRealTimers() })

vi.mock('vue-router', () => ({
  useRoute: () => ({
    fullPath: '/profile'
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: null
  })
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn()
  })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => {
        if (key.startsWith('profile.concurrencyRules.')) {
          const messages = (testLocale === 'zh' ? zh : en).profile.concurrencyRules
          const message = messages[key.split('.').pop() as keyof typeof messages]
          return message.replace(/\{(\w+)\}/g, (_, name: string) => params?.[name] ?? '')
        }
        if (key === 'profile.accountBalance') return 'Account Balance'
        if (key === 'profile.concurrencyLimit') return 'Concurrency Limit'
        if (key === 'profile.memberSince') return 'Member Since'
        if (key === 'profile.administrator') return 'Administrator'
        if (key === 'profile.user') return 'User'
        if (key === 'profile.authBindings.providers.email') return 'Email'
        if (key === 'profile.authBindings.providers.linuxdo') return 'LinuxDo'
        if (key === 'profile.authBindings.providers.wechat') return 'WeChat'
        if (key === 'profile.authBindings.providers.oidc') return params?.providerName || 'OIDC'
        if (key === 'profile.authBindings.source.avatar') {
          return `Avatar synced from ${params?.providerName || 'provider'}`
        }
        if (key === 'profile.authBindings.source.username') {
          return `Username synced from ${params?.providerName || 'provider'}`
        }
        return key
      }
    })
  }
})

function createUser(overrides: Partial<User> = {}): User {
  return {
    id: 5,
    username: 'alice',
    email: 'alice@example.com',
    avatar_url: null,
    role: 'user',
    balance: 10,
    concurrency: 2,
    status: 'active',
    allowed_groups: null,
    balance_notify_enabled: true,
    balance_notify_threshold: null,
    balance_notify_extra_emails: [],
    created_at: '2026-04-20T00:00:00Z',
    updated_at: '2026-04-20T00:00:00Z',
    ...overrides
  }
}

describe('ProfileInfoCard', () => {
  it('shows concurrency rules from the question mark beside the effective limit', async () => {
    vi.useFakeTimers()
    const wrapper = mount(ProfileInfoCard, {
      props: { user: createUser({ concurrency: 32, effective_concurrency: 1 }) },
      global: { stubs: { Icon: true } }
    })
    const help = wrapper.get('[data-testid="profile-concurrency-help"]')
    expect(help.attributes('aria-label')).toBe('Concurrency rules')
    expect(wrapper.get('[data-testid="profile-overview-metric-concurrency"]').text()).toContain('1')
    const rules = document.querySelector('[data-testid="profile-concurrency-rules"]')!
    const tooltip = rules.closest('[role="tooltip"]') as HTMLElement
    expect(tooltip.style.display).toBe('none')
    help.element.parentElement!.dispatchEvent(new MouseEvent('mouseenter'))
    await vi.advanceTimersByTimeAsync(100)
    expect(tooltip.style.display).not.toBe('none')
    for (const key of ['subscription', 'balance', 'note'] as const) {
      expect(rules.textContent).toContain(en.profile.concurrencyRules[key])
    }
    expect(rules.textContent).toContain('$0 ≤ balance < $20')
    expect(rules.textContent).toContain('$20 ≤ balance < $100')
    expect(rules.textContent).toContain('Balance ≥ $100 (no upper bound)')
    wrapper.unmount()
    vi.useRealTimers()
  })

  it.each(['en', 'zh'] as const)('shows custom bounds and concurrency in %s using account dollars, with subscription priority', async locale => {
    testLocale = locale
    const wrapper = mount(ProfileInfoCard, {
      props: { user: createUser({ balance_concurrency_rules: [
        { min_balance: 0, concurrency: 4 }, { min_balance: 7.125, concurrency: 9 }, { min_balance: 250, concurrency: 16 }
      ] }) },
      global: { stubs: { Icon: true } }
    })
    // Click works on touch devices as well as with the keyboard-activatable button.
    await wrapper.get('[data-testid="profile-concurrency-help"]').trigger('click')
    const rules = document.querySelector('[data-testid="profile-concurrency-rules"]')!
    const tiers = Array.from(rules.querySelectorAll('[data-testid="profile-concurrency-tier"]'))
    expect(tiers).toHaveLength(3)
    expect(tiers[0].textContent).toContain(locale === 'en' ? '$0 ≤ balance < $7.125' : '$0 ≤ 余额 < $7.125')
    expect(tiers[1].textContent).toContain(locale === 'en' ? '$7.125 ≤ balance < $250' : '$7.125 ≤ 余额 < $250')
    expect(tiers[2].textContent).toContain(locale === 'en' ? 'Balance ≥ $250 (no upper bound)' : '余额 ≥ $250（无上限）')
    expect(tiers.map(tier => tier.lastElementChild?.textContent)).toEqual(['4', '9', '16'])
    expect(rules.textContent).toContain((locale === 'en' ? en : zh).profile.concurrencyRules.subscription)
    expect(rules.textContent).not.toMatch(/legacy|历史订阅|旧订阅|RMB|人民币|¥|\$20\b|\$100\b/)
    expect((rules.closest('[role="tooltip"]') as HTMLElement).style.display).not.toBe('none')

    await wrapper.setProps({ user: createUser({ balance_concurrency_rules: [{ min_balance: 0, concurrency: 30 }] }) })
    expect(rules.querySelectorAll('[data-testid="profile-concurrency-tier"]')).toHaveLength(1)
    expect(rules.textContent).toContain(locale === 'en' ? 'Balance ≥ $0 (no upper bound)' : '余额 ≥ $0（无上限）')
    expect(rules.textContent).toContain('30')
  })

  it('does not substitute legacy defaults for an explicitly present empty rules array', () => {
    mount(ProfileInfoCard, {
      props: { user: createUser({ balance_concurrency_rules: [] }) },
      global: { stubs: { Icon: true } }
    })
    expect(document.querySelectorAll('[data-testid="profile-concurrency-tier"]')).toHaveLength(0)
  })

  it('renders basic account information inside the new overview shell', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser()
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.text()).toContain('alice@example.com')
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain('User')
    expect(wrapper.get('[data-testid="profile-basics-panel"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="profile-auth-bindings-panel"]').exists()).toBe(true)
  })

  it('renders the effective plan concurrency while preserving the base value', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser({ concurrency: 5, effective_concurrency: 16 })
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.get('[data-testid="profile-overview-metric-concurrency"]').text()).toContain('16')
  })

  it('renders third-party source hints from profile sources', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser({
          avatar_url: 'https://cdn.example.com/linuxdo.png',
          profile_sources: {
            avatar: { provider: 'linuxdo', source: 'linuxdo' },
            username: { provider: 'linuxdo', source: 'linuxdo' }
          }
        })
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.text()).toContain('Avatar synced from LinuxDo')
    expect(wrapper.text()).toContain('Username synced from LinuxDo')
  })

  it('uses the configured OIDC provider name in source hints', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser({
          profile_sources: {
            username: { provider: 'oidc', source: 'oidc' }
          }
        }),
        oidcProviderName: 'ExampleID'
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.text()).toContain('Username synced from ExampleID')
  })

  it('does not display synthetic oauth-only emails as a real bound email', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser({
          email: 'legacy-user@oidc-connect.invalid',
          email_bound: false,
          auth_bindings: {
            email: { bound: false }
          }
        })
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.text()).not.toContain('legacy-user@oidc-connect.invalid')
  })

  it('does not display synthetic oauth-only emails when only legacy identity bindings mark email as unbound', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser({
          email: 'legacy-user@wechat-connect.invalid',
          identity_bindings: {
            email: { bound: false }
          }
        })
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.text()).not.toContain('legacy-user@wechat-connect.invalid')
  })

  it('renders the approved overview hero and two-column content shell', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser()
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.get('[data-testid="profile-overview-hero"]').text()).toContain('alice@example.com')
    expect(wrapper.get('[data-testid="profile-overview-metric-balance"]').text()).toContain('Account Balance')
    expect(wrapper.get('[data-testid="profile-overview-metric-concurrency"]').text()).toContain('Concurrency Limit')
    expect(wrapper.get('[data-testid="profile-overview-metric-member-since"]').text()).toContain('Member Since')
    expect(wrapper.find('[data-testid="profile-info-summary-grid"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="profile-main-column"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="profile-side-column"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="profile-basics-panel"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="profile-auth-bindings-panel"]').exists()).toBe(true)
  })
})
