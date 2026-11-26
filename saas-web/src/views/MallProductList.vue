<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ArrowDown } from '@element-plus/icons-vue'
import ProductImage from '../components/ProductImage.vue'
import StorefrontHeader from '../components/storefront/StorefrontHeader.vue'
import StorefrontProductLink from '../components/storefront/StorefrontProductLink.vue'
import { getStorefrontPlatformCategories, type PlatformCategory } from '../api/platformCategories'
import { getProducts, type ProductListItem, type ProductQueryParams } from '../api/storefront'
import { storefrontPriceVisible } from '../config/storefront'
import { formatMoney } from '../utils/money'
import { mallSeoMeta, useSeoMeta } from '../composables/useSeoMeta'
import { imageVariantURL } from '../utils/imageVariants'
import { buildProductSeoPath } from '../utils/productSeoUrl'
import { currentLocale, withLocalePath } from '../utils/localePath.ts'
import type { StorefrontAnalyticsSource, StorefrontPageType } from '../utils/storefrontAnalytics.ts'
import {
  mallSourceOptions,
  mallSourceQueryValue,
  mallSourceRouteQuery,
  normalizeMallSource,
  type MallSource,
} from '../utils/mallSource'

type MallSort = 'comprehensive' | 'latest' | 'hot' | 'price_asc' | 'price_desc'

interface CategoryOption {
  category: PlatformCategory
  depth: number
}

const PAGE_SIZE = 30
const MALL_CATEGORY_MAX_LEVEL = 2

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const activeLocale = computed(() => currentLocale(route))

const platformCategories = ref<PlatformCategory[]>([])
const products = ref<ProductListItem[]>([])
const total = ref(0)
const loading = ref(true)
const errorMessage = ref('')
const expandedPlatformCategoryId = ref<number | null>(null)
let loadSequence = 0
let categoryLoadSequence = 0

const keyword = computed(() => queryStringValue(route.query.keyword).trim())
const selectedPlatformCategoryId = computed(() => positiveInteger(route.query.platform_category_id))
const currentSource = computed(() => normalizeMallSource(route.query.source))
const productAnalyticsSource = computed<StorefrontAnalyticsSource>(() => {
  if (keyword.value) {
    return { type: 'search', id: 'results' }
  }
  if (selectedPlatformCategoryId.value) {
    return { type: 'category', id: String(selectedPlatformCategoryId.value) }
  }
  return { type: 'mall', id: currentSource.value }
})
const productAnalyticsPageType = computed<StorefrontPageType>(() => (keyword.value ? 'search' : 'mall'))
useSeoMeta(computed(() => mallSeoMeta(currentSource.value, activeLocale.value)))
const currentPage = computed(() => positiveInteger(route.query.page) ?? 1)
const currentSort = computed<MallSort>(() => {
  const value = queryStringValue(route.query.sort)
  const allowedSorts = storefrontPriceVisible ? ['latest', 'hot', 'price_asc', 'price_desc'] : ['latest', 'hot']
  return allowedSorts.includes(value) ? (value as MallSort) : 'comprehensive'
})
const isResultMode = computed(() => Boolean(keyword.value || selectedPlatformCategoryId.value || currentSource.value !== 'all'))
const categoryOptions = computed(() => flattenCategories(platformCategories.value))
const rootCategoryOptions = computed(() => sortedCategories(platformCategories.value))
const selectedCategory = computed(() => findCategory(platformCategories.value, selectedPlatformCategoryId.value))
const selectedRootPlatformCategoryId = computed(() => findRootCategoryId(platformCategories.value, selectedPlatformCategoryId.value))
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))
const sortOptions = computed(() =>
  storefrontPriceVisible
    ? (['comprehensive', 'latest', 'hot', 'price'] as const)
    : (['comprehensive', 'latest', 'hot'] as const),
)
const sourceTabs = computed(() =>
  mallSourceOptions.map((source) => ({
    source,
    label: t(`mall.sourceOptions.${source}`),
  })),
)
const pageTitle = computed(() => {
  if (keyword.value) {
    return t('mall.searchResults', { keyword: keyword.value })
  }
  if (selectedPlatformCategoryId.value) {
    return t('mall.categoryProducts')
  }
  if (currentSource.value !== 'all') {
    return t(`mall.sourceOptions.${currentSource.value}`)
  }
  return t('mall.allProducts')
})

onMounted(() => {
  void loadPlatformCategories()
})

watch(activeLocale, () => {
  void loadPlatformCategories()
  void loadProducts()
})

watch(
  [selectedRootPlatformCategoryId, () => platformCategories.value],
  ([rootCategoryId]) => {
    expandedPlatformCategoryId.value = rootCategoryId ?? null
  },
  { immediate: true },
)

watch(
  [
    () => route.query.keyword,
    () => route.query.platform_category_id,
    () => route.query.source,
    () => route.query.sort,
    () => route.query.page,
  ],
  () => {
    void loadProducts()
  },
  { immediate: true },
)

function queryStringValue(value: unknown) {
  if (Array.isArray(value)) {
    return typeof value[0] === 'string' ? value[0] : ''
  }
  return typeof value === 'string' ? value : ''
}

function positiveInteger(value: unknown) {
  const parsed = Number(queryStringValue(value))
  return Number.isInteger(parsed) && parsed > 0 ? parsed : undefined
}

function isLegacyDefaultCategory(category: PlatformCategory) {
  return category.name.trim().toLowerCase() === 'default category'
}

function removeLegacyDefaultCategories(categories: PlatformCategory[]): PlatformCategory[] {
  return sortedCategories(categories)
    .filter((category) => !isLegacyDefaultCategory(category))
    .map((category) => ({
      ...category,
      children: removeLegacyDefaultCategories(category.children ?? []),
    }))
}

function sortedCategories(categories: PlatformCategory[]): PlatformCategory[] {
  return [...categories].sort((a, b) => a.sort_order - b.sort_order || a.id - b.id)
}

function flattenCategories(categories: PlatformCategory[], depth = 0): CategoryOption[] {
  return sortedCategories(categories)
    .flatMap((category) => {
      const option = { category, depth }
      if (depth >= MALL_CATEGORY_MAX_LEVEL - 1) {
        return [option]
      }
      return [
        option,
        ...flattenCategories(category.children ?? [], depth + 1),
      ]
    })
}

function findCategory(categories: PlatformCategory[], id?: number): PlatformCategory | undefined {
  if (!id) {
    return undefined
  }
  for (const category of categories) {
    if (category.id === id) {
      return category
    }
    const child = findCategory(category.children ?? [], id)
    if (child) {
      return child
    }
  }
  return undefined
}

function findRootCategoryId(categories: PlatformCategory[], id?: number): number | null {
  if (!id) {
    return null
  }
  for (const category of categories) {
    if (category.id === id) {
      return category.id
    }
    if (findCategory(category.children ?? [], id)) {
      return category.id
    }
  }
  return null
}

function childCategories(category: PlatformCategory) {
  return sortedCategories(category.children ?? [])
}

function hasSecondLevelCategories(category: PlatformCategory) {
  return childCategories(category).length > 0
}

function isCategoryExpanded(category: PlatformCategory) {
  return expandedPlatformCategoryId.value === category.id
}

function isCategoryActive(category: PlatformCategory) {
  return selectedPlatformCategoryId.value === category.id
}

function toggleCategoryExpansion(category: PlatformCategory) {
  expandedPlatformCategoryId.value = isCategoryExpanded(category) ? null : category.id
}

function collapseAllCategories() {
  expandedPlatformCategoryId.value = null
}

function apiSort(): ProductQueryParams['sort'] {
  return currentSort.value === 'comprehensive' ? undefined : currentSort.value
}

function routeQuery(options: {
  platformCategoryId?: number
  source?: MallSource
  sort?: MallSort
  page?: number
} = {}) {
  const categoryId = Object.prototype.hasOwnProperty.call(options, 'platformCategoryId')
    ? options.platformCategoryId
    : selectedPlatformCategoryId.value
  const source = options.source ?? currentSource.value
  const sort = options.sort ?? currentSort.value
  const page = options.page ?? currentPage.value
  const query = mallSourceRouteQuery({
    keyword: keyword.value,
    platformCategoryId: categoryId,
    sort,
  }, source)
  if (page > 1) {
    query.page = String(page)
  }
  return query
}

async function loadPlatformCategories() {
  const sequence = ++categoryLoadSequence
  try {
    const categories = await getStorefrontPlatformCategories({ maxLevel: MALL_CATEGORY_MAX_LEVEL })
    if (sequence !== categoryLoadSequence) {
      return
    }
    platformCategories.value = removeLegacyDefaultCategories(categories)
  } catch {
    if (sequence !== categoryLoadSequence) {
      return
    }
    platformCategories.value = []
  }
}

async function loadProducts() {
  const sequence = ++loadSequence
  loading.value = true
  errorMessage.value = ''

  try {
    const page = await getProducts({
      page: currentPage.value,
      page_size: PAGE_SIZE,
      keyword: keyword.value || undefined,
      platform_category_id: selectedPlatformCategoryId.value,
      source: mallSourceQueryValue(currentSource.value),
      sort: apiSort(),
    })
    if (sequence !== loadSequence) {
      return
    }
    products.value = page.items
    total.value = page.total
  } catch (error) {
    if (sequence !== loadSequence) {
      return
    }
    products.value = []
    total.value = 0
    errorMessage.value = error instanceof Error ? error.message : t('mall.error')
  } finally {
    if (sequence === loadSequence) {
      loading.value = false
    }
  }
}

async function selectCategory(platformCategoryId?: number) {
  await router.push({
    path: withLocalePath(activeLocale.value, '/mall'),
    query: routeQuery({ platformCategoryId, page: 1 }),
  })
}

async function selectSource(source: MallSource) {
  if (source === currentSource.value) {
    return
  }
  await router.push({
    path: withLocalePath(activeLocale.value, '/mall'),
    query: routeQuery({ source, page: 1 }),
  })
}

async function selectSort(sort: 'comprehensive' | 'latest' | 'hot' | 'price') {
  let nextSort: MallSort
  if (sort === 'price') {
    if (!storefrontPriceVisible) {
      return
    }
    nextSort = currentSort.value === 'price_asc' ? 'price_desc' : 'price_asc'
  } else {
    nextSort = sort
  }
  await router.push({
    path: withLocalePath(activeLocale.value, '/mall'),
    query: routeQuery({ sort: nextSort, page: 1 }),
  })
}

function isSourceActive(source: MallSource) {
  return currentSource.value === source
}

function isSortActive(sort: 'comprehensive' | 'latest' | 'hot' | 'price') {
  if (sort === 'price') {
    return currentSort.value === 'price_asc' || currentSort.value === 'price_desc'
  }
  return currentSort.value === sort
}

function productPriceLabel(product: ProductListItem) {
  if (product.display_price === null) {
    return t('mall.priceUnavailable')
  }
  if (product.max_price > product.display_price) {
    return `${formatMoney(product.display_price)} - ${formatMoney(product.max_price)}`
  }
  return formatMoney(product.display_price)
}

function hasOriginalPrice(product: ProductListItem) {
  return product.display_price !== null && product.original_price > product.display_price
}

async function changePage(page: number) {
  if (page < 1 || page > pageCount.value || page === currentPage.value) {
    return
  }
  await router.push({
    path: withLocalePath(activeLocale.value, '/mall'),
    query: routeQuery({ page }),
  })
  window.scrollTo({ top: 0, behavior: 'smooth' })
}
</script>

<template>
  <main class="min-h-screen bg-slate-50 pb-16 text-slate-950">
    <StorefrontHeader />

    <section class="mx-auto max-w-7xl px-4 py-5 sm:px-6 lg:px-8">
      <div class="rounded-2xl border border-slate-200 bg-white px-4 py-4 shadow-sm sm:px-5">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div class="min-w-0">
            <p v-if="isResultMode" class="text-xs font-bold uppercase tracking-[0.2em] text-orange-500">
              {{ t('mall.filteredProducts') }}
            </p>
            <h1 class="mt-1 truncate text-2xl font-black tracking-tight text-slate-950 sm:text-3xl">
              {{ pageTitle }}
            </h1>
            <div class="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-slate-500">
              <span>{{ t('mall.total', { total }) }}</span>
              <span v-if="selectedCategory" class="rounded-full bg-orange-50 px-2.5 py-1 font-semibold text-orange-700">
                {{ t('mall.categoryLabel', { category: selectedCategory.name }) }}
              </span>
            </div>
          </div>

          <div class="flex min-w-0 items-center gap-1 overflow-x-auto rounded-xl bg-slate-50 p-1 text-sm font-bold">
            <span class="shrink-0 px-2 text-xs text-slate-400">{{ t('mall.sort') }}</span>
            <button
              v-for="option in sortOptions"
              :key="option"
              type="button"
              class="shrink-0 rounded-lg px-3 py-2 transition"
              :class="isSortActive(option) ? 'bg-white text-orange-600 shadow-sm' : 'text-slate-600 hover:text-orange-600'"
              @click="selectSort(option)"
            >
              {{ t(`mall.sortOptions.${option}`) }}
              <span v-if="option === 'price' && currentSort === 'price_asc'">↑</span>
              <span v-else-if="option === 'price' && currentSort === 'price_desc'">↓</span>
            </button>
          </div>
        </div>

        <div class="mt-4 flex flex-wrap gap-2 border-t border-slate-100 pt-4">
          <button
            v-for="tab in sourceTabs"
            :key="tab.source"
            type="button"
            class="rounded-full border px-3.5 py-2 text-xs font-black transition sm:px-4 sm:text-sm"
            :class="isSourceActive(tab.source) ? 'border-orange-200 bg-orange-50 text-orange-700' : 'border-slate-200 bg-white text-slate-600 hover:border-orange-200 hover:text-orange-600'"
            @click="selectSource(tab.source)"
          >
            {{ tab.label }}
          </button>
        </div>
      </div>

      <div v-if="categoryOptions.length" class="mt-3 md:hidden">
        <div class="scrollbar-hide flex max-w-full gap-2 overflow-x-auto pb-1">
          <button
            type="button"
            class="shrink-0 rounded-full border px-3.5 py-2 text-xs font-bold transition"
            :class="selectedPlatformCategoryId ? 'border-slate-200 bg-white text-slate-600' : 'border-orange-200 bg-orange-50 text-orange-700'"
            @click="selectCategory(undefined)"
          >
            {{ t('mall.allCategories') }}
          </button>
          <button
            v-for="{ category } in categoryOptions"
            :key="category.id"
            type="button"
            class="shrink-0 rounded-full border px-3.5 py-2 text-xs font-bold transition"
            :class="selectedPlatformCategoryId === category.id ? 'border-orange-200 bg-orange-50 text-orange-700' : 'border-slate-200 bg-white text-slate-600'"
            @click="selectCategory(category.id)"
          >
            {{ category.name }}
          </button>
        </div>
      </div>

      <div
        class="mt-4 grid gap-4"
        :class="rootCategoryOptions.length ? 'md:grid-cols-[230px_minmax(0,1fr)]' : 'grid-cols-1'"
      >
        <aside v-if="rootCategoryOptions.length" class="hidden md:block">
          <div class="sticky top-24 overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm">
            <div class="flex items-center justify-between gap-3 border-b border-slate-100 px-4 py-3">
              <h2 class="text-sm font-black text-slate-950">{{ t('mall.categoryFilter') }}</h2>
              <button
                v-if="expandedPlatformCategoryId"
                type="button"
                class="shrink-0 text-xs font-bold text-slate-500 transition hover:text-orange-600"
                @click="collapseAllCategories"
              >
                {{ t('mall.collapseAll') }}
              </button>
            </div>
            <div class="max-h-[calc(100vh-9rem)] overflow-y-auto p-2">
              <button
                type="button"
                class="mb-1 w-full rounded-xl px-3 py-2.5 text-left text-sm font-bold transition"
                :class="selectedPlatformCategoryId ? 'text-slate-600 hover:bg-slate-50' : 'bg-orange-50 text-orange-700'"
                @click="selectCategory(undefined)"
              >
                {{ t('mall.allCategories') }}
              </button>
              <div v-for="category in rootCategoryOptions" :key="category.id" class="mb-1">
                <div
                  class="flex min-w-0 items-stretch rounded-xl transition"
                  :class="isCategoryActive(category) ? 'bg-orange-50 text-orange-700' : isCategoryExpanded(category) ? 'bg-slate-50 text-slate-800' : 'text-slate-600 hover:bg-slate-50 hover:text-slate-950'"
                >
                  <button
                    type="button"
                    class="min-w-0 flex-1 truncate rounded-l-xl px-3 py-2.5 text-left text-sm font-bold"
                    @click="selectCategory(category.id)"
                  >
                    {{ category.name }}
                  </button>
                  <button
                    v-if="hasSecondLevelCategories(category)"
                    type="button"
                    class="flex w-9 shrink-0 items-center justify-center rounded-r-xl transition hover:bg-orange-50 hover:text-orange-700"
                    :aria-label="isCategoryExpanded(category) ? t('mall.collapseCategory') : t('mall.expandCategory')"
                    :aria-expanded="isCategoryExpanded(category)"
                    @click.stop="toggleCategoryExpansion(category)"
                  >
                    <el-icon class="transition-transform duration-200" :class="isCategoryExpanded(category) ? 'rotate-180' : ''">
                      <ArrowDown />
                    </el-icon>
                  </button>
                </div>
                <div
                  v-if="isCategoryExpanded(category)"
                  class="ml-2 mt-1 max-h-56 overflow-y-auto border-l border-slate-100 py-1 pl-2 pr-1"
                >
                  <button
                    v-for="child in childCategories(category)"
                    :key="child.id"
                    type="button"
                    class="mb-1 w-full truncate rounded-lg px-3 py-2 text-left text-xs font-semibold transition"
                    :class="isCategoryActive(child) ? 'bg-orange-50 text-orange-700' : 'text-slate-500 hover:bg-slate-50 hover:text-slate-900'"
                    @click="selectCategory(child.id)"
                  >
                    {{ child.name }}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </aside>

        <div class="min-w-0">
          <div v-if="loading" class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
            <div v-for="index in 10" :key="index" class="overflow-hidden rounded-2xl border border-slate-100 bg-white">
              <div class="aspect-square animate-pulse bg-slate-200" />
              <div class="space-y-3 p-3">
                <div class="h-4 animate-pulse rounded bg-slate-200" />
                <div class="h-4 w-2/3 animate-pulse rounded bg-slate-100" />
                <div class="h-5 w-1/2 animate-pulse rounded bg-orange-100" />
              </div>
            </div>
          </div>

          <div v-else-if="errorMessage" class="rounded-2xl border border-red-100 bg-red-50 p-8 text-center text-sm font-semibold text-red-600">
            {{ errorMessage }}
          </div>

          <div v-else-if="products.length" class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
            <StorefrontProductLink
              v-for="product in products"
              :key="product.id"
              :to="buildProductSeoPath(product, activeLocale)"
              :product-id="product.id"
              :analytics-source="productAnalyticsSource"
              :page-type="productAnalyticsPageType"
              class="group flex min-w-0 flex-col overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm transition duration-200 hover:-translate-y-0.5 hover:border-orange-300 hover:shadow-lg"
            >
              <div class="aspect-square w-full shrink-0 overflow-hidden bg-slate-100">
                <ProductImage
                  :src="imageVariantURL(product.display_image, 'card')"
                  :alt="product.display_name"
                  loading="lazy"
                  class="h-full w-full"
                  image-class="h-full w-full object-cover transition duration-300 group-hover:scale-[1.04]"
                />
              </div>
              <div class="flex min-h-[108px] flex-1 flex-col p-3">
                <h2 class="line-clamp-2 min-h-10 text-sm font-semibold leading-5 text-slate-800 group-hover:text-orange-700">
                  {{ product.display_name }}
                </h2>
                <div class="mt-auto pt-2">
                  <template v-if="storefrontPriceVisible">
                    <p
                      class="text-base font-black leading-6"
                      :class="product.display_price === null ? 'text-slate-400' : 'text-orange-600'"
                    >
                      {{ productPriceLabel(product) }}
                    </p>
                    <p
                      v-if="hasOriginalPrice(product)"
                      class="mt-0.5 text-xs font-medium text-slate-400 line-through"
                    >
                      {{ formatMoney(product.original_price) }}
                    </p>
                  </template>
                  <p v-else class="min-h-6 text-base font-black leading-6 text-orange-600">
                    {{ t('storefront.contactForPrice') }}
                  </p>
                </div>
              </div>
            </StorefrontProductLink>
          </div>

          <div v-else class="rounded-2xl border border-dashed border-slate-200 bg-white px-6 py-16 text-center">
            <div class="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-orange-50 text-2xl">⌕</div>
            <h2 class="mt-4 text-lg font-black text-slate-900">{{ t('mall.empty') }}</h2>
            <p class="mt-1 text-sm text-slate-500">{{ t('mall.emptyHint') }}</p>
          </div>

          <nav v-if="!loading && !errorMessage && pageCount > 1" class="mt-6 flex items-center justify-center gap-3" :aria-label="t('mall.pagination')">
            <button
              type="button"
              class="rounded-xl border border-slate-200 bg-white px-4 py-2 text-sm font-bold text-slate-700 transition hover:border-orange-200 hover:text-orange-600 disabled:cursor-not-allowed disabled:opacity-40"
              :disabled="currentPage <= 1"
              @click="changePage(currentPage - 1)"
            >
              {{ t('mall.previousPage') }}
            </button>
            <span class="text-sm font-semibold text-slate-500">
              {{ t('mall.pageStatus', { page: currentPage, pages: pageCount }) }}
            </span>
            <button
              type="button"
              class="rounded-xl border border-slate-200 bg-white px-4 py-2 text-sm font-bold text-slate-700 transition hover:border-orange-200 hover:text-orange-600 disabled:cursor-not-allowed disabled:opacity-40"
              :disabled="currentPage >= pageCount"
              @click="changePage(currentPage + 1)"
            >
              {{ t('mall.nextPage') }}
            </button>
          </nav>
        </div>
      </div>
    </section>
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
</style>
