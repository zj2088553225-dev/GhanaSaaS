<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import { login } from '../api/auth'
import { useAuthStore } from '../stores/auth'
import { adminPath, isAdminFrontendPath } from '../utils/adminPath.ts'

interface LoginForm {
  phone: string
  password: string
}

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const formRef = ref<FormInstance>()
const loading = ref(false)
const form = reactive<LoginForm>({
  phone: '',
  password: '',
})

const rules: FormRules<LoginForm> = {
  phone: [{ required: true, message: '请输入手机号', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

function defaultAdminPath(role?: string) {
  if (role === 'super_admin') {
    return adminPath('/global-dashboard')
  }
  if (role === 'picker') {
    return adminPath('/orders')
  }
  return adminPath('/dashboard')
}

async function submitLogin() {
  if (!formRef.value) {
    return
  }

  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) {
    return
  }

  loading.value = true
  try {
    const result = await login({ phone: form.phone, password: form.password })
    authStore.setAuth(result)
    ElMessage.success('登录成功')
    const redirect = typeof route.query.redirect === 'string' && isAdminFrontendPath(route.query.redirect)
      ? route.query.redirect
      : defaultAdminPath(result.user.role)
    await router.replace(redirect)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '登录失败')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <main class="flex min-h-screen items-center justify-center bg-slate-100 px-4 py-10">
    <el-card class="w-full max-w-md shadow-sm">
      <div class="mb-8 text-center">
        <p class="text-sm font-semibold uppercase tracking-[0.3em] text-orange-500">Admin</p>
        <h1 class="mt-2 text-2xl font-black text-slate-950">后台登录</h1>
        <p class="mt-2 text-sm text-slate-500">请输入管理员账号进入商品后台</p>
      </div>

      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" @submit.prevent="submitLogin">
        <el-form-item label="手机号" prop="phone">
          <el-input v-model="form.phone" placeholder="请输入手机号" autocomplete="username" />
        </el-form-item>

        <el-form-item label="密码" prop="password">
          <el-input v-model="form.password" type="password" placeholder="请输入密码" autocomplete="current-password" show-password />
        </el-form-item>

        <el-button class="mt-3 w-full" type="primary" native-type="submit" :loading="loading">
          登录
        </el-button>
      </el-form>
    </el-card>
  </main>
</template>
