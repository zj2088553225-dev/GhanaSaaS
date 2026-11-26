import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const productList = readFileSync(new URL('../src/views/admin/products/ProductList.vue', import.meta.url), 'utf8')
const header = readFileSync(new URL('../src/components/storefront/StorefrontHeader.vue', import.meta.url), 'utf8')
const zh = readFileSync(new URL('../src/locales/zh.ts', import.meta.url), 'utf8')
const en = readFileSync(new URL('../src/locales/en.ts', import.meta.url), 'utf8')

assert.match(productList, /imageVariantURL/)
assert.match(productList, /productThumbnail/)
assert.match(productList, /firstGalleryImage/)
assert.match(productList, /product\.main_image\?\.trim\(\)\s*\|\|/)
assert.match(productList, /slider_images/)
assert.match(productList, /'thumb'/)
assert.match(productList, /label="商品图"/)
assert.match(productList, /h-12 w-12/)
assert.match(productList, /object-cover/)
assert.match(productList, /暂无图片/)

assert.match(header, /<RouterLink\s+:to="localizedPath\('\/merchant\/join'\)"/)
assert.doesNotMatch(
  header,
  /<RouterLink\s+to="\/merchant\/join"/,
  'merchant join link must not use an unlocalized static path',
)
assert.match(header, /t\('nav\.openShop'\)/)
assert.match(header, /isPathActive\('\/merchant\/join'\)/)
assert.match(zh, /openShop:\s*'商家入驻'/)
assert.match(en, /openShop:\s*'Open a Shop'/)

console.log('product list thumbnail tests passed')
