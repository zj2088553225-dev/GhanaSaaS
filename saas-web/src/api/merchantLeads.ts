import request, { type ApiResponse } from '../utils/request'

export type MerchantLeadStatus = 'new' | 'contacted' | 'approved' | 'rejected'

export interface MerchantLeadPayload {
  contact_name: string
  shop_name: string
  whatsapp: string
  phone?: string
  category: string
  has_offline_shop?: boolean
  city?: string
  product_count?: string
  message?: string
}

export interface MerchantLead extends MerchantLeadPayload {
  id: number
  phone: string
  has_offline_shop: boolean
  city: string
  product_count: string
  message: string
  status: MerchantLeadStatus
  created_at: string
  updated_at: string
}

interface MerchantLeadSubmitResponse {
  message?: string
}

export async function submitMerchantLead(payload: MerchantLeadPayload): Promise<string> {
  const response = await request.post<MerchantLeadSubmitResponse>('/storefront/merchant-leads', payload)
  return response.data.message || 'Application submitted successfully'
}

export async function getMerchantLeads(): Promise<MerchantLead[]> {
  const response = await request.get<ApiResponse<MerchantLead[]>>('/super/merchant-leads')
  return response.data.data ?? []
}

export async function updateMerchantLeadStatus(id: number, status: MerchantLeadStatus): Promise<MerchantLead> {
  const response = await request.patch<ApiResponse<MerchantLead>>(`/super/merchant-leads/${id}/status`, { status })
  if (!response.data.data) {
    throw new Error('Merchant lead update failed')
  }
  return response.data.data
}
