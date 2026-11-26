<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import type { AnalyticsDateRange } from '../../api/analytics'
import {
  analyticsPresetRange,
  type AnalyticsDatePreset,
  validateAnalyticsDateRange,
} from '../../utils/analyticsDateRange'

const props = withDefaults(
  defineProps<{
    modelValue: AnalyticsDateRange
    loading?: boolean
  }>(),
  { loading: false },
)

const emit = defineEmits<{
  'update:modelValue': [value: AnalyticsDateRange]
  apply: [value: AnalyticsDateRange]
}>()

const preset = ref<AnalyticsDatePreset>('last7')
const draftDates = ref<[string, string] | null>([props.modelValue.from, props.modelValue.to])

const rangeText = computed(() => {
  const dates = draftDates.value
  return dates ? `${dates[0]} 至 ${dates[1]}` : '请选择日期范围'
})

watch(
  () => props.modelValue,
  (value) => {
    draftDates.value = [value.from, value.to]
  },
  { deep: true },
)

function choosePreset(value: AnalyticsDatePreset) {
  preset.value = value
  if (value === 'custom') {
    return
  }
  const range = analyticsPresetRange(value)
  draftDates.value = [range.from, range.to]
}

function applyRange() {
  const dates = draftDates.value
  const next: AnalyticsDateRange = {
    from: dates?.[0] || '',
    to: dates?.[1] || '',
    timezone: props.modelValue.timezone,
  }
  const validationError = validateAnalyticsDateRange(next)
  if (validationError) {
    ElMessage.warning(validationError)
    return
  }
  emit('update:modelValue', next)
  emit('apply', next)
}
</script>

<template>
  <div class="flex flex-col gap-3 lg:flex-row lg:items-end">
    <div class="min-w-40">
      <label class="mb-1.5 block text-xs font-bold text-slate-500">快捷范围</label>
      <el-select :model-value="preset" class="w-full" aria-label="Analytics 日期快捷范围" @change="choosePreset">
        <el-option label="今天" value="today" />
        <el-option label="昨天" value="yesterday" />
        <el-option label="近 7 天" value="last7" />
        <el-option label="近 30 天" value="last30" />
        <el-option label="自定义" value="custom" />
      </el-select>
    </div>

    <div class="min-w-0 flex-1 lg:min-w-80">
      <label class="mb-1.5 block text-xs font-bold text-slate-500">日期范围（GMT / Accra Time）</label>
      <el-date-picker
        v-if="preset === 'custom'"
        v-model="draftDates"
        type="daterange"
        value-format="YYYY-MM-DD"
        format="YYYY-MM-DD"
        range-separator="至"
        start-placeholder="开始日期"
        end-placeholder="结束日期"
        unlink-panels
        class="analytics-date-picker"
        aria-label="Analytics 自定义日期范围"
      />
      <div v-else class="flex h-8 items-center rounded border border-slate-200 bg-slate-50 px-3 text-sm font-semibold text-slate-700">
        {{ rangeText }}
      </div>
    </div>

    <el-button type="primary" :loading="loading" @click="applyRange">查询</el-button>
  </div>
</template>

<style scoped>
.analytics-date-picker {
  width: 100%;
}
</style>
