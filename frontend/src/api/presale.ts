import { apiClient } from './client'
import type { PaymentOrder, SubscriptionPlan } from '@/types/payment'

export interface PresalePeriod {
  month: string
  timezone: string
  starts_at: string
  expires_at: string
  full_refund_before: string
}
export interface PresaleCatalog {
  period: PresalePeriod
  plans: SubscriptionPlan[]
  enabled: boolean
  server_time: string
}
export interface PresaleQuote extends PresalePeriod {
  plan_id: number
  renewal: boolean
  current_subscription_id?: number
  current_expires_at?: string
}
export interface PresaleRefundQuote {
  refund_amount: number
  gateway_amount: number
  fee_percent: number
  unused_days: number
  currency: string
  coupon_applied: boolean
  policy: 'full' | 'preparation' | 'unused_days' | 'unfulfilled'
}
export const presaleAPI = {
  catalog: () => apiClient.get<PresaleCatalog>('/payment/public/presale'),
  quote: (id: number) => apiClient.get<PresaleQuote>(`/payment/presale/${id}/quote`),
  mine: () => apiClient.get<PaymentOrder[]>('/payment/presale/my'),
  refundQuote: (id: number) => apiClient.get<PresaleRefundQuote>(`/payment/presale/orders/${id}/refund-quote`),
}
