<script setup lang="ts">
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import StorefrontHeader from '../components/storefront/StorefrontHeader.vue'
import PreviewModeNotice from '../components/storefront/PreviewModeNotice.vue'
import { storefrontAuthEnabled } from '../composables/usePreviewMode'
import { currentLocale, withLocalePath } from '../utils/localePath.ts'

const route = useRoute()
const { t } = useI18n()
const orderNo = typeof route.query.order_no === 'string' ? route.query.order_no : ''
</script>

<template>
  <main class="min-h-screen bg-slate-50 text-slate-950">
    <StorefrontHeader />
    <PreviewModeNotice v-if="!storefrontAuthEnabled" />
    <section v-else class="mx-auto flex min-h-[calc(100vh-9rem)] max-w-lg items-center px-4 py-10">
      <div class="w-full rounded-2xl border border-slate-200 bg-white p-8 text-center shadow-sm">
        <p class="mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-emerald-50 text-3xl font-black text-emerald-600">✓</p>
        <h1 class="mt-6 text-2xl font-black">{{ t('messages.orderSuccessTitle') }}</h1>
        <p class="mt-3 text-sm leading-6 text-slate-500">{{ t('messages.orderSuccessDescription') }}</p>
        <p v-if="orderNo" class="mt-5 rounded-2xl bg-slate-50 px-4 py-3 text-sm font-black text-slate-700">{{ t('messages.orderNo', { orderNo }) }}</p>
        <div class="mt-6 grid gap-3 sm:grid-cols-2">
          <RouterLink :to="withLocalePath(currentLocale(route), '/mall')" class="flex min-h-12 items-center justify-center rounded-2xl bg-orange-500 px-5 py-3 text-sm font-black text-white shadow-lg shadow-orange-200 hover:bg-orange-600">
            {{ t('messages.continueShopping') }}
          </RouterLink>
          <RouterLink :to="withLocalePath(currentLocale(route), '/cart')" class="flex min-h-12 items-center justify-center rounded-2xl border border-slate-200 px-5 py-3 text-sm font-black text-slate-700 hover:bg-slate-50">
            {{ t('messages.viewCart') }}
          </RouterLink>
        </div>
      </div>
    </section>
  </main>
</template>
