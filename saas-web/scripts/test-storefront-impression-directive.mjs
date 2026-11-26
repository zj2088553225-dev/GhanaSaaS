import assert from 'node:assert/strict'
import { createServer } from 'vite'

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
}

const storage = new MemoryStorage()
const location = { pathname: '/en/mall' }
Object.defineProperty(globalThis, 'window', {
  configurable: true,
  value: { localStorage: storage, location },
})
Object.defineProperty(globalThis, 'document', {
  configurable: true,
  value: { referrer: '' },
})
Object.defineProperty(globalThis, 'navigator', {
  configurable: true,
  value: { userAgent: 'Mozilla/5.0 Chrome/140' },
})
Object.defineProperty(globalThis, 'fetch', {
  configurable: true,
  value: () => Promise.resolve({ ok: true, status: 202 }),
})

const observers = []
class FakeIntersectionObserver {
  disconnected = false
  observed = []

  constructor(callback, options) {
    this.callback = callback
    this.options = options
    observers.push(this)
  }

  observe(element) {
    this.observed.push(element)
  }

  disconnect() {
    this.disconnected = true
  }

  emit(isIntersecting, intersectionRatio) {
    this.callback([{ isIntersecting, intersectionRatio }])
  }
}
Object.defineProperty(globalThis, 'IntersectionObserver', {
  configurable: true,
  value: FakeIntersectionObserver,
})

const vite = await createServer({
  configFile: false,
  appType: 'custom',
  server: { middlewareMode: true },
  optimizeDeps: { noDiscovery: true },
})
const analytics = await vite.ssrLoadModule('/src/utils/storefrontAnalytics.ts')
const directiveModule = await vite.ssrLoadModule('/src/directives/storefrontImpression.ts')
const directive = directiveModule.vStorefrontImpression
const mounted = directive.mounted
const updated = directive.updated
const unmounted = directive.unmounted
assert.equal(typeof mounted, 'function')
assert.equal(typeof updated, 'function')
assert.equal(typeof unmounted, 'function')
assert.equal(directiveModule.STOREFRONT_IMPRESSION_THRESHOLD, 0.15)

analytics.resetStorefrontAnalyticsForTests()
await vite.close()
analytics.beginStorefrontPageLifecycle('/en/mall')
const element = {}
const firstEvent = {
  event_name: 'product_impression',
  product_id: 41,
  source_type: 'mall',
  source_id: 'all',
  page_type: 'mall',
}
mounted(element, { value: firstEvent })
assert.equal(observers.length, 1)
assert.deepEqual(observers[0].options, { threshold: 0.15 })
assert.deepEqual(observers[0].observed, [element])

observers[0].emit(false, 1)
observers[0].emit(true, 0.149999)
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 0)
assert.equal(observers[0].disconnected, false)

observers[0].emit(true, 0.15)
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 1)
assert.equal(observers[0].disconnected, true)
updated(element, { value: firstEvent })
assert.equal(observers.length, 1, 'same page/entity/source signature must not observe twice')

const secondEvent = { ...firstEvent, product_id: 42 }
updated(element, { value: secondEvent })
assert.equal(observers.length, 2)
observers[1].emit(true, 0.15)
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 2)
unmounted(element)
assert.equal(observers[1].disconnected, true)

delete globalThis.IntersectionObserver
const fallbackElement = {}
mounted(fallbackElement, {
  value: { ...firstEvent, product_id: 43 },
})
await Promise.resolve()
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 3)
unmounted(fallbackElement)

Object.defineProperty(globalThis, 'IntersectionObserver', {
  configurable: true,
  value: FakeIntersectionObserver,
})
location.pathname = '/admin/analytics'
const adminElement = {}
mounted(adminElement, {
  value: { ...firstEvent, product_id: 44 },
})
observers.at(-1).emit(true, 1)
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 3)
unmounted(adminElement)

analytics.resetStorefrontAnalyticsForTests()
