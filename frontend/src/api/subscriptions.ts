/**
 * User Subscription API
 * API for regular users to view their own subscriptions and progress
 */

import { apiClient } from './client'
import type { UserSubscription, SubscriptionProgress } from '@/types'
import { createIdempotencyKey } from '@/utils/idempotency'

/**
 * Subscription summary for user dashboard
 */
export interface SubscriptionSummary {
  active_count: number
  subscriptions: Array<{
    id: number
    group_name: string
    status: string
    daily_progress: number | null
    weekly_progress: number | null
    monthly_progress: number | null
    reset_count?: number
    expires_at: string | null
    days_remaining: number | null
  }>
}

/**
 * Get list of current user's subscriptions
 */
export async function getMySubscriptions(): Promise<UserSubscription[]> {
  const response = await apiClient.get<UserSubscription[]>('/subscriptions')
  return response.data
}

/**
 * Get current user's active subscriptions
 */
export async function getActiveSubscriptions(): Promise<UserSubscription[]> {
  const response = await apiClient.get<UserSubscription[]>('/subscriptions/active')
  return response.data
}

/**
 * Get progress for all user's active subscriptions
 */
export async function getSubscriptionsProgress(): Promise<SubscriptionProgress[]> {
  const response = await apiClient.get<SubscriptionProgress[]>('/subscriptions/progress')
  return response.data
}

/**
 * Get subscription summary for dashboard display
 */
export async function getSubscriptionSummary(): Promise<SubscriptionSummary> {
  const response = await apiClient.get<SubscriptionSummary>('/subscriptions/summary')
  return response.data
}

/**
 * Get progress for a specific subscription
 */
export async function getSubscriptionProgress(
  subscriptionId: number
): Promise<SubscriptionProgress> {
  const response = await apiClient.get<SubscriptionProgress>(
    `/subscriptions/${subscriptionId}/progress`
  )
  return response.data
}

export async function earlyResetSubscription(
  subscriptionId: number,
  idempotencyKey = createIdempotencyKey('subscription-early-reset')
): Promise<UserSubscription> {
  const response = await apiClient.post<UserSubscription>(
    `/subscriptions/${subscriptionId}/early-reset`,
    {},
    {
      headers: {
        'Idempotency-Key': idempotencyKey
      }
    }
  )
  return response.data
}

/**
 * Consume one administrator-granted reset allowance and reset the current
 * subscription's daily and weekly windows.
 */
export async function resetSubscriptionQuota(
  subscriptionId: number,
  idempotencyKey = createIdempotencyKey('subscription-quota-reset')
): Promise<UserSubscription> {
  const response = await apiClient.post<UserSubscription>(
    `/subscriptions/${subscriptionId}/reset-quota`,
    {},
    {
      headers: {
        'Idempotency-Key': idempotencyKey
      }
    }
  )
  return response.data
}

// Short alias used by callers that share the admin/user quota terminology.
export const resetQuota = resetSubscriptionQuota

export default {
  getMySubscriptions,
  getActiveSubscriptions,
  getSubscriptionsProgress,
  getSubscriptionSummary,
  getSubscriptionProgress,
  earlyResetSubscription,
  resetSubscriptionQuota,
  resetQuota
}
