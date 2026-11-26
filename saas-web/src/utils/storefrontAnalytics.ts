export const STOREFRONT_VISITOR_KEY = 'gc_visitor_id'
export const STOREFRONT_SESSION_KEY = 'gc_session'
export const STOREFRONT_SESSION_TIMEOUT_MS = 30 * 60 * 1000
export const STOREFRONT_ANALYTICS_BATCH_SIZE = 20
export const STOREFRONT_ANALYTICS_BATCH_BYTE_BUDGET = 48 * 1024
export const STOREFRONT_ANALYTICS_FLUSH_INTERVAL_MS = 2_000
export const STOREFRONT_ANALYTICS_MAX_QUEUE_SIZE = 500
export const STOREFRONT_ANALYTICS_MAX_RETRIES = 2

export const STOREFRONT_EVENT_NAMES = [
  'page_view',
  'product_impression',
  'product_click',
  'product_view',
  'content_impression',
  'content_click',
  'whatsapp_click',
  'phone_click',
] as const

export type StorefrontEventName = (typeof STOREFRONT_EVENT_NAMES)[number]
export type StorefrontEntityType = 'product' | 'marketing_content' | 'shop' | 'storefront'
export type StorefrontSourceType =
  | 'home_product_floor'
  | 'mall'
  | 'category'
  | 'search'
  | 'shop'
  | 'product_detail'
  | 'marketing_content'
  | 'direct'
export type StorefrontPageType = 'home' | 'mall' | 'product' | 'shop' | 'search' | 'other'
export type StorefrontDeviceType = 'desktop' | 'mobile' | 'tablet' | 'unknown'

export interface StorefrontAnalyticsSource {
  type: StorefrontSourceType
  id: string
}

export interface StorefrontEventInput {
  event_name: StorefrontEventName
  entity_type?: StorefrontEntityType
  entity_id?: string
  product_id?: number
  source_type?: StorefrontSourceType
  source_id?: string
  page_type?: StorefrontPageType
  page_path?: string
  locale?: 'en' | 'zh'
  referrer?: string
  device_type?: StorefrontDeviceType
}

export interface StorefrontEventPayload extends StorefrontEventInput {
  event_id: string
  visitor_id: string
  session_id: string
  source_type: StorefrontSourceType
  page_type: StorefrontPageType
  page_path: string
  locale: 'en' | 'zh'
  referrer: string
  device_type: StorefrontDeviceType
}

interface StorefrontSessionState {
  id: string
  last_activity_at: number
}

interface QueuedStorefrontEvent {
  payload: StorefrontEventPayload
  attempts: number
  retryBatchID?: number
}

const allowedEventNames = new Set<string>(STOREFRONT_EVENT_NAMES)
const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
const impressionKeys = new Set<string>()
const storefrontEventQueue: QueuedStorefrontEvent[] = []
const storefrontAnalyticsTextEncoder = new TextEncoder()

let memoryVisitorID = ''
let memorySession: StorefrontSessionState | null = null
let pageLifecycleID = 'initial'
let lastNavigationKey = ''
let deliveryInitialized = false
let deliveryGeneration = 0
let flushTimer: ReturnType<typeof setInterval> | null = null
let normalFlushPromise: Promise<void> | null = null
let inFlightBatch: QueuedStorefrontEvent[] | null = null
let pagehideFlushed = false
let retryBatchSequence = 0
let drainByteSafeBatches = false
let droppedEventCount = 0

function analyticsStorage(): Storage | null {
  try {
    if (typeof window === 'undefined' || !window.localStorage) {
      return null
    }
    return window.localStorage
  } catch {
    return null
  }
}

function readStorage(key: string) {
  try {
    return analyticsStorage()?.getItem(key) ?? null
  } catch {
    return null
  }
}

function writeStorage(key: string, value: string) {
  try {
    analyticsStorage()?.setItem(key, value)
  } catch {
    // Privacy mode and quota failures use the in-memory values above.
  }
}

function createAnonymousUUID() {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  const bytes = new Uint8Array(16)
  if (typeof crypto !== 'undefined' && typeof crypto.getRandomValues === 'function') {
    crypto.getRandomValues(bytes)
  } else {
    for (let index = 0; index < bytes.length; index += 1) {
      bytes[index] = Math.floor(Math.random() * 256)
    }
  }
  bytes[6] = (bytes[6] & 0x0f) | 0x40
  bytes[8] = (bytes[8] & 0x3f) | 0x80
  const hex = Array.from(bytes, (value) => value.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

export function getStorefrontVisitorID() {
  if (memoryVisitorID) {
    return memoryVisitorID
  }
  const stored = readStorage(STOREFRONT_VISITOR_KEY)?.trim() || ''
  memoryVisitorID = uuidPattern.test(stored) ? stored.toLowerCase() : createAnonymousUUID()
  writeStorage(STOREFRONT_VISITOR_KEY, memoryVisitorID)
  return memoryVisitorID
}

function parseStoredSession(value: string | null): StorefrontSessionState | null {
  if (!value) {
    return null
  }
  try {
    const parsed = JSON.parse(value) as Partial<StorefrontSessionState>
    if (!uuidPattern.test(parsed.id || '') || !Number.isFinite(parsed.last_activity_at)) {
      return null
    }
    return { id: String(parsed.id).toLowerCase(), last_activity_at: Number(parsed.last_activity_at) }
  } catch {
    return null
  }
}

export function getStorefrontSession(now = Date.now()): StorefrontSessionState {
  const stored = parseStoredSession(readStorage(STOREFRONT_SESSION_KEY))
  const current = stored && memorySession
    ? stored.last_activity_at >= memorySession.last_activity_at ? stored : memorySession
    : stored || memorySession
  const expired = !current || now < current.last_activity_at || now - current.last_activity_at >= STOREFRONT_SESSION_TIMEOUT_MS
  const session = expired ? { id: createAnonymousUUID(), last_activity_at: now } : { ...current, last_activity_at: now }
  memorySession = session
  writeStorage(STOREFRONT_SESSION_KEY, JSON.stringify(session))
  return session
}

function storefrontLocale(): 'en' | 'zh' {
  if (typeof window === 'undefined') {
    return 'en'
  }
  return window.location.pathname.split('/')[1]?.toLowerCase() === 'zh' ? 'zh' : 'en'
}

function storefrontPagePath() {
  if (typeof window === 'undefined') {
    return '/'
  }
  return window.location.pathname || '/'
}

function storefrontReferrer() {
  if (typeof document === 'undefined' || !document.referrer) {
    return ''
  }
  try {
    const referrer = new URL(document.referrer)
    referrer.search = ''
    referrer.hash = ''
    const value = referrer.toString()
    return value.length <= 1024 ? value : ''
  } catch {
    return ''
  }
}

export function storefrontDeviceType(): StorefrontDeviceType {
  if (typeof navigator === 'undefined') {
    return 'unknown'
  }
  const userAgent = navigator.userAgent.toLowerCase()
  if (/ipad|tablet|playbook|silk/.test(userAgent) || (/android/.test(userAgent) && !/mobile/.test(userAgent))) {
    return 'tablet'
  }
  if (/mobile|iphone|ipod|android/.test(userAgent)) {
    return 'mobile'
  }
  return userAgent ? 'desktop' : 'unknown'
}

function analyticsBatchEndpoint() {
  const baseURL = String(import.meta.env?.VITE_API_BASE_URL || '/api/v1').replace(/\/$/, '')
  return `${baseURL}/storefront/events/batch`
}

export function storefrontAnalyticsBatchBody(events: readonly StorefrontEventPayload[]) {
  return JSON.stringify({ events })
}

export function storefrontAnalyticsBatchByteSize(events: readonly StorefrontEventPayload[]) {
  return storefrontAnalyticsTextEncoder.encode(storefrontAnalyticsBatchBody(events)).byteLength
}

interface StorefrontAnalyticsBatchSplit<T> {
  batches: T[][]
  oversized: T[]
}

function splitStorefrontAnalyticsItems<T>(
  items: readonly T[],
  payloadFor: (item: T) => StorefrontEventPayload,
): StorefrontAnalyticsBatchSplit<T> {
  const batches: T[][] = []
  const oversized: T[] = []
  let currentBatch: T[] = []

  for (const item of items) {
    const payload = payloadFor(item)
    // The current server field limits keep one normalized event far below
    // 48 KiB. This guard prevents unexpected caller input from stalling the
    // queue if that contract is ever bypassed or expanded.
    if (storefrontAnalyticsBatchByteSize([payload]) > STOREFRONT_ANALYTICS_BATCH_BYTE_BUDGET) {
      oversized.push(item)
      continue
    }

    const candidate = [...currentBatch, item]
    const candidatePayloads = candidate.map(payloadFor)
    if (candidate.length > STOREFRONT_ANALYTICS_BATCH_SIZE ||
      storefrontAnalyticsBatchByteSize(candidatePayloads) > STOREFRONT_ANALYTICS_BATCH_BYTE_BUDGET) {
      if (currentBatch.length > 0) {
        batches.push(currentBatch)
      }
      currentBatch = [item]
      continue
    }
    currentBatch = candidate
  }

  if (currentBatch.length > 0) {
    batches.push(currentBatch)
  }
  return { batches, oversized }
}

export function splitStorefrontAnalyticsBatches(events: readonly StorefrontEventPayload[]) {
  return splitStorefrontAnalyticsItems(events, (event) => event)
}

async function sendStorefrontBatch(events: readonly StorefrontEventPayload[], keepalive = false) {
  const response = await fetch(analyticsBatchEndpoint(), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: storefrontAnalyticsBatchBody(events),
    credentials: 'omit',
    keepalive,
    referrerPolicy: 'no-referrer',
  })
  if (!response.ok) {
    throw new Error(`analytics batch rejected (${response.status})`)
  }
}

function warnTrackingFailure(error: unknown) {
  if (import.meta.env?.DEV) {
    console.warn('[storefront analytics] event failed', error)
  }
}

function recordOversizedStorefrontAnalyticsEvents(count: number) {
  if (count === 0) {
    return
  }
  droppedEventCount += count
  if (import.meta.env?.DEV) {
    console.warn('[storefront analytics] oversized events dropped', count)
  }
}

export function trackStorefrontEvent(input: StorefrontEventInput): void {
  try {
    if (!allowedEventNames.has(input.event_name)) {
      if (import.meta.env?.DEV) {
        console.warn('[storefront analytics] unsupported event', input.event_name)
      }
      return
    }
    const session = getStorefrontSession()
    const payload: StorefrontEventPayload = {
      ...input,
      event_id: createAnonymousUUID(),
      visitor_id: getStorefrontVisitorID(),
      session_id: session.id,
      source_type: input.source_type || 'direct',
      page_type: input.page_type || 'other',
      page_path: input.page_path || storefrontPagePath(),
      locale: input.locale || storefrontLocale(),
      referrer: input.referrer ?? storefrontReferrer(),
      device_type: input.device_type || storefrontDeviceType(),
    }
    initializeStorefrontAnalyticsDelivery()
    enqueueStorefrontEvent(payload)
  } catch (error) {
    warnTrackingFailure(error)
  }
}

function initializeStorefrontAnalyticsDelivery() {
  if (deliveryInitialized) {
    return
  }
  deliveryInitialized = true
  flushTimer = setInterval(() => {
    void flushStorefrontAnalyticsQueue()
  }, STOREFRONT_ANALYTICS_FLUSH_INTERVAL_MS)
  const timerWithUnref = flushTimer as unknown as { unref?: () => void }
  timerWithUnref.unref?.()
  if (typeof window !== 'undefined' && typeof window.addEventListener === 'function') {
    window.addEventListener('pagehide', flushStorefrontAnalyticsOnPageHide)
    window.addEventListener('pageshow', resetStorefrontAnalyticsPageHideFlush)
  }
}

function enqueueStorefrontEvent(payload: StorefrontEventPayload) {
  storefrontEventQueue.push({ payload, attempts: 0 })
  trimStorefrontEventQueue()
  if (storefrontEventQueue.length >= STOREFRONT_ANALYTICS_BATCH_SIZE) {
    void flushStorefrontAnalyticsQueue()
  }
}

function eventDropPriority(event: QueuedStorefrontEvent) {
  if (event.payload.event_name === 'product_impression' || event.payload.event_name === 'content_impression') {
    return 0
  }
  if (event.payload.event_name === 'page_view') {
    return 1
  }
  return 2
}

function trimStorefrontEventQueue() {
  while (storefrontEventQueue.length > STOREFRONT_ANALYTICS_MAX_QUEUE_SIZE) {
    let dropIndex = storefrontEventQueue.findIndex((event) => eventDropPriority(event) === 0)
    if (dropIndex < 0) {
      dropIndex = storefrontEventQueue.findIndex((event) => eventDropPriority(event) === 1)
    }
    if (dropIndex < 0) {
      dropIndex = 0
    }
    storefrontEventQueue.splice(dropIndex, 1)
    droppedEventCount += 1
  }
  if (droppedEventCount > 0 && import.meta.env?.DEV) {
    console.warn('[storefront analytics] queue dropped events', droppedEventCount)
  }
}

function requeueFailedBatch(batch: QueuedStorefrontEvent[], generation: number) {
  if (generation !== deliveryGeneration) {
    return
  }
  const sharedRetryBatchID = batch.length > 0 && batch.every((event) =>
    event.retryBatchID !== undefined && event.retryBatchID === batch[0].retryBatchID)
    ? batch[0].retryBatchID
    : undefined
  const retryBatchID = sharedRetryBatchID ?? ++retryBatchSequence
  const retryable = batch
    .map((event) => ({ ...event, attempts: event.attempts + 1, retryBatchID }))
    .filter((event) => event.attempts <= STOREFRONT_ANALYTICS_MAX_RETRIES)
  const dropped = batch.length - retryable.length
  if (dropped > 0) {
    droppedEventCount += dropped
  }
  storefrontEventQueue.unshift(...retryable)
  trimStorefrontEventQueue()
}

function removeStorefrontEventsFromQueue(events: readonly QueuedStorefrontEvent[]) {
  const removed = new Set(events)
  for (let index = storefrontEventQueue.length - 1; index >= 0; index -= 1) {
    if (removed.has(storefrontEventQueue[index])) {
      storefrontEventQueue.splice(index, 1)
    }
  }
}

function takeNextStorefrontAnalyticsBatch(): QueuedStorefrontEvent[] {
  while (storefrontEventQueue.length > 0) {
    const retryBatchID = storefrontEventQueue[0].retryBatchID
    const retryBoundary = retryBatchID === undefined
      ? -1
      : storefrontEventQueue.findIndex((event) => event.retryBatchID !== retryBatchID)
    const candidates = retryBatchID === undefined
      ? storefrontEventQueue
      : storefrontEventQueue.slice(0, retryBoundary < 0 ? storefrontEventQueue.length : retryBoundary)
    const split = splitStorefrontAnalyticsItems(candidates, (event) => event.payload)
    recordOversizedStorefrontAnalyticsEvents(split.oversized.length)
    removeStorefrontEventsFromQueue(split.oversized)
    if (split.batches.length === 0) {
      continue
    }
    if (split.batches.length > 1) {
      drainByteSafeBatches = true
    }
    const batch = split.batches[0]
    removeStorefrontEventsFromQueue(batch)
    return batch
  }
  drainByteSafeBatches = false
  return []
}

async function runNormalFlush(generation: number) {
  const batch = takeNextStorefrontAnalyticsBatch()
  if (batch.length === 0) {
    return false
  }
  inFlightBatch = batch
  try {
    await sendStorefrontBatch(batch.map((event) => event.payload))
    return true
  } catch (error) {
    requeueFailedBatch(batch, generation)
    warnTrackingFailure(error)
    return false
  } finally {
    if (inFlightBatch === batch) {
      inFlightBatch = null
    }
  }
}

export function flushStorefrontAnalyticsQueue(): Promise<void> {
  if (normalFlushPromise) {
    return normalFlushPromise
  }
  const generation = deliveryGeneration
  let promise: Promise<void>
  promise = runNormalFlush(generation).then((success) => {
    if (normalFlushPromise === promise) {
      normalFlushPromise = null
    }
    if (storefrontEventQueue.length === 0) {
      drainByteSafeBatches = false
    }
    if (success && generation === deliveryGeneration && storefrontEventQueue.length > 0 &&
      (drainByteSafeBatches || storefrontEventQueue.length >= STOREFRONT_ANALYTICS_BATCH_SIZE)) {
      queueMicrotask(() => {
        void flushStorefrontAnalyticsQueue()
      })
    }
  })
  normalFlushPromise = promise
  return promise
}

function beaconBatch(batch: QueuedStorefrontEvent[]) {
  const events = batch.map((event) => event.payload)
  const body = storefrontAnalyticsBatchBody(events)
  let accepted = false
  try {
    if (typeof navigator !== 'undefined' && typeof navigator.sendBeacon === 'function') {
      accepted = navigator.sendBeacon(analyticsBatchEndpoint(), new Blob([body], { type: 'application/json' }))
    }
  } catch {
    accepted = false
  }
  if (!accepted) {
    const generation = deliveryGeneration
    void sendStorefrontBatch(events, true).catch((error) => {
      requeueFailedBatch(batch, generation)
      warnTrackingFailure(error)
    })
  }
}

export function flushStorefrontAnalyticsOnPageHide() {
  if (pagehideFlushed) {
    return
  }
  pagehideFlushed = true
  const queuedSnapshot = storefrontEventQueue.splice(0, storefrontEventQueue.length)
  const pending = [...(inFlightBatch || []), ...queuedSnapshot]
  const seenEventIDs = new Set<string>()
  const snapshot = pending.filter((event) => {
    if (seenEventIDs.has(event.payload.event_id)) {
      return false
    }
    seenEventIDs.add(event.payload.event_id)
    return true
  })
  const split = splitStorefrontAnalyticsItems(snapshot, (event) => event.payload)
  recordOversizedStorefrontAnalyticsEvents(split.oversized.length)
  for (const batch of split.batches) {
    beaconBatch(batch)
  }
}

function resetStorefrontAnalyticsPageHideFlush() {
  pagehideFlushed = false
}

export function storefrontAnalyticsQueueForTests() {
  return storefrontEventQueue.map((event) => ({ payload: { ...event.payload }, attempts: event.attempts }))
}

export function storefrontAnalyticsInFlightBatchForTests() {
  return (inFlightBatch || []).map((event) => ({ payload: { ...event.payload }, attempts: event.attempts }))
}

export function storefrontAnalyticsDroppedCountForTests() {
  return droppedEventCount
}

export function disposeStorefrontAnalyticsDeliveryForTests() {
  const wasInitialized = deliveryInitialized
  deliveryGeneration += 1
  if (flushTimer !== null) {
    clearInterval(flushTimer)
    flushTimer = null
  }
  if (wasInitialized && typeof window !== 'undefined' && typeof window.removeEventListener === 'function') {
    window.removeEventListener('pagehide', flushStorefrontAnalyticsOnPageHide)
    window.removeEventListener('pageshow', resetStorefrontAnalyticsPageHideFlush)
  }
  deliveryInitialized = false
  normalFlushPromise = null
  inFlightBatch = null
  pagehideFlushed = false
  retryBatchSequence = 0
  drainByteSafeBatches = false
}

export function beginStorefrontPageLifecycle(navigationKey: string) {
  if (!navigationKey || navigationKey === lastNavigationKey) {
    return false
  }
  lastNavigationKey = navigationKey
  pageLifecycleID = createAnonymousUUID()
  impressionKeys.clear()
  return true
}

export function currentStorefrontPageLifecycleID() {
  return pageLifecycleID
}

export function trackStorefrontPageView(navigationKey: string, input: Omit<StorefrontEventInput, 'event_name'>) {
  if (!beginStorefrontPageLifecycle(navigationKey)) {
    return
  }
  trackStorefrontEvent({ ...input, event_name: 'page_view' })
}

export function trackStorefrontImpression(input: StorefrontEventInput) {
  if (input.event_name !== 'product_impression' && input.event_name !== 'content_impression') {
    return
  }
  const session = getStorefrontSession()
  const entityKey = input.product_id ? `product:${input.product_id}` : `${input.entity_type || ''}:${input.entity_id || ''}`
  const sourceKey = `${input.source_type || 'direct'}:${input.source_id || ''}`
  const dedupeKey = `${session.id}:${pageLifecycleID}:${input.event_name}:${entityKey}:${sourceKey}`
  if (impressionKeys.has(dedupeKey)) {
    return
  }
  impressionKeys.add(dedupeKey)
  trackStorefrontEvent(input)
}

export function resetStorefrontAnalyticsForTests() {
  disposeStorefrontAnalyticsDeliveryForTests()
  memoryVisitorID = ''
  memorySession = null
  pageLifecycleID = 'initial'
  lastNavigationKey = ''
  impressionKeys.clear()
  storefrontEventQueue.splice(0, storefrontEventQueue.length)
  droppedEventCount = 0
}

if (import.meta.hot) {
  import.meta.hot.dispose(() => {
    disposeStorefrontAnalyticsDeliveryForTests()
  })
}
