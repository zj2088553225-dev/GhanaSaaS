<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { registerCustomer } from '../../api/auth'
import StorefrontHeader from '../../components/storefront/StorefrontHeader.vue'
import { currentLocale, withLocalePath } from '../../utils/localePath.ts'

const { t } = useI18n()
const route = useRoute()
const form = reactive({ email: '', password: '', confirmPassword: '' })
const loading = ref(false)
const errorMessage = ref('')
const successMessage = ref('')

function localizedPath(path: string) {
  return withLocalePath(currentLocale(route), path)
}

async function submit() {
  errorMessage.value = ''
  successMessage.value = ''
  if (!form.email.trim()) {
    errorMessage.value = t('auth.invalidEmail')
    return
  }
  if (form.password.length < 8) {
    errorMessage.value = t('auth.passwordTooShort')
    return
  }
  if (form.password !== form.confirmPassword) {
    errorMessage.value = t('auth.passwordMismatch')
    return
  }

  loading.value = true
  try {
    await registerCustomer({
      email: form.email.trim().toLowerCase(),
      password: form.password,
    })
    successMessage.value = t('auth.registrationSuccess')
    form.password = ''
    form.confirmPassword = ''
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : t('auth.failed')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <main class="min-h-screen bg-slate-50 text-slate-950">
    <StorefrontHeader />
    <section class="mx-auto flex min-h-[calc(100vh-8rem)] max-w-md items-center px-4 py-10">
      <div class="w-full rounded-3xl border border-slate-200 bg-white p-7 shadow-sm">
        <p class="text-xs font-black uppercase tracking-[0.28em] text-orange-500">{{ t('auth.account') }}</p>
        <h1 class="mt-2 text-3xl font-black">{{ t('auth.registerTitle') }}</h1>
        <p class="mt-2 text-sm leading-6 text-slate-500">{{ t('auth.registerDescription') }}</p>

        <form class="mt-7 space-y-5" data-testid="register-form" @submit.prevent="submit">
          <label class="block">
            <span class="text-sm font-black text-slate-700">{{ t('auth.email') }}</span>
            <input
              v-model="form.email"
              type="email"
              autocomplete="email"
              required
              class="mt-2 h-12 w-full rounded-2xl border border-slate-200 px-4 text-sm font-semibold outline-none focus:border-orange-400 focus:ring-2 focus:ring-orange-100"
              :placeholder="t('auth.emailPlaceholder')"
            />
          </label>
          <label class="block">
            <span class="text-sm font-black text-slate-700">{{ t('auth.password') }}</span>
            <input
              v-model="form.password"
              type="password"
              autocomplete="new-password"
              required
              class="mt-2 h-12 w-full rounded-2xl border border-slate-200 px-4 text-sm font-semibold outline-none focus:border-orange-400 focus:ring-2 focus:ring-orange-100"
              :placeholder="t('auth.passwordHint')"
            />
          </label>
          <label class="block">
            <span class="text-sm font-black text-slate-700">{{ t('auth.confirmPassword') }}</span>
            <input
              v-model="form.confirmPassword"
              type="password"
              autocomplete="new-password"
              required
              class="mt-2 h-12 w-full rounded-2xl border border-slate-200 px-4 text-sm font-semibold outline-none focus:border-orange-400 focus:ring-2 focus:ring-orange-100"
              :placeholder="t('auth.confirmPasswordPlaceholder')"
            />
          </label>
          <p v-if="errorMessage" role="alert" class="rounded-2xl bg-red-50 px-4 py-3 text-sm font-bold text-red-600">
            {{ errorMessage }}
          </p>
          <p v-if="successMessage" role="status" class="rounded-2xl bg-emerald-50 px-4 py-3 text-sm font-bold text-emerald-700">
            {{ successMessage }}
          </p>
          <button
            type="submit"
            data-testid="register-submit"
            class="flex min-h-12 w-full items-center justify-center rounded-2xl bg-orange-500 px-5 py-3 text-base font-black text-white shadow-lg shadow-orange-200 hover:bg-orange-600 disabled:cursor-not-allowed disabled:bg-slate-300 disabled:shadow-none"
            :disabled="loading"
          >
            {{ loading ? t('auth.processing') : t('auth.register') }}
          </button>
        </form>

        <p class="mt-6 text-center text-sm font-semibold text-slate-500">
          {{ t('auth.haveAccount') }}
          <RouterLink :to="localizedPath('/login')" class="ml-1 font-black text-orange-600 hover:text-orange-700">
            {{ t('auth.login') }}
          </RouterLink>
        </p>
      </div>
    </section>
  </main>
</template>
