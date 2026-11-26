<script setup lang="ts">
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useCustomerAuthStore } from '../stores/customerAuth'
import { storefrontAuthEnabled } from '../composables/usePreviewMode'
import { currentLocale, withLocalePath } from '../utils/localePath.ts'

const props = withDefaults(defineProps<{ enabled?: boolean }>(), {
  enabled: storefrontAuthEnabled,
})
const { t } = useI18n()
const route = useRoute()
const authStore = useCustomerAuthStore()
</script>

<template>
  <RouterLink
    v-if="props.enabled"
    :to="withLocalePath(currentLocale(route), authStore.isAuthenticated ? '/account' : '/login')"
    data-testid="store-auth-navigation"
    class="rounded-2xl border border-slate-200 px-3 py-2 text-xs font-black text-slate-600 transition hover:border-orange-200 hover:bg-orange-50 hover:text-orange-700 sm:text-sm"
  >
    {{ authStore.isAuthenticated ? t('auth.account') : t('auth.login') }}
  </RouterLink>
</template>
