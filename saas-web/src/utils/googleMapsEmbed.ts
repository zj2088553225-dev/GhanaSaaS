import type { SitePageBlock } from '../api/sitePages'
import type { SupportedLocale } from './localePath'

export const DEFAULT_MAP_HEIGHT = 320
export const MIN_MAP_HEIGHT = 200
export const MAX_MAP_HEIGHT = 800

export function googleMapsEmbedKey() {
  return String(import.meta.env.VITE_GOOGLE_MAPS_EMBED_KEY || '').trim()
}

export function normalizeMapHeight(value?: number) {
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) {
    return DEFAULT_MAP_HEIGHT
  }
  return Math.min(Math.max(Math.round(parsed), MIN_MAP_HEIGHT), MAX_MAP_HEIGHT)
}

export function googleMapsPlaceQuery(block: SitePageBlock) {
  const placeID = block.place_id?.trim()
  if (placeID) {
    return `place_id:${placeID}`
  }
  return block.place_query?.trim() || ''
}

export function buildGoogleMapsEmbedURL(block: SitePageBlock, apiKey = googleMapsEmbedKey()) {
  const key = apiKey.trim()
  const query = googleMapsPlaceQuery(block)
  if (!key || !query) {
    return ''
  }

  const params = new URLSearchParams({ key, q: query })
  return `https://www.google.com/maps/embed/v1/place?${params.toString()}`
}

export function localizedMapText(
  block: SitePageBlock,
  locale: SupportedLocale,
  field: 'title' | 'subtitle',
) {
  const primary = locale === 'zh' ? block[`${field}_zh`] : block[`${field}_en`]
  const fallback = locale === 'zh' ? block[`${field}_en`] : block[`${field}_zh`]
  return primary?.trim() || fallback?.trim() || ''
}
