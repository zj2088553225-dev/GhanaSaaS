<script setup lang="ts">
import { Plus } from '@element-plus/icons-vue'
import { editableBlockTypeOptions, blockLabel } from '../blockLabels'
import type { SitePageBlockType } from '../../../api/sitePages'

defineEmits<{
  add: [type: SitePageBlockType]
}>()

function blockIcon(type: SitePageBlockType): string {
  switch (type) {
    case 'hero_carousel':
      return '🖼️'
    case 'hero_banner':
      return '📢'
    case 'category_nav':
      return '📂'
    case 'map_section':
      return '📍'
    case 'product_floor':
      return '📦'
    case 'product_grid':
      return '🛍️'
    case 'promo_banner':
      return '🎁'
    case 'footer_links':
      return '🔗'
    case 'hero':
      return '🏠'
    case 'image_text':
      return '📝'
    case 'feature_grid':
      return '✨'
    case 'shop_grid':
      return '🏪'
    default:
      return '⬜'
  }
}
</script>

<template>
  <div class="flex h-full flex-col rounded-lg border border-slate-200 bg-white shadow-sm">
    <div class="border-b border-slate-100 px-4 py-3">
      <h3 class="text-sm font-black text-slate-950">区块库</h3>
      <p class="mt-0.5 text-xs text-slate-500">点击添加到页面</p>
    </div>
    <div class="flex-1 overflow-y-auto p-3">
      <div class="grid grid-cols-1 gap-2">
        <button
          v-for="option in editableBlockTypeOptions"
          :key="option.value"
          type="button"
          class="flex items-center gap-3 rounded-lg border border-slate-100 bg-slate-50 px-3 py-2.5 text-left transition hover:border-orange-200 hover:bg-orange-50"
          @click="$emit('add', option.value)"
        >
          <span class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-white text-base shadow-sm">
            {{ blockIcon(option.value) }}
          </span>
          <span class="min-w-0">
            <span class="block text-sm font-black text-slate-800">{{ option.label }}</span>
            <span class="block truncate text-xs text-slate-500">{{ blockLabel(option.value) }}</span>
          </span>
          <el-icon class="ml-auto shrink-0 text-orange-500"><Plus /></el-icon>
        </button>
      </div>
    </div>
  </div>
</template>
