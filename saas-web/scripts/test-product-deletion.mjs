import assert from 'node:assert/strict'
import {
  PRODUCT_DELETE_FAILED_MESSAGE,
  PRODUCT_HAS_ORDER_RECORDS_MESSAGE,
  productDeletionErrorMessage,
} from '../src/utils/productDeletion.ts'

assert.equal(
  productDeletionErrorMessage(new Error(PRODUCT_HAS_ORDER_RECORDS_MESSAGE)),
  PRODUCT_HAS_ORDER_RECORDS_MESSAGE,
)
assert.equal(
  productDeletionErrorMessage(new Error('ERROR: SQLSTATE 23503 fk_other_constraint')),
  PRODUCT_DELETE_FAILED_MESSAGE,
)
assert.equal(productDeletionErrorMessage(new Error('网络错误')), '网络错误')
assert.equal(productDeletionErrorMessage(null), PRODUCT_DELETE_FAILED_MESSAGE)

console.log('product deletion tests passed')
