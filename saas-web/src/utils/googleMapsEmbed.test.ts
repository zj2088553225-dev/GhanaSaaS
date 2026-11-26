import { describe, expect, it } from 'vitest'
import type { SitePageBlock } from '../api/sitePages'
import { createDefaultHomeBlocks } from '../components/site-blocks/blockDefaults'
import {
  buildGoogleMapsEmbedURL,
  DEFAULT_MAP_HEIGHT,
  googleMapsPlaceQuery,
  localizedMapText,
  MAX_MAP_HEIGHT,
  MIN_MAP_HEIGHT,
  normalizeMapHeight,
} from './googleMapsEmbed'

function mapBlock(overrides: Partial<SitePageBlock> = {}): SitePageBlock {
  return {
    type: 'map_section',
    place_query: 'Accra, Ghana',
    ...overrides,
  }
}

describe('Google Maps Embed homepage block', () => {
  it('builds a place URL without exposing a URL when the key is missing', () => {
    expect(buildGoogleMapsEmbedURL(mapBlock(), '')).toBe('')

    const result = new URL(buildGoogleMapsEmbedURL(mapBlock(), 'public embed key'))
    expect(`${result.origin}${result.pathname}`).toBe('https://www.google.com/maps/embed/v1/place')
    expect(result.searchParams.get('key')).toBe('public embed key')
    expect(result.searchParams.get('q')).toBe('Accra, Ghana')
  })

  it('prefers place_id over the configured place query', () => {
    const block = mapBlock({ place_id: 'ChIJ-Test-ID', place_query: 'Ignored address' })
    expect(googleMapsPlaceQuery(block)).toBe('place_id:ChIJ-Test-ID')

    const result = new URL(buildGoogleMapsEmbedURL(block, 'test-key'))
    expect(result.searchParams.get('q')).toBe('place_id:ChIJ-Test-ID')
  })

  it('uses locale-specific titles with a bilingual fallback', () => {
    const block = mapBlock({
      title_en: 'Visit Us',
      title_zh: '到访我们',
      subtitle_en: 'Our Accra location',
    })
    expect(localizedMapText(block, 'en', 'title')).toBe('Visit Us')
    expect(localizedMapText(block, 'zh', 'title')).toBe('到访我们')
    expect(localizedMapText(block, 'zh', 'subtitle')).toBe('Our Accra location')
  })

  it('normalizes the configurable desktop height', () => {
    expect(normalizeMapHeight(undefined)).toBe(DEFAULT_MAP_HEIGHT)
    expect(normalizeMapHeight(421.6)).toBe(422)
    expect(normalizeMapHeight(1)).toBe(MIN_MAP_HEIGHT)
    expect(normalizeMapHeight(9999)).toBe(MAX_MAP_HEIGHT)
  })

  it('places the default map after the top category area and before the first product floor', () => {
    const blocks = createDefaultHomeBlocks()
    const mapIndex = blocks.findIndex((block) => block.type === 'map_section')
    const categoryIndex = blocks.findIndex((block) => block.type === 'category_nav')
    const firstProductFloorIndex = blocks.findIndex((block) => block.type === 'product_floor')

    expect(mapIndex).toBe(categoryIndex + 1)
    expect(mapIndex).toBeLessThan(firstProductFloorIndex)
    expect(blocks[mapIndex]?.sort_order).toBe(mapIndex)
    expect(blocks[mapIndex]?.map_height).toBe(DEFAULT_MAP_HEIGHT)
  })
})
