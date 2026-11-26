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
}

const storage = new MemoryStorage()
const listeners = new Map()
const addedListeners = []
const removedListeners = []
const intervalCallbacks = []
const clearedIntervals = []

Object.defineProperty(globalThis, 'window', {
  configurable: true,
  value: {
    localStorage: storage,
    location: { pathname: '/en/mall' },
    addEventListener(name, callback) {
      addedListeners.push(name)
      listeners.set(name, callback)
    },
    removeEventListener(name, callback) {
      removedListeners.push(name)
      if (listeners.get(name) === callback) listeners.delete(name)
    },
  },
})
Object.defineProperty(globalThis, 'document', {
  configurable: true,
  value: { referrer: '' },
})
Object.defineProperty(globalThis, 'navigator', {
  configurable: true,
  value: { userAgent: 'Mozilla/5.0', sendBeacon: () => true },
})
Object.defineProperty(globalThis, 'setInterval', {
  configurable: true,
  value(callback, milliseconds) {
    intervalCallbacks.push({ callback, milliseconds })
    return { id: intervalCallbacks.length, unref() {} }
  },
})
Object.defineProperty(globalThis, 'clearInterval', {
  configurable: true,
  value(timer) {
    clearedIntervals.push(timer)
  },
})

const fetchCalls = []
let fetchHandler = (_url, _init) => Promise.resolve({ ok: true, status: 202 })
Object.defineProperty(globalThis, 'fetch', {
  configurable: true,
  value(url, init) {
    fetchCalls.push({ url, init, payload: JSON.parse(init.body) })
    return fetchHandler(url, init)
  },
})

const analytics = await import('../src/utils/storefrontAnalytics.ts')
const trackerSource = readFileSync(new URL('../src/utils/storefrontAnalytics.ts', import.meta.url), 'utf8')
const deliveryDocs = readFileSync(new URL('../../docs/storefront-analytics-delivery.md', import.meta.url), 'utf8')

function pageView(index = 0) {
  return {
    event_name: 'page_view',
    source_type: 'direct',
    source_id: `page-${index}`,
    page_type: 'mall',
    page_path: '/en/mall',
  }
}

function impression(index) {
  return {
    event_name: 'product_impression',
    product_id: index + 1,
    source_type: 'mall',
    source_id: 'all',
    page_type: 'mall',
    page_path: '/en/mall',
  }
}

function click(index) {
  return {
    event_name: 'product_click',
    product_id: index + 1,
    source_type: 'mall',
    source_id: 'all',
    page_type: 'mall',
    page_path: '/en/mall',
  }
}

function deferred() {
  let resolve
  let reject
  const promise = new Promise((promiseResolve, promiseReject) => {
    resolve = promiseResolve
    reject = promiseReject
  })
  return { promise, resolve, reject }
}

async function beaconEvents(calls) {
  const payloads = await Promise.all(calls.map(async (call) => JSON.parse(await call.body.text())))
  return payloads.flatMap((payload) => payload.events)
}

async function settlePromises() {
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
  await Promise.resolve()
}

const beaconCalls = []

// 1-4. Events enter a queue, flush at 20 or two seconds, and use one batch API.
analytics.resetStorefrontAnalyticsForTests()
fetchCalls.length = 0
analytics.trackStorefrontEvent(pageView())
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 1)
assert.equal(fetchCalls.length, 0)
assert.equal(analytics.STOREFRONT_ANALYTICS_BATCH_SIZE, 20)
assert.equal(analytics.STOREFRONT_ANALYTICS_FLUSH_INTERVAL_MS, 2_000)
assert.equal(intervalCallbacks.at(-1).milliseconds, 2_000)
intervalCallbacks.at(-1).callback()
await analytics.flushStorefrontAnalyticsQueue()
assert.equal(fetchCalls.length, 1)
assert.match(fetchCalls[0].url, /\/storefront\/events\/batch$/)
assert.equal(fetchCalls[0].payload.events.length, 1)

analytics.resetStorefrontAnalyticsForTests()
fetchCalls.length = 0
for (let index = 0; index < 20; index += 1) analytics.trackStorefrontEvent(pageView(index))
await analytics.flushStorefrontAnalyticsQueue()
assert.equal(fetchCalls.length, 1)
assert.equal(fetchCalls[0].payload.events.length, 20)

// 5-7. The 500-item cap drops impressions before a queued click.
analytics.resetStorefrontAnalyticsForTests()
fetchCalls.length = 0
let releasePending
fetchHandler = () => new Promise((resolve) => { releasePending = resolve })
for (let index = 0; index < 20; index += 1) analytics.trackStorefrontEvent(click(index))
analytics.trackStorefrontEvent(click(999))
for (let index = 0; index < 500; index += 1) analytics.trackStorefrontEvent(impression(index))
const cappedQueue = analytics.storefrontAnalyticsQueueForTests()
assert.equal(cappedQueue.length, analytics.STOREFRONT_ANALYTICS_MAX_QUEUE_SIZE)
assert.ok(cappedQueue.some((event) => event.payload.event_name === 'product_click' && event.payload.product_id === 1000))
assert.ok(analytics.storefrontAnalyticsDroppedCountForTests() >= 1)
analytics.resetStorefrontAnalyticsForTests()
releasePending({ ok: true, status: 202 })
await settlePromises()

// 8. A second full batch cannot start while the first normal flush is pending.
fetchCalls.length = 0
let firstResolve
fetchHandler = () => new Promise((resolve) => { firstResolve = resolve })
for (let index = 0; index < 20; index += 1) analytics.trackStorefrontEvent(pageView(index))
for (let index = 20; index < 40; index += 1) analytics.trackStorefrontEvent(pageView(index))
assert.equal(fetchCalls.length, 1)
fetchHandler = () => Promise.resolve({ ok: true, status: 202 })
firstResolve({ ok: true, status: 202 })
await Promise.resolve()
await Promise.resolve()
await analytics.flushStorefrontAnalyticsQueue()
assert.equal(fetchCalls.length, 2)

// 9. Failed batches retain their payload for only two retries, then drop.
analytics.resetStorefrontAnalyticsForTests()
fetchCalls.length = 0
fetchHandler = () => Promise.reject(new Error('network down'))
analytics.trackStorefrontEvent(click(1))
await analytics.flushStorefrontAnalyticsQueue()
assert.equal(analytics.storefrontAnalyticsQueueForTests()[0].attempts, 1)
await analytics.flushStorefrontAnalyticsQueue()
assert.equal(analytics.storefrontAnalyticsQueueForTests()[0].attempts, 2)
await analytics.flushStorefrontAnalyticsQueue()
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 0)
assert.equal(fetchCalls.length, 3)

// 10-12. pagehide uses sendBeacon once; a false result uses keepalive fetch.
analytics.resetStorefrontAnalyticsForTests()
fetchHandler = () => Promise.resolve({ ok: true, status: 202 })
fetchCalls.length = 0
beaconCalls.length = 0
navigator.sendBeacon = (url, body) => {
  beaconCalls.push({ url, body })
  return true
}
analytics.trackStorefrontEvent(click(2))
analytics.trackStorefrontEvent(click(3))
assert.equal(addedListeners.filter((name) => name === 'pagehide').length > 0, true)
listeners.get('pagehide')()
assert.equal(beaconCalls.length, 1)
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 0)
assert.equal(JSON.parse(await beaconCalls[0].body.text()).events.length, 2)
assert.equal(fetchCalls.length, 0)

analytics.resetStorefrontAnalyticsForTests()
fetchCalls.length = 0
navigator.sendBeacon = () => false
analytics.trackStorefrontEvent(click(4))
analytics.flushStorefrontAnalyticsOnPageHide()
await Promise.resolve()
assert.equal(fetchCalls.length, 1)
assert.equal(fetchCalls[0].init.keepalive, true)

// 13. A pagehide during the first unresolved 20-event flush includes the
// complete in-flight batch and retains every original event_id.
analytics.resetStorefrontAnalyticsForTests()
fetchCalls.length = 0
beaconCalls.length = 0
const p1Pending = deferred()
fetchHandler = () => p1Pending.promise
navigator.sendBeacon = (url, body) => {
  beaconCalls.push({ url, body })
  return true
}
for (let index = 0; index < 19; index += 1) analytics.trackStorefrontEvent(pageView(index))
analytics.trackStorefrontEvent(click(999))
assert.equal(fetchCalls.length, 1)
assert.equal(fetchCalls[0].payload.events.length, 20)
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 0)
assert.equal(analytics.storefrontAnalyticsInFlightBatchForTests().length, 20)
const p1NormalEvents = fetchCalls[0].payload.events
const p1NormalIDs = p1NormalEvents.map((event) => event.event_id)
listeners.get('pagehide')()
const p1BeaconEvents = await beaconEvents(beaconCalls)
assert.equal(p1BeaconEvents.length, 20)
assert.deepEqual(p1BeaconEvents.map((event) => event.event_id), p1NormalIDs)
assert.ok(p1BeaconEvents.some((event) => event.event_name === 'product_click' && event.product_id === 1000))
assert.equal(analytics.storefrontAnalyticsInFlightBatchForTests().length, 20)
p1Pending.resolve({ ok: true, status: 202 })
await analytics.flushStorefrontAnalyticsQueue()
assert.equal(analytics.storefrontAnalyticsInFlightBatchForTests().length, 0)
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 0)

// 14. New queued events are appended after the older in-flight batch in the
// pagehide snapshot.
analytics.resetStorefrontAnalyticsForTests()
fetchCalls.length = 0
beaconCalls.length = 0
const pendingWithQueue = deferred()
fetchHandler = () => pendingWithQueue.promise
navigator.sendBeacon = (url, body) => {
  beaconCalls.push({ url, body })
  return true
}
for (let index = 0; index < 20; index += 1) analytics.trackStorefrontEvent(pageView(index))
const pendingNormalIDs = fetchCalls[0].payload.events.map((event) => event.event_id)
assert.equal(analytics.beginStorefrontPageLifecycle('/en/product/2001'), true)
for (let index = 0; index < 5; index += 1) analytics.trackStorefrontEvent(click(index + 2000))
const pendingQueueIDs = analytics.storefrontAnalyticsQueueForTests().map((event) => event.payload.event_id)
listeners.get('pagehide')()
const combinedBeaconEvents = await beaconEvents(beaconCalls)
assert.equal(combinedBeaconEvents.length, 25)
assert.deepEqual(combinedBeaconEvents.map((event) => event.event_id), [...pendingNormalIDs, ...pendingQueueIDs])
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 0)
pendingWithQueue.resolve({ ok: true, status: 202 })
await analytics.flushStorefrontAnalyticsQueue()
assert.equal(analytics.storefrontAnalyticsInFlightBatchForTests().length, 0)

// 15. A rejected beacon falls back to keepalive fetch with the in-flight IDs.
analytics.resetStorefrontAnalyticsForTests()
fetchCalls.length = 0
const pendingWithFallback = deferred()
fetchHandler = (_url, init) => init.keepalive
  ? Promise.resolve({ ok: true, status: 202 })
  : pendingWithFallback.promise
navigator.sendBeacon = () => false
for (let index = 0; index < 20; index += 1) analytics.trackStorefrontEvent(pageView(index))
const fallbackExpectedIDs = fetchCalls[0].payload.events.map((event) => event.event_id)
listeners.get('pagehide')()
const keepaliveCall = fetchCalls.find((call) => call.init.keepalive)
assert.ok(keepaliveCall)
assert.deepEqual(keepaliveCall.payload.events.map((event) => event.event_id), fallbackExpectedIDs)
pendingWithFallback.resolve({ ok: true, status: 202 })
await analytics.flushStorefrontAnalyticsQueue()

// 16. Repeated pagehide in one lifecycle does not send the snapshot twice.
analytics.resetStorefrontAnalyticsForTests()
fetchCalls.length = 0
beaconCalls.length = 0
const pendingRepeatedPagehide = deferred()
fetchHandler = () => pendingRepeatedPagehide.promise
navigator.sendBeacon = (url, body) => {
  beaconCalls.push({ url, body })
  return true
}
for (let index = 0; index < 20; index += 1) analytics.trackStorefrontEvent(pageView(index))
listeners.get('pagehide')()
listeners.get('pagehide')()
assert.equal(beaconCalls.length, 1)
pendingRepeatedPagehide.resolve({ ok: true, status: 202 })
await analytics.flushStorefrontAnalyticsQueue()

// 17. A normal failure that finishes before pagehide is represented only by
// its requeued batch, with no duplicate event_id in the snapshot.
analytics.resetStorefrontAnalyticsForTests()
fetchCalls.length = 0
beaconCalls.length = 0
fetchHandler = () => Promise.reject(new Error('normal flush failed'))
navigator.sendBeacon = (url, body) => {
  beaconCalls.push({ url, body })
  return true
}
for (let index = 0; index < 20; index += 1) analytics.trackStorefrontEvent(pageView(index))
const failedNormalIDs = fetchCalls[0].payload.events.map((event) => event.event_id)
await analytics.flushStorefrontAnalyticsQueue()
assert.equal(analytics.storefrontAnalyticsInFlightBatchForTests().length, 0)
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 20)
listeners.get('pagehide')()
const failedNormalBeaconEvents = await beaconEvents(beaconCalls)
const failedNormalBeaconIDs = failedNormalBeaconEvents.map((event) => event.event_id)
assert.equal(failedNormalBeaconEvents.length, 20)
assert.deepEqual(failedNormalBeaconIDs, failedNormalIDs)
assert.equal(new Set(failedNormalBeaconIDs).size, 20)

// 18. A completed normal batch is no longer in-flight and is not included in
// a later pagehide snapshot.
analytics.resetStorefrontAnalyticsForTests()
fetchCalls.length = 0
beaconCalls.length = 0
fetchHandler = () => Promise.resolve({ ok: true, status: 202 })
navigator.sendBeacon = (url, body) => {
  beaconCalls.push({ url, body })
  return true
}
for (let index = 0; index < 20; index += 1) analytics.trackStorefrontEvent(pageView(index))
const completedIDs = fetchCalls[0].payload.events.map((event) => event.event_id)
await analytics.flushStorefrontAnalyticsQueue()
assert.equal(analytics.storefrontAnalyticsInFlightBatchForTests().length, 0)
for (let index = 0; index < 5; index += 1) analytics.trackStorefrontEvent(click(index + 3000))
const laterQueuedIDs = analytics.storefrontAnalyticsQueueForTests().map((event) => event.payload.event_id)
listeners.get('pagehide')()
const laterBeaconIDs = (await beaconEvents(beaconCalls)).map((event) => event.event_id)
assert.deepEqual(laterBeaconIDs, laterQueuedIDs)
assert.equal(laterBeaconIDs.some((eventID) => completedIDs.includes(eventID)), false)

// 19. Snapshot de-duplication is defensive even if a failed keepalive copy is
// requeued while the original normal request remains unresolved.
analytics.resetStorefrontAnalyticsForTests()
fetchCalls.length = 0
beaconCalls.length = 0
const pendingDuplicateDefense = deferred()
fetchHandler = (_url, init) => init.keepalive
  ? Promise.reject(new Error('keepalive failed'))
  : pendingDuplicateDefense.promise
navigator.sendBeacon = () => false
for (let index = 0; index < 20; index += 1) analytics.trackStorefrontEvent(pageView(index))
const duplicateDefenseIDs = fetchCalls[0].payload.events.map((event) => event.event_id)
listeners.get('pagehide')()
await settlePromises()
assert.equal(analytics.storefrontAnalyticsInFlightBatchForTests().length, 20)
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 20)
listeners.get('pageshow')()
navigator.sendBeacon = (url, body) => {
  beaconCalls.push({ url, body })
  return true
}
listeners.get('pagehide')()
const deduplicatedBeaconIDs = (await beaconEvents(beaconCalls)).map((event) => event.event_id)
assert.deepEqual(deduplicatedBeaconIDs, duplicateDefenseIDs)
assert.equal(new Set(deduplicatedBeaconIDs).size, 20)
pendingDuplicateDefense.resolve({ ok: true, status: 202 })
await analytics.flushStorefrontAnalyticsQueue()

// 20. A retry request that is itself pending is still represented in the
// pagehide snapshot with the original event IDs.
analytics.resetStorefrontAnalyticsForTests()
fetchCalls.length = 0
beaconCalls.length = 0
const pendingRetryRequest = deferred()
let normalRetryCalls = 0
fetchHandler = (_url, init) => {
  if (init.keepalive) return Promise.resolve({ ok: true, status: 202 })
  normalRetryCalls += 1
  if (normalRetryCalls === 1) return Promise.reject(new Error('first request failed'))
  return pendingRetryRequest.promise
}
navigator.sendBeacon = (url, body) => {
  beaconCalls.push({ url, body })
  return true
}
for (let index = 0; index < 20; index += 1) analytics.trackStorefrontEvent(click(index + 4000))
const retryPendingIDs = fetchCalls[0].payload.events.map((event) => event.event_id)
await analytics.flushStorefrontAnalyticsQueue()
const retryPendingFlush = analytics.flushStorefrontAnalyticsQueue()
assert.equal(analytics.storefrontAnalyticsInFlightBatchForTests().length, 20)
listeners.get('pagehide')()
assert.deepEqual((await beaconEvents(beaconCalls)).map((event) => event.event_id), retryPendingIDs)
pendingRetryRequest.resolve({ ok: true, status: 202 })
await retryPendingFlush

// 21-22. Both an all-impression queue and an all-high-value queue remain
// bounded at 500. High-value additions displace impressions, while an
// impression cannot displace a high-value event.
analytics.resetStorefrontAnalyticsForTests()
fetchCalls.length = 0
const pendingImpressionCap = deferred()
fetchHandler = () => pendingImpressionCap.promise
for (let index = 0; index < 520; index += 1) analytics.trackStorefrontEvent(impression(index + 5000))
assert.equal(analytics.storefrontAnalyticsInFlightBatchForTests().length, 20)
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 500)
analytics.trackStorefrontEvent(click(9999))
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 500)
assert.ok(analytics.storefrontAnalyticsQueueForTests().some((event) =>
  event.payload.event_name === 'product_click' && event.payload.product_id === 10000))
analytics.trackStorefrontEvent(impression(10000))
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 500)
assert.ok(analytics.storefrontAnalyticsDroppedCountForTests() >= 2)
analytics.resetStorefrontAnalyticsForTests()
pendingImpressionCap.resolve({ ok: true, status: 202 })
await settlePromises()

fetchCalls.length = 0
const pendingHighValueCap = deferred()
fetchHandler = () => pendingHighValueCap.promise
for (let index = 0; index < 520; index += 1) analytics.trackStorefrontEvent(click(index + 11000))
assert.equal(analytics.storefrontAnalyticsInFlightBatchForTests().length, 20)
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 500)
analytics.trackStorefrontEvent(impression(12000))
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 500)
assert.equal(analytics.storefrontAnalyticsQueueForTests().some((event) =>
  event.payload.event_name === 'product_impression'), false)
analytics.trackStorefrontEvent(click(12001))
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 500)
assert.ok(analytics.storefrontAnalyticsQueueForTests().some((event) =>
  event.payload.event_name === 'product_click' && event.payload.product_id === 12002))
analytics.resetStorefrontAnalyticsForTests()
pendingHighValueCap.resolve({ ok: true, status: 202 })
await settlePromises()
analytics.trackStorefrontEvent(pageView(13000))

// 23-28. There is no blocking unload handler or auth dependency; initialization
// is singleton, reset disposes it, no event queue is persisted, and docs state
// the honest in-memory best-effort boundary.
assert.doesNotMatch(trackerSource, /beforeunload/)
assert.doesNotMatch(trackerSource, /Authorization/)
assert.equal(addedListeners.filter((name) => name === 'pagehide').length, removedListeners.filter((name) => name === 'pagehide').length + 1)
assert.match(trackerSource, /if \(deliveryInitialized\)/)
analytics.resetStorefrontAnalyticsForTests()
assert.equal(addedListeners.filter((name) => name === 'pagehide').length, removedListeners.filter((name) => name === 'pagehide').length)
assert.equal(addedListeners.filter((name) => name === 'pageshow').length, removedListeners.filter((name) => name === 'pageshow').length)
assert.ok(clearedIntervals.length > 0)
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 0)
assert.equal(analytics.storefrontAnalyticsInFlightBatchForTests().length, 0)
assert.deepEqual([...storage.values.keys()].sort(), ['gc_session', 'gc_visitor_id'])
assert.match(deliveryDocs, /in-memory best-effort delivery with bounded retries/i)
assert.match(deliveryDocs, /not guaranteed\s+at-least-once/i)
assert.match(deliveryDocs, /not exactly-once/i)
assert.match(deliveryDocs, /does not mean that the server\s+acknowledged persistence/i)
assert.match(deliveryDocs, /server request-body hard limit of 64 KiB/i)
assert.match(deliveryDocs, /48 KiB delivery budget/i)
assert.match(deliveryDocs, /TextEncoder/)
assert.doesNotMatch(trackerSource, /localStorage.*queue|queue.*localStorage/i)

console.log('storefront analytics batch delivery tests passed')
