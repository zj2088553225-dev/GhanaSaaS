import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { merchantJoinSeoMeta, routeSeoMeta } from '../src/composables/useSeoMeta.ts'

const router = readFileSync(new URL('../src/router/index.ts', import.meta.url), 'utf8')
const home = readFileSync(new URL('../src/views/PlatformHome.vue', import.meta.url), 'utf8')
const page = readFileSync(new URL('../src/views/storefront/MerchantJoin.vue', import.meta.url), 'utf8')
const api = readFileSync(new URL('../src/api/merchantLeads.ts', import.meta.url), 'utf8')
const admin = readFileSync(new URL('../src/views/admin/MerchantLeads.vue', import.meta.url), 'utf8')

assert.match(router, /path:\s*'\/merchant\/join'/)
assert.match(router, /path:\s*'\/:locale\(en\|zh\)\/merchant\/join'/)
assert.match(router, /name:\s*'merchant-join'/)
assert.match(router, /path:\s*'merchant-leads'/)
assert.match(router, /adminPath\('\/merchant-leads'\)/)
assert.match(home, /localizedPath\('\/merchant\/join'\)/)

for (const field of [
  'contact_name',
  'shop_name',
  'whatsapp',
  'phone',
  'category',
  'has_offline_shop',
  'city',
  'product_count',
  'message',
]) {
  assert.match(page, new RegExp(field), `${field} should be present on merchant join page`)
  assert.match(api, new RegExp(field), `${field} should be present in merchant lead API payload`)
}

assert.match(page, /submitMerchantLead/)
assert.match(page, /VITE_MERCHANT_WHATSAPP_URL/)
assert.doesNotMatch(page, /https?:\/\/(?!www\.ghanachinatown\.com)/)
assert.doesNotMatch(page, /<iframe/i)
assert.doesNotMatch(page, /getPublishedHomeSitePage|merchantJoin\.visual|Merchant Storefront/)
assert.match(page, /merchantJoin\.preview\.title/)
assert.match(page, /merchantJoin\.preview\.live/)
assert.match(page, /merchantJoin\.preview\.categories/)
assert.match(page, /merchantJoin\.preview\.bannerTitle/)
assert.match(page, /merchantJoin\.preview\.welcomeTitle/)
assert.match(page, /merchantJoin\.preview\.featuredTitle/)
assert.match(page, /merchantJoin\.preview\.note/)
assert.match(page, /previewCategories/)
assert.match(page, /previewProducts/)
assert.match(api, /\/storefront\/merchant-leads/)
assert.match(api, /\/super\/merchant-leads/)
assert.match(admin, /招商线索 \/ Merchant Leads/)
assert.match(admin, /new/)
assert.match(admin, /contacted/)
assert.match(admin, /approved/)
assert.match(admin, /rejected/)

const seo = merchantJoinSeoMeta()
assert.equal(seo.title, 'Join Ghana Chinatown as a Merchant | Open Your Store')
assert.equal(seo.canonical, 'https://www.ghanachinatown.com/en/merchant/join')
assert.equal(merchantJoinSeoMeta('zh').canonical, 'https://www.ghanachinatown.com/zh/merchant/join')
assert.equal(seo.robots, 'index,follow')
assert.equal(routeSeoMeta('/merchant/join').canonical, seo.canonical)
assert.equal(routeSeoMeta('/zh/merchant/join').canonical, 'https://www.ghanachinatown.com/zh/merchant/join')

console.log('merchant join tests passed')
