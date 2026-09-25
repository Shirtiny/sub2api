import { apiClient } from './client'
import type { PaymentOrder, SubscriptionPlan, PresaleBalanceBenefit, PromotionActivity } from '@/types/payment'

export interface PresalePeriod {
  month: string
  timezone: string
  starts_at: string
  expires_at: string
  full_refund_before: string
}
// Presentation contract shared by balance/day cards. The server only includes
// activity types actually supported by presale fulfillment.
export type PresaleActivity = Pick<PromotionActivity, 'id' | 'name' | 'type' | 'bonus_currency' | 'starts_at' | 'ends_at' | 'max_uses_per_user'> & {
  plan_bonuses: Array<{ plan_id: number; bonus_balance?: number; bonus_days?: number }>
}
export interface PresaleCatalog {
  activities?: PresaleActivity[]
  period: PresalePeriod
  plans: SubscriptionPlan[]
  enabled: boolean
  server_time: string
}
export interface PresaleQuote extends PresalePeriod {
  balance_bonus?: PresaleBalanceBenefit | null
  plan_id: number
  renewal: boolean
  current_subscription_id?: number
  current_expires_at?: string
}
export interface PresaleRefundQuote {
  balance_bonus_amount?: number
  balance_bonus_reclaim?: number
  balance_bonus_deduction?: number
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
