<script setup lang="ts">
import { computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import ProductImage from '../ProductImage.vue'
import type { ProductGalleryMedia } from '../../utils/productSku'
import { imageVariantURL } from '../../utils/imageVariants'

const props = defineProps<{
  media: ProductGalleryMedia[]
  modelValue: string
  alt: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const { t } = useI18n()

const normalizedMedia = computed(() => {
  const seen = new Set<string>()
  return props.media
    .map((item) => ({ type: item.type, url: item.url.trim() }))
    .filter((item) => {
      if (!item.url || seen.has(item.url)) {
        return false
      }
      seen.add(item.url)
      return true
    })
})
const activeMedia = computed<ProductGalleryMedia>(() => {
  const selected = normalizedMedia.value.find((item) => item.url === props.modelValue)
  if (selected) {
    return selected
  }
  if (props.modelValue) {
    return { type: 'image', url: props.modelValue }
  }
  return normalizedMedia.value[0] || { type: 'image', url: '' }
})
const thumbnailMedia = computed(() => {
  if (activeMedia.value.url && !normalizedMedia.value.some((item) => item.url === activeMedia.value.url)) {
    return [activeMedia.value, ...normalizedMedia.value]
  }
  return normalizedMedia.value
})

watch(
  normalizedMedia,
  (media) => {
    if (!props.modelValue && media[0]) {
      emit('update:modelValue', media[0].url)
    }
  },
  { immediate: true },
)
</script>

<template>
  <div class="min-w-0">
    <div class="mx-auto aspect-square w-full max-w-[560px] overflow-hidden rounded-2xl border border-slate-100 bg-slate-50">
      <video
        v-if="activeMedia.type === 'video' && activeMedia.url"
        :key="activeMedia.url"
        :src="activeMedia.url"
        class="h-full w-full bg-black object-contain"
        controls
        preload="metadata"
        playsinline
      />
      <ProductImage
        v-else
        :src="imageVariantURL(activeMedia.url, 'large')"
        :alt="alt"
        loading="eager"
        class="h-full w-full"
        image-class="h-full w-full object-contain"
      />
    </div>

    <div v-if="thumbnailMedia.length" class="scrollbar-hide mt-3 flex max-w-full gap-2 overflow-x-auto pb-1">
      <button
        v-for="item in thumbnailMedia"
        :key="`${item.type}:${item.url}`"
        type="button"
        class="relative h-20 w-20 shrink-0 overflow-hidden rounded-xl border-2 bg-white p-1 transition"
        :class="activeMedia.url === item.url ? 'border-orange-500 shadow-sm' : 'border-slate-200 hover:border-orange-300'"
        @click="emit('update:modelValue', item.url)"
      >
        <div v-if="item.type === 'video'" class="flex h-full w-full flex-col items-center justify-center rounded-lg bg-slate-900 text-white">
          <span class="text-xl">▶</span>
          <span class="mt-1 text-xs font-bold">{{ t('media.video') }}</span>
        </div>
        <ProductImage v-else :src="imageVariantURL(item.url, 'thumb')" :alt="alt" class="h-full w-full rounded-lg" image-class="h-full w-full object-cover" />
      </button>
    </div>
  </div>
</template>

<style scoped>
.scrollbar-hide::-webkit-scrollbar {
  display: none;
}

.scrollbar-hide {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
</style>
