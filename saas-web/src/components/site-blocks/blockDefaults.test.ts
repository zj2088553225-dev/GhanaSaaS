import { describe, expect, it } from 'vitest'
import {
  cloneBlockWithDefaults,
  createDefaultHomeBlocks,
} from './blockDefaults'

describe('homepage product floor source types', () => {
  it('marks only explicitly configured default floor as popular', () => {
    const floors = createDefaultHomeBlocks().filter((block) => block.type === 'product_floor')
    expect(floors[0]?.source_type).toBe('popular')
    expect(floors.slice(1).every((block) => block.source_type === 'latest')).toBe(true)
  })

  it('preserves legacy latest and category semantics', () => {
    expect(cloneBlockWithDefaults({ type: 'product_floor' }).source_type).toBe('latest')
    expect(cloneBlockWithDefaults({
      type: 'product_floor',
      platform_category_id: 7,
    }).source_type).toBe('category')
  })

  it('preserves the future manual enum without inventing selections', () => {
    expect(cloneBlockWithDefaults({
      type: 'product_floor',
      source_type: 'manual',
    }).source_type).toBe('manual')
  })
})
