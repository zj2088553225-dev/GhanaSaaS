import request, { type ApiResponse } from '../utils/request'

export interface StorefrontTrackingSetting {
  meta_pixel_enabled: boolean
  meta_pixel_id: string
  custom_head_enabled: boolean
  custom_head_html: string
}

export async function getStorefrontTrackingSetting(): Promise<StorefrontTrackingSetting> {
  const response = await request.get<ApiResponse<StorefrontTrackingSetting>>('/super/storefront-tracking')
  return response.data.data ?? emptyStorefrontTrackingSetting()
}

export async function saveStorefrontTrackingSetting(
  setting: StorefrontTrackingSetting,
): Promise<StorefrontTrackingSetting> {
  const response = await request.put<ApiResponse<StorefrontTrackingSetting>>('/super/storefront-tracking', setting)
  if (!response.data.data) {
    throw new Error('Tracking setting save failed')
  }
  return response.data.data
}

export async function getPublicStorefrontTracking(): Promise<StorefrontTrackingSetting> {
  const response = await request.get<ApiResponse<StorefrontTrackingSetting>>('/storefront/tracking')
  return response.data.data ?? emptyStorefrontTrackingSetting()
}

function emptyStorefrontTrackingSetting(): StorefrontTrackingSetting {
  return {
    meta_pixel_enabled: false,
    meta_pixel_id: '',
    custom_head_enabled: false,
    custom_head_html: '',
  }
}
