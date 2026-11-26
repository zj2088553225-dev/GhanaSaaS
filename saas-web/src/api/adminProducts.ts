import request, { type ApiResponse } from '../utils/request'

export type ProductStatus = 'on_shelf' | 'off_shelf'

export interface SpecTemplateItem {
  name: string
  values: string[]
}

export type SKUSpecs = Record<string, string>

export interface AdminSKU {
  id: number
  product_id: number
  name: string
  erp_sku_code: string
  image_url?: string
  specs: SKUSpecs
  retail_price: number
  original_price: number
  sellable_stock: number
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface AdminProduct {
  id: number
  tenant_id: number
  platform_category_id: number
  category_id: number | null
  shop_category_id?: number | null
  name: string
  description: string
  detail_html: string
  main_image: string
  video_url: string
  material: string
  dimensions: string
  weight: string
  slider_images: unknown
  spec_template: SpecTemplateItem[]
  status: ProductStatus
  skus: AdminSKU[]
  translations: AdminProductTranslations
  created_at: string
  updated_at: string
}

export interface AdminProductTranslationFields {
  name: string
  description: string
  detail_html: string
  material: string
  dimensions: string
  weight: string
}

export interface AdminProductTranslations {
  zh: AdminProductTranslationFields
}

export interface AdminSKUInput {
  id?: number
  name: string
  erp_sku_code: string
  image_url?: string
  specs: SKUSpecs
  retail_price: number
  original_price: number
  sellable_stock: number
}

export interface AdminProductPayload {
  platform_category_id: number
  name: string
  description: string
  detail_html: string
  main_image: string
  video_url: string
  material: string
  dimensions: string
  weight: string
  slider_images: string[]
  spec_template: SpecTemplateItem[]
  status: ProductStatus
  skus: AdminSKUInput[]
  translations?: AdminProductTranslations
}

export interface AdminProductQueryParams {
  keyword?: string
  page?: number
  page_size?: number
  category_id?: number
  platform_category_id?: number
  status?: ProductStatus
}

export interface AdminProductPage {
  items: AdminProduct[]
  total: number
  page: number
  page_size: number
}

export interface ProductImportRowError {
  row: number
  field: string
  message: string
}

export interface ProductImportPreviewProduct {
  name: string
  platform_category_path: string
  status: ProductStatus
  sku_count: number
  min_price: number
  max_price: number
  total_stock: number
}

export interface ProductImportPreview {
  rows: number
  valid_rows: number
  invalid_rows: number
  errors: ProductImportRowError[]
  preview_products: ProductImportPreviewProduct[]
}

export interface ProductImportResult {
  imported_products: number
  imported_skus: number
}

export type ProductVideoUploadProgressHandler = (percentage: number | null) => void

export interface ProductVideoUploadResult {
  url: string
  duplicated: boolean
  message?: string
  message_en?: string
}

export async function getAdminProducts(params: AdminProductQueryParams = {}): Promise<AdminProductPage> {
  const response = await request.get<ApiResponse<AdminProductPage>>('/admin/products', { params })
  return response.data.data ?? { items: [], total: 0, page: params.page ?? 1, page_size: params.page_size ?? 20 }
}

export async function getAdminProductDetail(id: number): Promise<AdminProduct> {
  const response = await request.get<ApiResponse<AdminProduct>>(`/admin/products/${id}`)
  if (!response.data.data) {
    throw new Error('Product not found')
  }
  return response.data.data
}

export async function createAdminProduct(payload: AdminProductPayload): Promise<AdminProduct> {
  const response = await request.post<ApiResponse<AdminProduct>>('/admin/products', payload)
  if (!response.data.data) {
    throw new Error('Product creation failed')
  }
  return response.data.data
}

export async function updateAdminProduct(id: number, payload: AdminProductPayload): Promise<AdminProduct> {
  const response = await request.put<ApiResponse<AdminProduct>>(`/admin/products/${id}`, payload)
  if (!response.data.data) {
    throw new Error('Product update failed')
  }
  return response.data.data
}

export async function uploadAdminProductVideo(
  file: File,
  language: string,
  onProgress?: ProductVideoUploadProgressHandler,
): Promise<ProductVideoUploadResult> {
  const form = new FormData()
  form.append('file', file)

  const response = await request.post<ApiResponse<ProductVideoUploadResult>>('/admin/uploads/video', form, {
    timeout: 0,
    headers: {
      'Accept-Language': language,
    },
    onUploadProgress: (progressEvent) => {
      if (!progressEvent.total || progressEvent.total <= 0) {
        onProgress?.(null)
        return
      }
      const percentage = Math.floor((progressEvent.loaded / progressEvent.total) * 100)
      onProgress?.(Math.min(99, Math.max(0, percentage)))
    },
  })

  const result = response.data.data
  if (!result?.url) {
    throw new Error('Video upload returned no URL')
  }
  return result
}

export async function updateAdminProductStatus(id: number, status: ProductStatus): Promise<AdminProduct> {
  const response = await request.patch<ApiResponse<AdminProduct>>(`/admin/products/${id}/status`, { status })
  if (!response.data.data) {
    throw new Error('Product status update failed')
  }
  return response.data.data
}

export async function deleteAdminProduct(id: number): Promise<void> {
  await request.delete(`/admin/products/${id}`)
}

export async function downloadProductImportTemplate(): Promise<Blob> {
  const response = await request.get('/admin/products/import/template', { responseType: 'blob' })
  return response.data as Blob
}

export async function previewProductImport(file: File): Promise<ProductImportPreview> {
  const form = new FormData()
  form.append('file', file)
  const response = await request.post<ApiResponse<ProductImportPreview>>('/admin/products/import/preview', form)
  if (!response.data.data) {
    throw new Error('Excel 解析失败')
  }
  return response.data.data
}

export async function confirmProductImport(file: File): Promise<ProductImportResult> {
  const form = new FormData()
  form.append('file', file)
  const response = await request.post<ApiResponse<ProductImportResult>>('/admin/products/import/confirm', form)
  if (!response.data.data) {
    throw new Error('商品导入失败')
  }
  return response.data.data
}
