<script setup lang="ts">
import { RouterLink, useRoute } from 'vue-router'
import { currentLocale, withLocalePath } from '../utils/localePath.ts'

const route = useRoute()
</script>

<template>
  <main class="flex min-h-screen items-center justify-center bg-slate-50 px-4">
    <section class="w-full max-w-md rounded-xl bg-white p-8 text-center shadow-sm ring-1 ring-slate-100">
      <p class="text-sm font-black uppercase tracking-[0.22em] text-orange-500">404</p>
      <h1 class="mt-3 text-2xl font-black text-slate-950">页面不存在</h1>
      <p class="mt-3 text-sm leading-6 text-slate-500">请确认地址是否正确。</p>
      <RouterLink :to="withLocalePath(currentLocale(route), '/')" class="mt-6 inline-flex rounded-lg bg-orange-500 px-5 py-3 text-sm font-black text-white transition hover:bg-orange-600">
        返回首页
      </RouterLink>
    </section>
  </main>
</template>
