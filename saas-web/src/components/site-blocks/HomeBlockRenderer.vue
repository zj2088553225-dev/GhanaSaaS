<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { SitePageBlock } from '../../api/sitePages'
import { getStorefrontPlatformCategoryChildren, type PlatformCategory } from '../../api/platformCategories'
import { getProducts, getPublicShops, type ProductListItem, type PublicShopProfile } from '../../api/storefront'
import ProductImage from '../ProductImage.vue'
import StorefrontProductLink from '../storefront/StorefrontProductLink.vue'
import PopularProducts from '../storefront/PopularProducts.vue'
import HeroCarousel from '../site-blocks/HeroCarousel.vue'
import GoogleMapSection from './GoogleMapSection.vue'
import { vStorefrontImpression } from '../../directives/storefrontImpression'
import { storefrontPriceVisible } from '../../config/storefront'
import { formatMoney } from '../../utils/money'
import { normalizeLimit } from './blockDefaults'
import { homeProductLimit, legacyHomeDefaultTranslationKey } from '../../utils/homeStorefront'
import { productSourceLinkQuery, productSourceParam } from '../../utils/mallSource'
import { imageVariantURL } from '../../utils/imageVariants'
import { buildProductSeoPath } from '../../utils/productSeoUrl'
import { currentLocale, ensureLocalePath, withLocalePath } from '../../utils/localePath.ts'
import {
  trackStorefrontEvent,
  type StorefrontAnalyticsSource,
  type StorefrontEventInput,
} from '../../utils/storefrontAnalytics.ts'

const props = defineProps<{
  blocks: SitePageBlock[]
}>()
const { t } = useI18n()
const route = useRoute()
const activeLocale = computed(() => currentLocale(route))

const productsByBlock = ref<Record<number, ProductListItem[]>>({})
const shopsByBlock = ref<Record<number, PublicShopProfile[]>>({})
const categoriesByBlock = ref<Record<number, PlatformCategory[]>>({})
const loadingByBlock = ref<Record<number, boolean>>({})
const floorAccentClasses = ['bg-orange-500', 'bg-sky-500', 'bg-emerald-500', 'bg-violet-500']
let dynamicLoadSequence = 0

watch(
  [() => props.blocks, () => activeLocale.value],
  ([blocks]) => {
    void loadDynamicBlocks(blocks || [])
  },
  { immediate: true, deep: true },
)

async function loadDynamicBlocks(blocks: SitePageBlock[]) {
  const sequence = ++dynamicLoadSequence
  const locale = activeLocale.value
  const productEntries: Record<number, ProductListItem[]> = {}
  const shopEntries: Record<number, PublicShopProfile[]> = {}
  const categoryEntries: Record<number, PlatformCategory[]> = {}
  const productRequestCache = new Map<string, Promise<ProductListItem[]>>()
  loadingByBlock.value = Object.fromEntries(blocks.map((_, index) => [index, true]))

  function productRequestForBlock(block: SitePageBlock) {
    const sourceType = block.source_type || (block.platform_category_id ? 'category' : 'latest')
    const params = {
      page: 1,
      page_size: homeProductLimit(block.limit),
      platform_category_id: sourceType === 'category' ? block.platform_category_id : undefined,
      source: productSourceParam(block.source),
      sort: block.type === 'product_floor'
        ? (sourceType === 'latest' ? 'latest' : block.sort)
        : block.source === 'hot' ? 'hot' : undefined,
    }
    const cacheKey = JSON.stringify({ locale, params })
    const cached = productRequestCache.get(cacheKey)
    if (cached) {
      return cached
    }

    const request = getProducts(params)
      .then((pageData) => pageData.items)
      .catch(() => {
        if (import.meta.env.DEV) {
          console.warn('[HomeBlockRenderer] Product block request failed', {
            blockType: block.type,
            platformCategoryID: block.platform_category_id,
          })
        }
        return []
      })
    productRequestCache.set(cacheKey, request)
    return request
  }

  await Promise.allSettled(
    blocks.map(async (block, index) => {
      if (block.type === 'product_grid' || block.type === 'product_floor') {
        if (block.type === 'product_floor' &&
          (block.source_type === 'popular' || block.source_type === 'manual')) {
          loadingByBlock.value[index] = false
          return
        }
        try {
          productEntries[index] = await productRequestForBlock(block)
        } catch {
          productEntries[index] = []
        } finally {
          if (sequence === dynamicLoadSequence) {
            loadingByBlock.value[index] = false
          }
        }
        return
      }
      if (block.type === 'shop_grid') {
        try {
          const pageData = await getPublicShops({ page: 1, page_size: normalizeLimit(block.limit) })
          shopEntries[index] = pageData.items
        } catch {
          shopEntries[index] = []
        } finally {
          if (sequence === dynamicLoadSequence) {
            loadingByBlock.value[index] = false
          }
        }
        return
      }
      if (block.type === 'category_nav' && !block.items?.length) {
        try {
          categoryEntries[index] = await getStorefrontPlatformCategoryChildren(block.platform_category_id || null, { maxLevel: 2 })
        } catch {
          categoryEntries[index] = []
        } finally {
          if (sequence === dynamicLoadSequence) {
            loadingByBlock.value[index] = false
          }
        }
        return
      }
      if (sequence === dynamicLoadSequence) {
        loadingByBlock.value[index] = false
      }
    }),
  )

  if (sequence !== dynamicLoadSequence) {
    return
  }
  productsByBlock.value = productEntries
  shopsByBlock.value = shopEntries
  categoriesByBlock.value = categoryEntries
}

function heroStyle(block: SitePageBlock) {
  const imageURL = imageVariantURL(block.backgroundImage || block.image_url, 'large')
  if (!imageURL) {
    return {}
  }
  return {
    backgroundImage: `linear-gradient(90deg, rgba(15, 23, 42, 0.9), rgba(15, 23, 42, 0.28)), url(${imageURL})`,
    backgroundSize: 'cover',
    backgroundPosition: 'center',
  }
}

function shouldRenderBlock(block: SitePageBlock, index: number) {
  if (block.type === 'product_floor') {
    if (block.source_type === 'popular') {
      return block.enabled !== false
    }
    if (block.source_type === 'manual') {
      return false
    }
    return block.enabled !== false && (loadingByBlock.value[index] || Boolean(productsByBlock.value[index]?.length))
  }
  if (block.type === 'map_section') {
    return block.enabled !== false
  }
  if (block.type === 'hero_carousel') {
    return (block.slides || []).some((slide) => slide.enabled !== false)
  }
  return true
}

function categoryNavItems(block: SitePageBlock, index: number) {
  if (block.items?.length) {
    return block.items.map((item) => ({
      ...item,
      link: localizedCMSLink(item.link || ''),
    }))
  }
  return (categoriesByBlock.value[index] || []).map((category) => ({
    tracking_key: block.tracking_key
      ? `${block.tracking_key}:${category.id}`
      : `category:${category.id}`,
    name: category.name,
    image: '',
    link: localizedPath(`/mall?platform_category_id=${category.id}`),
  }))
}

function localizedCMSDefault(value: string | undefined, fallbackKey: string) {
  const normalized = value?.trim()
  if (!normalized) {
    return t(fallbackKey)
  }
  const translationKey = legacyHomeDefaultTranslationKey(normalized)
  return translationKey ? t(translationKey) : normalized
}

function floorGridClass(block: SitePageBlock) {
  switch (Number(block.columns || 5)) {
    case 6:
      return 'lg:grid-cols-6'
    case 4:
      return 'lg:grid-cols-4'
    default:
      return 'lg:grid-cols-5'
  }
}

function floorAccentClass(index: number) {
  const floorIndex = props.blocks.slice(0, index).filter((block) => block.type === 'product_floor').length
  return floorAccentClasses[floorIndex % floorAccentClasses.length]
}

function productListLink(block: SitePageBlock) {
  const query = new URLSearchParams(productSourceLinkQuery(block.source))
  if (block.platform_category_id) {
    query.set('platform_category_id', String(block.platform_category_id))
  }
  const queryString = query.toString()
  return localizedPath(queryString ? `/mall?${queryString}` : '/mall')
}

function promoImage(block: SitePageBlock) {
  return imageVariantURL(block.image_url || block.image, 'large')
}

function promoLink(block: SitePageBlock) {
  return localizedCMSLink(block.link_url || block.buttonLink || '')
}

function localizedPath(path: string) {
  return withLocalePath(currentLocale(route), path)
}

function localizedCMSLink(link: string) {
  return ensureLocalePath(link, currentLocale(route))
}

function formatProductPrice(product: ProductListItem) {
  if (product.max_price && product.max_price > product.min_price) {
    return `${formatMoney(product.min_price)} - ${formatMoney(product.max_price)}`
  }
  return formatMoney(product.min_price)
}

function hasOriginalPrice(product: ProductListItem) {
  return product.original_price > 0 && product.original_price > product.min_price
}

function productAnalyticsSource(block: SitePageBlock): StorefrontAnalyticsSource {
  return { type: 'home_product_floor', id: block.tracking_key || '' }
}

function marketingContentEvent(
  eventName: 'content_impression' | 'content_click',
  trackingKey?: string,
): StorefrontEventInput | null {
  if (!trackingKey) {
    return null
  }
  return {
    event_name: eventName,
    entity_type: 'marketing_content',
    entity_id: trackingKey,
    source_type: 'marketing_content',
    source_id: trackingKey,
    page_type: 'home',
  }
}

function marketingBlockImpression(block: SitePageBlock) {
  if (!['hero', 'hero_banner', 'image_text', 'feature_grid', 'promo_banner', 'contact', 'image'].includes(block.type)) {
    return null
  }
  return marketingContentEvent('content_impression', block.tracking_key)
}

function trackMarketingContentClick(trackingKey?: string) {
  const event = marketingContentEvent('content_click', trackingKey)
  if (event) {
    trackStorefrontEvent(event)
  }
}

function contactPhoneHref(value?: string) {
  const normalized = value?.trim().replace(/[^\d+*#,;]/g, '') || ''
  return normalized ? `tel:${normalized}` : ''
}

function contactWhatsAppHref(value?: string) {
  const normalized = value?.trim() || ''
  if (/^https?:\/\//i.test(normalized)) {
    return normalized
  }
  const digits = normalized.replace(/[^\d]/g, '')
  return digits ? `https://wa.me/${digits}` : ''
}

function trackMarketingContact(block: SitePageBlock, eventName: 'phone_click' | 'whatsapp_click') {
  trackMarketingContentClick(block.tracking_key)
  if (!block.tracking_key) {
    return
  }
  trackStorefrontEvent({
    event_name: eventName,
    entity_type: 'marketing_content',
    entity_id: block.tracking_key,
    source_type: 'marketing_content',
    source_id: block.tracking_key,
    page_type: 'home',
  })
}
</script>

<template>
  <div class="bg-slate-50">
    <template v-for="(block, index) in blocks" :key="block.tracking_key || `${block.type}-${index}`">
    <PopularProducts
      v-if="block.type === 'product_floor' && block.source_type === 'popular' && block.enabled !== false"
      :block="block"
    />
    <section
      v-else-if="shouldRenderBlock(block, index)"
      v-storefront-impression="marketingBlockImpression(block)"
      class="mx-auto max-w-screen-2xl px-4 sm:px-6 lg:px-8"
      :class="block.type === 'hero' || block.type === 'hero_banner' || block.type === 'hero_carousel' ? 'py-6 sm:py-8' : 'py-6 sm:py-12'"
    >
      <div
        v-if="block.type === 'hero'"
        class="relative min-h-[320px] overflow-hidden rounded-[2rem] bg-gradient-to-br from-slate-950 via-slate-900 to-emerald-800 px-5 py-10 text-white shadow-2xl sm:min-h-[420px] sm:px-10 sm:py-14 lg:min-h-[520px] lg:px-14"
        :style="heroStyle(block)"
      >
        <div class="absolute inset-x-0 bottom-0 h-32 bg-gradient-to-t from-slate-950/70 to-transparent"></div>
        <div class="relative flex min-h-[220px] max-w-3xl flex-col justify-center sm:min-h-[320px] lg:min-h-[390px]">
          <p class="text-xs font-bold uppercase tracking-[0.28em] text-orange-300">Ghana Chinatown</p>
          <h2 class="mt-4 max-w-3xl text-3xl font-black leading-tight sm:text-4xl lg:text-6xl">{{ block.title || 'Ghana Chinatown' }}</h2>
          <p class="mt-5 max-w-2xl text-base leading-7 text-slate-100 sm:leading-8 sm:text-lg">{{ localizedCMSDefault(block.subtitle, 'homeBlocks.defaultHeroSubtitle') }}</p>
          <div class="mt-8 flex flex-wrap gap-3">
            <a
              v-if="block.buttonText && block.buttonLink"
              :href="localizedCMSLink(block.buttonLink)"
              class="inline-flex rounded-xl bg-orange-500 px-5 py-3 text-sm font-black text-white shadow-lg shadow-orange-950/30 transition hover:bg-orange-400"
              @click="trackMarketingContentClick(block.tracking_key)"
            >
              {{ localizedCMSDefault(block.buttonText, 'homeBlocks.browseShops') }}
            </a>
            <RouterLink :to="localizedPath('/shops')" class="inline-flex rounded-xl bg-white/10 px-5 py-3 text-sm font-black text-white ring-1 ring-white/20 transition hover:bg-white/15">
              {{ t('homeBlocks.browseShops') }}
            </RouterLink>
          </div>
        </div>
      </div>

      <div
        v-else-if="block.type === 'hero_banner'"
        class="relative min-h-[320px] overflow-hidden rounded-[2rem] bg-slate-950 px-5 py-10 text-white shadow-2xl sm:min-h-[420px] sm:px-10 sm:py-14 lg:min-h-[520px] lg:px-14"
        :style="heroStyle(block)"
      >
        <div class="absolute inset-x-0 bottom-0 h-36 bg-gradient-to-t from-slate-950/80 to-transparent"></div>
        <div class="relative flex min-h-[220px] max-w-3xl flex-col justify-center sm:min-h-[320px] lg:min-h-[390px]">
          <p class="text-xs font-bold uppercase tracking-[0.28em] text-orange-300">{{ t('homeBlocks.marketplace') }}</p>
          <h2 class="mt-4 max-w-3xl text-3xl font-black leading-tight sm:text-4xl lg:text-6xl">{{ block.title || 'Ghana Chinatown' }}</h2>
          <p class="mt-5 max-w-2xl text-base leading-7 text-slate-100 sm:leading-8 sm:text-lg">{{ localizedCMSDefault(block.subtitle, 'homeBlocks.defaultMarketplaceSubtitle') }}</p>
          <div class="mt-8 flex flex-wrap gap-3">
            <RouterLink
              v-if="block.button_text && block.link_url"
              :to="localizedCMSLink(block.link_url)"
              class="inline-flex rounded-xl bg-orange-500 px-5 py-3 text-sm font-black text-white shadow-lg shadow-orange-950/30 transition hover:bg-orange-400"
              @click="trackMarketingContentClick(block.tracking_key)"
            >
              {{ localizedCMSDefault(block.button_text, 'homeBlocks.defaultShopNow') }}
            </RouterLink>
            <RouterLink :to="localizedPath('/mall')" class="inline-flex rounded-xl bg-white/10 px-5 py-3 text-sm font-black text-white ring-1 ring-white/20 transition hover:bg-white/15">
              {{ t('homeBlocks.browseProducts') }}
            </RouterLink>
          </div>
        </div>
      </div>

      <HeroCarousel v-else-if="block.type === 'hero_carousel'" :block="block" />

      <div v-else-if="block.type === 'category_nav'" class="rounded-2xl bg-white p-5 shadow-sm sm:p-7">
        <div class="mb-5 flex items-end justify-between gap-4">
          <div>
            <p class="text-xs font-bold uppercase tracking-[0.22em] text-emerald-600">{{ t('homeBlocks.categories') }}</p>
            <h2 class="mt-1 text-2xl font-black text-slate-950">{{ localizedCMSDefault(block.title, 'homeBlocks.popularCategories') }}</h2>
          </div>
        </div>
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
          <a
            v-for="(item, itemIndex) in categoryNavItems(block, index)"
            :key="item.tracking_key || item.link || itemIndex"
            v-storefront-impression="marketingContentEvent('content_impression', item.tracking_key)"
            :href="item.link || '#'"
            class="group overflow-hidden rounded-xl border border-slate-100 bg-slate-50 p-3 transition hover:-translate-y-1 hover:border-orange-200 hover:bg-orange-50"
            @click="trackMarketingContentClick(item.tracking_key)"
          >
            <div class="flex aspect-square items-center justify-center overflow-hidden rounded-lg bg-white text-2xl font-black text-slate-300">
              <img v-if="item.image" :src="imageVariantURL(item.image, 'thumb')" :alt="item.name || ''" loading="lazy" decoding="async" class="h-full w-full object-cover transition group-hover:scale-105" />
              <span v-else>{{ item.name?.slice(0, 1) || t('homeBlocks.categoryFallback').slice(0, 1) }}</span>
            </div>
            <p class="mt-3 text-center text-sm font-black text-slate-800">{{ item.name || t('homeBlocks.categoryFallback') }}</p>
          </a>
        </div>
      </div>

      <GoogleMapSection v-else-if="block.type === 'map_section'" :block="block" :locale="activeLocale" />

      <div v-else-if="block.type === 'product_floor'" class="rounded-2xl bg-white p-4 shadow-sm sm:p-5">
        <div class="mb-4 flex items-start justify-between gap-3 sm:items-end sm:gap-4">
          <div class="flex min-w-0 items-start gap-3 sm:gap-4">
            <span aria-hidden="true" class="mt-1 h-8 w-1 shrink-0 rounded-full sm:h-10" :class="floorAccentClass(index)" />
            <div class="min-w-0">
              <p class="text-xs font-bold uppercase tracking-[0.22em] text-orange-500">{{ block.platform_category_path || t('homeBlocks.products') }}</p>
              <h2 class="mt-1 break-words text-xl font-black text-slate-950 sm:text-2xl">{{ localizedCMSDefault(block.title, 'homeBlocks.productFloor') }}</h2>
              <p v-if="block.subtitle" class="mt-1 text-sm leading-6 text-slate-500 sm:mt-2">{{ localizedCMSDefault(block.subtitle, 'homeBlocks.defaultFreshPicks') }}</p>
            </div>
          </div>
          <RouterLink v-if="block.show_more_link !== false" :to="productListLink(block)" class="shrink-0 pt-1 text-sm font-black text-orange-600 hover:text-orange-500 sm:pt-0">
            {{ t('homeBlocks.more') }}
          </RouterLink>
        </div>
        <div v-if="loadingByBlock[index]" class="grid animate-pulse grid-cols-2 gap-3 sm:grid-cols-3" :class="floorGridClass(block)">
          <div v-for="item in homeProductLimit(block.limit)" :key="item" class="aspect-[3/4] rounded-xl bg-slate-100" />
        </div>
        <div v-else class="grid grid-cols-2 gap-3 sm:grid-cols-3" :class="floorGridClass(block)">
          <StorefrontProductLink
            v-for="product in productsByBlock[index]"
            :key="product.id"
            :to="buildProductSeoPath(product, currentLocale(route))"
            :product-id="product.id"
            :analytics-source="productAnalyticsSource(block)"
            page-type="home"
            class="group flex flex-col overflow-hidden rounded-xl border border-slate-100 bg-white shadow-sm transition hover:-translate-y-1 hover:border-orange-100 hover:shadow-xl hover:shadow-slate-200/70"
          >
            <div class="aspect-square w-full overflow-hidden bg-slate-100">
              <ProductImage
                :src="imageVariantURL(product.main_image, 'card')"
                :alt="product.name"
                class="h-full w-full"
                image-class="h-full w-full object-cover transition duration-300 group-hover:scale-[1.03]"
              />
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

      <div v-else-if="block.type === 'image_text'" class="grid items-center gap-8 rounded-2xl bg-white p-5 shadow-sm md:grid-cols-2 md:p-8">
        <div class="overflow-hidden rounded-2xl bg-slate-100" :class="block.layout === 'image_right' ? 'md:order-2' : ''">
          <ProductImage :src="imageVariantURL(block.image, 'large')" :alt="block.title || ''" class="aspect-[4/3] w-full" image-class="h-full w-full object-cover" />
        </div>
        <div>
          <p class="text-xs font-bold uppercase tracking-[0.22em] text-orange-500">{{ t('homeBlocks.about') }}</p>
          <h2 class="mt-2 text-3xl font-black leading-tight text-slate-950">{{ localizedCMSDefault(block.title, 'homeBlocks.introduction') }}</h2>
          <p class="mt-3 whitespace-pre-line text-base leading-7 text-slate-600 sm:mt-4 sm:leading-8">{{ localizedCMSDefault(block.content, 'homeBlocks.defaultConnectContent') }}</p>
          <a
            v-if="block.buttonText && block.buttonLink"
            :href="localizedCMSLink(block.buttonLink)"
            class="mt-5 inline-flex rounded-xl bg-slate-950 px-5 py-3 text-sm font-black text-white transition hover:bg-slate-800 sm:mt-6"
            @click="trackMarketingContentClick(block.tracking_key)"
          >
            {{ localizedCMSDefault(block.buttonText, 'homeBlocks.browseShops') }}
          </a>
        </div>
      </div>

      <div v-else-if="block.type === 'feature_grid'" class="rounded-2xl bg-slate-950 p-5 text-white shadow-sm sm:p-8">
        <div class="mb-6 max-w-2xl">
          <p class="text-xs font-bold uppercase tracking-[0.22em] text-orange-300">{{ t('homeBlocks.advantages') }}</p>
          <h2 class="mt-2 text-3xl font-black">{{ localizedCMSDefault(block.title, 'homeBlocks.serviceAdvantages') }}</h2>
          <p class="mt-3 text-sm leading-7 text-slate-300 sm:leading-8">{{ localizedCMSDefault(block.subtitle, 'homeBlocks.defaultAdvantagesSubtitle') }}</p>
        </div>
        <div class="grid gap-4 md:grid-cols-3">
          <div v-for="(item, itemIndex) in block.items || []" :key="item.tracking_key || itemIndex" class="rounded-xl bg-white/10 p-5 ring-1 ring-white/10">
            <div class="flex h-11 w-11 items-center justify-center rounded-lg bg-emerald-500 text-base font-black text-white">
              {{ item.icon || itemIndex + 1 }}
            </div>
            <h3 class="mt-4 text-lg font-black">{{ localizedCMSDefault(item.title, 'homeBlocks.advantageTitle') }}</h3>
            <p class="mt-2 text-sm leading-7 text-slate-300 sm:leading-8">{{ item.description ? localizedCMSDefault(item.description, 'homeBlocks.defaultMerchantShowcaseDescription') : '' }}</p>
          </div>
        </div>
      </div>

      <div v-else-if="block.type === 'product_grid'" class="rounded-2xl bg-white p-4 shadow-sm sm:p-5">
        <div class="mb-4 flex items-center justify-between gap-4">
          <div>
            <p class="text-xs font-bold uppercase tracking-[0.22em] text-orange-500">{{ t('homeBlocks.products') }}</p>
            <h2 class="mt-1 text-xl font-black text-slate-950 sm:text-2xl">{{ localizedCMSDefault(block.title, 'homeBlocks.featuredProducts') }}</h2>
            <p v-if="block.subtitle" class="mt-1 text-sm leading-6 text-slate-500 sm:mt-2">{{ block.subtitle }}</p>
          </div>
          <RouterLink :to="productListLink(block)" class="text-sm font-black text-orange-600 hover:text-orange-500">{{ t('homeBlocks.viewMore') }}</RouterLink>
        </div>
        <div v-if="loadingByBlock[index]" class="grid animate-pulse grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
          <div v-for="item in homeProductLimit(block.limit)" :key="item" class="aspect-[3/4] rounded-xl bg-slate-100" />
        </div>
        <div v-else-if="productsByBlock[index]?.length" class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
          <StorefrontProductLink
            v-for="product in productsByBlock[index]"
            :key="product.id"
            :to="buildProductSeoPath(product, currentLocale(route))"
            :product-id="product.id"
            :analytics-source="productAnalyticsSource(block)"
            page-type="home"
            class="group flex flex-col overflow-hidden rounded-xl border border-slate-100 bg-white shadow-sm transition hover:-translate-y-1 hover:border-orange-100 hover:shadow-xl hover:shadow-slate-200/70"
          >
            <div class="aspect-square w-full overflow-hidden bg-slate-100">
              <ProductImage
                :src="imageVariantURL(product.main_image, 'card')"
                :alt="product.name"
                class="h-full w-full"
                image-class="h-full w-full object-cover transition duration-300 group-hover:scale-[1.03]"
              />
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
        <p v-else class="rounded-xl bg-slate-50 p-8 text-center text-sm text-slate-500">{{ t('homeBlocks.noProducts') }}</p>
      </div>

      <div v-else-if="block.type === 'shop_grid'" class="rounded-2xl bg-white p-5 shadow-sm sm:p-7">
        <div class="mb-5 flex items-center justify-between gap-4">
          <div>
            <p class="text-xs font-bold uppercase tracking-[0.22em] text-emerald-600">{{ t('homeBlocks.merchants') }}</p>
            <h2 class="mt-1 text-2xl font-black text-slate-950">{{ localizedCMSDefault(block.title, 'homeBlocks.recommendedMerchants') }}</h2>
          </div>
          <RouterLink :to="localizedPath('/shops')" class="text-sm font-black text-orange-600 hover:text-orange-500">{{ t('homeBlocks.allMerchants') }}</RouterLink>
        </div>
        <div v-if="shopsByBlock[index]?.length" class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          <RouterLink
            v-for="shop in shopsByBlock[index]"
            :key="shop.slug"
            :to="localizedPath(`/shops/${shop.slug}`)"
            class="overflow-hidden rounded-xl border border-slate-100 bg-white shadow-sm transition hover:-translate-y-1 hover:border-orange-100 hover:shadow-xl hover:shadow-slate-200/70"
          >
            <div class="h-32 bg-slate-100">
              <img v-if="shop.banner_url" :src="imageVariantURL(shop.banner_url, 'large')" :alt="shop.shop_name" loading="lazy" decoding="async" class="h-full w-full object-cover" />
            </div>
            <div class="p-4">
              <div class="-mt-10 mb-3 flex h-16 w-16 items-center justify-center overflow-hidden rounded-xl border-4 border-white bg-slate-100 text-xl font-black text-slate-500">
                <img v-if="shop.logo_url" :src="imageVariantURL(shop.logo_url, 'thumb')" :alt="shop.shop_name" loading="lazy" decoding="async" class="h-full w-full object-cover" />
                <span v-else>{{ shop.shop_name.slice(0, 1) }}</span>
              </div>
              <h3 class="text-base font-black text-slate-950">{{ shop.shop_name }}</h3>
              <p class="mt-2 line-clamp-2 text-sm leading-6 text-slate-500">{{ shop.description || t('homeBlocks.merchantHomepage') }}</p>
            </div>
          </RouterLink>
        </div>
        <p v-else class="rounded-xl bg-slate-50 p-8 text-center text-sm text-slate-500">{{ t('homeBlocks.noMerchants') }}</p>
      </div>

      <a
        v-else-if="block.type === 'promo_banner'"
        :href="promoLink(block) || '#'"
        class="grid overflow-hidden rounded-2xl bg-slate-950 text-white shadow-sm transition hover:-translate-y-0.5 hover:shadow-xl md:grid-cols-[1fr_360px]"
        @click="trackMarketingContentClick(block.tracking_key)"
      >
        <div class="flex flex-col justify-center p-6 sm:p-10">
          <p class="text-xs font-bold uppercase tracking-[0.22em] text-orange-300">{{ t('homeBlocks.promotion') }}</p>
          <h2 class="mt-2 text-xl font-black leading-tight sm:text-2xl lg:text-3xl">{{ localizedCMSDefault(block.title, 'homeBlocks.promoBanner') }}</h2>
          <p class="mt-3 text-sm leading-7 text-slate-300">{{ localizedCMSDefault(block.subtitle, 'homeBlocks.defaultDealsSubtitle') }}</p>
          <span
            v-if="block.buttonText && block.buttonLink"
            class="mt-6 inline-flex w-fit rounded-xl bg-orange-500 px-5 py-3 text-sm font-black text-white transition hover:bg-orange-400"
          >
            {{ localizedCMSDefault(block.buttonText, 'homeBlocks.defaultShopDeals') }}
          </span>
        </div>
        <div class="min-h-64 bg-slate-800">
          <ProductImage :src="promoImage(block)" :alt="block.title || ''" class="h-full min-h-64 w-full" image-class="h-full w-full object-cover" />
        </div>
      </a>

      <footer v-else-if="block.type === 'footer_links'" class="rounded-2xl bg-slate-950 p-6 text-white shadow-sm sm:p-8">
        <div class="grid gap-8 md:grid-cols-[1.2fr_2fr]">
          <div>
            <p class="text-xs font-bold uppercase tracking-[0.22em] text-orange-300">{{ t('homeBlocks.marketplace') }}</p>
            <h2 class="mt-2 text-2xl font-black">{{ localizedCMSDefault(block.title, 'homeBlocks.defaultFooterLinks') }}</h2>
          </div>
          <div class="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
            <div v-for="(group, groupIndex) in block.items || []" :key="group.tracking_key || groupIndex">
              <h3 class="text-sm font-black text-slate-100">{{ localizedCMSDefault(group.title, 'homeBlocks.links') }}</h3>
              <div class="mt-3 grid gap-2">
                <a v-for="(link, linkIndex) in group.links || []" :key="link.tracking_key || linkIndex" :href="localizedCMSLink(link.link || '#')" class="text-sm font-semibold text-slate-400 hover:text-orange-300">
                  {{ localizedCMSDefault(link.name, 'homeBlocks.link') }}
                </a>
              </div>
            </div>
          </div>
        </div>
      </footer>

      <div v-else-if="block.type === 'contact'" class="grid gap-6 rounded-2xl bg-emerald-950 p-6 text-white shadow-sm md:grid-cols-[1.1fr_0.9fr] sm:p-8">
        <div>
          <p class="text-xs font-bold uppercase tracking-[0.22em] text-emerald-300">{{ t('homeBlocks.contact') }}</p>
          <h2 class="mt-2 text-3xl font-black">{{ localizedCMSDefault(block.title, 'homeBlocks.contactUs') }}</h2>
          <p class="mt-3 whitespace-pre-line text-base leading-7 text-emerald-50 sm:mt-4 sm:leading-8">{{ localizedCMSDefault(block.content, 'homeBlocks.defaultContactContent') }}</p>
        </div>
        <div class="grid content-center gap-3">
          <a
            v-if="block.phone && contactPhoneHref(block.phone)"
            :href="contactPhoneHref(block.phone)"
            class="rounded-xl bg-white/10 p-4 transition hover:bg-white/15"
            @click="trackMarketingContact(block, 'phone_click')"
          >
            <p class="text-xs font-bold text-emerald-200">{{ t('homeBlocks.phone') }}</p>
            <p class="mt-1 text-base font-black">{{ block.phone }}</p>
          </a>
          <a
            v-if="block.whatsapp && contactWhatsAppHref(block.whatsapp)"
            :href="contactWhatsAppHref(block.whatsapp)"
            target="_blank"
            rel="noopener noreferrer"
            class="rounded-xl bg-white/10 p-4 transition hover:bg-white/15"
            @click="trackMarketingContact(block, 'whatsapp_click')"
          >
            <p class="text-xs font-bold text-emerald-200">WhatsApp</p>
            <p class="mt-1 text-base font-black">{{ block.whatsapp }}</p>
          </a>
          <div v-if="block.address" class="rounded-xl bg-white/10 p-4">
            <p class="text-xs font-bold text-emerald-200">{{ t('homeBlocks.address') }}</p>
            <p class="mt-1 text-base font-black">{{ block.address }}</p>
          </div>
        </div>
      </div>

      <div v-else-if="block.type === 'text'" class="rounded-2xl bg-white p-6 shadow-sm sm:p-8">
        <h2 class="text-2xl font-black text-slate-950">{{ block.title }}</h2>
        <p class="mt-3 whitespace-pre-line text-base leading-7 text-slate-600 sm:mt-4 sm:leading-8">{{ block.content }}</p>
      </div>

      <div v-else-if="block.type === 'image'" class="overflow-hidden rounded-2xl bg-white p-4 shadow-sm">
        <h2 v-if="block.title" class="mb-4 px-2 text-2xl font-black text-slate-950">{{ block.title }}</h2>
        <ProductImage :src="imageVariantURL(block.image, 'large')" :alt="block.alt || block.title || ''" class="max-h-[520px] w-full rounded-xl" image-class="h-full w-full object-cover" />
      </div>
    </section>
    </template>
  </div>
</template>
