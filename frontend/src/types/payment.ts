/**
 * Payment System Type Definitions
 */

// ==================== Enums / Union Types ====================

export type OrderStatus =
  | 'PENDING'
  | 'PAID'
  | 'RECHARGING'
  | 'COMPLETED'
  | 'EXPIRED'
  | 'CANCELLED'
  | 'FAILED'
  | 'REFUND_REQUESTED'
  | 'REFUNDING'
  | 'PARTIALLY_REFUNDED'
  | 'REFUNDED'
  | 'REFUND_FAILED'

export type PaymentType = 'alipay' | 'wxpay' | 'alipay_direct' | 'wxpay_direct' | 'stripe' | 'easypay' | 'airwallex'

export type OrderType = 'balance' | 'subscription'

// ==================== Configuration ====================

export interface PaymentConfig {
  payment_enabled: boolean
  min_amount: number
  max_amount: number
  daily_limit: number
  max_pending_orders: number
  order_timeout_minutes: number
  balance_disabled: boolean
  balance_recharge_multiplier: number
  enabled_payment_types: PaymentType[]
  help_image_url: string
  help_text: string
  stripe_publishable_key: string
}

export interface MethodLimit {
  currency?: string
  daily_limit: number
  daily_used: number
  daily_remaining: number
  single_min: number
  single_max: number
  fee_rate: number
  available: boolean
}

/** Response from /payment/limits API */
export interface MethodLimitsResponse {
  methods: Record<string, MethodLimit>
  global_min: number  // widest min across all methods; 0 = no minimum
  global_max: number  // widest max across all methods; 0 = no maximum
}

/** Response from /payment/checkout-info API — single call for the payment page */
export interface CheckoutInfoResponse {
  methods: Record<string, MethodLimit>
  global_min: number
  global_max: number
  plans: SubscriptionPlan[]
  balance_disabled: boolean
  balance_recharge_multiplier: number
  recharge_fee_rate: number
  help_text: string
  help_image_url: string
  stripe_publishable_key: string
  /** When true, Alipay payments on mobile always show the QR code instead of redirecting */
  alipay_force_qrcode?: boolean
}

// ==================== Orders ====================

export interface PaymentOrder {
  id: number
  user_id: number
  amount: number
  pay_amount: number
  currency?: string
  fee_rate: number
  payment_type: string
  out_trade_no: string
  status: OrderStatus
  order_type: OrderType
  created_at: string
  expires_at: string
  paid_at?: string
  failed_at?: string
  failed_reason?: string
  completed_at?: string
  refund_amount: number
  refund_reason?: string
  refund_requested_at?: string
  refund_requested_by?: number
  refund_request_reason?: string
  plan_id?: number
  provider_instance_id?: string
  subscription_group_id?: number
  subscription_multiplier?: number
  subscription_source_group_id?: number
  subscription_source_price?: number
  subscription_source_original_price?: number
  cafe_coupon_discount?: number
}

// ==================== Plans & Channels ====================

export interface SubscriptionPlan {
  id: number
  group_id: number
  group_platform?: string
  group_name?: string
  rate_multiplier?: number
  daily_limit_usd?: number | null
  weekly_limit_usd?: number | null
  monthly_limit_usd?: number | null
  supported_model_scopes?: string[]
  name: string
  description: string
  price: number
  original_price?: number
  validity_days: number
  concurrency: number
  validity_unit: string
  early_reset_enabled?: boolean
  early_reset_duration_days?: number
  /** Stored as JSON string in backend; API layer should parse before use */
  features: string[]
  for_sale: boolean
  sort_order: number
  custom_multiplier_enabled?: boolean
  custom_multiplier_min?: number
  custom_multiplier_max?: number
  subscription_bonus?: SubscriptionBonusBenefit
}

export interface SubscriptionBonusBenefit {
  activity_id: number
  days: number
  ends_at: string
}

export type PromotionActivityStatus = 'disabled' | 'scheduled' | 'active' | 'ended'

export interface PromotionActivityPlanBonus {
  id?: number
  plan_id: number
  bonus_days: number
}

export interface PromotionActivity {
  id: number
  name: string
  type: 'subscription_bonus_days'
  enabled: boolean
  status: PromotionActivityStatus
  starts_at: string
  ends_at: string
  max_uses_per_user: number
  plan_bonuses: PromotionActivityPlanBonus[]
  created_at: string
  updated_at: string
}

export interface PromotionActivityRecord extends PromotionActivity {
  participant_count: number
  participation_count: number
  reserved_count: number
  granted_count: number
  released_count: number
  granted_bonus_days: number
}

export interface PromotionActivityParticipant {
  user_id: number
  user_email: string
  user_name: string
  participation_count: number
  reserved_count: number
  granted_count: number
  released_count: number
  granted_bonus_days: number
  first_participated_at: string
  last_participated_at: string
}

export type PromotionParticipationStatus = 'reserved' | 'granted' | 'released'

export interface PromotionActivityParticipationRecord {
  id: number
  activity_id: number
  user_id: number
  user_email: string
  user_name: string
  order_id: number
  out_trade_no: string
  order_status: OrderStatus
  payment_type: string
  amount: number
  pay_amount: number
  plan_id: number
  plan_name: string
  subscription_days?: number
  subscription_bonus_days: number
  status: PromotionParticipationStatus
  bonus_days: number
  reserved_at: string
  granted_at?: string
  released_at?: string
  release_reason?: string
  created_at: string
  order_created_at?: string
  paid_at?: string
  completed_at?: string
  failed_at?: string
  failed_reason?: string
  refund_amount: number
  refund_at?: string
}

export interface UpsertPromotionActivityRequest {
  name: string
  type: 'subscription_bonus_days'
  enabled: boolean
  starts_at: string
  ends_at: string
  max_uses_per_user: number
  plan_bonuses: Array<Pick<PromotionActivityPlanBonus, 'plan_id' | 'bonus_days'>>
}

export interface PaymentChannel {
  id: number
  group_id?: number
  name: string
  platform: string
  rate_multiplier: number
  description: string
  models: string[]
  features: string[]
  enabled: boolean
}

// ==================== Providers ====================

export interface ProviderInstance {
  id: number
  provider_key: string
  name: string
  config: Record<string, string>
  supported_types: string[]
  enabled: boolean
  payment_mode: string
  refund_enabled: boolean
  allow_user_refund: boolean
  limits: string
  sort_order: number
}

// ==================== Request / Response ====================

export interface CreateOrderRequest {
  amount: number
  payment_type: string
  order_type: string
  plan_id?: number
  return_url?: string
  payment_source?: string
  cafe_coupon_code?: string
  expected_subscription_bonus_activity_id?: number
  openid?: string
  wechat_resume_token?: string
  is_mobile?: boolean
  multiplier?: number
}

export interface CafeCouponSummary {
  code: string
  type: 'cash' | 'discount'
  value: number
  period: 'day' | 'week' | 'month'
  expires_at: string
  claimed_at: string
  display_name: string
  copy_text?: string
  already_claimed?: boolean
  can_claim?: boolean
  remaining_days?: number
  next_claim_at?: string
  status?: 'issued' | 'applied' | 'void'
  transferable?: boolean
  validity?: string
  valid_until_month_end?: boolean
}

export type CafeCouponClaimResponse = CafeCouponSummary

export interface CafeCouponStatusResponse {
  eligible: boolean
  can_claim: boolean
  already_claimed: boolean
  next_claim_at?: string
  remaining_days: number
  membership_level: number
  type?: 'cash' | 'discount'
  value?: number
  period: 'day' | 'week' | 'month'
  period_start?: string
  period_end?: string
  expires_at?: string
  transferable: boolean
  validity: string
  valid_until_month_end: boolean
  coupon?: CafeCouponSummary
}

export interface CafeCouponInfoRequest {
  code: string
}

export interface CafeCouponInfoResponse {
  valid: boolean
  coupon?: CafeCouponSummary
  message?: string
}

export interface CafeCouponPreviewRequest {
  code: string
  amount: number
  order_type: OrderType
  plan_id?: number
  multiplier?: number
}

export interface CafeCouponPreviewResponse {
  valid: boolean
  discount_amount: number
  pay_amount: number
  coupon?: CafeCouponSummary
  message?: string
}

export type CreateOrderResultType = 'order_created' | 'oauth_required' | 'jsapi_ready' | 'payment_completed'

export interface WechatOAuthInfo {
  authorize_url?: string
  appid?: string
  openid?: string
  scope?: string
  state?: string
  redirect_url?: string
}

export interface WechatJSAPIPayload {
  appId?: string
  timeStamp?: string
  nonceStr?: string
  package?: string
  signType?: string
  paySign?: string
}

export interface CreateOrderResult {
  order_id: number
  amount: number
  pay_url?: string
  qr_code?: string
  client_secret?: string
  intent_id?: string
  currency?: string
  country_code?: string
  payment_env?: string
  pay_amount: number
  fee_rate: number
  expires_at: string
  result_type?: CreateOrderResultType
  payment_type?: string
  out_trade_no?: string
  payment_mode?: string
  resume_token?: string
  oauth?: WechatOAuthInfo
  jsapi?: WechatJSAPIPayload
  jsapi_payload?: WechatJSAPIPayload
}

export interface DashboardStats {
  today_amount: number
  total_amount: number
  today_count: number
  total_count: number
  avg_amount: number
  daily_series: { date: string; amount: number; count: number }[]
  payment_methods: { type: string; amount: number; count: number }[]
  top_users: { user_id: number; email: string; amount: number }[]
}
