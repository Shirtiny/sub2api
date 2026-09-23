import apiClient from '../client'
import type { BasePaginationResponse } from '@/types'

export interface WaitlistEntry { id: number; email: string; created_at: string }

export async function listWaitlist(page: number, pageSize: number, signal?: AbortSignal): Promise<BasePaginationResponse<WaitlistEntry>> {
  const { data } = await apiClient.get<BasePaginationResponse<WaitlistEntry>>('/admin/waitlist', {
    params: { page, page_size: pageSize }, signal
  })
  return data
}
