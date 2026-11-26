<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import ProductImage from '../components/ProductImage.vue'
import ProductImageGallery from '../components/storefront/ProductImageGallery.vue'
import StorefrontHeader from '../components/storefront/StorefrontHeader.vue'
import { getProductDetail, type ProductDetail, type SKU } from '../api/storefront'
import { storefrontPriceVisible } from '../config/storefront'
import { useCartStore } from '../stores/cart'
import { formatMoney } from '../utils/money'
import { sanitizeProductDetailHtml } from '../utils/sanitizeProductDetail'
import { imageVariantURL } from '../utils/imageVariants'
import { buildProductSeoPath, parseProductIdFromSeoParam } from '../utils/productSeoUrl'
import { absoluteURL, canonicalURL, plainText, useSeoMeta, type SeoMetaInput } from '../composables/useSeoMeta'
import { currentLocale, withLocalePath } from '../utils/localePath.ts'
import { trackStorefrontEvent } from '../utils/storefrontAnalytics.ts'
import {
  activeImageForSku,
  buildProductGallery,
  buildProductMediaGallery,
  localizedSkuDisplayName,
  skuImageURL,
  skuStock,
  validateSkuBatchQuantity,
} from '../utils/productSku'

type DetailTab = 'detail' | 'params' | 'reviews'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const cartStore = useCartStore()
const activeLocale = computed(() => currentLocale(route))
const routeProductParam = computed(() => {
  const value = route.params.productParam
  return Array.isArray(value) ? value[0] || '' : String(value || '')
})

const product = ref<ProductDetail | null>(null)
const skuQuantities = ref<Record<number, number>>({})
const errorMessage = ref('')
const purchaseMessage = ref('')
const addedMessage = ref('')
const activeImage = ref('')
const activeTab = ref<DetailTab>('detail')
const detailSection = ref<HTMLElement>()
const paramsSection = ref<HTMLElement>()
const reviewsSection = ref<HTMLElement>()
let loadSequence = 0
const viewedProductVisits = new Set<string>()

const MOBILE_HEADER_OFFSET = 116
const DESKTOP_HEADER_OFFSET = 69
const DETAIL_TAB_HEIGHT = 56
const DETAIL_SECTION_GAP = 16
const DEFAULT_PRODUCT_DESCRIPTION = 'View this product on Ghana Chinatown.'

const detailTabs = computed(() => [
  { key: 'detail' as const, label: t('product.detailTab') },
  { key: 'params' as const, label: t('product.paramsTab') },
  { key: 'reviews' as const, label: t('product.reviewsTab') },
])
const skus = computed(() => product.value?.skus ?? [])
const isOnShelf = computed(() => product.value?.status === 'on_shelf')
const totalStock = computed(() => product.value?.total_stock ?? skus.value.reduce((total, sku) => total + skuStock(sku), 0))
const galleryImages = computed(() => buildProductGallery(product.value))
const galleryMedia = computed(() => buildProductMediaGallery(product.value))
const productMainImage = computed(
  () => product.value?.main_image?.trim() || product.value?.mainImage?.trim() || product.value?.image_url?.trim() || '',
)
const routeProductId = computed(() => parseProductIdFromSeoParam(routeProductParam.value))
const productSeoPath = computed(() =>
  product.value
    ? buildProductSeoPath(product.value, activeLocale.value)
    : routeProductId.value
      ? buildProductSeoPath({ id: routeProductId.value }, activeLocale.value)
      : withLocalePath(activeLocale.value, '/products'),
)
const productDescription = computed(() => product.value?.description?.trim() ?? '')
const sanitizedDetailHTML = computed(() => sanitizeProductDetailHtml(product.value?.detail_html))
const displayPrice = computed(() => {
  const minPrice = product.value?.min_price ?? 0
  const maxPrice = product.value?.max_price ?? minPrice
  return minPrice === maxPrice ? formatMoney(minPrice) : `${formatMoney(minPrice)} - ${formatMoney(maxPrice)}`
})
const displayOriginalPrice = computed(() => {
  const originalPrice = product.value?.original_price ?? 0
  const currentPrice = product.value?.min_price ?? 0
  return originalPrice > currentPrice ? formatMoney(originalPrice) : ''
})
const productSeoMeta = computed<SeoMetaInput>(() => {
  const canonicalPath = productSeoPath.value
  const canonical = canonicalURL(route.path === canonicalPath ? route.path : canonicalPath)
  if (!product.value) {
    return {
      title: 'Ghana Chinatown Product',
      description: DEFAULT_PRODUCT_DESCRIPTION,
      canonical,
      robots: errorMessage.value ? 'noindex,nofollow' : 'index,follow',
    }
  }

  const description = plainText(product.value.description || product.value.detail_html) || `View ${product.value.name} on Ghana Chinatown.`
  const images = [productMainImage.value, ...galleryImages.value].filter(Boolean).map((image) => absoluteURL(image))
  const uniqueImages = Array.from(new Set(images))
  const shopName = product.value.shop_name?.trim() || ''
  const titleSuffix = product.value.source === 't6'
    ? 'T6 Mall | Ghana Chinatown'
    : shopName
      ? `${shopName} | Ghana Chinatown`
      : 'Ghana Chinatown'
  const brandName = product.value.source === 't6' ? 'T6 Mall' : shopName || 'Ghana Chinatown'
  return {
    title: `${product.value.name} | ${titleSuffix}`,
    description,
    canonical,
    robots: product.value.status === 'on_shelf' ? 'index,follow' : 'noindex,nofollow',
    type: 'product',
    image: uniqueImages[0],
    jsonLd: [
      {
        '@context': 'https://schema.org',
        '@type': 'Product',
        name: product.value.name,
        description,
        image: uniqueImages,
        brand: {
          '@type': 'Brand',
          name: brandName,
        },
        url: canonical,
      },
    ],
  }
})
useSeoMeta(productSeoMeta)

const selectedBatchItems = computed(() =>
  skus.value
    .map((sku) => ({ sku, quantity: Math.max(0, Math.floor(Number(skuQuantities.value[sku.id]) || 0)) }))
    .filter((item) => item.quantity > 0),
)
const selectedBatchCount = computed(() => selectedBatchItems.value.reduce((sum, item) => sum + item.quantity, 0))
const selectedBatchAmount = computed(() =>
  selectedBatchItems.value.reduce((sum, item) => sum + item.sku.retail_price * item.quantity, 0),
)
const batchAddDisabled = computed(
  () => !product.value || !isOnShelf.value || !skus.value.some((sku) => sku.is_active && skuStock(sku) > 0),
)
const batchButtonLabel = computed(() => {
  if (!isOnShelf.value) {
    return t('product.offShelf')
  }
  if (!skus.value.some((sku) => sku.is_active && skuStock(sku) > 0)) {
    return t('product.outOfStock')
  }
  return t('product.addToCart')
})

onMounted(async () => {
  await loadProduct()
  window.addEventListener('scroll', updateActiveDetailTab, { passive: true })
  window.addEventListener('resize', updateActiveDetailTab)
  updateActiveDetailTab()
})

watch(
  () => routeProductParam.value,
  () => {
    const nextID = routeProductId.value
    if (nextID && product.value?.id === nextID) {
      return
    }
    void loadProduct()
  },
)

watch(activeLocale, () => {
  void loadProduct()
})

onUnmounted(() => {
  window.removeEventListener('scroll', updateActiveDetailTab)
  window.removeEventListener('resize', updateActiveDetailTab)
})

async function loadProduct() {
  const sequence = ++loadSequence
  errorMessage.value = ''
  product.value = null

  try {
    const id = routeProductId.value
    if (!id) {
      throw new Error(t('product.invalid'))
    }

    const detail = await getProductDetail(id)
    if (sequence !== loadSequence) {
      return
    }
    product.value = detail
    activeImage.value = buildProductMediaGallery(detail)[0]?.url ?? ''
    activeTab.value = 'detail'
    purchaseMessage.value = ''
    addedMessage.value = ''

    skuQuantities.value = {}
    for (const sku of detail.skus) {
      skuQuantities.value[sku.id] = 0
    }
    await replaceWithCanonicalProductPath(detail)
    if (sequence === loadSequence && product.value?.id === detail.id) {
      trackLoadedProductView(detail, sequence)
    }
  } catch (error) {
    if (sequence !== loadSequence) {
      return
    }
    errorMessage.value = error instanceof Error ? error.message : t('product.error')
  }
  if (sequence === loadSequence && product.value) {
    await nextTick()
    updateActiveDetailTab()
  }
}

function trackLoadedProductView(detail: ProductDetail, visitID: number) {
  const visitKey = `${visitID}:${detail.id}`
  if (viewedProductVisits.has(visitKey)) {
    return
  }
  viewedProductVisits.add(visitKey)
  trackStorefrontEvent({
    event_name: 'product_view',
    entity_type: 'product',
    entity_id: String(detail.id),
    product_id: detail.id,
    source_type: 'product_detail',
    source_id: String(detail.id),
    page_type: 'product',
  })
}

async function replaceWithCanonicalProductPath(detail: ProductDetail) {
  const nextPath = buildProductSeoPath(detail, activeLocale.value)
  if (route.path === nextPath) {
    return
  }
  await router.replace({
    path: nextPath,
    query: route.query,
    hash: route.hash,
  })
}

function detailSectionElement(key: DetailTab) {
  switch (key) {
    case 'params':
      return paramsSection.value
    case 'reviews':
      return reviewsSection.value
    default:
      return detailSection.value
  }
}

function scrollToDetailSection(key: DetailTab) {
  const section = detailSectionElement(key)
  if (!section) {
    return
  }

  activeTab.value = key
  const top = window.scrollY + section.getBoundingClientRect().top - detailSectionScrollOffset()
  window.scrollTo({ top: Math.max(0, top), behavior: 'smooth' })
}

function detailSectionScrollOffset() {
  const headerOffset = window.innerWidth >= 768 ? DESKTOP_HEADER_OFFSET : MOBILE_HEADER_OFFSET
  return headerOffset + DETAIL_TAB_HEIGHT + DETAIL_SECTION_GAP
}

function updateActiveDetailTab() {
  const sections: Array<{ key: DetailTab; element?: HTMLElement }> = [
    { key: 'detail', element: detailSection.value },
    { key: 'params', element: paramsSection.value },
    { key: 'reviews', element: reviewsSection.value },
  ]
  const availableSections = sections.filter(
    (section): section is { key: DetailTab; element: HTMLElement } => Boolean(section.element),
  )
  if (!availableSections.length) {
    return
  }

  const reachedBottom =
    window.scrollY + window.innerHeight >= document.documentElement.scrollHeight - DETAIL_SECTION_GAP
  if (reachedBottom) {
    activeTab.value = availableSections[availableSections.length - 1].key
    return
  }

  const threshold = detailSectionScrollOffset()
  const currentSection = [...availableSections]
    .reverse()
    .find((section) => section.element.getBoundingClientRect().top <= threshold)
  activeTab.value = currentSection?.key ?? 'detail'
}

function skuImage(sku: SKU): string {
  return skuImageURL(sku) || productMainImage.value || galleryImages.value[0] || ''
}

function activateSkuImage(sku: SKU) {
  activeImage.value = activeImageForSku(activeImage.value, sku)
}

function skuDisplayName(sku: SKU): string {
  return localizedSkuDisplayName(sku, t('product.defaultSku'))
}

function quantityForSku(sku: SKU): number {
  return Math.max(0, Math.floor(Number(skuQuantities.value[sku.id]) || 0))
}

function maxAddableQuantity(sku: SKU): number {
  const stock = skuStock(sku)
  const inCart = cartStore.quantityForSku(sku.id)
  return Math.max(0, stock - inCart)
}

function isSkuDisabled(sku: SKU): boolean {
  return !sku.is_active || skuStock(sku) <= 0
}

function decreaseSkuQuantity(sku: SKU) {
  activateSkuImage(sku)
  const current = quantityForSku(sku)
  if (current > 0) {
    skuQuantities.value[sku.id] = current - 1
  }
}

function increaseSkuQuantity(sku: SKU) {
  activateSkuImage(sku)
  const current = quantityForSku(sku)
  const max = maxAddableQuantity(sku)
  if (current < max) {
    skuQuantities.value[sku.id] = current + 1
  }
}

function normalizeSkuQuantity(sku: SKU) {
  activateSkuImage(sku)
  const max = maxAddableQuantity(sku)
  const current = Math.max(0, Math.floor(Number(skuQuantities.value[sku.id]) || 0))
  skuQuantities.value[sku.id] = Math.min(current, max)
}

function addBatchToCart() {
  purchaseMessage.value = ''
  addedMessage.value = ''

  if (!product.value || !isOnShelf.value) {
    return
  }

  if (selectedBatchCount.value === 0) {
    purchaseMessage.value = t('product.chooseQuantity')
    return
  }

  const items: { product_id: number; sku_id: number; quantity: number }[] = []
  for (const { sku, quantity } of selectedBatchItems.value) {
    if (!sku.is_active) {
      purchaseMessage.value = t('cart.skuUnavailable')
      return
    }
    const validation = validateSkuBatchQuantity(sku, quantity, cartStore.quantityForSku(sku.id))
    if (validation === 'out_of_stock') {
      purchaseMessage.value = t('product.outOfStock')
      return
    }
    if (validation === 'exceeds_stock') {
      purchaseMessage.value = t('product.stockShortage')
      return
    }
    items.push({ product_id: product.value.id, sku_id: sku.id, quantity })
  }

  cartStore.addItems(items)
  addedMessage.value = t('product.added')

  for (const item of items) {
    skuQuantities.value[item.sku_id] = 0
  }
}
</script>

<template>
  <main class="min-h-screen overflow-x-clip bg-slate-50 pb-20 text-slate-950 md:pb-28">
    <StorefrontHeader />

    <section class="mx-auto max-w-7xl px-4 py-5 sm:px-6 lg:px-8">
      <article v-if="product" class="space-y-5">
        <section class="grid min-w-0 items-start gap-6 rounded-2xl border border-slate-100 bg-white p-4 shadow-sm sm:p-5 lg:grid-cols-[minmax(0,520px)_minmax(0,1fr)]">
          <div class="min-w-0 self-start lg:sticky lg:top-24">
            <ProductImageGallery v-model="activeImage" :media="galleryMedia" :alt="product.name" />
          </div>

          <section class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <span
                class="rounded-full px-3 py-1 text-xs font-black"
                :class="isOnShelf ? 'bg-emerald-50 text-emerald-700' : 'bg-slate-100 text-slate-500'"
              >
                {{ isOnShelf ? t('product.onShelf') : t('product.offShelf') }}
              </span>
              <span v-if="product.platform_category" class="rounded-full bg-slate-100 px-3 py-1 text-xs font-bold text-slate-600">
                {{ product.platform_category.path || product.platform_category.name }}
              </span>
            </div>

            <h1 class="mt-3 text-2xl font-black leading-tight text-slate-950 sm:text-3xl">{{ product.name }}</h1>
            <p v-if="productDescription" class="mt-3 line-clamp-3 whitespace-pre-line text-sm leading-6 text-slate-600">
              {{ productDescription }}
            </p>

            <div class="mt-5 rounded-xl bg-orange-50 px-4 py-4">
              <template v-if="storefrontPriceVisible">
                <p class="text-xs font-black uppercase tracking-[0.18em] text-orange-500">{{ t('product.price') }}</p>
                <div class="mt-2 flex flex-wrap items-end gap-3">
                  <p class="text-3xl font-black text-orange-600 sm:text-4xl">{{ displayPrice }}</p>
                  <p v-if="displayOriginalPrice" class="pb-1 text-sm font-bold text-slate-400 line-through">{{ displayOriginalPrice }}</p>
                </div>
              </template>
              <p v-else class="text-2xl font-black text-orange-600 sm:text-3xl">{{ t('storefront.contactForPrice') }}</p>
              <div class="mt-3 flex flex-wrap items-center gap-3 border-t border-orange-100 pt-3 text-sm">
                <span class="font-bold text-slate-500">{{ t('product.totalStock') }}</span>
                <span class="font-black text-slate-900">{{ totalStock }}</span>
                <span class="font-bold text-slate-500">{{ t('product.skuCount') }}</span>
                <span class="font-black text-slate-900">{{ skus.length }}</span>
              </div>
            </div>

            <!-- SKU batch selection list -->
            <div class="mt-6">
              <div class="mb-3 flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
                <p class="text-base font-black text-slate-900">{{ t('product.batchSelection') }}</p>
                <p class="text-xs text-slate-500">{{ t('product.batchSelectionHint') }}</p>
              </div>

              <div v-if="skus.length" class="max-h-[420px] overflow-y-auto rounded-xl border border-slate-200">
                <div
                  v-for="sku in skus"
                  :key="sku.id"
                  class="grid grid-cols-[56px_minmax(0,1fr)] items-center gap-3 border-b border-slate-100 px-3 py-3 last:border-b-0 sm:grid-cols-[56px_minmax(0,1fr)_auto]"
                  :class="isSkuDisabled(sku) ? 'bg-slate-50 opacity-60' : 'bg-white'"
                  @click="activateSkuImage(sku)"
                >
                  <div class="h-14 w-14 shrink-0 overflow-hidden rounded-lg border border-slate-100 bg-slate-50">
                    <ProductImage :src="imageVariantURL(skuImage(sku), 'thumb')" :alt="skuDisplayName(sku)" class="h-full w-full" image-class="h-full w-full object-cover" />
                  </div>
                  <div class="min-w-0 flex-1">
                    <p class="line-clamp-2 text-sm font-black leading-5 text-slate-900">{{ skuDisplayName(sku) }}</p>
                    <div class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs font-bold">
                      <span v-if="storefrontPriceVisible" class="text-orange-600">{{ formatMoney(sku.retail_price) }}</span>
                      <span :class="isSkuDisabled(sku) ? 'text-slate-400' : 'text-slate-500'">
                        {{ t('product.stock') }} {{ skuStock(sku) }}
                      </span>
                    </div>
                  </div>
                  <div class="col-start-2 flex items-center sm:col-auto">
                    <div class="flex items-center rounded-lg border border-slate-200 bg-white p-0.5">
                      <button
                        type="button"
                        class="h-8 w-8 rounded-md text-base font-black hover:bg-slate-100 disabled:text-slate-300"
                        :disabled="isSkuDisabled(sku) || quantityForSku(sku) <= 0"
                        @click="decreaseSkuQuantity(sku)"
                      >
                        -
                      </button>
                      <input
                        v-model.number="skuQuantities[sku.id]"
                        type="number"
                        min="0"
                        :max="maxAddableQuantity(sku)"
                        class="h-8 w-12 border-0 p-0 text-center text-sm font-black outline-none disabled:bg-slate-50"
                        :disabled="isSkuDisabled(sku)"
                        @focus="activateSkuImage(sku)"
                        @blur="normalizeSkuQuantity(sku)"
                      />
                      <button
                        type="button"
                        class="h-8 w-8 rounded-md text-base font-black hover:bg-slate-100 disabled:text-slate-300"
                        :disabled="isSkuDisabled(sku) || quantityForSku(sku) >= maxAddableQuantity(sku)"
                        @click="increaseSkuQuantity(sku)"
                      >
                        +
                      </button>
                    </div>
                  </div>
                </div>
              </div>
              <div v-else class="rounded-xl border border-dashed border-slate-200 bg-slate-50 p-6 text-center text-sm font-bold text-slate-500">
                {{ t('product.noPurchasableSku') }}
              </div>
            </div>

            <p v-if="purchaseMessage" class="mt-4 rounded-lg bg-red-50 px-4 py-3 text-sm font-bold text-red-600">
              {{ purchaseMessage }}
            </p>
            <p v-else-if="addedMessage" class="mt-4 rounded-lg bg-emerald-50 px-4 py-3 text-sm font-bold text-emerald-700">
              {{ addedMessage }}
            </p>
          </section>
        </section>

        <section class="rounded-2xl border border-slate-100 bg-white shadow-sm">
          <div
            class="scrollbar-hide sticky top-[116px] z-20 flex w-full max-w-full overflow-x-auto overscroll-x-contain whitespace-nowrap border-b border-slate-100 bg-white/95 px-2 backdrop-blur md:top-[69px]"
          >
            <button
              v-for="tab in detailTabs"
              :key="tab.key"
              type="button"
              class="min-h-14 shrink-0 px-5 text-sm font-black transition"
              :class="activeTab === tab.key ? 'border-b-2 border-orange-500 text-orange-600' : 'text-slate-500 hover:text-slate-900'"
              @click="scrollToDetailSection(tab.key)"
            >
              {{ tab.label }}
            </button>
          </div>

          <div class="divide-y divide-slate-100">
            <section
              ref="detailSection"
              data-detail-section="detail"
              class="scroll-mt-40 px-4 py-7 sm:px-6 md:scroll-mt-32"
            >
              <h2 class="text-xl font-black text-slate-950">{{ t('product.detailTab') }}</h2>
              <div
                v-if="sanitizedDetailHTML"
                class="product-detail-rich mx-auto mt-5 max-w-5xl text-sm leading-7 text-slate-700"
                v-html="sanitizedDetailHTML"
              />
              <div v-else class="mt-5 whitespace-pre-line text-sm leading-7 text-slate-700">
                {{ product.description || t('product.noDescription') }}
              </div>
            </section>

            <section
              ref="paramsSection"
              data-detail-section="params"
              class="scroll-mt-40 px-4 py-7 sm:px-6 md:scroll-mt-32"
            >
              <h2 class="text-xl font-black text-slate-950">{{ t('product.paramsTab') }}</h2>
              <dl class="mt-5 overflow-hidden rounded-xl border border-slate-200 text-sm">
                <div class="grid grid-cols-[120px_minmax(0,1fr)] border-b border-slate-100 last:border-b-0 sm:grid-cols-[180px_minmax(0,1fr)]">
                  <dt class="bg-slate-50 px-4 py-3 font-bold text-slate-500">{{ t('product.material') }}</dt>
                  <dd class="px-4 py-3 font-semibold text-slate-800">{{ product.material || '-' }}</dd>
                </div>
                <div class="grid grid-cols-[120px_minmax(0,1fr)] border-b border-slate-100 last:border-b-0 sm:grid-cols-[180px_minmax(0,1fr)]">
                  <dt class="bg-slate-50 px-4 py-3 font-bold text-slate-500">{{ t('product.dimensions') }}</dt>
                  <dd class="px-4 py-3 font-semibold text-slate-800">{{ product.dimensions || '-' }}</dd>
                </div>
                <div class="grid grid-cols-[120px_minmax(0,1fr)] sm:grid-cols-[180px_minmax(0,1fr)]">
                  <dt class="bg-slate-50 px-4 py-3 font-bold text-slate-500">{{ t('product.weight') }}</dt>
                  <dd class="px-4 py-3 font-semibold text-slate-800">{{ product.weight || '-' }}</dd>
                </div>
              </dl>
            </section>

            <section
              ref="reviewsSection"
              data-detail-section="reviews"
              class="scroll-mt-40 px-4 py-7 sm:px-6 md:scroll-mt-32"
            >
              <h2 class="text-xl font-black text-slate-950">{{ t('product.reviewsTab') }}</h2>
              <div class="mt-5 rounded-lg bg-slate-50 p-10 text-center text-sm font-bold text-slate-500">
                {{ t('product.noReviews') }}
              </div>
            </section>
          </div>
        </section>
      </article>

      <div v-else-if="errorMessage" class="rounded-lg bg-red-50 p-8 text-center text-sm font-semibold text-red-600">
        {{ errorMessage }}
      </div>

      <div v-else class="min-h-[480px]" aria-hidden="true"></div>
    </section>

    <!-- Bottom sticky action bar -->
    <div v-if="product && isOnShelf" class="fixed inset-x-0 bottom-0 z-30 border-t border-slate-200 bg-white p-3 shadow-[0_-4px_20px_rgba(0,0,0,0.05)] md:px-6 md:py-4">
      <div class="mx-auto flex max-w-7xl items-center justify-between gap-3">
        <div class="min-w-0">
          <p class="text-xs text-slate-500">{{ t('product.selectedCount', { count: selectedBatchCount }) }}</p>
          <p v-if="storefrontPriceVisible" class="text-lg font-black text-orange-600 md:text-xl">
            {{ selectedBatchAmount > 0 ? formatMoney(selectedBatchAmount) : '-' }}
          </p>
        </div>
        <div class="flex shrink-0 items-center gap-2">
          <RouterLink :to="withLocalePath(activeLocale, '/cart')" class="hidden rounded-lg border border-orange-200 bg-white px-4 py-3 text-sm font-black text-orange-600 hover:bg-orange-50 sm:block">
            {{ t('nav.cart') }}
          </RouterLink>
          <button
            type="button"
            class="rounded-lg bg-orange-500 px-5 py-3 text-sm font-black text-white shadow-lg shadow-orange-200 transition hover:bg-orange-600 disabled:bg-slate-200 disabled:text-slate-400 disabled:shadow-none"
            :disabled="batchAddDisabled"
            @click="addBatchToCart"
          >
            {{ batchButtonLabel }}
          </button>
        </div>
      </div>
    </div>
  </main>
</template>

<style scoped>
.scrollbar-hide::-webkit-scrollbar {
  display: none;
}

.scrollbar-hide {
  -ms-overflow-style: none;
  scrollbar-width: none;
}

.product-detail-rich :deep(img) {
  display: block;
  height: auto;
  margin-left: auto;
  margin-right: auto;
  max-width: 100%;
}

.product-detail-rich :deep(p),
.product-detail-rich :deep(div),
.product-detail-rich :deep(figure) {
  margin: 0;
  max-width: 100%;
}

.product-detail-rich :deep(h1),
.product-detail-rich :deep(h2),
.product-detail-rich :deep(h3),
.product-detail-rich :deep(h4) {
  color: #0f172a;
  font-weight: 900;
  margin: 1.5rem 0 0.75rem;
}

.product-detail-rich :deep(ul),
.product-detail-rich :deep(ol) {
  margin: 1rem 0;
  padding-left: 1.5rem;
}

.product-detail-rich :deep(ul) {
  list-style: disc;
}

.product-detail-rich :deep(ol) {
  list-style: decimal;
}

.product-detail-rich :deep(li) {
  margin: 0.25rem 0;
}

.product-detail-rich :deep(table) {
  border-collapse: collapse;
  display: block;
  max-width: 100%;
  overflow-x: auto;
  width: 100%;
}

.product-detail-rich :deep(td),
.product-detail-rich :deep(th) {
  border: 1px solid #e2e8f0;
  padding: 0.75rem;
}
</style>
