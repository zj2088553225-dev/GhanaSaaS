<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  createAnonymousReservation,
  type AnonymousReservation,
} from '../../api/reservations'
import { anonymousReservationEnabled } from '../../config/storefront'
import { isAPIRequestError } from '../../utils/request'
import CloudflareTurnstile from './CloudflareTurnstile.vue'

const props = defineProps<{
  open: boolean
  productId: number
  productTitle: string
}>()

const emit = defineEmits<{
  close: []
  created: [reservation: AnonymousReservation]
}>()

const { t, locale } = useI18n()
const quantity = ref(1)
const note = ref('')
const turnstileToken = ref('')
const turnstileResetKey = ref(0)
const submitting = ref(false)
const errorMessage = ref('')
const reservation = ref<AnonymousReservation | null>(null)
const duplicate = ref(false)

const canSubmit = computed(() => (
  anonymousReservationEnabled
  && quantity.value >= 1
  && quantity.value <= 99
  && note.value.length <= 500
  && Boolean(turnstileToken.value)
  && !submitting.value
))

watch([() => props.open, () => props.productId], ([open]) => {
  if (open) reset()
}, { immediate: true })

function reset() {
  quantity.value = 1
  note.value = ''
  turnstileToken.value = ''
  turnstileResetKey.value += 1
  submitting.value = false
  errorMessage.value = ''
  reservation.value = null
  duplicate.value = false
}

function closeDialog() {
  reset()
  emit('close')
}

function formatDate(value?: string | null) {
  if (!value) return '—'
  return new Intl.DateTimeFormat(locale.value === 'zh' ? 'zh-CN' : 'en-GB', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value))
}

async function createReservation() {
  if (!canSubmit.value) return
  submitting.value = true
  errorMessage.value = ''
  duplicate.value = false
  const token = turnstileToken.value
  turnstileToken.value = ''
  turnstileResetKey.value += 1
  try {
    const result = await createAnonymousReservation(props.productId, {
      quantity: quantity.value,
      note: note.value.trim() || undefined,
      turnstile_token: token,
    })
    reservation.value = result
    emit('created', result)
  } catch (error) {
    if (isAPIRequestError<AnonymousReservation>(error) && error.code === 'RESERVATION_ALREADY_EXISTS' && error.data) {
      reservation.value = error.data
      duplicate.value = true
      return
    }
    if (isAPIRequestError(error) && (error.code === 'BROWSER_RATE_LIMITED' || error.code === 'IP_RATE_LIMITED')) {
      errorMessage.value = t('reservations.anonymous.rateLimited')
      return
    }
    if (isAPIRequestError(error) && error.code === 'TURNSTILE_FAILED') {
      errorMessage.value = t('reservations.anonymous.turnstileFailed')
      return
    }
    if (isAPIRequestError(error) && error.code === 'PRODUCT_NOT_RESERVABLE') {
      errorMessage.value = t('reservations.anonymous.productUnavailable')
      return
    }
    errorMessage.value = t('reservations.anonymous.createFailed')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <Teleport v-if="anonymousReservationEnabled" to="body">
    <div v-if="open" class="fixed inset-0 z-[100] flex items-end justify-center bg-slate-950/55 sm:items-center sm:p-4" @click.self="closeDialog">
      <section role="dialog" aria-modal="true" :aria-label="t('reservations.anonymous.title')" class="max-h-[92vh] w-full overflow-y-auto rounded-t-3xl bg-white p-5 shadow-2xl sm:max-w-lg sm:rounded-3xl sm:p-7">
        <div class="flex items-start justify-between gap-4">
          <div>
            <p class="text-xs font-bold uppercase tracking-[0.22em] text-orange-500">{{ t('reservations.anonymous.eyebrow') }}</p>
            <h2 class="mt-1 text-2xl font-black text-slate-950">{{ t('reservations.anonymous.title') }}</h2>
            <p class="mt-1 text-sm text-slate-500">{{ productTitle }}</p>
          </div>
          <button type="button" class="rounded-full bg-slate-100 px-3 py-2 text-sm font-black text-slate-600" :aria-label="t('nav.close')" @click="closeDialog">✕</button>
        </div>

        <div v-if="!reservation" class="mt-6 space-y-4" data-testid="anonymous-reservation-form">
          <label class="block">
            <span class="mb-1 block text-sm font-black text-slate-800">{{ t('reservations.anonymous.quantity') }}</span>
            <input v-model.number="quantity" type="number" min="1" max="99" inputmode="numeric" class="w-full rounded-xl border border-slate-200 px-3 py-3 text-sm outline-none focus:border-orange-400" />
          </label>
          <label class="block">
            <span class="mb-1 block text-sm font-black text-slate-800">{{ t('reservations.anonymous.note') }}</span>
            <textarea v-model="note" maxlength="500" rows="3" class="w-full resize-none rounded-xl border border-slate-200 px-3 py-3 text-sm outline-none focus:border-orange-400" :placeholder="t('reservations.anonymous.notePlaceholder')" />
            <span class="mt-1 block text-right text-xs text-slate-400">{{ note.length }}/500</span>
          </label>

          <CloudflareTurnstile action="reservation_create" :reset-key="turnstileResetKey" @token="turnstileToken = $event" @error="errorMessage = $event" />
          <p v-if="errorMessage" role="alert" class="rounded-xl bg-red-50 px-3 py-2 text-sm font-semibold text-red-700">{{ errorMessage }}</p>
          <button type="button" data-testid="anonymous-reservation-submit" class="w-full rounded-xl bg-orange-500 px-5 py-3.5 text-sm font-black text-white disabled:cursor-not-allowed disabled:opacity-40" :disabled="!canSubmit" @click="createReservation">
            {{ submitting ? t('reservations.anonymous.submitting') : t('reservations.anonymous.submit') }}
          </button>
          <p class="text-xs leading-5 text-slate-500">{{ t('reservations.anonymous.cookieNotice') }}</p>
        </div>

        <div v-else class="mt-7 text-center" data-testid="anonymous-reservation-success">
          <div class="mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-emerald-100 text-3xl text-emerald-600">✓</div>
          <h3 class="mt-4 text-2xl font-black text-slate-950">
            {{ duplicate ? t('reservations.anonymous.alreadyExists') : t('reservations.anonymous.success') }}
          </h3>
          <p class="mt-2 text-sm text-slate-600">{{ t('reservations.anonymous.codeLabel') }}</p>
          <p class="mt-2 break-all rounded-xl bg-slate-950 px-4 py-3 font-mono text-lg font-black tracking-wide text-white">{{ reservation.reservation_code }}</p>
          <p class="mt-3 text-sm text-slate-600">{{ t('reservations.anonymous.expiresAt', { time: formatDate(reservation.expires_at) }) }}</p>
          <p class="mt-3 rounded-xl bg-amber-50 p-3 text-xs leading-5 text-amber-800">{{ t('reservations.anonymous.saveCode') }}</p>
          <RouterLink :to="`/${locale}/reservations/mine`" class="mt-5 block w-full rounded-xl border border-slate-200 px-5 py-3.5 text-sm font-black text-slate-800" @click="closeDialog">
            {{ t('reservations.anonymous.myReservations') }}
          </RouterLink>
          <button type="button" class="mt-3 w-full rounded-xl bg-slate-950 px-5 py-3.5 text-sm font-black text-white" @click="closeDialog">{{ t('nav.close') }}</button>
        </div>
      </section>
    </div>
  </Teleport>
</template>
