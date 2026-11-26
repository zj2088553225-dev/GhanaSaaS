import request, { type ApiResponse } from '../utils/request'
import type { MallSource } from '../utils/mallSource'

export interface Category {
  id: number
  parent_id: number | null
  name: string
  sort: number
  icon: string
  children: Category[]
}

export interface SpecTemplateItem {
  name: string
  values: string[]
}

export type SKUSpecs = Record<string, string>

export interface SKU {
  id: number
  product_id: number
  name: string
  erp_sku_code: string
  image_url?: string
  image?: string
  specs: SKUSpecs
  retail_price: number
  original_price: number
  sellable_stock: number
  stock?: number
  is_active: boolean
}

export interface ProductListItem {
  id: number
  category_id: number | null
  platform_category_id: number
  name: string
  display_name: string
  description: string
  display_description: string
  main_image: string
  display_image: string
  slider_images: unknown
  spec_template: SpecTemplateItem[]
  status: 'on_shelf' | 'off_shelf'
  min_price: number
  max_price: number
  original_price: number
  display_price: number | null
}

export interface PlatformCategory {
  id: number
  name: string
  path: string
}

export interface ProductDetail {
  id: number
  category_id: number | null
  platform_category_id: number
  platform_category?: PlatformCategory | null
  source?: MallSource
  shop_name?: string
  shop_slug?: string
  name: string
  description: string
  detail_html: string
  main_image: string
  video_url: string
  material: string
  dimensions: string
  weight: string
  mainImage?: string
  image_url?: string
  slider_images: unknown
  images?: string[]
  gallery_images?: string[]
  spec_template: SpecTemplateItem[]
  specs?: SpecTemplateItem[]
  status: 'on_shelf' | 'off_shelf'
  min_price: number
  max_price: number
  original_price: number
  total_stock: number
  skus: SKU[]
}

export interface ProductQueryParams {
  category_id?: number
  platform_category_id?: number
  keyword?: string
  source?: MallSource
  is_self_operated?: boolean
  sort?: 'latest' | 'hot' | 'price_asc' | 'price_desc'
  page?: number
  page_size?: number
  limit?: number
}

export interface ProductPage {
  items: ProductListItem[]
  total: number
  page: number
  page_size: number
}

export type PopularProduct = ProductListItem

export interface PopularProductQueryParams {
  limit?: number
  category_id?: number
}

interface StorefrontProductListResponse {
  id?: unknown
  category_id?: unknown
  platform_category_id?: unknown
  name?: unknown
  title?: unknown
  description?: unknown
  main_image?: unknown
  image_url?: unknown
  image?: unknown
  images?: unknown
  slider_images?: unknown
  spec_template?: unknown
  status?: unknown
  min_price?: unknown
  max_price?: unknown
  retail_price?: unknown
  original_price?: unknown
}

interface StorefrontPopularProductResponse extends StorefrontProductListResponse {
  product_id?: unknown
}

interface StorefrontProductPageResponse {
  items?: StorefrontProductListResponse[]
  total?: unknown
  page?: unknown
  page_size?: unknown
}

export interface PublicShopProfile {
  tenant_id: number
  slug: string
  shop_name: string
  logo_url: string
  banner_url: string
  description: string
  phone: string
  whatsapp: string
  address: string
  business_hours: string
}

export interface PublicShopPage {
  items: PublicShopProfile[]
  total: number
  page: number
  page_size: number
}

export async function getCategories(): Promise<Category[]> {
  const response = await request.get<ApiResponse<Category[]>>('/storefront/categories')
  return response.data.data ?? []
}

export async function getProducts(params: ProductQueryParams = {}): Promise<ProductPage> {
  const response = await request.get<ApiResponse<StorefrontProductPageResponse>>('/storefront/products', { params })
  return normalizeProductPage(response.data.data, params)
}

export async function getPopularProducts(params: PopularProductQueryParams = {}): Promise<PopularProduct[]> {
  const response = await request.get<ApiResponse<StorefrontPopularProductResponse[]>>('/storefront/products/popular', { params })
  return (response.data.data ?? [])
    .map((product) => normalizeProductListItem({ ...product, id: product.product_id }))
    .filter((product) => product.id > 0)
}

export async function getProductDetail(id: number): Promise<ProductDetail> {
  const response = await request.get<ApiResponse<ProductDetail>>(`/storefront/products/${id}`)
  if (!response.data.data) {
    throw new Error('Product not found')
  }
  return response.data.data
}

export async function getPublicShopProfile(slug: string): Promise<PublicShopProfile> {
  const response = await request.get<ApiResponse<PublicShopProfile>>(`/storefront/shops/${slug}`)
  if (!response.data.data) {
    throw new Error('Shop not found')
  }
  return response.data.data
}

export async function getPublicShopProducts(slug: string, params: ProductQueryParams = {}): Promise<ProductPage> {
  const response = await request.get<ApiResponse<StorefrontProductPageResponse>>(`/storefront/shops/${slug}/products`, { params })
  return normalizeProductPage(response.data.data, params)
}

export async function getPublicShops(params: { page?: number; page_size?: number; limit?: number } = {}): Promise<PublicShopPage> {
  const response = await request.get<ApiResponse<PublicShopPage>>('/storefront/shops', { params })
  return response.data.data ?? { items: [], total: 0, page: params.page ?? 1, page_size: params.page_size ?? params.limit ?? 20 }
}

function normalizeProductPage(
  page: StorefrontProductPageResponse | null | undefined,
  params: ProductQueryParams,
): ProductPage {
  return {
    items: (page?.items ?? []).map(normalizeProductListItem).filter((item) => item.id > 0),
    total: positiveInteger(page?.total) ?? 0,
    page: positiveInteger(page?.page) ?? params.page ?? 1,
    page_size: positiveInteger(page?.page_size) ?? params.page_size ?? params.limit ?? 20,
  }
}

function normalizeProductListItem(product: StorefrontProductListResponse): ProductListItem {
  const displayName = nonEmptyString(product.name) ?? nonEmptyString(product.title) ?? 'Unnamed Product'
  const displayDescription = nonEmptyString(product.description) ?? ''
  const displayImage =
    nonEmptyString(product.main_image) ??
    nonEmptyString(product.image_url) ??
    nonEmptyString(product.image) ??
    firstImage(product.images) ??
    ''
  const displayPrice = nonNegativeNumber(product.min_price) ?? nonNegativeNumber(product.retail_price)
  const maxPrice = nonNegativeNumber(product.max_price) ?? displayPrice ?? 0

  return {
    id: positiveInteger(product.id) ?? 0,
    category_id: positiveInteger(product.category_id) ?? null,
    platform_category_id: positiveInteger(product.platform_category_id) ?? 0,
    name: displayName,
    display_name: displayName,
    description: displayDescription,
    display_description: displayDescription,
    main_image: displayImage,
    display_image: displayImage,
    slider_images: product.slider_images ?? product.images ?? [],
    spec_template: Array.isArray(product.spec_template) ? (product.spec_template as SpecTemplateItem[]) : [],
    status: product.status === 'off_shelf' ? 'off_shelf' : 'on_shelf',
    min_price: displayPrice ?? 0,
    max_price: maxPrice,
    original_price: nonNegativeNumber(product.original_price) ?? 0,
    display_price: displayPrice,
  }
}

function nonEmptyString(value: unknown): string | null {
  return typeof value === 'string' && value.trim() ? value.trim() : null
}

function nonNegativeNumber(value: unknown): number | null {
  const parsed = typeof value === 'number' ? value : typeof value === 'string' && value.trim() ? Number(value) : NaN
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : null
}

function positiveInteger(value: unknown): number | null {
  const parsed = nonNegativeNumber(value)
  return parsed !== null && Number.isInteger(parsed) && parsed > 0 ? parsed : null
}

function firstImage(value: unknown): string | null {
  if (!Array.isArray(value)) {
    return null
  }
  for (const item of value) {
    const image = nonEmptyString(item)
    if (image) {
      return image
    }
  }
  return null
}
