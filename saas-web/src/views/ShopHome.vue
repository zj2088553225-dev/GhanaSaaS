<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import ProductImage from '../components/ProductImage.vue'
import StorefrontHeader from '../components/storefront/StorefrontHeader.vue'
import StorefrontProductLink from '../components/storefront/StorefrontProductLink.vue'
import { getPublicShopProducts, getPublicShopProfile, type ProductListItem, type PublicShopProfile } from '../api/storefront'
import { storefrontPriceVisible } from '../config/storefront'
import { formatMoney } from '../utils/money'
import { absoluteURL, canonicalURL, plainText, useSeoMeta, type SeoMetaInput } from '../composables/useSeoMeta'
import { imageVariantURL } from '../utils/imageVariants'
import { buildProductSeoPath } from '../utils/productSeoUrl'
import { currentLocale, withLocalePath } from '../utils/localePath.ts'
import { trackStorefrontEvent, type StorefrontAnalyticsSource } from '../utils/storefrontAnalytics.ts'

const route = useRoute()
const { t } = useI18n()
const activeLocale = computed(() => currentLocale(route))

const shop = ref<PublicShopProfile | null>(null)
const products = ref<ProductListItem[]>([])
const total = ref(0)
const loading = ref(true)
const productsLoading = ref(false)
const errorMessage = ref('')
const query = reactive({
  page: 1,
  page_size: 12,
})
let shopLoadSequence = 0
let productLoadSequence = 0

const slug = computed(() => {
  const value = route.params.slug
  return Array.isArray(value) ? value[0] || '' : value || ''
})

const contactItems = computed(() => {
  if (!shop.value) {
    return []
  }

  return [
    { label: t('shop.phone'), value: shop.value.phone },
    { label: t('shop.address'), value: shop.value.address },
    { label: t('shop.businessHours'), value: shop.value.business_hours },
  ].filter((item) => Boolean(item.value))
})

const fallbackBannerSubtitle = computed(() => {
  const description = shop.value?.description?.replace(/\s+/g, ' ').trim()
  return description || t('shop.fallbackSubtitle')
})

const whatsAppHref = computed(() => {
  const value = shop.value?.whatsapp?.trim()
  if (!value) {
    return ''
  }
  if (/^https?:\/\//i.test(value)) {
    return value
  }

  const digits = value.replace(/[^\d]/g, '')
  return digits ? `https://wa.me/${digits}` : ''
})
const phoneHref = computed(() => {
  const value = shop.value?.phone?.trim()
  if (!value) {
    return ''
  }
  const normalized = value.replace(/[^\d+*#,;]/g, '')
  return normalized ? `tel:${normalized}` : ''
})
const shopAnalyticsSource = computed<StorefrontAnalyticsSource>(() => ({
  type: 'shop',
  id: String(shop.value?.tenant_id || slug.value),
}))

const shopSeoMeta = computed<SeoMetaInput>(() => {
  if (!shop.value) {
    return {
      title: 'Ghana Chinatown Store',
      description: 'Browse store products on Ghana Chinatown.',
      canonical: canonicalURL(withLocalePath(activeLocale.value, `/shops/${slug.value}`)),
      robots: errorMessage.value ? 'noindex,nofollow' : 'index,follow',
    }
  }

  const description = plainText(shop.value.description) || `Browse products from ${shop.value.shop_name} on Ghana Chinatown.`
  const image = shop.value.banner_url || shop.value.logo_url
  return {
    title: `${shop.value.shop_name} | Ghana Chinatown Store`,
    description,
    canonical: canonicalURL(withLocalePath(activeLocale.value, `/shops/${shop.value.slug}`)),
    robots: 'index,follow',
    type: 'website',
    image,
    jsonLd: [
      {
        '@context': 'https://schema.org',
        '@type': 'Store',
        name: shop.value.shop_name,
        url: canonicalURL(withLocalePath(activeLocale.value, `/shops/${shop.value.slug}`)),
        image: absoluteURL(image),
        telephone: shop.value.phone || shop.value.whatsapp || undefined,
        description,
      },
    ],
  }
})
useSeoMeta(shopSeoMeta)

onMounted(() => {
  void loadShop()
})

watch(slug, () => {
  query.page = 1
  void loadShop()
})

watch(activeLocale, () => {
  void loadShop()
})

async function loadShop() {
  const sequence = ++shopLoadSequence
  productLoadSequence++
  loading.value = true
  errorMessage.value = ''
  shop.value = null
  products.value = []
  total.value = 0

  try {
    const profile = await getPublicShopProfile(slug.value)
    if (sequence !== shopLoadSequence) {
      return
    }
    shop.value = profile
    await loadProducts()
  } catch (error) {
    if (sequence !== shopLoadSequence) {
      return
    }
    errorMessage.value = error instanceof Error ? error.message : t('shop.loadFailed')
  } finally {
    if (sequence === shopLoadSequence) {
      loading.value = false
    }
  }
}

async function loadProducts() {
  if (!slug.value) {
    return
  }

  const sequence = ++productLoadSequence
  productsLoading.value = true
  try {
    const page = await getPublicShopProducts(slug.value, {
      page: query.page,
      page_size: query.page_size,
    })
    if (sequence !== productLoadSequence) {
      return
    }
    products.value = page.items
    total.value = page.total
    query.page = page.page
    query.page_size = page.page_size
  } catch (error) {
    if (sequence !== productLoadSequence) {
      return
    }
    errorMessage.value = error instanceof Error ? error.message : t('shop.productLoadFailed')
  } finally {
    if (sequence === productLoadSequence) {
      productsLoading.value = false
    }
  }
}

async function changePage(page: number) {
  query.page = page
  await loadProducts()
}

async function changePageSize(pageSize: number) {
  query.page_size = pageSize
  query.page = 1
  await loadProducts()
}

function trackShopContact(eventName: 'whatsapp_click' | 'phone_click') {
  if (!shop.value) {
    return
  }
  const shopID = String(shop.value.tenant_id || shop.value.slug)
  trackStorefrontEvent({
    event_name: eventName,
    entity_type: 'shop',
    entity_id: shopID,
    source_type: 'shop',
    source_id: shopID,
    page_type: 'shop',
  })
}
</script>

<template>
  <main class="min-h-screen bg-slate-50 pb-16 text-slate-950">
    <StorefrontHeader />

    <section class="mx-auto max-w-7xl px-4 py-5 sm:px-6 lg:px-8">
      <div v-if="loading" class="rounded-3xl bg-white p-8 text-center text-sm font-semibold text-slate-500 shadow-sm">
        {{ t('shop.loading') }}
      </div>

      <div v-else-if="errorMessage" class="rounded-3xl bg-red-50 p-8 text-center text-sm font-semibold text-red-600">
        {{ errorMessage }}
      </div>

      <template v-else-if="shop">
        <section class="overflow-hidden rounded-3xl bg-white shadow-sm">
          <div class="relative h-[178px] overflow-hidden bg-orange-50 sm:h-[236px] lg:h-[248px]">
            <ProductImage
              v-if="shop.banner_url"
              :src="imageVariantURL(shop.banner_url, 'large')"
              :alt="shop.shop_name"
              loading="eager"
              fetchpriority="high"
              class="h-full w-full"
              image-class="h-full w-full object-cover"
            />
            <template v-if="shop.banner_url">
              <div class="absolute inset-0 bg-gradient-to-r from-slate-950/55 via-slate-950/18 to-transparent" />
              <div class="absolute inset-x-0 bottom-0 h-24 bg-gradient-to-t from-slate-950/45 to-transparent" />
              <div class="absolute inset-0 flex items-end p-5 sm:p-8">
                <div class="max-w-2xl text-white drop-shadow">
                  <p class="text-[11px] font-black uppercase tracking-[0.24em] text-orange-200">Ghana Chinatown Store</p>
                  <h2 class="mt-2 line-clamp-1 text-2xl font-black leading-tight sm:text-4xl">{{ shop.shop_name }}</h2>
                  <p v-if="shop.description" class="mt-2 line-clamp-1 text-sm font-semibold text-white/85 sm:text-base">
                    {{ shop.description }}
                  </p>
                </div>
              </div>
            </template>
            <div v-else class="relative h-full w-full overflow-hidden bg-gradient-to-br from-orange-50 via-white to-slate-100">
              <div class="absolute -right-16 -top-16 h-48 w-48 rounded-full bg-orange-200/45 blur-2xl sm:h-72 sm:w-72" />
              <div class="absolute bottom-[-72px] right-12 h-44 w-44 rounded-full bg-amber-100/80 blur-xl sm:h-60 sm:w-60" />
              <div class="absolute -left-16 bottom-8 h-36 w-36 rounded-full bg-slate-200/70 blur-xl" />
              <div class="absolute right-6 top-8 h-28 w-56 -rotate-12 rounded-[2rem] bg-white/55 ring-1 ring-white/70 sm:right-20 sm:h-36 sm:w-72" />
              <div class="absolute right-24 top-24 hidden h-20 w-20 rounded-full border border-orange-200/70 sm:block" />
              <div class="relative flex h-full max-w-2xl flex-col justify-center px-5 py-5 sm:px-8">
                <p class="text-xs font-black uppercase tracking-[0.22em] text-orange-500">Ghana Chinatown Store</p>
                <h2 class="mt-2 text-2xl font-black leading-tight text-slate-950 sm:text-4xl">{{ t('shop.heroTitle') }}</h2>
                <p class="mt-2 line-clamp-1 max-w-xl text-sm font-semibold text-slate-600 sm:text-base">
                  {{ fallbackBannerSubtitle }}
                </p>
                <div class="mt-4 flex flex-wrap gap-2">
                  <span class="rounded-full bg-white/85 px-3 py-1 text-xs font-bold text-orange-700 shadow-sm ring-1 ring-orange-100">{{ t('shop.badges.quality') }}</span>
                  <span class="rounded-full bg-white/85 px-3 py-1 text-xs font-bold text-slate-700 shadow-sm ring-1 ring-slate-200">{{ t('shop.badges.service') }}</span>
                  <span class="rounded-full bg-white/85 px-3 py-1 text-xs font-bold text-emerald-700 shadow-sm ring-1 ring-emerald-100">{{ t('shop.badges.shipping') }}</span>
                </div>
              </div>
            </div>
          </div>

          <div class="px-4 py-5 sm:px-6 sm:py-6 lg:px-8">
            <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:gap-5">
              <div class="flex min-w-0 items-center gap-4">
                <div class="h-20 w-20 shrink-0 overflow-hidden rounded-2xl border border-slate-100 bg-white shadow-md sm:h-24 sm:w-24">
                  <ProductImage :src="imageVariantURL(shop.logo_url, 'thumb')" :alt="shop.shop_name" class="h-full w-full" />
                </div>

                <div class="min-w-0">
                  <h2 class="line-clamp-1 text-2xl font-black leading-tight text-slate-950 sm:text-3xl">{{ shop.shop_name }}</h2>
                  <p class="mt-1 truncate text-sm font-semibold text-slate-500">/shops/{{ shop.slug }}</p>
                  <p class="mt-2 line-clamp-1 max-w-3xl text-sm leading-6 text-slate-600">
                    {{ shop.description || t('shop.fallbackDescription') }}
                  </p>
                  <div v-if="contactItems.length" class="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-slate-500">
                    <span v-for="item in contactItems" :key="item.label" class="min-w-0">
                      <span class="font-bold text-slate-600">{{ item.label }}</span>
                      <span class="ml-1 font-semibold">{{ item.value }}</span>
                    </span>
                  </div>
                </div>
              </div>

              <div v-if="(shop.phone && phoneHref) || shop.whatsapp" class="flex w-full shrink-0 flex-col gap-2 sm:ml-auto sm:w-auto sm:flex-row">
                <a
                  v-if="shop.phone && phoneHref"
                  :href="phoneHref"
                  class="inline-flex w-full items-center justify-center gap-2 rounded-full border border-slate-200 bg-white px-4 py-2.5 text-sm font-black text-slate-700 transition hover:bg-slate-50 sm:w-auto"
                  @click="trackShopContact('phone_click')"
                >
                  <span aria-hidden="true">☎</span>
                  <span>{{ shop.phone }}</span>
                </a>
                <a
                  v-if="shop.whatsapp"
                  :href="whatsAppHref || undefined"
                  :target="whatsAppHref ? '_blank' : undefined"
                  :rel="whatsAppHref ? 'noopener noreferrer' : undefined"
                  class="inline-flex w-full items-center justify-center gap-2 rounded-full border border-orange-200 bg-orange-50 px-4 py-2.5 text-sm font-black text-orange-700 transition hover:bg-orange-100 sm:w-auto"
                  @click="trackShopContact('whatsapp_click')"
                >
                  <span class="flex h-2.5 w-2.5 rounded-full bg-emerald-500" />
                  <span>WhatsApp</span>
                  <span class="max-w-[12rem] truncate font-bold text-orange-600">{{ shop.whatsapp }}</span>
                </a>
              </div>
              <div v-else class="hidden shrink-0 rounded-full bg-slate-50 px-4 py-2 text-sm font-bold text-slate-500 sm:ml-auto sm:block">
                {{ t('shop.homepage') }}
              </div>
            </div>
          </div>
        </section>

        <section class="mt-5 rounded-3xl bg-white p-4 shadow-sm sm:p-5">
          <div class="mb-4 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h2 class="text-xl font-black text-slate-950">{{ t('shop.productsTitle') }}</h2>
              <p class="mt-1 text-sm text-slate-500">{{ t('shop.productsSubtitle') }}</p>
            </div>
            <button
              type="button"
              class="rounded-xl border border-orange-200 bg-orange-50 px-4 py-2 text-sm font-bold text-orange-700 transition hover:bg-orange-100 disabled:cursor-wait disabled:opacity-60"
              :disabled="productsLoading"
              @click="loadProducts"
            >
              {{ productsLoading ? t('shop.loadingAction') : t('shop.refresh') }}
            </button>
          </div>

          <div v-loading="productsLoading">
            <div v-if="products.length" class="grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-4">
              <StorefrontProductLink
                v-for="product in products"
                :key="product.id"
                :to="buildProductSeoPath(product, activeLocale)"
                :product-id="product.id"
                :analytics-source="shopAnalyticsSource"
                page-type="shop"
                class="group overflow-hidden rounded-3xl border border-slate-100 bg-white shadow-sm transition duration-200 hover:-translate-y-1 hover:border-orange-100 hover:shadow-xl hover:shadow-slate-200/70"
              >
                <div class="aspect-square w-full overflow-hidden bg-slate-100">
                  <ProductImage
                    :src="imageVariantURL(product.display_image, 'card')"
                    :alt="product.display_name"
                    loading="lazy"
                    class="h-full w-full"
                    image-class="h-full w-full object-cover transition duration-300 group-hover:scale-[1.03]"
                  />
                </div>
                <div class="p-3">
                  <h3 class="line-clamp-2 min-h-10 text-sm font-bold text-slate-900">{{ product.display_name }}</h3>
                  <p class="mt-2 text-sm font-black text-orange-600">
                    {{
                      storefrontPriceVisible
                        ? product.display_price === null
                          ? t('mall.priceUnavailable')
                          : formatMoney(product.display_price)
                        : t('storefront.contactForPrice')
                    }}
                  </p>
                </div>
              </StorefrontProductLink>
            </div>

            <p v-else class="rounded-2xl bg-slate-50 p-8 text-center text-sm font-semibold text-slate-500">
              {{ t('shop.emptyProducts') }}
            </p>
          </div>

          <div v-if="total > query.page_size" class="mt-4 flex justify-end">
            <el-pagination
              background
              layout="total, sizes, prev, pager, next"
              :current-page="query.page"
              :page-size="query.page_size"
              :page-sizes="[12, 24, 48]"
              :total="total"
              @current-change="changePage"
              @size-change="changePageSize"
            />
          </div>
        </section>
      </template>
    </section>
  </main>
</template>
