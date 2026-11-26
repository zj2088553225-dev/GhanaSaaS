import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

class MemoryStorage {
  values = new Map()

  getItem(key) {
    return this.values.has(key) ? this.values.get(key) : null
  }

  setItem(key, value) {
    this.values.set(key, String(value))
  }

  removeItem(key) {
    this.values.delete(key)
  }

  clear() {
    this.values.clear()
  }
}

const storage = new MemoryStorage()
Object.defineProperty(globalThis, 'window', {
  configurable: true,
  value: {
    localStorage: storage,
    location: { pathname: '/en/mall' },
  },
})
Object.defineProperty(globalThis, 'document', {
  configurable: true,
  value: { referrer: 'https://www.ghanachinatown.com/en/?campaign=one' },
})

const fetchCalls = []
Object.defineProperty(globalThis, 'fetch', {
  configurable: true,
  value: (url, init) => {
    fetchCalls.push({ url, init, payload: JSON.parse(init.body) })
    return Promise.resolve({ ok: true, status: 202 })
  },
})

const analytics = await import('../src/utils/storefrontAnalytics.ts')
const trackerSource = readFileSync(new URL('../src/utils/storefrontAnalytics.ts', import.meta.url), 'utf8')
const directiveSource = readFileSync(new URL('../src/directives/storefrontImpression.ts', import.meta.url), 'utf8')
const productLink = readFileSync(new URL('../src/components/storefront/StorefrontProductLink.vue', import.meta.url), 'utf8')
const productDetail = readFileSync(new URL('../src/views/ProductDetail.vue', import.meta.url), 'utf8')
const mall = readFileSync(new URL('../src/views/MallProductList.vue', import.meta.url), 'utf8')
const shop = readFileSync(new URL('../src/views/ShopHome.vue', import.meta.url), 'utf8')
const merchantJoin = readFileSync(new URL('../src/views/storefront/MerchantJoin.vue', import.meta.url), 'utf8')
const router = readFileSync(new URL('../src/router/index.ts', import.meta.url), 'utf8')
const blockDefaults = readFileSync(new URL('../src/components/site-blocks/blockDefaults.ts', import.meta.url), 'utf8')
const blockRenderer = readFileSync(new URL('../src/components/site-blocks/HomeBlockRenderer.vue', import.meta.url), 'utf8')
const carousel = readFileSync(new URL('../src/components/site-blocks/HeroCarousel.vue', import.meta.url), 'utf8')
const homeEditor = readFileSync(new URL('../src/views/admin/SitePageHomeEditor.vue', import.meta.url), 'utf8')
const sitePageService = readFileSync(new URL('../../internal/service/site_page_service.go', import.meta.url), 'utf8')
const sitePageTrackingMigration = readFileSync(new URL('../../internal/pkg/database/site_page_tracking_keys.go', import.meta.url), 'utf8')

// 1. Anonymous visitor identity uses the required key and is reused.
storage.clear()
analytics.resetStorefrontAnalyticsForTests()
const visitorID = analytics.getStorefrontVisitorID()
assert.match(visitorID, /^[0-9a-f-]{36}$/)
assert.equal(storage.getItem('gc_visitor_id'), visitorID)
assert.equal(analytics.getStorefrontVisitorID(), visitorID)
assert.equal(analytics.STOREFRONT_VISITOR_KEY, 'gc_visitor_id')

// 2. Session activity is reused below 30 minutes and renewed after inactivity.
storage.removeItem(analytics.STOREFRONT_SESSION_KEY)
analytics.resetStorefrontAnalyticsForTests()
const firstSession = analytics.getStorefrontSession(1_000)
const activeSession = analytics.getStorefrontSession(1_000 + analytics.STOREFRONT_SESSION_TIMEOUT_MS - 1)
assert.equal(activeSession.id, firstSession.id)
storage.setItem(analytics.STOREFRONT_SESSION_KEY, JSON.stringify(firstSession))
analytics.resetStorefrontAnalyticsForTests()
const expiredSession = analytics.getStorefrontSession(1_000 + analytics.STOREFRONT_SESSION_TIMEOUT_MS)
assert.notEqual(expiredSession.id, firstSession.id)
assert.equal(analytics.STOREFRONT_SESSION_TIMEOUT_MS, 30 * 60 * 1000)

// 3-4. Calls remain fire-and-forget, enter the shared batch queue, and reject
// names outside the explicit allowlist.
fetchCalls.length = 0
const returnValue = analytics.trackStorefrontEvent({
  event_name: 'page_view',
  source_type: 'mall',
  source_id: 'all',
  page_type: 'mall',
})
assert.equal(returnValue, undefined)
assert.equal(fetchCalls.length, 0)
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 1)
await analytics.flushStorefrontAnalyticsQueue()
assert.equal(fetchCalls.length, 1)
assert.equal(fetchCalls[0].payload.events[0].visitor_id, visitorID)
assert.match(fetchCalls[0].payload.events[0].event_id, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i)
assert.equal(fetchCalls[0].payload.events.length, 1)
assert.equal(fetchCalls[0].init.keepalive, false)
assert.equal(fetchCalls[0].init.credentials, 'omit')
assert.equal('tenant_id' in fetchCalls[0].payload.events[0], false)
analytics.trackStorefrontEvent({ event_name: 'purchase' })
assert.equal(fetchCalls.length, 1)
assert.deepEqual(analytics.STOREFRONT_EVENT_NAMES, [
  'page_view',
  'product_impression',
  'product_click',
  'product_view',
  'content_impression',
  'content_click',
  'whatsapp_click',
  'phone_click',
])

// 5. product_view occurs only after the real product request resolves.
const detailLoad = productDetail.slice(productDetail.indexOf('async function loadProduct()'), productDetail.indexOf('async function replaceWithCanonicalProductPath'))
assert.ok(detailLoad.indexOf('const detail = await getProductDetail(id)') < detailLoad.indexOf('trackLoadedProductView(detail, sequence)'))
assert.match(detailLoad, /if \(sequence === loadSequence && product\.value\?\.id === detail\.id\)/)
assert.match(productDetail, /const visitKey = `\$\{visitID\}:\$\{detail\.id\}`/)
assert.match(productDetail, /viewedProductVisits\.has\(visitKey\)/)
assert.doesNotMatch(productDetail.slice(0, productDetail.indexOf('async function loadProduct()')), /event_name:\s*'product_view'/)

// 6-7. Visibility is IntersectionObserver-based and impression dedupe includes session/page/source/entity.
assert.match(directiveSource, /new IntersectionObserver/)
assert.match(directiveSource, /entry\.isIntersecting/)
assert.match(directiveSource, /currentStorefrontPageLifecycleID\(\)/)
assert.match(productLink, /void route\.fullPath/)
assert.match(trackerSource, /const dedupeKey = `\$\{session\.id\}:\$\{pageLifecycleID\}:\$\{input\.event_name\}:\$\{entityKey\}:\$\{sourceKey\}`/)
fetchCalls.length = 0
analytics.resetStorefrontAnalyticsForTests()
analytics.beginStorefrontPageLifecycle('/en/mall')
const impression = {
  event_name: 'product_impression',
  product_id: 42,
  source_type: 'mall',
  source_id: 'all',
  page_type: 'mall',
}
analytics.trackStorefrontImpression(impression)
analytics.trackStorefrontImpression(impression)
assert.equal(fetchCalls.length, 0)
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 1)

// 8-9. One reusable product link owns impression/click context; admin preview is excluded.
assert.match(productLink, /event_name:\s*eventName/)
assert.match(productLink, /product_id:\s*props\.productId/)
assert.match(productLink, /source_type:\s*props\.analyticsSource\.type/)
assert.match(productLink, /source_id:\s*props\.analyticsSource\.id/)
assert.match(productLink, /v-storefront-impression="impressionEvent"/)
assert.match(mall, /<StorefrontProductLink[\s\S]*:analytics-source="productAnalyticsSource"/)
assert.match(directiveSource, /isAdminFrontendPath\(window\.location\.pathname\)/)
assert.match(productLink, /trackingEnabled:\s*true/)
assert.doesNotMatch(homeEditor, /trackStorefrontEvent|StorefrontProductLink|v-storefront-impression/)

// 10-11. CMS nodes have stable UUID keys and localized content inherits the English structure/key.
assert.match(blockDefaults, /createTrackingKey/)
assert.match(blockDefaults, /tracking_key:\s*trackingKey\(normalized\.tracking_key\)/)
assert.match(blockDefaults, /tracking_key:\s*trackingKey\(slide\.tracking_key\)/)
assert.match(sitePageService, /normalizeSitePageTrackingKey/)
assert.match(sitePageService, /ensureLegacySitePageTrackingKeys/)
assert.match(sitePageService, /legacySitePageTrackingKey/)
assert.match(sitePageTrackingMigration, /BackfillSitePageTrackingKeys/)
assert.match(sitePageTrackingMigration, /UpdateColumns\(updates\)/)
assert.match(sitePageTrackingMigration, /forcePreferred/)
assert.match(sitePageService, /sitePageTranslationContentFromBase\(base, content, false\)/)
assert.match(homeEditor, /mergeLocalizedCmsNode\(asCmsNode\(block\)/)
assert.match(blockRenderer, /entity_type:\s*'marketing_content'/)
assert.match(blockRenderer, /`\$\{block\.tracking_key\}:\$\{category\.id\}`/)
assert.match(carousel, /slide\.tracking_key/)

// 12-14. WhatsApp/phone clicks are tracked without preventing the real navigation.
assert.match(shop, /trackShopContact\('whatsapp_click'\)/)
assert.match(shop, /trackShopContact\('phone_click'\)/)
assert.match(shop, /:href="phoneHref"/)
assert.match(merchantJoin, /trackMerchantWhatsAppClick/)
assert.match(blockRenderer, /trackMarketingContact\(block, 'whatsapp_click'\)/)
assert.match(blockRenderer, /trackMarketingContact\(block, 'phone_click'\)/)
assert.doesNotMatch(productLink, /@click\.prevent/)
assert.doesNotMatch(shop, /@click\.prevent="trackShopContact/)
assert.match(trackerSource, /enqueueStorefrontEvent\(payload\)/)
assert.match(trackerSource, /catch \(error\) \{\s*warnTrackingFailure\(error\)/)

// 15. Only explicit localized storefront route names can emit page_view.
const pageViewContext = router.slice(router.indexOf('function storefrontPageViewContext'), router.indexOf('function syncRouteLocale'))
assert.match(pageViewContext, /case 'home'/)
assert.match(pageViewContext, /case 'mall-product-list'/)
assert.match(pageViewContext, /case 'product-detail'/)
assert.match(pageViewContext, /case 'shop-home'/)
assert.match(pageViewContext, /default:\s*return null/)
assert.doesNotMatch(pageViewContext, /admin-|not-found|gc-control-7x/)
assert.match(router, /const navigationKey = pageView\.product_id/)
assert.match(router, /trackStorefrontPageView\(navigationKey, pageView\)/)

analytics.resetStorefrontAnalyticsForTests()
console.log('storefront analytics tracking tests passed')
