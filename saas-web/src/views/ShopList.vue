<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink, useRoute } from 'vue-router'
import StorefrontHeader from '../components/storefront/StorefrontHeader.vue'
import { getPublicShops, type PublicShopProfile } from '../api/storefront'
import { currentLocale, withLocalePath } from '../utils/localePath.ts'

const route = useRoute()
const { t } = useI18n()
const shops = ref<PublicShopProfile[]>([])
const loading = ref(true)
const errorMessage = ref('')
const activeLocale = computed(() => currentLocale(route))
let loadSequence = 0

onMounted(() => {
  void loadShops()
})

watch(activeLocale, () => {
  void loadShops()
})

async function loadShops() {
  const sequence = ++loadSequence
  loading.value = true
  errorMessage.value = ''
  try {
    const page = await getPublicShops({ page: 1, page_size: 48 })
    if (sequence !== loadSequence) {
      return
    }
    shops.value = page.items
  } catch (error) {
    if (sequence !== loadSequence) {
      return
    }
    errorMessage.value = error instanceof Error ? error.message : t('shopList.loadFailed')
  } finally {
    if (sequence === loadSequence) {
      loading.value = false
    }
  }
}
</script>

<template>
  <main class="min-h-screen bg-slate-50 text-slate-950">
    <StorefrontHeader />

    <section class="mx-auto max-w-7xl px-4 py-5 sm:px-6 lg:px-8">
      <div class="mb-5 rounded-2xl border border-slate-200 bg-white px-4 py-4 shadow-sm sm:px-5">
        <p class="text-xs font-bold uppercase tracking-[0.2em] text-orange-500">{{ t('shopList.eyebrow') }}</p>
        <h2 class="text-3xl font-black text-slate-950">{{ t('shopList.title') }}</h2>
        <p class="mt-2 text-sm text-slate-500">{{ t('shopList.subtitle') }}</p>
      </div>

      <div v-if="loading" class="rounded-3xl bg-white p-8 text-center text-sm font-semibold text-slate-500 shadow-sm">{{ t('shopList.loading') }}</div>
      <div v-else-if="errorMessage" class="rounded-3xl bg-red-50 p-8 text-center text-sm font-semibold text-red-600">{{ errorMessage }}</div>

      <div v-else-if="shops.length" class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <RouterLink
          v-for="shop in shops"
          :key="shop.slug"
          :to="withLocalePath(activeLocale, `/shops/${shop.slug}`)"
          class="overflow-hidden rounded-3xl border border-slate-100 bg-white shadow-sm transition duration-200 hover:-translate-y-1 hover:border-orange-100 hover:shadow-xl hover:shadow-slate-200/70"
        >
          <div class="h-36 bg-slate-100">
            <img v-if="shop.banner_url" :src="shop.banner_url" :alt="shop.shop_name" class="h-full w-full object-cover" />
          </div>
          <div class="p-4">
            <div class="-mt-10 mb-3 flex h-16 w-16 items-center justify-center overflow-hidden rounded-2xl border-4 border-white bg-slate-100 text-xl font-black text-slate-500">
              <img v-if="shop.logo_url" :src="shop.logo_url" :alt="shop.shop_name" class="h-full w-full object-cover" />
              <span v-else>{{ shop.shop_name.slice(0, 1) }}</span>
            </div>
            <h3 class="text-base font-black text-slate-950">{{ shop.shop_name }}</h3>
            <p class="mt-2 line-clamp-2 text-sm leading-6 text-slate-500">{{ shop.description || t('shopList.shopPage') }}</p>
          </div>
        </RouterLink>
      </div>

      <p v-else class="rounded-3xl bg-white p-8 text-center text-sm text-slate-500 shadow-sm">{{ t('shopList.empty') }}</p>
    </section>
  </main>
</template>
