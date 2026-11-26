<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useRoute, useRouter } from 'vue-router'
import {
  ANALYTICS_TIMEZONE,
  exportContentAnalytics,
  getContentAnalytics,
  type AnalyticsDateRange,
  type AnalyticsSortOrder,
  type ContentAnalyticsItem,
  type ContentAnalyticsList,
  type ContentAnalyticsMetric,
  type ContentAnalyticsType,
} from '../../../api/analytics'
import ProductImage from '../../../components/ProductImage.vue'
import AnalyticsDateRangeFilter from '../../../components/analytics/AnalyticsDateRangeFilter.vue'
import AnalyticsEmptyState from '../../../components/analytics/AnalyticsEmptyState.vue'
import AnalyticsNavigation from '../../../components/analytics/AnalyticsNavigation.vue'
import { normalizedAnalyticsRange } from '../../../utils/analyticsDateRange'
import { downloadAnalyticsCSV } from '../../../utils/analyticsDownload'
import {
  analyticsContentTypeLabel,
  formatAnalyticsCount,
  formatAnalyticsRate,
} from '../../../utils/analyticsFormat'

type ExistsFilter = '' | 'true' | 'false'

const contentTypes: ContentAnalyticsType[] = [
  'hero',
  'hero_banner',
  'hero_slide',
  'promo_banner',
  'image',
  'image_text',
  'contact',
  'category_item',
  'feature_grid',
  'link',
  'other',
]
const contentTypeSet = new Set(contentTypes)
const contentMetricSet = new Set<ContentAnalyticsMetric>(['impressions', 'clicks', 'unique_visitors', 'ctr'])

const route = useRoute()
const router = useRouter()
const range = ref<AnalyticsDateRange>(normalizedAnalyticsRange(route.query.from, route.query.to))
const search = ref(routeString('search').slice(0, 200))
const contentType = ref<ContentAnalyticsType | ''>(readContentType())
const existsFilter = ref<ExistsFilter>(readExistsFilter())
const sort = ref<ContentAnalyticsMetric>(readSort())
const order = ref<AnalyticsSortOrder>(routeString('order') === 'asc' ? 'asc' : 'desc')
const page = ref(readPositiveInt('page', 1, 1_000_000))
const pageSize = ref(readPositiveInt('page_size', 20, 100))
const minImpressions = ref(readNonNegativeInt('min_impressions', 100, 1_000_000_000))
const list = ref<ContentAnalyticsList | null>(null)
const loading = ref(false)
const error = ref('')
const exportLoading = ref(false)
let requestSequence = 0
let requestController: AbortController | null = null

const sortPreset = computed({
  get: () => `${sort.value}:${order.value}`,
  set: (value: string) => {
    const [metric, direction] = value.split(':')
    if (contentMetricSet.has(metric as ContentAnalyticsMetric)) sort.value = metric as ContentAnalyticsMetric
    order.value = direction === 'asc' ? 'asc' : 'desc'
  },
})
const usesCTRThreshold = computed(() => sort.value === 'ctr')

function routeString(key: string) {
  const value = route.query[key]
  return typeof value === 'string' ? value.trim() : ''
}

function readPositiveInt(key: string, fallback: number, maximum: number) {
  const value = routeString(key)
  if (!/^[1-9]\d*$/.test(value)) return fallback
  const parsed = Number(value)
  return Number.isSafeInteger(parsed) && parsed <= maximum ? parsed : fallback
}

function readNonNegativeInt(key: string, fallback: number, maximum: number) {
  const value = routeString(key)
  if (!/^\d+$/.test(value)) return fallback
  const parsed = Number(value)
  return Number.isSafeInteger(parsed) && parsed <= maximum ? parsed : fallback
}

function readContentType(): ContentAnalyticsType | '' {
  const value = routeString('content_type') as ContentAnalyticsType
  return contentTypeSet.has(value) ? value : ''
}

function readExistsFilter(): ExistsFilter {
  const value = routeString('currently_exists')
  return value === 'true' || value === 'false' ? value : ''
}

function readSort(): ContentAnalyticsMetric {
  const value = routeString('sort') as ContentAnalyticsMetric
  return contentMetricSet.has(value) ? value : 'clicks'
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
  if (search.value.trim()) query.search = search.value.trim()
  if (contentType.value) query.content_type = contentType.value
  if (existsFilter.value) query.currently_exists = existsFilter.value
  if (usesCTRThreshold.value) query.min_impressions = String(minImpressions.value)
  return query
}

async function loadContent() {
  requestController?.abort()
  requestController = new AbortController()
  const sequence = ++requestSequence
  loading.value = true
  error.value = ''
  try {
    const result = await getContentAnalytics(
      {
        from: range.value.from,
        to: range.value.to,
        timezone: ANALYTICS_TIMEZONE,
        page: page.value,
        page_size: pageSize.value,
        search: search.value.trim() || undefined,
        content_type: contentType.value || undefined,
        currently_exists: existsFilter.value ? existsFilter.value === 'true' : undefined,
        sort: sort.value,
        order: order.value,
        min_impressions: usesCTRThreshold.value ? minImpressions.value : undefined,
      },
      { signal: requestController.signal },
    )
    if (sequence === requestSequence && !requestController.signal.aborted) list.value = result
  } catch (requestError) {
    if (sequence === requestSequence && !requestController.signal.aborted) {
      error.value = requestError instanceof Error && requestError.message ? requestError.message : '内容分析加载失败'
    }
  } finally {
    if (sequence === requestSequence) loading.value = false
  }
}

async function replaceURLAndLoad() {
  await router.replace({ name: 'admin-analytics-content', query: listURLQuery() })
  await loadContent()
}

function applyDate(next: AnalyticsDateRange) {
  range.value = { ...next, timezone: ANALYTICS_TIMEZONE }
  page.value = 1
  void replaceURLAndLoad()
}

function applyFilters() {
  search.value = search.value.trim().slice(0, 200)
  minImpressions.value = Math.max(0, Math.min(1_000_000_000, Number(minImpressions.value) || 0))
  page.value = 1
  void replaceURLAndLoad()
}

function changePage(value: number) {
  page.value = value
  void replaceURLAndLoad()
}

function changePageSize(value: number) {
  pageSize.value = value
  page.value = 1
  void replaceURLAndLoad()
}

function openDetail(item: ContentAnalyticsItem) {
  void router.push({
    name: 'admin-analytics-content-detail',
    params: { trackingKey: item.tracking_key },
    query: listURLQuery(),
  })
}

function contentTitle(item: ContentAnalyticsItem) {
  return item.currently_exists ? item.title || analyticsContentTypeLabel(item.content_type) : '已删除内容'
}

function shortTrackingKey(value: string) {
  return value.length > 18 ? `${value.slice(0, 8)}…${value.slice(-6)}` : value
}

async function exportCSV() {
  if (exportLoading.value) return
  exportLoading.value = true
  try {
    const download = await exportContentAnalytics({
      from: range.value.from,
      to: range.value.to,
      timezone: ANALYTICS_TIMEZONE,
      search: search.value.trim() || undefined,
      content_type: contentType.value || undefined,
      currently_exists: existsFilter.value ? existsFilter.value === 'true' : undefined,
      sort: sort.value,
      order: order.value,
      min_impressions: usesCTRThreshold.value ? minImpressions.value : undefined,
    })
    downloadAnalyticsCSV(download)
    ElMessage.success('内容分析 CSV 已导出')
  } catch {
    ElMessage.error('导出失败，请缩小日期范围或筛选条件后重试')
  } finally {
    exportLoading.value = false
  }
}

onMounted(() => {
  void router.replace({ name: 'admin-analytics-content', query: listURLQuery() })
  void loadContent()
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
        <h2 class="text-2xl font-black text-slate-950">内容分析</h2>
        <p class="mt-1 text-sm text-slate-500">比较营销图片和内容入口的曝光、点击与 CTR</p>
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
      <el-divider />
      <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-[minmax(220px,1fr)_180px_160px_180px_170px_auto] xl:items-end">
        <div>
          <label class="mb-1.5 block text-xs font-bold text-slate-500">搜索内容</label>
          <el-input v-model="search" maxlength="200" clearable placeholder="English title 或 tracking key" @keyup.enter="applyFilters" />
        </div>
        <div>
          <label class="mb-1.5 block text-xs font-bold text-slate-500">内容类型</label>
          <el-select v-model="contentType" class="w-full">
            <el-option label="全部类型" value="" />
            <el-option v-for="item in contentTypes" :key="item" :label="analyticsContentTypeLabel(item)" :value="item" />
          </el-select>
        </div>
        <div>
          <label class="mb-1.5 block text-xs font-bold text-slate-500">当前状态</label>
          <el-select v-model="existsFilter" class="w-full">
            <el-option label="全部" value="" />
            <el-option label="当前存在" value="true" />
            <el-option label="已删除" value="false" />
          </el-select>
        </div>
        <div>
          <label class="mb-1.5 block text-xs font-bold text-slate-500">快速排序</label>
          <el-select v-model="sortPreset" class="w-full">
            <el-option label="点击最高" value="clicks:desc" />
            <el-option label="曝光最高" value="impressions:desc" />
            <el-option label="独立访客最高" value="unique_visitors:desc" />
            <el-option label="CTR 最高" value="ctr:desc" />
            <el-option label="CTR 最低" value="ctr:asc" />
          </el-select>
        </div>
        <div v-if="usesCTRThreshold">
          <label class="mb-1.5 block text-xs font-bold text-slate-500">最小曝光</label>
          <el-input-number v-model="minImpressions" :min="0" :max="1000000000" :step="50" class="w-full" />
        </div>
        <el-button type="primary" @click="applyFilters">查询内容</el-button>
      </div>
    </el-card>

    <el-alert
      type="warning"
      :closable="false"
      show-icon
      title="高曝光低点击通常意味着素材、标题、展示位置或目标内容需要进一步检查。"
      description="该信号用于辅助排查，不代表图片本身一定存在问题。"
    />

    <el-card shadow="never" class="border border-slate-200">
      <template #header>
        <div class="flex items-center justify-between">
          <div>
            <h3 class="font-black text-slate-950">营销内容表现</h3>
            <p class="mt-1 text-xs text-slate-500">默认按点击从高到低；CTR 排序默认要求至少 100 次曝光</p>
          </div>
          <div class="flex items-center gap-2">
            <el-button v-if="error" size="small" @click="loadContent">重试</el-button>
            <el-button type="primary" plain :loading="exportLoading" :disabled="exportLoading" @click="exportCSV">导出 CSV</el-button>
          </div>
        </div>
      </template>
      <el-alert v-if="error" class="mb-4" type="error" :closable="false" :title="error" />
      <el-table
        v-loading="loading"
        :data="list?.items || []"
        stripe
        row-key="tracking_key"
        @row-click="openDetail"
      >
        <el-table-column label="内容" min-width="300" fixed="left">
          <template #default="scope">
            <div class="flex items-center gap-3">
              <div class="h-14 w-20 shrink-0 overflow-hidden rounded-lg border border-slate-200">
                <ProductImage :src="scope.row.image_url" :alt="contentTitle(scope.row)" />
              </div>
              <div class="min-w-0">
                <p class="truncate font-semibold text-slate-900">{{ contentTitle(scope.row) }}</p>
                <el-tooltip :content="scope.row.tracking_key" placement="top">
                  <p class="mt-1 font-mono text-xs text-slate-400">{{ shortTrackingKey(scope.row.tracking_key) }}</p>
                </el-tooltip>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="类型" min-width="125">
          <template #default="scope">{{ analyticsContentTypeLabel(scope.row.content_type) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="105">
          <template #default="scope">
            <el-tag :type="scope.row.currently_exists ? 'success' : 'danger'">
              {{ scope.row.currently_exists ? '当前存在' : '已删除' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="发布状态" width="105">
          <template #default="scope">
            <el-tag v-if="scope.row.currently_exists" :type="scope.row.published ? 'success' : 'warning'">
              {{ scope.row.published ? '已发布' : '草稿' }}
            </el-tag>
            <span v-else class="text-slate-400">—</span>
          </template>
        </el-table-column>
        <el-table-column label="曝光" width="115" align="right">
          <template #default="scope">{{ formatAnalyticsCount(scope.row.impressions) }}</template>
        </el-table-column>
        <el-table-column label="点击" width="115" align="right">
          <template #default="scope">{{ formatAnalyticsCount(scope.row.clicks) }}</template>
        </el-table-column>
        <el-table-column label="独立访客" width="125" align="right">
          <template #default="scope">{{ formatAnalyticsCount(scope.row.unique_visitors) }}</template>
        </el-table-column>
        <el-table-column label="CTR" width="110" align="right">
          <template #default="scope">{{ formatAnalyticsRate(scope.row.ctr) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="105" fixed="right">
          <template #default="scope"><el-button link type="primary" @click.stop="openDetail(scope.row)">查看详情</el-button></template>
        </el-table-column>
        <template #empty><AnalyticsEmptyState title="暂无内容行为数据" description="营销内容产生曝光和点击后将在这里显示" /></template>
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
