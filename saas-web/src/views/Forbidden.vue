<script setup lang="ts">
import { RouterLink, useRoute } from 'vue-router'
import { currentLocale, withLocalePath } from '../utils/localePath.ts'

const route = useRoute()
</script>

<template>
  <main class="flex min-h-screen items-center justify-center bg-slate-50 px-4">
    <section class="w-full max-w-md rounded-xl bg-white p-8 text-center shadow-sm ring-1 ring-slate-100">
      <p class="text-sm font-black uppercase tracking-[0.22em] text-orange-500">403</p>
      <h1 class="mt-3 text-2xl font-black text-slate-950">无权访问</h1>
      <p class="mt-3 text-sm leading-6 text-slate-500">当前账号无权访问该后台页面。</p>
      <RouterLink :to="withLocalePath(currentLocale(route), '/')" class="mt-6 inline-flex rounded-lg bg-orange-500 px-5 py-3 text-sm font-black text-white transition hover:bg-orange-600">
        返回首页
      </RouterLink>
    </section>
  </main>
</template>
