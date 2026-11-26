import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const storefrontSources = [
  '../src/views/ProductDetail.vue',
  '../src/views/MallProductList.vue',
  '../src/views/PlatformHome.vue',
  '../src/views/ShopList.vue',
  '../src/views/ShopHome.vue',
  '../src/views/storefront/MerchantJoin.vue',
  '../src/views/Cart.vue',
  '../src/views/storefront/MyOrders.vue',
  '../src/views/auth/CustomerLogin.vue',
  '../src/views/auth/Register.vue',
  '../src/views/auth/ForgotPassword.vue',
  '../src/views/auth/ResetPassword.vue',
  '../src/views/Account.vue',
  '../src/components/site-blocks/HomeBlockRenderer.vue',
  '../src/components/storefront/HomeCategoryNav.vue',
  '../src/components/storefront/PreviewModeNotice.vue',
  '../src/components/storefront/ProductImageGallery.vue',
  '../src/components/storefront/StorefrontFloatingBar.vue',
  '../src/components/storefront/StorefrontHeader.vue',
  '../src/components/storefront/StorefrontTrackingHead.vue',
  '../src/components/StoreAuthStatus.vue',
]

const forbiddenCopy = [
  'SKU 批量选择',
  '为一个或多个规格分别设置数量',
  '暂无可购买 SKU',
  '已选',
  '店铺资料加载中',
  '店铺商品',
  '当前店铺已上架商品',
  '我的订单',
  '请前往仓库自提',
  '同城配送订单待发货',
  '视频',
  '商家列表加载失败',
  '公开商家',
  '商家列表',
  '浏览已公开的商家主页',
  '正在加载商家',
  '商家主页',
  '暂无公开商家',
]

for (const sourcePath of storefrontSources) {
  const source = readFileSync(new URL(sourcePath, import.meta.url), 'utf8')
  for (const copy of forbiddenCopy) {
    assert.equal(source.includes(copy), false, `${sourcePath} should not hard-code ${copy}`)
  }
  assert.equal(/[\p{Script=Han}]/u.test(source), false, `${sourcePath} should not contain hard-coded Chinese UI`)
}

const enLocale = readFileSync(new URL('../src/locales/en.ts', import.meta.url), 'utf8')
const zhLocale = readFileSync(new URL('../src/locales/zh.ts', import.meta.url), 'utf8')

for (const key of [
  'batchSelection',
  'batchSelectionHint',
  'noPurchasableSku',
  'selectedCount',
  'orders',
  'shop',
  'shopList',
  'media',
]) {
  assert.match(enLocale, new RegExp(`${key}:`), `en locale should define ${key}`)
  assert.match(zhLocale, new RegExp(`${key}:`), `zh locale should define ${key}`)
}

const zhShopListCopy = [
  '商家列表加载失败',
  '公开商家',
  '商家列表',
  '浏览已公开的商家主页',
  '正在加载商家',
  '商家主页',
  '暂无公开商家',
]

for (const copy of zhShopListCopy) {
  assert.equal(enLocale.includes(copy), false, `en locale should not include Chinese shop list copy: ${copy}`)
  assert.equal(zhLocale.includes(copy), true, `zh locale should retain Chinese shop list copy: ${copy}`)
}

console.log('English storefront hard-coded Chinese copy tests passed')
