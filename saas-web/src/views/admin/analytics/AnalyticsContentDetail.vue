<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  ANALYTICS_TIMEZONE,
  getContentAnalyticsDetail,
  type AnalyticsDateRange,
  type ContentAnalyticsDetail,
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
import { normalizedAnalyticsRange } from '../../../utils/analyticsDateRange'
import {
  analyticsContentTargetLabel,
  analyticsContentTypeLabel,
  formatAnalyticsCount,
  formatAnalyticsRate,
} from '../../../utils/analyticsFormat'

type ContentSummaryKey = keyof ContentAnalyticsDetail['summary']

interface ContentMetricDefinition {
  key: ContentSummaryKey
  label: string
  tooltip: string
  rate?: boolean
}

const trackingKeyPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
const route = useRoute()
const router = useRouter()
const range = ref<AnalyticsDateRange>(normalizedAnalyticsRange(route.query.from, route.query.to))
const detail = ref<ContentAnalyticsDetail | null>(null)
const loading = ref(false)
const error = ref('')
const selectedTrendMetrics = ref(['impressions', 'clicks', 'unique_visitors'])
let requestSequence = 0
let requestController: AbortController | null = null

const metricCards: ContentMetricDefinition[] = [
  { key: 'impressions', label: '曝光', tooltip: '内容真正进入用户可视区域的事件次数。' },
  { key: 'clicks', label: '点击', tooltip: '用户主动点击该营销内容的事件次数。' },
  { key: 'unique_visitors', label: '独立访客', tooltip: '该内容曝光和点击事件中的匿名 visitor ID 去重数。' },
  { key: 'ctr', label: 'CTR', tooltip: '内容点击 / 内容曝光。', rate: true },
]

const trendMetrics: AnalyticsChartMetric[] = [
  { key: 'impressions', label: '曝光', color: '#2563eb' },
  { key: 'clicks', label: '点击', color: '#f97316' },
  { key: 'unique_visitors', label: '独立访客', color: '#10b981' },
]

const trendPoints = computed<AnalyticsChartPoint[]>(() =>
  (detail.value?.trend || []).map((point) => ({
    bucket: point.bucket,
    values: {
      impressions: point.impressions,
      clicks: point.clicks,
      unique_visitors: point.unique_visitors,
    },
  })),
)

const trackingKey = computed(() => {
  const value = Array.isArray(route.params.trackingKey) ? route.params.trackingKey[0] : route.params.trackingKey
  return typeof value === 'string' ? value.trim().toLowerCase() : ''
})

function routeString(key: string) {
  const value = route.query[key]
  return typeof value === 'string' ? value : ''
}

function preservedQuery() {
  const result: Record<string, string> = {}
  for (const key of ['search', 'content_type', 'currently_exists', 'sort', 'order', 'page', 'page_size', 'min_impressions']) {
    const value = routeString(key)
    if (value) result[key] = value
  }
  result.from = range.value.from
  result.to = range.value.to
  return result
}

async function loadDetail() {
  if (!trackingKeyPattern.test(trackingKey.value)) {
    detail.value = null
    error.value = 'Tracking key 格式无效'
    return
  }
  requestController?.abort()
  requestController = new AbortController()
  const sequence = ++requestSequence
  loading.value = true
  error.value = ''
  try {
    const result = await getContentAnalyticsDetail(
      trackingKey.value,
      { from: range.value.from, to: range.value.to, timezone: ANALYTICS_TIMEZONE },
      { signal: requestController.signal },
    )
    if (sequence === requestSequence && !requestController.signal.aborted) detail.value = result
  } catch (requestError) {
    if (sequence === requestSequence && !requestController.signal.aborted) {
      detail.value = null
      error.value = requestError instanceof Error && requestError.message ? requestError.message : '内容分析详情加载失败'
    }
  } finally {
    if (sequence === requestSequence) loading.value = false
  }
}

async function replaceURLAndLoad() {
  await router.replace({
    name: 'admin-analytics-content-detail',
    params: { trackingKey: trackingKey.value },
    query: preservedQuery(),
  })
  await loadDetail()
}

function applyDate(next: AnalyticsDateRange) {
  range.value = { ...next, timezone: ANALYTICS_TIMEZONE }
  void replaceURLAndLoad()
}

function backToList() {
  void router.push({ name: 'admin-analytics-content', query: preservedQuery() })
}

function summaryValue(key: ContentSummaryKey) {
  return detail.value?.summary[key] || 0
}

function localeLabel(locale: string) {
  if (locale === 'en') return 'English'
  if (locale === 'zh') return '中文'
  return '其他'
}

function deviceLabel(device: string) {
  const labels: Record<string, string> = {
    desktop: 'Desktop',
    mobile: 'Mobile',
    tablet: 'Tablet',
    unknown: 'Unknown',
  }
  return labels[device] || labels.unknown
}

function contentTitle() {
  const content = detail.value?.content
  if (!content?.currently_exists) return '已删除内容'
  return content.title || analyticsContentTypeLabel(content.content_type)
}

onMounted(() => {
  void router.replace({
    name: 'admin-analytics-content-detail',
    params: { trackingKey: trackingKey.value },
    query: preservedQuery(),
  })
  void loadDetail()
})

watch(trackingKey, (value, previous) => {
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
      <div>
        <el-button link type="primary" class="mb-2" @click="backToList">← 返回内容分析</el-button>
        <h2 class="text-2xl font-black text-slate-950">营销内容详情</h2>
        <p class="mt-1 text-sm text-slate-500">查看同一 tracking key 的总体表现与语言、设备和页面拆分</p>
      </div>
      <el-alert
        class="mt-4"
        type="info"
        :closable="false"
        show-icon
        title="中英文内容通过同一个 tracking key 合并统计，语言维度仅作为行为拆分。"
      />
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
        v-if="!detail.content.currently_exists"
        type="warning"
        :closable="false"
        show-icon
        title="该内容当前已删除，以下为历史行为数据。"
      />
      <el-alert
        v-else-if="!detail.content.published"
        type="warning"
        :closable="false"
        show-icon
        title="当前仅存在于草稿，未发布。"
      />

      <el-card shadow="never" class="border border-slate-200">
        <div class="grid gap-5 lg:grid-cols-[320px_1fr]">
          <div class="aspect-[16/9] overflow-hidden rounded-xl border border-slate-200">
            <ProductImage :src="detail.content.image_url" :alt="contentTitle()" />
          </div>
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h3 class="text-xl font-black text-slate-950">{{ contentTitle() }}</h3>
              <el-tag :type="detail.content.currently_exists ? 'success' : 'danger'">
                {{ detail.content.currently_exists ? '当前存在' : '已删除' }}
              </el-tag>
              <el-tag v-if="detail.content.currently_exists" :type="detail.content.published ? 'success' : 'warning'">
                {{ detail.content.published ? '已发布' : '草稿' }}
              </el-tag>
            </div>
            <dl class="mt-5 grid gap-3 text-sm sm:grid-cols-[100px_1fr]">
              <dt class="font-bold text-slate-500">内容类型</dt>
              <dd class="text-slate-800">{{ analyticsContentTypeLabel(detail.content.content_type) }}</dd>
              <dt class="font-bold text-slate-500">Tracking key</dt>
              <dd class="break-all font-mono text-slate-700">{{ detail.content.tracking_key }}</dd>
              <dt class="font-bold text-slate-500">Target</dt>
              <dd class="break-all text-slate-700">
                {{ analyticsContentTargetLabel(detail.content.target_type, detail.content.target_value) }}
              </dd>
            </dl>
          </div>
        </div>
      </el-card>

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
            <h3 class="font-black text-slate-950">内容趋势</h3>
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

      <div class="grid gap-5 xl:grid-cols-2">
        <el-card shadow="never" class="border border-slate-200">
          <template #header><h3 class="font-black text-slate-950">语言表现</h3></template>
          <el-table :data="detail.locales" stripe>
            <el-table-column label="语言" min-width="110">
              <template #default="scope">{{ localeLabel(scope.row.locale) }}</template>
            </el-table-column>
            <el-table-column label="曝光" align="right">
              <template #default="scope">{{ formatAnalyticsCount(scope.row.impressions) }}</template>
            </el-table-column>
            <el-table-column label="点击" align="right">
              <template #default="scope">{{ formatAnalyticsCount(scope.row.clicks) }}</template>
            </el-table-column>
            <el-table-column label="独立访客" min-width="100" align="right">
              <template #default="scope">{{ formatAnalyticsCount(scope.row.unique_visitors) }}</template>
            </el-table-column>
            <el-table-column label="CTR" align="right">
              <template #default="scope">{{ formatAnalyticsRate(scope.row.ctr) }}</template>
            </el-table-column>
          </el-table>
        </el-card>

        <el-card shadow="never" class="border border-slate-200">
          <template #header><h3 class="font-black text-slate-950">设备表现</h3></template>
          <el-table :data="detail.devices" stripe>
            <el-table-column label="设备" min-width="110">
              <template #default="scope">{{ deviceLabel(scope.row.device_type) }}</template>
            </el-table-column>
            <el-table-column label="曝光" align="right">
              <template #default="scope">{{ formatAnalyticsCount(scope.row.impressions) }}</template>
            </el-table-column>
            <el-table-column label="点击" align="right">
              <template #default="scope">{{ formatAnalyticsCount(scope.row.clicks) }}</template>
            </el-table-column>
            <el-table-column label="独立访客" min-width="100" align="right">
              <template #default="scope">{{ formatAnalyticsCount(scope.row.unique_visitors) }}</template>
            </el-table-column>
            <el-table-column label="CTR" align="right">
              <template #default="scope">{{ formatAnalyticsRate(scope.row.ctr) }}</template>
            </el-table-column>
          </el-table>
        </el-card>
      </div>

      <el-card shadow="never" class="border border-slate-200">
        <template #header>
          <div>
            <h3 class="font-black text-slate-950">出现页面 TOP 20</h3>
            <p class="mt-1 text-xs text-slate-500">仅展示清洗后的页面路径与聚合事件数</p>
          </div>
        </template>
        <el-table :data="detail.pages" stripe>
          <el-table-column prop="page_path" label="页面路径" min-width="280">
            <template #default="scope"><span class="font-mono text-sm">{{ scope.row.page_path || 'unknown' }}</span></template>
          </el-table-column>
          <el-table-column label="曝光" width="140" align="right">
            <template #default="scope">{{ formatAnalyticsCount(scope.row.impressions) }}</template>
          </el-table-column>
          <el-table-column label="点击" width="140" align="right">
            <template #default="scope">{{ formatAnalyticsCount(scope.row.clicks) }}</template>
          </el-table-column>
          <template #empty><AnalyticsEmptyState title="暂无页面数据" description="本周期内没有该内容的页面路径聚合数据" /></template>
        </el-table>
      </el-card>
    </template>

    <el-card v-else-if="loading" shadow="never"><el-skeleton animated :rows="8" /></el-card>
  </section>
</template>
