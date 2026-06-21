import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, patch } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  patch: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    get,
    post,
    patch
  }
}))

import { listCafeCoupons, getCafeCoupon, voidCafeCoupon, updateCafeCouponStatus, resetCafeCouponClaimPeriod } from '@/api/admin/promo'
import type { AdminCafeCoupon } from '@/types'

type Assert<T extends true> = T
type IsExact<T, U> = (
  (<G>() => G extends T ? 1 : 2) extends (<G>() => G extends U ? 1 : 2)
    ? ((<G>() => G extends U ? 1 : 2) extends (<G>() => G extends T ? 1 : 2) ? true : false)
    : false
)

type ExpectedAdminCafeCoupon = {
  id: number
  code: string
  user_id: number
  membership_level: import('@/types').CafeCouponMembershipLevel
  type: 'cash' | 'discount'
  value: number
  period: 'day' | 'week' | 'month'
  period_start: string
  period_end: string
  expires_at: string
  status: 'issued' | 'applied' | 'void'
  order_id?: number | null
  applied_at?: string | null
  created_at: string
  updated_at: string
  user?: import('@/types').User | null
}

const responseContractExact: Assert<IsExact<AdminCafeCoupon, ExpectedAdminCafeCoupon>> = true

const coupon: AdminCafeCoupon = {
  id: 7,
  code: 'CAFE-TEST',
  user_id: 42,
  membership_level: 2,
  type: 'cash',
  value: 8,
  period: 'month',
  period_start: '2026-06-01T00:00:00Z',
  period_end: '2026-06-30T23:59:59Z',
  expires_at: '2026-06-30T23:59:59Z',
  status: 'issued',
  order_id: null,
  applied_at: null,
  created_at: '2026-06-11T00:00:00Z',
  updated_at: '2026-06-11T00:00:00Z',
  user: null
}

describe('admin promo API Café coupons', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    patch.mockReset()
  })

  it('lists Café coupons with backend-compatible filters', async () => {
    const response = {
      items: [coupon],
      total: 1,
      page: 2,
      page_size: 10,
      pages: 1
    }
    get.mockResolvedValue({ data: response })

    const result = await listCafeCoupons(2, 10, {
      search: 'CAFE',
      status: 'issued',
      type: 'cash',
      membership_level: 0,
      sort_by: 'created_at',
      sort_order: 'desc'
    })

    expect(get).toHaveBeenCalledWith('/admin/promo-codes/cafe-coupons', {
      params: {
        page: 2,
        page_size: 10,
        search: 'CAFE',
        status: 'issued',
        type: 'cash',
        membership_level: 0,
        sort_by: 'created_at',
        sort_order: 'desc'
      },
      signal: undefined
    })
    expect(result).toEqual(response)
  })

  it('gets and voids a Café coupon by id', async () => {
    get.mockResolvedValueOnce({ data: coupon })
    post.mockResolvedValueOnce({ data: { ...coupon, status: 'void' } })

    await expect(getCafeCoupon(7)).resolves.toEqual(coupon)
    await expect(voidCafeCoupon(7)).resolves.toEqual({ ...coupon, status: 'void' })

    expect(get).toHaveBeenCalledWith('/admin/promo-codes/cafe-coupons/7')
    expect(post).toHaveBeenCalledWith('/admin/promo-codes/cafe-coupons/7/void')
  })

  it('updates status and resets claim period', async () => {
    patch.mockResolvedValueOnce({ data: { ...coupon, status: 'applied' } })
    post.mockResolvedValueOnce({ data: { ...coupon, status: 'void' } })

    await expect(updateCafeCouponStatus(7, { status: 'applied' })).resolves.toEqual({ ...coupon, status: 'applied' })
    await expect(resetCafeCouponClaimPeriod(7)).resolves.toEqual({ ...coupon, status: 'void' })

    expect(patch).toHaveBeenCalledWith('/admin/promo-codes/cafe-coupons/7/status', { status: 'applied' })
    expect(post).toHaveBeenCalledWith('/admin/promo-codes/cafe-coupons/7/reset-claim-period')
  })

  it('keeps AdminCafeCoupon aligned with the backend contract', () => {
    expect(responseContractExact).toBe(true)
  })
})
