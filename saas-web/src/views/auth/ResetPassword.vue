<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { resetPassword } from '../../api/auth'
import StorefrontHeader from '../../components/storefront/StorefrontHeader.vue'
import { currentLocale, withLocalePath } from '../../utils/localePath.ts'

const { t } = useI18n()
const route = useRoute()
const queryToken = typeof route.query.token === 'string' ? route.query.token : ''
const form = reactive({ token: queryToken, password: '', confirmPassword: '' })
const loading = ref(false)
const errorMessage = ref('')
const successMessage = ref('')

function localizedPath(path: string) {
  return withLocalePath(currentLocale(route), path)
}

async function submit() {
  errorMessage.value = ''
  successMessage.value = ''
  if (!form.token.trim()) {
    errorMessage.value = t('auth.missingResetToken')
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
    await resetPassword(form.token.trim(), form.password)
    successMessage.value = t('auth.resetSuccess')
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
        <p class="text-xs font-black uppercase tracking-[0.28em] text-orange-500">{{ t('auth.security') }}</p>
        <h1 class="mt-2 text-3xl font-black">{{ t('auth.resetPassword') }}</h1>
        <p class="mt-2 text-sm leading-6 text-slate-500">{{ t('auth.resetDescription') }}</p>
        <form class="mt-7 space-y-5" @submit.prevent="submit">
          <label v-if="!queryToken" class="block">
            <span class="text-sm font-black text-slate-700">{{ t('auth.resetToken') }}</span>
            <input v-model="form.token" required class="mt-2 h-12 w-full rounded-2xl border border-slate-200 px-4 text-sm font-semibold outline-none focus:border-orange-400" />
          </label>
          <label class="block">
            <span class="text-sm font-black text-slate-700">{{ t('auth.newPassword') }}</span>
            <input v-model="form.password" type="password" autocomplete="new-password" required class="mt-2 h-12 w-full rounded-2xl border border-slate-200 px-4 text-sm font-semibold outline-none focus:border-orange-400" :placeholder="t('auth.passwordHint')" />
          </label>
          <label class="block">
            <span class="text-sm font-black text-slate-700">{{ t('auth.confirmPassword') }}</span>
            <input v-model="form.confirmPassword" type="password" autocomplete="new-password" required class="mt-2 h-12 w-full rounded-2xl border border-slate-200 px-4 text-sm font-semibold outline-none focus:border-orange-400" />
          </label>
          <p v-if="errorMessage" role="alert" class="rounded-2xl bg-red-50 px-4 py-3 text-sm font-bold text-red-600">{{ errorMessage }}</p>
          <p v-if="successMessage" role="status" class="rounded-2xl bg-emerald-50 px-4 py-3 text-sm font-bold text-emerald-700">{{ successMessage }}</p>
          <button type="submit" class="w-full rounded-2xl bg-orange-500 px-5 py-3 font-black text-white hover:bg-orange-600 disabled:bg-slate-300" :disabled="loading">
            {{ loading ? t('auth.processing') : t('auth.resetPassword') }}
          </button>
        </form>
        <RouterLink :to="localizedPath('/login')" class="mt-6 block text-center text-sm font-black text-orange-600 hover:text-orange-700">
          {{ t('auth.backToLogin') }}
        </RouterLink>
      </div>
    </section>
  </main>
</template>
