<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  getSuperReservationProduct,
  getSuperReservationProductAnalytics,
  type ReservationProductAdmin,
  type ReservationProductAnalytics,
} from '../../../api/reservations'
import { adminPath } from '../../../utils/adminPath'

const route = useRoute()
const router = useRouter()
const productID = computed(() => Number(Array.isArray(route.params.id) ? route.params.id[0] : route.params.id) || 0)
const product = ref<ReservationProductAdmin | null>(null)
const analytics = ref<ReservationProductAnalytics | null>(null)
const loading = ref(false)

const funnel = computed(() => analytics.value ? [
  { label: '预约详情访问', value: analytics.value.page_views },
  { label: '匿名感兴趣', value: analytics.value.unique_interests },
  { label: '提交手机号', value: analytics.value.reservation_submissions },
  { label: '完成验证', value: analytics.value.verified_reservations },
] : [])

onMounted(() => void loadAnalytics())

async function loadAnalytics() {
  loading.value = true
  try {
    const [productResult, analyticsResult] = await Promise.all([
      getSuperReservationProduct(productID.value),
      getSuperReservationProductAnalytics(productID.value),
    ])
    product.value = productResult
    analytics.value = analyticsResult
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '预约分析加载失败')
  } finally {
    loading.value = false
  }
}

function formatTime(value: string | null) {
  return value ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '-'
}
</script>

<template>
  <section v-loading="loading" class="space-y-5">
    <header class="flex flex-col gap-4 rounded-2xl border border-slate-200 bg-white p-6 shadow-sm sm:flex-row sm:items-center sm:justify-between">
      <div>
        <p class="text-xs font-bold uppercase tracking-[0.22em] text-orange-500">Exact Reservation Funnel</p>
        <h1 class="mt-1 text-2xl font-black text-slate-950">{{ product?.title_zh || product?.title_en || '预约分析' }}</h1>
        <p class="mt-1 text-sm text-slate-500">后台精确指标；匿名兴趣与已验证预约分别统计。</p>
      </div>
      <div class="flex gap-2"><el-button @click="router.push(adminPath('/reservation-products'))">返回列表</el-button><el-button type="primary" @click="router.push(adminPath(`/reservation-products/${productID}/edit`))">编辑商品</el-button></div>
    </header>

    <template v-if="analytics">
      <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <div v-for="step in funnel" :key="step.label" class="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
          <p class="text-xs font-bold text-slate-500">{{ step.label }}</p>
          <p class="mt-2 text-3xl font-black text-slate-950">{{ step.value.toLocaleString() }}</p>
        </div>
      </div>

      <div class="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
        <h2 class="text-lg font-black text-slate-900">预约漏斗</h2>
        <div class="mt-5 flex flex-col items-stretch gap-2 lg:flex-row lg:items-center">
          <template v-for="(step, index) in funnel" :key="step.label">
            <div class="flex-1 rounded-xl bg-slate-50 px-5 py-4 text-center"><p class="text-sm font-bold text-slate-500">{{ step.label }}</p><p class="mt-1 text-2xl font-black text-slate-950">{{ step.value }}</p></div>
            <span v-if="index < funnel.length - 1" class="text-center text-xl font-black text-orange-400">→</span>
          </template>
        </div>
        <p class="mt-4 rounded-xl bg-amber-50 px-4 py-3 text-xs leading-5 text-amber-800">当前没有可靠的独立预约曝光事件源，因此 impressions 与 page_views 不伪造历史值；既有 Storefront Analytics 指标定义保持不变。</p>
      </div>

      <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <div class="rounded-2xl border border-slate-200 bg-white p-5"><p class="text-xs font-bold text-slate-500">验证率</p><p class="mt-2 text-2xl font-black">{{ (analytics.verification_rate * 100).toFixed(1) }}%</p></div>
        <div class="rounded-2xl border border-slate-200 bg-white p-5"><p class="text-xs font-bold text-slate-500">取消预约</p><p class="mt-2 text-2xl font-black">{{ analytics.cancelled_reservations }}</p></div>
        <div class="rounded-2xl border border-slate-200 bg-white p-5"><p class="text-xs font-bold text-slate-500">拦截次数</p><p class="mt-2 text-2xl font-black">{{ analytics.blocked_attempts }}</p></div>
        <div class="rounded-2xl border border-slate-200 bg-white p-5"><p class="text-xs font-bold text-slate-500">曝光（独立源）</p><p class="mt-2 text-2xl font-black">{{ analytics.impressions }}</p></div>
      </div>

      <div class="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm">
        <div class="border-b border-slate-100 p-5"><h2 class="text-lg font-black text-slate-900">已验证联系方式</h2><p class="mt-1 text-xs text-slate-500">仅显示脱敏号码。本轮不提供明文号码或 CSV 导出。</p></div>
        <el-table :data="analytics.contacts" empty-text="暂无已验证预约">
          <el-table-column prop="masked_phone" label="脱敏号码" min-width="180" />
          <el-table-column label="渠道" width="120"><template #default="scope">{{ scope.row.channel === 'sms' ? 'SMS' : 'WhatsApp' }}</template></el-table-column>
          <el-table-column label="验证时间" min-width="190"><template #default="scope">{{ formatTime(scope.row.verified_at) }}</template></el-table-column>
          <el-table-column label="状态" width="110"><template #default="scope"><el-tag :type="scope.row.status === 'active' ? 'success' : 'info'">{{ scope.row.status === 'active' ? '有效' : '已取消' }}</el-tag></template></el-table-column>
        </el-table>
      </div>
    </template>
  </section>
</template>
