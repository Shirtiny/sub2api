import { describe, expect, it } from 'vitest'

import { ratioToneClass } from '@/utils/subscriptionQuota'

describe('ratioToneClass', () => {
  it('is green below 70%', () => {
    expect(ratioToneClass(0)).toBe('bg-green-500')
    expect(ratioToneClass(0.6999)).toBe('bg-green-500')
  })

  it('is orange from 70% up to 90%', () => {
    expect(ratioToneClass(0.7)).toBe('bg-orange-500')
    expect(ratioToneClass(0.8999)).toBe('bg-orange-500')
  })

  it('is red from 90% upwards', () => {
    expect(ratioToneClass(0.9)).toBe('bg-red-500')
    expect(ratioToneClass(1)).toBe('bg-red-500')
  })

  // 管理员中途重置配额后，汇总表实际花费会高于窗口计费额，比值可以超过 1。
  it('stays red for ratios above 100%', () => {
    expect(ratioToneClass(1.1834)).toBe('bg-red-500')
  })

  it('treats null and undefined as zero', () => {
    expect(ratioToneClass(null)).toBe('bg-green-500')
    expect(ratioToneClass(undefined)).toBe('bg-green-500')
  })
})
