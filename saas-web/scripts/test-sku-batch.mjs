import assert from 'node:assert/strict'
import { applySkuBatchToRows } from '../src/utils/skuBatch.ts'

const skus = [
  {
    name: 'S / Blue',
    erp_sku_code: 'SKU-1001-001',
    retail_price: null,
    original_price: null,
    sellable_stock: null,
  },
  {
    name: 'M / Green',
    erp_sku_code: 'SKU-1001-002',
    retail_price: 9.9,
    original_price: 12,
    sellable_stock: 3,
  },
]

const updated = applySkuBatchToRows(skus, {
  retail_price: 19.9,
  original_price: 29.9,
  sellable_stock: 8,
})

assert.deepEqual(
  updated.map((sku) => [sku.retail_price, sku.original_price, sku.sellable_stock]),
  [
    [19.9, 29.9, 8],
    [19.9, 29.9, 8],
  ],
)
assert.deepEqual(
  updated.map((sku) => sku.erp_sku_code),
  ['SKU-1001-001', 'SKU-1001-002'],
)

const partial = applySkuBatchToRows(updated, {
  retail_price: null,
  original_price: 39.9,
  sellable_stock: null,
})

assert.equal(partial[0].retail_price, 19.9)
assert.equal(partial[0].original_price, 39.9)
assert.equal(partial[0].sellable_stock, 8)

partial[0].retail_price = 18.8
assert.equal(partial[0].retail_price, 18.8)
assert.equal(partial[1].retail_price, 19.9)
