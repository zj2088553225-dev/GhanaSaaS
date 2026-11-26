<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { getGlobalDashboardSummary, type GlobalDashboardSummary } from '../../api/superDashboard'
import { formatMoney as formatGhsMoney } from '../../utils/money'

const loading = ref(false)
const summary = ref<GlobalDashboardSummary>({
  total_tenants: 0,
  total_revenue: 0,
  total_orders: 0,
  active_users: 0,
})

const metrics = computed(() => [
  { label: '租户总数', value: summary.value.total_tenants, suffix: '家' },
  { label: '平台总流水', value: summary.value.total_revenue, formatter: formatMoney },
  { label: '订单总数', value: summary.value.total_orders, suffix: '单' },
  { label: 'C 端买家', value: summary.value.active_users, suffix: '人' },
])

onMounted(() => {
  void loadSummary()
})

async function loadSummary() {
  loading.value = true
  try {
    summary.value = await getGlobalDashboardSummary()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '全局看板加载失败')
  } finally {
    loading.value = false
  }
}

function formatMoney(value: number | string) {
  return formatGhsMoney(Number(value))
}
</script>

<template>
  <section v-loading="loading" class="space-y-4">
    <div class="flex flex-col gap-3 rounded-2xl bg-white p-4 shadow-sm md:flex-row md:items-center md:justify-between">
      <div>
        <h2 class="text-xl font-black text-slate-950">全局看板</h2>
        <p class="mt-1 text-sm text-slate-500">平台级租户、订单、流水与买家概览</p>
      </div>

      <el-button @click="loadSummary">刷新</el-button>
    </div>

    <el-row :gutter="16">
      <el-col v-for="metric in metrics" :key="metric.label" :xs="24" :sm="12" :xl="6">
        <el-card shadow="never" class="global-metric-card">
          <p class="mb-4 text-sm font-bold text-slate-500">{{ metric.label }}</p>
          <el-statistic :value="metric.value" :suffix="metric.suffix" :formatter="metric.formatter" />
        </el-card>
      </el-col>
    </el-row>
  </section>
</template>

<style scoped>
.global-metric-card {
  min-height: 132px;
  border: 1px solid #e2e8f0;
}
</style>
