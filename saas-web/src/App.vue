<script setup lang="ts">
import StorefrontTrackingHead from './components/storefront/StorefrontTrackingHead.vue'
import { isAdminFrontendPath, isLegacyAdminEntryPath } from './utils/adminPath.ts'

function isStorefrontRoute(path: string) {
  return !isAdminFrontendPath(path) && !isLegacyAdminEntryPath(path)
}
</script>

<template>
  <RouterView v-slot="{ Component, route }">
    <div :class="{ 'storefront-app': isStorefrontRoute(route.path) }">
      <StorefrontTrackingHead
        v-if="isStorefrontRoute(route.path)"
        :path="route.path"
        :full-path="route.fullPath"
      />
      <component :is="Component" />
    </div>
  </RouterView>
</template>
