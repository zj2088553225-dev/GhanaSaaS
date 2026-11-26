<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import ExcelJS from 'exceljs'
import {
  getAdminOrders,
  markAdminOrderPaid,
  shipAdminOrder,
  updateAdminOrderStatus,
  type Order,
  type PaymentStatus,
  type OrderStatus,
} from '../../../api/orders'
import { useAuthStore } from '../../../stores/auth'
import { formatMoney } from '../../../utils/money'

const authStore = useAuthStore()
const loading = ref(false)
const orders = ref<Order[]>([])
const total = ref(0)
const printOrder = ref<Order | null>(null)
const printPreviewVisible = ref(false)
const shipDialogVisible = ref(false)
const shipSubmitting = ref(false)
const shipTarget = ref<Order | null>(null)
const shipForm = reactive({
  tracking_number: '',
})
const query = reactive<{
  order_no: string
  phone: string
  status: '' | OrderStatus
  dateRange: [string, string] | []
  page: number
  page_size: number
}>({
  order_no: '',
  phone: '',
  status: '',
  dateRange: [],
  page: 1,
  page_size: 20,
})

const statusOptions = [
  { label: '全部状态', value: '' },
  { label: '待发货', value: 'pending' },
  { label: '配货中', value: 'processing' },
  { label: '已发货', value: 'shipped' },
  { label: '已完成', value: 'completed' },
]
const exportHeaders = [
  '订单号',
  '订单状态',
  '下单时间',
  '客户名称',
  '客户手机',
  '收货地址',
  '总金额',
  '商品名称',
  'SKU名称',
  'SKU条码',
  '库位',
  '单价',
  '配货数量',
  '行金额',
]

const tableData = computed(() => orders.value)
const canShipOrders = computed(() => {
  const role = authStore.user?.role
  return role === 'tenant_admin' || role === 'picker'
})
const canMarkPaid = computed(() => {
  const role = authStore.user?.role
  return role === 'tenant_admin' || role === 'finance'
})

onMounted(() => {
  window.addEventListener('afterprint', onAfterPrint)
  void loadOrders()
})

onBeforeUnmount(() => {
  window.removeEventListener('afterprint', onAfterPrint)
})

async function loadOrders() {
  loading.value = true
  try {
    const page = await getAdminOrders(buildQuery())
    orders.value = page.items
    total.value = page.total
    query.page = page.page
    query.page_size = page.page_size
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '订单列表加载失败')
  } finally {
    loading.value = false
  }
}

function buildQuery(exportMode = false) {
  const [startDate, endDate] = query.dateRange
  return {
    order_no: query.order_no || undefined,
    phone: query.phone || undefined,
    status: query.status || undefined,
    start_date: startDate || undefined,
    end_date: endDate || undefined,
    page: query.page,
    page_size: query.page_size,
    export: exportMode || undefined,
    no_page: exportMode || undefined,
  }
}

function statusLabel(status: OrderStatus) {
  switch (status) {
    case 'pending':
      return '待发货'
    case 'processing':
      return '配货中'
    case 'shipped':
      return '已发货'
    case 'completed':
      return '已完成'
  }
}

function statusTagType(status: OrderStatus) {
  switch (status) {
    case 'pending':
      return 'warning'
    case 'processing':
      return 'primary'
    case 'shipped':
      return 'info'
    case 'completed':
      return 'success'
  }
}

function paymentStatusLabel(status: PaymentStatus) {
  switch (status) {
    case 'unpaid':
      return '未支付'
    case 'paid':
      return '已支付'
    case 'refunded':
      return '已退款'
  }
}

function paymentStatusTagType(status: PaymentStatus) {
  switch (status) {
    case 'unpaid':
      return 'danger'
    case 'paid':
      return 'success'
    case 'refunded':
      return 'info'
  }
}

function fulfillmentMethodLabel(order: Order) {
  return order.fulfillment_method === 'delivery' ? '同城配送' : '仓库自提'
}

function formatCurrency(amount: number) {
  return formatMoney(amount)
}

function formatDateTime(value: string) {
  return new Intl.DateTimeFormat('zh-CN', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value))
}

function itemSummary(order: Order) {
  return `${order.items.length} 件 / ${order.items.reduce((sum, item) => sum + item.quantity, 0)} 件商品`
}

async function searchOrders() {
  query.page = 1
  await loadOrders()
}

async function resetFilters() {
  query.order_no = ''
  query.phone = ''
  query.status = ''
  query.dateRange = []
  query.page = 1
  await loadOrders()
}

async function changePage(page: number) {
  query.page = page
  await loadOrders()
}

async function changePageSize(pageSize: number) {
  query.page_size = pageSize
  query.page = 1
  await loadOrders()
}

function openShipDialog(order: Order) {
  shipTarget.value = order
  shipForm.tracking_number = order.tracking_number || ''
  shipDialogVisible.value = true
}

function resetShipDialog() {
  shipTarget.value = null
  shipForm.tracking_number = ''
}

async function submitShip() {
  if (!shipTarget.value) {
    return
  }
  const trackingNumber = shipForm.tracking_number.trim()
  if (shipTarget.value.fulfillment_method === 'delivery' && !trackingNumber) {
    ElMessage.warning('配送订单必须填写物流单号')
    return
  }

  shipSubmitting.value = true
  try {
    await shipAdminOrder(shipTarget.value.id, trackingNumber ? { tracking_number: trackingNumber } : {})
    ElMessage.success('订单已发货')
    shipDialogVisible.value = false
    await loadOrders()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '发货失败')
  } finally {
    shipSubmitting.value = false
  }
}

async function completeOrder(order: Order) {
  try {
    await ElMessageBox.confirm(
      `确认将订单 ${order.order_no} 标记为已完成吗？`,
      '状态确认',
      {
        confirmButtonText: '确认',
        cancelButtonText: '取消',
        type: 'warning',
      },
    )
    await updateAdminOrderStatus(order.id, 'completed')
    ElMessage.success('订单状态已更新')
    await loadOrders()
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      ElMessage.error(error instanceof Error ? error.message : '状态更新失败')
    }
  }
}

async function markPaid(order: Order) {
  try {
    await ElMessageBox.confirm(`确认订单 ${order.order_no} 已收款吗？`, '收款确认', {
      confirmButtonText: '标记已收款',
      cancelButtonText: '取消',
      type: 'warning',
    })
    await markAdminOrderPaid(order.id)
    ElMessage.success('支付状态已更新')
    await loadOrders()
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      ElMessage.error(error instanceof Error ? error.message : '支付状态更新失败')
    }
  }
}

async function preparePrint(order: Order) {
  printOrder.value = order
  printPreviewVisible.value = true
  await nextTick()
  window.print()
  onAfterPrint()
}

async function exportOrders() {
  loading.value = true
  try {
    const page = await getAdminOrders(buildQuery(true))
    const rows = page.items.flatMap((order) =>
      order.items.map((item) => ({
        订单号: order.order_no,
        订单状态: statusLabel(order.status),
        下单时间: formatDateTime(order.created_at),
        客户名称: order.contact_name,
        客户手机: order.contact_phone,
        收货地址: order.address,
        总金额: formatCurrency(order.total_amount),
        商品名称: item.product_name,
        SKU名称: item.sku?.name || '-',
        SKU条码: item.sku?.barcode || item.sku?.erp_sku_code || '-',
        库位: item.sku?.location || '-',
        单价: formatCurrency(item.price),
        配货数量: item.quantity,
        行金额: formatCurrency(item.price * item.quantity),
      })),
    )

    const workbook = new ExcelJS.Workbook()
    const worksheet = workbook.addWorksheet('Orders')
    worksheet.columns = exportHeaders.map((key) => ({ header: key, key, width: 18 }))
    worksheet.addRows(rows)

    const buffer = await workbook.xlsx.writeBuffer()
    const blob = new Blob([buffer], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `orders-${new Date().toISOString().slice(0, 10)}.xlsx`
    link.click()
    URL.revokeObjectURL(url)
    ElMessage.success('Excel 已导出')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '导出失败')
  } finally {
    loading.value = false
  }
}

function onAfterPrint() {
  printPreviewVisible.value = false
  printOrder.value = null
}

</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-col gap-3 rounded-2xl bg-white p-4 shadow-sm print:hidden md:flex-row md:items-center md:justify-between">
      <div>
        <h2 class="text-xl font-black text-slate-950">订单管理</h2>
        <p class="mt-1 text-sm text-slate-500">管理订单状态、打印配货单并导出 Excel</p>
      </div>

      <div class="flex flex-wrap gap-2">
        <el-button @click="loadOrders">刷新</el-button>
        <el-button type="success" @click="exportOrders">导出 Excel</el-button>
      </div>
    </div>

    <el-card shadow="never" class="print:hidden">
      <div class="mb-4 grid gap-3 md:grid-cols-4">
        <el-input v-model="query.order_no" clearable placeholder="搜索订单号" @keyup.enter="searchOrders" />
        <el-input v-model="query.phone" clearable placeholder="搜索手机号" @keyup.enter="searchOrders" />
        <el-select v-model="query.status" placeholder="状态" @change="searchOrders">
          <el-option v-for="option in statusOptions" :key="option.value" :label="option.label" :value="option.value" />
        </el-select>
        <el-date-picker
          v-model="query.dateRange"
          type="daterange"
          range-separator="至"
          start-placeholder="开始日期"
          end-placeholder="结束日期"
          value-format="YYYY-MM-DD"
        />
      </div>
      <div class="mb-4 flex flex-wrap gap-2">
        <el-button type="primary" @click="searchOrders">搜索</el-button>
        <el-button @click="resetFilters">重置</el-button>
      </div>

      <el-table v-loading="loading" :data="tableData" border>
        <el-table-column prop="order_no" label="订单号" min-width="200" />
        <el-table-column label="客户" min-width="180">
          <template #default="{ row }: { row: Order }">
            <div class="space-y-1">
              <p class="font-semibold text-slate-900">{{ row.contact_name }}</p>
              <p class="text-xs text-slate-500">{{ row.contact_phone }}</p>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="总金额" width="130">
          <template #default="{ row }: { row: Order }">{{ formatCurrency(row.total_amount) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="120">
          <template #default="{ row }: { row: Order }">
            <el-tag :type="statusTagType(row.status)">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="支付状态" width="120">
          <template #default="{ row }: { row: Order }">
            <el-tag :type="paymentStatusTagType(row.payment_status)">{{ paymentStatusLabel(row.payment_status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="履约方式" min-width="150">
          <template #default="{ row }: { row: Order }">
            <div class="space-y-1">
              <p class="font-semibold text-slate-900">{{ fulfillmentMethodLabel(row) }}</p>
              <p v-if="row.tracking_number" class="text-xs font-bold text-orange-600">{{ row.tracking_number }}</p>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="商品明细" min-width="180">
          <template #default="{ row }: { row: Order }">{{ itemSummary(row) }}</template>
        </el-table-column>
        <el-table-column label="下单时间" width="180">
          <template #default="{ row }: { row: Order }">{{ formatDateTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="420" fixed="right">
          <template #default="{ row }: { row: Order }">
            <div class="flex flex-wrap gap-2 print:hidden">
              <el-button v-if="canMarkPaid && row.payment_status === 'unpaid'" size="small" type="success" @click="markPaid(row)">标记已收款</el-button>
              <el-button
                v-if="canShipOrders && (row.status === 'pending' || row.status === 'processing')"
                size="small"
                type="warning"
                @click="openShipDialog(row)"
              >
                发货/录入单号
              </el-button>
              <el-button v-if="canShipOrders && row.status === 'shipped'" size="small" type="primary" @click="completeOrder(row)">完成</el-button>
              <el-button size="small" @click="preparePrint(row)">打印配货单</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <div class="mt-4 flex justify-end print:hidden">
        <el-pagination
          background
          layout="total, sizes, prev, pager, next"
          :current-page="query.page"
          :page-size="query.page_size"
          :page-sizes="[10, 20, 50]"
          :total="total"
          @current-change="changePage"
          @size-change="changePageSize"
        />
      </div>
    </el-card>

    <el-dialog v-model="shipDialogVisible" title="发货/录入单号" width="480px" class="print:hidden" @closed="resetShipDialog">
      <div v-if="shipTarget" class="space-y-4">
        <div class="rounded-xl bg-slate-50 px-4 py-3 text-sm">
          <p class="font-semibold text-slate-900">{{ shipTarget.order_no }}</p>
          <p class="mt-1 text-slate-500">{{ fulfillmentMethodLabel(shipTarget) }}</p>
        </div>

        <el-form label-position="top" @submit.prevent="submitShip">
          <el-form-item v-if="shipTarget.fulfillment_method === 'delivery'" label="物流单号" required>
            <el-input v-model="shipForm.tracking_number" maxlength="128" show-word-limit placeholder="请输入物流单号" @keyup.enter="submitShip" />
          </el-form-item>
          <p v-else class="text-sm font-semibold text-slate-600">自提订单确认备货后即可标记为已发货。</p>
        </el-form>
      </div>

      <template #footer>
        <el-button :disabled="shipSubmitting" @click="shipDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="shipSubmitting" @click="submitShip">提交发货</el-button>
      </template>
    </el-dialog>

    <div v-if="printPreviewVisible && printOrder" class="print:block hidden bg-white p-8 text-black">
      <div class="mb-6 text-center">
        <h1 class="text-2xl font-bold">Warehouse Picking List / 仓库配货单</h1>
      </div>

      <div class="mb-6 grid grid-cols-2 gap-4 text-sm">
        <div><span class="font-semibold">订单号：</span><span class="text-2xl font-bold">{{ printOrder.order_no }}</span></div>
        <div><span class="font-semibold">下单日期：</span>{{ formatDateTime(printOrder.created_at) }}</div>
        <div><span class="font-semibold">联系人：</span>{{ printOrder.contact_name }}</div>
        <div><span class="font-semibold">联系电话：</span>{{ printOrder.contact_phone }}</div>
        <div class="col-span-2"><span class="font-semibold">收货地址：</span>{{ printOrder.address }}</div>
      </div>

      <table class="w-full border-collapse border border-black text-sm">
        <thead>
          <tr>
            <th class="border border-black p-2">序号</th>
            <th class="border border-black p-2">商品名称</th>
            <th class="border border-black p-2">SKU 规格</th>
            <th class="border border-black p-2">SKU 条码</th>
            <th class="border border-black p-2 text-lg font-bold">配货数量</th>
            <th class="border border-black p-2">库位</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(item, index) in printOrder.items" :key="item.id">
            <td class="border border-black p-2 text-center">{{ index + 1 }}</td>
            <td class="border border-black p-2">{{ item.product_name }}</td>
            <td class="border border-black p-2">{{ item.sku?.name || '-' }}</td>
            <td class="border border-black p-2">{{ item.sku?.barcode || item.sku?.erp_sku_code || '-' }}</td>
            <td class="border border-black p-2 text-center text-lg font-bold">{{ item.quantity }}</td>
            <td class="border border-black p-2">{{ item.sku?.location || '-' }}</td>
          </tr>
        </tbody>
      </table>

      <div class="mt-8 grid grid-cols-2 gap-6 text-sm">
        <div>检货人签名：________________</div>
        <div>日期：________________</div>
        <div class="col-span-2">备注：____________________________________________</div>
      </div>
    </div>
  </section>
</template>

<style scoped>
@media print {
  :global(body) {
    background: white;
  }

  .print\:hidden {
    display: none !important;
  }

  .print\:block {
    display: block !important;
  }
}
</style>
