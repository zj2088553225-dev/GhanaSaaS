<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import ProductImage from '../../../components/ProductImage.vue'
import {
  archiveSuperReservationProduct,
  getSuperReservationProducts,
  type ReservationProductAdmin,
  type ReservationProductStatus,
} from '../../../api/reservations'
import { adminPath } from '../../../utils/adminPath'

const PAGE_SIZE = 20
const router = useRouter()
const items = ref<ReservationProductAdmin[]>([])
const loading = ref(false)
const total = ref(0)
const page = ref(1)
const search = ref('')
const status = ref<ReservationProductStatus | ''>('')
const pages = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))

const statusLabels: Record<ReservationProductStatus, string> = {
  draft: '草稿', scheduled: '定时开放', open: '开放', closed: '关闭', archived: '已归档',
}
const heatLabels = {
  new: '新品', popular: '较受欢迎', trending: '热度上升', highly_requested: '需求很高',
}

onMounted(() => void loadItems())

async function loadItems() {
  loading.value = true
  try {
    const result = await getSuperReservationProducts({
      page: page.value,
      page_size: PAGE_SIZE,
      search: search.value.trim() || undefined,
      status: status.value || undefined,
    })
    items.value = result.items
    total.value = result.total
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '预约商品加载失败')
  } finally {
    loading.value = false
  }
}

async function applyFilters() {
  page.value = 1
  await loadItems()
}

async function changePage(next: number) {
  page.value = next
  await loadItems()
}

function formatTime(value: string | null) {
  return value ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '不限'
}

function editProduct(id: number) {
  void router.push(adminPath(`/reservation-products/${id}/edit`))
}

function showAnalytics(id: number) {
  void router.push(adminPath(`/reservation-products/${id}/analytics`))
}

function previewProduct(item: ReservationProductAdmin) {
  const route = router.resolve(`/en/reservations/${item.slug}`)
  window.open(route.href, '_blank', 'noopener,noreferrer')
}

async function archiveProduct(item: ReservationProductAdmin) {
  try {
    await ElMessageBox.confirm(`归档“${item.title_zh || item.title_en}”？历史预约与统计会保留。`, '归档预约商品', {
      type: 'warning', confirmButtonText: '归档', cancelButtonText: '取消',
    })
    await archiveSuperReservationProduct(item.id)
    ElMessage.success('已归档')
    await loadItems()
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') ElMessage.error(error instanceof Error ? error.message : '归档失败')
  }
}
</script>

<template>
  <section class="space-y-5">
    <header class="flex flex-col gap-4 rounded-2xl border border-slate-200 bg-white p-6 shadow-sm lg:flex-row lg:items-center lg:justify-between">
      <div>
        <p class="text-xs font-bold uppercase tracking-[0.22em] text-orange-500">Super Admin</p>
        <h1 class="mt-1 text-2xl font-black text-slate-950">新品预约</h1>
        <p class="mt-1 text-sm text-slate-500">管理独立测品与关联 SPU；不复制价格、库存或交易信息。</p>
      </div>
      <el-button type="primary" size="large" @click="router.push(adminPath('/reservation-products/create'))">创建测品</el-button>
    </header>

    <div class="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
      <form class="flex flex-col gap-3 md:flex-row" @submit.prevent="applyFilters">
        <el-input v-model="search" clearable placeholder="搜索标题或 slug" class="max-w-md" @clear="applyFilters" />
        <el-select v-model="status" clearable placeholder="全部状态" class="w-44" @change="applyFilters">
          <el-option v-for="(label, value) in statusLabels" :key="value" :label="label" :value="value" />
        </el-select>
        <el-button native-type="submit" type="primary">筛选</el-button>
      </form>
    </div>

    <div class="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm">
      <el-table v-loading="loading" :data="items" empty-text="暂无预约商品">
        <el-table-column label="商品" min-width="250">
          <template #default="scope">
            <div class="flex items-center gap-3 py-1">
              <div class="h-14 w-14 shrink-0 overflow-hidden rounded-xl"><ProductImage :src="scope.row.main_image_url" :alt="scope.row.title_en" /></div>
              <div class="min-w-0">
                <p class="truncate font-black text-slate-900">{{ scope.row.title_zh || scope.row.title_en }}</p>
                <p class="truncate text-xs text-slate-400">{{ scope.row.slug }}</p>
                <span v-if="scope.row.linked_product_id" class="mt-1 inline-block text-xs font-semibold text-blue-600">关联 SPU #{{ scope.row.linked_product_id }}</span>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="105">
          <template #default="scope"><el-tag :type="scope.row.status === 'open' ? 'success' : scope.row.status === 'archived' ? 'info' : 'warning'">{{ statusLabels[scope.row.status as ReservationProductStatus] }}</el-tag></template>
        </el-table-column>
        <el-table-column label="开放时间" min-width="210">
          <template #default="scope">
            <p class="text-xs text-slate-600">{{ formatTime(scope.row.reservation_start_at) }}</p>
            <p class="text-xs text-slate-400">至 {{ formatTime(scope.row.reservation_end_at) }}</p>
          </template>
        </el-table-column>
        <el-table-column prop="exact_interest_count" label="匿名兴趣" width="95" align="right" />
        <el-table-column prop="exact_verified_count" label="验证预约" width="95" align="right" />
        <el-table-column label="有效热度" width="120">
          <template #default="scope">
            <p class="font-bold text-slate-700">{{ heatLabels[scope.row.effective_heat_level as keyof typeof heatLabels] }}</p>
            <p class="text-[11px] text-slate-400">自动：{{ heatLabels[scope.row.automatic_heat_level as keyof typeof heatLabels] }}<span v-if="scope.row.heat_level_override"> · 已覆盖</span></p>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="245" fixed="right">
          <template #default="scope">
            <el-button link type="primary" @click="editProduct(scope.row.id)">编辑</el-button>
            <el-button link type="success" @click="showAnalytics(scope.row.id)">分析</el-button>
            <el-button link @click="previewProduct(scope.row)">预览</el-button>
            <el-button v-if="scope.row.status !== 'archived'" link type="danger" @click="archiveProduct(scope.row)">归档</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="total > PAGE_SIZE" class="flex items-center justify-end border-t border-slate-100 p-4">
        <el-pagination layout="prev, pager, next, total" :current-page="page" :page-size="PAGE_SIZE" :total="total" @current-change="changePage" />
      </div>
    </div>
    <p class="text-xs text-slate-400">共 {{ total }} 个测品，{{ pages }} 页。前台仅展示热度等级，不公开这里的精确人数。</p>
  </section>
</template>
