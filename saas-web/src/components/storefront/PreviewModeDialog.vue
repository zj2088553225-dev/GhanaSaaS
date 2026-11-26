<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { usePreviewMode } from '../../composables/usePreviewMode'

const { t } = useI18n()
const { dialogOpen, hidePreviewMode } = usePreviewMode()
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div
        v-if="dialogOpen"
        class="fixed inset-0 z-[100] flex items-center justify-center bg-slate-950/45 p-4 backdrop-blur-sm"
        role="presentation"
        @click.self="hidePreviewMode"
      >
        <section
          class="w-full max-w-md rounded-3xl bg-white p-6 shadow-2xl sm:p-7"
          role="dialog"
          aria-modal="true"
          :aria-label="t('previewMode.title')"
        >
          <div class="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-orange-100 text-2xl" aria-hidden="true">
            ⏳
          </div>
          <h2 class="mt-4 text-center text-2xl font-black text-slate-950">{{ t('previewMode.title') }}</h2>
          <p class="mt-3 text-center text-sm font-semibold leading-7 text-slate-600">{{ t('previewMode.description') }}</p>
          <p class="mt-2 text-center text-sm font-semibold leading-7 text-slate-600">{{ t('previewMode.contact') }}</p>
          <button
            type="button"
            class="mt-6 flex min-h-12 w-full items-center justify-center rounded-2xl bg-orange-500 px-5 py-3 text-base font-black text-white shadow-lg shadow-orange-200 transition hover:bg-orange-600"
            @click="hidePreviewMode"
          >
            {{ t('previewMode.confirm') }}
          </button>
        </section>
      </div>
    </Transition>
  </Teleport>
</template>
