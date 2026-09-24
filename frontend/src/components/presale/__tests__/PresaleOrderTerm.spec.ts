import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import PresaleOrderTerm from '../PresaleOrderTerm.vue'
import zh from '@/i18n/locales/presale.zh'
import en from '@/i18n/locales/presale.en'
import type { PaymentOrder } from '@/types/payment'

const state = vi.hoisted(() => ({ locale: 'zh' }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({
  locale: { value: state.locale },
  t: (key: string, args: Record<string, string> = {}) => {
    const messages = (state.locale === 'zh' ? zh : en) as Record<string, unknown>
    const value = messages[key.replace('presale.', '')]
    return typeof value === 'string' ? value.replace(/\{(\w+)\}/g, (_, name) => args[name] ?? '') : key
  }
}) }))

enableAutoUnmount(afterEach)

describe('PresaleOrderTerm', () => {
  it.each(['zh', 'en'])('keeps the term and renewal without a timezone caption in %s', locale => {
    state.locale = locale
    const wrapper = mount(PresaleOrderTerm, {
      props: { order: {
        presale_plan_name: '小杯', status: 'COMPLETED', presale_renewal: true,
        presale_starts_at: '2099-09-30T16:00:00Z', presale_expires_at: '2099-10-31T16:00:00Z'
      } as PaymentOrder }
    })
    expect(wrapper.get('h3').text()).toBe('小杯')
    expect(wrapper.text()).toContain((locale === 'zh' ? zh : en).renewal)
    expect(wrapper.text()).not.toMatch(/UTC\+8|Beijing time|北京时间/)
    expect(wrapper.findAll('p')).toHaveLength(2) // Renewal and dates, no empty caption row.
    expect(wrapper.text()).toContain(locale === 'zh' ? '2099/10/01 00:00' : '01/10/2099, 00:00')
  })
})
