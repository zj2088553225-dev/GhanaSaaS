<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useRoute, useRouter } from 'vue-router'
import {
  getProductPopularity,
  type ProductPopularityItem,
} from '../../../api/analytics'
import ProductImage from '../../../components/ProductImage.vue'
import AnalyticsEmptyState from '../../../components/analytics/AnalyticsEmptyState.vue'
import AnalyticsNavigation from '../../../components/analytics/AnalyticsNavigation.vue'
import TenantAnalyticsFilter from '../../../components/analytics/TenantAnalyticsFilter.vue'
import { useAuthStore } from '../../../stores/auth'
import { formatAnalyticsCount } from '../../../utils/analyticsFormat'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const isSuperAdmin = computed(() => authStore.user?.role === 'super_admin')
const tenantID = ref<number | null>(readPositiveInteger(route.query.tenant_id))
const categoryID = ref<number | null>(readPositiveInteger(route.query.category_id))
const limit = ref(readPositiveInteger(route.query.limit) || 20)
const items = ref<ProductPopularityItem[]>([])
const loading = ref(false)
const errorMessage = ref('')
let requestSequence = 0
let requestController: AbortController | null = null

function readPositiveInteger(value: unknown) {
  const first = Array.isArray(value) ? value[0] : value
  const parsed = typeof first === 'string' ? Number(first) : NaN
  return Number.isInteger(parsed) && parsed > 0 ? parsed : null
}

function requestQuery() {
  return {
    tenant_id: isSuperAdmin.value ? tenantID.value || undefined : undefined,
    category_id: categoryID.value || undefined,
    limit: Math.min(Math.max(limit.value, 1), 100),
  }
}

function routeQuery() {
  const query: Record<string, string> = { limit: String(requestQuery().limit) }
  if (isSuperAdmin.value && tenantID.value) {
    query.tenant_id = String(tenantID.value)
  }
  if (categoryID.value) {
    query.category_id = String(categoryID.value)
  }
  return query
}

async function loadRanking() {
  const sequence = ++requestSequence
  requestController?.abort()
  requestController = new AbortController()
  loading.value = true
  errorMessage.value = ''
  try {
    const result = await getProductPopularity(requestQuery(), { signal: requestController.signal })
    if (sequence === requestSequence) {
      items.value = result
    }
  } catch (error) {
    if (sequence !== requestSequence || requestController.signal.aborted) {
      return
    }
    items.value = []
    errorMessage.value = error instanceof Error ? error.message : '热门排名加载失败'
  } finally {
    if (sequence === requestSequence) {
      loading.value = false
    }
  }
}

async function applyFilters() {
  limit.value = Math.min(Math.max(Number(limit.value) || 20, 1), 100)
  await router.replace({ name: 'admin-analytics-popularity', query: routeQuery() })
  await loadRanking()
}

function changeTenant(value: number | null) {
  tenantID.value = value
  void applyFilters()
}

function invalidTenant() {
  tenantID.value = null
  ElMessage.warning('租户筛选已重置')
  void applyFilters()
}

function reservationCount(item: ProductPopularityItem) {
  return item.interest_count + item.verified_count
}

onMounted(() => {
  void router.replace({ name: 'admin-analytics-popularity', query: routeQuery() })
  void loadRanking()
})

onBeforeUnmount(() => {
  requestSequence += 1
  requestController?.abort()
})
</script>

<template>
  <section class="space-y-5" data-testid="analytics-popularity">
    <div class="rounded-2xl border border-slate-200 bg-white px-5 pt-5 shadow-sm">
      <div class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div>
          <h2 class="text-2xl font-black text-slate-950">Analytics Product Ranking</h2>
          <p class="mt-1 text-sm text-slate-500">融合浏览、点击、咨询和预约行为的商品热度排名</p>
        </div>
        <TenantAnalyticsFilter v-if="isSuperAdmin" v-model="tenantID" @change="changeTenant" @invalid="invalidTenant" />
      </div>
      <div class="mt-5"><AnalyticsNavigation /></div>
    </div>

    <el-card shadow="never" class="border border-slate-200">
      <div class="grid gap-3 sm:grid-cols-[minmax(180px,1fr)_160px_auto] sm:items-end">
        <div>
          <label class="mb-1.5 block text-xs font-bold text-slate-500">商品分类 ID</label>
          <el-input-number v-model="categoryID" :min="1" :precision="0" controls-position="right" class="w-full" placeholder="全部分类" />
        </div>
        <div>
          <label class="mb-1.5 block text-xs font-bold text-slate-500">显示数量</label>
          <el-input-number v-model="limit" :min="1" :max="100" :precision="0" controls-position="right" class="w-full" />
        </div>
        <el-button type="primary" :loading="loading" @click="applyFilters">查询</el-button>
      </div>
    </el-card>

    <el-card shadow="never" class="border border-slate-200">
      <div v-if="errorMessage" class="py-12 text-center" data-testid="popularity-error">
        <p class="text-sm font-bold text-rose-600">{{ errorMessage }}</p>
        <el-button class="mt-4" type="primary" @click="loadRanking">重试</el-button>
      </div>
      <AnalyticsEmptyState
        v-else-if="!loading && items.length === 0"
        title="暂无热门商品数据"
        description="商城产生浏览、点击、咨询或预约行为后会显示排名"
      />
      <el-table v-else v-loading="loading" :data="items" row-key="product_id" class="w-full">
        <el-table-column label="商品" min-width="260">
          <template #default="{ row }">
            <div class="flex items-center gap-3">
              <ProductImage :src="row.main_image" :alt="row.name" class="h-12 w-12 shrink-0 overflow-hidden rounded-lg bg-slate-100" image-class="h-full w-full object-cover" />
              <div class="min-w-0">
                <p class="truncate font-bold text-slate-900">{{ row.name }}</p>
                <p class="text-xs text-slate-400">#{{ row.product_id }}</p>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="热度分" prop="hot_score" width="120">
          <template #default="{ row }"><span class="font-black text-orange-600">{{ row.hot_score.toFixed(1) }}</span></template>
        </el-table-column>
        <el-table-column label="浏览" prop="views" width="110">
          <template #default="{ row }">{{ formatAnalyticsCount(row.views) }}</template>
        </el-table-column>
        <el-table-column label="点击" prop="clicks" width="110">
          <template #default="{ row }">{{ formatAnalyticsCount(row.clicks) }}</template>
        </el-table-column>
        <el-table-column label="咨询" prop="contacts" width="110">
          <template #default="{ row }">{{ formatAnalyticsCount(row.contacts) }}</template>
        </el-table-column>
        <el-table-column label="预约" width="110">
          <template #default="{ row }">{{ formatAnalyticsCount(reservationCount(row)) }}</template>
        </el-table-column>
      </el-table>
    </el-card>
  </section>
</template>
