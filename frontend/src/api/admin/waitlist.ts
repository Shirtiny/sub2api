import apiClient from '../client'
import type { BasePaginationResponse } from '@/types'

export interface WaitlistEntry {
  id: number
  email: string
  created_at: string
  approved_at: string | null
  approved_by: number | null
  granted_user_id: number | null
  approval_notice_sent_at: string | null
}

export async function listWaitlist(page: number, pageSize: number, signal?: AbortSignal): Promise<BasePaginationResponse<WaitlistEntry>> {
  const { data } = await apiClient.get<BasePaginationResponse<WaitlistEntry>>('/admin/waitlist', {
    params: { page, page_size: pageSize }, signal
  })
  return data
}

export async function approveWaitlist(id: number): Promise<void> {
  const { data } = await apiClient.post<{ approved: boolean }>(`/admin/waitlist/${id}/approve`, {}, { timeout: 60000 })
  if (data?.approved !== true) throw new Error('Waiting list approval not acknowledged')
}
