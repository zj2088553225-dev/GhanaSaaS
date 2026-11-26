<script setup lang="ts">
import { computed } from 'vue'
import type { AnalyticsMetricValue } from '../../api/analytics'
import { formatAnalyticsChange, formatAnalyticsCount, formatAnalyticsRate } from '../../utils/analyticsFormat'

const props = withDefaults(
  defineProps<{
    label: string
    tooltip: string
    metric?: AnalyticsMetricValue
    value?: number
    rate?: boolean
    loading?: boolean
    showChange?: boolean
  }>(),
  { metric: undefined, value: undefined, rate: false, loading: false, showChange: true },
)

const valueText = computed(() =>
  props.rate
    ? formatAnalyticsRate(props.metric?.value ?? props.value)
    : formatAnalyticsCount(props.metric?.value ?? props.value),
)
const changeText = computed(() => formatAnalyticsChange(props.metric?.change_rate))
const changeClass = computed(() => {
  const change = props.metric?.change_rate
  if (change === null || change === undefined) {
    return 'text-blue-600'
  }
  if (change > 0) {
    return 'text-emerald-600'
  }
  if (change < 0) {
    return 'text-red-600'
  }
  return 'text-slate-500'
})
const changeDirection = computed(() => {
  const change = props.metric?.change_rate
  if (change === null || change === undefined) {
    return '新增'
  }
  if (change > 0) {
    return '上升'
  }
  if (change < 0) {
    return '下降'
  }
  return '持平'
})
</script>

<template>
  <el-card shadow="never" class="analytics-metric-card h-full border border-slate-200">
    <el-skeleton v-if="loading" animated :rows="2" />
    <template v-else>
      <div class="flex items-center gap-1.5 text-sm font-bold text-slate-500">
        <span>{{ label }}</span>
        <el-tooltip :content="tooltip" placement="top">
          <span class="inline-flex h-4 w-4 cursor-help items-center justify-center rounded-full border border-slate-300 text-[10px]">?</span>
        </el-tooltip>
      </div>
      <p class="mt-4 text-3xl font-black tracking-tight text-slate-950">{{ valueText }}</p>
      <p v-if="showChange" class="mt-3 text-xs font-bold" :class="changeClass">
        {{ changeDirection }} {{ changeText }}
        <span class="font-medium text-slate-400"> · 较上一等长周期</span>
      </p>
    </template>
  </el-card>
</template>

<style scoped>
.analytics-metric-card {
  min-height: 154px;
}
</style>
