import type { SitePageBlock } from '../../api/sitePages'

export type HomeFloorDiagnostic = 'checking' | 'ok' | 'missing' | 'empty' | 'unavailable'

export function homeFloorDiagnosticKey(block: SitePageBlock): string | null {
  const categoryID = Number(block.platform_category_id)
  const sourceType = block.source_type || (categoryID > 0 ? 'category' : 'latest')
  if (block.type !== 'product_floor' || sourceType !== 'category' ||
    !Number.isInteger(categoryID) || categoryID <= 0) {
    return null
  }
  return `${categoryID}:${block.source || 'all'}`
}
