/**
 * Admin Payment API endpoints
 * Handles payment management operations for administrators
 */

import type { PresaleRefundQuote } from '../presale'
import { apiClient } from '../client'
import type {
  DashboardStats,
  PaymentOrder,
  PaymentChannel,
  SubscriptionPlan,
  ProviderInstance,
  PromotionActivity,
  PromotionActivityParticipant,
  PromotionActivityParticipationRecord,
  PromotionActivityRecord,
  UpsertPromotionActivityRequest
} from '@/types/payment'
import type { BasePaginationResponse } from '@/types'

export interface AdminPaymentOrder extends PaymentOrder {
  updated_at?: string
  user_email?: string
  user_name?: string
  user_notes?: string
  payment_trade_no?: string
  provider_key?: string
  client_ip?: string
  src_host?: string
  src_url?: string
  subscription_days?: number
  subscription_bonus_days?: number
  subscription_concurrency?: number
  subscription_early_reset_enabled?: boolean
  subscription_early_reset_duration_days?: number
  presale_subscription_id?: number
  cafe_coupon_code?: string
  refund_at?: string
}

export interface PresaleOfflineRequest {
  mode: 'cancel' | 'refund'
  amount: number
  reason: string
  reference: string
  confirmed: boolean
  expected_updated_at: string
}

export interface AdminOrderSummary {
  currency: string
  payment_mode?: string
  plan_name?: string
  plan_name_source?: 'snapshot' | 'current'
  group_name?: string
  provider_name?: string
}

export interface PaymentAuditLog {
  id: number
  action: string
  detail?: string | null
  operator?: string | null
  created_at: string
}

export interface AdminOrderDetailResponse {
  order: AdminPaymentOrder
  auditLogs: PaymentAuditLog[]
  summary: AdminOrderSummary
}

/** Admin-facing payment config returned by GET /admin/payment/config */
export interface AdminPaymentConfig {
  enabled: boolean
  min_amount: number
  max_amount: number
  daily_limit: number
  order_timeout_minutes: number
  max_pending_orders: number
  enabled_payment_types: string[]
  balance_disabled: boolean
  balance_recharge_multiplier: number
  load_balance_strategy: string
  product_name_prefix: string
  product_name_suffix: string
  help_image_url: string
  help_text: string
  guest_shop_enabled: boolean
  guest_shop_stripe_instance_id: number
}

/** Fields accepted by PUT /admin/payment/config (all optional via pointer semantics) */
export interface UpdatePaymentConfigRequest {
  enabled?: boolean
  min_amount?: number
  max_amount?: number
  daily_limit?: number
  order_timeout_minutes?: number
  max_pending_orders?: number
  enabled_payment_types?: string[]
  balance_disabled?: boolean
  balance_recharge_multiplier?: number
  load_balance_strategy?: string
  product_name_prefix?: string
  product_name_suffix?: string
  help_image_url?: string
  help_text?: string
  guest_shop_enabled?: boolean
  guest_shop_stripe_instance_id?: number
}

export const adminPaymentAPI = {
  getPresaleRefundQuote(id: number) { return apiClient.get<PresaleRefundQuote>(`/admin/payment/orders/${id}/presale-refund-quote`) },
  processPresaleOffline(id: number, data: PresaleOfflineRequest) {
    return apiClient.post<{ status: PaymentOrder['status']; affiliate_pending: boolean }>(`/admin/payment/orders/${id}/presale-offline`, data)
  },
  // ==================== Config ====================

  /** Get payment configuration (admin view) */
  getConfig() {
    return apiClient.get<AdminPaymentConfig>('/admin/payment/config')
  },

  /** Update payment configuration */
  updateConfig(data: UpdatePaymentConfigRequest) {
    return apiClient.put('/admin/payment/config', data)
  },

  // ==================== Dashboard ====================

  /** Get payment dashboard statistics */
  getDashboard(days?: number) {
    return apiClient.get<DashboardStats>('/admin/payment/dashboard', {
      params: days ? { days } : undefined
    })
  },

  // ==================== Orders ====================

  /** Get all orders (paginated, with filters) */
  getOrders(params?: {
    page?: number
    page_size?: number
    status?: string
    payment_type?: string
    user_id?: number
    keyword?: string
    start_date?: string
    end_date?: string
    order_type?: string
  }) {
    return apiClient.get<BasePaginationResponse<AdminPaymentOrder>>('/admin/payment/orders', { params })
  },

  /** Get a specific order by ID */
  getOrder(id: number) {
    return apiClient.get<AdminOrderDetailResponse>(`/admin/payment/orders/${id}`)
  },

  /** Cancel an order (admin) */
  cancelOrder(id: number) {
    return apiClient.post(`/admin/payment/orders/${id}/cancel`)
  },

  /** Retry recharge for a failed order */
  retryRecharge(id: number) {
    return apiClient.post(`/admin/payment/orders/${id}/retry`)
  },

  /** Process a refund */
  refundOrder(id: number, data: { amount: number; reason: string; deduct_balance?: boolean; force?: boolean }) {
    return apiClient.post(`/admin/payment/orders/${id}/refund`, data)
  },

  // ==================== Channels ====================

  /** Get all payment channels */
  getChannels() {
    return apiClient.get<PaymentChannel[]>('/admin/payment/channels')
  },

  /** Create a payment channel */
  createChannel(data: Partial<PaymentChannel>) {
    return apiClient.post<PaymentChannel>('/admin/payment/channels', data)
  },

  /** Update a payment channel */
  updateChannel(id: number, data: Partial<PaymentChannel>) {
    return apiClient.put<PaymentChannel>(`/admin/payment/channels/${id}`, data)
  },

  /** Delete a payment channel */
  deleteChannel(id: number) {
    return apiClient.delete(`/admin/payment/channels/${id}`)
  },

  // ==================== Subscription Plans ====================

  /** Get all subscription plans */
  getPlans() {
    return apiClient.get<SubscriptionPlan[]>('/admin/payment/plans')
  },

  /** Create a subscription plan */
  createPlan(data: Record<string, unknown>) {
    return apiClient.post<SubscriptionPlan>('/admin/payment/plans', data)
  },

  /** Update a subscription plan */
  updatePlan(id: number, data: Record<string, unknown>) {
    return apiClient.put<SubscriptionPlan>(`/admin/payment/plans/${id}`, data)
  },

  /** Delete a subscription plan */
  deletePlan(id: number) {
    return apiClient.delete(`/admin/payment/plans/${id}`)
  },

  // ==================== Promotion Activities ====================

  getActivities() {
    return apiClient.get<PromotionActivity[]>('/admin/payment/activities')
  },

  createActivity(data: UpsertPromotionActivityRequest) {
    return apiClient.post<PromotionActivity>('/admin/payment/activities', data)
  },

  updateActivity(id: number, data: UpsertPromotionActivityRequest) {
    return apiClient.put<PromotionActivity>(`/admin/payment/activities/${id}`, data)
  },

  deleteActivity(id: number) {
    return apiClient.delete(`/admin/payment/activities/${id}`)
  },

  getActivityRecords(params?: { page?: number; page_size?: number; keyword?: string; status?: string }) {
    return apiClient.get<BasePaginationResponse<PromotionActivityRecord>>('/admin/payment/activity-records', { params })
  },

  getActivityParticipants(id: number, params?: { page?: number; page_size?: number; keyword?: string }) {
    return apiClient.get<BasePaginationResponse<PromotionActivityParticipant>>(`/admin/payment/activities/${id}/participants`, { params })
  },

  getActivityParticipations(id: number, params?: { page?: number; page_size?: number; user_id?: number; keyword?: string; status?: string }) {
    return apiClient.get<BasePaginationResponse<PromotionActivityParticipationRecord>>(`/admin/payment/activities/${id}/participations`, { params })
  },

  // ==================== Provider Instances ====================

  /** Get all provider instances */
  getProviders() {
    return apiClient.get<ProviderInstance[]>('/admin/payment/providers')
  },

  /** Create a provider instance */
  createProvider(data: Partial<ProviderInstance>) {
    return apiClient.post<ProviderInstance>('/admin/payment/providers', data)
  },

  /** Update a provider instance */
  updateProvider(id: number, data: Partial<ProviderInstance>) {
    return apiClient.put<ProviderInstance>(`/admin/payment/providers/${id}`, data)
  },

  /** Delete a provider instance */
  deleteProvider(id: number) {
    return apiClient.delete(`/admin/payment/providers/${id}`)
  }
}

export default adminPaymentAPI
