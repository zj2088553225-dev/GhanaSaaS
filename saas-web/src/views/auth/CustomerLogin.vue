<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { loginCustomer } from '../../api/auth'
import StorefrontHeader from '../../components/storefront/StorefrontHeader.vue'
import { customerPasswordResetEnabled } from '../../config/storefrontAuth'
import { useCustomerAuthStore } from '../../stores/customerAuth'
import { currentLocale, withLocalePath } from '../../utils/localePath.ts'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const authStore = useCustomerAuthStore()
const props = withDefaults(defineProps<{ passwordResetEnabled?: boolean }>(), {
  passwordResetEnabled: customerPasswordResetEnabled,
})
const form = reactive({ email: '', password: '' })
const loading = ref(false)
const errorMessage = ref('')

function localizedPath(path: string) {
  return withLocalePath(currentLocale(route), path)
}

function loginRedirect() {
  const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : ''
  return redirect.startsWith('/') && !redirect.startsWith('//')
    ? redirect
    : localizedPath('/account')
}

async function submit() {
  if (!form.email.trim() || !form.password) {
    errorMessage.value = t('auth.missingCredentials')
    return
  }

  loading.value = true
  errorMessage.value = ''
  try {
    const result = await loginCustomer({
      email: form.email.trim().toLowerCase(),
      password: form.password,
    })
    authStore.setAuth(result)
    await router.replace(loginRedirect())
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
        <h1 class="mt-2 text-3xl font-black">{{ t('auth.loginTitle') }}</h1>
        <p class="mt-2 text-sm leading-6 text-slate-500">{{ t('auth.emailLoginDescription') }}</p>

        <form class="mt-7 space-y-5" data-testid="login-form" @submit.prevent="submit">
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
              autocomplete="current-password"
              required
              class="mt-2 h-12 w-full rounded-2xl border border-slate-200 px-4 text-sm font-semibold outline-none focus:border-orange-400 focus:ring-2 focus:ring-orange-100"
              :placeholder="t('auth.passwordPlaceholder')"
            />
          </label>

          <div v-if="props.passwordResetEnabled" class="flex justify-end">
            <RouterLink :to="localizedPath('/forgot-password')" class="text-sm font-black text-orange-600 hover:text-orange-700">
              {{ t('auth.forgotPassword') }}
            </RouterLink>
          </div>
          <p v-if="errorMessage" role="alert" class="rounded-2xl bg-red-50 px-4 py-3 text-sm font-bold text-red-600">
            {{ errorMessage }}
          </p>
          <button
            type="submit"
            data-testid="login-submit"
            class="flex min-h-12 w-full items-center justify-center rounded-2xl bg-orange-500 px-5 py-3 text-base font-black text-white shadow-lg shadow-orange-200 hover:bg-orange-600 disabled:cursor-not-allowed disabled:bg-slate-300 disabled:shadow-none"
            :disabled="loading"
          >
            {{ loading ? t('auth.processing') : t('auth.login') }}
          </button>
        </form>

        <p class="mt-6 text-center text-sm font-semibold text-slate-500">
          {{ t('auth.noAccount') }}
          <RouterLink :to="localizedPath('/register')" class="ml-1 font-black text-orange-600 hover:text-orange-700">
            {{ t('auth.register') }}
          </RouterLink>
        </p>
      </div>
    </section>
  </main>
</template>
