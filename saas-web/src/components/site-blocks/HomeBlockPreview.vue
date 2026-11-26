<script setup lang="ts">
import type { SitePageBlock } from '../../api/sitePages'
import type { SupportedLocale } from '../../utils/localePath'
import ProductImage from '../ProductImage.vue'
import GoogleMapSection from './GoogleMapSection.vue'
import { blockLabel } from './blockLabels'
import type { HomeFloorDiagnostic } from './homeFloorDiagnostics'

withDefaults(defineProps<{
  block: SitePageBlock
  floorDiagnostic?: HomeFloorDiagnostic
  locale?: SupportedLocale
}>(), {
  locale: 'en',
})

function heroStyle(block: SitePageBlock) {
  const imageURL = block.backgroundImage || block.image_url
  if (!imageURL) {
    return {}
  }
  return {
    backgroundImage: `linear-gradient(90deg, rgba(15, 23, 42, 0.88), rgba(15, 23, 42, 0.36)), url(${imageURL})`,
    backgroundSize: 'cover',
    backgroundPosition: 'center',
  }
}

function carouselSlides(block: SitePageBlock) {
  return (block.slides || []).filter((slide) => slide.enabled !== false)
}
</script>

<template>
  <section class="overflow-hidden rounded-lg border border-slate-200 bg-white">
    <div v-if="block.type === 'hero'" class="bg-gradient-to-br from-slate-950 via-slate-900 to-emerald-800 p-5 text-white" :style="heroStyle(block)">
      <p class="text-xs font-semibold uppercase tracking-[0.22em] text-orange-300">Platform Home</p>
      <h3 class="mt-2 text-2xl font-black">{{ block.title || '首屏标题' }}</h3>
      <p class="mt-2 text-sm leading-6 text-slate-100">{{ block.subtitle }}</p>
      <span v-if="block.buttonText" class="mt-4 inline-flex rounded-md bg-orange-500 px-3 py-2 text-xs font-bold text-white">
        {{ block.buttonText }}
      </span>
    </div>

    <div v-else-if="block.type === 'hero_banner'" class="grid gap-3 bg-slate-950 p-4 text-white sm:grid-cols-[1fr_150px]" :style="heroStyle(block)">
      <div>
        <p class="text-xs font-semibold uppercase tracking-[0.22em] text-orange-300">Mall Hero</p>
        <h3 class="mt-2 text-2xl font-black">{{ block.title || '商城大 Banner' }}</h3>
        <p class="mt-2 text-sm leading-6 text-slate-100">{{ block.subtitle }}</p>
        <span v-if="block.button_text" class="mt-4 inline-flex rounded-md bg-orange-500 px-3 py-2 text-xs font-bold text-white">
          {{ block.button_text }}
        </span>
      </div>
      <div class="aspect-video overflow-hidden rounded-lg bg-slate-800">
        <img v-if="block.image_url" :src="block.image_url" :alt="block.title || ''" class="h-full w-full object-cover" />
      </div>
    </div>

    <div v-else-if="block.type === 'hero_carousel'" class="p-4">
      <div class="aspect-[16/6] overflow-hidden rounded-xl bg-slate-100">
        <ProductImage
          :src="carouselSlides(block)[0]?.image"
          :alt="carouselSlides(block)[0]?.title || '轮播图'"
          class="h-full w-full"
          :image-class="`h-full w-full ${block.fit === 'contain' ? 'object-contain' : 'object-cover'}`"
        />
      </div>
      <div class="mt-3 flex items-center justify-between">
        <div class="flex items-center gap-1.5">
          <span
            v-for="(_, index) in carouselSlides(block)"
            :key="index"
            class="h-2 w-2 rounded-full"
            :class="index === 0 ? 'bg-slate-900' : 'bg-slate-300'"
          />
          <span v-if="!carouselSlides(block).length" class="h-2 w-2 rounded-full bg-slate-300" />
        </div>
        <span class="text-xs text-slate-500">{{ carouselSlides(block).length }} 张</span>
      </div>
    </div>

    <div v-else-if="block.type === 'category_nav'" class="p-4">
      <h3 class="text-base font-black text-slate-950">{{ block.title || '分类导航' }}</h3>
      <div class="mt-3 grid grid-cols-3 gap-2">
        <div v-for="(item, index) in block.items || []" :key="index" class="rounded-lg bg-slate-50 p-2 text-center">
          <div class="mx-auto flex aspect-square w-full items-center justify-center overflow-hidden rounded-md bg-white text-xs font-black text-slate-400">
            <img v-if="item.image" :src="item.image" :alt="item.name || ''" class="h-full w-full object-cover" />
            <span v-else>{{ item.name?.slice(0, 1) || '类' }}</span>
          </div>
          <p class="mt-2 truncate text-xs font-bold text-slate-700">{{ item.name || '分类' }}</p>
        </div>
      </div>
    </div>

    <GoogleMapSection v-else-if="block.type === 'map_section'" :block="block" :locale="locale" preview />

    <div v-else-if="block.type === 'image_text'" class="grid gap-3 p-4 sm:grid-cols-2">
      <div class="aspect-[4/3] overflow-hidden rounded-lg bg-slate-100" :class="block.layout === 'image_right' ? 'sm:order-2' : ''">
        <img v-if="block.image" :src="block.image" :alt="block.title || ''" class="h-full w-full object-cover" />
      </div>
      <div>
        <h3 class="text-base font-black text-slate-950">{{ block.title || '图文介绍' }}</h3>
        <p class="mt-2 line-clamp-4 whitespace-pre-line text-xs leading-5 text-slate-600">{{ block.content }}</p>
        <span v-if="block.buttonText" class="mt-3 inline-flex rounded-md bg-slate-950 px-3 py-2 text-xs font-bold text-white">
          {{ block.buttonText }}
        </span>
      </div>
    </div>

    <div v-else-if="block.type === 'feature_grid'" class="p-4">
      <h3 class="text-base font-black text-slate-950">{{ block.title || '服务优势' }}</h3>
      <p class="mt-1 text-xs text-slate-500">{{ block.subtitle }}</p>
      <div class="mt-3 grid gap-2">
        <div v-for="(item, index) in block.items || []" :key="index" class="flex gap-3 rounded-lg bg-slate-50 p-3">
          <span class="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-emerald-600 text-xs font-black text-white">
            {{ item.icon || index + 1 }}
          </span>
          <div>
            <p class="text-xs font-black text-slate-900">{{ item.title || '优势标题' }}</p>
            <p class="mt-1 line-clamp-2 text-xs leading-5 text-slate-500">{{ item.description }}</p>
          </div>
        </div>
      </div>
    </div>

    <div v-else-if="block.type === 'product_floor'" class="p-4">
      <div class="flex items-start justify-between gap-3">
        <div>
          <h3 class="text-base font-black text-slate-950">{{ block.title || '商品楼层' }}</h3>
          <p class="mt-1 text-xs text-slate-500">{{ block.subtitle || block.platform_category_path || '按平台类目展示商品' }}</p>
        </div>
        <span class="rounded bg-orange-50 px-2 py-1 text-xs font-bold text-orange-700">{{ block.enabled === false ? '停用' : '启用' }}</span>
      </div>
      <p v-if="floorDiagnostic === 'missing'" class="mt-3 rounded-md bg-red-50 px-3 py-2 text-xs font-bold text-red-700">
        绑定的平台类目已不存在，请重新选择
      </p>
      <p v-else-if="floorDiagnostic === 'empty'" class="mt-3 rounded-md bg-amber-50 px-3 py-2 text-xs font-bold text-amber-700">
        当前类目暂无可展示商品
      </p>
      <div v-if="floorDiagnostic !== 'missing' && floorDiagnostic !== 'empty'" class="mt-3 grid grid-cols-2 gap-2">
        <div v-for="index in 4" :key="index" class="rounded-lg border border-slate-100 bg-slate-50 p-2">
          <div class="aspect-square rounded-md bg-white"></div>
          <div class="mt-2 h-2 rounded bg-slate-200"></div>
          <div class="mt-2 h-2 w-2/3 rounded bg-orange-200"></div>
        </div>
      </div>
      <p class="mt-2 text-xs text-slate-500">数量 {{ block.limit || 10 }}，{{ block.columns || 5 }} 列</p>
    </div>

    <div v-else-if="block.type === 'product_grid' || block.type === 'shop_grid'" class="p-4">
      <h3 class="text-base font-black text-slate-950">{{ block.title || blockLabel(block.type) }}</h3>
      <div class="mt-3 grid grid-cols-2 gap-2">
        <div v-for="index in 4" :key="index" class="rounded-lg border border-slate-100 bg-slate-50 p-2">
          <div class="aspect-square rounded-md bg-white"></div>
          <div class="mt-2 h-2 rounded bg-slate-200"></div>
          <div class="mt-2 h-2 w-2/3 rounded bg-orange-200"></div>
        </div>
      </div>
      <p class="mt-2 text-xs text-slate-500">显示数量 {{ block.limit || 8 }}</p>
    </div>

    <div v-else-if="block.type === 'promo_banner'" class="grid gap-3 bg-slate-950 p-4 text-white sm:grid-cols-[1fr_120px]">
      <div>
        <h3 class="text-lg font-black">{{ block.title || '广告横幅' }}</h3>
        <p class="mt-2 text-xs leading-5 text-slate-200">{{ block.subtitle }}</p>
        <span v-if="block.buttonText" class="mt-3 inline-flex rounded-md bg-orange-500 px-3 py-2 text-xs font-bold text-white">
          {{ block.buttonText }}
        </span>
      </div>
      <div class="aspect-video overflow-hidden rounded-lg bg-slate-800">
        <img v-if="block.image_url || block.image" :src="block.image_url || block.image" :alt="block.title || ''" class="h-full w-full object-cover" />
      </div>
    </div>

    <div v-else-if="block.type === 'footer_links'" class="bg-slate-950 p-4 text-white">
      <h3 class="text-base font-black">{{ block.title || '页脚链接' }}</h3>
      <div class="mt-3 grid gap-3 sm:grid-cols-2">
        <div v-for="(group, index) in block.items || []" :key="index">
          <p class="text-xs font-black text-slate-200">{{ group.title || '链接分组' }}</p>
          <p class="mt-2 text-xs text-slate-400">{{ group.links?.map((link) => link.name).join(' / ') || '暂无链接' }}</p>
        </div>
      </div>
    </div>

    <div v-else-if="block.type === 'contact'" class="bg-emerald-950 p-4 text-white">
      <h3 class="text-lg font-black">{{ block.title || '联系我们' }}</h3>
      <p class="mt-2 whitespace-pre-line text-xs leading-5 text-emerald-50">{{ block.content }}</p>
      <div class="mt-3 grid gap-1 text-xs text-emerald-100">
        <span v-if="block.phone">电话：{{ block.phone }}</span>
        <span v-if="block.whatsapp">WhatsApp：{{ block.whatsapp }}</span>
        <span v-if="block.address">地址：{{ block.address }}</span>
      </div>
    </div>

    <div v-else class="p-4">
      <h3 class="text-base font-black text-slate-950">{{ block.title || blockLabel(block.type) }}</h3>
      <p class="mt-2 whitespace-pre-line text-xs leading-5 text-slate-600">{{ block.content }}</p>
    </div>
  </section>
</template>
