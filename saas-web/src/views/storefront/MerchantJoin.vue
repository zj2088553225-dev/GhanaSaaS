<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import StorefrontHeader from '../../components/storefront/StorefrontHeader.vue'
import { merchantJoinSeoMeta, useSeoMeta } from '../../composables/useSeoMeta'
import { submitMerchantLead, type MerchantLeadPayload } from '../../api/merchantLeads'
import { currentLocale } from '../../utils/localePath.ts'
import { trackStorefrontEvent } from '../../utils/storefrontAnalytics.ts'

interface MerchantJoinForm {
  contact_name: string
  shop_name: string
  whatsapp: string
  phone: string
  category: string
  has_offline_shop: boolean
  city: string
  product_count: string
  message: string
}

const { t } = useI18n()
const route = useRoute()
const formSection = ref<HTMLElement | null>(null)
const submitting = ref(false)
const successMessage = ref('')
const errorMessage = ref('')
const merchantWhatsAppURL = import.meta.env.VITE_MERCHANT_WHATSAPP_URL || ''

const form = reactive<MerchantJoinForm>({
  contact_name: '',
  shop_name: '',
  whatsapp: '',
  phone: '',
  category: '',
  has_offline_shop: false,
  city: '',
  product_count: '',
  message: '',
})

const fieldErrors = reactive<Record<keyof MerchantJoinForm, string>>({
  contact_name: '',
  shop_name: '',
  whatsapp: '',
  phone: '',
  category: '',
  has_offline_shop: '',
  city: '',
  product_count: '',
  message: '',
})

const advantages = [
  { titleKey: 'merchantJoin.advantages.storefront.title', descriptionKey: 'merchantJoin.advantages.storefront.description' },
  { titleKey: 'merchantJoin.advantages.exposure.title', descriptionKey: 'merchantJoin.advantages.exposure.description' },
  { titleKey: 'merchantJoin.advantages.customers.title', descriptionKey: 'merchantJoin.advantages.customers.description' },
  { titleKey: 'merchantJoin.advantages.management.title', descriptionKey: 'merchantJoin.advantages.management.description' },
]

const merchantTypes = [
  'merchantJoin.types.retail',
  'merchantJoin.types.wholesalers',
  'merchantJoin.types.brands',
  'merchantJoin.types.services',
]

const processSteps = [
  'merchantJoin.process.submit',
  'merchantJoin.process.review',
  'merchantJoin.process.setup',
  'merchantJoin.process.publish',
]

const benefits = [
  'merchantJoin.benefits.shopHomepage',
  'merchantJoin.benefits.productListing',
  'merchantJoin.benefits.traffic',
  'merchantJoin.benefits.seo',
  'merchantJoin.benefits.imageOptimization',
  'merchantJoin.benefits.futureFeatures',
]

const previewCategories = [
  'merchantJoin.preview.categoryItems.electronics',
  'merchantJoin.preview.categoryItems.homeAppliances',
  'merchantJoin.preview.categoryItems.healthBeauty',
  'merchantJoin.preview.categoryItems.carAccessories',
  'merchantJoin.preview.categoryItems.motherBaby',
]

const previewProducts = [
  'bg-orange-100',
  'bg-emerald-100',
  'bg-sky-100',
  'bg-amber-100',
]

useSeoMeta(computed(() => merchantJoinSeoMeta(currentLocale(route))))

function scrollToForm() {
  formSection.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

function trackMerchantWhatsAppClick() {
  trackStorefrontEvent({
    event_name: 'whatsapp_click',
    source_type: 'direct',
    source_id: 'merchant_join',
    page_type: 'other',
  })
}

function clearFieldErrors() {
  for (const key of Object.keys(fieldErrors) as Array<keyof MerchantJoinForm>) {
    fieldErrors[key] = ''
  }
}

function validateForm() {
  clearFieldErrors()
  let valid = true
  for (const key of ['contact_name', 'shop_name', 'whatsapp', 'category'] as const) {
    if (!form[key].trim()) {
      fieldErrors[key] = t(`merchantJoin.form.errors.${key}`)
      valid = false
    }
  }
  return valid
}

function resetForm() {
  form.contact_name = ''
  form.shop_name = ''
  form.whatsapp = ''
  form.phone = ''
  form.category = ''
  form.has_offline_shop = false
  form.city = ''
  form.product_count = ''
  form.message = ''
  clearFieldErrors()
}

function payload(): MerchantLeadPayload {
  return {
    contact_name: form.contact_name.trim(),
    shop_name: form.shop_name.trim(),
    whatsapp: form.whatsapp.trim(),
    phone: form.phone.trim(),
    category: form.category.trim(),
    has_offline_shop: form.has_offline_shop,
    city: form.city.trim(),
    product_count: form.product_count.trim(),
    message: form.message.trim(),
  }
}

function friendlySubmitError(error: unknown) {
  const message = error instanceof Error ? error.message : ''
  if (/invalid input/i.test(message)) {
    return t('merchantJoin.form.invalid')
  }
  return message || t('merchantJoin.form.failed')
}

async function submitForm() {
  successMessage.value = ''
  errorMessage.value = ''
  if (!validateForm()) {
    errorMessage.value = t('merchantJoin.form.invalid')
    return
  }

  submitting.value = true
  try {
    await submitMerchantLead(payload())
    resetForm()
    successMessage.value = t('merchantJoin.form.success')
  } catch (error) {
    errorMessage.value = friendlySubmitError(error)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <main class="min-h-screen bg-slate-50 pb-24 text-slate-950">
    <StorefrontHeader wide />

    <section class="mx-auto grid max-w-screen-2xl gap-6 px-4 py-8 sm:px-6 lg:grid-cols-[minmax(0,1.1fr)_minmax(360px,0.9fr)] lg:px-8 lg:py-12">
      <div class="min-w-0 rounded-2xl bg-white p-6 shadow-sm sm:p-8 lg:p-10">
        <p class="text-xs font-black uppercase tracking-[0.26em] text-orange-600">{{ t('merchantJoin.hero.eyebrow') }}</p>
        <h1 class="mt-4 max-w-3xl text-3xl font-black leading-tight text-slate-950 sm:text-5xl">
          {{ t('merchantJoin.hero.title') }}
        </h1>
        <p class="mt-5 max-w-2xl text-base font-semibold leading-8 text-slate-600 sm:text-lg">
          {{ t('merchantJoin.hero.subtitle') }}
        </p>

        <div class="mt-7 flex flex-col gap-3 sm:flex-row sm:flex-wrap">
          <button
            type="button"
            class="inline-flex min-h-12 items-center justify-center rounded-xl bg-orange-500 px-6 py-3 text-sm font-black text-white shadow-lg shadow-orange-100 hover:bg-orange-600 disabled:cursor-not-allowed disabled:opacity-60 sm:w-auto"
            @click="scrollToForm"
          >
            {{ t('merchantJoin.hero.apply') }}
          </button>
          <a
            v-if="merchantWhatsAppURL"
            :href="merchantWhatsAppURL"
            target="_blank"
            rel="noopener noreferrer"
            class="inline-flex min-h-12 items-center justify-center rounded-xl border border-orange-200 bg-orange-50 px-6 py-3 text-sm font-black text-orange-700 hover:bg-orange-100 sm:w-auto"
            @click="trackMerchantWhatsAppClick"
          >
            {{ t('merchantJoin.hero.whatsapp') }}
          </a>
        </div>
      </div>

      <div class="min-w-0 rounded-2xl bg-white p-5 shadow-sm sm:p-6">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <p class="text-xs font-black uppercase tracking-[0.2em] text-orange-600">GHANA CHINATOWN</p>
            <p class="mt-1 text-xl font-black text-slate-950">{{ t('merchantJoin.preview.title') }}</p>
          </div>
          <div class="shrink-0 rounded-full bg-orange-500 px-3 py-1 text-xs font-black text-white">{{ t('merchantJoin.preview.live') }}</div>
        </div>

        <div class="mt-4 overflow-hidden rounded-xl border border-slate-200 bg-slate-50">
          <div class="flex min-w-0 items-center gap-2 border-b border-slate-200 bg-white px-3 py-2">
            <p class="shrink-0 text-[11px] font-black text-slate-950">Ghana Chinatown</p>
            <div class="hidden h-6 min-w-0 flex-1 items-center rounded-full bg-slate-100 px-3 text-[10px] font-bold text-slate-400 sm:flex">
              {{ t('home.searchPlaceholder') }}
            </div>
            <div class="ml-auto hidden shrink-0 items-center gap-2 text-[10px] font-black text-slate-500 md:flex">
              <span>{{ t('nav.home') }}</span>
              <span>{{ t('nav.allProducts') }}</span>
              <span>{{ t('nav.t6Mall') }}</span>
            </div>
          </div>

          <div class="grid gap-2 p-3 sm:grid-cols-[108px_minmax(0,1fr)_96px]">
            <div class="min-w-0 rounded-lg bg-white p-3 shadow-sm">
              <p class="truncate text-[10px] font-black text-slate-700">{{ t('merchantJoin.preview.categories') }}</p>
              <div class="mt-2 space-y-1.5">
                <div v-for="item in previewCategories" :key="item" class="truncate rounded-md bg-slate-50 px-2 py-1.5 text-[10px] font-bold text-slate-500">
                  {{ t(item) }}
                </div>
              </div>
            </div>

            <div class="relative min-h-36 min-w-0 overflow-hidden rounded-lg bg-gradient-to-br from-orange-500 via-amber-400 to-rose-300 p-4 text-white shadow-sm">
              <div class="absolute right-4 top-4 h-14 w-14 rounded-full bg-white/20" />
              <div class="absolute bottom-3 right-10 h-8 w-20 rounded-full bg-white/15" />
              <p class="relative text-lg font-black leading-tight">{{ t('merchantJoin.preview.bannerTitle') }}</p>
              <p class="relative mt-1 text-sm font-black">{{ t('merchantJoin.preview.bannerSubtitle') }}</p>
              <div class="relative mt-5 flex gap-2">
                <div class="h-12 w-12 rounded-lg bg-white/25" />
                <div class="h-12 w-12 rounded-lg bg-white/25" />
                <div class="hidden h-12 w-12 rounded-lg bg-white/25 sm:block" />
              </div>
            </div>

            <div class="min-w-0 rounded-lg bg-white p-3 text-center shadow-sm">
              <div class="mx-auto flex h-10 w-10 items-center justify-center rounded-full bg-orange-100 text-lg">Hi</div>
              <p class="mt-2 text-[11px] font-black text-slate-900">{{ t('merchantJoin.preview.welcomeTitle') }}</p>
              <p class="mt-1 text-[10px] font-bold leading-4 text-slate-500">{{ t('merchantJoin.preview.welcomeSubtitle') }}</p>
              <div class="mt-3 grid grid-cols-2 gap-1.5">
                <div class="h-6 rounded-md bg-orange-50" />
                <div class="h-6 rounded-md bg-slate-50" />
              </div>
            </div>
          </div>

          <div class="border-t border-slate-200 bg-white p-3">
            <div class="flex items-center justify-between gap-3">
              <p class="text-xs font-black text-slate-900">{{ t('merchantJoin.preview.featuredTitle') }}</p>
              <div class="h-2 w-14 rounded-full bg-orange-200" />
            </div>
            <div class="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
              <div v-for="item in previewProducts" :key="item" class="min-w-0 rounded-lg border border-slate-100 bg-slate-50 p-2">
                <div class="aspect-square rounded-md" :class="item" />
                <div class="mt-2 h-2 w-4/5 rounded-full bg-slate-200" />
                <div class="mt-1.5 h-2 w-1/2 rounded-full bg-slate-200" />
                <p class="mt-2 truncate text-[10px] font-black text-orange-600">{{ t('storefront.contactForPrice') }}</p>
              </div>
            </div>
          </div>
        </div>

        <p class="mt-4 rounded-xl bg-orange-50 px-4 py-3 text-sm font-bold leading-6 text-orange-700">
          {{ t('merchantJoin.preview.note') }}
        </p>
      </div>
    </section>

    <section class="mx-auto max-w-screen-2xl px-4 py-4 sm:px-6 lg:px-8">
      <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <article v-for="item in advantages" :key="item.titleKey" class="rounded-2xl bg-white p-5 shadow-sm">
          <h2 class="text-base font-black text-slate-950">{{ t(item.titleKey) }}</h2>
          <p class="mt-2 text-sm font-semibold leading-6 text-slate-500">{{ t(item.descriptionKey) }}</p>
        </article>
      </div>
    </section>

    <section class="mx-auto grid max-w-screen-2xl gap-6 px-4 py-4 sm:px-6 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)] lg:px-8">
      <div class="rounded-2xl bg-white p-6 shadow-sm">
        <h2 class="text-xl font-black text-slate-950">{{ t('merchantJoin.types.title') }}</h2>
        <div class="mt-5 grid gap-3 sm:grid-cols-2">
          <div v-for="item in merchantTypes" :key="item" class="rounded-xl border border-slate-200 px-4 py-3 text-sm font-black text-slate-700">
            {{ t(item) }}
          </div>
        </div>
      </div>

      <div class="rounded-2xl bg-white p-6 shadow-sm">
        <h2 class="text-xl font-black text-slate-950">{{ t('merchantJoin.process.title') }}</h2>
        <div class="mt-5 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <div v-for="(item, index) in processSteps" :key="item" class="rounded-xl border border-slate-200 p-4">
            <span class="flex h-8 w-8 items-center justify-center rounded-full bg-orange-500 text-sm font-black text-white">{{ index + 1 }}</span>
            <p class="mt-3 text-sm font-black text-slate-800">{{ t(item) }}</p>
          </div>
        </div>
      </div>
    </section>

    <section class="mx-auto max-w-screen-2xl px-4 py-4 sm:px-6 lg:px-8">
      <div class="rounded-2xl bg-white p-6 shadow-sm">
        <h2 class="text-xl font-black text-slate-950">{{ t('merchantJoin.benefits.title') }}</h2>
        <div class="mt-5 grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          <div v-for="item in benefits" :key="item" class="flex min-w-0 gap-3 rounded-xl border border-slate-200 p-4">
            <span class="mt-1 h-2.5 w-2.5 shrink-0 rounded-full bg-orange-500" />
            <p class="min-w-0 text-sm font-semibold leading-6 text-slate-600">{{ t(item) }}</p>
          </div>
        </div>
      </div>
    </section>

    <section ref="formSection" class="mx-auto max-w-5xl scroll-mt-24 px-4 py-6 sm:px-6 lg:px-8">
      <div class="rounded-2xl bg-white p-6 shadow-sm sm:p-8">
        <div class="max-w-2xl">
          <h2 class="text-2xl font-black text-slate-950">{{ t('merchantJoin.form.title') }}</h2>
          <p class="mt-2 text-sm font-semibold leading-6 text-slate-500">{{ t('merchantJoin.form.subtitle') }}</p>
        </div>

        <form class="mt-6 grid gap-4 md:grid-cols-2" @submit.prevent="submitForm">
          <label class="block">
            <span class="text-sm font-black text-slate-700">{{ t('merchantJoin.form.contactName') }}</span>
            <input v-model="form.contact_name" maxlength="120" class="merchant-input" :aria-invalid="Boolean(fieldErrors.contact_name)" />
            <span v-if="fieldErrors.contact_name" class="merchant-error">{{ fieldErrors.contact_name }}</span>
          </label>

          <label class="block">
            <span class="text-sm font-black text-slate-700">{{ t('merchantJoin.form.shopName') }}</span>
            <input v-model="form.shop_name" maxlength="160" class="merchant-input" :aria-invalid="Boolean(fieldErrors.shop_name)" />
            <span v-if="fieldErrors.shop_name" class="merchant-error">{{ fieldErrors.shop_name }}</span>
          </label>

          <label class="block">
            <span class="text-sm font-black text-slate-700">{{ t('merchantJoin.form.whatsapp') }}</span>
            <input v-model="form.whatsapp" maxlength="64" class="merchant-input" inputmode="tel" :aria-invalid="Boolean(fieldErrors.whatsapp)" />
            <span v-if="fieldErrors.whatsapp" class="merchant-error">{{ fieldErrors.whatsapp }}</span>
          </label>

          <label class="block">
            <span class="text-sm font-black text-slate-700">{{ t('merchantJoin.form.phone') }}</span>
            <input v-model="form.phone" maxlength="64" class="merchant-input" inputmode="tel" />
          </label>

          <label class="block">
            <span class="text-sm font-black text-slate-700">{{ t('merchantJoin.form.category') }}</span>
            <input v-model="form.category" maxlength="120" class="merchant-input" :aria-invalid="Boolean(fieldErrors.category)" />
            <span v-if="fieldErrors.category" class="merchant-error">{{ fieldErrors.category }}</span>
          </label>

          <label class="block">
            <span class="text-sm font-black text-slate-700">{{ t('merchantJoin.form.city') }}</span>
            <input v-model="form.city" maxlength="120" class="merchant-input" />
          </label>

          <label class="block">
            <span class="text-sm font-black text-slate-700">{{ t('merchantJoin.form.productCount') }}</span>
            <input v-model="form.product_count" maxlength="64" class="merchant-input" />
          </label>

          <label class="flex items-center gap-3 rounded-xl border border-slate-200 px-4 py-3">
            <input v-model="form.has_offline_shop" type="checkbox" class="h-4 w-4 rounded border-slate-300 text-orange-500 focus:ring-orange-400" />
            <span class="text-sm font-black text-slate-700">{{ t('merchantJoin.form.hasOfflineShop') }}</span>
          </label>

          <label class="block md:col-span-2">
            <span class="text-sm font-black text-slate-700">{{ t('merchantJoin.form.message') }}</span>
            <textarea v-model="form.message" maxlength="1000" rows="4" class="merchant-input min-h-32 resize-y py-3" />
          </label>

          <div class="md:col-span-2">
            <div v-if="successMessage" class="mb-4 rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm font-bold text-emerald-700">
              {{ successMessage }}
            </div>
            <div v-if="errorMessage" class="mb-4 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm font-bold text-red-600">
              {{ errorMessage }}
            </div>
            <button
              type="submit"
              class="inline-flex min-h-12 w-full items-center justify-center rounded-xl bg-orange-500 px-6 py-3 text-sm font-black text-white shadow-lg shadow-orange-100 hover:bg-orange-600 disabled:cursor-not-allowed disabled:opacity-60 sm:w-auto"
              :disabled="submitting"
            >
              {{ submitting ? t('merchantJoin.form.submitting') : t('merchantJoin.form.submit') }}
            </button>
          </div>
        </form>
      </div>
    </section>
  </main>
</template>

<style scoped>
.merchant-input {
  margin-top: 0.5rem;
  min-height: 3rem;
  width: 100%;
  border-radius: 0.75rem;
  border: 1px solid #e2e8f0;
  background: #fff;
  padding: 0 1rem;
  font-size: 0.875rem;
  font-weight: 600;
  color: #0f172a;
  outline: none;
}

.merchant-input:focus {
  border-color: #fb923c;
  box-shadow: 0 0 0 3px #fed7aa;
}

.merchant-input[aria-invalid='true'] {
  border-color: #f87171;
}

.merchant-error {
  margin-top: 0.375rem;
  display: block;
  font-size: 0.75rem;
  font-weight: 700;
  color: #dc2626;
}
</style>
