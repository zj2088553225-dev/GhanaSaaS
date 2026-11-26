import request, { type ApiResponse } from '../utils/request'

export interface ShopProfile {
  id?: number
  tenant_id?: number
  slug: string
  shop_name: string
  logo_url: string
  banner_url: string
  description: string
  phone: string
  whatsapp: string
  address: string
  business_hours: string
  is_public: boolean
  translations: ShopProfileTranslations
}

export interface ShopProfileTranslationFields {
  shop_name: string
  description: string
  address: string
  business_hours: string
}

export interface ShopProfileTranslations {
  zh: ShopProfileTranslationFields
}

export type ShopProfilePayload = Omit<ShopProfile, 'id' | 'tenant_id'>

const emptyProfile: ShopProfile = {
  slug: '',
  shop_name: '',
  logo_url: '',
  banner_url: '',
  description: '',
  phone: '',
  whatsapp: '',
  address: '',
  business_hours: '',
  is_public: false,
  translations: {
    zh: {
      shop_name: '',
      description: '',
      address: '',
      business_hours: '',
    },
  },
}

export async function getShopProfile(): Promise<ShopProfile> {
  const response = await request.get<ApiResponse<ShopProfile>>('/admin/shop-profile')
  return { ...emptyProfile, ...(response.data.data ?? {}) }
}

export async function updateShopProfile(payload: ShopProfilePayload): Promise<ShopProfile> {
  const response = await request.put<ApiResponse<ShopProfile>>('/admin/shop-profile', payload)
  if (!response.data.data) {
    throw new Error('Shop profile update failed')
  }
  return { ...emptyProfile, ...response.data.data }
}
