import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import {
  absoluteURL,
  canonicalURL,
  homeSeoMeta,
  isNoindexPath,
  mallSeoMeta,
  merchantJoinSeoMeta,
  routeSeoMeta,
} from '../src/composables/useSeoMeta.ts'
import { adminPath, ADMIN_BASE_PATH, ADMIN_LOGIN_PATH } from '../src/utils/adminPath.ts'
import { buildProductSeoPath } from '../src/utils/productSeoUrl.ts'

const robots = readFileSync(new URL('../public/robots.txt', import.meta.url), 'utf8')
assert.match(robots, /Sitemap: https:\/\/www\.ghanachinatown\.com\/sitemap\.xml/)
assert.match(robots, /Disallow: \/admin\//)
assert.match(robots, /Disallow: \/gc-control-7x\//)
assert.match(robots, /Disallow: \/cart/)
assert.match(robots, /Disallow: \/checkout/)
assert.match(robots, /Disallow: \/store-login/)

assert.equal(canonicalURL('/'), 'https://www.ghanachinatown.com/en')
assert.equal(canonicalURL('/en'), 'https://www.ghanachinatown.com/en')
assert.equal(canonicalURL('/zh'), 'https://www.ghanachinatown.com/zh')
assert.equal(canonicalURL('/mall?keyword=phone'), 'https://www.ghanachinatown.com/en/mall')
assert.equal(canonicalURL('/en/mall?keyword=phone'), 'https://www.ghanachinatown.com/en/mall')
assert.equal(canonicalURL('/zh/mall?keyword=phone'), 'https://www.ghanachinatown.com/zh/mall')
assert.equal(canonicalURL('/mall?source=t6&keyword=phone'), 'https://www.ghanachinatown.com/en/mall?source=t6')
assert.equal(canonicalURL('/zh/mall?source=t6&keyword=phone'), 'https://www.ghanachinatown.com/zh/mall?source=t6')
assert.equal(canonicalURL('/mall?source=merchant&page=2'), 'https://www.ghanachinatown.com/en/mall?source=merchant')
assert.equal(canonicalURL('/products/apple-iphone-12-14'), 'https://www.ghanachinatown.com/en/products/apple-iphone-12-14')
assert.equal(canonicalURL('/zh/products/apple-iphone-12-14'), 'https://www.ghanachinatown.com/zh/products/apple-iphone-12-14')
assert.equal(canonicalURL('/shops/yunfei-store'), 'https://www.ghanachinatown.com/en/shops/yunfei-store')
assert.equal(canonicalURL('/zh/shops/yunfei-store'), 'https://www.ghanachinatown.com/zh/shops/yunfei-store')
assert.equal(absoluteURL('/uploads/product.webp'), 'https://www.ghanachinatown.com/uploads/product.webp')

const home = homeSeoMeta()
assert.equal(home.title, 'Ghana Chinatown | Ghana Chinese Marketplace')
assert.equal(home.canonical, 'https://www.ghanachinatown.com/en')
assert.equal(home.robots, 'index,follow')
assert.equal(home.alternates?.en, 'https://www.ghanachinatown.com/en')
assert.equal(home.alternates?.zh, 'https://www.ghanachinatown.com/zh')
assert.equal(home.alternates?.xDefault, 'https://www.ghanachinatown.com/en')
assert.doesNotThrow(() => JSON.parse(JSON.stringify(home.jsonLd)))
assert.equal(home.jsonLd?.[0]?.['@type'], 'WebSite')
assert.equal(home.jsonLd?.[1]?.['@type'], 'Organization')

const mall = mallSeoMeta()
assert.equal(mall.title, 'All Products | Ghana Chinatown')
assert.equal(mall.canonical, 'https://www.ghanachinatown.com/en/mall')
assert.equal(mall.description, 'Browse all public products from T6 Mall and merchants on Ghana Chinatown.')

const t6Mall = mallSeoMeta('t6')
assert.equal(t6Mall.title, 'T6 Mall Products | Ghana Chinatown')
assert.equal(t6Mall.canonical, 'https://www.ghanachinatown.com/en/mall?source=t6')
assert.equal(t6Mall.description, 'Browse official T6 Mall products on Ghana Chinatown.')
assert.equal(t6Mall.alternates?.zh, 'https://www.ghanachinatown.com/zh/mall?source=t6')

const merchantMall = mallSeoMeta('merchant', 'zh')
assert.equal(merchantMall.title, 'Merchant Products | Ghana Chinatown')
assert.equal(merchantMall.canonical, 'https://www.ghanachinatown.com/zh/mall?source=merchant')
assert.equal(merchantMall.description, 'Browse products from public merchants on Ghana Chinatown.')

const merchantJoin = merchantJoinSeoMeta()
assert.equal(merchantJoin.title, 'Join Ghana Chinatown as a Merchant | Open Your Store')
assert.equal(merchantJoin.canonical, 'https://www.ghanachinatown.com/en/merchant/join')
assert.equal(merchantJoin.robots, 'index,follow')
assert.equal(
  merchantJoin.description,
  'Apply to become a merchant on Ghana Chinatown and showcase your products to local and Chinese customers in Ghana.',
)

assert.equal(routeSeoMeta('/mall?source=t6').canonical, 'https://www.ghanachinatown.com/en/mall?source=t6')
assert.equal(routeSeoMeta('/zh/mall?source=t6').canonical, 'https://www.ghanachinatown.com/zh/mall?source=t6')
assert.equal(routeSeoMeta('/merchant/join').canonical, 'https://www.ghanachinatown.com/en/merchant/join')
assert.equal(routeSeoMeta('/zh/merchant/join').canonical, 'https://www.ghanachinatown.com/zh/merchant/join')
assert.equal(routeSeoMeta('/zh/mall?source=t6').alternates?.en, 'https://www.ghanachinatown.com/en/mall?source=t6')
assert.equal(routeSeoMeta('/zh/mall?source=t6').alternates?.zh, 'https://www.ghanachinatown.com/zh/mall?source=t6')
assert.equal(routeSeoMeta('/zh/mall?source=t6').alternates?.xDefault, 'https://www.ghanachinatown.com/en/mall?source=t6')
assert.equal(routeSeoMeta('/en/products/apple-iphone-12-14').canonical, 'https://www.ghanachinatown.com/en/products/apple-iphone-12-14')
assert.equal(routeSeoMeta('/zh/products/apple-iphone-12-14').canonical, 'https://www.ghanachinatown.com/zh/products/apple-iphone-12-14')
assert.equal(routeSeoMeta('/zh/products/apple-iphone-12-14').alternates?.en, 'https://www.ghanachinatown.com/en/products/apple-iphone-12-14')
assert.equal(routeSeoMeta('/zh/products/apple-iphone-12-14').alternates?.zh, 'https://www.ghanachinatown.com/zh/products/apple-iphone-12-14')
assert.equal(routeSeoMeta('/zh/products/apple-iphone-12-14').alternates?.xDefault, 'https://www.ghanachinatown.com/en/products/apple-iphone-12-14')
assert.equal(routeSeoMeta('/en/shops/yunfei-store').canonical, 'https://www.ghanachinatown.com/en/shops/yunfei-store')
assert.equal(routeSeoMeta('/zh/shops/yunfei-store').canonical, 'https://www.ghanachinatown.com/zh/shops/yunfei-store')
assert.equal(routeSeoMeta('/zh/shops/yunfei-store').alternates?.en, 'https://www.ghanachinatown.com/en/shops/yunfei-store')
assert.equal(routeSeoMeta('/zh/shops/yunfei-store').alternates?.zh, 'https://www.ghanachinatown.com/zh/shops/yunfei-store')
assert.equal(routeSeoMeta('/zh/shops/yunfei-store').alternates?.xDefault, 'https://www.ghanachinatown.com/en/shops/yunfei-store')

assert.equal(ADMIN_BASE_PATH, '/gc-control-7x')
assert.equal(ADMIN_LOGIN_PATH, '/gc-control-7x/login')

for (const path of ['/403', '/admin', '/admin/products', '/login', '/en/register', '/zh/forgot-password', '/en/reset-password', '/zh/account', adminPath(), adminPath('/products'), ADMIN_LOGIN_PATH, '/store-login', '/en/store-login', '/zh/cart', '/cart', '/checkout', '/orders', '/orders/success', '/under-construction', '/reservations/mine', '/en/reservations/mine', '/zh/reservations/mine']) {
  assert.equal(isNoindexPath(path), true, `${path} should be noindex`)
  assert.equal(routeSeoMeta(path).robots, 'noindex,nofollow,noarchive')
  assert.equal(routeSeoMeta(path).alternates, null)
}
for (const path of ['/', '/en', '/zh', '/mall', '/en/mall', '/zh/mall', '/merchant/join', '/zh/merchant/join', '/shops/yunfei-store', '/zh/shops/yunfei-store', '/products/demo-product-1', '/en/products/demo-product-1']) {
  assert.equal(isNoindexPath(path), false, `${path} should be indexable`)
}

const productSchema = {
  '@context': 'https://schema.org',
  '@type': 'Product',
  name: 'Demo Product',
  description: 'A clean product description.',
  image: ['https://www.ghanachinatown.com/uploads/product.webp'],
  brand: { '@type': 'Brand', name: 'Ghana Chinatown' },
  url: canonicalURL(buildProductSeoPath({ id: 1, name: 'Demo Product' }, 'zh')),
}
assert.doesNotThrow(() => JSON.parse(JSON.stringify(productSchema)))
assert.equal(Object.prototype.hasOwnProperty.call(productSchema, 'offers'), false)
assert.equal(productSchema.url, 'https://www.ghanachinatown.com/zh/products/demo-product-1')

console.log('seo meta tests passed')
