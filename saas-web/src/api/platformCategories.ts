import request, { type ApiResponse } from '../utils/request'
import { DEFAULT_LOCALE, localeFromPath } from '../utils/localePath.ts'
import { buildPlatformCategorySearchParams, type PlatformCategorySearchLocale } from '../utils/platformCategorySearch.ts'

export const PLATFORM_CATEGORY_IMPORT_TIMEOUT_MS = 180000

export type PlatformCategoryStatus = 'active' | 'disabled'

export interface PlatformCategory {
  id: number
  parent_id: number | null
  name: string
  path: string
  zh_name?: string
  zh_path?: string
  level: number
  sort_order: number
  status: PlatformCategoryStatus
  is_leaf: boolean
  has_children?: boolean
  children?: PlatformCategory[]
  translations: PlatformCategoryTranslations
  created_at: string
  updated_at: string
}

export interface PlatformCategoryTranslationFields {
  name: string
}

export interface PlatformCategoryTranslations {
  zh: PlatformCategoryTranslationFields
}

export interface PlatformCategoryPayload {
  parent_id?: number | null
  name: string
  sort_order: number
  status: PlatformCategoryStatus
  translations?: PlatformCategoryTranslations
}

export interface PlatformCategoryListOptions {
  maxLevel?: number
}

export interface PlatformCategorySearchOptions {
  limit?: number
  locale?: PlatformCategorySearchLocale
  leafOnly?: boolean
  signal?: AbortSignal
}

export interface PlatformCategorySearchRebuildSummary {
  categories: number
  expected_rows: number
  created: number
  updated: number
  unchanged: number
  deleted_orphans: number
  missing_translations: number
  fallback_zh_rows: number
  invalid_rows: number
  verified: boolean
}

export interface PlatformCategoryImportRowError {
  row: number
  field: string
  message: string
  message_en?: string
}

export interface PlatformCategoryImportItem {
  row: number
  path: string
  level: number
  sort_order: number
  status: PlatformCategoryStatus
  action: 'create' | 'update' | 'skip'
  normalized: boolean
  normalization_note?: string
  error?: string
}

export interface PlatformCategoryImportPreview {
  rows: number
  valid_rows: number
  invalid_rows: number
  errors: PlatformCategoryImportRowError[]
  preview_categories: PlatformCategoryImportItem[]
}

export interface PlatformCategoryImportResult {
  created: number
  updated: number
  unchanged: number
}

export async function getAdminPlatformCategories(): Promise<PlatformCategory[]> {
  const response = await request.get<ApiResponse<PlatformCategory[]>>('/admin/platform-categories')
  return response.data.data ?? []
}

export async function getStorefrontPlatformCategories(options: PlatformCategoryListOptions = {}): Promise<PlatformCategory[]> {
  const response = await request.get<ApiResponse<PlatformCategory[]>>('/storefront/platform-categories', {
    params: platformCategoryListParams(options),
  })
  return response.data.data ?? []
}

export async function getSuperPlatformCategories(): Promise<PlatformCategory[]> {
  const response = await request.get<ApiResponse<PlatformCategory[]>>('/super/platform-categories')
  return response.data.data ?? []
}

async function getPlatformCategoryChildren(
  endpoint: '/admin/platform-categories' | '/super/platform-categories' | '/storefront/platform-categories',
  parentId: number | null = null,
  options: PlatformCategoryListOptions = {},
): Promise<PlatformCategory[]> {
  const response = await request.get<ApiResponse<PlatformCategory[]>>(endpoint, {
    params: { parent_id: parentId ?? 0, ...platformCategoryListParams(options) },
  })
  return response.data.data ?? []
}

async function searchPlatformCategories(
  endpoint: '/admin/platform-categories/search' | '/super/platform-categories/search',
  keyword: string,
  options: PlatformCategorySearchOptions = {},
): Promise<PlatformCategory[]> {
  const limit = options.limit ?? 50
  const locale = options.locale ?? 'all'
  const response = await request.get<ApiResponse<PlatformCategory[]>>(endpoint, {
    signal: options.signal,
    params: buildPlatformCategorySearchParams(keyword, { limit, locale, leafOnly: options.leafOnly }),
  })
  return response.data.data ?? []
}

async function getPlatformCategoryById(
  endpoint: '/admin/platform-categories' | '/super/platform-categories',
  id: number,
): Promise<PlatformCategory> {
  const response = await request.get<ApiResponse<PlatformCategory>>(`${endpoint}/${id}`)
  if (!response.data.data) {
    throw new Error('平台类目不存在')
  }
  return response.data.data
}

export function getAdminPlatformCategoryChildren(parentId: number | null = null): Promise<PlatformCategory[]> {
  return getPlatformCategoryChildren('/admin/platform-categories', parentId)
}

export function getSuperPlatformCategoryChildren(parentId: number | null = null): Promise<PlatformCategory[]> {
  return getPlatformCategoryChildren('/super/platform-categories', parentId)
}

const storefrontChildrenCache = new Map<string, Promise<PlatformCategory[]>>()

export function getStorefrontPlatformCategoryChildren(
  parentId: number | null = null,
  options: PlatformCategoryListOptions = {},
): Promise<PlatformCategory[]> {
  const cacheKey = `${currentStorefrontLocale()}:${parentId ?? 0}:${options.maxLevel ?? 'all'}`
  const cached = storefrontChildrenCache.get(cacheKey)
  if (cached) {
    return cached
  }
  const request = getPlatformCategoryChildren('/storefront/platform-categories', parentId, options)
    .catch((error) => {
      storefrontChildrenCache.delete(cacheKey)
      throw error
    })
  storefrontChildrenCache.set(cacheKey, request)
  return request
}

function currentStorefrontLocale() {
  if (typeof window === 'undefined') {
    return DEFAULT_LOCALE
  }
  return localeFromPath(window.location.pathname) ?? DEFAULT_LOCALE
}

function platformCategoryListParams(options: PlatformCategoryListOptions) {
  return options.maxLevel ? { max_level: options.maxLevel } : undefined
}

export function searchAdminPlatformCategories(
  keyword: string,
  limitOrOptions: number | PlatformCategorySearchOptions = 50,
  options: Omit<PlatformCategorySearchOptions, 'limit'> = {},
): Promise<PlatformCategory[]> {
  const searchOptions = typeof limitOrOptions === 'number'
    ? { ...options, limit: limitOrOptions }
    : limitOrOptions
  return searchPlatformCategories('/admin/platform-categories/search', keyword, searchOptions)
}

export function searchSuperPlatformCategories(
  keyword: string,
  limitOrOptions: number | PlatformCategorySearchOptions = 50,
): Promise<PlatformCategory[]> {
  const searchOptions = typeof limitOrOptions === 'number' ? { limit: limitOrOptions, locale: 'all' as const } : limitOrOptions
  return searchPlatformCategories('/super/platform-categories/search', keyword, searchOptions)
}

export function getAdminPlatformCategoryById(id: number): Promise<PlatformCategory> {
  return getPlatformCategoryById('/admin/platform-categories', id)
}

export function getSuperPlatformCategoryById(id: number): Promise<PlatformCategory> {
  return getPlatformCategoryById('/super/platform-categories', id)
}

export async function createPlatformCategory(payload: PlatformCategoryPayload): Promise<PlatformCategory> {
  const response = await request.post<ApiResponse<PlatformCategory>>('/super/platform-categories', payload)
  if (!response.data.data) {
    throw new Error('平台类目创建失败')
  }
  return response.data.data
}

export async function updatePlatformCategory(id: number, payload: PlatformCategoryPayload): Promise<PlatformCategory> {
  const response = await request.put<ApiResponse<PlatformCategory>>(`/super/platform-categories/${id}`, payload)
  if (!response.data.data) {
    throw new Error('平台类目更新失败')
  }
  return response.data.data
}

export async function deletePlatformCategory(id: number): Promise<void> {
  await request.delete(`/super/platform-categories/${id}`)
}

export async function rebuildPlatformCategorySearchIndex(): Promise<PlatformCategorySearchRebuildSummary> {
  const response = await request.post<ApiResponse<PlatformCategorySearchRebuildSummary>>(
    '/super/platform-categories/rebuild-search-index',
    undefined,
    { timeout: PLATFORM_CATEGORY_IMPORT_TIMEOUT_MS },
  )
  if (!response.data.data) {
    throw new Error('平台类目搜索索引重建失败')
  }
  return response.data.data
}

export type PlatformCategoryImportTemplateFormat = 'levels' | 'path'

export async function downloadPlatformCategoryImportTemplate(format: PlatformCategoryImportTemplateFormat = 'path'): Promise<Blob> {
  const endpoint = format === 'path'
    ? '/super/platform-categories/import-template'
    : '/super/platform-categories/import/template'
  const response = await request.get(endpoint, {
    params: format === 'path' ? undefined : { format },
    responseType: 'blob',
  })
  return response.data as Blob
}

export async function exportPlatformCategoryBilingualTable(): Promise<Blob> {
  const response = await request.get('/super/platform-categories/export', {
    responseType: 'blob',
  })
  return response.data as Blob
}

export async function previewPlatformCategoryImport(file: File): Promise<PlatformCategoryImportPreview> {
  const body = new FormData()
  body.append('file', file)
  const response = await request.post<ApiResponse<PlatformCategoryImportPreview>>('/super/platform-categories/import/preview', body)
  if (!response.data.data) {
    throw new Error('平台类目 Excel 解析失败')
  }
  return response.data.data
}

export async function confirmPlatformCategoryImport(file: File): Promise<PlatformCategoryImportResult> {
  const body = new FormData()
  body.append('file', file)
  const response = await request.post<ApiResponse<PlatformCategoryImportResult>>('/super/platform-categories/import', body, {
    timeout: PLATFORM_CATEGORY_IMPORT_TIMEOUT_MS,
  })
  if (!response.data.data) {
    throw new Error('平台类目导入失败')
  }
  return response.data.data
}
