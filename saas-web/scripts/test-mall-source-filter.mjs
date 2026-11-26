import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import en from '../src/locales/en.ts'
import zh from '../src/locales/zh.ts'
import {
  mallSourceQueryValue,
  mallSourceRouteQuery,
  normalizeMallSource,
  productSourceLinkQuery,
  productSourceParam,
} from '../src/utils/mallSource.ts'

const adminLayout = readFileSync(new URL('../src/layouts/AdminLayout.vue', import.meta.url), 'utf8')

assert.equal(normalizeMallSource(undefined), 'all')
assert.equal(normalizeMallSource('all'), 'all')
assert.equal(normalizeMallSource('t6'), 't6')
assert.equal(normalizeMallSource('merchant'), 'merchant')
assert.equal(normalizeMallSource('unsupported'), 'all')

assert.equal(mallSourceQueryValue('all'), undefined)
assert.equal(mallSourceQueryValue('t6'), 't6')
assert.equal(mallSourceQueryValue('merchant'), 'merchant')

assert.deepEqual(
  mallSourceRouteQuery(
    {
      keyword: 'phone',
      platformCategoryId: 123,
      sort: 'hot',
    },
    't6',
  ),
  {
    keyword: 'phone',
    platform_category_id: '123',
    sort: 'hot',
    source: 't6',
  },
)

assert.deepEqual(
  mallSourceRouteQuery(
    {
      keyword: 'phone',
      platformCategoryId: 123,
      sort: 'hot',
    },
    'all',
  ),
  {
    keyword: 'phone',
    platform_category_id: '123',
    sort: 'hot',
  },
)

assert.equal(productSourceParam('t6'), 't6')
assert.equal(productSourceParam('merchant'), 'merchant')
assert.equal(productSourceParam('featured'), 't6')
assert.equal(productSourceParam('all'), undefined)
assert.deepEqual(productSourceLinkQuery('merchant'), { source: 'merchant' })
assert.deepEqual(productSourceLinkQuery('all'), {})

assert.equal(en.nav.home, 'Home')
assert.equal(en.nav.allProducts, 'All Products')
assert.equal(en.nav.t6Mall, 'T6 Mall')
assert.equal(en.nav.cart, 'Cart')
assert.equal(en.nav.orders, 'My Orders')
assert.equal(zh.nav.home, '首页')
assert.equal(zh.nav.allProducts, '全部商品')
assert.equal(zh.nav.t6Mall, 'T6商城')
assert.equal(zh.nav.cart, '购物车')
assert.equal(zh.nav.orders, '我的订单')
assert.equal(adminLayout.includes("label: '自营商品'"), false)

console.log('mall source filter tests passed')
