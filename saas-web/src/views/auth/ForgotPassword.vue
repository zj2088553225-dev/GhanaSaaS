<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { requestPasswordReset } from '../../api/auth'
import StorefrontHeader from '../../components/storefront/StorefrontHeader.vue'
import { currentLocale, withLocalePath } from '../../utils/localePath.ts'

const { t } = useI18n()
const route = useRoute()
const email = ref('')
const loading = ref(false)
const message = ref('')

function localizedPath(path: string) {
  return withLocalePath(currentLocale(route), path)
}

async function submit() {
  loading.value = true
  message.value = ''
  try {
    await requestPasswordReset(email.value.trim().toLowerCase())
  } catch {
    // Keep the public response indistinguishable and avoid account enumeration.
  } finally {
    message.value = t('auth.resetRequestSuccess')
    loading.value = false
  }
}
</script>

<template>
  <main class="min-h-screen bg-slate-50 text-slate-950">
    <StorefrontHeader />
    <section class="mx-auto flex min-h-[calc(100vh-8rem)] max-w-md items-center px-4 py-10">
      <div class="w-full rounded-3xl border border-slate-200 bg-white p-7 shadow-sm">
        <p class="text-xs font-black uppercase tracking-[0.28em] text-orange-500">{{ t('auth.security') }}</p>
        <h1 class="mt-2 text-3xl font-black">{{ t('auth.forgotPassword') }}</h1>
        <p class="mt-2 text-sm leading-6 text-slate-500">{{ t('auth.forgotDescription') }}</p>
        <form class="mt-7 space-y-5" @submit.prevent="submit">
          <label class="block">
            <span class="text-sm font-black text-slate-700">{{ t('auth.email') }}</span>
            <input
              v-model="email"
              type="email"
              autocomplete="email"
              required
              class="mt-2 h-12 w-full rounded-2xl border border-slate-200 px-4 text-sm font-semibold outline-none focus:border-orange-400 focus:ring-2 focus:ring-orange-100"
              :placeholder="t('auth.emailPlaceholder')"
            />
          </label>
          <p v-if="message" role="status" class="rounded-2xl bg-emerald-50 px-4 py-3 text-sm font-bold leading-6 text-emerald-700">
            {{ message }}
          </p>
          <button type="submit" class="w-full rounded-2xl bg-orange-500 px-5 py-3 font-black text-white hover:bg-orange-600 disabled:bg-slate-300" :disabled="loading">
            {{ loading ? t('auth.processing') : t('auth.sendResetInstructions') }}
          </button>
        </form>
        <RouterLink :to="localizedPath('/login')" class="mt-6 block text-center text-sm font-black text-orange-600 hover:text-orange-700">
          {{ t('auth.backToLogin') }}
        </RouterLink>
      </div>
    </section>
  </main>
</template>
