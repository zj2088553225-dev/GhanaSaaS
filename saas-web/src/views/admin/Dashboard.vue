<script setup lang="ts">
import { LineChart, type LineSeriesOption } from 'echarts/charts'
import { GridComponent, type GridComponentOption, TooltipComponent, type TooltipComponentOption } from 'echarts/components'
import * as echarts from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { getDashboardSummary, type DashboardSummary, type LowStockSKU } from '../../api/dashboard'
import { formatMoney } from '../../utils/money'

type ChartOption = echarts.ComposeOption<GridComponentOption | TooltipComponentOption | LineSeriesOption>

echarts.use([GridComponent, TooltipComponent, LineChart, CanvasRenderer])

const loading = ref(false)
const summary = ref<DashboardSummary>({
  today_orders: 0,
  today_revenue: 0,
  pending_orders: 0,
  revenue_trend: [],
  low_stock_skus: [],
})
const chartRef = ref<HTMLDivElement | null>(null)
let chart: echarts.ECharts | null = null

const weekRevenue = computed(() => summary.value.revenue_trend.reduce((total, item) => total + item.amount, 0))

function formatAmount(value: number | string) {
  return formatMoney(Number(value))
}

function formatChartAmount(value: unknown) {
  if (Array.isArray(value)) {
    return ''
  }
  return formatMoney(Number(value ?? 0))
}

function compactDate(value: string) {
  const parts = value.split('-')
  return parts.length === 3 ? `${parts[1]}/${parts[2]}` : value
}

function formatSpecs(row: LowStockSKU) {
  if (!row.specs || Object.keys(row.specs).length === 0) {
    return row.name || '-'
  }
  return Object.entries(row.specs)
    .map(([key, value]) => `${key}: ${value}`)
    .join(' / ')
}

function renderChart() {
  if (!chartRef.value) {
    return
  }
  if (!chart) {
    chart = echarts.init(chartRef.value)
  }

  const trend = summary.value.revenue_trend
  const option: ChartOption = {
    color: ['#f97316'],
    grid: {
      left: 42,
      right: 24,
      top: 32,
      bottom: 36,
    },
    tooltip: {
      trigger: 'axis',
      valueFormatter: formatChartAmount,
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: trend.map((item) => compactDate(item.date)),
      axisLine: { lineStyle: { color: '#cbd5e1' } },
      axisLabel: { color: '#64748b', fontWeight: 700 },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value',
      axisLabel: {
        color: '#64748b',
        formatter: (value: number) => formatMoney(value).replace('.00', ''),
      },
      splitLine: { lineStyle: { color: '#e2e8f0' } },
    },
    series: [
      {
        name: '营收',
        type: 'line',
        smooth: true,
        symbol: 'circle',
        symbolSize: 8,
        lineStyle: { width: 4 },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(249, 115, 22, 0.26)' },
            { offset: 1, color: 'rgba(249, 115, 22, 0.02)' },
          ]),
        },
        data: trend.map((item) => item.amount),
      },
    ],
  }
  chart.setOption(option)
}

function resizeChart() {
  chart?.resize()
}

async function loadSummary() {
  loading.value = true
  try {
    summary.value = await getDashboardSummary()
    await nextTick()
    renderChart()
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void loadSummary()
  window.addEventListener('resize', resizeChart)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', resizeChart)
  chart?.dispose()
  chart = null
})
</script>

<template>
  <div v-loading="loading" class="space-y-5">
    <el-row :gutter="16">
      <el-col :xs="24" :md="8" :xl="6">
        <el-card shadow="never" class="dashboard-card">
          <p class="mb-4 text-sm font-bold text-slate-500">今日订单</p>
          <el-statistic :value="summary.today_orders" suffix="单" />
        </el-card>
      </el-col>
      <el-col :xs="24" :md="8" :xl="6">
        <el-card shadow="never" class="dashboard-card">
          <p class="mb-4 text-sm font-bold text-slate-500">今日营收</p>
          <el-statistic :value="summary.today_revenue" :formatter="formatAmount" />
        </el-card>
      </el-col>
      <el-col :xs="24" :md="8" :xl="6">
        <el-card shadow="never" class="dashboard-card">
          <p class="mb-4 text-sm font-bold text-slate-500">待处理订单</p>
          <el-statistic :value="summary.pending_orders" suffix="单" />
        </el-card>
      </el-col>
      <el-col :xs="24" :md="24" :xl="6">
        <el-card shadow="never" class="dashboard-card">
          <p class="mb-4 text-sm font-bold text-slate-500">近 7 日营收</p>
          <el-statistic :value="weekRevenue" :formatter="formatAmount" />
        </el-card>
      </el-col>
    </el-row>

    <el-card shadow="never" class="border border-slate-200">
      <template #header>
        <div class="flex items-center justify-between">
          <div>
            <p class="text-base font-black text-slate-950">营收趋势</p>
            <p class="mt-1 text-xs font-semibold text-slate-500">近 7 天每日营收</p>
          </div>
          <el-button text type="primary" @click="loadSummary">刷新</el-button>
        </div>
      </template>
      <div ref="chartRef" class="h-80 w-full"></div>
    </el-card>

    <el-card shadow="never" class="border border-slate-200">
      <template #header>
        <div class="flex items-center justify-between">
          <div>
            <p class="text-base font-black text-slate-950">库存预警 (Low Stock Alert)</p>
            <p class="mt-1 text-xs font-semibold text-slate-500">剩余库存不高于 5 的 SKU</p>
          </div>
        </div>
      </template>

      <el-table :data="summary.low_stock_skus" size="small" border>
        <el-table-column prop="product_name" label="商品" min-width="180" />
        <el-table-column label="规格" min-width="180">
          <template #default="{ row }">
            {{ formatSpecs(row) }}
          </template>
        </el-table-column>
        <el-table-column prop="sellable_stock" label="剩余库存" width="110" align="right">
          <template #default="{ row }">
            <el-tag :type="row.sellable_stock === 0 ? 'danger' : 'warning'" effect="light">
              {{ row.sellable_stock }}
            </el-tag>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<style scoped>
.dashboard-card {
  min-height: 132px;
  border: 1px solid #e2e8f0;
}
</style>
