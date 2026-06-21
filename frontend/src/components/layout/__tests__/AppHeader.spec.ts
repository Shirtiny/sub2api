import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppHeader.vue')
const componentSource = readFileSync(componentPath, 'utf8')

describe('AppHeader café coupon state', () => {
  it('hides the claim button when status says the coupon is already claimed', () => {
    expect(componentSource).toContain("cafeCouponStatus.value?.already_claimed === true")
    expect(componentSource).toContain("const showCafeCouponClaimButton = computed(() => cafeCouponEnabled.value && !cafeCouponClaimed.value)")
  })

  it('uses claimed coupon cooldown only for claimed remaining text', () => {
    expect(componentSource).toContain('const explicit = cafeCouponClaim.value?.remaining_days')
    expect(componentSource).toContain('const nextClaimAt = cafeCouponClaim.value?.next_claim_at || cafeCouponClaim.value?.expires_at')
    expect(componentSource).not.toContain('cafeCouponStatus.value?.remaining_days ??')
  })

  it('lets users copy the code text and use the coupon from the modal', () => {
    expect(componentSource).toContain('cursor-pointer')
    expect(componentSource).toContain('@click="copyCafeCouponCode"')
    expect(componentSource).toContain('@keydown.enter.prevent="copyCafeCouponCode"')
    expect(componentSource).toContain("t('membership.cafeCoupon.use')")
    expect(componentSource).toContain('function useCafeCouponCode()')
    expect(componentSource).toContain("router.push({ path: '/purchase', query: { cafe_coupon_code: code } })")
  })
})
