<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import ProductImage from '../../components/ProductImage.vue'
import StorefrontHeader from '../../components/storefront/StorefrontHeader.vue'
import ReservationHeatBadge from '../../components/storefront/ReservationHeatBadge.vue'
import { createReservationInterest, getReservationProducts, type ReservationProductPublic } from '../../api/reservations'
import { anonymousReservationEnabled, reservationEnabled } from '../../config/storefront'
import { currentLocale, withLocalePath } from '../../utils/localePath'
import { getStorefrontSession, getStorefrontVisitorID } from '../../utils/storefrontAnalytics'
import { newReservationID, reservationWindowLabel } from '../../utils/reservationUi'

const PAGE_SIZE = 12
const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const locale = computed(() => currentLocale(route))
const products = ref<ReservationProductPublic[]>([])
const total = ref(0)
const loading = ref(true)
const errorMessage = ref('')
const searchInput = ref(queryString(route.query.search))
const interestBusy = ref(new Set<number>())
const interestErrors = ref<Record<number, string>>({})
let loadSequence = 0
let interestGeneration = 0

const currentPage = computed(() => positiveInteger(route.query.page) || 1)
const search = computed(() => queryString(route.query.search).trim())
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))

watch([() => route.query.page, () => route.query.search, locale], () => void loadProducts(), { immediate: true })
watch(search, (value) => { searchInput.value = value })

function queryString(value: unknown) {
  if (Array.isArray(value)) return typeof value[0] === 'string' ? value[0] : ''
  return typeof value === 'string' ? value : ''
}

function positiveInteger(value: unknown) {
  const number = Number(queryString(value))
  return Number.isInteger(number) && number > 0 ? number : 0
}

async function loadProducts() {
  if (!reservationEnabled) {
    loadSequence += 1
    products.value = []
    total.value = 0
    loading.value = false
    errorMessage.value = ''
    return
  }
  const sequence = ++loadSequence
  interestGeneration += 1
  interestBusy.value = new Set()
  interestErrors.value = {}
  loading.value = true
  errorMessage.value = ''
  try {
    const page = await getReservationProducts({
      page: currentPage.value,
      page_size: PAGE_SIZE,
      search: search.value || undefined,
      visitor_id: getStorefrontVisitorID(),
    })
    if (sequence !== loadSequence) return
    products.value = page.items
    total.value = page.total
  } catch {
    if (sequence !== loadSequence) return
    products.value = []
    total.value = 0
    errorMessage.value = t('reservations.loadError')
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

async function submitSearch() {
  const query: Record<string, string> = {}
  if (searchInput.value.trim()) query.search = searchInput.value.trim()
  await router.push({ path: withLocalePath(locale.value, '/reservations'), query })
}

async function changePage(page: number) {
  if (page < 1 || page > pageCount.value || page === currentPage.value) return
  const query: Record<string, string> = { page: String(page) }
  if (search.value) query.search = search.value
  await router.push({ path: withLocalePath(locale.value, '/reservations'), query })
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

function detailPath(product: ReservationProductPublic) {
  return withLocalePath(locale.value, `/reservations/${product.slug}`)
}

function windowLabel(product: ReservationProductPublic) {
  return reservationWindowLabel(product.reservation_start_at, product.reservation_end_at, locale.value, t('reservations.openNow'))
}

async function recordInterest(product: ReservationProductPublic) {
  if (!reservationEnabled) return
  if (product.viewer_state.interested || interestBusy.value.has(product.id)) return
  const generation = interestGeneration
  interestBusy.value = new Set(interestBusy.value).add(product.id)
  interestErrors.value = { ...interestErrors.value, [product.id]: '' }
  try {
    const session = getStorefrontSession()
    const result = await createReservationInterest(product.id, {
      visitor_id: getStorefrontVisitorID(),
      session_id: session.id,
      idempotency_key: newReservationID(),
      source_type: 'reservation_list',
      source_id: String(product.id),
    })
    if (generation === interestGeneration && products.value.includes(product) && result.interested) product.viewer_state.interested = true
  } catch {
    if (generation === interestGeneration && products.value.includes(product)) {
      interestErrors.value = { ...interestErrors.value, [product.id]: t('reservations.interestError') }
    }
  } finally {
    if (generation !== interestGeneration) return
    const next = new Set(interestBusy.value)
    next.delete(product.id)
    interestBusy.value = next
  }
}
</script>

<template>
  <main class="min-h-screen bg-slate-50 pb-16 text-slate-950">
    <StorefrontHeader reservation-mode />

    <section class="mx-auto max-w-7xl px-4 py-8 sm:px-6 lg:px-8">
      <div class="overflow-hidden rounded-3xl bg-slate-950 px-6 py-9 text-white sm:px-10 sm:py-12">
        <p class="text-xs font-black uppercase tracking-[0.28em] text-orange-400">{{ t('reservations.eyebrow') }}</p>
        <div class="mt-3 max-w-3xl">
          <h1 class="text-3xl font-black tracking-tight sm:text-5xl">{{ t('reservations.title') }}</h1>
          <p class="mt-4 max-w-2xl text-sm leading-7 text-slate-300 sm:text-base">{{ t('reservations.subtitle') }}</p>
        </div>
        <div class="mt-6 flex flex-col gap-3 sm:flex-row sm:items-center">
          <form class="flex max-w-xl flex-1 rounded-xl bg-white p-1" @submit.prevent="submitSearch">
            <input v-model="searchInput" type="search" class="min-w-0 flex-1 bg-transparent px-3 py-2.5 text-sm text-slate-950 outline-none" :placeholder="t('reservations.searchPlaceholder')" />
            <button type="submit" class="rounded-lg bg-orange-500 px-5 text-sm font-black text-white">{{ t('nav.search') }}</button>
          </form>
          <RouterLink v-if="anonymousReservationEnabled" :to="withLocalePath(locale, '/reservations/mine')" class="rounded-xl border border-white/30 px-5 py-3 text-center text-sm font-black text-white">
            {{ t('reservations.anonymous.myReservations') }}
          </RouterLink>
        </div>
      </div>

      <div v-if="loading" data-testid="reservation-loading" class="grid gap-5 py-8 sm:grid-cols-2 lg:grid-cols-3">
        <div v-for="index in 6" :key="index" class="h-96 animate-pulse rounded-2xl bg-white" />
      </div>

      <div v-else-if="errorMessage" data-testid="reservation-error" class="mt-8 rounded-2xl border border-red-100 bg-red-50 p-8 text-center">
        <p class="font-bold text-red-700">{{ errorMessage }}</p>
        <button type="button" class="mt-4 rounded-xl bg-red-600 px-5 py-2.5 text-sm font-black text-white" @click="loadProducts">{{ t('reservations.retry') }}</button>
      </div>

      <div v-else-if="!products.length" data-testid="reservation-empty" class="mt-8 rounded-2xl border border-slate-200 bg-white p-12 text-center">
        <h2 class="text-xl font-black text-slate-900">{{ t('reservations.empty') }}</h2>
        <p class="mt-2 text-sm text-slate-500">{{ t('reservations.emptyHint') }}</p>
      </div>

      <div v-else class="mt-8 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
        <article v-for="product in products" :key="product.id" class="group overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm transition hover:-translate-y-0.5 hover:shadow-lg">
          <RouterLink :to="detailPath(product)" class="block aspect-[4/3] overflow-hidden">
            <ProductImage :src="product.main_image_url" :alt="product.title" loading="lazy" image-class="h-full w-full object-cover transition duration-300 group-hover:scale-105" />
          </RouterLink>
          <div class="p-5">
            <div class="flex items-center justify-between gap-3">
              <ReservationHeatBadge :level="product.heat_level" />
              <span v-if="product.is_featured" class="text-xs font-black uppercase tracking-wider text-orange-600">{{ t('reservations.featured') }}</span>
            </div>
            <RouterLink :to="detailPath(product)" class="mt-3 block">
              <h2 class="line-clamp-2 text-xl font-black text-slate-950">{{ product.title }}</h2>
              <p class="mt-2 line-clamp-3 min-h-[4.5rem] text-sm leading-6 text-slate-600">{{ product.summary }}</p>
            </RouterLink>
            <p class="mt-3 text-xs font-semibold text-slate-400">{{ windowLabel(product) }}</p>
            <div class="mt-5 grid grid-cols-2 gap-2">
              <RouterLink :to="detailPath(product)" class="rounded-xl border border-slate-200 px-4 py-3 text-center text-sm font-black text-slate-700 hover:border-orange-200 hover:text-orange-600">
                {{ t('reservations.viewDetails') }}
              </RouterLink>
              <button type="button" class="rounded-xl bg-orange-500 px-4 py-3 text-sm font-black text-white disabled:bg-emerald-600" :disabled="product.viewer_state.interested || interestBusy.has(product.id)" @click="recordInterest(product)">
                {{ product.viewer_state.interested ? t('reservations.interested') : interestBusy.has(product.id) ? t('reservations.saving') : t('reservations.interest') }}
              </button>
            </div>
            <p v-if="interestErrors[product.id]" role="alert" class="mt-2 text-xs font-semibold text-red-600">{{ interestErrors[product.id] }}</p>
          </div>
        </article>
      </div>

      <nav v-if="!loading && pageCount > 1" :aria-label="t('reservations.pagination')" class="mt-8 flex items-center justify-center gap-3">
        <button type="button" class="rounded-xl border border-slate-200 bg-white px-4 py-2.5 text-sm font-black disabled:opacity-40" :disabled="currentPage <= 1" @click="changePage(currentPage - 1)">{{ t('mall.previousPage') }}</button>
        <span class="text-sm font-bold text-slate-500">{{ t('mall.pageStatus', { page: currentPage, pages: pageCount }) }}</span>
        <button type="button" class="rounded-xl border border-slate-200 bg-white px-4 py-2.5 text-sm font-black disabled:opacity-40" :disabled="currentPage >= pageCount" @click="changePage(currentPage + 1)">{{ t('mall.nextPage') }}</button>
      </nav>
    </section>
  </main>
</template>
