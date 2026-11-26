import type { ContentAnalyticsType } from '../api/analytics'

const countFormatter = new Intl.NumberFormat('en-US', { maximumFractionDigits: 0 })

function finiteNumber(value: number | null | undefined) {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}

export function formatAnalyticsCount(value: number | null | undefined) {
  return countFormatter.format(finiteNumber(value))
}

export function formatAnalyticsRate(value: number | null | undefined) {
  const rate = finiteNumber(value)
  if (rate === 0) {
    return '0%'
  }
  return `${(rate * 100).toFixed(2)}%`
}

export function formatAnalyticsChange(value: number | null | undefined) {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return 'New'
  }
  if (value === 0) {
    return '0%'
  }
  const prefix = value > 0 ? '+' : ''
  return `${prefix}${(value * 100).toFixed(2)}%`
}

const sourceLabels: Record<string, string> = {
  home_product_floor: '首页商品楼层',
  mall: '商城',
  category: '类目',
  search: '搜索',
  shop: '店铺',
  product_detail: '商品详情',
  marketing_content: '营销内容',
  direct: '直接访问',
  other: '其他',
}

export function analyticsSourceLabel(sourceType: string) {
  return sourceLabels[sourceType] || sourceLabels.other
}

const contentTypeLabels: Record<ContentAnalyticsType, string> = {
  hero: 'Hero',
  hero_banner: 'Hero Banner',
  hero_slide: '轮播图',
  promo_banner: '推广 Banner',
  image: '图片',
  image_text: '图文',
  contact: '联系内容',
  category_item: '类目入口',
  feature_grid: '功能宫格',
  link: '链接',
  other: '其他',
}

export function analyticsContentTypeLabel(contentType: ContentAnalyticsType | string) {
  return contentTypeLabels[contentType as ContentAnalyticsType] || contentTypeLabels.other
}

const contentTargetLabels: Record<string, string> = {
  product: '商品',
  shop: '店铺',
  category: '类目',
  internal: '站内页面',
  external: '外部链接',
  whatsapp: 'WhatsApp',
  phone: '电话',
  none: '无跳转',
}

export function analyticsContentTargetLabel(targetType: string, targetValue: string) {
  const label = contentTargetLabels[targetType] || contentTargetLabels.none
  return targetValue ? `${label}：${targetValue}` : label
}
