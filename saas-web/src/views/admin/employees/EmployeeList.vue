<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import { createEmployee, getEmployees, type Employee, type EmployeeRole } from '../../../api/employees'

interface EmployeeForm {
  phone: string
  password: string
  role: EmployeeRole | ''
}

const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const employees = ref<Employee[]>([])
const formRef = ref<FormInstance>()
const form = reactive<EmployeeForm>({
  phone: '',
  password: '',
  role: 'finance',
})

const roleOptions: Array<{ label: string; value: EmployeeRole }> = [
  { label: '财务', value: 'finance' },
  { label: '配货员', value: 'picker' },
]

const rules: FormRules<EmployeeForm> = {
  phone: [{ required: true, message: '请输入手机号', trigger: 'blur' }],
  password: [{ required: true, message: '请输入初始密码', trigger: 'blur' }],
  role: [{ required: true, message: '请选择角色', trigger: 'change' }],
}

onMounted(() => {
  void loadEmployees()
})

async function loadEmployees() {
  loading.value = true
  try {
    employees.value = await getEmployees()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '员工列表加载失败')
  } finally {
    loading.value = false
  }
}

function roleLabel(role: EmployeeRole) {
  return roleOptions.find((option) => option.value === role)?.label ?? role
}

function roleTagType(role: EmployeeRole) {
  return role === 'finance' ? 'success' : 'primary'
}

function formatDateTime(value: string) {
  return new Intl.DateTimeFormat('zh-CN', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value))
}

function resetForm() {
  form.phone = ''
  form.password = ''
  form.role = 'finance'
  formRef.value?.clearValidate()
}

function openCreateDialog() {
  resetForm()
  dialogVisible.value = true
}

async function submitEmployee() {
  if (!formRef.value) {
    return
  }

  const valid = await formRef.value.validate().catch(() => false)
  if (!valid || !form.role) {
    return
  }

  submitting.value = true
  try {
    await createEmployee({
      phone: form.phone,
      password: form.password,
      role: form.role,
    })
    ElMessage.success('员工已添加')
    dialogVisible.value = false
    await loadEmployees()
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '添加员工失败')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-col gap-3 rounded-2xl bg-white p-4 shadow-sm md:flex-row md:items-center md:justify-between">
      <div>
        <h2 class="text-xl font-black text-slate-950">员工管理</h2>
        <p class="mt-1 text-sm text-slate-500">为当前店面添加财务或配货员子账号</p>
      </div>

      <div class="flex flex-wrap gap-2">
        <el-button @click="loadEmployees">刷新</el-button>
        <el-button type="primary" @click="openCreateDialog">添加员工</el-button>
      </div>
    </div>

    <el-card shadow="never">
      <el-table v-loading="loading" :data="employees" border>
        <el-table-column prop="phone" label="手机号" min-width="160" />
        <el-table-column label="角色" width="140">
          <template #default="{ row }: { row: Employee }">
            <el-tag :type="roleTagType(row.role)">{{ roleLabel(row.role) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" min-width="180">
          <template #default="{ row }: { row: Employee }">
            {{ formatDateTime(row.created_at) }}
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" title="添加员工" width="460px" @closed="resetForm">
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <el-form-item label="手机号" prop="phone">
          <el-input v-model="form.phone" autocomplete="username" placeholder="请输入手机号" />
        </el-form-item>

        <el-form-item label="初始密码" prop="password">
          <el-input v-model="form.password" type="password" autocomplete="new-password" placeholder="请输入初始密码" show-password />
        </el-form-item>

        <el-form-item label="角色" prop="role">
          <el-select v-model="form.role" class="w-full" placeholder="请选择角色">
            <el-option v-for="option in roleOptions" :key="option.value" :label="option.label" :value="option.value" />
          </el-select>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitEmployee">确认添加</el-button>
      </template>
    </el-dialog>
  </section>
</template>
