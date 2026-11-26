import assert from 'node:assert/strict'
import {
  homeProductLimit,
  legacyHomeDefaultTranslationKey,
  rootPlatformCategories,
} from '../src/utils/homeStorefront.ts'

assert.equal(homeProductLimit(24), 10)
assert.equal(homeProductLimit(6), 6)
assert.equal(homeProductLimit(0), 8)
assert.equal(legacyHomeDefaultTranslationKey('Latest Hot Products'), 'homeBlocks.popularProducts')
assert.equal(legacyHomeDefaultTranslationKey('Phone Parts'), 'homeBlocks.defaultPhoneParts')
assert.equal(legacyHomeDefaultTranslationKey('服务优势'), 'homeBlocks.serviceAdvantages')
assert.equal(legacyHomeDefaultTranslationKey('CMS custom title'), null)

assert.deepEqual(
  rootPlatformCategories([
    { id: 3, parent_id: 1, sort_order: 1 },
    { id: 2, parent_id: null, sort_order: 2 },
    { id: 1, parent_id: null, sort_order: 1 },
  ]),
  [
    { id: 1, parent_id: null, sort_order: 1 },
    { id: 2, parent_id: null, sort_order: 2 },
  ],
)

console.log('home storefront tests passed')
