/**
 * Admin Promo Codes API endpoints
 */

import { apiClient } from '../client'
import type {
  AdminCafeCoupon,
  BasePaginationResponse,
  CafeCouponMembershipLevel,
  CreatePromoCodeRequest,
  PromoCode,
  PromoCodeUsage,
  UpdateCafeCouponStatusRequest,
  UpdatePromoCodeRequest
} from '@/types'

export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    status?: string
    search?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  },
  options?: {
    signal?: AbortSignal
  }
): Promise<BasePaginationResponse<PromoCode>> {
  const { data } = await apiClient.get<BasePaginationResponse<PromoCode>>('/admin/promo-codes', {
    params: { page, page_size: pageSize, ...filters },
    signal: options?.signal
  })
  return data
}

export async function getById(id: number): Promise<PromoCode> {
  const { data } = await apiClient.get<PromoCode>(`/admin/promo-codes/${id}`)
  return data
}

export async function create(request: CreatePromoCodeRequest): Promise<PromoCode> {
  const { data } = await apiClient.post<PromoCode>('/admin/promo-codes', request)
  return data
}

export async function update(id: number, request: UpdatePromoCodeRequest): Promise<PromoCode> {
  const { data } = await apiClient.put<PromoCode>(`/admin/promo-codes/${id}`, request)
  return data
}

export async function deleteCode(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/promo-codes/${id}`)
  return data
}

export async function getUsages(
  id: number,
  page: number = 1,
  pageSize: number = 20
): Promise<BasePaginationResponse<PromoCodeUsage>> {
  const { data } = await apiClient.get<BasePaginationResponse<PromoCodeUsage>>(
    `/admin/promo-codes/${id}/usages`,
    { params: { page, page_size: pageSize } }
  )
  return data
}

export async function listCafeCoupons(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    search?: string
    status?: string
    type?: string
    membership_level?: CafeCouponMembershipLevel
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  },
  options?: {
    signal?: AbortSignal
  }
): Promise<BasePaginationResponse<AdminCafeCoupon>> {
  const { data } = await apiClient.get<BasePaginationResponse<AdminCafeCoupon>>(
    '/admin/promo-codes/cafe-coupons',
    {
      params: { page, page_size: pageSize, ...filters },
      signal: options?.signal
    }
  )
  return data
}

export async function getCafeCoupon(id: number): Promise<AdminCafeCoupon> {
  const { data } = await apiClient.get<AdminCafeCoupon>(`/admin/promo-codes/cafe-coupons/${id}`)
  return data
}

export async function voidCafeCoupon(id: number): Promise<AdminCafeCoupon> {
  const { data } = await apiClient.post<AdminCafeCoupon>(`/admin/promo-codes/cafe-coupons/${id}/void`)
  return data
}

export async function updateCafeCouponStatus(id: number, request: UpdateCafeCouponStatusRequest): Promise<AdminCafeCoupon> {
  const { data } = await apiClient.patch<AdminCafeCoupon>(`/admin/promo-codes/cafe-coupons/${id}/status`, request)
  return data
}

export async function resetCafeCouponClaimPeriod(id: number): Promise<AdminCafeCoupon> {
  const { data } = await apiClient.post<AdminCafeCoupon>(`/admin/promo-codes/cafe-coupons/${id}/reset-claim-period`)
  return data
}

const promoAPI = {
  list,
  getById,
  create,
  update,
  delete: deleteCode,
  getUsages,
  listCafeCoupons,
  getCafeCoupon,
  voidCafeCoupon,
  updateCafeCouponStatus,
  resetCafeCouponClaimPeriod
}

export default promoAPI
