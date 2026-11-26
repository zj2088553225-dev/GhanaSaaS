<script setup lang="ts">
import { computed } from 'vue'
import { Bottom, Delete, Top } from '@element-plus/icons-vue'
import type { SitePageBlock } from '../../../api/sitePages'
import { blockLabel } from '../blockLabels'

const props = defineProps<{
  blocks: SitePageBlock[]
  selected: number | null
}>()

const emit = defineEmits<{
  'update:blocks': [blocks: SitePageBlock[]]
  'update:selected': [index: number | null]
  remove: [index: number]
}>()

const draggables = computed(() => props.blocks.map((_, index) => `block-${index}`))

function select(index: number) {
  emit('update:selected', index)
}

function move(index: number, direction: -1 | 1) {
  const target = index + direction
  if (target < 0 || target >= props.blocks.length) {
    return
  }
  const next = [...props.blocks]
  const [item] = next.splice(index, 1)
  next.splice(target, 0, item)
  emit('update:blocks', next)
  if (props.selected === index) {
    emit('update:selected', target)
  } else if (props.selected === target) {
    emit('update:selected', index)
  }
}

function remove(index: number) {
  emit('remove', index)
}

function blockSummary(block: SitePageBlock): string {
  switch (block.type) {
    case 'hero_carousel':
      return `${(block.slides || []).filter((s) => s.enabled !== false).length} 张轮播图`
    case 'product_floor':
      return `${productFloorDataSourceLabel(block.source_type)} · ${productFloorSourceLabel(block.source)} · ${block.platform_category_path || '全站商品'} · ${block.limit || 10} 个`
    case 'product_grid':
      return `${block.source || 'latest'} · ${block.limit || 8} 个`
    case 'category_nav':
      return block.items?.length ? `${block.items.length} 个分类项` : '自动展示平台类目'
    case 'map_section':
      return `${block.place_query || block.place_id || '未配置地点'} · ${block.map_height || 320}px`
    case 'footer_links':
      return `${block.items?.length || 0} 个链接分组`
    case 'shop_grid':
      return `${block.limit || 8} 个商家`
    default:
      return block.subtitle || block.content || ''
  }
}

function productFloorDataSourceLabel(sourceType?: string): string {
  switch (sourceType) {
    case 'popular':
      return '热门'
    case 'category':
      return '指定类目'
    case 'manual':
      return '人工选择（未实现）'
    default:
      return '最新'
  }
}

function productFloorSourceLabel(source?: string): string {
  switch (source) {
    case 't6':
      return 'T6 Mall'
    case 'merchant':
      return '商家商品'
    default:
      return '全部商品'
  }
}

function onDragStart(event: DragEvent, index: number) {
  if (!event.dataTransfer) {
    return
  }
  event.dataTransfer.effectAllowed = 'move'
  event.dataTransfer.setData('text/plain', String(index))
  ;(event.target as HTMLElement).classList.add('opacity-50')
}

function onDragEnd(event: DragEvent) {
  ;(event.target as HTMLElement).classList.remove('opacity-50')
}

function onDragOver(event: DragEvent) {
  event.preventDefault()
  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = 'move'
  }
}

function onDrop(event: DragEvent, targetIndex: number) {
  event.preventDefault()
  const raw = event.dataTransfer?.getData('text/plain')
  if (raw == null) {
    return
  }
  const fromIndex = Number(raw)
  if (!Number.isInteger(fromIndex) || fromIndex < 0 || fromIndex >= props.blocks.length || fromIndex === targetIndex) {
    return
  }
  const next = [...props.blocks]
  const [item] = next.splice(fromIndex, 1)
  next.splice(targetIndex, 0, item)
  emit('update:blocks', next)

  if (props.selected === fromIndex) {
    emit('update:selected', targetIndex)
  } else if (props.selected != null && fromIndex < props.selected && targetIndex >= props.selected) {
    emit('update:selected', props.selected - 1)
  } else if (props.selected != null && fromIndex > props.selected && targetIndex <= props.selected) {
    emit('update:selected', props.selected + 1)
  }
}
</script>

<template>
  <div class="flex h-full flex-col rounded-lg border border-slate-200 bg-white shadow-sm">
    <div class="border-b border-slate-100 px-4 py-3">
      <h3 class="text-sm font-black text-slate-950">页面画布</h3>
      <p class="mt-0.5 text-xs text-slate-500">拖拽排序，点击编辑</p>
    </div>
    <div class="flex-1 overflow-y-auto p-3">
      <div v-if="!blocks.length" class="rounded-lg bg-slate-50 p-8 text-center">
        <p class="text-sm text-slate-500">暂无区块</p>
        <p class="mt-1 text-xs text-slate-400">从左侧区块库添加</p>
      </div>

      <div
        v-for="(block, index) in blocks"
        :key="draggables[index]"
        draggable="true"
        role="button"
        tabindex="0"
        class="mb-2 cursor-move select-none rounded-lg border px-3 py-2.5 transition"
        :class="selected === index ? 'border-orange-500 bg-orange-50 ring-1 ring-orange-500' : 'border-slate-200 bg-white hover:border-orange-300 hover:bg-orange-50/40'"
        @click="select(index)"
        @dragstart="onDragStart($event, index)"
        @dragend="onDragEnd"
        @dragover="onDragOver"
        @drop="onDrop($event, index)"
      >
        <div class="flex items-start gap-2">
          <span class="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded bg-slate-100 text-[10px] font-black text-slate-600">
            {{ index + 1 }}
          </span>
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <strong class="text-sm font-black text-slate-900">{{ blockLabel(block.type) }}</strong>
              <el-tag v-if="block.enabled === false" type="info" size="small">停用</el-tag>
              <el-tag v-else type="success" size="small" effect="plain">启用</el-tag>
            </div>
            <p class="mt-0.5 truncate text-xs text-slate-500">
              {{ block.type === 'map_section' ? (block.title_zh || block.title_en || '未命名') : (block.title || '未命名') }}<span v-if="blockSummary(block)"> · {{ blockSummary(block) }}</span>
            </p>
          </div>
          <div class="flex shrink-0 items-center gap-1">
            <el-button :disabled="index === 0" size="small" text @click.stop="move(index, -1)">
              <el-icon><Top /></el-icon>
            </el-button>
            <el-button :disabled="index === blocks.length - 1" size="small" text @click.stop="move(index, 1)">
              <el-icon><Bottom /></el-icon>
            </el-button>
            <el-button type="danger" size="small" text @click.stop="remove(index)">
              <el-icon><Delete /></el-icon>
            </el-button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
