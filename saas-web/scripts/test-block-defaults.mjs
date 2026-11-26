import assert from 'node:assert/strict'
import { cloneBlockWithDefaults, prepareBlockForSave } from '../src/components/site-blocks/blockDefaults.ts'

const carouselMissingSlides = cloneBlockWithDefaults({ type: 'hero_carousel' })
assert.equal(carouselMissingSlides.slides.length, 1)
assert.equal(carouselMissingSlides.slides[0].link, '/mall')
assert.equal(carouselMissingSlides.fit, 'cover')

const carouselEmptySlides = cloneBlockWithDefaults({ type: 'hero_carousel', slides: [] })
assert.deepEqual(carouselEmptySlides.slides, [])

const carouselCover = prepareBlockForSave({ type: 'hero_carousel', fit: 'cover', slides: [] })
assert.equal(carouselCover.fit, 'cover')

const carouselInvalidFit = prepareBlockForSave({ type: 'hero_carousel', fit: 'invalid', slides: [] })
assert.equal(carouselInvalidFit.fit, 'cover')

const footerMissingItems = cloneBlockWithDefaults({ type: 'footer_links', title: 'Footer' })
assert.equal(footerMissingItems.items.length, 2)
assert.equal(footerMissingItems.items[0].links.length, 3)

const featureMissingItems = cloneBlockWithDefaults({ type: 'feature_grid', title: 'Features' })
assert.equal(featureMissingItems.items.length, 3)

const featureEmptyItems = cloneBlockWithDefaults({ type: 'feature_grid', items: [] })
assert.deepEqual(featureEmptyItems.items, [])

const defaultFloor = cloneBlockWithDefaults({ type: 'product_floor' })
assert.equal(defaultFloor.source, 'all')

const t6Floor = prepareBlockForSave({ type: 'product_floor', source: 't6', sort: 'hot' })
assert.equal(t6Floor.source, 't6')
assert.equal(t6Floor.sort, 'hot')

const legacyFeaturedFloor = prepareBlockForSave({ type: 'product_floor', source: 'featured' })
assert.equal(legacyFeaturedFloor.source, 't6')

console.log('block defaults tests passed')
