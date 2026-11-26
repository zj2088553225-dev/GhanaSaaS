import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import {
  DEFAULT_LOCALE,
  currentLocale,
  ensureLocalePath,
  isSupportedLocale,
  stripLocalePrefix,
  switchLocalePath,
  withLocalePath,
} from '../src/utils/localePath.ts'
import { routeSeoMeta } from '../src/composables/useSeoMeta.ts'

const router = readFileSync(new URL('../src/router/index.ts', import.meta.url), 'utf8')
const i18n = readFileSync(new URL('../src/i18n/index.ts', import.meta.url), 'utf8')
const languageSwitcher = readFileSync(new URL('../src/components/StoreLanguageSwitcher.vue', import.meta.url), 'utf8')
const header = readFileSync(new URL('../src/components/storefront/StorefrontHeader.vue', import.meta.url), 'utf8')
const home = readFileSync(new URL('../src/views/PlatformHome.vue', import.meta.url), 'utf8')
const mall = readFileSync(new URL('../src/views/MallProductList.vue', import.meta.url), 'utf8')
const productDetail = readFileSync(new URL('../src/views/ProductDetail.vue', import.meta.url), 'utf8')
const shopHome = readFileSync(new URL('../src/views/ShopHome.vue', import.meta.url), 'utf8')
const blockRenderer = readFileSync(new URL('../src/components/site-blocks/HomeBlockRenderer.vue', import.meta.url), 'utf8')

assert.equal(DEFAULT_LOCALE, 'en')
assert.equal(isSupportedLocale('en'), true)
assert.equal(isSupportedLocale('zh'), true)
assert.equal(isSupportedLocale('fr'), false)

assert.equal(withLocalePath('en', '/'), '/en')
assert.equal(withLocalePath('zh', '/'), '/zh')
assert.equal(withLocalePath('en', '/mall'), '/en/mall')
assert.equal(withLocalePath('zh', '/mall?source=t6'), '/zh/mall?source=t6')
assert.equal(withLocalePath('en', '/products/apple-iphone-12-14'), '/en/products/apple-iphone-12-14')
assert.equal(withLocalePath('zh', '/shops/yunfei-store'), '/zh/shops/yunfei-store')
assert.equal(withLocalePath('zh', '/merchant/join'), '/zh/merchant/join')
assert.equal(withLocalePath('zh', '/gc-control-7x/login'), '/gc-control-7x/login')
assert.equal(withLocalePath('zh', '/admin'), '/admin')
assert.equal(withLocalePath('zh', '/login'), '/zh/login')
assert.equal(withLocalePath('zh', '/api/v1/storefront/products'), '/api/v1/storefront/products')
assert.equal(withLocalePath('zh', '/uploads/images/demo.webp'), '/uploads/images/demo.webp')
assert.equal(withLocalePath('zh', '/sitemap.xml'), '/sitemap.xml')

assert.equal(stripLocalePrefix('/en/mall?source=t6'), '/mall?source=t6')
assert.equal(stripLocalePrefix('/zh/products/apple-iphone-12-14'), '/products/apple-iphone-12-14')
assert.equal(ensureLocalePath('/mall?source=t6', 'en'), '/en/mall?source=t6')
assert.equal(ensureLocalePath('/zh/mall?source=t6', 'en'), '/en/mall?source=t6')
assert.equal(
  switchLocalePath({ path: '/en/mall', fullPath: '/en/mall?source=t6&page=2' }, 'zh'),
  '/zh/mall?source=t6&page=2',
)
assert.equal(
  switchLocalePath({ path: '/zh/products/apple-iphone-12-14', fullPath: '/zh/products/apple-iphone-12-14' }, 'en'),
  '/en/products/apple-iphone-12-14',
)
assert.equal(currentLocale({ params: { locale: 'zh' }, path: '/zh/mall' }), 'zh')
assert.equal(currentLocale({ params: {}, path: '/mall' }), 'en')

for (const path of [
  '/:locale(en|zh)',
  '/:locale(en|zh)/mall',
  '/:locale(en|zh)/products/:productParam',
  '/:locale(en|zh)/shops/:slug',
  '/:locale(en|zh)/merchant/join',
]) {
  assert.match(router, new RegExp(`path:\\s*'${path.replace(/[|()[\]]/g, '\\$&')}'`), `${path} route should exist`)
}
assert.match(router, /redirect:\s*legacyStorefrontRedirect/)
assert.match(router, /path:\s*'\/admin'/)
assert.match(router, /path:\s*'\/login'/)
assert.match(router, /path:\s*ADMIN_BASE_PATH/)
assert.match(router, /path:\s*ADMIN_LOGIN_PATH/)
assert.match(router, /localizedStorefrontPath\(to,\s*'\/login'\)/)
assert.match(router, /localizedStorefrontPath\(to,\s*'\/account'\)/)

assert.match(i18n, /fallbackLocale:\s*DEFAULT_LOCALE/)
assert.match(i18n, /SUPPORTED_LOCALES/)
assert.match(languageSwitcher, /switchLocalePath/)
assert.match(languageSwitcher, /router\.replace/)

assert.match(header, /localizedPath\('\/merchant\/join'\)/)
assert.match(header, /localizedPath\('\/mall'\)/)
assert.match(header, /localizedPath\('\/cart'\)/)
assert.match(header, /localizedPath\('\/orders'\)/)
assert.match(home, /localizedPath\('\/merchant\/join'\)/)
assert.match(mall, /buildProductSeoPath\(product,\s*activeLocale\)/)
assert.match(productDetail, /buildProductSeoPath\(detail,\s*activeLocale\.value\)/)
assert.match(shopHome, /buildProductSeoPath\(product,\s*activeLocale\)/)
assert.match(blockRenderer, /localizedCMSLink/)
assert.match(blockRenderer, /localizedPath\('\/shops'\)/)

const t6Zh = routeSeoMeta('/zh/mall?source=t6')
assert.equal(t6Zh.canonical, 'https://www.ghanachinatown.com/zh/mall?source=t6')
assert.equal(t6Zh.alternates?.en, 'https://www.ghanachinatown.com/en/mall?source=t6')
assert.equal(t6Zh.alternates?.zh, 'https://www.ghanachinatown.com/zh/mall?source=t6')
assert.equal(t6Zh.alternates?.xDefault, 'https://www.ghanachinatown.com/en/mall?source=t6')
assert.equal(routeSeoMeta('/gc-control-7x/login').alternates, null)
assert.equal(routeSeoMeta('/en/cart').alternates, null)

console.log('locale route tests passed')
