<script setup lang="ts">
import { computed, ref, watch } from 'vue'

const props = defineProps<{
  src?: string | null
  fallbackSrc?: string | null
  alt: string
  imageClass?: string
  loading?: 'lazy' | 'eager'
  fetchpriority?: 'high' | 'low' | 'auto'
}>()

const failed = ref(false)
const currentSrc = ref('')
const usingFallback = ref(false)
const hasImage = computed(() => Boolean(currentSrc.value && !failed.value))

watch(
  [() => props.src, () => props.fallbackSrc],
  () => {
    failed.value = false
    usingFallback.value = false
    currentSrc.value = props.src?.trim() || props.fallbackSrc?.trim() || ''
  },
  { immediate: true },
)

function handleError() {
  const fallbackSrc = props.fallbackSrc?.trim() || ''
  if (!usingFallback.value && fallbackSrc && fallbackSrc !== currentSrc.value) {
    usingFallback.value = true
    currentSrc.value = fallbackSrc
    return
  }
  failed.value = true
  currentSrc.value = ''
}
</script>

<template>
  <div class="relative flex h-full w-full items-center justify-center overflow-hidden bg-slate-100 text-slate-400">
    <img
      v-if="hasImage"
      :src="currentSrc"
      :alt="alt"
      :loading="loading || 'lazy'"
      decoding="async"
      :fetchpriority="fetchpriority"
      :class="imageClass || 'h-full w-full object-cover'"
      @error="handleError"
    />
    <svg v-else class="h-10 w-10" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M4.75 19.25h14.5a1.5 1.5 0 0 0 1.5-1.5V6.25a1.5 1.5 0 0 0-1.5-1.5H4.75a1.5 1.5 0 0 0-1.5 1.5v11.5a1.5 1.5 0 0 0 1.5 1.5Z"
        stroke="currentColor"
        stroke-width="1.7"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
      <path
        d="m4 16 4.1-4.1a1.4 1.4 0 0 1 1.98 0l2.17 2.17 1.42-1.42a1.4 1.4 0 0 1 1.98 0L20 17"
        stroke="currentColor"
        stroke-width="1.7"
        stroke-linecap="round"
        stroke-linejoin="round"
      />
      <path d="M15.75 9.25h.01" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" />
    </svg>
  </div>
</template>
