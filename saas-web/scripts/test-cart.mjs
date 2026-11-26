import assert from 'node:assert/strict'
import { mergeCartItems, normalizeCartLine } from '../src/utils/cartItems.ts'

let lines = mergeCartItems([], [{ product_id: 1, sku_id: 101, quantity: 2 }])
assert.deepEqual(lines, [{ product_id: 1, sku_id: 101, quantity: 2 }])

// A multi-SKU batch keeps each SKU as an independent cart line.
lines = mergeCartItems(lines, [
  { product_id: 1, sku_id: 102, quantity: 3 },
  { product_id: 1, sku_id: 103, quantity: 1 },
])
assert.equal(lines.length, 3)
assert.equal(lines.find((line) => line.sku_id === 102)?.quantity, 3)
assert.equal(lines.find((line) => line.sku_id === 103)?.quantity, 1)

lines = mergeCartItems(lines, [{ product_id: 1, sku_id: 101, quantity: 4 }])
assert.equal(lines.find((line) => line.sku_id === 101)?.quantity, 6)
assert.equal(lines.reduce((sum, line) => sum + line.quantity, 0), 10)

lines = mergeCartItems(lines, [
  { product_id: 1, sku_id: 101, quantity: 0 },
  { product_id: 1, sku_id: 104, quantity: -1 },
  { product_id: 1, sku_id: 105, quantity: 1.5 },
  { product_id: 1, sku_id: 106 },
])
assert.equal(lines.length, 3)
assert.equal(lines.find((line) => line.sku_id === 101)?.quantity, 6)

assert.deepEqual(normalizeCartLine({ product_id: 2, sku_id: 201, quantity: 1 }), {
  product_id: 2,
  sku_id: 201,
  quantity: 1,
})
assert.equal(normalizeCartLine({ product_id: 2, sku_id: 201, quantity: 0 }), null)
assert.equal(normalizeCartLine({ product_id: 2, sku_id: 201, quantity: '1' }), null)

const rawPersistedShape = JSON.stringify({
  lines: lines.map((line) => ({
    sku_id: line.sku_id,
    product_id: line.product_id,
    quantity: line.quantity,
  })),
})
const parsed = JSON.parse(rawPersistedShape)
assert.equal(parsed.lines.length, 3)
assert.ok(parsed.lines.every((line) => line.sku_id && line.product_id && line.quantity))

console.log('cart tests passed')
