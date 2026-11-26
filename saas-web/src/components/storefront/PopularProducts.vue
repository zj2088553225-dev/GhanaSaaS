<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { SitePageBlock } from '../../api/sitePages'
import { getPopularProducts, getProducts, type PopularProduct } from '../../api/storefront'
import { storefrontPriceVisible } from '../../config/storefront'
import { formatMoney } from '../../utils/money'
import { imageVariantURL } from '../../utils/imageVariants'
import { buildProductSeoPath } from '../../utils/productSeoUrl'
import { currentLocale, withLocalePath } from '../../utils/localePath'
import ProductImage from '../ProductImage.vue'
import StorefrontProductLink from './StorefrontProductLink.vue'

const props = defineProps<{
  block?: SitePageBlock
}>()

const route = useRoute()
const { t } = useI18n()
const products = ref<PopularProduct[]>([])
const loading = ref(false)
const errorMessage = ref('')
const usingFallback = ref(false)
let requestSequence = 0

const locale = computed(() => currentLocale(route))
const limit = computed(() => {
  const requested = Number(props.block?.limit || 20)
  return Number.isFinite(requested) && requested > 0 ? Math.min(Math.floor(requested), 100) : 20
})
const columnsClass = computed(() => {
  switch (Number(props.block?.columns || 5)) {
    case 6:
      return 'lg:grid-cols-6'
    case 4:
      return 'lg:grid-cols-4'
    default:
      return 'lg:grid-cols-5'
  }
})

watch([locale, limit], () => void loadProducts(), { immediate: true })

async function loadProducts() {
  const sequence = ++requestSequence
  loading.value = true
  errorMessage.value = ''
  usingFallback.value = false
  try {
    const categoryID = Number(props.block?.category_id || 0)
    const result = await getPopularProducts({
      limit: limit.value,
      ...(categoryID > 0 ? { category_id: categoryID } : {}),
    })
    if (sequence === requestSequence) {
      products.value = result
    }
  } catch (error) {
    try {
      const categoryID = Number(props.block?.category_id || 0)
      const fallback = await getProducts({
        sort: 'latest',
        page: 1,
        page_size: limit.value,
        ...(categoryID > 0 ? { category_id: categoryID } : {}),
      })
      if (sequence === requestSequence) {
        products.value = fallback.items
        usingFallback.value = true
      }
    } catch {
      if (sequence === requestSequence) {
        products.value = []
        errorMessage.value = error instanceof Error ? error.message : t('homeBlocks.popularError')
      }
    }
  } finally {
    if (sequence === requestSequence) {
      loading.value = false
    }
  }
}

function productPath(product: PopularProduct) {
  return buildProductSeoPath(product, locale.value)
}

function mallPath() {
  return withLocalePath(locale.value, '/mall')
}

function formatProductPrice(product: PopularProduct) {
  if (product.max_price > product.min_price) {
    return `${formatMoney(product.min_price)} - ${formatMoney(product.max_price)}`
  }
  return formatMoney(product.min_price)
}

function hasOriginalPrice(product: PopularProduct) {
  return product.original_price > product.min_price
}
</script>

<template>
  <section class="mx-auto max-w-screen-2xl px-4 pb-4 sm:px-6 lg:px-8" data-testid="popular-products">
    <div class="rounded-2xl bg-white p-4 shadow-sm sm:p-5">
      <div class="mb-4 flex items-start justify-between gap-3 sm:items-end sm:gap-4">
        <div class="flex min-w-0 items-start gap-3 sm:gap-4">
          <span aria-hidden="true" class="mt-1 h-8 w-1 shrink-0 rounded-full bg-orange-500 sm:h-10" />
          <div class="min-w-0">
            <p class="text-xs font-bold uppercase tracking-[0.22em] text-orange-500">{{ t('homeBlocks.products') }}</p>
            <h2 class="mt-1 text-xl font-black text-slate-950 sm:text-2xl">{{ t('homeBlocks.popularProducts') }}</h2>
            <p class="mt-1 text-sm leading-6 text-slate-500 sm:mt-2">{{ t('homeBlocks.popularProductsSubtitle') }}</p>
          </div>
        </div>
        <RouterLink v-if="props.block?.show_more_link !== false" :to="mallPath()" class="shrink-0 pt-1 text-sm font-black text-orange-600 hover:text-orange-500 sm:pt-0">
          {{ t('homeBlocks.more') }}
        </RouterLink>
      </div>

      <div v-if="loading" class="grid animate-pulse grid-cols-2 gap-3 sm:grid-cols-3" :class="columnsClass" data-testid="popular-loading">
        <div v-for="item in Math.min(limit, 10)" :key="item" class="aspect-[3/4] rounded-xl bg-slate-100" />
      </div>

      <div v-else-if="errorMessage" class="rounded-xl border border-rose-100 bg-rose-50 p-8 text-center" data-testid="popular-error">
        <p class="text-sm font-bold text-rose-700">{{ t('homeBlocks.popularError') }}</p>
        <button type="button" class="mt-4 rounded-lg bg-slate-950 px-4 py-2 text-sm font-black text-white hover:bg-slate-800" @click="loadProducts">
          {{ t('homeBlocks.retry') }}
        </button>
      </div>

      <p v-else-if="products.length === 0" class="rounded-xl bg-slate-50 p-8 text-center text-sm text-slate-500" data-testid="popular-empty">
        {{ t('homeBlocks.noPopularProducts') }}
      </p>

      <div v-else>
        <p v-if="usingFallback" data-testid="popular-fallback" class="mb-3 text-xs font-semibold text-slate-500">
          {{ t('homeBlocks.popularFallback') }}
        </p>
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-3" :class="columnsClass">
          <StorefrontProductLink
          v-for="product in products"
          :key="product.id"
          :to="productPath(product)"
          :product-id="product.id"
          :analytics-source="{ type: 'home_product_floor', id: props.block?.tracking_key || 'popular-products' }"
          page-type="home"
          class="group flex flex-col overflow-hidden rounded-xl border border-slate-100 bg-white shadow-sm transition hover:-translate-y-1 hover:border-orange-100 hover:shadow-xl hover:shadow-slate-200/70"
        >
          <div class="relative aspect-square w-full overflow-hidden bg-slate-100">
            <ProductImage
              :src="imageVariantURL(product.main_image, 'card')"
              :alt="product.name"
              class="h-full w-full"
              image-class="h-full w-full object-cover transition duration-300 group-hover:scale-[1.03]"
            />
            <span class="absolute left-2 top-2 rounded-full bg-slate-950/90 px-2.5 py-1 text-[11px] font-black text-white shadow">
              🔥 {{ t('homeBlocks.popularBadge') }}
            </span>
          </div>
          <div class="flex flex-1 flex-col p-2 sm:p-3">
            <h3 class="line-clamp-2 min-h-[2.5rem] text-xs font-bold leading-5 text-slate-900 sm:text-sm">{{ product.name }}</h3>
            <div class="mt-auto pt-2">
              <template v-if="storefrontPriceVisible">
                <p class="text-sm font-black text-orange-600 sm:text-base">{{ formatProductPrice(product) }}</p>
                <p v-if="hasOriginalPrice(product)" class="mt-0.5 text-xs font-semibold text-slate-400 line-through">
                  {{ formatMoney(product.original_price) }}
                </p>
              </template>
              <p v-else class="min-h-6 text-sm font-black text-orange-600 sm:text-base">{{ t('storefront.contactForPrice') }}</p>
            </div>
          </div>
          </StorefrontProductLink>
        </div>
      </div>
    </div>
  </section>
</template>
