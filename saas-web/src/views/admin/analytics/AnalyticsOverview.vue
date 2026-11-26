<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  ANALYTICS_TIMEZONE,
  getAnalyticsOverview,
  getAnalyticsStatus,
  getAnalyticsSources,
  getAnalyticsTrends,
  getLowCTRProducts,
  getTopProducts,
  type AnalyticsDateQuery,
  type AnalyticsDateRange,
  type AnalyticsOverview,
  type AnalyticsOverviewMetrics,
  type AnalyticsSources,
  type AnalyticsDataStatus,
  type AnalyticsTrends,
  type ProductAnalyticsRanking,
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
import { adminPath } from '../../../utils/adminPath'
import { normalizedAnalyticsRange } from '../../../utils/analyticsDateRange'
import {
  analyticsSourceLabel,
  formatAnalyticsCount,
  formatAnalyticsRate,
} from '../../../utils/analyticsFormat'

type OverviewSection = 'overview' | 'trends' | 'sources' | 'top' | 'lowCTR' | 'status'

interface MetricCardDefinition {
  key: keyof AnalyticsOverviewMetrics
  label: string
  tooltip: string
  rate?: boolean
}

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()

const isSuperAdmin = computed(() => authStore.user?.role === 'super_admin')
const range = ref<AnalyticsDateRange>(normalizedAnalyticsRange(route.query.from, route.query.to))
const tenantID = ref<number | null>(initialTenantID())
const overview = ref<AnalyticsOverview | null>(null)
const trends = ref<AnalyticsTrends | null>(null)
const sources = ref<AnalyticsSources | null>(null)
const topProducts = ref<ProductAnalyticsRanking | null>(null)
const lowCTRProducts = ref<ProductAnalyticsRanking | null>(null)
const dataStatus = ref<AnalyticsDataStatus | null>(null)
const selectedTrendMetrics = ref(['product_impressions', 'product_clicks', 'product_views', 'contact_intents'])
const loading = ref<Record<OverviewSection, boolean>>({
  overview: false,
  trends: false,
  sources: false,
  top: false,
  lowCTR: false,
  status: false,
})
const errors = ref<Record<OverviewSection, string>>({
  overview: '',
  trends: '',
  sources: '',
  top: '',
  lowCTR: '',
  status: '',
})
let requestSequence = 0
let requestController: AbortController | null = null

const metricCards: MetricCardDefinition[] = [
  { key: 'visitors', label: '访客数', tooltip: '匿名浏览器 visitor ID 的独立数量，不等于登录用户。' },
  { key: 'sessions', label: '会话数', tooltip: '30 分钟无活动后会创建新的匿名浏览会话。' },
  { key: 'page_views', label: '页面浏览', tooltip: '公开商城页面成功进入的次数。' },
  { key: 'product_impressions', label: '商品曝光', tooltip: '商品卡片真正进入用户可视区域的次数。' },
  { key: 'product_clicks', label: '商品点击', tooltip: '用户主动点击商品卡片的次数。' },
  { key: 'product_views', label: '商品详情访问', tooltip: '商品详情成功加载真实商品的次数。' },
  { key: 'contact_intents', label: '联系意向', tooltip: 'WhatsApp 和电话点击事件总数。' },
  { key: 'product_ctr', label: '商品点击率', tooltip: '商品点击次数 / 商品曝光次数。', rate: true },
  { key: 'detail_view_rate', label: '详情访问率', tooltip: '商品详情访问 / 商品点击。', rate: true },
  { key: 'contact_intent_rate', label: '联系意向率', tooltip: '联系意向 / 商品详情访问。', rate: true },
]

const trendMetrics: AnalyticsChartMetric[] = [
  { key: 'page_views', label: '页面浏览', color: '#64748b' },
  { key: 'product_impressions', label: '商品曝光', color: '#2563eb' },
  { key: 'product_clicks', label: '商品点击', color: '#f97316' },
  { key: 'product_views', label: '商品详情访问', color: '#8b5cf6' },
  { key: 'contact_intents', label: '联系意向', color: '#10b981' },
]

const trendPoints = computed<AnalyticsChartPoint[]>(() =>
  (trends.value?.series || []).map((point) => ({
    bucket: point.bucket,
    values: {
      page_views: point.page_views,
      product_impressions: point.product_impressions,
      product_clicks: point.product_clicks,
      product_views: point.product_views,
      contact_intents: point.contact_intents,
    },
  })),
)

const statusIsStale = computed(() => {
  if (!dataStatus.value?.latest_event_at) return false
  const latest = new Date(dataStatus.value.latest_event_at).getTime()
  return Number.isFinite(latest) && Date.now() - latest > 24 * 60 * 60 * 1000
})

function initialTenantID() {
  if (authStore.user?.role !== 'super_admin') {
    return null
  }
  const value = Array.isArray(route.query.tenant_id) ? route.query.tenant_id[0] : route.query.tenant_id
  if (typeof value !== 'string' || !/^[1-9]\d*$/.test(value)) {
    return null
  }
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

function safeError(error: unknown, fallback: string) {
  if (error instanceof Error && error.name !== 'CanceledError' && error.name !== 'AbortError') {
    return error.message || fallback
  }
  return fallback
}

async function settleSection<T>(
  section: OverviewSection,
  sequence: number,
  task: Promise<T>,
  apply: (value: T) => void,
  fallback: string,
) {
  loading.value[section] = true
  errors.value[section] = ''
  try {
    const value = await task
    if (sequence === requestSequence && !requestController?.signal.aborted) {
      apply(value)
    }
  } catch (error) {
    if (sequence === requestSequence && !requestController?.signal.aborted) {
      errors.value[section] = safeError(error, fallback)
    }
  } finally {
    if (sequence === requestSequence) {
      loading.value[section] = false
    }
  }
}

async function loadDashboard() {
  requestController?.abort()
  requestController = new AbortController()
  const sequence = ++requestSequence
  const query = analyticsQuery()
  const options = { signal: requestController.signal }

  await Promise.all([
    settleSection('overview', sequence, getAnalyticsOverview(query, options), (value) => (overview.value = value), '概览指标加载失败'),
    settleSection('trends', sequence, getAnalyticsTrends(query, options), (value) => (trends.value = value), '趋势数据加载失败'),
    settleSection(
      'sources',
      sequence,
      getAnalyticsSources({ ...query, sort: 'clicks', limit: 20 }, options),
      (value) => (sources.value = value),
      '来源数据加载失败',
    ),
    settleSection(
      'top',
      sequence,
      getTopProducts({ ...query, metric: 'clicks', limit: 10 }, options),
      (value) => (topProducts.value = value),
      '热门商品加载失败',
    ),
    settleSection(
      'lowCTR',
      sequence,
      getLowCTRProducts({ ...query, min_impressions: 100, limit: 10 }, options),
      (value) => (lowCTRProducts.value = value),
      '低点击率商品加载失败',
    ),
    isSuperAdmin.value
      ? settleSection('status', sequence, getAnalyticsStatus(options), (value) => (dataStatus.value = value), '数据状态加载失败')
      : Promise.resolve(),
  ])
}

function formatStatusTime(value: string | null) {
  if (!value) return '—'
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) return '—'
  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: ANALYTICS_TIMEZONE,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(date)
}

function analyticsURLQuery() {
  const query: Record<string, string> = { from: range.value.from, to: range.value.to }
  if (isSuperAdmin.value && tenantID.value) {
    query.tenant_id = String(tenantID.value)
  }
  return query
}

async function updateFiltersAndReload() {
  await router.replace({ name: 'admin-analytics-overview', query: analyticsURLQuery() })
  await loadDashboard()
}

function applyDate(next: AnalyticsDateRange) {
  range.value = { ...next, timezone: ANALYTICS_TIMEZONE }
  void updateFiltersAndReload()
}

function changeTenant(value: number | null) {
  tenantID.value = value
  void updateFiltersAndReload()
}

function invalidTenant() {
  tenantID.value = null
  void updateFiltersAndReload()
}

function openProduct(productID: number) {
  void router.push({ path: adminPath(`/analytics/products/${productID}`), query: analyticsURLQuery() })
}

onMounted(() => {
  const expectedQuery = analyticsURLQuery()
  if (route.query.from !== expectedQuery.from || route.query.to !== expectedQuery.to || (!isSuperAdmin.value && route.query.tenant_id)) {
    void router.replace({ name: 'admin-analytics-overview', query: expectedQuery })
  }
  void loadDashboard()
})

onBeforeUnmount(() => {
  requestSequence += 1
  requestController?.abort()
  requestController = null
})
</script>

<template>
  <section class="space-y-5">
    <div class="rounded-2xl border border-slate-200 bg-white px-5 pt-5 shadow-sm">
      <div class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div>
          <h2 class="text-2xl font-black text-slate-950">商城数据分析</h2>
          <p class="mt-1 text-sm text-slate-500">查看商城访问、商品兴趣和联系意向数据</p>
        </div>
        <TenantAnalyticsFilter
          v-if="isSuperAdmin"
          v-model="tenantID"
          @change="changeTenant"
          @invalid="invalidTenant"
        />
      </div>
      <el-alert
        class="mt-4"
        type="info"
        :closable="false"
        show-icon
        title="数据自行为分析功能上线后开始累计，不包含上线前历史访问。"
      />
      <div class="mt-5">
        <AnalyticsNavigation />
      </div>
    </div>

    <el-card shadow="never" class="border border-slate-200">
      <AnalyticsDateRangeFilter
        :model-value="range"
        :loading="Object.values(loading).some(Boolean)"
        @apply="applyDate"
      />
    </el-card>

    <el-card v-if="isSuperAdmin" v-loading="loading.status" shadow="never" class="border border-slate-200">
      <template #header>
        <div class="flex items-center justify-between gap-3">
          <div>
            <h3 class="font-black text-slate-950">数据状态</h3>
            <p class="mt-1 text-xs text-slate-500">全平台埋点接收与数据质量概况，不受当前日期或租户筛选影响</p>
          </div>
          <el-button v-if="errors.status" size="small" @click="loadDashboard">重试</el-button>
        </div>
      </template>
      <el-alert v-if="errors.status" class="mb-4" type="error" :closable="false" :title="errors.status" />
      <el-alert
        v-else-if="dataStatus && !dataStatus.latest_event_at"
        class="mb-4"
        type="info"
        :closable="false"
        title="尚未收到商城行为事件"
      />
      <el-alert
        v-else-if="statusIsStale"
        class="mb-4"
        type="warning"
        :closable="false"
        title="最近 24 小时未收到新的商城行为事件，请检查前台埋点或访问情况。"
      />
      <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-6">
        <div class="rounded-xl bg-slate-50 p-4"><p class="text-xs font-bold text-slate-500">累计事件</p><strong class="mt-2 block text-xl">{{ formatAnalyticsCount(dataStatus?.total_events || 0) }}</strong></div>
        <div class="rounded-xl bg-slate-50 p-4"><p class="text-xs font-bold text-slate-500">Human Events</p><strong class="mt-2 block text-xl">{{ formatAnalyticsCount(dataStatus?.human_events || 0) }}</strong></div>
        <div class="rounded-xl bg-slate-50 p-4"><p class="text-xs font-bold text-slate-500">Bot Events</p><strong class="mt-2 block text-xl">{{ formatAnalyticsCount(dataStatus?.bot_events || 0) }}</strong></div>
        <div class="rounded-xl bg-slate-50 p-4"><p class="text-xs font-bold text-slate-500">最近 24 小时</p><strong class="mt-2 block text-xl">{{ formatAnalyticsCount(dataStatus?.events_last_24h || 0) }}</strong></div>
        <div class="rounded-xl bg-slate-50 p-4"><p class="text-xs font-bold text-slate-500">最早数据</p><strong class="mt-2 block text-sm">{{ formatStatusTime(dataStatus?.oldest_event_at || null) }}</strong></div>
        <div class="rounded-xl bg-slate-50 p-4"><p class="text-xs font-bold text-slate-500">最新事件</p><strong class="mt-2 block text-sm">{{ formatStatusTime(dataStatus?.latest_event_at || null) }}</strong></div>
      </div>
    </el-card>

    <el-alert
      v-if="errors.overview"
      type="error"
      :closable="false"
      show-icon
      :title="errors.overview"
    >
      <el-button size="small" @click="loadDashboard">重试</el-button>
    </el-alert>

    <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
      <AnalyticsMetricCard
        v-for="card in metricCards"
        :key="card.key"
        :label="card.label"
        :tooltip="card.tooltip"
        :metric="overview?.metrics[card.key]"
        :rate="card.rate"
        :loading="loading.overview"
      />
    </div>

    <el-card shadow="never" class="border border-slate-200">
      <template #header>
        <div class="flex items-center justify-between gap-3">
          <div>
            <h3 class="font-black text-slate-950">行为趋势</h3>
            <p class="mt-1 text-xs text-slate-500">按请求时区和日期范围自动选择小时或自然日粒度</p>
          </div>
          <el-button v-if="errors.trends" size="small" @click="loadDashboard">重试</el-button>
        </div>
      </template>
      <el-alert v-if="errors.trends" class="mb-4" type="error" :closable="false" :title="errors.trends" />
      <AnalyticsTrendChart
        v-model="selectedTrendMetrics"
        :points="trendPoints"
        :metrics="trendMetrics"
        :granularity="trends?.granularity || 'day'"
        :loading="loading.trends"
      />
    </el-card>

    <div class="grid gap-5 2xl:grid-cols-[1.35fr_1fr]">
      <el-card shadow="never" class="border border-slate-200">
        <template #header>
          <div class="flex items-center justify-between">
            <div>
              <h3 class="font-black text-slate-950">流量来源</h3>
              <p class="mt-1 text-xs text-slate-500">按事件发生时保存的来源上下文统计</p>
            </div>
            <el-button v-if="errors.sources" size="small" @click="loadDashboard">重试</el-button>
          </div>
        </template>
        <el-alert v-if="errors.sources" class="mb-4" type="error" :closable="false" :title="errors.sources" />
        <el-table v-loading="loading.sources" :data="sources?.sources || []" stripe>
          <el-table-column label="来源" min-width="140">
            <template #default="scope">{{ analyticsSourceLabel(scope.row.source_type) }}</template>
          </el-table-column>
          <el-table-column label="曝光" min-width="90" align="right">
            <template #default="scope">{{ formatAnalyticsCount(scope.row.impressions) }}</template>
          </el-table-column>
          <el-table-column label="点击" min-width="90" align="right">
            <template #default="scope">{{ formatAnalyticsCount(scope.row.clicks) }}</template>
          </el-table-column>
          <el-table-column label="详情访问" min-width="100" align="right">
            <template #default="scope">{{ formatAnalyticsCount(scope.row.product_views) }}</template>
          </el-table-column>
          <el-table-column label="联系意向" min-width="100" align="right">
            <template #default="scope">{{ formatAnalyticsCount(scope.row.contact_intents) }}</template>
          </el-table-column>
          <el-table-column label="CTR" min-width="90" align="right">
            <template #default="scope">{{ formatAnalyticsRate(scope.row.ctr) }}</template>
          </el-table-column>
          <template #empty><AnalyticsEmptyState /></template>
        </el-table>
      </el-card>

      <div class="grid gap-5 lg:grid-cols-2 2xl:grid-cols-1">
        <el-card shadow="never" class="border border-slate-200">
          <template #header>
            <div class="flex items-center justify-between">
              <div>
                <h3 class="font-black text-slate-950">热门商品 TOP 10</h3>
                <p class="mt-1 text-xs text-slate-500">按商品点击次数排序</p>
              </div>
              <el-button v-if="errors.top" size="small" @click="loadDashboard">重试</el-button>
            </div>
          </template>
          <el-alert v-if="errors.top" class="mb-3" type="error" :closable="false" :title="errors.top" />
          <el-skeleton v-if="loading.top" animated :rows="5" />
          <AnalyticsEmptyState v-else-if="!topProducts?.items.length" />
          <button
            v-for="(product, index) in topProducts?.items || []"
            v-else
            :key="product.product_id"
            type="button"
            class="flex w-full items-center gap-3 border-b border-slate-100 py-2.5 text-left last:border-0 hover:bg-slate-50"
            @click="openProduct(product.product_id)"
          >
            <span class="w-5 text-center text-xs font-black text-slate-400">{{ index + 1 }}</span>
            <div class="h-10 w-10 shrink-0 overflow-hidden rounded-lg"><ProductImage :src="product.main_image_url" :alt="product.product_name" /></div>
            <span class="min-w-0 flex-1 truncate text-sm font-semibold text-slate-800">{{ product.product_name }}</span>
            <span class="text-sm font-black text-orange-600">{{ formatAnalyticsCount(product.clicks) }} 次点击</span>
          </button>
        </el-card>

        <el-card shadow="never" class="border border-slate-200">
          <template #header>
            <div class="flex items-center justify-between">
              <div>
                <h3 class="font-black text-slate-950">高曝光低点击 TOP 10</h3>
                <p class="mt-1 text-xs text-slate-500">至少 100 次曝光，按 CTR 从低到高</p>
              </div>
              <el-button v-if="errors.lowCTR" size="small" @click="loadDashboard">重试</el-button>
            </div>
          </template>
          <el-alert v-if="errors.lowCTR" class="mb-3" type="error" :closable="false" :title="errors.lowCTR" />
          <el-skeleton v-if="loading.lowCTR" animated :rows="5" />
          <AnalyticsEmptyState v-else-if="!lowCTRProducts?.items.length" />
          <button
            v-for="product in lowCTRProducts?.items || []"
            v-else
            :key="product.product_id"
            type="button"
            class="flex w-full items-center gap-3 border-b border-slate-100 py-2.5 text-left last:border-0 hover:bg-slate-50"
            @click="openProduct(product.product_id)"
          >
            <div class="h-10 w-10 shrink-0 overflow-hidden rounded-lg"><ProductImage :src="product.main_image_url" :alt="product.product_name" /></div>
            <span class="min-w-0 flex-1 truncate text-sm font-semibold text-slate-800">{{ product.product_name }}</span>
            <span class="text-right text-xs text-slate-500">
              <strong class="block text-sm text-red-600">{{ formatAnalyticsRate(product.ctr) }}</strong>
              {{ formatAnalyticsCount(product.impressions) }} 曝光 · {{ formatAnalyticsCount(product.clicks) }} 点击
            </span>
          </button>
        </el-card>
      </div>
    </div>
  </section>
</template>
