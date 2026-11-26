import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

class FakeScript extends EventTarget {
  src = ''
  async = false
  defer = false
  dataset = {}
  remove() {
    if (existingScript === this) existingScript = null
  }
}

const appended = []
let existingScript = null
const fakeDocument = {
  querySelector() { return existingScript },
  createElement(tag) {
    assert.equal(tag, 'script')
    return new FakeScript()
  },
  head: {
    appendChild(script) {
      appended.push(script)
      existingScript = script
    },
  },
}
const fakeWindow = {}
globalThis.window = fakeWindow
globalThis.document = fakeDocument

const {
  loadTurnstileAPI,
  reservationTurnstileOptions,
  resetTurnstileLoaderForTests,
  TURNSTILE_SCRIPT_URL,
} = await import('../src/utils/turnstile.ts')

resetTurnstileLoaderForTests()
const firstLoad = loadTurnstileAPI()
const secondLoad = loadTurnstileAPI()
assert.strictEqual(firstLoad, secondLoad, 'Turnstile loader must be a global singleton')
assert.equal(appended.length, 1)
assert.equal(appended[0].src, TURNSTILE_SCRIPT_URL)
assert.equal(appended[0].dataset.gcTurnstile, 'true')

let renderedOptions
let resetCount = 0
let removeCount = 0
fakeWindow.turnstile = {
  render(_container, options) { renderedOptions = options; return 'widget-1' },
  reset(widget) { assert.equal(widget, 'widget-1'); resetCount += 1 },
  remove(widget) { assert.equal(widget, 'widget-1'); removeCount += 1 },
}
appended[0].dispatchEvent(new Event('load'))
const api = await firstLoad
assert.strictEqual(api, fakeWindow.turnstile)

let token = ''
let expired = 0
let errors = 0
const options = reservationTurnstileOptions(' site-key ', {
  success(value) { token = value },
  expired() { token = ''; expired += 1 },
  error() { errors += 1 },
})
assert.deepEqual(
  { sitekey: options.sitekey, theme: options.theme, size: options.size, appearance: options.appearance, action: options.action },
  { sitekey: 'site-key', theme: 'auto', size: 'flexible', appearance: 'interaction-only', action: 'reservation_otp' },
)
const createOptions = reservationTurnstileOptions('site-key', {
  success() {},
  expired() {},
  error() {},
}, 'reservation_create')
assert.equal(createOptions.action, 'reservation_create')
const widgetID = api.render({}, options)
assert.equal(widgetID, 'widget-1')
renderedOptions.callback('opaque-test-token')
assert.equal(token, 'opaque-test-token')
renderedOptions['expired-callback']()
assert.equal(token, '')
assert.equal(expired, 1)
renderedOptions['error-callback']()
assert.equal(errors, 1)
api.reset(widgetID)
api.remove(widgetID)
assert.equal(resetCount, 1)
assert.equal(removeCount, 1)

// A failed script is removed and a later attempt can load a fresh singleton.
fakeWindow.turnstile = undefined
existingScript = null
resetTurnstileLoaderForTests()
const failedLoad = loadTurnstileAPI()
const failedScript = appended.at(-1)
failedScript.dispatchEvent(new Event('error'))
await assert.rejects(failedLoad, /failed to load/)
assert.equal(existingScript, null)
const retryLoad = loadTurnstileAPI()
const retryScript = appended.at(-1)
assert.notStrictEqual(retryScript, failedScript)
fakeWindow.turnstile = {
  render() { return 'widget-retry' },
  reset() {},
  remove() {},
}
retryScript.dispatchEvent(new Event('load'))
await retryLoad

const { createReservationOperationGuard } = await import('../src/utils/reservationUi.ts')
const operationGuard = createReservationOperationGuard()
const staleSequence = operationGuard.capture()
operationGuard.invalidate()
assert.equal(operationGuard.isCurrent(staleSequence), false, 'closed/replaced dialog responses must become stale')
assert.equal(operationGuard.isCurrent(operationGuard.capture()), true)

const [component, dialog] = await Promise.all([
  readFile(new URL('../src/components/storefront/CloudflareTurnstile.vue', import.meta.url), 'utf8'),
  readFile(new URL('../src/components/storefront/ReservationVerificationDialog.vue', import.meta.url), 'utf8'),
])
assert.match(component, /onUnmounted\(\(\) => \{[\s\S]*disposed = true[\s\S]*removeWidget\(\)/)
assert.match(component, /generation !== renderGeneration/)
assert.match(component, /Turnstile site key is not configured/)
assert.match(component, /emit\('token', ''\)/)
assert.doesNotMatch(component, /console\.(log|info|debug)/)
assert.match(dialog, /turnstileResetKey\.value \+= 1/g)
assert.match(dialog, /canResend/)
assert.match(dialog, /resendRemaining/)
assert.match(dialog, /requestInFlight/)
assert.match(dialog, /verifyInFlight/)
assert.match(dialog, /requestIdempotencyKey \|\|= newReservationID\(\)/)
assert.match(dialog, /verifyIdempotencyKey \|\|= newReservationID\(\)/)
assert.match(dialog, /operationGuard\.isCurrent/)
assert.doesNotMatch(dialog, /turnstile_token[^\n]+(?:localStorage|sessionStorage)/i)

console.log('Turnstile singleton and lifecycle tests passed')
