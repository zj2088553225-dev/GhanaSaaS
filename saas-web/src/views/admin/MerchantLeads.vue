<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  getMerchantLeads,
  updateMerchantLeadStatus,
  type MerchantLead,
  type MerchantLeadStatus,
} from '../../api/merchantLeads'

const loading = ref(false)
const updatingID = ref<number | null>(null)
const leads = ref<MerchantLead[]>([])
const detailLead = ref<MerchantLead | null>(null)
const detailDialogVisible = computed({
  get: () => Boolean(detailLead.value),
  set: (visible: boolean) => {
    if (!visible) {
      detailLead.value = null
    }
  },
})

const statusOptions: Array<{ label: string; value: MerchantLeadStatus; tag: 'info' | 'warning' | 'success' | 'danger' }> = [
  { label: '新线索', value: 'new', tag: 'info' },
  { label: '已联系', value: 'contacted', tag: 'warning' },
  { label: '已通过', value: 'approved', tag: 'success' },
  { label: '已拒绝', value: 'rejected', tag: 'danger' },
]

const detailRows = computed(() => {
  if (!detailLead.value) {
    return []
  }
  return [
    ['店铺 / 公司名称', detailLead.value.shop_name],
    ['联系人姓名', detailLead.value.contact_name],
    ['WhatsApp', detailLead.value.whatsapp],
    ['联系电话', detailLead.value.phone || '-'],
    ['经营品类', detailLead.value.category],
    ['所在城市 / 区域', detailLead.value.city || '-'],
    ['预计商品数量', detailLead.value.product_count || '-'],
    ['是否有线下店', detailLead.value.has_offline_shop ? '是' : '否'],
    ['备注', detailLead.value.message || '-'],
  ]
})

onMounted(() => {
  void loadLeads()
})

async function loadLeads() {
  loading.value = true
  try {
    leads.value = await getMerchantLeads()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '招商线索加载失败')
  } finally {
    loading.value = false
  }
}

function statusMeta(status: MerchantLeadStatus) {
  return statusOptions.find((option) => option.value === status) ?? statusOptions[0]
}

function formatDateTime(value: string) {
  return new Intl.DateTimeFormat('zh-CN', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value))
}

async function changeStatus(row: MerchantLead, status: MerchantLeadStatus) {
  if (row.status === status) {
    return
  }
  updatingID.value = row.id
  try {
    const updated = await updateMerchantLeadStatus(row.id, status)
    row.status = updated.status
    row.updated_at = updated.updated_at
    ElMessage.success('状态已更新')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '状态更新失败')
  } finally {
    updatingID.value = null
  }
}
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-col gap-3 rounded-2xl bg-white p-4 shadow-sm md:flex-row md:items-center md:justify-between">
      <div>
        <h2 class="text-xl font-black text-slate-950">招商线索 / Merchant Leads</h2>
        <p class="mt-1 text-sm text-slate-500">查看公开招商页提交的入驻申请，并跟进线索状态。</p>
      </div>

      <el-button @click="loadLeads">刷新</el-button>
    </div>

    <el-card shadow="never">
      <el-table v-loading="loading" :data="leads" border>
        <el-table-column label="提交时间" min-width="180">
          <template #default="{ row }: { row: MerchantLead }">
            {{ formatDateTime(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column prop="shop_name" label="店铺名" min-width="180" />
        <el-table-column prop="contact_name" label="联系人" min-width="140" />
        <el-table-column prop="whatsapp" label="WhatsApp" min-width="150" />
        <el-table-column prop="category" label="品类" min-width="140" />
        <el-table-column prop="city" label="城市" min-width="120" />
        <el-table-column label="状态" width="120">
          <template #default="{ row }: { row: MerchantLead }">
            <el-tag :type="statusMeta(row.status).tag">{{ statusMeta(row.status).label }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="260" fixed="right">
          <template #default="{ row }: { row: MerchantLead }">
            <div class="flex items-center gap-2">
              <el-select
                :model-value="row.status"
                size="small"
                class="w-28"
                :disabled="updatingID === row.id"
                @change="(value: MerchantLeadStatus) => changeStatus(row, value)"
              >
                <el-option v-for="option in statusOptions" :key="option.value" :label="option.label" :value="option.value" />
              </el-select>
              <el-button size="small" @click="detailLead = row">详情</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="detailDialogVisible" title="招商线索详情" width="620px">
      <div v-if="detailLead" class="space-y-3">
        <div v-for="[label, value] in detailRows" :key="label" class="grid gap-1 border-b border-slate-100 pb-3 sm:grid-cols-[150px_minmax(0,1fr)]">
          <span class="text-sm font-bold text-slate-500">{{ label }}</span>
          <span class="min-w-0 whitespace-pre-wrap break-words text-sm font-semibold text-slate-900">{{ value }}</span>
        </div>
      </div>
    </el-dialog>
  </section>
</template>
