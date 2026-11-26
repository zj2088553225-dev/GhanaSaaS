<script setup lang="ts">
withDefaults(
  defineProps<{
    title?: string
    description?: string
  }>(),
  {
    title: '暂无行为数据',
    description: '数据将在商城访客产生访问和点击后开始累计',
  },
)
</script>

<template>
  <el-empty :description="title" :image-size="72">
    <p class="max-w-lg text-center text-sm text-slate-500">{{ description }}</p>
    <slot />
  </el-empty>
</template>
