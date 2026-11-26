<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { getCurrentCustomer, logoutCustomer } from '../api/auth'
import StorefrontHeader from '../components/storefront/StorefrontHeader.vue'
import { useCustomerAuthStore } from '../stores/customerAuth'
import { currentLocale, withLocalePath } from '../utils/localePath.ts'

const { t, locale } = useI18n()
const route = useRoute()
const router = useRouter()
const authStore = useCustomerAuthStore()
const loading = ref(false)
const errorMessage = ref('')

const registeredAt = computed(() => {
  const value = authStore.customer?.created_at
  if (!value) {
    return '—'
  }
  return new Intl.DateTimeFormat(locale.value === 'zh' ? 'zh-CN' : 'en-GH', {
    dateStyle: 'medium',
  }).format(new Date(value))
})

onMounted(async () => {
  loading.value = true
  try {
    const profile = await getCurrentCustomer()
    authStore.setAuth({ token: authStore.token, user: profile })
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : t('auth.failed')
  } finally {
    loading.value = false
  }
})

async function logout() {
  try {
    await logoutCustomer()
  } catch {
    // JWT logout is completed by deleting the local credential.
  }
  authStore.logout()
  await router.replace(withLocalePath(currentLocale(route), '/login'))
}
</script>

<template>
  <main class="min-h-screen bg-slate-50 text-slate-950">
    <StorefrontHeader />
    <section class="mx-auto max-w-3xl px-4 py-10 sm:px-6">
      <div class="rounded-3xl border border-slate-200 bg-white p-7 shadow-sm sm:p-9">
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div>
            <p class="text-xs font-black uppercase tracking-[0.28em] text-orange-500">{{ t('auth.account') }}</p>
            <h1 class="mt-2 text-3xl font-black">{{ t('auth.accountTitle') }}</h1>
          </div>
          <button type="button" data-testid="account-logout" class="rounded-2xl bg-red-50 px-4 py-2 text-sm font-black text-red-600 hover:bg-red-100" @click="logout">
            {{ t('auth.logout') }}
          </button>
        </div>

        <p v-if="errorMessage" role="alert" class="mt-6 rounded-2xl bg-red-50 px-4 py-3 text-sm font-bold text-red-600">{{ errorMessage }}</p>
        <dl v-else class="mt-8 divide-y divide-slate-100 rounded-2xl border border-slate-200">
          <div class="grid gap-1 px-5 py-4 sm:grid-cols-[10rem_1fr]">
            <dt class="text-sm font-black text-slate-500">{{ t('auth.email') }}</dt>
            <dd class="break-all font-bold text-slate-900">{{ loading ? t('auth.loading') : authStore.customer?.email }}</dd>
          </div>
          <div class="grid gap-1 px-5 py-4 sm:grid-cols-[10rem_1fr]">
            <dt class="text-sm font-black text-slate-500">{{ t('auth.registeredAt') }}</dt>
            <dd class="font-bold text-slate-900">{{ registeredAt }}</dd>
          </div>
          <div class="grid gap-3 px-5 py-4 sm:grid-cols-[10rem_1fr] sm:items-center">
            <dt class="text-sm font-black text-slate-500">{{ t('auth.phone') }}</dt>
            <dd class="flex flex-wrap items-center justify-between gap-3">
              <span class="font-bold text-slate-500">{{ authStore.customer?.phone || t('auth.phoneNotLinked') }}</span>
              <button type="button" disabled class="rounded-xl border border-slate-200 px-3 py-2 text-sm font-black text-slate-400" :title="t('auth.comingSoon')">
                {{ t('auth.bindPhone') }}
              </button>
            </dd>
          </div>
        </dl>
      </div>
    </section>
  </main>
</template>
