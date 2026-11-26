<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SitePageBlock } from '../../api/sitePages'
import type { SupportedLocale } from '../../utils/localePath'
import {
  buildGoogleMapsEmbedURL,
  googleMapsEmbedKey,
  googleMapsPlaceQuery,
  localizedMapText,
  normalizeMapHeight,
} from '../../utils/googleMapsEmbed'

const props = withDefaults(defineProps<{
  block: SitePageBlock
  locale: SupportedLocale
  preview?: boolean
}>(), {
  preview: false,
})

const { t } = useI18n()
const apiKey = computed(() => googleMapsEmbedKey())
const placeQuery = computed(() => googleMapsPlaceQuery(props.block))
const embedURL = computed(() => buildGoogleMapsEmbedURL(props.block, apiKey.value))
const title = computed(() => localizedMapText(props.block, props.locale, 'title'))
const subtitle = computed(() => localizedMapText(props.block, props.locale, 'subtitle'))
const mapHeightStyle = computed(() => ({
  '--map-desktop-height': `${normalizeMapHeight(props.block.map_height)}px`,
}))
const configurationMessage = computed(() => (
  !apiKey.value
    ? t('homeBlocks.mapMissingApiKey')
    : !placeQuery.value
      ? t('homeBlocks.mapMissingPlace')
      : ''
))
</script>

<template>
  <div
    class="w-full min-w-0 max-w-full bg-white"
    :class="preview ? 'p-4' : 'rounded-2xl p-4 shadow-sm sm:p-5'"
  >
    <div
      v-if="title || subtitle || block.external_map_url"
      class="mb-4 flex min-w-0 flex-col items-start gap-3 sm:flex-row sm:items-end sm:justify-between sm:gap-4"
    >
      <div class="min-w-0">
        <p class="text-xs font-bold uppercase tracking-[0.22em] text-emerald-600">{{ t('homeBlocks.mapLocation') }}</p>
        <h2
          v-if="title"
          class="mt-1 break-words font-black text-slate-950"
          :class="preview ? 'text-base' : 'text-xl sm:text-2xl'"
        >
          {{ title }}
        </h2>
        <p v-if="subtitle" class="mt-1 break-words text-sm leading-6 text-slate-500">{{ subtitle }}</p>
      </div>
      <a
        v-if="block.external_map_url"
        :href="block.external_map_url"
        target="_blank"
        rel="noopener noreferrer"
        class="shrink-0 text-sm font-black text-orange-600 hover:text-orange-500"
      >
        {{ t('homeBlocks.openInGoogleMaps') }}
      </a>
    </div>

    <div
      class="map-embed-frame w-full max-w-full overflow-hidden rounded-2xl bg-slate-100"
      :style="mapHeightStyle"
    >
      <iframe
        v-if="embedURL"
        :src="embedURL"
        :title="title || t('homeBlocks.mapEmbedTitle')"
        width="100%"
        height="100%"
        class="block h-full w-full border-0"
        loading="lazy"
        allowfullscreen
        referrerpolicy="strict-origin-when-cross-origin"
      />
      <div
        v-else
        data-testid="google-map-config-hint"
        role="status"
        class="flex h-full min-w-0 items-center justify-center px-5 text-center"
      >
        <div class="max-w-xl">
          <p class="text-sm font-black text-slate-700">{{ t('homeBlocks.mapConfigurationRequired') }}</p>
          <p class="mt-2 break-words text-xs leading-5 text-slate-500">{{ configurationMessage }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.map-embed-frame {
  height: 260px;
}

@media (min-width: 768px) {
  .map-embed-frame {
    height: var(--map-desktop-height, 320px);
  }
}
</style>
