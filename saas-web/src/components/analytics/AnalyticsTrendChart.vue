<script setup lang="ts">
import { LineChart, type LineSeriesOption } from 'echarts/charts'
import {
  GridComponent,
  LegendComponent,
  TooltipComponent,
  type GridComponentOption,
  type LegendComponentOption,
  type TooltipComponentOption,
} from 'echarts/components'
import * as echarts from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { AnalyticsGranularity } from '../../api/analytics'
import { formatAnalyticsCount } from '../../utils/analyticsFormat'
import AnalyticsEmptyState from './AnalyticsEmptyState.vue'

export interface AnalyticsChartPoint {
  bucket: string
  values: Record<string, number>
}

export interface AnalyticsChartMetric {
  key: string
  label: string
  color?: string
}

type ChartOption = echarts.ComposeOption<
  GridComponentOption | LegendComponentOption | TooltipComponentOption | LineSeriesOption
>

echarts.use([GridComponent, LegendComponent, TooltipComponent, LineChart, CanvasRenderer])

const props = withDefaults(
  defineProps<{
    points: AnalyticsChartPoint[]
    metrics: AnalyticsChartMetric[]
    modelValue: string[]
    granularity: AnalyticsGranularity
    loading?: boolean
  }>(),
  { loading: false },
)

const emit = defineEmits<{
  'update:modelValue': [value: string[]]
}>()

const chartRef = ref<HTMLDivElement | null>(null)
const selected = ref<string[]>([...props.modelValue])
let chart: echarts.ECharts | null = null
let resizeObserver: ResizeObserver | null = null

const hasData = computed(() => props.points.some((point) => Object.values(point.values).some((value) => value > 0)))
const selectedMetrics = computed(() => props.metrics.filter((metric) => selected.value.includes(metric.key)))

watch(
  () => props.modelValue,
  (value) => {
    selected.value = [...value]
  },
  { deep: true },
)

watch(
  [() => props.points, selectedMetrics, () => props.granularity],
  () => {
    void nextTick(renderChart)
  },
  { deep: true },
)

function updateSelected(value: string[]) {
  const limited = value.slice(0, 5)
  selected.value = limited
  emit('update:modelValue', limited)
}

function formatBucket(bucket: string) {
  if (props.granularity === 'hour') {
    return bucket.replace('T', ' ').slice(5, 16)
  }
  if (props.granularity === 'week') {
    return `${bucket.slice(5)} 周起`
  }
  return bucket.slice(5)
}

function renderChart() {
  if (!chartRef.value) {
    return
  }
  if (!chart) {
    chart = echarts.init(chartRef.value)
  }
  const metrics = selectedMetrics.value
  const option: ChartOption = {
    animationDuration: 300,
    color: metrics.map((metric) => metric.color || '#f97316'),
    grid: { left: 58, right: 24, top: 44, bottom: 42 },
    legend: { top: 4, data: metrics.map((metric) => metric.label), textStyle: { color: '#475569' } },
    tooltip: {
      trigger: 'axis',
      valueFormatter: (value: unknown) => formatAnalyticsCount(typeof value === 'number' ? value : Number(value)),
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: props.points.map((point) => formatBucket(point.bucket)),
      axisLine: { lineStyle: { color: '#cbd5e1' } },
      axisLabel: { color: '#64748b', hideOverlap: true },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      minInterval: 1,
      axisLabel: { color: '#64748b', formatter: (value: number) => formatAnalyticsCount(value) },
      splitLine: { lineStyle: { color: '#e2e8f0' } },
    },
    series: metrics.map((metric) => ({
      name: metric.label,
      type: 'line',
      smooth: true,
      showSymbol: props.points.length <= 31,
      symbolSize: 6,
      lineStyle: { width: 3 },
      emphasis: { focus: 'series' },
      data: props.points.map((point) => point.values[metric.key] || 0),
    })),
  }
  chart.setOption(option, { notMerge: true })
}

onMounted(() => {
  void nextTick(() => {
    renderChart()
    if (chartRef.value && typeof ResizeObserver !== 'undefined') {
      resizeObserver = new ResizeObserver(() => chart?.resize())
      resizeObserver.observe(chartRef.value)
    }
  })
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  resizeObserver = null
  chart?.dispose()
  chart = null
})
</script>

<template>
  <div class="relative">
    <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <el-checkbox-group :model-value="selected" :max="5" size="small" @change="updateSelected">
        <el-checkbox-button v-for="metric in metrics" :key="metric.key" :value="metric.key">
          {{ metric.label }}
        </el-checkbox-button>
      </el-checkbox-group>
      <span class="text-xs text-slate-400">最多同时显示 5 项</span>
    </div>

    <div v-loading="loading" class="relative min-h-80">
      <div ref="chartRef" class="h-80 w-full" aria-label="Analytics 趋势图"></div>
      <div v-if="!loading && !hasData" class="absolute inset-0 flex items-center justify-center bg-white/85">
        <AnalyticsEmptyState />
      </div>
    </div>
  </div>
</template>
