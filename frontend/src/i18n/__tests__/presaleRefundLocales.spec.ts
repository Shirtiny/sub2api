import { describe, expect, it } from 'vitest'
import { extractI18nErrorMessage } from '@/utils/apiError'
import zh from '../locales/zh'
import en from '../locales/en'

describe('coupon usage limit and presale refund messages', () => {
  it.each(['zh', 'en'] as const)('interpolates the account limit in every checkout namespace (%s)', locale => {
    const t = (key: string, params: Record<string, unknown> = {}) => {
      let value: unknown = locale === 'zh' ? zh : en
      for (const part of key.split('.')) value = value && typeof value === 'object' ? (value as Record<string, unknown>)[part] : undefined
      return typeof value === 'string' ? value.replace(/\{(\w+)\}/g, (_, name) => String(params[name] ?? '')) : key
    }
    for (const namespace of ['payment.cafeCoupon.errors', 'presale.errors', 'cafeCampaign.errors']) {
      const message = extractI18nErrorMessage({ reason: 'CAFE_CAMPAIGN_USAGE_LIMIT', metadata: { limit: '1' } }, t, namespace, 'fallback')
      expect(message).toContain('1')
      expect(message).not.toMatch(/fallback|\{limit\}|CAFE_CAMPAIGN_USAGE_LIMIT/)
      if (locale === 'zh') expect(message).toBe('无法应用，该券码每人只能使用 1 次。')
      else expect(message).toContain('Each account')
    }
  })
  it('keeps personal-coupon errors distinct and both refund languages aligned', () => {
    expect(zh.payment.cafeCoupon.errors.CAFE_COUPON_USED).toBe('该 Café券已被使用。')
    for (const locale of [zh, en]) {
      expect(locale.presale.refundCouponNotice).toBeTruthy()
      expect(locale.presale.errors.PRESALE_REFUND_REASON_REQUIRED).toBeTruthy()
      expect(locale.presale.errors.PRESALE_REFUND_REASON_TOO_LONG).toContain('{max}')
    }
    expect(zh.presale.refundCouponNotice).toBe('退款后，下单时使用的优惠券无法退还。')
    expect(en.presale.refundCouponNotice).toBe('The coupon used for this order cannot be returned after a refund.')
  })
})
