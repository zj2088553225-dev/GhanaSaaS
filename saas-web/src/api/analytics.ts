import request, { type ApiResponse } from '../utils/request'
import { getStoredUser } from '../stores/auth'

export const ANALYTICS_TIMEZONE = 'Africa/Accra'

export type AnalyticsGranularity = 'hour' | 'day' | 'week'
export type AnalyticsSortOrder = 'asc' | 'desc'
export type AnalyticsSourceSort = 'impressions' | 'clicks' | 'ctr' | 'contact_intents'
export type ProductAnalyticsMetric =
  | 'impressions'
  | 'clicks'
  | 'views'
  | 'contact_intents'
  | 'unique_visitors'
  | 'ctr'
  | 'detail_view_rate'
  | 'contact_intent_rate'
export type ProductAnalyticsStatus = 'on_shelf' | 'off_shelf' | 'deleted'
export type ContentAnalyticsMetric = 'impressions' | 'clicks' | 'unique_visitors' | 'ctr'
export type ContentAnalyticsType =
  | 'hero'
  | 'hero_banner'
  | 'hero_slide'
  | 'promo_banner'
  | 'image'
  | 'image_text'
  | 'contact'
  | 'category_item'
  | 'feature_grid'
  | 'link'
  | 'other'

export interface AnalyticsDateRange {
  from: string
  to: string
  timezone: string
}

export interface AnalyticsMetricValue {
  value: number
  previous_value: number | null
  change_rate: number | null
}

export interface AnalyticsOverviewMetrics {
  visitors: AnalyticsMetricValue
  sessions: AnalyticsMetricValue
  page_views: AnalyticsMetricValue
  product_impressions: AnalyticsMetricValue
  product_clicks: AnalyticsMetricValue
  product_views: AnalyticsMetricValue
  contact_intents: AnalyticsMetricValue
  product_ctr: AnalyticsMetricValue
  detail_view_rate: AnalyticsMetricValue
  contact_intent_rate: AnalyticsMetricValue
}

export interface AnalyticsOverview {
  range: AnalyticsDateRange
  metrics: AnalyticsOverviewMetrics
}

export interface AnalyticsTrendPoint {
  bucket: string
  page_views: number
  product_impressions: number
  product_clicks: number
  product_views: number
  contact_intents: number
}

export interface AnalyticsTrends {
  granularity: AnalyticsGranularity
  timezone: string
  series: AnalyticsTrendPoint[]
}

export interface AnalyticsSourceStat {
  source_type: string
  impressions: number
  clicks: number
  product_views: number
  contact_intents: number
  ctr: number
}

export interface AnalyticsSources {
  range: AnalyticsDateRange
  sources: AnalyticsSourceStat[]
}

export interface ProductAnalyticsMetadata {
  product_id: number
  product_name: string
  product_slug: string
  tenant_id: number
  tenant_name: string
  status: 'on_shelf' | 'off_shelf' | ''
  main_image_url: string
  currently_exists: boolean
}

export interface ProductAnalyticsCounts {
  impressions: number
  clicks: number
  views: number
  contact_intents: number
  unique_visitors: number
}

export interface ProductAnalyticsRates {
  ctr: number
  detail_view_rate: number
  contact_intent_rate: number
}

export interface ProductAnalyticsItem extends ProductAnalyticsMetadata, ProductAnalyticsCounts, ProductAnalyticsRates {}
export type ProductAnalyticsSummary = ProductAnalyticsCounts & ProductAnalyticsRates

export interface ProductAnalyticsList {
  range: AnalyticsDateRange
  items: ProductAnalyticsItem[]
  total: number
  page: number
  page_size: number
}

export interface ProductAnalyticsRanking {
  range: AnalyticsDateRange
  metric?: ProductAnalyticsMetric
  min_impressions?: number
  items: ProductAnalyticsItem[]
}

export interface ProductPopularityItem {
  product_id: number
  tenant_id?: number
  category_id: number | null
  platform_category_id: number
  name: string
  main_image: string
  min_price: number
  max_price: number
  original_price: number
  hot_score: number
  views: number
  clicks: number
  contacts: number
  interest_count: number
  verified_count: number
}

export interface ProductPopularityQuery {
  tenant_id?: number
  category_id?: number
  limit?: number
  locale?: string
}

export interface ProductAnalyticsTrendPoint extends ProductAnalyticsCounts {
  bucket: string
}

export interface ProductAnalyticsSourceStat extends ProductAnalyticsCounts {
  source_type: string
  source_id: string | null
  source_label: string | null
  ctr: number
}

export interface ProductAnalyticsDetail {
  range: AnalyticsDateRange
  granularity: AnalyticsGranularity
  product: ProductAnalyticsMetadata
  summary: ProductAnalyticsSummary
  trend: ProductAnalyticsTrendPoint[]
  sources: ProductAnalyticsSourceStat[]
}

export interface MarketingContentMetadata {
  tracking_key: string
  content_type: ContentAnalyticsType
  title: string
  image_url: string
  target_type: 'product' | 'shop' | 'category' | 'internal' | 'external' | 'whatsapp' | 'phone' | 'none'
  target_value: string
  published: boolean
  currently_exists: boolean
}

export interface ContentAnalyticsCounts {
  impressions: number
  clicks: number
  unique_visitors: number
}

export interface ContentAnalyticsItem extends MarketingContentMetadata, ContentAnalyticsCounts {
  ctr: number
}

export interface ContentAnalyticsList {
  range: AnalyticsDateRange
  items: ContentAnalyticsItem[]
  total: number
  page: number
  page_size: number
}

export interface ContentAnalyticsTrendPoint extends ContentAnalyticsCounts {
  bucket: string
}

export interface ContentAnalyticsLocaleStat extends ContentAnalyticsCounts {
  locale: 'en' | 'zh' | 'other'
  ctr: number
}

export interface ContentAnalyticsDeviceStat extends ContentAnalyticsCounts {
  device_type: 'desktop' | 'mobile' | 'tablet' | 'unknown'
  ctr: number
}

export interface ContentAnalyticsPageStat {
  page_path: string
  impressions: number
  clicks: number
}

export interface ContentAnalyticsDetail {
  range: AnalyticsDateRange
  granularity: AnalyticsGranularity
  content: MarketingContentMetadata
  summary: ContentAnalyticsCounts & { ctr: number }
  trend: ContentAnalyticsTrendPoint[]
  locales: ContentAnalyticsLocaleStat[]
  devices: ContentAnalyticsDeviceStat[]
  pages: ContentAnalyticsPageStat[]
}

export interface AnalyticsDataStatus {
  total_events: number
  human_events: number
  bot_events: number
  oldest_event_at: string | null
  latest_event_at: string | null
  events_last_24h: number
  events_by_name: Record<string, number>
}

export interface AnalyticsCSVDownload {
  blob: Blob
  filename: string
}

export interface AnalyticsDateQuery {
  from: string
  to: string
  timezone?: string
  tenant_id?: number
}

export interface AnalyticsRequestOptions {
  signal?: AbortSignal
}

export interface ProductAnalyticsListQuery extends AnalyticsDateQuery {
  page?: number
  page_size?: number
  search?: string
  status?: ProductAnalyticsStatus
  sort?: ProductAnalyticsMetric
  order?: AnalyticsSortOrder
}

export interface ProductAnalyticsRankingQuery extends AnalyticsDateQuery {
  metric?: ProductAnalyticsMetric
  limit?: number
  min_impressions?: number
  min_clicks?: number
  min_views?: number
}

export interface ContentAnalyticsListQuery extends Omit<AnalyticsDateQuery, 'tenant_id'> {
  page?: number
  page_size?: number
  search?: string
  content_type?: ContentAnalyticsType
  currently_exists?: boolean
  sort?: ContentAnalyticsMetric
  order?: AnalyticsSortOrder
  min_impressions?: number
}

export type ProductAnalyticsExportQuery = Omit<ProductAnalyticsListQuery, 'page' | 'page_size'>
export type ContentAnalyticsExportQuery = Omit<ContentAnalyticsListQuery, 'page' | 'page_size'>

type AnalyticsQueryValue = string | number | boolean | undefined
type AnalyticsQuery = Record<string, AnalyticsQueryValue>

export function usesSuperAnalyticsEndpoint() {
  return getStoredUser()?.role === 'super_admin'
}

export function analyticsEndpoint(path: string) {
  const base = usesSuperAnalyticsEndpoint() ? '/super/analytics' : '/admin/analytics'
  return `${base}${path}`
}

function cleanAnalyticsParams(params: AnalyticsQuery, allowTenant = true) {
  const cleaned: Record<string, string | number | boolean> = {}
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === '') {
      continue
    }
    if (key === 'tenant_id' && (!allowTenant || !usesSuperAnalyticsEndpoint())) {
      continue
    }
    cleaned[key] = value
  }
  return cleaned
}

async function analyticsGet<T>(path: string, params: AnalyticsQuery, options: AnalyticsRequestOptions = {}, superOnly = false): Promise<T> {
  if (superOnly && !usesSuperAnalyticsEndpoint()) {
    throw new Error('当前账号无权查看内容分析')
  }
  const response = await request.get<ApiResponse<T>>(analyticsEndpoint(path), {
    params: cleanAnalyticsParams(params, !superOnly),
    signal: options.signal,
  })
  if (!response.data.data) {
    throw new Error('Analytics response is empty')
  }
  return response.data.data
}

function dateParams(query: AnalyticsDateQuery): AnalyticsQuery {
  return {
    from: query.from,
    to: query.to,
    timezone: query.timezone || ANALYTICS_TIMEZONE,
    tenant_id: query.tenant_id,
  }
}

export function getAnalyticsOverview(query: AnalyticsDateQuery, options?: AnalyticsRequestOptions) {
  return analyticsGet<AnalyticsOverview>('/overview', { ...dateParams(query), compare: true }, options)
}

export function getAnalyticsStatus(options?: AnalyticsRequestOptions) {
  return analyticsGet<AnalyticsDataStatus>('/status', {}, options, true)
}

export function getAnalyticsTrends(query: AnalyticsDateQuery & { granularity?: AnalyticsGranularity }, options?: AnalyticsRequestOptions) {
  return analyticsGet<AnalyticsTrends>('/trends', { ...dateParams(query), granularity: query.granularity }, options)
}

export function getAnalyticsSources(
  query: AnalyticsDateQuery & { sort?: AnalyticsSourceSort; limit?: number },
  options?: AnalyticsRequestOptions,
) {
  return analyticsGet<AnalyticsSources>('/sources', { ...dateParams(query), sort: query.sort, limit: query.limit }, options)
}

export function getProductAnalytics(query: ProductAnalyticsListQuery, options?: AnalyticsRequestOptions) {
  return analyticsGet<ProductAnalyticsList>('/products', { ...query, ...dateParams(query) }, options)
}

export function getTopProducts(query: ProductAnalyticsRankingQuery, options?: AnalyticsRequestOptions) {
  return analyticsGet<ProductAnalyticsRanking>('/products/top', { ...query, ...dateParams(query) }, options)
}

export function getLowCTRProducts(
  query: AnalyticsDateQuery & { limit?: number; min_impressions?: number },
  options?: AnalyticsRequestOptions,
) {
  return analyticsGet<ProductAnalyticsRanking>('/products/low-ctr', { ...query, ...dateParams(query) }, options)
}

export function getProductPopularity(query: ProductPopularityQuery = {}, options?: AnalyticsRequestOptions) {
  return analyticsGet<ProductPopularityItem[]>('/popularity', { ...query }, options)
}

export function getProductAnalyticsDetail(
  productID: number,
  query: AnalyticsDateQuery & { granularity?: AnalyticsGranularity },
  options?: AnalyticsRequestOptions,
) {
  return analyticsGet<ProductAnalyticsDetail>(`/products/${productID}`, { ...dateParams(query), granularity: query.granularity }, options)
}

export function getContentAnalytics(query: ContentAnalyticsListQuery, options?: AnalyticsRequestOptions) {
  return analyticsGet<ContentAnalyticsList>('/content', { ...query, timezone: query.timezone || ANALYTICS_TIMEZONE }, options, true)
}

export function getContentAnalyticsDetail(
  trackingKey: string,
  query: Omit<AnalyticsDateQuery, 'tenant_id'> & { granularity?: AnalyticsGranularity },
  options?: AnalyticsRequestOptions,
) {
  return analyticsGet<ContentAnalyticsDetail>(
    `/content/${encodeURIComponent(trackingKey)}`,
    { ...query, timezone: query.timezone || ANALYTICS_TIMEZONE },
    options,
    true,
  )
}

async function analyticsCSVExport(
  path: string,
  params: AnalyticsQuery,
  fallbackFilename: string,
  superOnly = false,
): Promise<AnalyticsCSVDownload> {
  if (superOnly && !usesSuperAnalyticsEndpoint()) {
    throw new Error('当前账号无权导出内容分析')
  }
  const response = await request.get<Blob>(analyticsEndpoint(path), {
    params: cleanAnalyticsParams(params, !superOnly),
    responseType: 'blob',
  })
  const disposition = String(response.headers['content-disposition'] || '')
  const encoded = disposition.match(/filename\*=UTF-8''([^;]+)/i)?.[1]
  const plain = disposition.match(/filename="?([^";]+)"?/i)?.[1]
  let filename = fallbackFilename
  try {
    filename = decodeURIComponent(encoded || plain || fallbackFilename)
  } catch {
    filename = fallbackFilename
  }
  filename = filename.split(/[\\/]/).pop()?.replace(/[^A-Za-z0-9._-]/g, '-') || fallbackFilename
  return { blob: response.data, filename }
}

export function exportProductAnalytics(query: ProductAnalyticsExportQuery) {
  return analyticsCSVExport(
    '/products/export',
    { ...query, ...dateParams(query) },
    `product-analytics-${query.from.replaceAll('-', '')}-${query.to.replaceAll('-', '')}.csv`,
  )
}

export function exportContentAnalytics(query: ContentAnalyticsExportQuery) {
  return analyticsCSVExport(
    '/content/export',
    { ...query, timezone: query.timezone || ANALYTICS_TIMEZONE },
    `content-analytics-${query.from.replaceAll('-', '')}-${query.to.replaceAll('-', '')}.csv`,
    true,
  )
}
