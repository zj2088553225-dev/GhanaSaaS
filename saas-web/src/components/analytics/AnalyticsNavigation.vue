<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../../stores/auth'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()

const isSuperAdmin = computed(() => authStore.user?.role === 'super_admin')
const activeTab = computed(() => {
  if (route.path.includes('/analytics/popularity')) {
    return 'popularity'
  }
  if (route.path.includes('/analytics/products')) {
    return 'products'
  }
  if (route.path.includes('/analytics/content')) {
    return 'content'
  }
  return 'overview'
})
const productRouteReady = computed(() => router.hasRoute('admin-analytics-products'))
const contentRouteReady = computed(() => router.hasRoute('admin-analytics-content'))
const popularityRouteReady = computed(() => router.hasRoute('admin-analytics-popularity'))

function sharedQuery() {
  const query: Record<string, string> = {}
  for (const key of ['from', 'to']) {
    const value = route.query[key]
    if (typeof value === 'string' && value) {
      query[key] = value
    }
  }
  const tenantID = route.query.tenant_id
  if (isSuperAdmin.value && typeof tenantID === 'string' && tenantID) {
    query.tenant_id = tenantID
  }
  return query
}

function changeTab(name: string | number) {
  switch (String(name)) {
    case 'products':
      if (productRouteReady.value) {
        void router.push({ name: 'admin-analytics-products', query: sharedQuery() })
      }
      break
    case 'content':
      if (isSuperAdmin.value && contentRouteReady.value) {
        void router.push({ name: 'admin-analytics-content', query: sharedQuery() })
      }
      break
    case 'popularity':
      if (popularityRouteReady.value) {
        void router.push({ name: 'admin-analytics-popularity', query: sharedQuery() })
      }
      break
    default:
      void router.push({ name: 'admin-analytics-overview', query: sharedQuery() })
  }
}
</script>

<template>
  <el-tabs :model-value="activeTab" class="analytics-navigation" @tab-change="changeTab">
    <el-tab-pane label="数据概览" name="overview" />
    <el-tab-pane label="商品分析" name="products" :disabled="!productRouteReady" />
    <el-tab-pane label="热门排名" name="popularity" :disabled="!popularityRouteReady" />
    <el-tab-pane v-if="isSuperAdmin" label="内容分析" name="content" :disabled="!contentRouteReady" />
  </el-tabs>
</template>

<style scoped>
.analytics-navigation :deep(.el-tabs__header) {
  margin-bottom: 0;
}
</style>
