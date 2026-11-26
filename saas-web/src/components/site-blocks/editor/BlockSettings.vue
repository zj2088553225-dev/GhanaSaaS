<script setup lang="ts">
import { computed, ref } from 'vue'
import { Bottom, Plus, Top, Upload } from '@element-plus/icons-vue'
import type { UploadProps } from 'element-plus'
import { ElMessage } from 'element-plus'
import type { SitePageBlock } from '../../../api/sitePages'
import { useAuthStore } from '../../../stores/auth'
import AdminPlatformCategoryRemotePicker from '../../admin/AdminPlatformCategoryRemotePicker.vue'
import GoogleMapSection from '../GoogleMapSection.vue'
import { editableBlockTypeOptions } from '../blockLabels'
import { createTrackingKey } from '../blockDefaults'
import type { HomeFloorDiagnostic } from '../homeFloorDiagnostics'

const props = defineProps<{
  block: SitePageBlock
  floorDiagnostic?: HomeFloorDiagnostic
  structureReadonly?: boolean
}>()

const emit = defineEmits<{
  'change-type': [type: SitePageBlock['type']]
}>()

const mapPreviewLocale = ref<'en' | 'zh'>(props.structureReadonly ? 'zh' : 'en')

function onTypeChange(type: SitePageBlock['type']) {
  emit('change-type', type)
}

const authStore = useAuthStore()
const uploadAction = '/api/v1/admin/uploads/image'
const uploadHeaders = computed(() => (authStore.token ? { Authorization: `Bearer ${authStore.token}` } : {}))

type BlockImageField = 'image' | 'image_url' | 'backgroundImage'

function setBlockImageURL(block: SitePageBlock, field: BlockImageField, url: string) {
  ;(block as unknown as Record<BlockImageField, string>)[field] = url
}

const beforeImageUpload: UploadProps['beforeUpload'] = (file) => {
  if (!file.type.startsWith('image/')) {
    ElMessage.error('请上传图片文件')
    return false
  }
  return true
}

function handleBlockImageUpload(block: SitePageBlock, field: BlockImageField, response: unknown) {
  const url = (response as { data?: { url?: string } })?.data?.url
  if (!url) {
    ElMessage.error('图片上传失败')
    return
  }
  setBlockImageURL(block, field, url)
  ElMessage.success('图片已上传')
}

function imageUploadSuccessHandler(block: SitePageBlock, field: BlockImageField): UploadProps['onSuccess'] {
  return (response) => handleBlockImageUpload(block, field, response)
}

function handleUploadError() {
  ElMessage.error('图片上传失败')
}

function ensureItems(block: SitePageBlock) {
  if (!block.items) {
    block.items = []
  }
  return block.items
}

function addCategoryItem(block: SitePageBlock) {
  ensureItems(block).push({ tracking_key: createTrackingKey(), name: '分类名称', image: '', link: '/shops' })
}

function addFeatureItem(block: SitePageBlock) {
  ensureItems(block).push({ tracking_key: createTrackingKey(), title: '优势标题', description: '请输入优势描述', icon: '优' })
}

function addFooterGroup(block: SitePageBlock) {
  ensureItems(block).push({
    tracking_key: createTrackingKey(),
    title: '链接分组',
    links: [{ tracking_key: createTrackingKey(), name: '链接名称', link: '/' }],
  })
}

function addFooterLink(group: NonNullable<SitePageBlock['items']>[number]) {
  if (!group.links) {
    group.links = []
  }
  group.links.push({ tracking_key: createTrackingKey(), name: '链接名称', link: '/' })
}

function removeFooterLink(group: NonNullable<SitePageBlock['items']>[number], index: number) {
  group.links?.splice(index, 1)
}

function removeItem(block: SitePageBlock, index: number) {
  ensureItems(block).splice(index, 1)
}

function ensureSlides(block: SitePageBlock) {
  if (!block.slides) {
    block.slides = []
  }
  return block.slides
}

function addSlide(block: SitePageBlock) {
  ensureSlides(block).push({
    tracking_key: createTrackingKey(),
    image: '',
    link: '/mall',
    enabled: true,
  })
}

function moveSlide(block: SitePageBlock, index: number, direction: -1 | 1) {
  const slides = ensureSlides(block)
  const target = index + direction
  if (target < 0 || target >= slides.length) {
    return
  }
  const next = [...slides]
  const [item] = next.splice(index, 1)
  next.splice(target, 0, item)
  block.slides = next
}

function removeSlide(block: SitePageBlock, index: number) {
  ensureSlides(block).splice(index, 1)
}

function setSlideImageURL(block: SitePageBlock, index: number, url: string) {
  const slides = ensureSlides(block)
  if (slides[index]) {
    slides[index].image = url
  }
}

function handleSlideImageUpload(block: SitePageBlock, index: number, response: unknown) {
  const url = (response as { data?: { url?: string } })?.data?.url
  if (!url) {
    ElMessage.error('图片上传失败')
    return
  }
  setSlideImageURL(block, index, url)
  ElMessage.success('图片已上传')
}

function slideImageUploadSuccessHandler(block: SitePageBlock, index: number): UploadProps['onSuccess'] {
  return (response) => handleSlideImageUpload(block, index, response)
}
</script>

<template>
  <div class="h-full overflow-y-auto rounded-lg border border-slate-200 bg-white shadow-sm">
    <div class="border-b border-slate-100 px-4 py-3">
      <h3 class="text-sm font-black text-slate-950">区块设置</h3>
      <p class="mt-0.5 text-xs text-slate-500">{{ block.type }}</p>
    </div>
    <div class="p-4">
      <el-form label-width="100px">
        <el-form-item label="区块类型">
          <el-select v-model="block.type" @change="onTypeChange">
            <el-option v-for="option in editableBlockTypeOptions" :key="option.value" :label="option.label" :value="option.value" />
          </el-select>
        </el-form-item>

        <el-form-item v-if="block.type !== 'map_section'" label="标题">
          <el-input v-model="block.title" maxlength="160" show-word-limit />
        </el-form-item>

        <template v-if="block.type === 'hero'">
          <el-form-item label="副标题">
            <el-input v-model="block.subtitle" maxlength="240" show-word-limit />
          </el-form-item>
          <el-form-item label="背景图 URL">
            <div class="flex w-full gap-2">
              <el-input v-model="block.backgroundImage" placeholder="/uploads/home.webp" />
              <el-upload :show-file-list="false" :action="uploadAction" :headers="uploadHeaders" :before-upload="beforeImageUpload" :on-success="imageUploadSuccessHandler(block, 'backgroundImage')" :on-error="handleUploadError">
                <el-button><el-icon><Upload /></el-icon>上传</el-button>
              </el-upload>
            </div>
          </el-form-item>
          <el-form-item label="按钮文字">
            <el-input v-model="block.buttonText" maxlength="80" />
          </el-form-item>
          <el-form-item label="按钮链接">
            <el-input v-model="block.buttonLink" placeholder="/shops 或 /products/product-1" />
          </el-form-item>
        </template>

        <template v-else-if="block.type === 'hero_banner'">
          <el-form-item label="副标题">
            <el-input v-model="block.subtitle" maxlength="240" show-word-limit />
          </el-form-item>
          <el-form-item label="图片 URL">
            <div class="flex w-full gap-2">
              <el-input v-model="block.image_url" placeholder="/uploads/home-banner.webp" />
              <el-upload :show-file-list="false" :action="uploadAction" :headers="uploadHeaders" :before-upload="beforeImageUpload" :on-success="imageUploadSuccessHandler(block, 'image_url')" :on-error="handleUploadError">
                <el-button><el-icon><Upload /></el-icon>上传</el-button>
              </el-upload>
            </div>
          </el-form-item>
          <el-form-item label="按钮文字">
            <el-input v-model="block.button_text" maxlength="80" />
          </el-form-item>
          <el-form-item label="跳转链接">
            <el-input v-model="block.link_url" placeholder="/mall 或 /products/product-1" />
          </el-form-item>
        </template>

        <template v-else-if="block.type === 'hero_carousel'">
          <el-form-item label="自动播放">
            <el-switch v-model="block.autoplay" />
          </el-form-item>
          <el-form-item label="轮播间隔 (ms)">
            <el-input-number v-model="block.interval" :min="1000" :max="30000" :step="500" />
          </el-form-item>
          <el-form-item label="高度">
            <el-radio-group v-model="block.height">
              <el-radio-button label="large">大</el-radio-button>
              <el-radio-button label="medium">中</el-radio-button>
              <el-radio-button label="small">小</el-radio-button>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="图片适配方式">
            <el-radio-group v-model="block.fit">
              <el-radio-button label="contain">完整显示</el-radio-button>
              <el-radio-button label="cover">铺满裁切</el-radio-button>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="Slide 列表">
            <div class="w-full space-y-3">
              <div v-for="(slide, slideIndex) in block.slides || []" :key="slide.tracking_key" class="rounded-lg border border-slate-200 bg-slate-50 p-3">
                <div class="mb-3 flex items-center justify-between">
                  <strong class="text-sm text-slate-900">Slide {{ slideIndex + 1 }}</strong>
                  <el-button-group>
                    <el-button :disabled="slideIndex === 0" size="small" @click="moveSlide(block, slideIndex, -1)"><el-icon><Top /></el-icon></el-button>
                    <el-button :disabled="slideIndex === (block.slides || []).length - 1" size="small" @click="moveSlide(block, slideIndex, 1)"><el-icon><Bottom /></el-icon></el-button>
                    <el-button type="danger" plain size="small" @click="removeSlide(block, slideIndex)">删除</el-button>
                  </el-button-group>
                </div>
                <div class="space-y-3">
                  <div class="flex w-full gap-2">
                    <el-input v-model="slide.image" placeholder="图片 URL" />
                    <el-upload :show-file-list="false" :action="uploadAction" :headers="uploadHeaders" :before-upload="beforeImageUpload" :on-success="slideImageUploadSuccessHandler(block, slideIndex)" :on-error="handleUploadError">
                      <el-button><el-icon><Upload /></el-icon>上传</el-button>
                    </el-upload>
                  </div>
                  <el-input v-model="slide.title" maxlength="160" placeholder="标题" />
                  <el-input v-model="slide.subtitle" maxlength="240" placeholder="副标题" />
                  <div class="grid gap-3 md:grid-cols-2">
                    <el-input v-model="slide.buttonText" maxlength="80" placeholder="按钮文字" />
                    <el-input v-model="slide.link" placeholder="跳转链接，例如 /mall" />
                  </div>
                  <el-form-item label="启用" class="mb-0">
                    <el-switch v-model="slide.enabled" />
                  </el-form-item>
                </div>
              </div>
              <el-button @click="addSlide(block)"><el-icon><Plus /></el-icon>添加 Slide</el-button>
            </div>
          </el-form-item>
        </template>

        <template v-else-if="block.type === 'category_nav'">
          <el-form-item label="副标题">
            <el-input v-model="block.subtitle" maxlength="240" show-word-limit />
          </el-form-item>
          <el-form-item label="指定根类目">
            <AdminPlatformCategoryRemotePicker
              v-model="block.platform_category_id"
              v-model:model-path="block.platform_category_path"
              class="w-full"
              :disabled="structureReadonly"
              placeholder="不指定则展示全部一级平台类目；输入中英文名称或完整路径搜索"
            />
          </el-form-item>
          <el-form-item label="分类项">
            <div class="w-full space-y-3">
              <p class="text-xs text-slate-500">未手动添加分类项时，前台会自动展示平台类目树。</p>
              <div v-for="(item, itemIndex) in block.items || []" :key="item.tracking_key" class="rounded-lg border border-slate-200 bg-slate-50 p-3">
                <div class="mb-3 flex items-center justify-between">
                  <strong class="text-sm text-slate-900">分类 {{ itemIndex + 1 }}</strong>
                  <el-button type="danger" plain size="small" @click="removeItem(block, itemIndex)">删除</el-button>
                </div>
                <div class="grid gap-3 md:grid-cols-3">
                  <el-input v-model="item.name" maxlength="80" placeholder="名称" />
                  <el-input v-model="item.image" placeholder="图片 URL" />
                  <el-input v-model="item.link" placeholder="链接" />
                </div>
              </div>
              <el-button @click="addCategoryItem(block)"><el-icon><Plus /></el-icon>添加分类</el-button>
            </div>
          </el-form-item>
        </template>

        <template v-else-if="block.type === 'map_section'">
          <el-alert
            v-if="structureReadonly"
            class="mb-4"
            title="地图的中英文文案与配置请在 English 页签统一编辑"
            type="info"
            :closable="false"
            show-icon
          />
          <el-form-item label="启用">
            <el-switch v-model="block.enabled" :disabled="structureReadonly" />
          </el-form-item>
          <el-form-item label="英文标题">
            <el-input v-model="block.title_en" :disabled="structureReadonly" maxlength="160" show-word-limit />
          </el-form-item>
          <el-form-item label="中文标题">
            <el-input v-model="block.title_zh" :disabled="structureReadonly" maxlength="160" show-word-limit />
          </el-form-item>
          <el-form-item label="英文副标题">
            <el-input v-model="block.subtitle_en" :disabled="structureReadonly" maxlength="240" show-word-limit />
          </el-form-item>
          <el-form-item label="中文副标题">
            <el-input v-model="block.subtitle_zh" :disabled="structureReadonly" maxlength="240" show-word-limit />
          </el-form-item>
          <el-form-item label="地点/地址">
            <el-input
              v-model="block.place_query"
              :disabled="structureReadonly"
              maxlength="512"
              show-word-limit
              placeholder="例如 Accra, Ghana"
            />
          </el-form-item>
          <el-form-item label="Place ID">
            <el-input
              v-model="block.place_id"
              :disabled="structureReadonly"
              maxlength="256"
              placeholder="可选；填写后优先使用 place_id"
            />
          </el-form-item>
          <el-form-item label="桌面高度">
            <el-input-number
              v-model="block.map_height"
              :disabled="structureReadonly"
              :min="200"
              :max="800"
              :step="10"
            />
            <span class="ml-2 text-xs text-slate-500">px（手机固定 260px）</span>
          </el-form-item>
          <el-form-item label="外部地图 URL">
            <el-input
              v-model="block.external_map_url"
              :disabled="structureReadonly"
              maxlength="512"
              placeholder="可选，例如 Google Maps 分享链接"
            />
          </el-form-item>
          <el-form-item label="区块顺序">
            <div class="text-xs leading-5 text-slate-500">
              当前 sort_order：{{ block.sort_order ?? 0 }}；请在中间页面画布拖拽或使用上下按钮调整。
            </div>
          </el-form-item>
          <el-form-item label="地图预览">
            <div class="w-full min-w-0">
              <el-radio-group v-model="mapPreviewLocale" class="mb-3">
                <el-radio-button label="en">English</el-radio-button>
                <el-radio-button label="zh">中文</el-radio-button>
              </el-radio-group>
              <div class="overflow-hidden rounded-lg border border-slate-200">
                <GoogleMapSection :block="block" :locale="mapPreviewLocale" preview />
              </div>
            </div>
          </el-form-item>
        </template>

        <template v-else-if="block.type === 'image_text'">
          <el-form-item label="内容">
            <el-input v-model="block.content" type="textarea" :rows="5" maxlength="4000" show-word-limit />
          </el-form-item>
          <el-form-item label="图片 URL">
            <div class="flex w-full gap-2">
              <el-input v-model="block.image" placeholder="/uploads/about.webp" />
              <el-upload :show-file-list="false" :action="uploadAction" :headers="uploadHeaders" :before-upload="beforeImageUpload" :on-success="imageUploadSuccessHandler(block, 'image')" :on-error="handleUploadError">
                <el-button><el-icon><Upload /></el-icon>上传</el-button>
              </el-upload>
            </div>
          </el-form-item>
          <el-form-item label="布局">
            <el-radio-group v-model="block.layout">
              <el-radio-button label="image_left">图片在左</el-radio-button>
              <el-radio-button label="image_right">图片在右</el-radio-button>
            </el-radio-group>
          </el-form-item>
          <el-form-item label="按钮文字">
            <el-input v-model="block.buttonText" maxlength="80" />
          </el-form-item>
          <el-form-item label="按钮链接">
            <el-input v-model="block.buttonLink" placeholder="/shops 或 /" />
          </el-form-item>
        </template>

        <template v-else-if="block.type === 'feature_grid'">
          <el-form-item label="副标题">
            <el-input v-model="block.subtitle" maxlength="240" show-word-limit />
          </el-form-item>
          <el-form-item label="优势项">
            <div class="w-full space-y-3">
              <div v-for="(item, itemIndex) in block.items || []" :key="item.tracking_key" class="rounded-lg border border-slate-200 bg-slate-50 p-3">
                <div class="mb-3 flex items-center justify-between">
                  <strong class="text-sm text-slate-900">优势 {{ itemIndex + 1 }}</strong>
                  <el-button type="danger" plain size="small" @click="removeItem(block, itemIndex)">删除</el-button>
                </div>
                <div class="grid gap-3 md:grid-cols-[100px_1fr]">
                  <el-input v-model="item.icon" maxlength="16" placeholder="图标文字" />
                  <el-input v-model="item.title" maxlength="160" placeholder="标题" />
                </div>
                <el-input v-model="item.description" class="mt-3" type="textarea" :rows="3" maxlength="400" show-word-limit placeholder="描述" />
              </div>
              <el-button @click="addFeatureItem(block)"><el-icon><Plus /></el-icon>添加优势</el-button>
            </div>
          </el-form-item>
        </template>

        <template v-else-if="block.type === 'product_floor'">
          <el-alert
            v-if="floorDiagnostic === 'missing'"
            class="mb-4"
            title="绑定的平台类目已不存在，请重新选择"
            type="error"
            :closable="false"
            show-icon
          />
          <el-alert
            v-else-if="floorDiagnostic === 'empty'"
            class="mb-4"
            title="当前类目暂无可展示商品"
            type="warning"
            :closable="false"
            show-icon
          />
          <el-alert
            v-else-if="floorDiagnostic === 'unavailable'"
            class="mb-4"
            title="平台类目校验暂时失败，请刷新后重试"
            type="warning"
            :closable="false"
            show-icon
          />
          <el-form-item label="副标题">
            <el-input v-model="block.subtitle" maxlength="240" show-word-limit />
          </el-form-item>
          <el-form-item label="数据源">
            <el-select v-model="block.source_type">
              <el-option label="热门商品" value="popular" />
              <el-option label="最新商品" value="latest" />
              <el-option label="指定类目" value="category" />
              <el-option label="人工选择（暂未实现）" value="manual" disabled />
            </el-select>
          </el-form-item>
          <el-alert
            v-if="block.source_type === 'manual'"
            class="mb-4"
            title="人工商品选择结构尚未实现，请选择热门、最新或类目数据源"
            type="warning"
            :closable="false"
            show-icon
          />
          <el-form-item v-if="block.source_type === 'category'" label="平台类目">
            <AdminPlatformCategoryRemotePicker
              v-model="block.platform_category_id"
              v-model:model-path="block.platform_category_path"
              class="w-full"
              :disabled="structureReadonly"
              placeholder="搜索平台类目；为空则展示全站商品"
            />
          </el-form-item>
          <el-form-item label="商品来源">
            <el-select v-model="block.source">
              <el-option label="全部商品" value="all" />
              <el-option label="T6 Mall" value="t6" />
              <el-option label="商家商品" value="merchant" />
            </el-select>
          </el-form-item>
          <el-form-item v-if="block.source_type === 'category'" label="类目路径">
            <el-input v-model="block.platform_category_path" readonly placeholder="选择类目后自动填充" />
          </el-form-item>
          <el-form-item label="展示数量">
            <el-input-number v-model="block.limit" :min="1" :max="24" />
          </el-form-item>
          <el-form-item label="每行列数">
            <el-radio-group v-model="block.columns">
              <el-radio-button :label="4">4 列</el-radio-button>
              <el-radio-button :label="5">5 列</el-radio-button>
              <el-radio-button :label="6">6 列</el-radio-button>
            </el-radio-group>
          </el-form-item>
          <el-form-item v-if="block.source_type === 'category'" label="类目排序">
            <el-select v-model="block.sort">
              <el-option label="最新发布" value="latest" />
              <el-option label="热销优先" value="hot" />
            </el-select>
          </el-form-item>
          <el-form-item label="启用">
            <el-switch v-model="block.enabled" />
          </el-form-item>
          <el-form-item label="查看更多">
            <el-switch v-model="block.show_more_link" />
          </el-form-item>
        </template>

        <template v-else-if="block.type === 'product_grid'">
          <el-form-item label="副标题">
            <el-input v-model="block.subtitle" maxlength="240" show-word-limit />
          </el-form-item>
          <el-form-item label="商品来源">
            <el-select v-model="block.source">
              <el-option label="最新商品" value="latest" />
              <el-option label="精选商品" value="featured" />
              <el-option label="热销商品" value="hot" />
              <el-option label="指定类目" value="category" />
            </el-select>
          </el-form-item>
          <el-form-item v-if="block.source === 'category'" label="平台类目">
            <AdminPlatformCategoryRemotePicker
              v-model="block.platform_category_id"
              v-model:model-path="block.platform_category_path"
              class="w-full"
              :disabled="structureReadonly"
              placeholder="搜索英文/中文类目名称或完整路径"
            />
          </el-form-item>
          <el-form-item label="数量">
            <el-input-number v-model="block.limit" :min="1" :max="24" />
          </el-form-item>
        </template>

        <template v-else-if="block.type === 'shop_grid'">
          <el-form-item label="副标题">
            <el-input v-model="block.subtitle" maxlength="240" show-word-limit />
          </el-form-item>
          <el-form-item label="数量">
            <el-input-number v-model="block.limit" :min="1" :max="24" />
          </el-form-item>
        </template>

        <template v-else-if="block.type === 'promo_banner'">
          <el-form-item label="副标题">
            <el-input v-model="block.subtitle" maxlength="240" show-word-limit />
          </el-form-item>
          <el-form-item label="图片 URL">
            <div class="flex w-full gap-2">
              <el-input v-model="block.image_url" placeholder="/uploads/promo.webp" />
              <el-upload :show-file-list="false" :action="uploadAction" :headers="uploadHeaders" :before-upload="beforeImageUpload" :on-success="imageUploadSuccessHandler(block, 'image_url')" :on-error="handleUploadError">
                <el-button><el-icon><Upload /></el-icon>上传</el-button>
              </el-upload>
            </div>
          </el-form-item>
          <el-form-item label="按钮文字">
            <el-input v-model="block.buttonText" maxlength="80" />
          </el-form-item>
          <el-form-item label="按钮链接">
            <el-input v-model="block.link_url" placeholder="/mall 或 /products/product-1" />
          </el-form-item>
        </template>

        <template v-else-if="block.type === 'footer_links'">
          <el-form-item label="链接分组">
            <div class="w-full space-y-3">
              <div v-for="(group, groupIndex) in block.items || []" :key="group.tracking_key" class="rounded-lg border border-slate-200 bg-slate-50 p-3">
                <div class="mb-3 flex items-center justify-between">
                  <strong class="text-sm text-slate-900">分组 {{ groupIndex + 1 }}</strong>
                  <el-button type="danger" plain size="small" @click="removeItem(block, groupIndex)">删除分组</el-button>
                </div>
                <el-input v-model="group.title" maxlength="160" placeholder="分组标题" />
                <div class="mt-3 space-y-2">
                  <div v-for="(link, linkIndex) in group.links || []" :key="link.tracking_key" class="grid gap-2 md:grid-cols-[1fr_1fr_auto]">
                    <el-input v-model="link.name" maxlength="160" placeholder="链接名称" />
                    <el-input v-model="link.link" placeholder="/mall" />
                    <el-button type="danger" plain @click="removeFooterLink(group, linkIndex)">删除</el-button>
                  </div>
                </div>
                <el-button class="mt-3" size="small" @click="addFooterLink(group)"><el-icon><Plus /></el-icon>添加链接</el-button>
              </div>
              <el-button @click="addFooterGroup(block)"><el-icon><Plus /></el-icon>添加分组</el-button>
            </div>
          </el-form-item>
        </template>

        <template v-else-if="block.type === 'contact'">
          <el-form-item label="内容">
            <el-input v-model="block.content" type="textarea" :rows="4" maxlength="4000" show-word-limit />
          </el-form-item>
          <el-form-item label="电话">
            <el-input v-model="block.phone" maxlength="80" />
          </el-form-item>
          <el-form-item label="WhatsApp">
            <el-input v-model="block.whatsapp" maxlength="80" />
          </el-form-item>
          <el-form-item label="地址">
            <el-input v-model="block.address" maxlength="240" />
          </el-form-item>
        </template>

        <template v-else-if="block.type === 'text'">
          <el-form-item label="内容">
            <el-input v-model="block.content" type="textarea" :rows="5" maxlength="4000" show-word-limit />
          </el-form-item>
        </template>

        <template v-else-if="block.type === 'image'">
          <el-form-item label="图片 URL">
            <div class="flex w-full gap-2">
              <el-input v-model="block.image" placeholder="/uploads/image.webp" />
              <el-upload :show-file-list="false" :action="uploadAction" :headers="uploadHeaders" :before-upload="beforeImageUpload" :on-success="imageUploadSuccessHandler(block, 'image')" :on-error="handleUploadError">
                <el-button><el-icon><Upload /></el-icon>上传</el-button>
              </el-upload>
            </div>
          </el-form-item>
          <el-form-item label="替代文本">
            <el-input v-model="block.alt" maxlength="240" show-word-limit />
          </el-form-item>
        </template>
      </el-form>
    </div>
  </div>
</template>
