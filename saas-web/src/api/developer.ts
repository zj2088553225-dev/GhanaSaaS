import request, { type ApiResponse } from '../utils/request'

export interface DeveloperCredentials {
  app_key: string
  app_secret: string
}

export async function getDeveloperCredentials(): Promise<DeveloperCredentials> {
  const response = await request.get<ApiResponse<DeveloperCredentials>>('/admin/settings/developer')
  if (!response.data.data) {
    throw new Error('Developer credentials not found')
  }
  return response.data.data
}

export async function regenerateDeveloperSecret(): Promise<DeveloperCredentials> {
  const response = await request.post<ApiResponse<DeveloperCredentials>>('/admin/settings/developer/regenerate-secret')
  if (!response.data.data) {
    throw new Error('Secret regeneration failed')
  }
  return response.data.data
}
