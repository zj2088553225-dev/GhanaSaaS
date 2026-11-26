<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import ProductImage from '../components/ProductImage.vue'
import StorefrontHeader from '../components/storefront/StorefrontHeader.vue'
import { getProductDetail, type ProductDetail, type SKU } from '../api/storefront'
import { useCartStore, type CartLine } from '../stores/cart'
import { useCustomerAuthStore } from '../stores/customerAuth'
import { storefrontAuthEnabled, usePreviewMode } from '../composables/usePreviewMode'
import { storefrontPriceVisible } from '../config/storefront'
import { formatMoney } from '../utils/money'
import { buildProductGallery, skuImageURL } from '../utils/productSku'
import { buildProductSeoPath } from '../utils/productSeoUrl'
import { currentLocale, withLocalePath } from '../utils/localePath.ts'

interface HydratedLine {
  line: CartLine
  product: ProductDetail | null
  sku: SKU | null
  failed: boolean
}

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const cartStore = useCartStore()
const authStore = useCustomerAuthStore()
const { showPreviewMode } = usePreviewMode()

const loading = ref(false)
const details = ref<Record<number, ProductDetail>>({})
const failedProductIds = ref<number[]>([])
const activeLocale = computed(() => currentLocale(route))
let hydrateSequence = 0

const hydratedLines = computed<HydratedLine[]>(() => {
  return cartStore.lines.map((line) => {
    const product = details.value[line.product_id] ?? null
    const failed = failedProductIds.value.includes(line.product_id)
    const sku = product?.skus.find((item) => item.id === line.sku_id) ?? null

    return { line, product, sku, failed }
  })
})

const purchasableLines = computed(() => hydratedLines.value.filter(isPurchasable))
const subtotalCents = computed(() => purchasableLines.value.reduce((total, item) => total + (item.sku?.retail_price ?? 0) * item.line.quantity, 0))
const hasUnavailableLines = computed(() => hydratedLines.value.some((item) => !isPurchasable(item)))

onMounted(async () => {
  await hydrateCart()
})

watch(
  () => cartStore.lines.map((line) => `${line.product_id}:${line.sku_id}:${line.quantity}`).join('|'),
  async () => {
    await hydrateCart()
  },
)

watch(activeLocale, async () => {
  await hydrateCart()
})

async function hydrateCart() {
  const sequence = ++hydrateSequence
  const productIds = Array.from(new Set(cartStore.lines.map((line) => line.product_id)))
  if (!productIds.length) {
    details.value = {}
    failedProductIds.value = []
    loading.value = false
    return
  }

  loading.value = true
  const results = await Promise.all(
    productIds.map(async (productId) => {
      try {
        return { productId, detail: await getProductDetail(productId), failed: false }
      } catch {
        return { productId, detail: null, failed: true }
      }
    }),
  )

  const nextDetails: Record<number, ProductDetail> = {}
  const nextFailedIds: number[] = []

  if (sequence !== hydrateSequence) {
    return
  }

  results.forEach((result) => {
    if (result.detail) {
      nextDetails[result.productId] = result.detail
    }
    if (result.failed) {
      nextFailedIds.push(result.productId)
    }
  })

  details.value = nextDetails
  failedProductIds.value = nextFailedIds
  loading.value = false
}

function isPurchasable(item: HydratedLine) {
  return Boolean(item.product?.status === 'on_shelf' && item.sku && item.line.quantity > 0 && item.line.quantity <= item.sku.sellable_stock)
}

function lineStatus(item: HydratedLine) {
  if (item.failed) {
    return t('cart.loadFailed')
  }
  if (!item.product) {
    return t('cart.loadingLine')
  }
  if (item.product.status !== 'on_shelf') {
    return t('cart.offShelf')
  }
  if (!item.sku) {
    return t('cart.skuUnavailable')
  }
  if (item.line.quantity > item.sku.sellable_stock) {
    return t('cart.stockChanged', { count: item.sku.sellable_stock })
  }
  return ''
}

function lineSubtotal(item: HydratedLine) {
  if (!isPurchasable(item) || !item.sku) {
    return 0
  }

  return item.sku.retail_price * item.line.quantity
}

function productImage(item: HydratedLine) {
  const legacyLine = item.line as CartLine & {
    sku_image_url?: string
    image_url?: string
    product_image?: string
  }
  return (
    skuImageURL(item.sku) ||
    legacyLine.sku_image_url ||
    legacyLine.image_url ||
    legacyLine.product_image ||
    item.product?.main_image ||
    buildProductGallery(item.product)[0] ||
    ''
  )
}

function productDetailPath(item: HydratedLine) {
  return buildProductSeoPath({
    id: item.line.product_id,
    name: item.product?.name,
  }, activeLocale.value)
}

function skuDescription(item: HydratedLine) {
  if (!item.sku) {
    return t('cart.skuUnavailable')
  }
  const specs = Object.entries(item.sku.specs ?? {})
  if (specs.length) {
    return specs.map(([key, value]) => `${key} ${value}`).join(' / ')
  }
  return item.sku.name || t('product.defaultSku')
}

function decrease(line: CartLine) {
  cartStore.setQuantity(line.sku_id, line.quantity - 1)
}

function increase(item: HydratedLine) {
  if (!item.sku || item.line.quantity >= item.sku.sellable_stock) {
    return
  }

  cartStore.setQuantity(item.line.sku_id, item.line.quantity + 1)
}

function normalizeQuantity(line: CartLine, value: number) {
  cartStore.setQuantity(line.sku_id, Number.isInteger(value) ? value : 1)
}

function reduceToStock(item: HydratedLine) {
  if (!item.sku) {
    return
  }

  if (item.sku.sellable_stock <= 0) {
    cartStore.removeItem(item.line.sku_id)
    return
  }

  cartStore.setQuantity(item.line.sku_id, item.sku.sellable_stock)
}

async function goToCheckout() {
  if (!cartStore.lines.length || hasUnavailableLines.value || !purchasableLines.value.length) {
    return
  }

  if (!storefrontAuthEnabled) {
    showPreviewMode()
    return
  }

  if (!authStore.isAuthenticated) {
    await router.push({ path: withLocalePath(currentLocale(route), '/login'), query: { redirect: withLocalePath(currentLocale(route), '/checkout') } })
    return
  }

  await router.push(withLocalePath(currentLocale(route), '/checkout'))
}
</script>

<template>
  <main class="min-h-screen overflow-x-hidden bg-slate-50 pb-24 text-slate-950">
    <StorefrontHeader />

    <section class="mx-auto max-w-7xl px-4 py-5 sm:px-6 lg:px-8">
      <div class="mb-5 rounded-2xl border border-slate-200 bg-white px-4 py-4 shadow-sm sm:px-5">
        <p class="text-xs font-bold uppercase tracking-[0.2em] text-orange-500">{{ t('nav.cart') }}</p>
        <div class="mt-1 flex items-end justify-between gap-3">
          <h1 class="text-2xl font-black tracking-tight text-slate-950 sm:text-3xl">{{ t('cart.title') }}</h1>
          <span class="rounded-full bg-orange-50 px-3 py-1 text-sm font-black text-orange-700">
            {{ cartStore.totalQuantity }}
          </span>
        </div>
      </div>

      <div class="grid gap-5" :class="cartStore.lines.length ? 'lg:grid-cols-[1fr_360px]' : 'grid-cols-1'">
        <div class="space-y-3">
          <div v-if="!cartStore.lines.length" class="rounded-2xl border border-slate-100 bg-white p-8 text-center shadow-sm">
            <h2 class="text-xl font-black">{{ t('cart.empty') }}</h2>
            <p class="mt-2 text-sm text-slate-500">{{ t('cart.emptyHint') }}</p>
            <RouterLink :to="withLocalePath(currentLocale(route), '/mall')" class="mt-5 inline-flex min-h-12 items-center justify-center rounded-2xl bg-orange-500 px-5 py-3 text-sm font-black text-white shadow-lg shadow-orange-200 hover:bg-orange-600">
              {{ t('cart.continueShopping') }}
            </RouterLink>
          </div>

          <div v-else>
            <div v-if="loading" class="mb-3 rounded-2xl border border-slate-100 bg-white p-4 text-center text-sm font-semibold text-slate-500 shadow-sm">
              {{ t('cart.refreshing') }}
            </div>

            <article
              v-for="item in hydratedLines"
              :key="`${item.line.product_id}-${item.line.sku_id}`"
              class="mb-3 rounded-2xl border border-slate-100 bg-white p-3 shadow-sm transition duration-200 hover:-translate-y-0.5 hover:shadow-lg sm:p-4"
            >
              <div class="flex gap-3 sm:gap-4">
              <RouterLink
                :to="productDetailPath(item)"
                class="h-24 w-24 shrink-0 overflow-hidden rounded-2xl bg-slate-100 sm:h-32 sm:w-32"
              >
                <ProductImage :src="productImage(item)" :alt="item.product?.name || t('cart.product')" />
              </RouterLink>

              <div class="min-w-0 flex-1">
                <div class="flex items-start justify-between gap-3">
                  <div class="min-w-0">
                    <RouterLink
                      :to="productDetailPath(item)"
                      class="line-clamp-2 text-sm font-black text-slate-950 hover:text-orange-600 sm:text-base"
                    >
                      {{ item.product?.name || t('cart.product') }}
                    </RouterLink>
                    <p class="mt-1 text-xs font-semibold text-slate-500">
                      {{ skuDescription(item) }}
                    </p>
                  </div>
                  <button
                    type="button"
                    class="rounded-xl px-3 py-2 text-xs font-black text-slate-400 hover:bg-red-50 hover:text-red-600"
                    @click="cartStore.removeItem(item.line.sku_id)"
                  >
                    {{ t('cart.remove') }}
                  </button>
                </div>

                <div class="mt-3 flex flex-wrap items-center gap-2">
                  <span v-if="storefrontPriceVisible && item.sku" class="text-base font-black text-orange-600">{{ formatMoney(item.sku.retail_price) }}</span>
                  <span v-if="item.sku" class="rounded-full bg-slate-100 px-2.5 py-1 text-xs font-bold text-slate-500">
                    {{ t('product.stock') }}: {{ item.sku.sellable_stock }}
                  </span>
                  <span v-if="lineStatus(item)" class="rounded-full bg-red-50 px-2.5 py-1 text-xs font-bold text-red-600">
                    {{ lineStatus(item) }}
                  </span>
                </div>

                <div class="mt-4 flex flex-wrap items-center justify-between gap-3">
                  <div class="flex items-center rounded-2xl border border-slate-200 bg-white p-1">
                    <button type="button" class="h-10 w-10 rounded-xl text-lg font-black hover:bg-slate-100 disabled:text-slate-300" :disabled="item.line.quantity <= 1" @click="decrease(item.line)">
                      −
                    </button>
                    <input
                      :value="item.line.quantity"
                      type="number"
                      min="1"
                      class="h-10 w-16 border-0 text-center text-sm font-black outline-none"
                      @change="normalizeQuantity(item.line, Number(($event.target as HTMLInputElement).value))"
                    />
                    <button
                      type="button"
                      class="h-10 w-10 rounded-xl text-lg font-black hover:bg-slate-100 disabled:text-slate-300"
                      :disabled="!item.sku || item.line.quantity >= item.sku.sellable_stock"
                      @click="increase(item)"
                    >
                      +
                    </button>
                  </div>

                  <div v-if="storefrontPriceVisible" class="text-right">
                    <p class="text-xs font-semibold text-slate-500">{{ t('cart.lineSubtotal') }}</p>
                    <p class="text-base font-black text-slate-950">{{ formatMoney(lineSubtotal(item)) }}</p>
                  </div>
                </div>

                <button
                  v-if="item.sku && item.line.quantity > item.sku.sellable_stock"
                  type="button"
                  class="mt-3 min-h-11 rounded-2xl bg-orange-50 px-4 py-2 text-sm font-black text-orange-700 hover:bg-orange-100"
                  @click="reduceToStock(item)"
                >
                  {{ t('cart.reduceToStock') }}
                </button>
              </div>
              </div>
            </article>
          </div>
        </div>

        <aside v-if="cartStore.lines.length" class="h-fit rounded-2xl border border-slate-100 bg-white p-5 shadow-sm lg:sticky lg:top-24">
          <div class="flex items-center justify-between">
            <h2 class="text-xl font-black">{{ t('cart.summary') }}</h2>
            <button type="button" class="rounded-xl px-3 py-2 text-xs font-black text-slate-400 hover:bg-red-50 hover:text-red-600" @click="cartStore.clearCart">
              {{ t('cart.clear') }}
            </button>
          </div>

          <div class="mt-5 space-y-3 border-t border-slate-100 pt-5">
            <div class="flex items-center justify-between text-sm font-bold text-slate-600">
              <span>{{ t('cart.validItems') }}</span>
              <span>{{ purchasableLines.length }}</span>
            </div>
            <div v-if="storefrontPriceVisible" class="flex items-center justify-between text-lg font-black text-slate-950">
              <span>{{ t('cart.subtotal') }}</span>
              <span>{{ formatMoney(subtotalCents) }}</span>
            </div>
          </div>

          <p v-if="hasUnavailableLines" class="mt-4 rounded-2xl bg-amber-50 px-4 py-3 text-sm font-bold leading-6 text-amber-700">
            {{ t('cart.unavailableNotice') }}
          </p>

          <button
            type="button"
            class="mt-5 flex min-h-14 w-full items-center justify-center rounded-2xl px-5 py-4 text-base font-black shadow-lg transition enabled:bg-orange-500 enabled:text-white enabled:shadow-orange-200 enabled:hover:bg-orange-600 disabled:cursor-not-allowed disabled:bg-slate-200 disabled:text-slate-400 disabled:shadow-none"
            :disabled="hasUnavailableLines || !purchasableLines.length"
            @click="goToCheckout"
          >
            {{ t('cart.checkout') }}
          </button>

          <RouterLink :to="withLocalePath(currentLocale(route), '/mall')" class="mt-3 flex min-h-12 w-full items-center justify-center rounded-2xl border border-slate-200 px-5 py-3 text-sm font-black text-slate-700 hover:bg-slate-50">
            {{ t('cart.continueShopping') }}
          </RouterLink>
        </aside>
      </div>
    </section>
  </main>
</template>
