<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import ProductImage from '../../components/ProductImage.vue'
import StorefrontHeader from '../../components/storefront/StorefrontHeader.vue'
import {
  cancelAnonymousReservation,
  getMyAnonymousReservations,
  type AnonymousReservation,
} from '../../api/reservations'
import { anonymousReservationEnabled } from '../../config/storefront'
import { currentLocale, withLocalePath } from '../../utils/localePath'

const route = useRoute()
const { t } = useI18n()
const locale = computed(() => currentLocale(route))
const reservations = ref<AnonymousReservation[]>([])
const loading = ref(true)
const errorMessage = ref('')
const cancellingCode = ref('')

watch(locale, () => void loadReservations(), { immediate: true })

async function loadReservations() {
  if (!anonymousReservationEnabled) {
    reservations.value = []
    loading.value = false
    return
  }
  loading.value = true
  errorMessage.value = ''
  try {
    reservations.value = await getMyAnonymousReservations()
  } catch {
    errorMessage.value = t('reservations.anonymous.mineLoadFailed')
  } finally {
    loading.value = false
  }
}

async function cancel(item: AnonymousReservation) {
  if (item.status !== 'pending' || cancellingCode.value) return
  cancellingCode.value = item.reservation_code
  errorMessage.value = ''
  try {
    const updated = await cancelAnonymousReservation(item.reservation_code)
    reservations.value = reservations.value.map((reservation) => (
      reservation.reservation_code === updated.reservation_code ? updated : reservation
    ))
  } catch {
    errorMessage.value = t('reservations.anonymous.cancelFailed')
  } finally {
    cancellingCode.value = ''
  }
}

function formatDate(value?: string | null) {
  if (!value) return '—'
  return new Intl.DateTimeFormat(locale.value === 'zh' ? 'zh-CN' : 'en-GB', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value))
}
</script>

<template>
  <main class="min-h-screen bg-slate-50 pb-16 text-slate-950">
    <StorefrontHeader reservation-mode />
    <section class="mx-auto max-w-5xl px-4 py-8 sm:px-6 lg:px-8">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p class="text-xs font-black uppercase tracking-[0.22em] text-orange-500">{{ t('reservations.anonymous.eyebrow') }}</p>
          <h1 class="mt-2 text-3xl font-black">{{ t('reservations.anonymous.myReservations') }}</h1>
          <p class="mt-2 max-w-2xl text-sm leading-6 text-slate-500">{{ t('reservations.anonymous.mineCookieWarning') }}</p>
        </div>
        <RouterLink :to="withLocalePath(locale, '/reservations')" class="text-sm font-black text-orange-600">← {{ t('reservations.backToList') }}</RouterLink>
      </div>

      <p v-if="errorMessage" role="alert" class="mt-6 rounded-xl bg-red-50 p-4 text-sm font-semibold text-red-700">{{ errorMessage }}</p>
      <div v-if="loading" class="mt-6 space-y-4" data-testid="my-reservations-loading">
        <div v-for="index in 3" :key="index" class="h-36 animate-pulse rounded-2xl bg-white" />
      </div>
      <div v-else-if="!reservations.length" class="mt-6 rounded-2xl border border-slate-200 bg-white p-10 text-center" data-testid="my-reservations-empty">
        <p class="font-black text-slate-800">{{ t('reservations.anonymous.mineEmpty') }}</p>
      </div>
      <div v-else class="mt-6 space-y-4" data-testid="my-reservations-list">
        <article v-for="item in reservations" :key="item.reservation_code" class="grid gap-4 rounded-2xl border border-slate-200 bg-white p-4 shadow-sm sm:grid-cols-[7rem_minmax(0,1fr)_auto] sm:items-center">
          <ProductImage :src="item.product.main_image_url" :alt="item.product.title" loading="lazy" image-class="h-28 w-full rounded-xl object-cover" />
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h2 class="truncate font-black text-slate-900">{{ item.product.title }}</h2>
              <span class="rounded-full bg-slate-100 px-2.5 py-1 text-xs font-bold text-slate-600">{{ t(`reservations.anonymous.status.${item.status}`) }}</span>
            </div>
            <p class="mt-2 break-all font-mono text-xs font-bold text-slate-600">{{ item.reservation_code }}</p>
            <p class="mt-2 text-xs text-slate-500">
              {{ t('reservations.anonymous.quantity') }}: {{ item.quantity }} ·
              {{ t('reservations.anonymous.createdAt', { time: formatDate(item.created_at) }) }}
            </p>
            <p class="mt-1 text-xs text-slate-500">{{ t('reservations.anonymous.expiresAt', { time: formatDate(item.expires_at) }) }}</p>
          </div>
          <button v-if="item.status === 'pending'" type="button" class="rounded-xl border border-red-200 px-4 py-2.5 text-sm font-black text-red-600 disabled:opacity-50" :disabled="Boolean(cancellingCode)" @click="cancel(item)">
            {{ cancellingCode === item.reservation_code ? t('reservations.anonymous.cancelling') : t('reservations.anonymous.cancel') }}
          </button>
        </article>
      </div>
    </section>
  </main>
</template>
