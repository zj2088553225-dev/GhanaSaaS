<script setup lang="ts">
import { onMounted } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { usePreviewMode } from '../../composables/usePreviewMode'
import { currentLocale, withLocalePath } from '../../utils/localePath.ts'

const { t } = useI18n()
const route = useRoute()
const { showPreviewMode } = usePreviewMode()

onMounted(showPreviewMode)
</script>

<template>
  <section class="mx-auto flex min-h-[calc(100vh-9rem)] max-w-2xl items-center px-4 py-10 sm:px-6">
    <div class="w-full rounded-3xl border border-slate-200 bg-white p-7 text-center shadow-sm sm:p-10">
      <div class="mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-orange-100 text-3xl" aria-hidden="true">⏳</div>
      <p class="mt-5 text-xs font-black uppercase tracking-[0.24em] text-orange-500">{{ t('previewMode.badge') }}</p>
      <h1 class="mt-2 text-3xl font-black text-slate-950">{{ t('previewMode.title') }}</h1>
      <p class="mx-auto mt-4 max-w-xl text-sm font-semibold leading-7 text-slate-600">{{ t('previewMode.description') }}</p>
      <p class="mx-auto mt-2 max-w-xl text-sm font-semibold leading-7 text-slate-600">{{ t('previewMode.contact') }}</p>
      <RouterLink
        :to="withLocalePath(currentLocale(route), '/mall')"
        class="mt-7 inline-flex min-h-12 items-center justify-center rounded-2xl bg-orange-500 px-6 py-3 text-sm font-black text-white shadow-lg shadow-orange-200 hover:bg-orange-600"
      >
        {{ t('previewMode.browse') }}
      </RouterLink>
    </div>
  </section>
</template>
