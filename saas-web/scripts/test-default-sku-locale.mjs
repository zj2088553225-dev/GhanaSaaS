import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import {
  CANONICAL_DEFAULT_SKU_NAME,
  isDefaultSkuName,
  localizedSkuDisplayName,
} from '../src/utils/productSku.ts'
import en from '../src/locales/en.ts'
import zh from '../src/locales/zh.ts'

for (const name of ['默认规格', '默认SKU', 'Default SKU', 'Default Specification']) {
  assert.equal(isDefaultSkuName(name), true, `${name} should be a system default SKU marker`)
}

assert.equal(en.product.defaultSku, 'Default SKU')
assert.equal(zh.product.defaultSku, '默认规格')
assert.equal(localizedSkuDisplayName({ name: '默认规格', specs: {} }, en.product.defaultSku), 'Default SKU')
assert.equal(localizedSkuDisplayName({ name: '默认规格', specs: {} }, zh.product.defaultSku), '默认规格')
assert.equal(localizedSkuDisplayName({ name: 'Default SKU', specs: {} }, en.product.defaultSku), 'Default SKU')
assert.equal(localizedSkuDisplayName({ name: 'Default SKU', specs: {} }, zh.product.defaultSku), '默认规格')
assert.equal(localizedSkuDisplayName({ name: 'Wholesale Carton', specs: {} }, '默认规格'), 'Wholesale Carton')
assert.equal(
  localizedSkuDisplayName({ name: '默认规格', specs: { Size: 'Large', Color: 'Blue' } }, 'Default SKU'),
  'Size Large / Color Blue',
)

const productForm = readFileSync(new URL('../src/views/admin/products/ProductForm.vue', import.meta.url), 'utf8')
assert.equal(CANONICAL_DEFAULT_SKU_NAME, 'Default SKU')
assert.match(productForm, /skuNameForSave/)
assert.doesNotMatch(productForm, /name:\s*sku\.name\s*\|\|\s*'默认规格'/)

console.log('default SKU locale tests passed')
