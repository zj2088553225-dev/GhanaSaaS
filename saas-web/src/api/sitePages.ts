import request, { type ApiResponse } from '../utils/request'

export type SitePageBlockType =
  | 'hero'
  | 'hero_banner'
  | 'hero_carousel'
  | 'category_nav'
  | 'map_section'
  | 'image_text'
  | 'feature_grid'
  | 'product_floor'
  | 'product_grid'
  | 'shop_grid'
  | 'promo_banner'
  | 'footer_links'
  | 'contact'
  | 'text'
  | 'image'

export interface SitePageBlockItem {
  tracking_key?: string
  name?: string
  title?: string
  description?: string
  image?: string
  link?: string
  icon?: string
  links?: SitePageBlockItem[]
}

export interface SitePageBlockSlide {
  tracking_key?: string
  image?: string
  title?: string
  subtitle?: string
  buttonText?: string
  link?: string
  enabled?: boolean
}

export interface SitePageBlock {
  type: SitePageBlockType
  tracking_key?: string
  title?: string
  subtitle?: string
  content?: string
  image?: string
  image_url?: string
  alt?: string
  backgroundImage?: string
  link_url?: string
  button_text?: string
  buttonText?: string
  buttonLink?: string
  layout?: 'image_left' | 'image_right'
  source?: 'all' | 't6' | 'merchant' | 'latest' | 'featured' | 'hot' | 'category'
  source_type?: 'popular' | 'latest' | 'category' | 'manual'
  sort?: 'latest' | 'hot'
  limit?: number
  columns?: number
  enabled?: boolean
  show_more_link?: boolean
  category_id?: number
  platform_category_id?: number
  platform_category_path?: string
  items?: SitePageBlockItem[]
  slides?: SitePageBlockSlide[]
  autoplay?: boolean
  interval?: number
  height?: 'large' | 'medium' | 'small'
  fit?: 'contain' | 'cover'
  phone?: string
  whatsapp?: string
  address?: string
  title_zh?: string
  title_en?: string
  subtitle_zh?: string
  subtitle_en?: string
  place_query?: string
  place_id?: string
  map_height?: number
  external_map_url?: string
  sort_order?: number
}

export interface SitePageContent {
  blocks: SitePageBlock[]
}

export interface SitePageLocaleTranslation {
  draft_content?: SitePageContent
  published_content?: SitePageContent
}

export interface SitePageTranslations {
  zh?: SitePageLocaleTranslation
}

export interface SitePageAdmin {
  page_key: string
  title: string
  draft_content: SitePageContent
  published_content?: SitePageContent
  translations?: SitePageTranslations
  status: 'draft' | 'published'
  published_at?: string
}

export interface PublicSitePage {
  page_key: string
  title: string
  content: SitePageContent
  published_at?: string
}

export async function getHomeSitePage(): Promise<SitePageAdmin> {
  const response = await request.get<ApiResponse<SitePageAdmin>>('/admin/site-pages/home')
  if (!response.data.data) {
    throw new Error('Page not found')
  }
  return response.data.data
}

export async function saveHomeSitePageDraft(title: string, content: SitePageContent, translations?: SitePageTranslations): Promise<SitePageAdmin> {
  const response = await request.put<ApiResponse<SitePageAdmin>>('/admin/site-pages/home/draft', { title, content, translations })
  if (!response.data.data) {
    throw new Error('Save failed')
  }
  return response.data.data
}

export async function publishHomeSitePage(): Promise<SitePageAdmin> {
  const response = await request.post<ApiResponse<SitePageAdmin>>('/admin/site-pages/home/publish')
  if (!response.data.data) {
    throw new Error('Publish failed')
  }
  return response.data.data
}

export async function getPublishedHomeSitePage(): Promise<PublicSitePage> {
  const response = await request.get<ApiResponse<PublicSitePage>>('/storefront/pages/home')
  if (!response.data.data) {
    throw new Error('Page not found')
  }
  return response.data.data
}
