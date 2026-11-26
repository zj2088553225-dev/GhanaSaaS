import assert from 'node:assert/strict'

class MemoryStorage {
  values = new Map()

  getItem(key) {
    return this.values.has(key) ? this.values.get(key) : null
  }

  setItem(key, value) {
    this.values.set(key, String(value))
  }
}

const storage = new MemoryStorage()
const listeners = new Map()

Object.defineProperty(globalThis, 'window', {
  configurable: true,
  value: {
    localStorage: storage,
    location: { pathname: '/zh/unicode' },
    addEventListener(name, callback) {
      listeners.set(name, callback)
    },
    removeEventListener(name, callback) {
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
  value: () => ({ unref() {} }),
})
Object.defineProperty(globalThis, 'clearInterval', {
  configurable: true,
  value: () => {},
})

const utf8 = new TextEncoder()
const fetchCalls = []
let fetchHandler = () => Promise.resolve({ ok: true, status: 202 })
Object.defineProperty(globalThis, 'fetch', {
  configurable: true,
  value(url, init) {
    const call = {
      url,
      init,
      bodyBytes: utf8.encode(init.body).byteLength,
      payload: JSON.parse(init.body),
    }
    fetchCalls.push(call)
    return fetchHandler(url, init)
  },
})

const analytics = await import('../src/utils/storefrontAnalytics.ts')
const SERVER_BATCH_BODY_LIMIT = 64 * 1024

function deferred() {
  let resolve
  let reject
  const promise = new Promise((promiseResolve, promiseReject) => {
    resolve = promiseResolve
    reject = promiseReject
  })
  return { promise, resolve, reject }
}

function repeatCodePoint(character, count) {
  return Array.from({ length: count }, () => character).join('')
}

function eventID(index) {
  const value = index.toString(16)
  return `${value.padStart(8, '0')}-aaaa-4aaa-8aaa-${value.padStart(12, '0')}`
}

function unicodeEventInput(index, character) {
  const referrerPrefix = 'https://example.com/'
  const pagePath = `/${repeatCodePoint(character, 511)}`
  const referrer = `${referrerPrefix}${repeatCodePoint(character, 1024 - Array.from(referrerPrefix).length)}`
  assert.equal(Array.from(pagePath).length, 512)
  assert.equal(Array.from(referrer).length, 1024)
  return {
    event_name: 'page_view',
    entity_type: 'storefront',
    entity_id: `unicode-${index}`,
    source_type: 'direct',
    source_id: `unicode-${index}`,
    page_type: 'other',
    page_path: pagePath,
    locale: 'zh',
    referrer,
    device_type: 'desktop',
  }
}

function unicodeEventPayload(index, character) {
  return {
    ...unicodeEventInput(index, character),
    event_id: eventID(index),
    visitor_id: '11111111-1111-4111-8111-111111111111',
    session_id: '22222222-2222-4222-8222-222222222222',
  }
}

function deliveryStatePayloads() {
  return [
    ...analytics.storefrontAnalyticsInFlightBatchForTests(),
    ...analytics.storefrontAnalyticsQueueForTests(),
  ].map((event) => event.payload)
}

function assertByteSafeCalls(calls) {
  for (const call of calls) {
    assert.ok(call.bodyBytes <= analytics.STOREFRONT_ANALYTICS_BATCH_BYTE_BUDGET)
    assert.ok(call.payload.events.length <= analytics.STOREFRONT_ANALYTICS_BATCH_SIZE)
    assert.equal(call.bodyBytes, analytics.storefrontAnalyticsBatchByteSize(call.payload.events))
  }
}

async function beaconPayloads(calls) {
  const payloads = await Promise.all(calls.map(async (call) => JSON.parse(await call.body.text())))
  return payloads.flatMap((payload) => payload.events)
}

async function waitForNormalDelivery() {
  for (let attempt = 0; attempt < 100; attempt += 1) {
    await new Promise((resolve) => setImmediate(resolve))
    if (analytics.storefrontAnalyticsQueueForTests().length === 0 &&
      analytics.storefrontAnalyticsInFlightBatchForTests().length === 0) {
      return
    }
  }
  throw new Error('normal analytics delivery did not settle')
}

function assertCompleteOrderedDelivery(actualEvents, expectedIDs) {
  const actualIDs = actualEvents.map((event) => event.event_id)
  assert.deepEqual(actualIDs, expectedIDs)
  assert.equal(new Set(actualIDs).size, expectedIDs.length)
}

// 1-2. The pure splitter measures the final JSON envelope with real UTF-8
// bytes and preserves every event for both three- and four-byte code points.
assert.equal(analytics.STOREFRONT_ANALYTICS_BATCH_BYTE_BUDGET, 48 * 1024)
for (const character of ['界', '😀']) {
  const events = Array.from({ length: 20 }, (_, index) => unicodeEventPayload(index + 1, character))
  const rawBytes = analytics.storefrontAnalyticsBatchByteSize(events)
  assert.equal(rawBytes, utf8.encode(analytics.storefrontAnalyticsBatchBody(events)).byteLength)
  assert.ok(rawBytes > SERVER_BATCH_BODY_LIMIT)
  if (character === '😀') {
    assert.ok(events[0].page_path.length > Array.from(events[0].page_path).length)
  }
  const split = analytics.splitStorefrontAnalyticsBatches(events)
  assert.equal(split.oversized.length, 0)
  assert.ok(split.batches.length > 1)
  assertCompleteOrderedDelivery(split.batches.flat(), events.map((event) => event.event_id))
  for (const batch of split.batches) {
    assert.ok(batch.length <= analytics.STOREFRONT_ANALYTICS_BATCH_SIZE)
    assert.ok(analytics.storefrontAnalyticsBatchByteSize(batch) <= analytics.STOREFRONT_ANALYTICS_BATCH_BYTE_BUDGET)
  }
}

// 3. Unexpected over-budget events are dropped without blocking a later
// event, and diagnostics count them without persisting their payloads.
analytics.resetStorefrontAnalyticsForTests()
fetchCalls.length = 0
fetchHandler = () => Promise.resolve({ ok: true, status: 202 })
analytics.trackStorefrontEvent({
  ...unicodeEventInput(100, '界'),
  page_path: `/${'x'.repeat(analytics.STOREFRONT_ANALYTICS_BATCH_BYTE_BUDGET)}`,
})
analytics.trackStorefrontEvent({
  ...unicodeEventInput(101, '😀'),
  page_path: `/${'😀'.repeat(analytics.STOREFRONT_ANALYTICS_BATCH_BYTE_BUDGET)}`,
})
analytics.trackStorefrontEvent({
  event_name: 'page_view',
  source_type: 'direct',
  source_id: 'after-oversized',
  page_type: 'other',
  page_path: '/zh/after-oversized',
})
const afterOversizedID = analytics.storefrontAnalyticsQueueForTests()[2].payload.event_id
await analytics.flushStorefrontAnalyticsQueue()
assert.equal(fetchCalls.length, 1)
assert.deepEqual(fetchCalls[0].payload.events.map((event) => event.event_id), [afterOversizedID])
assert.equal(analytics.storefrontAnalyticsDroppedCountForTests(), 2)
assert.equal(analytics.storefrontAnalyticsQueueForTests().length, 0)

// 4. Real normal delivery drains all 20 maximum three-byte events through
// multiple byte-safe HTTP requests without changing IDs or order.
analytics.resetStorefrontAnalyticsForTests()
fetchCalls.length = 0
fetchHandler = () => Promise.resolve({ ok: true, status: 202 })
for (let index = 0; index < 20; index += 1) {
  analytics.trackStorefrontEvent(unicodeEventInput(index + 200, '界'))
}
assert.ok(analytics.storefrontAnalyticsInFlightBatchForTests().length > 0)
assert.ok(analytics.storefrontAnalyticsInFlightBatchForTests().length < 20)
assert.ok(analytics.storefrontAnalyticsQueueForTests().length > 0)
const normalExpectedIDs = deliveryStatePayloads().map((event) => event.event_id)
assert.equal(normalExpectedIDs.length, 20)
await waitForNormalDelivery()
assert.ok(fetchCalls.length > 1)
assertByteSafeCalls(fetchCalls)
assertCompleteOrderedDelivery(fetchCalls.flatMap((call) => call.payload.events), normalExpectedIDs)

// 5. A failed byte-safe normal batch retries with identical JSON and IDs; an
// event queued while the first request is pending is not merged into retry.
analytics.resetStorefrontAnalyticsForTests()
fetchCalls.length = 0
const firstAttempt = deferred()
const retryAttempt = deferred()
fetchHandler = () => {
  if (fetchCalls.length === 1) return firstAttempt.promise
  if (fetchCalls.length === 2) return retryAttempt.promise
  return Promise.resolve({ ok: true, status: 202 })
}
for (let index = 0; index < 20; index += 1) {
  analytics.trackStorefrontEvent(unicodeEventInput(index + 300, '界'))
}
const firstAttemptBody = fetchCalls[0].init.body
const firstAttemptIDs = fetchCalls[0].payload.events.map((event) => event.event_id)
analytics.trackStorefrontEvent({
  event_name: 'page_view',
  source_type: 'direct',
  source_id: 'queued-during-retry',
  page_type: 'other',
  page_path: '/zh/retry-queue',
})
const queuedDuringRetryID = analytics.storefrontAnalyticsQueueForTests().at(-1).payload.event_id
firstAttempt.reject(new Error('network down'))
await analytics.flushStorefrontAnalyticsQueue()
const retryFlush = analytics.flushStorefrontAnalyticsQueue()
assert.equal(fetchCalls.length, 2)
assert.equal(fetchCalls[1].init.body, firstAttemptBody)
assert.deepEqual(fetchCalls[1].payload.events.map((event) => event.event_id), firstAttemptIDs)
assert.equal(fetchCalls[1].payload.events.some((event) => event.event_id === queuedDuringRetryID), false)
assertByteSafeCalls(fetchCalls.slice(0, 2))
retryAttempt.resolve({ ok: true, status: 202 })
await retryFlush
await waitForNormalDelivery()
assert.ok(fetchCalls.slice(2).some((call) => call.payload.events.some((event) => event.event_id === queuedDuringRetryID)))

// 6-7. Pagehide re-splits in-flight first plus queued events into byte-safe
// Beacon batches for both 20 and 25 unique events.
for (const eventCount of [20, 25]) {
  analytics.resetStorefrontAnalyticsForTests()
  fetchCalls.length = 0
  const pendingNormal = deferred()
  fetchHandler = () => pendingNormal.promise
  const beaconCalls = []
  navigator.sendBeacon = (url, body) => {
    beaconCalls.push({ url, body })
    return true
  }
  for (let index = 0; index < eventCount; index += 1) {
    analytics.trackStorefrontEvent(unicodeEventInput(index + 400 + eventCount, '界'))
  }
  assert.ok(analytics.storefrontAnalyticsInFlightBatchForTests().length > 0)
  assert.ok(analytics.storefrontAnalyticsQueueForTests().length > 0)
  const pagehideExpectedIDs = deliveryStatePayloads().map((event) => event.event_id)
  assert.equal(pagehideExpectedIDs.length, eventCount)
  listeners.get('pagehide')()
  const pagehideEvents = await beaconPayloads(beaconCalls)
  assert.ok(beaconCalls.length > 1)
  assertCompleteOrderedDelivery(pagehideEvents, pagehideExpectedIDs)
  for (const call of beaconCalls) {
    assert.ok(call.body.size <= analytics.STOREFRONT_ANALYTICS_BATCH_BYTE_BUDGET)
    const payload = JSON.parse(await call.body.text())
    assert.ok(payload.events.length <= analytics.STOREFRONT_ANALYTICS_BATCH_SIZE)
    assert.equal(call.body.size, analytics.storefrontAnalyticsBatchByteSize(payload.events))
  }
  pendingNormal.resolve({ ok: true, status: 202 })
  await analytics.flushStorefrontAnalyticsQueue()
}

// 8. Beacon rejection falls back once per Unicode batch to independent,
// byte-safe keepalive requests containing all original IDs.
analytics.resetStorefrontAnalyticsForTests()
fetchCalls.length = 0
const pendingKeepaliveNormal = deferred()
fetchHandler = (_url, init) => init.keepalive
  ? Promise.resolve({ ok: true, status: 202 })
  : pendingKeepaliveNormal.promise
navigator.sendBeacon = () => false
for (let index = 0; index < 20; index += 1) {
  analytics.trackStorefrontEvent(unicodeEventInput(index + 500, '😀'))
}
const keepaliveExpectedIDs = deliveryStatePayloads().map((event) => event.event_id)
listeners.get('pagehide')()
const keepaliveCalls = fetchCalls.filter((call) => call.init.keepalive)
assert.ok(keepaliveCalls.length > 1)
assert.ok(keepaliveCalls.every((call) => call.init.keepalive === true))
assertByteSafeCalls(keepaliveCalls)
assertCompleteOrderedDelivery(keepaliveCalls.flatMap((call) => call.payload.events), keepaliveExpectedIDs)
pendingKeepaliveNormal.resolve({ ok: true, status: 202 })
await analytics.flushStorefrontAnalyticsQueue()

analytics.resetStorefrontAnalyticsForTests()
console.log('storefront analytics byte-safe batching tests passed')
