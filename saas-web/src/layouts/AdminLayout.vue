<script setup lang="ts">
import { computed } from 'vue'
import { RouterView, useRoute, useRouter } from 'vue-router'
import type { RouteLocationRaw } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { reservationEnabled } from '../config/storefront'
import { ADMIN_LOGIN_PATH, adminPath } from '../utils/adminPath.ts'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()

interface AdminMenuItem {
  index: string
  label: string
  route?: RouteLocationRaw
}

const HOME_EDITOR_PATH = adminPath('/site-pages/home')

const menuItems = computed(() => {
  const role = authStore.user?.role
  if (role === 'super_admin') {
    return [
      { index: adminPath('/global-dashboard'), label: '全局看板' },
      { index: adminPath('/analytics'), label: '数据分析' },
      { index: adminPath('/tenants'), label: '租户管理' },
      { index: adminPath('/merchant-leads'), label: '招商线索' },
      { index: adminPath('/platform-categories'), label: '平台类目' },
      { index: HOME_EDITOR_PATH, label: '主页编辑', route: { name: 'admin-site-page-home' } },
      { index: adminPath('/storefront-tracking'), label: '前台营销代码' },
      ...(reservationEnabled ? [
        { index: adminPath('/reservation-products'), label: '预约商品' },
        { index: adminPath('/reservations'), label: '预约记录' },
      ] : []),
    ] satisfies AdminMenuItem[]
  }
  if (role === 'finance') {
    return [
      { index: adminPath('/dashboard'), label: '数据看板' },
      { index: adminPath('/orders'), label: '订单管理' },
    ] satisfies AdminMenuItem[]
  }
  if (role === 'picker') {
    return [{ index: adminPath('/orders'), label: '订单管理' }] satisfies AdminMenuItem[]
  }
  return [
    { index: adminPath('/dashboard'), label: '数据看板' },
    { index: adminPath('/analytics'), label: '数据分析' },
    { index: adminPath('/products'), label: '商品管理' },
    { index: adminPath('/orders'), label: '订单管理' },
    { index: adminPath('/employees'), label: '员工管理' },
    { index: adminPath('/shop-profile'), label: '店铺主页' },
    { index: adminPath('/settings/developer'), label: '开发者设置' },
  ] satisfies AdminMenuItem[]
})

const activeMenu = computed(() => {
  if (route.path.startsWith(adminPath('/analytics'))) {
    return adminPath('/analytics')
  }
  if (route.path.startsWith(adminPath('/settings/developer'))) {
    return adminPath('/settings/developer')
  }
  if (route.path.startsWith(adminPath('/global-dashboard'))) {
    return adminPath('/global-dashboard')
  }
  if (route.path.startsWith(adminPath('/tenants'))) {
    return adminPath('/tenants')
  }
  if (route.path.startsWith(adminPath('/merchant-leads'))) {
    return adminPath('/merchant-leads')
  }
  if (route.path.startsWith(adminPath('/platform-categories'))) {
    return adminPath('/platform-categories')
  }
  if (route.path.startsWith(HOME_EDITOR_PATH)) {
    return HOME_EDITOR_PATH
  }
  if (route.path.startsWith(adminPath('/storefront-tracking'))) {
    return adminPath('/storefront-tracking')
  }
  if (route.path.startsWith(adminPath('/reservation-products'))) {
    return adminPath('/reservation-products')
  }
  if (route.path.startsWith(adminPath('/reservations'))) {
    return adminPath('/reservations')
  }
  if (route.path.startsWith(adminPath('/dashboard'))) {
    return adminPath('/dashboard')
  }
  if (route.path.startsWith(adminPath('/products'))) {
    return adminPath('/products')
  }
  if (route.path.startsWith(adminPath('/shop-categories'))) {
    return adminPath('/shop-categories')
  }
  if (route.path.startsWith(adminPath('/orders'))) {
    return adminPath('/orders')
  }
  if (route.path.startsWith(adminPath('/employees'))) {
    return adminPath('/employees')
  }
  if (route.path.startsWith(adminPath('/shop-profile'))) {
    return adminPath('/shop-profile')
  }
  return route.path
})

async function logout() {
  authStore.logout()
  await router.replace(ADMIN_LOGIN_PATH)
}

async function exitImpersonation() {
  await authStore.exitImpersonation()
}
</script>

<template>
  <div class="min-h-screen bg-slate-100 text-slate-950 print:bg-white">
    <el-container class="min-h-screen print:block">
      <el-aside width="220px" class="border-r border-slate-200 bg-slate-950 print:hidden">
        <div class="px-5 py-5 text-white">
          <p class="text-xs font-semibold uppercase tracking-[0.3em] text-orange-400">SaaS</p>
          <h1 class="mt-2 text-lg font-black">Admin UI</h1>
        </div>

        <el-menu :default-active="activeMenu" router background-color="#020617" text-color="#cbd5e1" active-text-color="#fb923c">
          <el-menu-item v-for="item in menuItems" :key="item.index" :index="item.index" :route="item.route || item.index">{{ item.label }}</el-menu-item>
        </el-menu>
      </el-aside>

      <el-container class="print:block">
        <div
          v-if="authStore.isImpersonating"
          class="flex items-center justify-between border-b border-red-200 bg-red-600 px-6 py-2 text-sm font-bold text-white print:hidden"
        >
          <span>当前正在代运营：{{ authStore.impersonatingTenantName || '租户后台' }}</span>
          <el-button size="small" type="danger" plain @click="exitImpersonation">返回 super_admin</el-button>
        </div>

        <el-header class="flex items-center justify-between border-b border-slate-200 bg-white px-6 print:hidden">
          <div>
            <p class="text-sm font-semibold text-slate-950">管理后台</p>
            <p class="text-xs text-slate-500">商品、SKU、订单与配货管理</p>
          </div>

          <div class="flex items-center gap-4">
            <div class="text-right text-sm">
              <p class="font-semibold text-slate-900">{{ authStore.user?.phone || '管理员' }}</p>
              <p class="text-xs text-slate-500">{{ authStore.user?.role || '-' }}</p>
            </div>
            <el-button type="danger" plain @click="logout">退出登录</el-button>
          </div>
        </el-header>

        <el-main class="p-6 print:p-0">
          <RouterView />
        </el-main>
      </el-container>
    </el-container>
  </div>
</template>
