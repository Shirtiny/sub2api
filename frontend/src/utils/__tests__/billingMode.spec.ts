import { describe, expect, it } from 'vitest'

import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_TOKEN,
  getDisplayBillingMode,
  isImageUsage,
} from '@/utils/billingMode'

describe('billingMode utils', () => {
  it('displays historical image rows with missing billing mode as image', () => {
    const row = { image_count: 2, billing_mode: null }

    expect(isImageUsage(row)).toBe(true)
    expect(getDisplayBillingMode(row)).toBe(BILLING_MODE_IMAGE)
  })

  it('preserves explicit token and per-request billing modes', () => {
    expect(getDisplayBillingMode({ image_count: 2, billing_mode: BILLING_MODE_TOKEN })).toBe(BILLING_MODE_TOKEN)
    expect(getDisplayBillingMode({ image_count: 2, billing_mode: BILLING_MODE_PER_REQUEST })).toBe(BILLING_MODE_PER_REQUEST)
  })
})
