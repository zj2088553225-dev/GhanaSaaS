import request, { type ApiResponse } from '../utils/request'
import { reservationEnabled, reservationMode } from '../config/storefront'

export type ReservationHeatLevel = 'new' | 'popular' | 'trending' | 'highly_requested'
export type ReservationProductStatus = 'draft' | 'scheduled' | 'open' | 'closed' | 'archived'
export type ReservationContactType = 'sms' | 'whatsapp'

export interface ReservationViewerState {
  interested: boolean
  verified_reserved: boolean
}

export interface ReservationProductPublic {
  id: number
  slug: string
  title: string
  title_en: string
  title_zh: string | null
  summary: string
  summary_en: string
  summary_zh: string | null
  description: string
  description_en: string
  description_zh: string | null
  main_image_url: string
  gallery: string[]
  reservation_start_at: string | null
  reservation_end_at: string | null
  is_featured: boolean
  heat_level: ReservationHeatLevel
  viewer_state: ReservationViewerState
}

export interface ReservationProductPublicPage {
  items: ReservationProductPublic[]
  total: number
  page: number
  page_size: number
}

export interface ReservationProductQuery {
  page?: number
  page_size?: number
  search?: string
  featured?: boolean
  visitor_id?: string
}

export interface ReservationInterestPayload {
  visitor_id: string
  session_id?: string | null
  idempotency_key: string
  source_type: 'reservation_list' | 'reservation_detail'
  source_id?: string | null
}

export interface ReservationInterestResult {
  interested: true
  created: boolean
}

export interface ReservationCapabilities {
  reservation_enabled: boolean
  reservation_mode: 'disabled' | 'anonymous' | 'otp'
  otp_channels: ReservationContactType[]
  turnstile_enabled: boolean
}

export type ReservationRecordStatus = 'active' | 'pending' | 'confirmed' | 'cancelled' | 'rejected' | 'expired'
export type ReservationIdentityType = 'anonymous' | 'phone_verified'

export interface AnonymousReservationProduct {
  id: number
  slug: string
  title: string
  main_image_url: string
}

export interface AnonymousReservation {
  reservation_code: string
  identity_type: ReservationIdentityType
  status: ReservationRecordStatus
  quantity: number
  note?: string
  created_at: string
  expires_at: string | null
  product: AnonymousReservationProduct
}

export interface AnonymousReservationCreatePayload {
  reservation_product_id?: number
  quantity: number
  note?: string
  turnstile_token: string
}

export interface ReservationOTPRequestPayload {
  visitor_id: string
  idempotency_key: string
  contact_type: ReservationContactType
  phone_country: string
  phone_number: string
  turnstile_token: string
  source_type: 'reservation_detail' | 'reservation_dialog'
  source_id?: string | null
}

export interface ReservationOTPRequestResult {
  challenge_id: string
  status: 'pending'
  resend_after_seconds: number
}

export interface ReservationOTPVerifyPayload {
  challenge_id: string
  code: string
  idempotency_key: string
}

export interface ReservationOTPVerifyResult {
  reserved: true
  status: 'verified'
  manage_token?: string
}

export interface ReservationCancelResult {
  cancelled: true
  status: 'cancelled'
}

export interface ReservationProductAdmin {
  id: number
  linked_product_id: number | null
  slug: string
  title_en: string
  title_zh: string | null
  summary_en: string
  summary_zh: string | null
  description_en: string
  description_zh: string | null
  main_image_url: string
  gallery: string[]
  status: ReservationProductStatus
  reservation_start_at: string | null
  reservation_end_at: string | null
  display_order: number
  is_featured: boolean
  heat_level_override: ReservationHeatLevel | null
  automatic_heat_level: ReservationHeatLevel
  effective_heat_level: ReservationHeatLevel
  exact_interest_count: number
  exact_verified_count: number
  created_by: number
  created_at: string
  updated_at: string
}

export interface ReservationProductAdminPage {
  items: ReservationProductAdmin[]
  total: number
  page: number
  page_size: number
}

export interface ReservationProductPayload {
  linked_product_id: number | null
  slug: string
  title_en: string
  title_zh: string | null
  summary_en: string
  summary_zh: string | null
  description_en: string
  description_zh: string | null
  main_image_url: string
  gallery: string[]
  status: ReservationProductStatus
  reservation_start_at: string | null
  reservation_end_at: string | null
  display_order: number
  is_featured: boolean
  heat_level_override: ReservationHeatLevel | null
}

export interface ReservationContactAdmin {
  masked_phone: string
  channel: ReservationContactType
  verified_at: string | null
  status: 'active' | 'cancelled'
}

export interface ReservationRecordAdmin {
  reservation_code: string
  identity_type: ReservationIdentityType
  status: ReservationRecordStatus
  quantity: number
  masked_phone?: string
  channel?: ReservationContactType
  browser_hash_prefix?: string
  ip_hash_prefix?: string
  browser_reservation_count: number
  recent_ip_reservation_count: number
  rate_limit_flagged: boolean
  expires_at?: string | null
  risk_flagged: boolean
  risk_flagged_at?: string | null
  created_at: string
  product: AnonymousReservationProduct
}

export interface ReservationRecordAdminPage {
  items: ReservationRecordAdmin[]
  total: number
  page: number
  page_size: number
}

export interface ReservationProductAnalytics {
  impressions: number
  page_views: number
  unique_interests: number
  reservation_submissions: number
  verified_reservations: number
  verification_rate: number
  cancelled_reservations: number
  blocked_attempts: number
  contacts: ReservationContactAdmin[]
}

function requiredData<T>(response: ApiResponse<T>, message: string): T {
  if (response.data === undefined) {
    throw new Error(message)
  }
  return response.data
}

function requireReservationEnabled() {
  if (!reservationEnabled) {
    throw new Error('Reservation feature is disabled')
  }
}

function requireReservationMode(expected: 'anonymous' | 'otp') {
  requireReservationEnabled()
  if (reservationMode !== expected) {
    throw new Error(`Reservation mode ${expected} is not enabled`)
  }
}

export async function getReservationProducts(params: ReservationProductQuery = {}): Promise<ReservationProductPublicPage> {
  requireReservationEnabled()
  const response = await request.get<ApiResponse<ReservationProductPublicPage>>('/storefront/reservation-products', { params })
  return response.data.data ?? { items: [], total: 0, page: params.page ?? 1, page_size: params.page_size ?? 20 }
}

export async function getReservationProduct(slug: string, visitorID?: string): Promise<ReservationProductPublic> {
  requireReservationEnabled()
  const response = await request.get<ApiResponse<ReservationProductPublic>>(`/storefront/reservation-products/${encodeURIComponent(slug)}`, {
    params: visitorID ? { visitor_id: visitorID } : undefined,
  })
  return requiredData(response.data, 'Reservation product is unavailable')
}

export async function createReservationInterest(productID: number, payload: ReservationInterestPayload): Promise<ReservationInterestResult> {
  requireReservationEnabled()
  const response = await request.post<ReservationInterestResult>(`/storefront/reservation-products/${productID}/interest`, payload)
  return response.data
}

export async function getReservationCapabilities(): Promise<ReservationCapabilities> {
  requireReservationEnabled()
  const response = await request.get<ApiResponse<ReservationCapabilities>>('/storefront/reservation-capabilities')
  return requiredData(response.data, 'Reservation verification is unavailable')
}

export async function requestReservationOTP(productID: number, payload: ReservationOTPRequestPayload): Promise<ReservationOTPRequestResult> {
  requireReservationMode('otp')
  const response = await request.post<ReservationOTPRequestResult>(`/storefront/reservation-products/${productID}/otp/request`, payload)
  return response.data
}

export async function verifyReservationOTP(productID: number, payload: ReservationOTPVerifyPayload): Promise<ReservationOTPVerifyResult> {
  requireReservationMode('otp')
  const response = await request.post<ReservationOTPVerifyResult>(`/storefront/reservation-products/${productID}/otp/verify`, payload)
  return response.data
}

export async function cancelReservation(productID: number, manageToken: string, idempotencyKey: string): Promise<ReservationCancelResult> {
  requireReservationMode('otp')
  const response = await request.post<ReservationCancelResult>(`/storefront/reservation-products/${productID}/cancel`, {
    manage_token: manageToken,
    idempotency_key: idempotencyKey,
  })
  return response.data
}

export async function createAnonymousReservation(productID: number, payload: AnonymousReservationCreatePayload): Promise<AnonymousReservation> {
  requireReservationMode('anonymous')
  const response = await request.post<ApiResponse<AnonymousReservation>>('/storefront/reservations', {
    ...payload,
    reservation_product_id: productID,
  })
  return requiredData(response.data, 'Anonymous reservation could not be created')
}

export async function getMyAnonymousReservations(): Promise<AnonymousReservation[]> {
  requireReservationMode('anonymous')
  const response = await request.get<ApiResponse<AnonymousReservation[]>>('/storefront/my-reservations')
  return response.data.data ?? []
}

export async function cancelAnonymousReservation(reservationCode: string): Promise<AnonymousReservation> {
  requireReservationMode('anonymous')
  const response = await request.post<ApiResponse<AnonymousReservation>>(`/storefront/my-reservations/${encodeURIComponent(reservationCode)}/cancel`)
  return requiredData(response.data, 'Reservation could not be cancelled')
}

export async function getSuperReservationProducts(params: { page?: number; page_size?: number; search?: string; status?: ReservationProductStatus } = {}): Promise<ReservationProductAdminPage> {
  requireReservationEnabled()
  const response = await request.get<ApiResponse<ReservationProductAdminPage>>('/super/reservation-products', { params })
  return response.data.data ?? { items: [], total: 0, page: params.page ?? 1, page_size: params.page_size ?? 20 }
}

export async function getSuperReservationProduct(id: number): Promise<ReservationProductAdmin> {
  requireReservationEnabled()
  const response = await request.get<ApiResponse<ReservationProductAdmin>>(`/super/reservation-products/${id}`)
  return requiredData(response.data, '预约商品不存在')
}

export async function createSuperReservationProduct(payload: ReservationProductPayload): Promise<ReservationProductAdmin> {
  requireReservationEnabled()
  const response = await request.post<ApiResponse<ReservationProductAdmin>>('/super/reservation-products', payload)
  return requiredData(response.data, '预约商品创建失败')
}

export async function updateSuperReservationProduct(id: number, payload: ReservationProductPayload): Promise<ReservationProductAdmin> {
  requireReservationEnabled()
  const response = await request.put<ApiResponse<ReservationProductAdmin>>(`/super/reservation-products/${id}`, payload)
  return requiredData(response.data, '预约商品更新失败')
}

export async function archiveSuperReservationProduct(id: number): Promise<void> {
  requireReservationEnabled()
  await request.delete(`/super/reservation-products/${id}`)
}

export async function getSuperReservationProductAnalytics(id: number): Promise<ReservationProductAnalytics> {
  requireReservationEnabled()
  const response = await request.get<ApiResponse<ReservationProductAnalytics>>(`/super/reservation-products/${id}/analytics`)
  return requiredData(response.data, '预约分析加载失败')
}

export async function getSuperReservations(params: {
  page?: number
  page_size?: number
  identity_type?: ReservationIdentityType
  status?: ReservationRecordStatus
  risk_flagged?: boolean
} = {}): Promise<ReservationRecordAdminPage> {
  requireReservationEnabled()
  const response = await request.get<ApiResponse<ReservationRecordAdminPage>>('/super/reservations', { params })
  return response.data.data ?? { items: [], total: 0, page: params.page ?? 1, page_size: params.page_size ?? 20 }
}

export async function setSuperReservationStatus(
  reservationCode: string,
  status: 'confirmed' | 'rejected' | 'cancelled',
): Promise<ReservationRecordAdmin> {
  requireReservationEnabled()
  const response = await request.patch<ApiResponse<ReservationRecordAdmin>>(`/super/reservations/${encodeURIComponent(reservationCode)}/status`, { status })
  return requiredData(response.data, '预约状态更新失败')
}

export async function setSuperReservationRisk(reservationCode: string, flagged: boolean): Promise<ReservationRecordAdmin> {
  requireReservationEnabled()
  const response = await request.patch<ApiResponse<ReservationRecordAdmin>>(`/super/reservations/${encodeURIComponent(reservationCode)}/risk`, { flagged })
  return requiredData(response.data, '风险标记更新失败')
}

export async function expireSuperReservations(): Promise<number> {
  requireReservationEnabled()
  const response = await request.post<ApiResponse<{ expired: number }>>('/super/reservations/expire')
  return response.data.data?.expired ?? 0
}
