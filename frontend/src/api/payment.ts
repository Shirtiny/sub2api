/**
 * User Payment API endpoints
 * Handles payment operations for regular users
 */

import { apiClient } from './client'
import type {
  PaymentConfig,
  SubscriptionPlan,
  PaymentChannel,
  MethodLimitsResponse,
  CheckoutInfoResponse,
  CreateOrderRequest,
  CreateOrderResult,
  PaymentOrder,
  CafeCouponInfoRequest,
  CafeCouponInfoResponse,
  CafeCouponPreviewRequest,
  CafeCouponPreviewResponse,
  CafeCouponClaimResponse,
  CafeCouponStatusResponse
} from '@/types/payment'
import type { BasePaginationResponse } from '@/types'

function createIdempotencyKey(prefix: string): string {
  const random = typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(36).slice(2)}`
  return `${prefix}-${random}`
}

export const paymentAPI = {
  /** Get payment configuration (enabled types, limits, etc.) */
  getConfig() {
    return apiClient.get<PaymentConfig>('/payment/config')
  },

  /** Get available subscription plans */
  getPlans() {
    return apiClient.get<SubscriptionPlan[]>('/payment/plans')
  },

  /** Get available payment channels */
  getChannels() {
    return apiClient.get<PaymentChannel[]>('/payment/channels')
  },

  /** Get all checkout page data in a single call */
  getCheckoutInfo() {
    return apiClient.get<CheckoutInfoResponse>('/payment/checkout-info')
  },

  /** Get payment method limits and fee rates */
  getLimits() {
    return apiClient.get<MethodLimitsResponse>('/payment/limits')
  },

  /** Create a new payment order */
  createOrder(data: CreateOrderRequest) {
    return apiClient.post<CreateOrderResult>('/payment/orders', data, {
      headers: { 'Idempotency-Key': createIdempotencyKey('payment-order') },
    })
  },

  /** Claim a café coupon for the current user */
  claimCafeCoupon() {
    return apiClient.post<CafeCouponClaimResponse>('/payment/cafe-coupons/claim', { source: 'membership' })
  },

  /** Get current café coupon claim status for the current user */
  getCafeCouponStatus() {
    return apiClient.get<CafeCouponStatusResponse>('/payment/cafe-coupons/status')
  },

  /** Validate a Cafe coupon and fetch display metadata. Read-only; does not consume the coupon. */
  getCafeCouponInfo(data: CafeCouponInfoRequest) {
    return apiClient.post<CafeCouponInfoResponse>('/payment/cafe-coupons/info', data)
  },

  /** Preview a Cafe coupon before creating an order. Read-only; does not consume the coupon. */
  previewCafeCoupon(data: CafeCouponPreviewRequest) {
    return apiClient.post<CafeCouponPreviewResponse>('/payment/cafe-coupons/preview', data)
  },

  /** Get current user's orders */
  getMyOrders(params?: { page?: number; page_size?: number; status?: string }) {
    return apiClient.get<BasePaginationResponse<PaymentOrder>>('/payment/orders/my', { params })
  },

  /** Get a specific order by ID */
  getOrder(id: number) {
    return apiClient.get<PaymentOrder>(`/payment/orders/${id}`)
  },

  /** Cancel a pending order */
  cancelOrder(id: number) {
    return apiClient.post(`/payment/orders/${id}/cancel`)
  },

  /** Verify order payment status with upstream provider */
  verifyOrder(outTradeNo: string) {
    return apiClient.post<PaymentOrder>('/payment/orders/verify', { out_trade_no: outTradeNo })
  },

  /** Legacy-compatible public order lookup by out_trade_no */
  verifyOrderPublic(outTradeNo: string) {
    return apiClient.post<PaymentOrder>('/payment/public/orders/verify', { out_trade_no: outTradeNo })
  },

  /** Resolve an order from a signed resume token without auth */
  resolveOrderPublicByResumeToken(resumeToken: string) {
    return apiClient.post<PaymentOrder>('/payment/public/orders/resolve', { resume_token: resumeToken })
  },

  /** Request a refund for a completed order */
  requestRefund(id: number, data: { reason: string }) {
    return apiClient.post(`/payment/orders/${id}/refund-request`, data)
  },

  /** Get provider instance IDs that allow user refund */
  getRefundEligibleProviders() {
    return apiClient.get<{ provider_instance_ids: string[] }>('/payment/orders/refund-eligible-providers')
  }
}
