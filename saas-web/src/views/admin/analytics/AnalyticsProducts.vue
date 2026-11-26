<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useRoute, useRouter } from 'vue-router'
import {
  ANALYTICS_TIMEZONE,
  exportProductAnalytics,
  getLowCTRProducts,
  getProductAnalytics,
  getTopProducts,
  type AnalyticsDateQuery,
  type AnalyticsDateRange,
  type AnalyticsSortOrder,
  type ProductAnalyticsList,
  type ProductAnalyticsItem,
  type ProductAnalyticsMetric,
  type ProductAnalyticsRanking,
  type ProductAnalyticsStatus,
} from '../../../api/analytics'
import ProductImage from '../../../components/ProductImage.vue'
import AnalyticsDateRangeFilter from '../../../components/analytics/AnalyticsDateRangeFilter.vue'
import AnalyticsEmptyState from '../../../components/analytics/AnalyticsEmptyState.vue'
import AnalyticsNavigation from '../../../components/analytics/AnalyticsNavigation.vue'
import TenantAnalyticsFilter from '../../../components/analytics/TenantAnalyticsFilter.vue'
import { useAuthStore } from '../../../stores/auth'
import { normalizedAnalyticsRange } from '../../../utils/analyticsDateRange'
import { downloadAnalyticsCSV } from '../../../utils/analyticsDownload'
import { formatAnalyticsCount, formatAnalyticsRate } from '../../../utils/analyticsFormat'

interface ProductTableSortChange {
  prop?: string
  order?: 'ascending' | 'descending' | null
}

const productSortOptions: Array<{ value: ProductAnalyticsMetric; label: string }> = [
  { value: 'impressions', label: '曝光' },
  { value: 'clicks', label: '点击' },
  { value: 'views', label: '详情访问' },
  { value: 'contact_intents', label: '联系意向' },
  { value: 'unique_visitors', label: '独立访客' },
  { value: 'ctr', label: 'CTR' },
  { value: 'detail_view_rate', label: '详情访问率' },
  { value: 'contact_intent_rate', label: '联系意向率' },
]
const allowedSorts = new Set<ProductAnalyticsMetric>(productSortOptions.map((item) => item.value))
const allowedStatuses = new Set<ProductAnalyticsStatus>(['on_shelf', 'off_shelf', 'deleted'])

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const isSuperAdmin = computed(() => authStore.user?.role === 'super_admin')
const range = ref<AnalyticsDateRange>(normalizedAnalyticsRange(route.query.from, route.query.to))
const tenantID = ref<number | null>(readTenantID())
const search = ref(readString('search').slice(0, 200))
const status = ref<ProductAnalyticsStatus | ''>(readStatus())
const sort = ref<ProductAnalyticsMetric>(readSort())
const order = ref<AnalyticsSortOrder>(readString('order') === 'asc' ? 'asc' : 'desc')
const page = ref(readPositiveInt('page', 1, 1_000_000))
const pageSize = ref(readPositiveInt('page_size', 20, 100))
const list = ref<ProductAnalyticsList | null>(null)
const topProducts = ref<ProductAnalyticsRanking | null>(null)
const lowCTRProducts = ref<ProductAnalyticsRanking | null>(null)
const topMetric = ref<ProductAnalyticsMetric>('clicks')
const lowMinImpressions = ref(100)
const listLoading = ref(false)
const topLoading = ref(false)
const lowLoading = ref(false)
const listError = ref('')
const topError = ref('')
const lowError = ref('')
const exportLoading = ref(false)
let listSequence = 0
let topSequence = 0
let lowSequence = 0
let listController: AbortController | null = null
let topController: AbortController | null = null
let lowController: AbortController | null = null

function readString(key: string) {
  const value = route.query[key]
  return typeof value === 'string' ? value.trim() : ''
}

function readPositiveInt(key: string, fallback: number, maximum: number) {
  const value = readString(key)
  if (!/^[1-9]\d*$/.test(value)) {
    return fallback
  }
  const parsed = Number(value)
  return Number.isSafeInteger(parsed) && parsed <= maximum ? parsed : fallback
}

function readTenantID() {
  return isSuperAdmin.value ? readPositiveInt('tenant_id', 0, Number.MAX_SAFE_INTEGER) || null : null
}

function readStatus(): ProductAnalyticsStatus | '' {
  const value = readString('status') as ProductAnalyticsStatus
  return allowedStatuses.has(value) ? value : ''
}

function readSort(): ProductAnalyticsMetric {
  const value = readString('sort') as ProductAnalyticsMetric
  return allowedSorts.has(value) ? value : 'clicks'
}

function analyticsQuery(): AnalyticsDateQuery {
  return {
    from: range.value.from,
    to: range.value.to,
    timezone: ANALYTICS_TIMEZONE,
    tenant_id: isSuperAdmin.value ? tenantID.value || undefined : undefined,
  }
}

function listURLQuery() {
  const query: Record<string, string> = {
    from: range.value.from,
    to: range.value.to,
    sort: sort.value,
    order: order.value,
    page: String(page.value),
    page_size: String(pageSize.value),
  }
  if (isSuperAdmin.value && tenantID.value) query.tenant_id = String(tenantID.value)
  if (search.value.trim()) query.search = search.value.trim()
  if (status.value) query.status = status.value
  return query
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback
}

async function loadList() {
  listController?.abort()
  listController = new AbortController()
  const sequence = ++listSequence
  listLoading.value = true
  listError.value = ''
  try {
    const result = await getProductAnalytics(
      {
        ...analyticsQuery(),
        page: page.value,
        page_size: pageSize.value,
        search: search.value.trim() || undefined,
        status: status.value || undefined,
        sort: sort.value,
        order: order.value,
      },
      { signal: listController.signal },
    )
    if (sequence === listSequence && !listController.signal.aborted) {
      list.value = result
    }
  } catch (error) {
    if (sequence === listSequence && !listController.signal.aborted) {
      listError.value = errorMessage(error, '商品分析加载失败')
    }
  } finally {
    if (sequence === listSequence) listLoading.value = false
  }
}

async function loadTopProducts() {
  topController?.abort()
  topController = new AbortController()
  const sequence = ++topSequence
  topLoading.value = true
  topError.value = ''
  try {
    const result = await getTopProducts(
      { ...analyticsQuery(), metric: topMetric.value, limit: 10 },
      { signal: topController.signal },
    )
    if (sequence === topSequence && !topController.signal.aborted) topProducts.value = result
  } catch (error) {
    if (sequence === topSequence && !topController.signal.aborted) topError.value = errorMessage(error, '热门商品加载失败')
  } finally {
    if (sequence === topSequence) topLoading.value = false
  }
}

async function loadLowCTRProducts() {
  lowController?.abort()
  lowController = new AbortController()
  const sequence = ++lowSequence
  lowLoading.value = true
  lowError.value = ''
  try {
    const result = await getLowCTRProducts(
      { ...analyticsQuery(), min_impressions: lowMinImpressions.value, limit: 10 },
      { signal: lowController.signal },
    )
    if (sequence === lowSequence && !lowController.signal.aborted) lowCTRProducts.value = result
  } catch (error) {
    if (sequence === lowSequence && !lowController.signal.aborted) lowError.value = errorMessage(error, '低点击率商品加载失败')
  } finally {
    if (sequence === lowSequence) lowLoading.value = false
  }
}

async function replaceURLAndLoad(loadRankingsToo = true) {
  await router.replace({ name: 'admin-analytics-products', query: listURLQuery() })
  await Promise.all([
    loadList(),
    loadRankingsToo ? Promise.all([loadTopProducts(), loadLowCTRProducts()]) : Promise.resolve(),
  ])
}

function applyFilters() {
  search.value = search.value.trim().slice(0, 200)
  page.value = 1
  void replaceURLAndLoad()
}

function applyDate(next: AnalyticsDateRange) {
  range.value = { ...next, timezone: ANALYTICS_TIMEZONE }
  page.value = 1
  void replaceURLAndLoad()
}

function changeTenant(value: number | null) {
  tenantID.value = value
  page.value = 1
  void replaceURLAndLoad()
}

function invalidTenant() {
  tenantID.value = null
  page.value = 1
  void replaceURLAndLoad()
}

function changePage(value: number) {
  page.value = value
  void replaceURLAndLoad(false)
}

function changePageSize(value: number) {
  pageSize.value = value
  page.value = 1
  void replaceURLAndLoad(false)
}

function changeTableSort(change: ProductTableSortChange) {
  const nextSort = change.prop as ProductAnalyticsMetric
  if (!allowedSorts.has(nextSort) || !change.order) return
  sort.value = nextSort
  order.value = change.order === 'ascending' ? 'asc' : 'desc'
  page.value = 1
  void replaceURLAndLoad(false)
}

function reloadTop() {
  void loadTopProducts()
}

function reloadLowCTR() {
  lowMinImpressions.value = Math.max(0, Math.min(1_000_000_000, Number(lowMinImpressions.value) || 0))
  void loadLowCTRProducts()
}

async function exportCSV() {
  if (exportLoading.value) return
  exportLoading.value = true
  try {
    const download = await exportProductAnalytics({
      ...analyticsQuery(),
      search: search.value.trim() || undefined,
      status: status.value || undefined,
      sort: sort.value,
      order: order.value,
    })
    downloadAnalyticsCSV(download)
    ElMessage.success('商品分析 CSV 已导出')
  } catch {
    ElMessage.error('导出失败，请缩小日期范围或筛选条件后重试')
  } finally {
    exportLoading.value = false
  }
}

function productName(product: ProductAnalyticsList['items'][number]) {
  return product.product_name || `Deleted product #${product.product_id}`
}

function statusLabel(product: ProductAnalyticsList['items'][number]) {
  if (!product.currently_exists) return '已删除'
  return product.status === 'on_shelf' ? '上架' : '下架'
}

function statusType(product: ProductAnalyticsList['items'][number]) {
  if (!product.currently_exists) return 'danger'
  return product.status === 'on_shelf' ? 'success' : 'info'
}

function metricValue(product: ProductAnalyticsList['items'][number], metric: ProductAnalyticsMetric) {
  return metric.endsWith('_rate') || metric === 'ctr'
    ? formatAnalyticsRate(product[metric])
    : formatAnalyticsCount(product[metric])
}

function openDetail(productID: number) {
  void router.push({ name: 'admin-analytics-product-detail', params: { id: productID }, query: listURLQuery() })
}

function openProductRow(product: ProductAnalyticsItem) {
  openDetail(product.product_id)
}

onMounted(() => {
  void router.replace({ name: 'admin-analytics-products', query: listURLQuery() })
  void Promise.all([loadList(), loadTopProducts(), loadLowCTRProducts()])
})

onBeforeUnmount(() => {
  listSequence += 1
  topSequence += 1
  lowSequence += 1
  listController?.abort()
  topController?.abort()
  lowController?.abort()
})
</script>

<template>
  <section class="space-y-5">
    <div class="rounded-2xl border border-slate-200 bg-white px-5 pt-5 shadow-sm">
      <div class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div>
          <h2 class="text-2xl font-black text-slate-950">商品分析</h2>
          <p class="mt-1 text-sm text-slate-500">查看商品曝光、点击、详情访问与联系意向</p>
        </div>
        <TenantAnalyticsFilter v-if="isSuperAdmin" v-model="tenantID" @change="changeTenant" @invalid="invalidTenant" />
      </div>
      <div class="mt-5"><AnalyticsNavigation /></div>
    </div>

    <el-card shadow="never" class="border border-slate-200">
      <AnalyticsDateRangeFilter :model-value="range" :loading="listLoading" @apply="applyDate" />
      <el-divider />
      <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-[minmax(220px,1fr)_160px_180px_130px_auto] xl:items-end">
        <div>
          <label class="mb-1.5 block text-xs font-bold text-slate-500">商品搜索</label>
          <el-input v-model="search" maxlength="200" clearable placeholder="商品名称或 slug" @keyup.enter="applyFilters" />
        </div>
        <div>
          <label class="mb-1.5 block text-xs font-bold text-slate-500">状态</label>
          <el-select v-model="status" class="w-full">
            <el-option label="全部" value="" />
            <el-option label="上架" value="on_shelf" />
            <el-option label="下架" value="off_shelf" />
            <el-option label="已删除" value="deleted" />
          </el-select>
        </div>
        <div>
          <label class="mb-1.5 block text-xs font-bold text-slate-500">排序指标</label>
          <el-select v-model="sort" class="w-full">
            <el-option v-for="item in productSortOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </div>
        <div>
          <label class="mb-1.5 block text-xs font-bold text-slate-500">顺序</label>
          <el-select v-model="order" class="w-full">
            <el-option label="从高到低" value="desc" />
            <el-option label="从低到高" value="asc" />
          </el-select>
        </div>
        <el-button type="primary" @click="applyFilters">查询商品</el-button>
      </div>
    </el-card>

    <div class="grid gap-5 xl:grid-cols-2">
      <el-card shadow="never" class="border border-slate-200">
        <template #header>
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h3 class="font-black text-slate-950">热门商品</h3>
              <p class="mt-1 text-xs text-slate-500">运营关注指标 TOP 10</p>
            </div>
            <el-select v-model="topMetric" size="small" class="w-36" @change="reloadTop">
              <el-option label="点击" value="clicks" />
              <el-option label="详情访问" value="views" />
              <el-option label="联系意向" value="contact_intents" />
              <el-option label="独立访客" value="unique_visitors" />
            </el-select>
          </div>
        </template>
        <el-alert v-if="topError" class="mb-3" type="error" :closable="false" :title="topError" />
        <el-skeleton v-if="topLoading" animated :rows="5" />
        <AnalyticsEmptyState v-else-if="!topProducts?.items.length" />
        <button
          v-for="(product, index) in topProducts?.items || []"
          v-else
          :key="product.product_id"
          type="button"
          class="flex w-full items-center gap-3 border-b border-slate-100 py-2.5 text-left last:border-0 hover:bg-slate-50"
          @click="openDetail(product.product_id)"
        >
          <span class="w-5 text-xs font-black text-slate-400">{{ index + 1 }}</span>
          <div class="h-10 w-10 shrink-0 overflow-hidden rounded-lg"><ProductImage :src="product.main_image_url" :alt="productName(product)" /></div>
          <span class="min-w-0 flex-1 truncate text-sm font-semibold">{{ productName(product) }}</span>
          <strong class="text-sm text-orange-600">{{ metricValue(product, topMetric) }}</strong>
        </button>
      </el-card>

      <el-card shadow="never" class="border border-slate-200">
        <template #header>
          <div class="flex flex-wrap items-end justify-between gap-3">
            <div>
              <h3 class="font-black text-slate-950">高曝光低点击</h3>
              <p class="mt-1 text-xs text-slate-500">满足最小曝光后按 CTR 升序</p>
            </div>
            <div class="flex items-center gap-2">
              <span class="text-xs font-bold text-slate-500">最小曝光</span>
              <el-input-number v-model="lowMinImpressions" :min="0" :max="1000000000" :step="50" size="small" class="w-32" />
              <el-button size="small" @click="reloadLowCTR">应用</el-button>
            </div>
          </div>
        </template>
        <el-alert v-if="lowError" class="mb-3" type="error" :closable="false" :title="lowError" />
        <el-skeleton v-if="lowLoading" animated :rows="5" />
        <AnalyticsEmptyState v-else-if="!lowCTRProducts?.items.length" />
        <button
          v-for="product in lowCTRProducts?.items || []"
          v-else
          :key="product.product_id"
          type="button"
          class="flex w-full items-center gap-3 border-b border-slate-100 py-2.5 text-left last:border-0 hover:bg-slate-50"
          @click="openDetail(product.product_id)"
        >
          <div class="h-10 w-10 shrink-0 overflow-hidden rounded-lg"><ProductImage :src="product.main_image_url" :alt="productName(product)" /></div>
          <span class="min-w-0 flex-1 truncate text-sm font-semibold">{{ productName(product) }}</span>
          <span class="text-right text-xs text-slate-500">
            <strong class="block text-sm text-red-600">{{ formatAnalyticsRate(product.ctr) }}</strong>
            {{ formatAnalyticsCount(product.impressions) }} 曝光 · {{ formatAnalyticsCount(product.clicks) }} 点击
          </span>
        </button>
      </el-card>
    </div>

    <el-card shadow="never" class="border border-slate-200">
      <template #header>
        <div class="flex items-center justify-between">
          <div>
            <h3 class="font-black text-slate-950">商品数据列表</h3>
            <p class="mt-1 text-xs text-slate-500">聚合、排序和分页均由后端完成</p>
          </div>
          <div class="flex items-center gap-2">
            <el-button v-if="listError" size="small" @click="loadList">重试</el-button>
            <el-button type="primary" plain :loading="exportLoading" :disabled="exportLoading" @click="exportCSV">导出 CSV</el-button>
          </div>
        </div>
      </template>
      <el-alert v-if="listError" class="mb-4" type="error" :closable="false" :title="listError" />
      <el-table
        v-loading="listLoading"
        :data="list?.items || []"
        stripe
        row-key="product_id"
        @sort-change="changeTableSort"
        @row-click="openProductRow"
      >
        <el-table-column label="商品" min-width="250" fixed="left">
          <template #default="scope">
            <div class="flex items-center gap-3">
              <div class="h-11 w-11 shrink-0 overflow-hidden rounded-lg"><ProductImage :src="scope.row.main_image_url" :alt="productName(scope.row)" /></div>
              <div class="min-w-0">
                <p class="truncate font-semibold text-slate-900">{{ productName(scope.row) }}</p>
                <p class="text-xs text-slate-400">Product ID: {{ scope.row.product_id }}</p>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column v-if="isSuperAdmin" prop="tenant_name" label="租户" min-width="150" />
        <el-table-column label="状态" width="90">
          <template #default="scope"><el-tag :type="statusType(scope.row)">{{ statusLabel(scope.row) }}</el-tag></template>
        </el-table-column>
        <el-table-column prop="impressions" label="曝光" width="105" align="right" sortable="custom">
          <template #default="scope">{{ formatAnalyticsCount(scope.row.impressions) }}</template>
        </el-table-column>
        <el-table-column prop="clicks" label="点击" width="105" align="right" sortable="custom">
          <template #default="scope">{{ formatAnalyticsCount(scope.row.clicks) }}</template>
        </el-table-column>
        <el-table-column prop="views" label="详情访问" width="120" align="right" sortable="custom">
          <template #default="scope">{{ formatAnalyticsCount(scope.row.views) }}</template>
        </el-table-column>
        <el-table-column prop="contact_intents" label="联系意向" width="120" align="right" sortable="custom">
          <template #default="scope">{{ formatAnalyticsCount(scope.row.contact_intents) }}</template>
        </el-table-column>
        <el-table-column prop="unique_visitors" label="独立访客" width="120" align="right" sortable="custom">
          <template #default="scope">{{ formatAnalyticsCount(scope.row.unique_visitors) }}</template>
        </el-table-column>
        <el-table-column prop="ctr" label="CTR" width="105" align="right" sortable="custom">
          <template #default="scope">{{ formatAnalyticsRate(scope.row.ctr) }}</template>
        </el-table-column>
        <el-table-column prop="detail_view_rate" label="详情访问率" width="130" align="right" sortable="custom">
          <template #default="scope">{{ formatAnalyticsRate(scope.row.detail_view_rate) }}</template>
        </el-table-column>
        <el-table-column prop="contact_intent_rate" label="联系意向率" width="130" align="right" sortable="custom">
          <template #default="scope">{{ formatAnalyticsRate(scope.row.contact_intent_rate) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="110" fixed="right">
          <template #default="scope"><el-button link type="primary" @click.stop="openDetail(scope.row.product_id)">查看数据</el-button></template>
        </el-table-column>
        <template #empty><AnalyticsEmptyState /></template>
      </el-table>
      <div class="mt-4 flex justify-end">
        <el-pagination
          background
          :current-page="page"
          :page-size="pageSize"
          :page-sizes="[20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          :total="list?.total || 0"
          @current-change="changePage"
          @size-change="changePageSize"
        />
      </div>
    </el-card>
  </section>
</template>
