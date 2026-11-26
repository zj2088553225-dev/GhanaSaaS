<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import HomeBlockRenderer from '../components/site-blocks/HomeBlockRenderer.vue'
import StorefrontHeader from '../components/storefront/StorefrontHeader.vue'
import StorefrontHolidayCalendar from '../components/storefront/StorefrontHolidayCalendar.vue'
import HomeCategoryNav from '../components/storefront/HomeCategoryNav.vue'
import HeroCarousel from '../components/site-blocks/HeroCarousel.vue'
import { getPublishedHomeSitePage, type PublicSitePage } from '../api/sitePages'
import { getStorefrontPlatformCategoryChildren, type PlatformCategory } from '../api/platformCategories'
import { homeSeoMeta, useSeoMeta } from '../composables/useSeoMeta'
import { currentLocale, withLocalePath } from '../utils/localePath.ts'

const route = useRoute()
const page = ref<PublicSitePage | null>(null)
const platformCategories = ref<PlatformCategory[]>([])
const loading = ref(true)
const errorMessage = ref('')
const { t } = useI18n()
const activeLocale = computed(() => currentLocale(route))
useSeoMeta(computed(() => homeSeoMeta(activeLocale.value)))
let pageLoadSequence = 0
let categoryLoadSequence = 0

const hasPublishedBlocks = computed(() => Boolean(page.value?.content?.blocks?.length))

const heroCarouselBlocks = computed(() =>
  (page.value?.content?.blocks || []).filter((block) => block.type === 'hero_carousel'),
)
const heroBlock = computed(() => heroCarouselBlocks.value[0])
const nonHeroBlocks = computed(() =>
  (page.value?.content?.blocks || []).filter(
    (block) =>
      block.type !== 'hero_carousel' &&
      block.type !== 'category_nav',
  ),
)

function localizedPath(path: string) {
  return withLocalePath(activeLocale.value, path)
}

onMounted(() => {
  void reloadPageData()
})

watch(activeLocale, () => {
  void reloadPageData()
})

async function reloadPageData() {
  void loadPage()
  void loadRootCategories()
}

async function loadPage() {
  const sequence = ++pageLoadSequence
  loading.value = true
  errorMessage.value = ''
  try {
    const loadedPage = await getPublishedHomeSitePage().catch((error) => {
      const message = error instanceof Error ? error.message : ''
      if (/404|not found|page not found/i.test(message)) {
        return null
      }
      throw error
    })
    if (sequence !== pageLoadSequence) {
      return
    }
    page.value = loadedPage
  } catch (error) {
    if (sequence !== pageLoadSequence) {
      return
    }
    errorMessage.value = error instanceof Error ? error.message : t('home.publishHint')
  } finally {
    if (sequence === pageLoadSequence) {
      loading.value = false
    }
  }
}

async function loadRootCategories() {
  const sequence = ++categoryLoadSequence
  try {
    const categories = await getStorefrontPlatformCategoryChildren(null, { maxLevel: 2 })
    if (sequence !== categoryLoadSequence) {
      return
    }
    platformCategories.value = categories
  } catch {
    if (sequence !== categoryLoadSequence) {
      return
    }
    platformCategories.value = []
  }
}
</script>

<template>
  <main class="min-h-screen bg-slate-50 text-slate-950">
    <StorefrontHeader wide />

    <div v-if="loading" class="mx-auto max-w-screen-2xl animate-pulse space-y-4 px-4 py-5 sm:px-6 lg:px-8">
      <div class="h-[200px] rounded-2xl bg-slate-200 sm:h-[280px] lg:h-[380px]" />
      <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
        <div v-for="index in 5" :key="index" class="aspect-square rounded-xl bg-slate-200" />
      </div>
      <p class="text-center text-sm font-semibold text-slate-500">{{ t('home.loadingPage') }}</p>
    </div>

    <div v-else-if="errorMessage" class="mx-auto max-w-3xl px-4 py-24 sm:px-6 lg:px-8">
      <div class="rounded-2xl bg-white p-8 text-center shadow-sm">
        <h2 class="text-2xl font-black text-slate-950">{{ t('home.unpublished') }}</h2>
        <p class="mt-3 text-sm text-slate-500">{{ errorMessage }}</p>
        <p class="mt-2 text-sm text-slate-500">{{ t('home.publishHint') }}</p>
      </div>
    </div>

    <template v-else-if="hasPublishedBlocks">
      <!-- Mobile category nav -->
      <section class="mx-auto max-w-screen-2xl px-4 pt-4 sm:px-6 xl:hidden lg:px-8">
        <HomeCategoryNav :categories="platformCategories" compact />
      </section>

      <!-- Hero section: left categories, middle carousel, right holiday calendar -->
      <section class="mx-auto max-w-screen-2xl px-4 py-4 sm:px-6 lg:px-8">
        <div class="grid min-w-0 gap-4 xl:grid-cols-[220px_minmax(0,1fr)_260px] 2xl:grid-cols-[232px_minmax(0,1fr)_272px]">
          <!-- Left categories (desktop) -->
          <div class="hidden min-w-0 xl:block">
            <HomeCategoryNav :categories="platformCategories" />
          </div>

          <!-- Middle carousel -->
          <div class="min-w-0">
            <HeroCarousel v-if="heroBlock" :block="heroBlock" />
            <div v-else class="flex h-[200px] items-center justify-center rounded-xl bg-slate-100 text-sm text-slate-500 sm:h-[280px] lg:h-[380px]">
              {{ t('home.noCarousel') }}
            </div>
          </div>

          <!-- Holiday calendar: right rail on desktop, below the hero on mobile/tablet. -->
          <div class="min-w-0 xl:block">
            <StorefrontHolidayCalendar />
            <RouterLink
              :to="localizedPath('/merchant/join')"
              class="mt-2 flex min-h-9 items-center justify-center rounded-lg bg-white px-3 text-xs font-black text-slate-600 shadow-sm hover:bg-orange-50 hover:text-orange-700"
            >
              {{ t('home.merchants') }}
            </RouterLink>
          </div>
        </div>
      </section>

      <!-- Remaining blocks -->
      <HomeBlockRenderer :blocks="nonHeroBlocks" />
    </template>

    <div v-else class="mx-auto max-w-3xl px-4 py-24 sm:px-6 lg:px-8">
      <div class="rounded-2xl bg-white p-8 text-center shadow-sm">
        <h2 class="text-2xl font-black text-slate-950">{{ t('home.unpublished') }}</h2>
        <p class="mt-3 text-sm text-slate-500">{{ t('home.publishHint') }}</p>
      </div>
    </div>
  </main>
</template>
