import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const values = new Map()
const listeners = new Map()
Object.defineProperty(globalThis, 'window', {
  configurable: true,
  value: {
    localStorage: {
      getItem: (key) => values.get(key) ?? null,
      setItem: (key, value) => values.set(key, String(value)),
    },
    location: { pathname: '/en/mall' },
    addEventListener: (name, callback) => listeners.set(name, callback),
    removeEventListener: (name) => listeners.delete(name),
  },
})
Object.defineProperty(globalThis, 'document', { configurable: true, value: { referrer: '' } })
Object.defineProperty(globalThis, 'navigator', {
  configurable: true,
  value: { userAgent: 'Mozilla/5.0', sendBeacon: () => true },
})
Object.defineProperty(globalThis, 'setInterval', {
  configurable: true,
  value: () => ({ unref() {} }),
})
Object.defineProperty(globalThis, 'clearInterval', { configurable: true, value: () => {} })

const requests = []
let failNext = false
Object.defineProperty(globalThis, 'fetch', {
  configurable: true,
  value(url, init) {
    requests.push({ url, init, payload: JSON.parse(init.body) })
    if (failNext) {
      failNext = false
      return Promise.reject(new Error('response lost'))
    }
    return Promise.resolve({ ok: true, status: 202 })
  },
})

const analytics = await import('../src/utils/storefrontAnalytics.ts')
const repositorySource = readFileSync(new URL('../../internal/repository/storefront_event_repository.go', import.meta.url), 'utf8')
const analyticsRepositorySource = readFileSync(new URL('../../internal/repository/storefront_analytics_repository.go', import.meta.url), 'utf8')
const migrationSource = readFileSync(new URL('../../internal/pkg/database/storefront_event_reliability.go', import.meta.url), 'utf8')
const reliabilityDocs = readFileSync(new URL('../../docs/storefront-analytics-reliability.md', import.meta.url), 'utf8')

function click(productID) {
  return {
    event_name: 'product_click',
    product_id: productID,
    source_type: 'mall',
    source_id: 'all',
    page_type: 'mall',
  }
}

// event_id is allocated before enqueue and retained across bounded retry.
analytics.resetStorefrontAnalyticsForTests()
analytics.trackStorefrontEvent(click(42))
const queuedID = analytics.storefrontAnalyticsQueueForTests()[0].payload.event_id
assert.match(queuedID, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i)
failNext = true
await analytics.flushStorefrontAnalyticsQueue()
assert.equal(requests[0].payload.events[0].event_id, queuedID)
assert.equal(analytics.storefrontAnalyticsQueueForTests()[0].payload.event_id, queuedID)
await analytics.flushStorefrontAnalyticsQueue()
assert.equal(requests[1].payload.events[0].event_id, queuedID)

// Separate business events receive separate IDs; flush/retry never generates IDs.
analytics.resetStorefrontAnalyticsForTests()
analytics.trackStorefrontEvent(click(43))
analytics.trackStorefrontEvent(click(44))
const queued = analytics.storefrontAnalyticsQueueForTests()
assert.notEqual(queued[0].payload.event_id, queued[1].payload.event_id)

// Server contracts use a targeted DB conflict and a shared human-only filter.
assert.match(repositorySource, /Columns:\s*\[\]clause\.Column\{\{Name:\s*"event_id"\}\}/)
assert.match(repositorySource, /DoNothing:\s*true/)
assert.match(migrationSource, /ALTER COLUMN event_id SET NOT NULL/)
assert.match(migrationSource, /idx_storefront_events_event_id_unique/)
assert.match(analyticsRepositorySource, /e\.traffic_type = \?/)
assert.match(reliabilityDocs, /process-local/i)
assert.match(reliabilityDocs, /multi-instance/i)
assert.match(reliabilityDocs, /basic crawler filtering/i)

analytics.resetStorefrontAnalyticsForTests()
console.log('storefront analytics idempotency and bot filtering tests passed')
