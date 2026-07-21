/**
 * Admin Subscriptions API endpoints
 * Handles user subscription management for administrators
 */

import { apiClient } from '../client'
import type {
  UserSubscription,
  SubscriptionProgress,
  AssignSubscriptionRequest,
  BulkAssignSubscriptionRequest,
  BulkShiftWindowRequest,
  BulkShiftWindowResult,
  ExtendSubscriptionRequest,
  PaginatedResponse,
  SubscriptionStats,
  SubscriptionUsageSeries
} from '@/types'
import { createIdempotencyKey } from '@/utils/idempotency'

/**
 * List all subscriptions with pagination
 * @param page - Page number (default: 1)
 * @param pageSize - Items per page (default: 20)
 * @param filters - Optional filters (status, user_id, group_id, sort_by, sort_order)
 * @returns Paginated list of subscriptions
 */
export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    status?: 'active' | 'expired' | 'revoked'
    user_id?: number
    group_id?: number
    platform?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  },
  options?: {
    signal?: AbortSignal
  }
): Promise<PaginatedResponse<UserSubscription>> {
  const { data } = await apiClient.get<PaginatedResponse<UserSubscription>>(
    '/admin/subscriptions',
    {
      params: {
        page,
        page_size: pageSize,
        ...filters
      },
      signal: options?.signal
    }
  )
  return data
}

/**
 * Get subscription by ID
 * @param id - Subscription ID
 * @returns Subscription details
 */
export async function getById(id: number): Promise<UserSubscription> {
  const { data } = await apiClient.get<UserSubscription>(`/admin/subscriptions/${id}`)
  return data
}

/**
 * Get subscription progress
 * @param id - Subscription ID
 * @returns Subscription progress with usage stats
 */
export async function getProgress(id: number): Promise<SubscriptionProgress> {
  const { data } = await apiClient.get<SubscriptionProgress>(`/admin/subscriptions/${id}/progress`)
  return data
}

/**
 * Assign subscription to user
 * @param request - Assignment request
 * @returns Created subscription
 */
export async function assign(request: AssignSubscriptionRequest): Promise<UserSubscription> {
  const { data } = await apiClient.post<UserSubscription>('/admin/subscriptions/assign', request)
  return data
}

/**
 * Bulk assign subscriptions to multiple users
 * @param request - Bulk assignment request
 * @returns Created subscriptions
 */
export async function bulkAssign(
  request: BulkAssignSubscriptionRequest
): Promise<UserSubscription[]> {
  const { data } = await apiClient.post<UserSubscription[]>(
    '/admin/subscriptions/bulk-assign',
    request
  )
  return data
}

/**
 * Extend subscription validity
 * @param id - Subscription ID
 * @param request - Extension request with days
 * @returns Updated subscription
 */
export async function extend(
  id: number,
  request: ExtendSubscriptionRequest
): Promise<UserSubscription> {
  const { data } = await apiClient.post<UserSubscription>(
    `/admin/subscriptions/${id}/extend`,
    request
  )
  return data
}

/**
 * Revoke subscription
 * @param id - Subscription ID
 * @returns Success confirmation
 */
export async function revoke(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/subscriptions/${id}`)
  return data
}

/**
 * Reset daily, weekly, and/or monthly usage quota for a subscription
 * @param id - Subscription ID
 * @param options - Which windows to reset
 * @returns Updated subscription
 */
export async function resetQuota(
  id: number,
  options: { daily: boolean; weekly: boolean; monthly: boolean }
): Promise<UserSubscription> {
  const { data } = await apiClient.post<UserSubscription>(
    `/admin/subscriptions/${id}/reset-quota`,
    options
  )
  return data
}

export async function bulkResetQuota(
  options: { daily: boolean; weekly: boolean; monthly: boolean }
): Promise<{ count: number }> {
  const { data } = await apiClient.post<{ count: number }>(
    '/admin/subscriptions/bulk-reset-quota',
    options
  )
  return data
}

/**
 * Shift the reset window start of many subscriptions at once
 * @param request - Which windows to move, the hour offset, and the filter scope.
 *                  Pass `dry_run: true` to preview the affected count without writing.
 * @returns Matched / updated / skipped counts
 */
export async function bulkShiftWindow(
  request: BulkShiftWindowRequest
): Promise<BulkShiftWindowResult> {
  // 后端对写入路径包了幂等协调器，但 key 只从请求头取——不带头就完全不去重，
  // 双击或网络重试会把窗口平移两次（+14h 变 +28h）。每次调用生成新 key：
  // 同一次提交的重试被去重，用户有意的第二次提交仍能生效。
  // dry_run 是只读预览，不需要幂等键。
  const headers = request.dry_run
    ? undefined
    : { 'Idempotency-Key': createIdempotencyKey('subscription-shift-window') }
  const { data } = await apiClient.post<BulkShiftWindowResult>(
    '/admin/subscriptions/bulk-shift-window',
    request,
    { headers }
  )
  return data
}

/**
 * Get aggregated subscription statistics (remaining quota, per-plan breakdown, usage ranking)
 * @param params - horizon_days (1|3|7|14|30, default 7) and ranking_limit (1..100, default 20)
 * @returns Aggregated stats snapshot
 */
export async function getStats(
  params?: { horizon_days?: number; ranking_limit?: number },
  options?: { signal?: AbortSignal }
): Promise<SubscriptionStats> {
  const { data } = await apiClient.get<SubscriptionStats>('/admin/subscriptions/stats', {
    params,
    signal: options?.signal
  })
  return data
}

/**
 * Get the daily / weekly / whole-cycle usage series for one subscription
 * @param id - Subscription ID
 * @returns Usage series with per-day, per-week and cycle-level usage ratios
 */
export async function getUsageSeries(
  id: number,
  options?: { signal?: AbortSignal }
): Promise<SubscriptionUsageSeries> {
  const { data } = await apiClient.get<SubscriptionUsageSeries>(
    `/admin/subscriptions/${id}/usage-series`,
    { signal: options?.signal }
  )
  return data
}

/**
 * List subscriptions by group
 * @param groupId - Group ID
 * @param page - Page number
 * @param pageSize - Items per page
 * @returns Paginated list of subscriptions in the group
 */
export async function listByGroup(
  groupId: number,
  page: number = 1,
  pageSize: number = 20
): Promise<PaginatedResponse<UserSubscription>> {
  const { data } = await apiClient.get<PaginatedResponse<UserSubscription>>(
    `/admin/groups/${groupId}/subscriptions`,
    {
      params: { page, page_size: pageSize }
    }
  )
  return data
}

/**
 * List subscriptions by user
 * @param userId - User ID
 * @param page - Page number
 * @param pageSize - Items per page
 * @returns Paginated list of user's subscriptions
 */
export async function listByUser(
  userId: number,
  page: number = 1,
  pageSize: number = 20
): Promise<PaginatedResponse<UserSubscription>> {
  const { data } = await apiClient.get<PaginatedResponse<UserSubscription>>(
    `/admin/users/${userId}/subscriptions`,
    {
      params: { page, page_size: pageSize }
    }
  )
  return data
}

export const subscriptionsAPI = {
  list,
  getById,
  getProgress,
  assign,
  bulkAssign,
  extend,
  revoke,
  resetQuota,
  bulkResetQuota,
  bulkShiftWindow,
  getStats,
  getUsageSeries,
  listByGroup,
  listByUser
}

export default subscriptionsAPI
