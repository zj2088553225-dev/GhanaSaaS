import assert from 'node:assert/strict'
import {
  buildProductSeoPath,
  buildProductSlug,
  parseProductIdFromSeoParam,
} from '../src/utils/productSeoUrl.ts'

assert.equal(buildProductSlug('Cruiser Motorcycle'), 'cruiser-motorcycle')
assert.equal(buildProductSlug('42MM Two-Color PVC Pipe Cutter'), '42mm-two-color-pvc-pipe-cutter')
assert.equal(buildProductSlug('Apple iPhone 12'), 'apple-iphone-12')
assert.equal(buildProductSlug('Phones & Accessories'), 'phones-and-accessories')
assert.equal(buildProductSlug('中文商品'), 'product')
const longSlug = buildProductSlug('A very long product name that should be clipped at the recommended maximum length for SEO URLs')
assert.equal(longSlug.length <= 80, true)
assert.equal(longSlug.startsWith('a-very-long-product-name-that-should-be-clipped'), true)

assert.equal(
  buildProductSeoPath({ id: 15, name: 'Cruiser Motorcycle' }),
  '/products/cruiser-motorcycle-15',
)
assert.equal(
  buildProductSeoPath({ id: 14, name: 'Apple iPhone 12' }),
  '/products/apple-iphone-12-14',
)
assert.equal(buildProductSeoPath({ id: 123, name: '中文商品' }), '/products/product-123')
assert.equal(buildProductSeoPath({ id: '11', title: 'High-Speed Countertop Blender' }), '/products/high-speed-countertop-blender-11')
assert.equal(buildProductSeoPath({ id: 14, name: 'Apple iPhone 12' }, 'en'), '/en/products/apple-iphone-12-14')
assert.equal(buildProductSeoPath({ id: 14, name: 'Apple iPhone 12' }, 'zh'), '/zh/products/apple-iphone-12-14')

assert.equal(parseProductIdFromSeoParam('cruiser-motorcycle-15'), 15)
assert.equal(parseProductIdFromSeoParam('apple-iphone-12-14'), 14)
assert.equal(parseProductIdFromSeoParam('15'), 15)
assert.equal(parseProductIdFromSeoParam('missing-id'), 0)

console.log('product SEO URL tests passed')
