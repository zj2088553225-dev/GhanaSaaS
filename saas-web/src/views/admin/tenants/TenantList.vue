<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import { createSuperTenant, getSuperTenants, type Tenant, type TenantType } from '../../../api/superTenants'
import { useAuthStore } from '../../../stores/auth'

interface TenantForm {
  name: string
  type: TenantType
  admin_phone: string
  admin_password: string
}

const loading = ref(false)
const submitting = ref(false)
const impersonatingTenantID = ref<number | null>(null)
const dialogVisible = ref(false)
const tenants = ref<Tenant[]>([])
const formRef = ref<FormInstance>()
const authStore = useAuthStore()
const form = reactive<TenantForm>({
  name: '',
  type: '3rd_party',
  admin_phone: '',
  admin_password: '',
})

const tenantTypeOptions: Array<{ label: string; value: TenantType }> = [
  { label: '第三方店铺', value: '3rd_party' },
  { label: '平台自营', value: 'self_operated' },
]

const rules: FormRules<TenantForm> = {
  name: [{ required: true, message: '请输入店铺名称', trigger: 'blur' }],
  type: [{ required: true, message: '请选择店铺类型', trigger: 'change' }],
  admin_phone: [{ required: true, message: '请输入管理员手机号', trigger: 'blur' }],
  admin_password: [{ required: true, message: '请输入管理员初始密码', trigger: 'blur' }],
}

onMounted(() => {
  void loadTenants()
})

async function loadTenants() {
  loading.value = true
  try {
    tenants.value = await getSuperTenants()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '租户列表加载失败')
  } finally {
    loading.value = false
  }
}

function tenantTypeLabel(type: TenantType) {
  return tenantTypeOptions.find((option) => option.value === type)?.label ?? type
}

function statusLabel(status: Tenant['status']) {
  return status === 'active' ? '启用' : '禁用'
}

function formatDateTime(value: string) {
  return new Intl.DateTimeFormat('zh-CN', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value))
}

function resetForm() {
  form.name = ''
  form.type = '3rd_party'
  form.admin_phone = ''
  form.admin_password = ''
  formRef.value?.clearValidate()
}

function openCreateDialog() {
  resetForm()
  dialogVisible.value = true
}

async function submitTenant() {
  if (!formRef.value) {
    return
  }

  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) {
    return
  }

  submitting.value = true
  try {
    await createSuperTenant({
      name: form.name,
      type: form.type,
      admin_phone: form.admin_phone,
      admin_password: form.admin_password,
    })
    ElMessage.success('店铺已创建')
    dialogVisible.value = false
    await loadTenants()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '创建店铺失败')
  } finally {
    submitting.value = false
  }
}

async function impersonate(row: Tenant) {
  impersonatingTenantID.value = row.id
  try {
    await authStore.impersonate(row.id, { tenantName: row.name })
    ElMessage.success(`已进入店铺：${row.name}`)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '进入店铺失败')
  } finally {
    impersonatingTenantID.value = null
  }
}
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-col gap-3 rounded-2xl bg-white p-4 shadow-sm md:flex-row md:items-center md:justify-between">
      <div>
        <h2 class="text-xl font-black text-slate-950">租户管理</h2>
        <p class="mt-1 text-sm text-slate-500">新建店铺并生成首个租户管理员账号</p>
      </div>

      <div class="flex flex-wrap gap-2">
        <el-button @click="loadTenants">刷新</el-button>
        <el-button type="primary" @click="openCreateDialog">新建店铺</el-button>
      </div>
    </div>

    <el-card shadow="never">
      <el-table v-loading="loading" :data="tenants" border>
        <el-table-column prop="id" label="ID" width="90" />
        <el-table-column prop="name" label="店铺名称" min-width="180" />
        <el-table-column label="类型" width="140">
          <template #default="{ row }: { row: Tenant }">
            {{ tenantTypeLabel(row.type) }}
          </template>
        </el-table-column>
        <el-table-column label="状态" width="110">
          <template #default="{ row }: { row: Tenant }">
            <el-tag :type="row.status === 'active' ? 'success' : 'info'">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" min-width="180">
          <template #default="{ row }: { row: Tenant }">
            {{ formatDateTime(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="170" fixed="right">
          <template #default="{ row }: { row: Tenant }">
            <el-button type="danger" plain :loading="impersonatingTenantID === row.id" @click="impersonate(row)">进入店铺</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" title="新建店铺" width="480px" @closed="resetForm">
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <el-form-item label="店铺名称" prop="name">
          <el-input v-model="form.name" placeholder="请输入店铺名称" />
        </el-form-item>

        <el-form-item label="店铺类型" prop="type">
          <el-select v-model="form.type" class="w-full" placeholder="请选择店铺类型">
            <el-option v-for="option in tenantTypeOptions" :key="option.value" :label="option.label" :value="option.value" />
          </el-select>
        </el-form-item>

        <el-form-item label="管理员手机号" prop="admin_phone">
          <el-input v-model="form.admin_phone" autocomplete="username" placeholder="请输入管理员手机号" />
        </el-form-item>

        <el-form-item label="管理员初始密码" prop="admin_password">
          <el-input v-model="form.admin_password" type="password" autocomplete="new-password" placeholder="请输入初始密码" show-password />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitTenant">确认创建</el-button>
      </template>
    </el-dialog>
  </section>
</template>
