<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { checkout, type FulfillmentMethod, type Order } from '../api/orders'
import StorefrontHeader from '../components/storefront/StorefrontHeader.vue'
import PreviewModeNotice from '../components/storefront/PreviewModeNotice.vue'
import { getProductDetail, type ProductDetail } from '../api/storefront'
import { useCartStore, type CartLine } from '../stores/cart'
import { storefrontPriceVisible } from '../config/storefront'
import { formatMoney } from '../utils/money'
import { storefrontAuthEnabled, usePreviewMode } from '../composables/usePreviewMode'
import { currentLocale, withLocalePath } from '../utils/localePath.ts'

interface CheckoutLine {
  line: CartLine
  product: ProductDetail | null
}

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const cartStore = useCartStore()
const { showPreviewMode } = usePreviewMode()
const loading = ref(false)
const submitting = ref(false)
const errorMessage = ref('')
const products = ref<Record<number, ProductDetail>>({})
const activeLocale = computed(() => currentLocale(route))
let hydrateSequence = 0
const form = reactive({
  contact_name: '',
  contact_phone: '',
  address: '',
  fulfillment_method: 'pickup' as FulfillmentMethod,
})

const lines = computed<CheckoutLine[]>(() =>
  cartStore.lines.map((line) => ({
    line,
    product: products.value[line.product_id] ?? null,
  })),
)

const subtotalCents = computed(() =>
  lines.value.reduce((total, item) => {
    const sku = item.product?.skus.find((candidate) => candidate.id === item.line.sku_id)
    return total + (sku?.retail_price ?? 0) * item.line.quantity
  }, 0),
)

onMounted(async () => {
  if (!storefrontAuthEnabled) {
    return
  }
  if (!cartStore.lines.length) {
    await router.replace(withLocalePath(activeLocale.value, '/cart'))
    return
  }
  await hydrateProducts()
})

watch(activeLocale, async () => {
  if (!storefrontAuthEnabled || !cartStore.lines.length) {
    return
  }
  await hydrateProducts()
})

async function hydrateProducts() {
  const sequence = ++hydrateSequence
  loading.value = true
  errorMessage.value = ''
  try {
    const productIds = Array.from(new Set(cartStore.lines.map((line) => line.product_id)))
    const details = await Promise.all(productIds.map((productID) => getProductDetail(productID)))
    if (sequence !== hydrateSequence) {
      return
    }
    products.value = Object.fromEntries(details.map((detail) => [detail.id, detail]))
  } catch (error) {
    if (sequence !== hydrateSequence) {
      return
    }
    errorMessage.value = error instanceof Error ? error.message : t('checkout.refreshFailed')
  } finally {
    if (sequence === hydrateSequence) {
      loading.value = false
    }
  }
}

function lineName(item: CheckoutLine) {
  const sku = item.product?.skus.find((candidate) => candidate.id === item.line.sku_id)
  if (!item.product || !sku) {
    return t('checkout.skuFallback', { id: item.line.sku_id })
  }
  return `${item.product.name} / ${sku.name}`
}

function lineSubtotal(item: CheckoutLine) {
  const sku = item.product?.skus.find((candidate) => candidate.id === item.line.sku_id)
  return (sku?.retail_price ?? 0) * item.line.quantity
}

function validateForm() {
  if (!form.contact_name.trim() || !form.contact_phone.trim() || !form.address.trim()) {
    errorMessage.value = t('checkout.invalidForm')
    return false
  }
  if (!cartStore.lines.length) {
    errorMessage.value = t('checkout.emptyCart')
    return false
  }
  return true
}

async function submitOrder() {
  if (!storefrontAuthEnabled) {
    showPreviewMode()
    return
  }
  if (!validateForm()) {
    return
  }

  submitting.value = true
  errorMessage.value = ''
  try {
    const order: Order = await checkout({
      contact_name: form.contact_name.trim(),
      contact_phone: form.contact_phone.trim(),
      address: form.address.trim(),
      fulfillment_method: form.fulfillment_method,
      items: cartStore.lines.map((line) => ({
        sku_id: line.sku_id,
        quantity: line.quantity,
      })),
    })
    cartStore.clearCart()
    await router.replace({ path: withLocalePath(activeLocale.value, '/orders/success'), query: { order_no: order.order_no } })
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : t('checkout.submitFailed')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <main class="min-h-screen overflow-x-hidden bg-slate-50 pb-20 text-slate-950">
    <StorefrontHeader />

    <PreviewModeNotice v-if="!storefrontAuthEnabled" />

    <section v-else class="mx-auto grid max-w-7xl gap-5 px-4 py-5 sm:px-6 lg:grid-cols-[1fr_380px] lg:px-8">
      <div class="rounded-2xl border border-slate-200 bg-white px-4 py-4 shadow-sm sm:px-5 lg:col-span-2">
        <RouterLink :to="withLocalePath(currentLocale(route), '/cart')" class="text-sm font-bold text-orange-600 hover:text-orange-700">{{ t('nav.backToCart') }}</RouterLink>
        <p class="mt-3 text-xs font-bold uppercase tracking-[0.2em] text-orange-500">{{ t('checkout.eyebrow') }}</p>
        <h1 class="mt-1 text-2xl font-black tracking-tight text-slate-950 sm:text-3xl">{{ t('checkout.title') }}</h1>
      </div>

      <div class="space-y-4">
        <div class="rounded-3xl bg-white p-5 shadow-sm">
          <h2 class="text-xl font-black">{{ t('checkout.shippingInfo') }}</h2>
          <form class="mt-5 space-y-4" @submit.prevent="submitOrder">
            <label class="block">
              <span class="text-sm font-black text-slate-700">{{ t('checkout.contactName') }}</span>
              <input v-model="form.contact_name" class="mt-2 h-12 w-full rounded-2xl border border-slate-200 px-4 text-sm font-semibold outline-none focus:border-orange-400" :placeholder="t('checkout.contactNamePlaceholder')" />
            </label>
            <label class="block">
              <span class="text-sm font-black text-slate-700">{{ t('checkout.contactPhone') }}</span>
              <input v-model="form.contact_phone" class="mt-2 h-12 w-full rounded-2xl border border-slate-200 px-4 text-sm font-semibold outline-none focus:border-orange-400" :placeholder="t('checkout.contactPhonePlaceholder')" />
            </label>
            <fieldset>
              <legend class="text-sm font-black text-slate-700">{{ t('checkout.fulfillmentMethod') }}</legend>
              <div class="mt-2 grid gap-2 sm:grid-cols-2">
                <label class="flex min-h-14 cursor-pointer items-center gap-3 rounded-2xl border border-slate-200 px-4 py-3 text-sm font-bold text-slate-700 has-[:checked]:border-orange-400 has-[:checked]:bg-orange-50 has-[:checked]:text-orange-800">
                  <input v-model="form.fulfillment_method" type="radio" name="fulfillment_method" value="pickup" class="h-4 w-4 accent-orange-500" />
                  <span>{{ t('checkout.pickupOption') }}</span>
                </label>
                <label class="flex min-h-14 cursor-pointer items-center gap-3 rounded-2xl border border-slate-200 px-4 py-3 text-sm font-bold text-slate-700 has-[:checked]:border-orange-400 has-[:checked]:bg-orange-50 has-[:checked]:text-orange-800">
                  <input v-model="form.fulfillment_method" type="radio" name="fulfillment_method" value="delivery" class="h-4 w-4 accent-orange-500" />
                  <span>{{ t('checkout.deliveryOption') }}</span>
                </label>
              </div>
            </fieldset>
            <label class="block">
              <span class="text-sm font-black text-slate-700">{{ t('checkout.address') }}</span>
              <textarea v-model="form.address" class="mt-2 min-h-28 w-full rounded-2xl border border-slate-200 px-4 py-3 text-sm font-semibold outline-none focus:border-orange-400" :placeholder="t('checkout.addressPlaceholder')" />
            </label>
          </form>
        </div>

        <div class="rounded-3xl bg-white p-5 shadow-sm">
          <div class="flex items-center justify-between">
            <h2 class="text-xl font-black">{{ t('checkout.itemList') }}</h2>
            <button type="button" class="rounded-xl px-3 py-2 text-xs font-black text-slate-500 hover:bg-slate-100" @click="hydrateProducts">{{ t('checkout.refreshItems') }}</button>
          </div>

          <div v-if="loading" class="mt-5 rounded-2xl bg-slate-50 p-5 text-center text-sm font-bold text-slate-500">{{ t('checkout.refreshingItems') }}</div>
          <div v-else class="mt-5 divide-y divide-slate-100">
            <div v-for="item in lines" :key="`${item.line.product_id}-${item.line.sku_id}`" class="flex items-center justify-between gap-4 py-4">
              <div class="min-w-0">
                <p class="line-clamp-2 text-sm font-black">{{ lineName(item) }}</p>
                <p class="mt-1 text-xs font-bold text-slate-500">{{ t('checkout.quantity', { count: item.line.quantity }) }}</p>
              </div>
              <p v-if="storefrontPriceVisible" class="shrink-0 text-sm font-black text-orange-600">{{ formatMoney(lineSubtotal(item)) }}</p>
            </div>
          </div>
        </div>
      </div>

      <aside class="h-fit rounded-3xl bg-white p-5 shadow-sm lg:sticky lg:top-24">
        <h2 class="text-xl font-black">{{ t('checkout.orderSummary') }}</h2>
        <div v-if="storefrontPriceVisible" class="mt-5 flex items-center justify-between border-t border-slate-100 pt-5 text-lg font-black">
          <span>{{ t('cart.subtotal') }}</span>
          <span>{{ formatMoney(subtotalCents) }}</span>
        </div>
        <p class="mt-3 text-xs font-semibold leading-5 text-slate-500">{{ t('checkout.finalNotice') }}</p>
        <p v-if="errorMessage" class="mt-4 rounded-2xl bg-red-50 px-4 py-3 text-sm font-bold text-red-600">{{ errorMessage }}</p>
        <button type="button" class="mt-5 flex min-h-14 w-full items-center justify-center rounded-2xl bg-orange-500 px-5 py-4 text-base font-black text-white shadow-lg shadow-orange-200 hover:bg-orange-600 disabled:cursor-not-allowed disabled:bg-slate-300 disabled:shadow-none" :disabled="submitting || loading || !cartStore.lines.length" @click="submitOrder">
          {{ submitting ? t('checkout.submitting') : t('checkout.submit') }}
        </button>
      </aside>
    </section>
  </main>
</template>
