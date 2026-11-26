import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import {
  newReservationID,
  nextReservationDialogState,
  RESERVATION_COUNTRIES,
  reservationHeatTranslationKey,
  reservationWindowLabel,
} from '../src/utils/reservationUi.ts'

const ids = ['123e4567-e89b-42d3-a456-426614174000']
assert.equal(newReservationID(() => ids[0]), ids[0])
assert.equal(RESERVATION_COUNTRIES[0].code, 'GH')
assert.ok(RESERVATION_COUNTRIES.some((country) => country.code === 'CN'))
assert.equal(reservationHeatTranslationKey('highly_requested'), 'reservations.heat.highly_requested')
assert.match(reservationWindowLabel('2026-08-01T12:00:00Z', '2026-08-10T12:00:00Z', 'en', 'open'), /2026/)
assert.equal(reservationWindowLabel(null, null, 'zh', '正在开放'), '正在开放')

let dialogState = nextReservationDialogState('contact', 'request')
assert.equal(dialogState, 'challenge')
dialogState = nextReservationDialogState(dialogState, 'requested')
assert.equal(dialogState, 'code')
dialogState = nextReservationDialogState(dialogState, 'verified')
assert.equal(dialogState, 'success')
assert.equal(nextReservationDialogState('code', 'fail'), 'error')
assert.equal(nextReservationDialogState('error', 'reset'), 'contact')

const [router, header, list, detail, dialog, anonymousDialog, mine, config, api, en, zh] = await Promise.all([
  readFile(new URL('../src/router/index.ts', import.meta.url), 'utf8'),
  readFile(new URL('../src/components/storefront/StorefrontHeader.vue', import.meta.url), 'utf8'),
  readFile(new URL('../src/views/reservations/ReservationProductList.vue', import.meta.url), 'utf8'),
  readFile(new URL('../src/views/reservations/ReservationProductDetail.vue', import.meta.url), 'utf8'),
  readFile(new URL('../src/components/storefront/ReservationVerificationDialog.vue', import.meta.url), 'utf8'),
  readFile(new URL('../src/components/storefront/AnonymousReservationDialog.vue', import.meta.url), 'utf8'),
  readFile(new URL('../src/views/reservations/MyAnonymousReservations.vue', import.meta.url), 'utf8'),
  readFile(new URL('../src/config/storefront.ts', import.meta.url), 'utf8'),
  readFile(new URL('../src/api/reservations.ts', import.meta.url), 'utf8'),
  readFile(new URL('../src/locales/en.ts', import.meta.url), 'utf8'),
  readFile(new URL('../src/locales/zh.ts', import.meta.url), 'utf8'),
])

assert.match(router, /path: '\/:locale\(en\|zh\)\/reservations'/)
assert.match(router, /path: '\/:locale\(en\|zh\)\/reservations\/:slug'/)
assert.match(router, /path: '\/:locale\(en\|zh\)\/reservations\/mine'/)
assert.ok((header.match(/localizedPath\('\/reservations'\)/g) || []).length >= 2, 'desktop and mobile navigation must include reservations')
assert.match(header, /v-if="!reservationMode"[^>]+localizedPath\('\/cart'\)/)
assert.match(header, /StorefrontFloatingBar v-if="!reservationMode"/)

assert.match(list, /getReservationProducts/)
assert.match(list, /page_size: PAGE_SIZE/)
assert.match(list, /getStorefrontVisitorID\(\)/)
assert.match(list, /getStorefrontSession\(\)/)
assert.match(list, /createReservationInterest/)
assert.match(list, /source_type: 'reservation_list'/)
assert.match(list, /loading="lazy"/)
assert.match(list, /reservation-loading/)
assert.match(list, /reservation-error/)
assert.match(list, /reservation-empty/)
assert.match(list, /product\.viewer_state\.interested = true/)
assert.match(list, /generation === interestGeneration/)

assert.match(detail, /sanitizeProductDetailHtml/)
assert.match(detail, /source_type: 'reservation_detail'/)
assert.match(detail, /ReservationVerificationDialog/)
assert.match(detail, /viewer_state\.verified_reserved = true/)
assert.match(detail, /sequence === interestSequence && product\.value === target/)
assert.doesNotMatch(`${list}\n${detail}`, /storefrontPriceVisible|useCartStore|addToCart|checkout|createOrder/)

assert.match(dialog, /phoneCountry = ref\('GH'\)/)
assert.match(dialog, /getReservationCapabilities/)
assert.match(dialog, /availableChannels\.value\.includes/)
assert.match(dialog, /requestReservationOTP/)
assert.match(dialog, /verifyReservationOTP/)
assert.match(dialog, /autocomplete="one-time-code"/)
assert.match(dialog, /sessionStorage\.setItem\(reservationManageTokenKey/)
assert.match(dialog, /requestInFlight/)
assert.match(dialog, /verifyInFlight/)
assert.match(dialog, /operationGuard\.isCurrent/)
assert.doesNotMatch(dialog, /console\.(log|info|debug)|router\.(push|replace)|URLSearchParams/)
assert.doesNotMatch(dialog, /sessionStorage\.setItem\([^\n]*phoneNumber/)
assert.match(config, /VITE_RESERVATION_MODE/)
assert.match(config, /'disabled' \| 'anonymous' \| 'otp'/)
assert.match(detail, /v-if="otpReservationEnabled"/)
assert.match(detail, /v-if="anonymousReservationEnabled"/)
assert.match(anonymousDialog, /action="reservation_create"/)
assert.match(anonymousDialog, /createAnonymousReservation/)
assert.match(api, /request\.post<ApiResponse<AnonymousReservation>>\('\/storefront\/reservations'/)
assert.match(api, /reservation_product_id: productID/)
assert.match(anonymousDialog, /RESERVATION_ALREADY_EXISTS/)
assert.match(anonymousDialog, /BROWSER_RATE_LIMITED/)
assert.doesNotMatch(anonymousDialog, /phone_number|requestReservationOTP|verifyReservationOTP|WhatsApp|SMS/)
assert.match(mine, /getMyAnonymousReservations/)
assert.match(mine, /cancelAnonymousReservation/)
assert.match(mine, /item\.status === 'pending'/)

const publicType = api.match(/export interface ReservationProductPublic \{([\s\S]*?)\n\}/)?.[1] || ''
assert.ok(publicType)
assert.doesNotMatch(publicType, /linked_product_id|exact_|contact_hash|contact_encrypted|ip_risk|provider_reference/)
assert.match(en, /Product Reservations/)
assert.match(en, /Highly requested/)
assert.match(zh, /新品预约/)
assert.match(zh, /需求很高/)

console.log('reservation storefront behavior tests passed')
