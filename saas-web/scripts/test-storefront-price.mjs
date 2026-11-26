import assert from 'node:assert/strict'
import { parseStorefrontFeatureFlag, storefrontPriceVisible } from '../src/config/storefront.ts'

assert.equal(parseStorefrontFeatureFlag(undefined), false)
assert.equal(parseStorefrontFeatureFlag('false'), false)
assert.equal(parseStorefrontFeatureFlag('TRUE'), true)
assert.equal(parseStorefrontFeatureFlag(' true '), true)
assert.equal(storefrontPriceVisible, false)

console.log('storefront price visibility tests passed')
