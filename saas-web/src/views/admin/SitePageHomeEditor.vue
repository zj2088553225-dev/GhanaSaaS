<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getHomeSitePage, publishHomeSitePage, saveHomeSitePageDraft, type SitePageAdmin, type SitePageBlock, type SitePageContent } from '../../api/sitePages'
import HomeBlockPreview from '../../components/site-blocks/HomeBlockPreview.vue'
import { cloneBlockWithDefaults, createDefaultBlock, createDefaultHomeBlocks, prepareBlockForSave } from '../../components/site-blocks/blockDefaults'
import BlockLibrary from '../../components/site-blocks/editor/BlockLibrary.vue'
import BlockCanvas from '../../components/site-blocks/editor/BlockCanvas.vue'
import BlockSettings from '../../components/site-blocks/editor/BlockSettings.vue'
import { getAdminPlatformCategoryById } from '../../api/platformCategories'
import { getProducts } from '../../api/storefront'
import { productSourceParam } from '../../utils/mallSource'
import { homeFloorDiagnosticKey, type HomeFloorDiagnostic } from '../../components/site-blocks/homeFloorDiagnostics'

const { t } = useI18n()

const loading = ref(false)
const saving = ref(false)
const publishing = ref(false)
const pageTitle = ref('平台主页')
const status = ref<'draft' | 'published'>('draft')
const publishedAt = ref('')
const activeLanguage = ref<'en' | 'zh'>('en')
const blocks = ref<SitePageBlock[]>([])
const zhBlocks = ref<SitePageBlock[]>([])
const selectedBlockIndex = ref<number | null>(null)
const selectedZhBlockIndex = ref<number | null>(null)
const floorDiagnostics = ref<Record<string, HomeFloorDiagnostic>>({})
let floorValidationSequence = 0

const statusText = computed(() => (status.value === 'published' ? '已发布' : '草稿'))
const selectedBlock = computed(() => (selectedBlockIndex.value != null ? blocks.value[selectedBlockIndex.value] : null))
const selectedZhBlock = computed(() => (selectedZhBlockIndex.value != null ? zhBlocks.value[selectedZhBlockIndex.value] : null))
const floorValidationSignature = computed(() => JSON.stringify(
  blocks.value
    .filter((block) => block.type === 'product_floor')
    .map((block) => ({
      source_type: block.source_type,
      platform_category_id: block.platform_category_id,
      source: block.source || 'all',
    })),
))
const blockStructureSignature = computed(() => blocks.value
  .map((block) => `${block.tracking_key || ''}:${block.type}`)
  .join('|'))

watch(
  blocks,
  (currentBlocks) => {
    currentBlocks.forEach((block, index) => {
      if (block.type === 'map_section' && block.sort_order !== index) {
        block.sort_order = index
      }
    })
  },
  { deep: true, immediate: true },
)

watch(floorValidationSignature, () => {
  void validateProductFloors()
})

watch(blockStructureSignature, () => {
  syncZhBlocksWithEnglish()
}, { flush: 'post' })

const translatableCmsFields = new Set(['title', 'subtitle', 'description', 'content', 'buttonText', 'button_text', 'label', 'text', 'name', 'alt', 'address'])
const nestedCmsLists = new Set(['slides', 'items', 'links'])

function defaultHomePage(): SitePageAdmin {
  return {
    page_key: 'home',
    title: '平台主页',
    draft_content: { blocks: [] },
    status: 'draft',
  }
}

function isHomePageNotFoundError(error: unknown) {
  return error instanceof Error && /(404|not found|page not found)/i.test(error.message)
}

onMounted(() => {
  void loadInitialData()
})

async function loadInitialData() {
  loading.value = true
  try {
    const page = await getHomeSitePage().catch((error) => {
      if (isHomePageNotFoundError(error)) {
        return defaultHomePage()
      }
      throw error
    })
    applyPage(page)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '主页内容加载失败')
  } finally {
    loading.value = false
  }
}

async function loadPage() {
  loading.value = true
  try {
    applyPage(await getHomeSitePage())
  } catch (error) {
    if (isHomePageNotFoundError(error)) {
      applyPage(defaultHomePage())
      return
    }
    ElMessage.error(error instanceof Error ? error.message : '主页内容加载失败')
  } finally {
    loading.value = false
  }
}

function applyPage(page: SitePageAdmin) {
  pageTitle.value = page.title || '平台主页'
  blocks.value = cloneBlocks(page.draft_content)
  zhBlocks.value = cloneLocalizedBlocks(page.translations?.zh?.draft_content, page.draft_content)
  status.value = page.status
  publishedAt.value = page.published_at || ''
  if (selectedBlockIndex.value != null && selectedBlockIndex.value >= blocks.value.length) {
    selectedBlockIndex.value = null
  }
  if (selectedZhBlockIndex.value != null && selectedZhBlockIndex.value >= zhBlocks.value.length) {
    selectedZhBlockIndex.value = null
  }
}

function floorDiagnosticFor(block: SitePageBlock | null) {
  if (!block) {
    return undefined
  }
  const key = homeFloorDiagnosticKey(block)
  return key ? floorDiagnostics.value[key] : undefined
}

function isMissingPlatformCategoryError(error: unknown) {
  return error instanceof Error && /(类目不存在|not found)/i.test(error.message)
}

async function validateProductFloors() {
  const sequence = ++floorValidationSequence
  const entries = new Map<string, SitePageBlock>()
  for (const block of blocks.value) {
    const key = homeFloorDiagnosticKey(block)
    if (key && !entries.has(key)) {
      entries.set(key, block)
    }
  }

  const nextDiagnostics = Object.fromEntries(
    Array.from(entries.keys(), (key) => [key, 'checking' as HomeFloorDiagnostic]),
  )
  floorDiagnostics.value = nextDiagnostics

  const results = await Promise.all(Array.from(entries, async ([key, block]) => {
    try {
      const category = await getAdminPlatformCategoryById(Number(block.platform_category_id))
      const page = await getProducts({
        page: 1,
        page_size: 1,
        platform_category_id: category.id,
        source: productSourceParam(block.source),
      })
      return { key, categoryID: category.id, categoryPath: category.path, diagnostic: (page.items.length ? 'ok' : 'empty') as HomeFloorDiagnostic }
    } catch (error) {
      return {
        key,
        categoryID: Number(block.platform_category_id),
        categoryPath: '',
        diagnostic: (isMissingPlatformCategoryError(error) ? 'missing' : 'unavailable') as HomeFloorDiagnostic,
      }
    }
  }))

  if (sequence !== floorValidationSequence) {
    return
  }
  floorDiagnostics.value = Object.fromEntries(results.map((result) => [result.key, result.diagnostic]))
  for (const result of results) {
    if (!result.categoryPath) {
      continue
    }
    for (const block of blocks.value) {
      if (block.type === 'product_floor' && Number(block.platform_category_id) === result.categoryID) {
        block.platform_category_path = result.categoryPath
      }
    }
  }
}

function cloneBlocks(content?: SitePageContent) {
  return (content?.blocks || []).map((block, index) => {
    const cloned = cloneBlockWithDefaults(block)
    if (cloned.type === 'map_section') {
      cloned.sort_order = index
    }
    return cloned
  })
}

function cloneLocalizedBlocks(localizedContent?: SitePageContent, englishContent?: SitePageContent) {
  const englishBlocks = cloneBlocks(englishContent)
  const localizedBlocks = cloneBlocks(localizedContent)
  return englishBlocks.map((block, index) => mergeLocalizedCmsNode(asCmsNode(block), localizedBlocks[index] ? asCmsNode(localizedBlocks[index]) : undefined, false) as unknown as SitePageBlock)
}

function addBlock(type: SitePageBlock['type']) {
  const block = createDefaultBlock(type)
  const defaultIndex = type === 'map_section'
    ? blocks.value.findIndex((candidate) => candidate.type === 'product_floor')
    : -1
  const insertIndex = defaultIndex >= 0 ? defaultIndex : blocks.value.length
  blocks.value.splice(insertIndex, 0, block)
  selectedBlockIndex.value = insertIndex
  syncZhBlocksWithEnglish()
}

function replaceBlockWithDefaults(type: SitePageBlock['type']) {
  if (selectedBlockIndex.value == null) {
    return
  }
  const currentTitle = blocks.value[selectedBlockIndex.value]?.title
  blocks.value[selectedBlockIndex.value] = createDefaultBlock(type)
  if (currentTitle) {
    blocks.value[selectedBlockIndex.value].title = currentTitle
  }
  syncZhBlocksWithEnglish()
}

function useDefaultTemplate() {
  blocks.value = createDefaultHomeBlocks()
  zhBlocks.value = cloneLocalizedBlocks(undefined, { blocks: blocks.value })
  selectedBlockIndex.value = null
  selectedZhBlockIndex.value = null
  ElMessage.success('默认首页模板已生成')
}

async function removeBlock(index: number) {
  try {
    await ElMessageBox.confirm('确认删除这个区块？', '删除区块', { type: 'warning' })
  } catch {
    return
  }
  blocks.value.splice(index, 1)
  syncZhBlocksWithEnglish()
  if (selectedBlockIndex.value === index) {
    selectedBlockIndex.value = null
  } else if (selectedBlockIndex.value != null && selectedBlockIndex.value > index) {
    selectedBlockIndex.value -= 1
  }
}

function contentForSave(): SitePageContent {
  return {
    blocks: blocks.value.map((block, index) => prepareBlockForSave(
      block.type === 'map_section' ? { ...block, sort_order: index } : block,
    )),
  }
}

function zhContentForSave(englishContent: SitePageContent): SitePageContent {
  const localizedBlocks = zhBlocks.value.map((block) => prepareBlockForSave(block))
  const localizedByTrackingKey = new Map(
    localizedBlocks
      .filter((block) => block.tracking_key)
      .map((block) => [block.tracking_key as string, block]),
  )
  return {
    blocks: englishContent.blocks.map((block, index) => {
      const localized = (block.tracking_key && localizedByTrackingKey.get(block.tracking_key)) || localizedBlocks[index]
      return mergeLocalizedCmsNode(asCmsNode(block), localized ? asCmsNode(localized) : undefined, false) as unknown as SitePageBlock
    }),
  }
}

function asCmsNode(block: SitePageBlock): Record<string, unknown> {
  return block as unknown as Record<string, unknown>
}

function syncZhBlocksWithEnglish() {
  const englishContent = contentForSave()
  zhBlocks.value = zhContentForSave(englishContent).blocks.map((block) => cloneBlockWithDefaults(block))
  if (selectedZhBlockIndex.value != null && selectedZhBlockIndex.value >= zhBlocks.value.length) {
    selectedZhBlockIndex.value = null
  }
}

function mergeLocalizedCmsNode(baseNode: Record<string, unknown>, localizedNode: Record<string, unknown> | undefined, fallbackToBase: boolean): Record<string, unknown> {
  const normalizedLocalized = localizedNode && baseNode.type === localizedNode.type ? localizedNode : localizedNode && !baseNode.type ? localizedNode : undefined
  const result: Record<string, unknown> = {}

  for (const [key, baseValue] of Object.entries(baseNode)) {
    if (translatableCmsFields.has(key)) {
      const localizedValue = normalizedLocalized ? normalizeLocalizedText(normalizedLocalized[key]) : ''
      result[key] = localizedValue || (fallbackToBase ? cloneCmsValue(baseValue) : '')
      continue
    }
    if (nestedCmsLists.has(key)) {
      result[key] = mergeLocalizedCmsList(baseValue, normalizedLocalized?.[key], fallbackToBase)
      continue
    }
    result[key] = cloneCmsValue(baseValue)
  }

  return result
}

function mergeLocalizedCmsList(baseValue: unknown, localizedValue: unknown, fallbackToBase: boolean) {
  const baseItems = cmsNodeList(baseValue)
  const localizedItems = cmsNodeList(localizedValue)
  return baseItems.map((item, index) => mergeLocalizedCmsNode(item, localizedItems[index], fallbackToBase))
}

function cmsNodeList(value: unknown): Record<string, unknown>[] {
  if (!Array.isArray(value)) {
    return []
  }
  return value.filter((item): item is Record<string, unknown> => typeof item === 'object' && item != null && !Array.isArray(item))
}

function normalizeLocalizedText(value: unknown) {
  return typeof value === 'string' ? value.trim() : ''
}

function cloneCmsValue<T>(value: T): T {
  if (Array.isArray(value)) {
    return value.map((item) => cloneCmsValue(item)) as T
  }
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.entries(value).map(([key, nested]) => [key, cloneCmsValue(nested)])) as T
  }
  return value
}

async function saveDraft(showMessage = true) {
  saving.value = true
  try {
    const englishContent = contentForSave()
    const saved = await saveHomeSitePageDraft(pageTitle.value.trim() || '平台主页', englishContent, {
      zh: {
        draft_content: zhContentForSave(englishContent),
      },
    })
    applyPage(saved)
    if (showMessage) {
      ElMessage.success('草稿已保存')
    }
    return true
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '保存失败')
    return false
  } finally {
    saving.value = false
  }
}

async function publishPage() {
  publishing.value = true
  try {
    const saved = await saveDraft(false)
    if (!saved) {
      return
    }
    const published = await publishHomeSitePage()
    applyPage(published)
    ElMessage.success('主页已发布')
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '发布失败')
  } finally {
    publishing.value = false
  }
}
</script>

<template>
  <section v-loading="loading" class="space-y-4">
    <div class="flex flex-col gap-3 rounded-lg bg-white p-4 shadow-sm lg:flex-row lg:items-center lg:justify-between">
      <div>
        <h2 class="text-xl font-black text-slate-950">主页编辑</h2>
        <p class="mt-1 text-sm text-slate-500">编辑平台主页草稿，发布后前台 / 会读取，/platform-home 保留为预览路径</p>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <el-tag :type="status === 'published' ? 'success' : 'info'">{{ statusText }}</el-tag>
        <span v-if="publishedAt" class="text-xs text-slate-500">发布时间：{{ new Date(publishedAt).toLocaleString() }}</span>
        <el-button @click="loadPage">刷新</el-button>
        <el-button type="primary" :loading="saving" @click="saveDraft()">保存草稿</el-button>
        <el-button type="success" :loading="publishing" @click="publishPage">发布</el-button>
      </div>
    </div>

    <el-tabs v-model="activeLanguage" class="rounded-lg bg-white px-4 pt-2 shadow-sm">
      <el-tab-pane :label="t('adminTranslations.english')" name="en">
        <div class="grid gap-4 xl:grid-cols-[260px_1fr_360px]" style="height: calc(100vh - 270px); min-height: 560px;">
          <BlockLibrary class="h-full" @add="addBlock" />
          <BlockCanvas v-model:blocks="blocks" v-model:selected="selectedBlockIndex" @remove="removeBlock" />

          <aside class="h-full">
            <BlockSettings v-if="selectedBlock" :block="selectedBlock" :floor-diagnostic="floorDiagnosticFor(selectedBlock)" @change-type="replaceBlockWithDefaults" />
            <div v-else class="flex h-full flex-col rounded-lg border border-slate-200 bg-white shadow-sm">
              <div class="border-b border-slate-100 px-4 py-3">
                <h3 class="text-sm font-black text-slate-950">实时预览</h3>
              </div>
              <div class="flex-1 overflow-y-auto p-4">
                <div v-if="!blocks.length" class="rounded-lg bg-slate-50 p-6 text-center text-sm text-slate-500">
                  <p>暂无区块</p>
                  <el-button class="mt-4" type="primary" @click="useDefaultTemplate">使用默认首页模板</el-button>
                </div>
                <div v-else class="space-y-4">
                  <HomeBlockPreview v-for="(block, index) in blocks" :key="`preview-${index}`" :block="block" :floor-diagnostic="floorDiagnosticFor(block)" locale="en" />
                </div>
              </div>
            </div>
          </aside>
        </div>
      </el-tab-pane>

      <el-tab-pane :label="t('adminTranslations.chinese')" name="zh">
        <div class="grid gap-4 xl:grid-cols-[260px_1fr_360px]" style="height: calc(100vh - 270px); min-height: 560px;">
          <div class="flex h-full flex-col rounded-lg border border-slate-200 bg-white shadow-sm">
            <div class="border-b border-slate-100 px-4 py-3">
              <h3 class="text-sm font-black text-slate-950">中文文案</h3>
              <p class="mt-0.5 text-xs text-slate-500">结构、图片、链接、商品来源与 English 保持一致</p>
            </div>
            <div class="space-y-3 p-4 text-sm text-slate-600">
              <p>中文内容可选；未填写字段会在前台回退 English。</p>
              <p>保存时只保留标题、副标题、按钮文字、轮播文案、楼层标题和 footer 文案等文本字段。</p>
              <el-button type="primary" plain @click="syncZhBlocksWithEnglish">同步 English 结构</el-button>
            </div>
          </div>

          <BlockCanvas v-model:blocks="zhBlocks" v-model:selected="selectedZhBlockIndex" @remove="syncZhBlocksWithEnglish" />

          <aside class="h-full">
            <BlockSettings v-if="selectedZhBlock" :block="selectedZhBlock" :floor-diagnostic="floorDiagnosticFor(selectedZhBlock)" structure-readonly @change-type="syncZhBlocksWithEnglish" />
            <div v-else class="flex h-full flex-col rounded-lg border border-slate-200 bg-white shadow-sm">
              <div class="border-b border-slate-100 px-4 py-3">
                <h3 class="text-sm font-black text-slate-950">中文预览</h3>
              </div>
              <div class="flex-1 overflow-y-auto p-4">
                <div v-if="!zhBlocks.length" class="rounded-lg bg-slate-50 p-6 text-center text-sm text-slate-500">
                  <p>暂无区块</p>
                  <el-button class="mt-4" type="primary" @click="syncZhBlocksWithEnglish">同步 English 结构</el-button>
                </div>
                <div v-else class="space-y-4">
                  <HomeBlockPreview v-for="(block, index) in zhBlocks" :key="`zh-preview-${index}`" :block="block" :floor-diagnostic="floorDiagnosticFor(block)" locale="zh" />
                </div>
              </div>
            </div>
          </aside>
        </div>
      </el-tab-pane>
    </el-tabs>
  </section>
</template>
