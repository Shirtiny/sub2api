import { apiClient } from '../client'
import type { BasePaginationResponse } from '@/types'
import type { OrderStatus } from '@/types/payment'

export interface CafeCampaign {
  id: number; code: string; name: string; discount_percent: number; enabled: boolean
  starts_at: string; expires_at: string; created_by: number; created_at: string; updated_at: string
}
export interface CreateCafeCampaign {
  code: string; name: string; discount_percent: number; start_date: string; end_date: string; enabled: boolean
}
export interface CafeCampaignUse {
  user_id: number; user_email: string; order_id: number; order_status: OrderStatus
  used_at?: string; paid_at?: string; expires_at: string; discount_amount: number; created_at: string
}
export const cafeCampaignAPI = {
  list: (page = 1, page_size = 20) => apiClient.get<BasePaginationResponse<CafeCampaign>>('/admin/promo-codes/cafe-campaigns', { params: { page, page_size } }),
  create: (data: CreateCafeCampaign) => apiClient.post<CafeCampaign>('/admin/promo-codes/cafe-campaigns', data),
  setEnabled: (row: CafeCampaign, enabled: boolean) => apiClient.patch<CafeCampaign>(`/admin/promo-codes/cafe-campaigns/${row.id}/status`, { enabled, expected_updated_at: row.updated_at }),
  uses: (id: number, page = 1, page_size = 20) => apiClient.get<BasePaginationResponse<CafeCampaignUse>>(`/admin/promo-codes/cafe-campaigns/${id}/usages`, { params: { page, page_size } }),
}
