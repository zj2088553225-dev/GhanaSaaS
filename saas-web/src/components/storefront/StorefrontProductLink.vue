<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink, useRoute, type RouteLocationRaw } from 'vue-router'
import { vStorefrontImpression } from '../../directives/storefrontImpression'
import { isAdminFrontendPath, isLegacyAdminEntryPath } from '../../utils/adminPath'
import {
  trackStorefrontEvent,
  type StorefrontAnalyticsSource,
  type StorefrontEventInput,
  type StorefrontPageType,
} from '../../utils/storefrontAnalytics'

defineOptions({ inheritAttrs: false })

const props = withDefaults(defineProps<{
  to: RouteLocationRaw
  productId: number
  analyticsSource: StorefrontAnalyticsSource
  pageType: StorefrontPageType
  trackingEnabled?: boolean
}>(), {
  trackingEnabled: true,
})
const route = useRoute()

function canTrack() {
  if (!props.trackingEnabled || props.productId <= 0 || !props.analyticsSource.id) {
    return false
  }
  if (typeof window === 'undefined') {
    return false
  }
  return !isAdminFrontendPath(window.location.pathname) && !isLegacyAdminEntryPath(window.location.pathname)
}

function productEvent(eventName: 'product_impression' | 'product_click'): StorefrontEventInput {
  return {
    event_name: eventName,
    entity_type: 'product',
    entity_id: String(props.productId),
    product_id: props.productId,
    source_type: props.analyticsSource.type,
    source_id: props.analyticsSource.id,
    page_type: props.pageType,
  }
}

const impressionEvent = computed(() => {
  // Route reuse can preserve this component across a new page lifecycle.
  // Reading fullPath ensures the directive is updated and re-observes it.
  void route.fullPath
  return canTrack() ? productEvent('product_impression') : null
})

function trackProductClick() {
  if (canTrack()) {
    trackStorefrontEvent(productEvent('product_click'))
  }
}

function handleProductClick(event: MouseEvent, navigate: (event?: MouseEvent) => unknown) {
  trackProductClick()
  void navigate(event)
}
</script>

<template>
  <RouterLink :to="to" custom v-slot="{ href, navigate }">
    <a
      v-storefront-impression="impressionEvent"
      v-bind="$attrs"
      :href="href"
      @click="handleProductClick($event, navigate)"
    >
      <slot />
    </a>
  </RouterLink>
</template>
