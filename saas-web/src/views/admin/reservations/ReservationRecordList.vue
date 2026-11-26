<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  expireSuperReservations,
  getSuperReservations,
  setSuperReservationRisk,
  setSuperReservationStatus,
  type ReservationIdentityType,
  type ReservationRecordAdmin,
  type ReservationRecordStatus,
} from '../../../api/reservations'

const PAGE_SIZE = 20
const items = ref<ReservationRecordAdmin[]>([])
const loading = ref(false)
const total = ref(0)
const page = ref(1)
const identityType = ref<ReservationIdentityType | ''>('')
const status = ref<ReservationRecordStatus | ''>('')
const riskFlagged = ref<'' | 'true' | 'false'>('')

const statusLabels: Record<ReservationRecordStatus, string> = {
  active: 'OTP 有效',
  pending: '待确认',
  confirmed: '已确认',
  cancelled: '已取消',
  rejected: '已拒绝',
  expired: '已过期',
}

onMounted(() => void loadItems())

async function loadItems() {
  loading.value = true
  try {
    const result = await getSuperReservations({
      page: page.value,
      page_size: PAGE_SIZE,
      identity_type: identityType.value || undefined,
      status: status.value || undefined,
      risk_flagged: riskFlagged.value === '' ? undefined : riskFlagged.value === 'true',
    })
    items.value = result.items
    total.value = result.total
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '预约记录加载失败')
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

async function updateStatus(item: ReservationRecordAdmin, next: 'confirmed' | 'rejected' | 'cancelled') {
  try {
    await ElMessageBox.confirm(`确认将预约 ${item.reservation_code} 标记为“${statusLabels[next]}”？`, '更新预约状态', {
      type: next === 'confirmed' ? 'success' : 'warning',
      confirmButtonText: '确认',
      cancelButtonText: '返回',
    })
    await setSuperReservationStatus(item.reservation_code, next)
    ElMessage.success('预约状态已更新')
    await loadItems()
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      ElMessage.error(error instanceof Error ? error.message : '预约状态更新失败')
    }
  }
}

async function toggleRisk(item: ReservationRecordAdmin) {
  try {
    await setSuperReservationRisk(item.reservation_code, !item.risk_flagged)
    ElMessage.success(item.risk_flagged ? '已取消异常标记' : '已标记为异常')
    await loadItems()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '风险标记更新失败')
  }
}

async function expireDue() {
  try {
    const count = await expireSuperReservations()
    ElMessage.success(`已处理 ${count} 条过期预约`)
    await loadItems()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '过期处理失败')
  }
}

function formatTime(value?: string | null) {
  return value ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '—'
}
</script>

<template>
  <section class="space-y-5">
    <header class="flex flex-col gap-4 rounded-2xl border border-slate-200 bg-white p-6 shadow-sm lg:flex-row lg:items-center lg:justify-between">
      <div>
        <p class="text-xs font-bold uppercase tracking-[0.22em] text-orange-500">Reservation Records</p>
        <h1 class="mt-1 text-2xl font-black text-slate-950">预约记录</h1>
        <p class="mt-1 text-sm text-slate-500">仅展示截断后的身份哈希；不保存或展示原始 IP、Cookie Token。</p>
      </div>
      <el-button type="warning" plain @click="expireDue">执行过期处理</el-button>
    </header>

    <div class="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
      <form class="flex flex-col gap-3 md:flex-row" @submit.prevent="applyFilters">
        <el-select v-model="identityType" clearable placeholder="全部身份" class="w-44" @change="applyFilters">
          <el-option label="匿名浏览器" value="anonymous" />
          <el-option label="手机已验证" value="phone_verified" />
        </el-select>
        <el-select v-model="status" clearable placeholder="全部状态" class="w-44" @change="applyFilters">
          <el-option v-for="(label, value) in statusLabels" :key="value" :label="label" :value="value" />
        </el-select>
        <el-select v-model="riskFlagged" clearable placeholder="全部风险" class="w-40" @change="applyFilters">
          <el-option label="已标记异常" value="true" />
          <el-option label="未标记" value="false" />
        </el-select>
        <el-button native-type="submit" type="primary">筛选</el-button>
      </form>
    </div>

    <div class="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm">
      <el-table v-loading="loading" :data="items" empty-text="暂无预约记录">
        <el-table-column label="预约" min-width="220">
          <template #default="scope">
            <p class="font-mono text-xs font-black text-slate-800">{{ scope.row.reservation_code }}</p>
            <p class="mt-1 truncate text-sm font-bold text-slate-700">{{ scope.row.product.title }}</p>
            <p class="text-xs text-slate-400">数量 {{ scope.row.quantity }} · {{ formatTime(scope.row.created_at) }}</p>
          </template>
        </el-table-column>
        <el-table-column label="身份" min-width="190">
          <template #default="scope">
            <el-tag size="small" :type="scope.row.identity_type === 'anonymous' ? 'warning' : 'success'">
              {{ scope.row.identity_type === 'anonymous' ? 'anonymous' : 'phone_verified' }}
            </el-tag>
            <template v-if="scope.row.identity_type === 'anonymous'">
              <p class="mt-2 font-mono text-xs text-slate-500">Browser {{ scope.row.browser_hash_prefix || '—' }}</p>
              <p class="font-mono text-xs text-slate-500">IP {{ scope.row.ip_hash_prefix || '—' }}</p>
            </template>
            <p v-else class="mt-2 text-xs text-slate-500">{{ scope.row.masked_phone }} · {{ scope.row.channel }}</p>
          </template>
        </el-table-column>
        <el-table-column label="频次" width="130">
          <template #default="scope">
            <p class="text-xs text-slate-600">同浏览器：{{ scope.row.browser_reservation_count }}</p>
            <p class="text-xs text-slate-600">同 IP 24h：{{ scope.row.recent_ip_reservation_count }}</p>
          </template>
        </el-table-column>
        <el-table-column label="状态/到期" min-width="160">
          <template #default="scope">
            <el-tag :type="scope.row.status === 'confirmed' || scope.row.status === 'active' ? 'success' : scope.row.status === 'pending' ? 'warning' : 'info'">
              {{ statusLabels[scope.row.status as ReservationRecordStatus] }}
            </el-tag>
            <p class="mt-2 text-xs text-slate-500">{{ formatTime(scope.row.expires_at) }}</p>
          </template>
        </el-table-column>
        <el-table-column label="风险" width="105">
          <template #default="scope">
            <el-tag :type="scope.row.risk_flagged ? 'danger' : 'info'" size="small">{{ scope.row.risk_flagged ? '异常' : '正常' }}</el-tag>
            <el-tag v-if="scope.row.rate_limit_flagged" type="warning" size="small" class="mt-1">近期限流</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" min-width="245" fixed="right">
          <template #default="scope">
            <el-button v-if="scope.row.identity_type === 'anonymous' && scope.row.status === 'pending'" link type="success" @click="updateStatus(scope.row, 'confirmed')">确认</el-button>
            <el-button v-if="scope.row.identity_type === 'anonymous' && (scope.row.status === 'pending' || scope.row.status === 'confirmed')" link type="warning" @click="updateStatus(scope.row, 'rejected')">拒绝</el-button>
            <el-button v-if="scope.row.identity_type === 'anonymous' && (scope.row.status === 'pending' || scope.row.status === 'confirmed')" link type="danger" @click="updateStatus(scope.row, 'cancelled')">取消</el-button>
            <el-button link :type="scope.row.risk_flagged ? 'info' : 'danger'" @click="toggleRisk(scope.row)">{{ scope.row.risk_flagged ? '取消异常' : '标记异常' }}</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div v-if="total > PAGE_SIZE" class="flex items-center justify-end border-t border-slate-100 p-4">
        <el-pagination layout="prev, pager, next, total" :current-page="page" :page-size="PAGE_SIZE" :total="total" @current-change="changePage" />
      </div>
    </div>
  </section>
</template>
