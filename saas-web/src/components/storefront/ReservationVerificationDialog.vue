<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  getReservationCapabilities,
  requestReservationOTP,
  verifyReservationOTP,
  type ReservationContactType,
} from '../../api/reservations'
import { parseStorefrontFeatureFlag, reservationEnabled } from '../../config/storefront'
import { getStorefrontVisitorID } from '../../utils/storefrontAnalytics'
import {
  newReservationID,
  nextReservationDialogState,
  createReservationOperationGuard,
  RESERVATION_COUNTRIES,
  reservationManageTokenKey,
  type ReservationDialogState,
} from '../../utils/reservationUi'
import CloudflareTurnstile from './CloudflareTurnstile.vue'

const props = defineProps<{
  open: boolean
  productId: number
  productTitle: string
}>()

const emit = defineEmits<{
  close: []
  verified: []
}>()

const { t } = useI18n()
const turnstileEnabled = parseStorefrontFeatureFlag(import.meta.env.VITE_TURNSTILE_ENABLED)
const state = ref<ReservationDialogState>('contact')
const availableChannels = ref<ReservationContactType[]>([])
const channel = ref<ReservationContactType>('sms')
const phoneCountry = ref('GH')
const phoneNumber = ref('')
const consented = ref(false)
const turnstileToken = ref('')
const turnstileResetKey = ref(0)
const challengeID = ref('')
const otpCode = ref('')
const errorMessage = ref('')
const resendRemaining = ref(0)
const loadingCapabilities = ref(false)
const requestInFlight = ref(false)
const verifyInFlight = ref(false)
const operationGuard = createReservationOperationGuard()
let requestIdempotencyKey = ''
let verifyIdempotencyKey = ''
let countdownTimer: ReturnType<typeof setInterval> | null = null

const canRequest = computed(() => (
  availableChannels.value.includes(channel.value)
  && phoneNumber.value.trim().length >= 4
  && consented.value
  && (!turnstileEnabled || Boolean(turnstileToken.value))
  && !requestInFlight.value
))
const canVerify = computed(() => challengeID.value.length > 0 && /^\d{4,10}$/.test(otpCode.value.trim()) && !verifyInFlight.value)
const canResend = computed(() => resendRemaining.value <= 0 && (!turnstileEnabled || Boolean(turnstileToken.value)) && !requestInFlight.value)

watch([() => props.open, () => props.productId], ([open]) => {
  resetDialog()
  if (open && reservationEnabled) {
    void loadCapabilities()
  }
}, { immediate: true })

watch([channel, phoneCountry, phoneNumber], () => {
  if (!requestInFlight.value && !challengeID.value) requestIdempotencyKey = ''
})

async function loadCapabilities() {
  if (!reservationEnabled) return
  const sequence = operationGuard.capture()
  loadingCapabilities.value = true
  try {
    const capabilities = await getReservationCapabilities()
    if (!operationGuard.isCurrent(sequence) || !props.open) return
    if (!capabilities.reservation_enabled) {
      availableChannels.value = []
      showError(t('reservations.verify.serviceUnavailable'))
      return
    }
    availableChannels.value = capabilities.otp_channels.filter((item): item is ReservationContactType => item === 'sms' || item === 'whatsapp')
    if (!availableChannels.value.includes(channel.value)) {
      channel.value = availableChannels.value[0] || 'sms'
    }
    if (!availableChannels.value.length) {
      showError(t('reservations.verify.noChannels'))
    }
  } catch {
    if (!operationGuard.isCurrent(sequence) || !props.open) return
    availableChannels.value = []
    showError(t('reservations.verify.serviceUnavailable'))
  } finally {
    if (operationGuard.isCurrent(sequence)) loadingCapabilities.value = false
  }
}

function resetDialog() {
  operationGuard.invalidate()
  state.value = nextReservationDialogState(state.value, 'reset')
  availableChannels.value = []
  channel.value = 'sms'
  phoneCountry.value = 'GH'
  phoneNumber.value = ''
  consented.value = false
  turnstileToken.value = ''
  turnstileResetKey.value += 1
  challengeID.value = ''
  otpCode.value = ''
  errorMessage.value = ''
  resendRemaining.value = 0
  requestInFlight.value = false
  verifyInFlight.value = false
  loadingCapabilities.value = false
  requestIdempotencyKey = ''
  verifyIdempotencyKey = ''
  stopCountdown()
}

function closeDialog() {
  resetDialog()
  emit('close')
}

function showError(message: string) {
  errorMessage.value = message
  state.value = nextReservationDialogState(state.value, 'fail')
}

function safeWorkflowError(error: unknown, fallbackKey: string) {
  const message = error instanceof Error ? error.message.toLowerCase() : ''
  if (message.includes('rate') || message.includes('retry') || message.includes('cooldown')) return t('reservations.verify.tooManyRequests')
  if (message.includes('human verification')) return t('reservations.verify.turnstileFailed')
  if (message.includes('phone') || message.includes('invalid input')) return t('reservations.verify.invalidPhone')
  if (message.includes('code') || message.includes('challenge')) return t('reservations.verify.invalidCode')
  return t(fallbackKey)
}

async function sendOTP() {
  if (!reservationEnabled) return
  if (requestInFlight.value) return
  if (!canRequest.value && !challengeID.value) return
  if (challengeID.value && !canResend.value) return
  const sequence = operationGuard.capture()
  const productID = props.productId
  const token = turnstileToken.value
  requestIdempotencyKey ||= newReservationID()
  const idempotencyKey = requestIdempotencyKey
  requestInFlight.value = true
  // A Turnstile token is single use. Clear it before the request so neither a
  // double click nor an ambiguous network failure can reuse it.
  turnstileToken.value = ''
  turnstileResetKey.value += 1
  state.value = nextReservationDialogState(state.value, 'request')
  errorMessage.value = ''
  try {
    const result = await requestReservationOTP(productID, {
      visitor_id: getStorefrontVisitorID(),
      idempotency_key: idempotencyKey,
      contact_type: channel.value,
      phone_country: phoneCountry.value,
      phone_number: phoneNumber.value.trim(),
      turnstile_token: token,
      source_type: 'reservation_dialog',
      source_id: String(productID),
    })
    if (!operationGuard.isCurrent(sequence) || !props.open || props.productId !== productID) return
    challengeID.value = result.challenge_id
    verifyIdempotencyKey = newReservationID()
    requestIdempotencyKey = ''
    otpCode.value = ''
    startCountdown(result.resend_after_seconds)
    state.value = nextReservationDialogState(state.value, 'requested')
  } catch (error) {
    if (!operationGuard.isCurrent(sequence) || !props.open || props.productId !== productID) return
    showError(safeWorkflowError(error, 'reservations.verify.requestFailed'))
  } finally {
    if (operationGuard.isCurrent(sequence)) requestInFlight.value = false
  }
}

async function verifyCode() {
  if (!reservationEnabled) return
  if (!canVerify.value || verifyInFlight.value) return
  const sequence = operationGuard.capture()
  const productID = props.productId
  const challenge = challengeID.value
  const code = otpCode.value.trim()
  verifyIdempotencyKey ||= newReservationID()
  const idempotencyKey = verifyIdempotencyKey
  verifyInFlight.value = true
  errorMessage.value = ''
  try {
    const result = await verifyReservationOTP(productID, {
      challenge_id: challenge,
      code,
      idempotency_key: idempotencyKey,
    })
    if (!operationGuard.isCurrent(sequence) || !props.open || props.productId !== productID || challengeID.value !== challenge) return
    if (result.manage_token) {
      try {
        sessionStorage.setItem(reservationManageTokenKey(productID), result.manage_token)
      } catch {
        // The reservation is valid even when private browsing blocks session storage.
      }
    }
    phoneNumber.value = ''
    otpCode.value = ''
    state.value = nextReservationDialogState(state.value, 'verified')
    emit('verified')
  } catch (error) {
    if (!operationGuard.isCurrent(sequence) || !props.open || props.productId !== productID || challengeID.value !== challenge) return
    showError(safeWorkflowError(error, 'reservations.verify.verifyFailed'))
  } finally {
    if (operationGuard.isCurrent(sequence)) verifyInFlight.value = false
  }
}

function recover() {
  errorMessage.value = ''
  state.value = challengeID.value ? 'code' : 'contact'
  turnstileToken.value = ''
  turnstileResetKey.value += 1
}

function startCountdown(seconds: number) {
  stopCountdown()
  resendRemaining.value = Math.max(0, Math.ceil(seconds))
  if (!resendRemaining.value) return
  countdownTimer = setInterval(() => {
    resendRemaining.value = Math.max(0, resendRemaining.value - 1)
    if (!resendRemaining.value) stopCountdown()
  }, 1000)
}

function stopCountdown() {
  if (countdownTimer) clearInterval(countdownTimer)
  countdownTimer = null
}

onUnmounted(() => {
  operationGuard.invalidate()
  stopCountdown()
})
</script>

<template>
  <Teleport v-if="reservationEnabled" to="body">
    <div v-if="open" class="fixed inset-0 z-[100] flex items-end justify-center bg-slate-950/55 p-0 sm:items-center sm:p-4" @click.self="closeDialog">
      <section role="dialog" aria-modal="true" :aria-label="t('reservations.verify.title')" class="max-h-[92vh] w-full overflow-y-auto rounded-t-3xl bg-white p-5 shadow-2xl sm:max-w-lg sm:rounded-3xl sm:p-7">
        <div class="flex items-start justify-between gap-4">
          <div>
            <p class="text-xs font-bold uppercase tracking-[0.22em] text-orange-500">{{ t('reservations.verify.eyebrow') }}</p>
            <h2 class="mt-1 text-2xl font-black text-slate-950">{{ t('reservations.verify.title') }}</h2>
            <p class="mt-1 text-sm text-slate-500">{{ productTitle }}</p>
          </div>
          <button type="button" class="rounded-full bg-slate-100 px-3 py-2 text-sm font-black text-slate-600" :aria-label="t('nav.close')" @click="closeDialog">✕</button>
        </div>

        <div v-if="state === 'contact'" class="mt-6 space-y-4">
          <p v-if="loadingCapabilities" class="text-sm text-slate-500">{{ t('reservations.loading') }}</p>
          <fieldset v-else class="space-y-2">
            <legend class="text-sm font-black text-slate-800">{{ t('reservations.verify.channel') }}</legend>
            <div class="grid grid-cols-2 gap-2">
              <button
                v-for="item in availableChannels"
                :key="item"
                type="button"
                class="rounded-xl border px-4 py-3 text-sm font-black"
                :class="channel === item ? 'border-orange-400 bg-orange-50 text-orange-700' : 'border-slate-200 text-slate-600'"
                @click="channel = item"
              >
                {{ item === 'sms' ? 'SMS' : 'WhatsApp' }}
              </button>
            </div>
          </fieldset>

          <div class="grid grid-cols-[9rem_minmax(0,1fr)] gap-2">
            <label class="block">
              <span class="mb-1 block text-sm font-black text-slate-800">{{ t('reservations.verify.country') }}</span>
              <select v-model="phoneCountry" class="w-full rounded-xl border border-slate-200 bg-white px-3 py-3 text-sm outline-none focus:border-orange-400">
                <option v-for="country in RESERVATION_COUNTRIES" :key="country.code" :value="country.code">
                  {{ country.code }} {{ country.callingCode }}
                </option>
              </select>
            </label>
            <label class="block">
              <span class="mb-1 block text-sm font-black text-slate-800">{{ t('reservations.verify.phone') }}</span>
              <input v-model="phoneNumber" type="tel" inputmode="tel" autocomplete="tel" class="w-full rounded-xl border border-slate-200 px-3 py-3 text-sm outline-none focus:border-orange-400" :placeholder="t('reservations.verify.phonePlaceholder')" />
            </label>
          </div>

          <label class="flex items-start gap-3 rounded-xl bg-slate-50 p-3 text-xs leading-5 text-slate-600">
            <input v-model="consented" type="checkbox" class="mt-1 h-4 w-4 accent-orange-500" />
            <span>{{ channel === 'whatsapp' ? t('reservations.verify.whatsappConsent') : t('reservations.verify.smsConsent') }}</span>
          </label>

          <CloudflareTurnstile :reset-key="turnstileResetKey" @token="turnstileToken = $event" @error="errorMessage = $event" />
          <p v-if="errorMessage" role="alert" class="text-sm font-semibold text-red-600">{{ errorMessage }}</p>
          <button type="button" class="w-full rounded-xl bg-orange-500 px-5 py-3.5 text-sm font-black text-white disabled:cursor-not-allowed disabled:opacity-40" data-testid="reservation-send-otp" :disabled="!canRequest || requestInFlight" @click="sendOTP">
            {{ t('reservations.verify.sendCode') }}
          </button>
        </div>

        <div v-else-if="state === 'challenge'" class="mt-8 py-10 text-center">
          <div class="mx-auto h-9 w-9 animate-spin rounded-full border-4 border-orange-100 border-t-orange-500" />
          <p class="mt-4 text-sm font-bold text-slate-600">{{ t('reservations.verify.sending') }}</p>
        </div>

        <div v-else-if="state === 'code'" class="mt-6 space-y-4">
          <p class="rounded-xl bg-blue-50 p-3 text-sm leading-6 text-blue-800">{{ t('reservations.verify.codeHint', { channel: channel === 'sms' ? 'SMS' : 'WhatsApp' }) }}</p>
          <label class="block">
            <span class="mb-1 block text-sm font-black text-slate-800">{{ t('reservations.verify.code') }}</span>
            <input v-model="otpCode" type="text" inputmode="numeric" autocomplete="one-time-code" maxlength="10" class="w-full rounded-xl border border-slate-200 px-4 py-3 text-center text-xl font-black tracking-[0.35em] outline-none focus:border-orange-400" @keyup.enter="verifyCode" />
          </label>
          <button type="button" class="w-full rounded-xl bg-orange-500 px-5 py-3.5 text-sm font-black text-white disabled:opacity-40" :disabled="!canVerify || verifyInFlight" @click="verifyCode">
            {{ t('reservations.verify.confirm') }}
          </button>
          <div class="border-t border-slate-100 pt-4">
            <CloudflareTurnstile :reset-key="turnstileResetKey" @token="turnstileToken = $event" @error="errorMessage = $event" />
            <button type="button" class="mt-3 w-full rounded-xl border border-slate-200 px-4 py-3 text-sm font-black text-slate-700 disabled:opacity-40" :disabled="!canResend || requestInFlight" @click="sendOTP">
              {{ resendRemaining > 0 ? t('reservations.verify.resendCountdown', { seconds: resendRemaining }) : t('reservations.verify.resend') }}
            </button>
          </div>
        </div>

        <div v-else-if="state === 'success'" class="mt-7 text-center">
          <div class="mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-emerald-100 text-3xl text-emerald-600">✓</div>
          <h3 class="mt-4 text-2xl font-black text-slate-950">{{ t('reservations.verify.success') }}</h3>
          <p class="mt-2 text-sm leading-6 text-slate-600">{{ t('reservations.verify.successHint') }}</p>
          <button type="button" class="mt-6 w-full rounded-xl bg-slate-950 px-5 py-3.5 text-sm font-black text-white" @click="closeDialog">{{ t('nav.close') }}</button>
        </div>

        <div v-else class="mt-7 text-center">
          <div class="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-red-100 text-2xl text-red-600">!</div>
          <h3 class="mt-4 text-xl font-black text-slate-950">{{ t('reservations.verify.errorTitle') }}</h3>
          <p role="alert" class="mt-2 text-sm leading-6 text-slate-600">{{ errorMessage }}</p>
          <button type="button" class="mt-5 w-full rounded-xl bg-slate-950 px-5 py-3 text-sm font-black text-white" @click="recover">{{ t('reservations.verify.tryAgain') }}</button>
        </div>

        <p class="mt-5 border-t border-slate-100 pt-4 text-center text-[11px] leading-5 text-slate-400">{{ t('reservations.verify.privacy') }}</p>
      </section>
    </div>
  </Teleport>
</template>
