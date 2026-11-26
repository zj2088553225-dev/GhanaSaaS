<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import i18n, { STORAGE_KEY, type SupportedLocale } from '../i18n'
import { currentLocale, switchLocalePath } from '../utils/localePath.ts'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const activeLocale = computed<SupportedLocale>(() => currentLocale(route))
const nextLocale = computed<SupportedLocale>(() => (activeLocale.value === 'zh' ? 'en' : 'zh'))
const nextLabel = computed(() => (nextLocale.value === 'zh' ? '中文' : 'EN'))

async function switchLanguage() {
  i18n.global.locale.value = nextLocale.value
  localStorage.setItem(STORAGE_KEY, nextLocale.value)
  await router.replace(switchLocalePath(route, nextLocale.value))
}
</script>

<template>
  <button
    type="button"
    class="rounded-2xl border border-slate-200 px-3 py-2 text-xs font-black text-slate-600 transition hover:border-orange-200 hover:bg-orange-50 hover:text-orange-700"
    :aria-label="t('nav.language')"
    @click="switchLanguage"
  >
    {{ nextLabel }}
  </button>
</template>
