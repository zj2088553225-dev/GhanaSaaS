<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import ProductImage from '../../components/ProductImage.vue'
import StorefrontHeader from '../../components/storefront/StorefrontHeader.vue'
import ReservationHeatBadge from '../../components/storefront/ReservationHeatBadge.vue'
import AnonymousReservationDialog from '../../components/storefront/AnonymousReservationDialog.vue'
import ReservationVerificationDialog from '../../components/storefront/ReservationVerificationDialog.vue'
import { createReservationInterest, getReservationProduct, type ReservationProductPublic } from '../../api/reservations'
import { anonymousReservationEnabled, otpReservationEnabled, reservationEnabled } from '../../config/storefront'
import { sanitizeProductDetailHtml } from '../../utils/sanitizeProductDetail'
import { currentLocale, withLocalePath } from '../../utils/localePath'
import { getStorefrontSession, getStorefrontVisitorID } from '../../utils/storefrontAnalytics'
import { newReservationID, reservationWindowLabel } from '../../utils/reservationUi'

const route = useRoute()
const { t } = useI18n()
const locale = computed(() => currentLocale(route))
const slug = computed(() => Array.isArray(route.params.slug) ? route.params.slug[0] || '' : String(route.params.slug || ''))
const product = ref<ReservationProductPublic | null>(null)
const activeImage = ref('')
const loading = ref(true)
const errorMessage = ref('')
const interestBusy = ref(false)
const interestError = ref('')
const verificationOpen = ref(false)
let loadSequence = 0
let interestSequence = 0

const gallery = computed(() => {
  const images = [product.value?.main_image_url || '', ...(product.value?.gallery || [])].filter(Boolean)
  return Array.from(new Set(images))
})
const safeDescription = computed(() => sanitizeProductDetailHtml(product.value?.description || ''))
const windowLabel = computed(() => product.value
  ? reservationWindowLabel(product.value.reservation_start_at, product.value.reservation_end_at, locale.value, t('reservations.openNow'))
  : '')

watch([slug, locale], () => {
  interestSequence += 1
  interestBusy.value = false
  verificationOpen.value = false
  void loadProduct()
}, { immediate: true })

async function loadProduct() {
  if (!reservationEnabled) {
    loadSequence += 1
    product.value = null
    loading.value = false
    errorMessage.value = ''
    return
  }
  const sequence = ++loadSequence
  loading.value = true
  errorMessage.value = ''
  try {
    const result = await getReservationProduct(slug.value, getStorefrontVisitorID())
    if (sequence !== loadSequence) return
    product.value = result
    activeImage.value = result.main_image_url || result.gallery[0] || ''
  } catch {
    if (sequence !== loadSequence) return
    product.value = null
    errorMessage.value = t('reservations.loadError')
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

async function recordInterest() {
  if (!reservationEnabled) return
  if (!product.value || product.value.viewer_state.interested || interestBusy.value) return
  const target = product.value
  const targetID = target.id
  const sequence = ++interestSequence
  interestBusy.value = true
  interestError.value = ''
  try {
    const session = getStorefrontSession()
    const result = await createReservationInterest(targetID, {
      visitor_id: getStorefrontVisitorID(),
      session_id: session.id,
      idempotency_key: newReservationID(),
      source_type: 'reservation_detail',
      source_id: String(targetID),
    })
    if (sequence === interestSequence && product.value === target && result.interested) target.viewer_state.interested = true
  } catch {
    if (sequence === interestSequence && product.value === target) interestError.value = t('reservations.interestError')
  } finally {
    if (sequence === interestSequence) interestBusy.value = false
  }
}

function markVerified() {
  if (product.value) product.value.viewer_state.verified_reserved = true
}
</script>

<template>
  <main class="min-h-screen bg-slate-50 pb-16 text-slate-950">
    <StorefrontHeader reservation-mode />
    <section class="mx-auto max-w-7xl px-4 py-7 sm:px-6 lg:px-8">
      <RouterLink :to="withLocalePath(locale, '/reservations')" class="text-sm font-black text-orange-600 hover:text-orange-700">← {{ t('reservations.backToList') }}</RouterLink>

      <div v-if="loading" data-testid="reservation-detail-loading" class="mt-6 grid animate-pulse gap-8 lg:grid-cols-2">
        <div class="aspect-square rounded-3xl bg-white" />
        <div class="h-96 rounded-3xl bg-white" />
      </div>
      <div v-else-if="errorMessage || !product" class="mt-6 rounded-2xl border border-red-100 bg-red-50 p-10 text-center">
        <p class="font-bold text-red-700">{{ errorMessage }}</p>
        <button type="button" class="mt-4 rounded-xl bg-red-600 px-5 py-2.5 text-sm font-black text-white" @click="loadProduct">{{ t('reservations.retry') }}</button>
      </div>

      <template v-else>
        <div class="mt-6 grid gap-8 lg:grid-cols-[minmax(0,1.05fr)_minmax(0,.95fr)]">
          <div>
            <div class="aspect-square overflow-hidden rounded-3xl border border-slate-200 bg-white shadow-sm">
              <ProductImage :src="activeImage" :alt="product.title" loading="eager" image-class="h-full w-full object-contain" />
            </div>
            <div v-if="gallery.length > 1" class="mt-3 flex gap-3 overflow-x-auto pb-1">
              <button v-for="image in gallery" :key="image" type="button" class="h-20 w-20 shrink-0 overflow-hidden rounded-xl border-2 bg-white" :class="activeImage === image ? 'border-orange-500' : 'border-transparent'" @click="activeImage = image">
                <ProductImage :src="image" :alt="product.title" loading="lazy" image-class="h-full w-full object-cover" />
              </button>
            </div>
          </div>

          <div class="rounded-3xl border border-slate-200 bg-white p-6 shadow-sm sm:p-8">
            <div class="flex flex-wrap items-center gap-3">
              <ReservationHeatBadge :level="product.heat_level" />
              <span v-if="product.is_featured" class="text-xs font-black uppercase tracking-wider text-orange-600">{{ t('reservations.featured') }}</span>
            </div>
            <h1 class="mt-4 text-3xl font-black tracking-tight text-slate-950 sm:text-4xl">{{ product.title }}</h1>
            <p class="mt-4 text-base leading-7 text-slate-600">{{ product.summary }}</p>
            <div class="mt-6 rounded-2xl bg-slate-50 p-4">
              <p class="text-xs font-black uppercase tracking-[0.18em] text-slate-400">{{ t('reservations.window') }}</p>
              <p class="mt-1 text-sm font-bold text-slate-700">{{ windowLabel }}</p>
            </div>
            <div v-if="product.viewer_state.verified_reserved" class="mt-5 rounded-xl bg-emerald-50 px-4 py-3 text-sm font-black text-emerald-700">✓ {{ t('reservations.alreadyReserved') }}</div>
            <div v-if="reservationEnabled" class="mt-6 grid gap-3 sm:grid-cols-2">
              <button type="button" class="rounded-xl border border-orange-200 bg-orange-50 px-5 py-3.5 text-sm font-black text-orange-700 disabled:border-emerald-200 disabled:bg-emerald-50 disabled:text-emerald-700" data-testid="reservation-interest" :disabled="product.viewer_state.interested || interestBusy" @click="recordInterest">
                {{ product.viewer_state.interested ? t('reservations.interested') : interestBusy ? t('reservations.saving') : t('reservations.interest') }}
              </button>
              <button type="button" class="rounded-xl bg-orange-500 px-5 py-3.5 text-sm font-black text-white" data-testid="reservation-submit" @click="verificationOpen = true">{{ t('reservations.reserveNow') }}</button>
            </div>
            <p v-if="interestError" role="alert" class="mt-2 text-xs font-semibold text-red-600">{{ interestError }}</p>
            <p class="mt-5 text-xs leading-5 text-slate-400">
              {{ anonymousReservationEnabled ? t('reservations.anonymous.noAccountHint') : t('reservations.noAccountHint') }}
            </p>
          </div>
        </div>

        <article class="prose prose-slate mt-8 max-w-none rounded-3xl border border-slate-200 bg-white p-6 shadow-sm sm:p-9">
          <h2 class="text-2xl font-black text-slate-950">{{ t('reservations.details') }}</h2>
          <div v-if="safeDescription" class="mt-5 leading-7 text-slate-700" v-html="safeDescription" />
          <p v-else class="mt-4 text-sm text-slate-500">{{ t('reservations.noDescription') }}</p>
        </article>

        <ReservationVerificationDialog
          v-if="otpReservationEnabled"
          :open="verificationOpen"
          :product-id="product.id"
          :product-title="product.title"
          @close="verificationOpen = false"
          @verified="markVerified"
        />
        <AnonymousReservationDialog
          v-if="anonymousReservationEnabled"
          :open="verificationOpen"
          :product-id="product.id"
          :product-title="product.title"
          @close="verificationOpen = false"
          @created="markVerified"
        />
      </template>
    </section>
  </main>
</template>
