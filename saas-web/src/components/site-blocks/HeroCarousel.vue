<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { SitePageBlock, SitePageBlockSlide } from '../../api/sitePages'
import ProductImage from '../ProductImage.vue'
import { vStorefrontImpression } from '../../directives/storefrontImpression'
import { imageVariantURL } from '../../utils/imageVariants'
import { currentLocale, ensureLocalePath } from '../../utils/localePath.ts'
import { trackStorefrontEvent, type StorefrontEventInput } from '../../utils/storefrontAnalytics.ts'

const props = defineProps<{
  block: SitePageBlock
}>()
const { t } = useI18n()
const route = useRoute()

const slides = computed(() => (props.block.slides || []).filter((slide) => slide.enabled !== false))
const autoplay = computed(() => props.block.autoplay !== false)
const interval = computed(() => (Number(props.block.interval) > 0 ? Number(props.block.interval) : 4000))
const imageFitClass = computed(() => (props.block.fit === 'contain' ? 'object-contain' : 'object-cover'))
const renderedSlides = computed(() => {
  const count = slides.value.length
  if (!count) {
    return []
  }
  const activeIndex = Math.min(currentIndex.value, count - 1)
  const indexes = new Set<number>([activeIndex])
  if (count > 1) {
    indexes.add((activeIndex + 1) % count)
  }
  return Array.from(indexes).map((index) => ({
    index,
    slide: slides.value[index],
  }))
})
const heightClass = computed(() => {
  switch (props.block.height) {
    case 'small':
      return 'h-[160px] sm:h-[220px] lg:h-[300px]'
    case 'medium':
      return 'h-[180px] sm:h-[250px] lg:h-[340px]'
    default:
      return 'h-[200px] sm:h-[280px] lg:h-[380px]'
  }
})

const currentIndex = ref(0)
let timer: ReturnType<typeof setInterval> | null = null

function startAutoplay() {
  stopAutoplay()
  if (!autoplay.value || slides.value.length <= 1) {
    return
  }
  timer = setInterval(() => {
    nextSlide()
  }, interval.value)
}

function stopAutoplay() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

function nextSlide() {
  if (slides.value.length <= 1) {
    return
  }
  currentIndex.value = (currentIndex.value + 1) % slides.value.length
}

function goToSlide(index: number) {
  currentIndex.value = index
}

function slideAlt(slide: SitePageBlockSlide, index: number) {
  const title = slide.title?.trim()
  if (title) {
    return title
  }
  return t('home.carouselAlt', { index: index + 1 })
}

function slideFetchPriority(index: number) {
  return index === 0 ? 'high' : undefined
}

watch(
  [slides, autoplay, interval],
  () => {
    if (currentIndex.value >= slides.value.length) {
      currentIndex.value = 0
    }
    startAutoplay()
  },
  { deep: true },
)

onMounted(startAutoplay)
onUnmounted(stopAutoplay)

function isExternalLink(link?: string) {
  if (!link) {
    return false
  }
  return /^https?:\/\//i.test(link) || link.startsWith('mailto:') || link.startsWith('tel:')
}

function linkComponent(slide: SitePageBlockSlide) {
  if (!slide.link) {
    return 'div'
  }
  return isExternalLink(slide.link) ? 'a' : RouterLink
}

function linkProps(slide: SitePageBlockSlide) {
  if (!slide.link) {
    return {}
  }
  if (isExternalLink(slide.link)) {
    return { href: slide.link, target: '_blank', rel: 'noopener noreferrer' }
  }
  return { to: ensureLocalePath(slide.link, currentLocale(route)) }
}

function slideEvent(eventName: 'content_impression' | 'content_click', slide: SitePageBlockSlide): StorefrontEventInput | null {
  if (!slide.tracking_key) {
    return null
  }
  return {
    event_name: eventName,
    entity_type: 'marketing_content',
    entity_id: slide.tracking_key,
    source_type: 'marketing_content',
    source_id: slide.tracking_key,
    page_type: 'home',
  }
}

function trackSlideClick(slide: SitePageBlockSlide) {
  if (!slide.link) {
    return
  }
  const contentEvent = slideEvent('content_click', slide)
  if (contentEvent) {
    trackStorefrontEvent(contentEvent)
  }
  const lowerLink = slide.link.trim().toLowerCase()
  const contactEvent = lowerLink.startsWith('tel:')
    ? 'phone_click'
    : lowerLink.includes('wa.me/') || lowerLink.includes('whatsapp.com/')
      ? 'whatsapp_click'
      : null
  if (contactEvent && slide.tracking_key) {
    trackStorefrontEvent({
      event_name: contactEvent,
      entity_type: 'marketing_content',
      entity_id: slide.tracking_key,
      source_type: 'marketing_content',
      source_id: slide.tracking_key,
      page_type: 'home',
    })
  }
}
</script>

<template>
  <section
    class="relative overflow-hidden rounded-xl bg-slate-100 sm:rounded-2xl"
    :class="heightClass"
    @mouseenter="stopAutoplay"
    @mouseleave="startAutoplay"
  >
    <div class="relative h-full w-full">
      <ProductImage
        v-if="!slides.length"
        :src="undefined"
        :alt="t('home.noCarousel')"
        class="h-full w-full"
        image-class="h-full w-full object-contain"
      />
      <div
        v-for="{ slide, index } in renderedSlides"
        :key="slide.tracking_key || index"
        v-storefront-impression="index === currentIndex ? slideEvent('content_impression', slide) : null"
        class="absolute inset-0 transition-opacity duration-500 ease-out"
        :class="index === currentIndex ? 'z-10 opacity-100' : 'z-0 opacity-0'"
      >
        <component
          :is="linkComponent(slide)"
          v-bind="linkProps(slide)"
          class="block h-full w-full"
          @click="trackSlideClick(slide)"
        >
          <ProductImage
            :src="imageVariantURL(slide.image, 'large')"
            :fallback-src="slide.image"
            :alt="slideAlt(slide, index)"
            :loading="index === 0 ? 'eager' : 'lazy'"
            :fetchpriority="slideFetchPriority(index)"
            class="h-full w-full"
            :image-class="`h-full w-full ${imageFitClass}`"
          />
        </component>
      </div>
    </div>

    <div v-if="slides.length > 1" class="absolute bottom-4 left-1/2 z-20 flex -translate-x-1/2 gap-2">
      <button
        v-for="(slide, index) in slides"
        :key="slide.tracking_key || index"
        type="button"
        class="h-2.5 w-2.5 rounded-full ring-1 ring-white/70 transition"
        :class="index === currentIndex ? 'bg-white' : 'bg-white/45 hover:bg-white/70'"
        :aria-label="t('home.goToSlide', { index: index + 1 })"
        @click="goToSlide(index)"
      />
    </div>
  </section>
</template>
