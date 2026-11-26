<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  ANALYTICS_TIMEZONE,
  getProductAnalyticsDetail,
  type AnalyticsDateQuery,
  type AnalyticsDateRange,
  type ProductAnalyticsDetail,
  type ProductAnalyticsSummary,
} from '../../../api/analytics'
import ProductImage from '../../../components/ProductImage.vue'
import AnalyticsDateRangeFilter from '../../../components/analytics/AnalyticsDateRangeFilter.vue'
import AnalyticsEmptyState from '../../../components/analytics/AnalyticsEmptyState.vue'
import AnalyticsMetricCard from '../../../components/analytics/AnalyticsMetricCard.vue'
import AnalyticsNavigation from '../../../components/analytics/AnalyticsNavigation.vue'
import AnalyticsTrendChart, {
  type AnalyticsChartMetric,
  type AnalyticsChartPoint,
} from '../../../components/analytics/AnalyticsTrendChart.vue'
import TenantAnalyticsFilter from '../../../components/analytics/TenantAnalyticsFilter.vue'
import { useAuthStore } from '../../../stores/auth'
import { normalizedAnalyticsRange } from '../../../utils/analyticsDateRange'
import { analyticsSourceLabel, formatAnalyticsCount, formatAnalyticsRate } from '../../../utils/analyticsFormat'

type ProductSummaryKey = keyof ProductAnalyticsSummary

interface ProductMetricDefinition {
  key: ProductSummaryKey
  label: string
  tooltip: string
  rate?: boolean
}

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const isSuperAdmin = computed(() => authStore.user?.role === 'super_admin')
const range = ref<AnalyticsDateRange>(normalizedAnalyticsRange(route.query.from, route.query.to))
const tenantID = ref<number | null>(readTenantID())
const detail = ref<ProductAnalyticsDetail | null>(null)
const loading = ref(false)
const error = ref('')
const selectedTrendMetrics = ref(['impressions', 'clicks', 'views', 'contact_intents', 'unique_visitors'])
let requestSequence = 0
let requestController: AbortController | null = null

const metricCards: ProductMetricDefinition[] = [
  { key: 'impressions', label: '曝光', tooltip: '商品卡片进入可视区域的事件次数。' },
  { key: 'clicks', label: '点击', tooltip: '用户主动点击商品卡片的事件次数。' },
  { key: 'views', label: '详情访问', tooltip: '商品详情成功加载的事件次数。' },
  { key: 'contact_intents', label: '联系意向', tooltip: '该商品 WhatsApp 与电话点击事件总数。' },
  { key: 'unique_visitors', label: '独立访客', tooltip: '日期范围内该商品相关事件的匿名 visitor ID 去重数。' },
  { key: 'ctr', label: 'CTR', tooltip: '商品点击 / 商品曝光。', rate: true },
  { key: 'detail_view_rate', label: '详情访问率', tooltip: '详情访问 / 商品点击。', rate: true },
  { key: 'contact_intent_rate', label: '联系意向率', tooltip: '联系意向 / 详情访问。', rate: true },
]

const trendMetrics: AnalyticsChartMetric[] = [
  { key: 'impressions', label: '曝光', color: '#2563eb' },
  { key: 'clicks', label: '点击', color: '#f97316' },
  { key: 'views', label: '详情访问', color: '#8b5cf6' },
  { key: 'contact_intents', label: '联系意向', color: '#10b981' },
  { key: 'unique_visitors', label: '独立访客', color: '#64748b' },
]

const trendPoints = computed<AnalyticsChartPoint[]>(() =>
  (detail.value?.trend || []).map((point) => ({
    bucket: point.bucket,
    values: {
      impressions: point.impressions,
      clicks: point.clicks,
      views: point.views,
      contact_intents: point.contact_intents,
      unique_visitors: point.unique_visitors,
    },
  })),
)

const productID = computed(() => {
  const value = Array.isArray(route.params.id) ? route.params.id[0] : route.params.id
  if (typeof value !== 'string' || !/^[1-9]\d*$/.test(value)) return 0
  const parsed = Number(value)
  return Number.isSafeInteger(parsed) ? parsed : 0
})

function routeString(key: string) {
  const value = route.query[key]
  return typeof value === 'string' ? value : ''
}

function readTenantID() {
  if (!isSuperAdmin.value) return null
  const value = routeString('tenant_id')
  if (!/^[1-9]\d*$/.test(value)) return null
  const parsed = Number(value)
  return Number.isSafeInteger(parsed) ? parsed : null
}

function analyticsQuery(): AnalyticsDateQuery {
  return {
    from: range.value.from,
    to: range.value.to,
    timezone: ANALYTICS_TIMEZONE,
    tenant_id: isSuperAdmin.value ? tenantID.value || undefined : undefined,
  }
}

function preservedQuery() {
  const result: Record<string, string> = {}
  for (const key of ['search', 'status', 'sort', 'order', 'page', 'page_size']) {
    const value = routeString(key)
    if (value) result[key] = value
  }
  result.from = range.value.from
  result.to = range.value.to
  if (isSuperAdmin.value && tenantID.value) result.tenant_id = String(tenantID.value)
  return result
}

async function loadDetail() {
  if (!productID.value) {
    error.value = '商品 ID 无效'
    detail.value = null
    return
  }
  requestController?.abort()
  requestController = new AbortController()
  const sequence = ++requestSequence
  loading.value = true
  error.value = ''
  try {
    const result = await getProductAnalyticsDetail(productID.value, analyticsQuery(), {
      signal: requestController.signal,
    })
    if (sequence === requestSequence && !requestController.signal.aborted) {
      detail.value = result
    }
  } catch (requestError) {
    if (sequence === requestSequence && !requestController.signal.aborted) {
      error.value = requestError instanceof Error && requestError.message ? requestError.message : '商品分析详情加载失败'
      detail.value = null
    }
  } finally {
    if (sequence === requestSequence) loading.value = false
  }
}

async function replaceURLAndLoad() {
  await router.replace({ name: 'admin-analytics-product-detail', params: { id: productID.value }, query: preservedQuery() })
  await loadDetail()
}

function applyDate(next: AnalyticsDateRange) {
  range.value = { ...next, timezone: ANALYTICS_TIMEZONE }
  void replaceURLAndLoad()
}

function changeTenant(value: number | null) {
  tenantID.value = value
  void replaceURLAndLoad()
}

function invalidTenant() {
  tenantID.value = null
  void replaceURLAndLoad()
}

function backToList() {
  void router.push({ name: 'admin-analytics-products', query: preservedQuery() })
}

function summaryValue(key: ProductSummaryKey) {
  return detail.value?.summary[key] || 0
}

function productName() {
  const product = detail.value?.product
  return product?.product_name || `Deleted product #${product?.product_id || productID.value}`
}

function productStatus() {
  const product = detail.value?.product
  if (!product?.currently_exists) return '已删除'
  return product.status === 'on_shelf' ? '上架' : '下架'
}

function productStatusType() {
  const product = detail.value?.product
  if (!product?.currently_exists) return 'danger'
  return product.status === 'on_shelf' ? 'success' : 'info'
}

function sourceName(sourceLabel: string | null, sourceID: string | null) {
  return sourceLabel || sourceID || '—'
}

onMounted(() => {
  void router.replace({ name: 'admin-analytics-product-detail', params: { id: productID.value }, query: preservedQuery() })
  void loadDetail()
})

watch(productID, (value, previous) => {
  if (value && previous && value !== previous) void loadDetail()
})

onBeforeUnmount(() => {
  requestSequence += 1
  requestController?.abort()
})
</script>

<template>
  <section class="space-y-5">
    <div class="rounded-2xl border border-slate-200 bg-white px-5 pt-5 shadow-sm">
      <div class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div>
          <el-button link type="primary" class="mb-2" @click="backToList">← 返回商品分析</el-button>
          <h2 class="text-2xl font-black text-slate-950">商品行为详情</h2>
          <p class="mt-1 text-sm text-slate-500">查看商品总体漏斗、趋势和事件来源上下文</p>
        </div>
        <TenantAnalyticsFilter v-if="isSuperAdmin" v-model="tenantID" @change="changeTenant" @invalid="invalidTenant" />
      </div>
      <div class="mt-5"><AnalyticsNavigation /></div>
    </div>

    <el-card shadow="never" class="border border-slate-200">
      <AnalyticsDateRangeFilter :model-value="range" :loading="loading" @apply="applyDate" />
    </el-card>

    <el-alert v-if="error" type="error" :closable="false" show-icon :title="error">
      <el-button size="small" @click="loadDetail">重试</el-button>
    </el-alert>

    <template v-if="detail">
      <el-alert
        v-if="!detail.product.currently_exists"
        type="warning"
        :closable="false"
        show-icon
        title="该商品当前已删除，以下为历史行为数据。"
      />

      <el-card shadow="never" class="border border-slate-200">
        <div class="flex flex-col gap-5 sm:flex-row sm:items-center">
          <div class="h-28 w-28 shrink-0 overflow-hidden rounded-xl border border-slate-200">
            <ProductImage :src="detail.product.main_image_url" :alt="productName()" />
          </div>
          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-center gap-2">
              <h3 class="truncate text-xl font-black text-slate-950">{{ productName() }}</h3>
              <el-tag :type="productStatusType()">{{ productStatus() }}</el-tag>
            </div>
            <p class="mt-2 text-sm text-slate-500">Product ID: {{ detail.product.product_id }}</p>
            <p v-if="isSuperAdmin" class="mt-1 text-sm text-slate-500">
              租户：{{ detail.product.tenant_name || `#${detail.product.tenant_id}` }}
            </p>
          </div>
        </div>
      </el-card>

      <el-alert
        type="info"
        :closable="false"
        show-icon
        title="独立访客基于匿名浏览器 visitor ID，不等于登录用户。"
      />

      <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <AnalyticsMetricCard
          v-for="card in metricCards"
          :key="card.key"
          :label="card.label"
          :tooltip="card.tooltip"
          :value="summaryValue(card.key)"
          :rate="card.rate"
          :loading="loading"
          :show-change="false"
        />
      </div>

      <el-card shadow="never" class="border border-slate-200">
        <template #header>
          <div>
            <h3 class="font-black text-slate-950">商品趋势</h3>
            <p class="mt-1 text-xs text-slate-500">各时间桶的独立访客不可相加为总独立访客</p>
          </div>
        </template>
        <AnalyticsTrendChart
          v-model="selectedTrendMetrics"
          :points="trendPoints"
          :metrics="trendMetrics"
          :granularity="detail.granularity"
          :loading="loading"
        />
      </el-card>

      <el-card shadow="never" class="border border-slate-200">
        <template #header>
          <div>
            <h3 class="font-black text-slate-950">来源上下文</h3>
            <p class="mt-1 text-xs text-slate-500">商品各类事件按其自身保存的来源信息聚合</p>
          </div>
        </template>
        <el-alert
          class="mb-4"
          type="warning"
          :closable="false"
          show-icon
          title="来源数据表示事件发生时的页面上下文，不代表严格转化归因。"
        />
        <el-table :data="detail.sources" stripe>
          <el-table-column label="来源" min-width="130">
            <template #default="scope">{{ analyticsSourceLabel(scope.row.source_type) }}</template>
          </el-table-column>
          <el-table-column label="来源标识" min-width="160">
            <template #default="scope">{{ scope.row.source_id || '—' }}</template>
          </el-table-column>
          <el-table-column label="来源名称" min-width="180">
            <template #default="scope">{{ sourceName(scope.row.source_label, scope.row.source_id) }}</template>
          </el-table-column>
          <el-table-column label="曝光" width="100" align="right">
            <template #default="scope">{{ formatAnalyticsCount(scope.row.impressions) }}</template>
          </el-table-column>
          <el-table-column label="点击" width="100" align="right">
            <template #default="scope">{{ formatAnalyticsCount(scope.row.clicks) }}</template>
          </el-table-column>
          <el-table-column label="详情访问" width="110" align="right">
            <template #default="scope">{{ formatAnalyticsCount(scope.row.views) }}</template>
          </el-table-column>
          <el-table-column label="联系意向" width="110" align="right">
            <template #default="scope">{{ formatAnalyticsCount(scope.row.contact_intents) }}</template>
          </el-table-column>
          <el-table-column label="独立访客" width="110" align="right">
            <template #default="scope">{{ formatAnalyticsCount(scope.row.unique_visitors) }}</template>
          </el-table-column>
          <el-table-column label="CTR" width="100" align="right">
            <template #default="scope">{{ formatAnalyticsRate(scope.row.ctr) }}</template>
          </el-table-column>
          <template #empty><AnalyticsEmptyState title="暂无来源数据" description="本周期内没有该商品的来源上下文事件" /></template>
        </el-table>
      </el-card>
    </template>

    <el-card v-else-if="loading" shadow="never"><el-skeleton animated :rows="8" /></el-card>
  </section>
</template>
