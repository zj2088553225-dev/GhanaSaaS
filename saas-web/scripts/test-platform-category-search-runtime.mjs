import assert from 'node:assert/strict'
import {
  buildPlatformCategorySearchParams,
  isCurrentPlatformCategorySearch,
  shouldSchedulePlatformCategorySearch,
} from '../src/utils/platformCategorySearch.ts'

const params = buildPlatformCategorySearchParams('手机 Mobile', {
  limit: 50,
  locale: 'all',
  leafOnly: true,
})
assert.deepEqual(params, {
  keyword: '手机 Mobile',
  limit: 50,
  locale: 'all',
  leaf_only: true,
})

const encoded = new URLSearchParams(Object.entries(params).map(([key, value]) => [key, String(value)])).toString()
assert.match(encoded, /keyword=%E6%89%8B%E6%9C%BA\+Mobile/)
assert.match(encoded, /locale=all/)
assert.equal(shouldSchedulePlatformCategorySearch('shouji', true), false, 'pinyin composition must not issue a request')
assert.equal(shouldSchedulePlatformCategorySearch('手机', false), true, 'committed Chinese input must issue a request')
assert.equal(shouldSchedulePlatformCategorySearch('\u3000\t', false), false, 'Unicode whitespace must not issue a request')

assert.equal(isCurrentPlatformCategorySearch(3, 3, '手机', ' 手机 '), true)
assert.equal(isCurrentPlatformCategorySearch(2, 3, '手机', '手机'), false, 'stale response must be rejected')
assert.equal(isCurrentPlatformCategorySearch(3, 3, '手机', 'Mobile'), false, 'response for an older keyword must be rejected')

console.log('platform category search runtime tests passed')
