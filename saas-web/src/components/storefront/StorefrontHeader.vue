<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import StoreLanguageSwitcher from '../StoreLanguageSwitcher.vue'
import StoreAuthStatus from '../StoreAuthStatus.vue'
import PreviewModeDialog from './PreviewModeDialog.vue'
import StorefrontFloatingBar from './StorefrontFloatingBar.vue'
import { useCartStore } from '../../stores/cart'
import { storefrontAuthEnabled, usePreviewMode } from '../../composables/usePreviewMode'
import { reservationEnabled } from '../../config/storefront'
import { currentLocale, stripLocalePrefix, withLocalePath } from '../../utils/localePath.ts'

const { t } = useI18n()
defineProps<{
  wide?: boolean
  reservationMode?: boolean
}>()

const route = useRoute()
const router = useRouter()
const cartStore = useCartStore()
const { showPreviewMode } = usePreviewMode()
const mobileMenuOpen = ref(false)
const searchKeyword = ref(queryStringValue(route.query.keyword))

const cartQuantity = computed(() => cartStore.totalQuantity)
const activeLocale = computed(() => currentLocale(route))
const t6MallRoute = computed(() => ({ path: localizedPath('/mall'), query: { source: 't6' } }))

function localizedPath(path: string) {
  return withLocalePath(activeLocale.value, path)
}

function queryStringValue(value: unknown) {
  if (Array.isArray(value)) {
    return typeof value[0] === 'string' ? value[0] : ''
  }
  return typeof value === 'string' ? value : ''
}

function isMallSourceActive(source: 'all' | 't6') {
  if (stripLocalePrefix(route.path) !== '/mall') {
    return false
  }
  const currentSource = queryStringValue(route.query.source).trim().toLowerCase()
  return source === 't6' ? currentSource === 't6' : currentSource !== 't6'
}

function isPathActive(path: string) {
  const storefrontPath = stripLocalePrefix(route.path)
  return path === '/' ? storefrontPath === '/' : storefrontPath === path || storefrontPath.startsWith(`${path}/`)
}

function desktopNavClass(active: boolean) {
  return [
    'rounded-xl px-3 py-2 hover:bg-slate-100 hover:text-orange-600',
    active ? 'bg-orange-50 text-orange-700' : '',
  ]
}

function mobileNavClass(active: boolean) {
  return [
    'block rounded-xl px-4 py-3 text-base font-bold hover:bg-slate-100 hover:text-orange-600',
    active ? 'bg-orange-50 text-orange-700' : 'text-slate-700',
  ]
}

watch(
  () => route.query.keyword,
  (value) => {
    searchKeyword.value = queryStringValue(value)
  },
)

function closeMobileMenu() {
  mobileMenuOpen.value = false
}

function toggleMobileMenu() {
  mobileMenuOpen.value = !mobileMenuOpen.value
}

function showPreviewFromMenu() {
  closeMobileMenu()
  showPreviewMode()
}

function goSearch() {
  const keyword = searchKeyword.value.trim()
  if (!keyword && stripLocalePrefix(route.path) !== '/mall') {
    return
  }
  const query: Record<string, string> = {}
  if (keyword) {
    query.keyword = keyword
  }
  const platformCategoryID = queryStringValue(route.query.platform_category_id)
  if (stripLocalePrefix(route.path) === '/mall' && platformCategoryID) {
    query.platform_category_id = platformCategoryID
  }
  router.push({ path: localizedPath('/mall'), query })
  closeMobileMenu()
}
</script>

<template>
  <header class="sticky top-0 z-30 border-b border-slate-200 bg-white/95 backdrop-blur">
    <div
      class="mx-auto flex items-center gap-3 px-4 py-3 sm:px-6 lg:px-8"
      :class="wide ? 'max-w-screen-2xl' : 'max-w-7xl'"
    >
      <!-- Logo / site name -->
      <RouterLink :to="localizedPath('/')" class="min-w-0 flex-1 md:flex-none">
        <h1 class="truncate text-lg font-black text-slate-950 sm:text-2xl">Ghana Chinatown</h1>
      </RouterLink>

      <!-- Desktop search -->
      <form class="mx-4 hidden min-w-0 max-w-xl flex-1 md:flex" @submit.prevent="goSearch">
        <div class="flex w-full rounded-xl border border-slate-200 bg-slate-50 transition focus-within:border-orange-300 focus-within:bg-white focus-within:ring-2 focus-within:ring-orange-100">
          <input
            v-model="searchKeyword"
            type="search"
            class="min-w-0 flex-1 rounded-l-xl bg-transparent px-4 py-2.5 text-sm text-slate-900 outline-none placeholder:text-slate-400"
            :placeholder="t('home.searchPlaceholder')"
          />
          <button type="submit" class="rounded-r-xl bg-orange-500 px-4 text-sm font-bold text-white hover:bg-orange-600">
            {{ t('nav.search') }}
          </button>
        </div>
      </form>

      <!-- Desktop navigation -->
      <nav class="hidden items-center gap-2 text-sm font-semibold text-slate-600 md:flex lg:gap-4">
        <RouterLink :to="localizedPath('/')" :class="desktopNavClass(isPathActive('/'))">
          {{ t('nav.home') }}
        </RouterLink>
        <RouterLink :to="localizedPath('/mall')" :class="desktopNavClass(isMallSourceActive('all'))">
          {{ t('nav.allProducts') }}
        </RouterLink>
        <RouterLink :to="t6MallRoute" :class="desktopNavClass(isMallSourceActive('t6'))">
          {{ t('nav.t6Mall') }}
        </RouterLink>
        <RouterLink v-if="reservationEnabled" :to="localizedPath('/reservations')" :class="desktopNavClass(isPathActive('/reservations'))" data-testid="reservation-navigation">
          {{ t('nav.reservations') }}
        </RouterLink>
        <RouterLink v-if="!reservationMode" :to="localizedPath('/cart')" :class="['relative', desktopNavClass(isPathActive('/cart'))]">
          {{ t('nav.cart') }}
          <span
            v-if="cartQuantity"
            class="absolute -right-2 -top-2 min-w-5 rounded-full bg-orange-500 px-1.5 py-0.5 text-center text-[11px] font-black text-white"
          >
            {{ cartQuantity }}
          </span>
        </RouterLink>
        <RouterLink v-if="!reservationMode && storefrontAuthEnabled" :to="localizedPath('/orders')" :class="desktopNavClass(isPathActive('/orders'))">
          {{ t('nav.orders') }}
        </RouterLink>
        <button v-else-if="!reservationMode" type="button" :class="desktopNavClass(isPathActive('/orders'))" @click="showPreviewMode">
          {{ t('nav.orders') }}
        </button>
        <StoreLanguageSwitcher />
        <StoreAuthStatus />
      </nav>

      <!-- Mobile actions -->
      <div class="flex shrink-0 items-center gap-1 md:hidden">
        <StoreAuthStatus />
        <RouterLink v-if="!reservationMode" :to="localizedPath('/cart')" class="relative rounded-xl p-2 text-slate-700 hover:bg-slate-100">
          <svg xmlns="http://www.w3.org/2000/svg" class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M3 3h2l.4 2M7 13h10l4-8H5.4M7 13L5.4 5M7 13l-2.293 2.293c-.63.63-.184 1.707.707 1.707H17m0 0a2 2 0 100 4 2 2 0 000-4zm-8 2a2 2 0 11-4 0 2 2 0 014 0z" />
          </svg>
          <span
            v-if="cartQuantity"
            class="absolute right-0 top-0 min-w-5 rounded-full bg-orange-500 px-1 py-0 text-center text-[10px] font-black text-white"
          >
            {{ cartQuantity }}
          </span>
        </RouterLink>

        <button
          type="button"
          class="rounded-xl p-2 text-slate-700 hover:bg-slate-100"
          :aria-expanded="mobileMenuOpen"
          :aria-label="t('nav.menu')"
          @click="toggleMobileMenu"
        >
          <svg v-if="!mobileMenuOpen" xmlns="http://www.w3.org/2000/svg" class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M4 6h16M4 12h16M4 18h16" />
          </svg>
          <svg v-else xmlns="http://www.w3.org/2000/svg" class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>
    </div>

    <!-- Mobile search bar -->
    <form class="border-t border-slate-100 px-4 py-2 md:hidden" @submit.prevent="goSearch">
      <div class="flex rounded-xl border border-slate-200 bg-slate-50 transition focus-within:border-orange-300 focus-within:bg-white focus-within:ring-2 focus-within:ring-orange-100">
        <input
          v-model="searchKeyword"
          type="search"
          class="min-w-0 flex-1 rounded-l-xl bg-transparent px-3 py-2 text-sm text-slate-900 outline-none placeholder:text-slate-400"
          :placeholder="t('home.searchPlaceholder')"
        />
        <button type="submit" class="rounded-r-xl bg-orange-500 px-3 text-sm font-bold text-white hover:bg-orange-600">
          {{ t('nav.search') }}
        </button>
      </div>
    </form>
  </header>

  <!-- Mobile menu drawer -->
  <Transition
    enter-active-class="transition duration-200 ease-out"
    enter-from-class="opacity-0"
    enter-to-class="opacity-100"
    leave-active-class="transition duration-150 ease-in"
    leave-from-class="opacity-100"
    leave-to-class="opacity-0"
  >
    <div
      v-if="mobileMenuOpen"
      class="fixed inset-0 z-40 bg-slate-950/40 backdrop-blur-sm md:hidden"
      @click="closeMobileMenu"
    />
  </Transition>

  <Transition
    enter-active-class="transition duration-200 ease-out"
    enter-from-class="translate-x-full"
    enter-to-class="translate-x-0"
    leave-active-class="transition duration-150 ease-in"
    leave-from-class="translate-x-0"
    leave-to-class="translate-x-full"
  >
    <div
      v-if="mobileMenuOpen"
      class="fixed inset-y-0 right-0 z-50 w-72 max-w-[80vw] overflow-y-auto border-l border-slate-200 bg-white shadow-2xl md:hidden"
    >
      <div class="flex items-center justify-between border-b border-slate-100 p-4">
        <p class="text-xs font-semibold uppercase tracking-[0.28em] text-orange-500">{{ t('nav.menu') }}</p>
        <button type="button" class="rounded-xl p-2 text-slate-600 hover:bg-slate-100" @click="closeMobileMenu">
          <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>

      <nav class="space-y-1 p-4">
        <RouterLink :to="localizedPath('/')" :class="mobileNavClass(isPathActive('/'))" @click="closeMobileMenu">
          {{ t('nav.home') }}
        </RouterLink>
        <RouterLink :to="localizedPath('/mall')" :class="mobileNavClass(isMallSourceActive('all'))" @click="closeMobileMenu">
          {{ t('nav.allProducts') }}
        </RouterLink>
        <RouterLink :to="t6MallRoute" :class="mobileNavClass(isMallSourceActive('t6'))" @click="closeMobileMenu">
          {{ t('nav.t6Mall') }}
        </RouterLink>
        <RouterLink v-if="reservationEnabled" :to="localizedPath('/reservations')" :class="mobileNavClass(isPathActive('/reservations'))" data-testid="reservation-navigation" @click="closeMobileMenu">
          {{ t('nav.reservations') }}
        </RouterLink>
        <RouterLink :to="localizedPath('/merchant/join')" :class="mobileNavClass(isPathActive('/merchant/join'))" @click="closeMobileMenu">
          {{ t('nav.openShop') }}
        </RouterLink>
        <RouterLink v-if="!reservationMode" :to="localizedPath('/cart')" :class="mobileNavClass(isPathActive('/cart'))" @click="closeMobileMenu">
          {{ t('nav.cart') }}
          <span v-if="cartQuantity" class="ml-2 rounded-full bg-orange-500 px-2 py-0.5 text-xs font-black text-white">{{ cartQuantity }}</span>
        </RouterLink>
        <RouterLink v-if="!reservationMode && storefrontAuthEnabled" :to="localizedPath('/orders')" :class="mobileNavClass(isPathActive('/orders'))" @click="closeMobileMenu">
          {{ t('nav.orders') }}
        </RouterLink>
        <button v-else-if="!reservationMode" type="button" :class="['w-full text-left', mobileNavClass(isPathActive('/orders'))]" @click="showPreviewFromMenu">
          {{ t('nav.orders') }}
        </button>
      </nav>

      <div class="border-t border-slate-100 p-4">
        <div class="mb-3 flex items-center justify-between rounded-xl border border-slate-200 px-4 py-3">
          <span class="text-sm font-bold text-slate-600">{{ t('nav.language') }}</span>
          <StoreLanguageSwitcher />
        </div>

        <StoreAuthStatus />
      </div>
    </div>
  </Transition>

  <PreviewModeDialog />
  <StorefrontFloatingBar v-if="!reservationMode" />
</template>
