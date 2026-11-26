<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  getStorefrontOrders,
  type Order,
  type OrderItem,
  type OrderStatus,
  type PaymentStatus,
} from '../../api/orders'
import ProductImage from '../../components/ProductImage.vue'
import StorefrontHeader from '../../components/storefront/StorefrontHeader.vue'
import PreviewModeNotice from '../../components/storefront/PreviewModeNotice.vue'
import { storefrontPriceVisible } from '../../config/storefront'
import { formatMoney } from '../../utils/money'
import { storefrontAuthEnabled } from '../../composables/usePreviewMode'
import { buildProductSeoPath } from '../../utils/productSeoUrl'
import { currentLocale, withLocalePath } from '../../utils/localePath.ts'

const route = useRoute()
const { t } = useI18n()
const loading = ref(false)
const errorMessage = ref('')
const orders = ref<Order[]>([])

const hasOrders = computed(() => orders.value.length > 0)

onMounted(() => {
  if (storefrontAuthEnabled) {
    void loadOrders()
  }
})

async function loadOrders() {
  loading.value = true
  errorMessage.value = ''
  try {
    orders.value = await getStorefrontOrders()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : t('orders.loadFailed')
  } finally {
    loading.value = false
  }
}

function statusLabel(status: OrderStatus) {
  return t(`orders.status.${status}`)
}

function statusClass(status: OrderStatus) {
  switch (status) {
    case 'pending':
      return 'bg-orange-50 text-orange-700 ring-orange-200'
    case 'processing':
      return 'bg-violet-50 text-violet-700 ring-violet-200'
    case 'shipped':
      return 'bg-blue-50 text-blue-700 ring-blue-200'
    case 'completed':
      return 'bg-emerald-50 text-emerald-700 ring-emerald-200'
  }
}

function paymentStatusLabel(status: PaymentStatus) {
  return t(`orders.payment.${status}`)
}

function paymentStatusClass(status: PaymentStatus) {
  switch (status) {
    case 'unpaid':
      return 'bg-red-50 text-red-700 ring-red-200'
    case 'paid':
      return 'bg-emerald-50 text-emerald-700 ring-emerald-200'
    case 'refunded':
      return 'bg-slate-100 text-slate-700 ring-slate-200'
  }
}

function formatDateTime(value: string) {
  return new Intl.DateTimeFormat('en-GB', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value))
}

function itemTitle(item: OrderItem) {
  return item.product?.name || item.product_name || item.sku?.name || 'Product'
}

function itemImage(item: OrderItem) {
  return item.product?.main_image || ''
}

function itemProductPath(item: OrderItem) {
  return item.product?.id
    ? buildProductSeoPath({ id: item.product.id, name: item.product.name || item.product_name }, currentLocale(route))
    : ''
}

function formatSpecs(item: OrderItem) {
  const specs = item.sku?.specs
  if (!specs || Object.keys(specs).length === 0) {
    return item.sku?.name || '-'
  }
  return Object.entries(specs)
    .map(([key, value]) => `${key}: ${value}`)
    .join(' / ')
}
</script>

<template>
  <main class="min-h-screen bg-slate-50 text-slate-950">
    <StorefrontHeader />

    <PreviewModeNotice v-if="!storefrontAuthEnabled" />

    <section v-else class="mx-auto max-w-7xl px-4 py-5 sm:px-6 lg:px-8">
      <div class="mb-5 flex flex-col gap-3 rounded-2xl border border-slate-200 bg-white px-4 py-4 shadow-sm sm:flex-row sm:items-end sm:justify-between sm:px-5">
        <div>
          <h1 class="text-2xl font-black text-slate-950">{{ t('orders.title') }}</h1>
          <p class="mt-2 text-sm font-medium text-slate-500">{{ t('orders.description') }}</p>
        </div>
        <button
          type="button"
          class="inline-flex min-h-11 items-center justify-center rounded-2xl border border-slate-200 bg-white px-4 text-sm font-black text-slate-700 shadow-sm hover:border-orange-200 hover:bg-orange-50 hover:text-orange-700"
          @click="loadOrders"
        >
          {{ t('orders.refresh') }}
        </button>
      </div>

      <div v-if="loading" class="rounded-3xl border border-slate-200 bg-white p-8 text-center text-sm font-bold text-slate-500 shadow-sm">
        {{ t('orders.loading') }}
      </div>

      <div v-else-if="errorMessage" class="rounded-3xl border border-red-100 bg-red-50 p-8 text-center text-sm font-bold text-red-600">
        {{ errorMessage }}
      </div>

      <div v-else-if="!hasOrders" class="rounded-3xl border border-slate-200 bg-white p-8 text-center shadow-sm">
        <p class="text-base font-black text-slate-900">{{ t('orders.empty') }}</p>
        <RouterLink
          :to="withLocalePath(currentLocale(route), '/mall')"
          class="mt-4 inline-flex min-h-11 items-center justify-center rounded-2xl bg-orange-500 px-5 text-sm font-black text-white shadow-lg shadow-orange-200 hover:bg-orange-600"
        >
          {{ t('orders.browseProducts') }}
        </RouterLink>
      </div>

      <div v-else class="space-y-4">
        <article v-for="order in orders" :key="order.id" class="overflow-hidden rounded-3xl border border-slate-200 bg-white shadow-sm">
          <div class="flex flex-col gap-3 border-b border-slate-100 p-4 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <p class="text-xs font-black uppercase tracking-[0.18em] text-slate-400">{{ t('orders.orderNo') }}</p>
              <h2 class="mt-1 text-base font-black text-slate-950">{{ order.order_no }}</h2>
              <p class="mt-1 text-xs font-bold text-slate-500">{{ formatDateTime(order.created_at) }}</p>
            </div>

            <div class="flex flex-wrap items-center gap-3">
              <span class="rounded-full px-3 py-1 text-xs font-black ring-1" :class="statusClass(order.status)">
                {{ statusLabel(order.status) }}
              </span>
              <span class="rounded-full px-3 py-1 text-xs font-black ring-1" :class="paymentStatusClass(order.payment_status)">
                {{ paymentStatusLabel(order.payment_status) }}
              </span>
              <p v-if="storefrontPriceVisible" class="text-lg font-black text-slate-950">{{ formatMoney(order.total_amount) }}</p>
            </div>
          </div>

          <div class="border-b border-slate-100 bg-slate-50 px-4 py-3">
            <p v-if="order.fulfillment_method === 'pickup'" class="text-sm font-black text-slate-800">{{ t('orders.pickupNotice') }}</p>
            <div v-else-if="order.tracking_number" class="flex flex-wrap items-center gap-2">
              <span class="text-xs font-black uppercase tracking-[0.18em] text-slate-500">{{ t('orders.trackingNo') }}</span>
              <span class="rounded-xl bg-orange-500 px-3 py-1 text-sm font-black text-white">{{ order.tracking_number }}</span>
            </div>
            <p v-else class="text-sm font-bold text-slate-600">{{ t('orders.deliveryPending') }}</p>
          </div>

          <div class="divide-y divide-slate-100">
            <div v-for="item in order.items" :key="item.id" class="flex gap-3 p-4">
              <RouterLink
                v-if="itemProductPath(item)"
                :to="itemProductPath(item)"
                class="h-20 w-20 shrink-0 overflow-hidden rounded-2xl border border-slate-100"
              >
                <ProductImage :src="itemImage(item)" :alt="itemTitle(item)" />
              </RouterLink>
              <div v-else class="h-20 w-20 shrink-0 overflow-hidden rounded-2xl border border-slate-100">
                <ProductImage :src="itemImage(item)" :alt="itemTitle(item)" />
              </div>

              <div class="min-w-0 flex-1">
                <RouterLink
                  v-if="itemProductPath(item)"
                  :to="itemProductPath(item)"
                  class="block truncate text-sm font-black text-slate-950 hover:text-orange-600"
                >
                  {{ itemTitle(item) }}
                </RouterLink>
                <p v-else class="truncate text-sm font-black text-slate-950">{{ itemTitle(item) }}</p>
                <p class="mt-1 text-xs font-semibold text-slate-500">{{ formatSpecs(item) }}</p>
                <div class="mt-3 flex flex-wrap items-center justify-between gap-2 text-sm">
                  <p v-if="storefrontPriceVisible" class="font-black text-slate-900">{{ formatMoney(item.price) }}</p>
                  <p class="font-bold text-slate-500">{{ t('orders.quantity', { count: item.quantity }) }}</p>
                </div>
              </div>
            </div>
          </div>
        </article>
      </div>
    </section>
  </main>
</template>
