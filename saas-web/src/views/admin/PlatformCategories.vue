<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Edit, Plus, Refresh } from '@element-plus/icons-vue'
import {
  confirmPlatformCategoryImport,
  createPlatformCategory,
  deletePlatformCategory,
  downloadPlatformCategoryImportTemplate,
  exportPlatformCategoryBilingualTable,
  getSuperPlatformCategoryById,
  getSuperPlatformCategoryChildren,
  previewPlatformCategoryImport,
  rebuildPlatformCategorySearchIndex,
  searchSuperPlatformCategories,
  updatePlatformCategory,
  type PlatformCategory,
  type PlatformCategoryImportItem,
  type PlatformCategoryImportPreview,
  type PlatformCategoryImportRowError,
  type PlatformCategoryImportTemplateFormat,
  type PlatformCategoryPayload,
  type PlatformCategoryStatus,
} from '../../api/platformCategories'
import {
  isCurrentPlatformCategorySearch,
  shouldSchedulePlatformCategorySearch,
} from '../../utils/platformCategorySearch.ts'

type PlatformCategoryFormMode = 'create' | 'edit' | null

interface PlatformCategoryFormModel {
  id?: number
  parent_id: number | null
  name: string
  zh_name: string
  sort_order?: number
  status: PlatformCategoryStatus
}

const { t } = useI18n()
const loading = ref(false)
const tableKey = ref(0)
const searchKeyword = ref('')
const searchLoading = ref(false)
const searchResults = ref<PlatformCategory[]>([])
const rebuildSearchIndexLoading = ref(false)
const categorySearchInputComposing = ref(false)
let categorySearchTimer: ReturnType<typeof setTimeout> | undefined
let categorySearchAbortController: AbortController | undefined
let categorySearchSequence = 0
const saving = ref(false)
const importDialogVisible = ref(false)
const importPreviewLoading = ref(false)
const importConfirming = ref(false)
const importFile = ref<File | null>(null)
const importPreview = ref<PlatformCategoryImportPreview | null>(null)
const importFileInput = ref<HTMLInputElement>()
const importErrorPage = ref(1)
const importPreviewPage = ref(1)
const dialogVisible = ref(false)
const formRef = ref<FormInstance>()
const categoryTableRef = ref<{ updateKeyChildren: (key: number, children: PlatformCategory[]) => void }>()
const categories = ref<PlatformCategory[]>([])
const parentOptions = ref<PlatformCategory[]>([])
const parentSearchLoading = ref(false)
let parentSearchAbortController: AbortController | undefined
let parentSearchSequence = 0
const form = reactive<PlatformCategoryFormModel>(emptyForm())
const mode = ref<PlatformCategoryFormMode>(null)
const currentEditingId = ref<number>()
const currentOriginalParentId = ref<number | null>(null)
const currentEditPath = ref('')
const isSearchMode = computed(() => Boolean(searchKeyword.value.trim()))
const hasImportErrors = computed(() => Boolean(importPreview.value?.invalid_rows || importPreview.value?.errors.length))
const importErrorPageSize = 50
const importPreviewPageSize = 100
const platformCategoryImportTimeoutMessage = '导入处理时间较长，请稍后刷新类目列表确认结果；请勿重复提交。'
const pagedImportErrors = computed(() => {
  const start = (importErrorPage.value - 1) * importErrorPageSize
  return importPreview.value?.errors.slice(start, start + importErrorPageSize) || []
})
const pagedImportCategories = computed(() => {
  const start = (importPreviewPage.value - 1) * importPreviewPageSize
  return importPreview.value?.preview_categories.slice(start, start + importPreviewPageSize) || []
})

const rules: FormRules<PlatformCategoryFormModel> = {
  name: [{ required: true, message: '请输入类目名称', trigger: 'blur' }],
  status: [{ required: true, message: '请选择状态', trigger: 'change' }],
}

onMounted(() => {
  void loadRootCategories()
})

onBeforeUnmount(() => {
  if (categorySearchTimer) {
    clearTimeout(categorySearchTimer)
  }
  categorySearchAbortController?.abort()
  parentSearchAbortController?.abort()
  categorySearchSequence += 1
  parentSearchSequence += 1
})

function emptyForm(): PlatformCategoryFormModel {
  return {
    parent_id: null,
    name: '',
    zh_name: '',
    sort_order: undefined,
    status: 'active',
  }
}

function resetForm(nextMode: PlatformCategoryFormMode = null) {
  formRef.value?.clearValidate()
  Object.assign(form, emptyForm())
  delete form.id
  mode.value = nextMode
  currentEditingId.value = undefined
  currentOriginalParentId.value = null
  currentEditPath.value = ''
  parentOptions.value = [...categories.value]
}

function platformCategoryErrorMessage(error: unknown, fallback: string, conflictMessage?: string) {
  if (!(error instanceof Error)) {
    return fallback
  }
  if (error.message === 'not found' || /(^|\s)(404|Not Found)(\s|$)/i.test(error.message)) {
    return '类目不存在，请刷新后重试'
  }
  if (error.message === 'conflict') {
    return conflictMessage || fallback
  }
  return error.message || fallback
}

async function loadRootCategories() {
  loading.value = true
  try {
    categories.value = await getSuperPlatformCategoryChildren()
    tableKey.value += 1
  } catch (error) {
    ElMessage.error(platformCategoryErrorMessage(error, '平台类目加载失败'))
  } finally {
    loading.value = false
  }
}

async function loadCategoryChildren(
  category: PlatformCategory,
  _treeNode: unknown,
  resolve: (children: PlatformCategory[]) => void,
) {
  try {
    const children = await getSuperPlatformCategoryChildren(category.id)
    category.children = children
    resolve(children)
  } catch (error) {
    resolve([])
    ElMessage.error(platformCategoryErrorMessage(error, '子类目加载失败'))
  }
}

async function performCategorySearch(keyword: string, sequence: number) {
  const controller = new AbortController()
  categorySearchAbortController = controller
  searchLoading.value = true
  try {
    const results = await searchSuperPlatformCategories(keyword, {
      limit: 100,
      locale: 'all',
      signal: controller.signal,
    })
    if (isCurrentPlatformCategorySearch(sequence, categorySearchSequence, keyword, searchKeyword.value)) {
      searchResults.value = results
    }
  } catch (error) {
    if (sequence === categorySearchSequence) {
      ElMessage.error(platformCategoryErrorMessage(error, '平台类目搜索失败'))
    }
  } finally {
    if (sequence === categorySearchSequence) {
      searchLoading.value = false
    }
    if (categorySearchAbortController === controller) {
      categorySearchAbortController = undefined
    }
  }
}

function scheduleCategorySearch(value: string) {
  if (categorySearchTimer) {
    clearTimeout(categorySearchTimer)
    categorySearchTimer = undefined
  }
  categorySearchAbortController?.abort()
  categorySearchAbortController = undefined
  const sequence = ++categorySearchSequence
  if (!shouldSchedulePlatformCategorySearch(value, categorySearchInputComposing.value)) {
    searchLoading.value = false
    if (!categorySearchInputComposing.value) {
      clearCategorySearch()
    }
    return
  }
  const keyword = value.trim()
  categorySearchTimer = setTimeout(() => {
    void performCategorySearch(keyword, sequence)
  }, 250)
}

async function submitCategorySearch() {
  if (categorySearchInputComposing.value) {
    return
  }
  scheduleCategorySearch(searchKeyword.value)
  if (categorySearchTimer) {
    clearTimeout(categorySearchTimer)
    categorySearchTimer = undefined
  }
  const keyword = searchKeyword.value.trim()
  if (keyword) {
    await performCategorySearch(keyword, categorySearchSequence)
  }
}

function handleCategorySearchCompositionStart() {
  categorySearchInputComposing.value = true
  if (categorySearchTimer) {
    clearTimeout(categorySearchTimer)
    categorySearchTimer = undefined
  }
  categorySearchAbortController?.abort()
  categorySearchAbortController = undefined
  categorySearchSequence += 1
  searchLoading.value = false
}

function handleCategorySearchCompositionEnd(event: CompositionEvent) {
  categorySearchInputComposing.value = false
  const input = event.target as HTMLInputElement | null
  searchKeyword.value = input?.value ?? searchKeyword.value
  scheduleCategorySearch(searchKeyword.value)
}

function clearCategorySearch() {
  categorySearchInputComposing.value = false
  searchKeyword.value = ''
  searchResults.value = []
  categorySearchSequence += 1
  searchLoading.value = false
  categorySearchAbortController?.abort()
  categorySearchAbortController = undefined
  if (categorySearchTimer) {
    clearTimeout(categorySearchTimer)
    categorySearchTimer = undefined
  }
}

async function handleParentSearch(keyword: string) {
  parentSearchAbortController?.abort()
  parentSearchAbortController = undefined
  const sequence = ++parentSearchSequence
  const normalized = keyword.trim()
  if (!normalized) {
    parentOptions.value = [...categories.value]
    parentSearchLoading.value = false
    return
  }
  const controller = new AbortController()
  parentSearchAbortController = controller
  parentSearchLoading.value = true
  try {
    const results = await searchSuperPlatformCategories(normalized, {
      limit: 50,
      locale: 'all',
      signal: controller.signal,
    })
    if (sequence === parentSearchSequence) {
      parentOptions.value = results.filter(isAllowedParentOption)
    }
  } catch (error) {
    if (sequence === parentSearchSequence) {
      ElMessage.error(platformCategoryErrorMessage(error, '上级类目搜索失败'))
    }
  } finally {
    if (sequence === parentSearchSequence) {
      parentSearchLoading.value = false
    }
    if (parentSearchAbortController === controller) {
      parentSearchAbortController = undefined
    }
  }
}

function isAllowedParentOption(category: PlatformCategory) {
  return !currentEditingId.value ||
    (category.id !== currentEditingId.value && !category.path.startsWith(`${currentEditPath.value}/`))
}

function addParentOption(category: PlatformCategory | null | undefined) {
  if (!category || !isAllowedParentOption(category) || parentOptions.value.some((item) => item.id === category.id)) {
    return
  }
  parentOptions.value.unshift(category)
}

function openCreateRoot() {
  resetForm('create')
  form.parent_id = null
  parentOptions.value = [...categories.value]
  dialogVisible.value = true
}

function openCreateChild(parent: PlatformCategory) {
  resetForm('create')
  form.parent_id = parent.id
  parentOptions.value = [parent, ...categories.value.filter((item) => item.id !== parent.id)]
  dialogVisible.value = true
}

async function openEdit(category: PlatformCategory) {
  resetForm('edit')
  let detail = category
  try {
    detail = await getSuperPlatformCategoryById(category.id)
  } catch {
    // The table row still allows editing if detail reload fails; translations will remain empty.
  }
  Object.assign(form, {
    id: detail.id,
    parent_id: detail.parent_id,
    name: detail.name,
    zh_name: detail.translations?.zh?.name || '',
    sort_order: detail.sort_order,
    status: detail.status,
  })
  currentEditingId.value = detail.id
  currentOriginalParentId.value = detail.parent_id
  currentEditPath.value = detail.path
  parentOptions.value = categories.value.filter(isAllowedParentOption)
  if (category.parent_id) {
    try {
      addParentOption(await getSuperPlatformCategoryById(category.parent_id))
    } catch {
      // The form can still be edited; the current parent may have been deleted concurrently.
    }
  }
  dialogVisible.value = true
}

function buildPayload(): PlatformCategoryPayload {
  const payload: PlatformCategoryPayload = {
    name: form.name.trim(),
    sort_order: form.sort_order ?? 0,
    status: form.status,
    translations: {
      zh: {
        name: form.zh_name.trim(),
      },
    },
  }
  if (form.parent_id != null && form.parent_id > 0) {
    payload.parent_id = form.parent_id
  }
  return payload
}

async function saveCategory() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) {
    return
  }
  saving.value = true
  try {
    const payload = buildPayload()
    const nextParentId = payload.parent_id ?? null
    const affectedParentIds = mode.value === 'edit'
      ? [currentOriginalParentId.value, nextParentId]
      : [nextParentId]
    if (mode.value === 'create') {
      await createPlatformCategory(payload)
      ElMessage.success('平台类目已创建')
    } else if (mode.value === 'edit' && currentEditingId.value) {
      await updatePlatformCategory(currentEditingId.value, payload)
      ElMessage.success('平台类目已更新')
    } else {
      ElMessage.error('请重新打开类目表单后再保存')
      return
    }
    dialogVisible.value = false
    resetForm()
    await refreshCategoryView(affectedParentIds)
  } catch (error) {
    ElMessage.error(platformCategoryErrorMessage(error, '保存失败', '同级类目已存在'))
  } finally {
    saving.value = false
  }
}

async function removeCategory(category: PlatformCategory) {
  try {
    await ElMessageBox.confirm(h('div', [
      h('p', '确认删除该类目及其所有空子类目吗？'),
      h('p', { class: 'mt-2 text-sm text-slate-500' }, 'Delete this category and all empty subcategories?'),
      h('p', { class: 'mt-2 text-sm font-bold text-slate-700' }, `类目：${category.name}`),
    ]), '删除确认', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning',
    })
    await deletePlatformCategory(category.id)
    ElMessage.success('平台类目已删除')
    if (form.id === category.id || currentEditingId.value === category.id) {
      dialogVisible.value = false
    }
    resetForm()
    await refreshCategoryView([category.parent_id])
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') {
      ElMessage.error(platformCategoryErrorMessage(error, '删除失败'))
    }
  }
}

async function refreshCategoryView(parentIds: Array<number | null>) {
  if (isSearchMode.value) {
    await submitCategorySearch()
    return
  }
  const uniqueParentIds = [...new Set(parentIds)]
  if (uniqueParentIds.includes(null)) {
    await loadRootCategories()
    return
  }
  await Promise.all(uniqueParentIds.map(async (parentId) => {
    if (parentId == null) {
      return
    }
    const children = await getSuperPlatformCategoryChildren(parentId)
    const parent = findLoadedCategory(categories.value, parentId)
    if (parent) {
      parent.children = children
      parent.has_children = children.length > 0
      parent.is_leaf = children.length === 0
    }
    categoryTableRef.value?.updateKeyChildren(parentId, children)
  }))
}

function findLoadedCategory(nodes: PlatformCategory[], id: number): PlatformCategory | undefined {
  for (const category of nodes) {
    if (category.id === id) {
      return category
    }
    const found = findLoadedCategory(category.children || [], id)
    if (found) {
      return found
    }
  }
  return undefined
}

function statusLabel(status: PlatformCategoryStatus) {
  return status === 'active' ? '启用' : '停用'
}

function englishPath(category: PlatformCategory) {
  return category.path.replaceAll('/', ' > ')
}

function zhName(category: PlatformCategory) {
  return category.zh_name?.trim() || category.translations?.zh?.name?.trim() || '未填写中文'
}

function zhPath(category: PlatformCategory) {
  return category.zh_path?.trim() || '未填写中文路径'
}

function importActionLabel(action: PlatformCategoryImportItem['action']) {
  if (action === 'create') {
    return '新增'
  }
  if (action === 'update') {
    return '更新'
  }
  return '跳过'
}

function importActionTagType(action: PlatformCategoryImportItem['action']) {
  if (action === 'create') {
    return 'success'
  }
  if (action === 'update') {
    return 'warning'
  }
  return 'info'
}

async function downloadImportTemplate(format: PlatformCategoryImportTemplateFormat = 'path') {
  try {
    const blob = await downloadPlatformCategoryImportTemplate(format)
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = format === 'path'
      ? 'platform-category-import-template-path.xlsx'
      : 'platform-category-import-template-levels.xlsx'
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '模板下载失败')
  }
}

async function exportBilingualTable() {
  try {
    const blob = await exportPlatformCategoryBilingualTable()
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `platform-category-bilingual-${new Date().toISOString().slice(0, 10)}.xlsx`
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '双语表导出失败')
  }
}

async function rebuildSearchIndex() {
  rebuildSearchIndexLoading.value = true
  try {
    await rebuildPlatformCategorySearchIndex()
    ElMessage.success('平台类目搜索索引已重建')
    if (isSearchMode.value) {
      await submitCategorySearch()
    }
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : '搜索索引重建失败')
  } finally {
    rebuildSearchIndexLoading.value = false
  }
}

function openImportDialog() {
  resetImportState()
  importDialogVisible.value = true
}

function resetImportState() {
  importFile.value = null
  importPreview.value = null
  importErrorPage.value = 1
  importPreviewPage.value = 1
  if (importFileInput.value) {
    importFileInput.value.value = ''
  }
}

function chooseImportFile() {
  if (importConfirming.value) {
    return
  }
  importFileInput.value?.click()
}

async function handleImportFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  importPreview.value = null
  if (!file) {
    importFile.value = null
    return
  }
  if (!file.name.toLowerCase().endsWith('.xlsx')) {
    ElMessage.error('仅支持 .xlsx 文件')
    input.value = ''
    return
  }
  if (file.size > 10 * 1024 * 1024) {
    ElMessage.error('Excel 文件不能超过 10MB')
    input.value = ''
    return
  }
  importFile.value = file
  await previewImportFile()
}

async function previewImportFile() {
  if (importConfirming.value) {
    ElMessage.warning('导入处理中，请勿重复操作')
    return
  }
  if (!importFile.value) {
    ElMessage.warning('请先选择 Excel 文件')
    return
  }
  importPreviewLoading.value = true
  try {
    importPreview.value = await previewPlatformCategoryImport(importFile.value)
    importErrorPage.value = 1
    importPreviewPage.value = 1
    if (hasImportErrors.value) {
      ElMessage.warning('Excel 存在错误，请修正后重新上传')
    } else {
      ElMessage.success('Excel 解析通过')
    }
  } catch (error) {
    ElMessage.error(error instanceof Error ? error.message : 'Excel 解析失败')
  } finally {
    importPreviewLoading.value = false
  }
}

async function confirmImportFile() {
  if (!importFile.value || !importPreview.value || hasImportErrors.value) {
    return
  }
  if (importConfirming.value) {
    return
  }
  importConfirming.value = true
  try {
    const result = await confirmPlatformCategoryImport(importFile.value)
    ElMessage.success(`导入完成：新建 ${result.created}，更新 ${result.updated}，不变 ${result.unchanged}`)
    importDialogVisible.value = false
    resetImportState()
    clearCategorySearch()
    await loadRootCategories()
  } catch (error) {
    const message = error instanceof Error ? error.message : ''
    if (/timeout|exceeded|ECONNABORTED/i.test(message)) {
      ElMessage.warning(platformCategoryImportTimeoutMessage)
    } else {
      ElMessage.error(message || '平台类目导入失败')
    }
  } finally {
    importConfirming.value = false
  }
}
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-col gap-3 rounded-2xl bg-white p-4 shadow-sm md:flex-row md:items-center md:justify-between">
      <div>
        <h2 class="text-xl font-black text-slate-950">平台类目管理</h2>
        <p class="mt-1 text-sm text-slate-500">平台统一类目，用于商品发布、搜索、导航和运营管理。商品发布时请选择最末级类目。</p>
      </div>
      <div class="flex flex-wrap gap-2">
        <el-button @click="downloadImportTemplate('path')">下载双语类目模板</el-button>
        <el-button @click="exportBilingualTable">导出当前双语表</el-button>
        <el-button :icon="Refresh" :loading="rebuildSearchIndexLoading" @click="rebuildSearchIndex">重建搜索索引</el-button>
        <el-button type="success" @click="openImportDialog">导入双语类目 Excel</el-button>
        <el-button type="primary" :icon="Plus" @click="openCreateRoot">新增根类目</el-button>
      </div>
    </div>

    <el-card shadow="never">
      <div class="mb-4 flex flex-col gap-2 md:flex-row md:items-center">
        <el-input
          v-model="searchKeyword"
          clearable
          class="max-w-xl"
          placeholder="按英文/中文类目名称或完整路径搜索，最多返回 100 条"
          @input="scheduleCategorySearch"
          @keyup.enter="submitCategorySearch"
          @clear="clearCategorySearch"
          @compositionstart="handleCategorySearchCompositionStart"
          @compositionend="handleCategorySearchCompositionEnd"
        />
        <el-button type="primary" :loading="searchLoading" @click="submitCategorySearch">搜索</el-button>
        <el-button v-if="isSearchMode" @click="clearCategorySearch">返回懒加载树</el-button>
        <span class="text-xs text-slate-500">
          {{ isSearchMode ? `搜索结果 ${searchResults.length} 条` : '默认只加载一级类目，展开后加载直接子类目' }}
        </span>
      </div>

      <el-table
        v-if="!isSearchMode"
        ref="categoryTableRef"
        :key="tableKey"
        v-loading="loading"
        :data="categories"
        row-key="id"
        border
        lazy
        :load="loadCategoryChildren"
        :tree-props="{ children: 'children', hasChildren: 'has_children' }"
      >
        <el-table-column label="类目名称" min-width="220">
          <template #default="{ row }: { row: PlatformCategory }">
            <div class="min-w-0">
              <p class="break-words font-semibold text-slate-900">{{ row.name }}</p>
              <p class="mt-1 break-words text-xs text-slate-500">{{ zhName(row) }}</p>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="完整路径" min-width="320">
          <template #default="{ row }: { row: PlatformCategory }">
            <div class="min-w-0">
              <p class="break-words font-semibold text-slate-900">{{ englishPath(row) }}</p>
              <p class="mt-1 break-words text-xs text-slate-500">{{ zhPath(row) }}</p>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="level" label="层级" width="90" />
        <el-table-column prop="sort_order" label="排序" width="90" />
        <el-table-column label="状态" width="100">
          <template #default="{ row }: { row: PlatformCategory }">
            <el-tag :type="row.status === 'active' ? 'success' : 'info'">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="260" fixed="right">
          <template #default="{ row }: { row: PlatformCategory }">
            <div class="flex flex-wrap gap-2">
              <el-button size="small" :icon="Plus" @click="openCreateChild(row)">子类目</el-button>
              <el-button size="small" :icon="Edit" @click="openEdit(row)">编辑</el-button>
              <el-button size="small" type="danger" :icon="Delete" @click="removeCategory(row)">删除</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <el-table v-else v-loading="searchLoading" :data="searchResults" row-key="id" border>
        <el-table-column label="类目名称" min-width="220">
          <template #default="{ row }: { row: PlatformCategory }">
            <div class="min-w-0">
              <p class="break-words font-semibold text-slate-900">{{ row.name }}</p>
              <p class="mt-1 break-words text-xs text-slate-500">{{ zhName(row) }}</p>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="完整路径" min-width="360">
          <template #default="{ row }: { row: PlatformCategory }">
            <div class="min-w-0">
              <p class="break-words font-semibold text-slate-900">{{ englishPath(row) }}</p>
              <p class="mt-1 break-words text-xs text-slate-500">{{ zhPath(row) }}</p>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="level" label="层级" width="90" />
        <el-table-column prop="sort_order" label="排序" width="90" />
        <el-table-column label="类型" width="110">
          <template #default="{ row }: { row: PlatformCategory }">
            <el-tag :type="row.is_leaf ? 'success' : 'warning'">{{ row.is_leaf ? '末级类目' : '父级类目' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }: { row: PlatformCategory }">
            <el-tag :type="row.status === 'active' ? 'success' : 'info'">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="260" fixed="right">
          <template #default="{ row }: { row: PlatformCategory }">
            <div class="flex flex-wrap gap-2">
              <el-button size="small" :icon="Plus" @click="openCreateChild(row)">子类目</el-button>
              <el-button size="small" :icon="Edit" @click="openEdit(row)">编辑</el-button>
              <el-button size="small" type="danger" :icon="Delete" @click="removeCategory(row)">删除</el-button>
            </div>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="未找到匹配的平台类目" />
        </template>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="mode === 'edit' ? '编辑平台类目' : '新增平台类目'" width="520px" @closed="resetForm()">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="110px">
        <el-form-item label="上级类目">
          <el-select
            v-model="form.parent_id"
            class="w-full"
            clearable
            filterable
            remote
            reserve-keyword
            :remote-method="handleParentSearch"
            :loading="parentSearchLoading"
            placeholder="根类目；输入名称或路径搜索"
          >
            <el-option :value="null" label="根类目" />
            <el-option v-for="item in parentOptions" :key="item.id" :value="item.id" :label="item.path" />
          </el-select>
        </el-form-item>
        <el-tabs class="admin-translation-tabs">
          <el-tab-pane :label="t('adminTranslations.english')">
            <el-form-item label="类目名称" prop="name">
              <el-input v-model="form.name" maxlength="120" show-word-limit />
            </el-form-item>
          </el-tab-pane>
          <el-tab-pane :label="t('adminTranslations.chinese')">
            <el-form-item label="中文名称">
              <el-input v-model="form.zh_name" maxlength="120" show-word-limit placeholder="可选；不填则前台 fallback" />
            </el-form-item>
          </el-tab-pane>
        </el-tabs>
        <el-form-item label="排序">
          <el-input-number v-model="form.sort_order" :min="0" :step="1" />
        </el-form-item>
        <el-form-item label="状态" prop="status">
          <el-radio-group v-model="form.status">
            <el-radio-button label="active">启用</el-radio-button>
            <el-radio-button label="disabled">停用</el-radio-button>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveCategory">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="importDialogVisible"
      title="平台类目 Excel 导入"
      width="900px"
      :close-on-click-modal="!importConfirming"
      :close-on-press-escape="!importConfirming"
      :show-close="!importConfirming"
      @closed="resetImportState"
    >
      <div class="space-y-4">
        <input ref="importFileInput" class="hidden" type="file" accept=".xlsx" @change="handleImportFileChange" />

        <div class="rounded-lg border border-dashed border-slate-200 bg-slate-50 p-4">
          <div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
            <div>
              <p class="text-sm font-bold text-slate-900">{{ importFile?.name || '请选择 .xlsx 文件' }}</p>
              <p class="mt-1 text-xs leading-5 text-slate-500">最大 10MB，最多 10000 行。推荐字段：English Path、中文路径、Sort Order、Status；导出文件中的 Category ID 可用于原地迁移历史类目。</p>
            </div>
            <div class="flex flex-wrap gap-2">
              <el-button :disabled="importConfirming" @click="chooseImportFile">选择文件</el-button>
              <el-button :disabled="!importFile || importConfirming" :loading="importPreviewLoading" @click="previewImportFile">重新解析</el-button>
            </div>
          </div>
        </div>

        <el-alert
          v-if="importConfirming"
          title="正在导入，处理大文件可能需要数分钟；请勿关闭页面或重复提交。"
          type="warning"
          :closable="false"
          show-icon
        />

        <div v-if="importPreview" class="grid gap-3 md:grid-cols-3">
          <div class="rounded-lg bg-slate-50 p-4 text-center">
            <p class="text-xs font-bold text-slate-500">总行数</p>
            <p class="mt-1 text-2xl font-black text-slate-950">{{ importPreview.rows }}</p>
          </div>
          <div class="rounded-lg bg-emerald-50 p-4 text-center">
            <p class="text-xs font-bold text-emerald-700">有效行</p>
            <p class="mt-1 text-2xl font-black text-emerald-700">{{ importPreview.valid_rows }}</p>
          </div>
          <div class="rounded-lg bg-red-50 p-4 text-center">
            <p class="text-xs font-bold text-red-600">错误行</p>
            <p class="mt-1 text-2xl font-black text-red-600">{{ importPreview.invalid_rows }}</p>
          </div>
        </div>

        <el-alert
          v-if="importPreview && hasImportErrors"
          title="Excel 存在错误，修正后重新上传。确认导入已禁用。"
          type="error"
          :closable="false"
          show-icon
        />

        <div v-if="importPreview?.errors.length" class="space-y-3">
          <p class="text-xs text-slate-500">错误明细共 {{ importPreview.errors.length }} 条，每页显示 {{ importErrorPageSize }} 条。</p>
          <el-table :data="pagedImportErrors" border max-height="220">
            <el-table-column prop="row" label="行号" width="90" />
            <el-table-column prop="field" label="字段" width="140" />
            <el-table-column label="错误" min-width="300">
              <template #default="{ row }: { row: PlatformCategoryImportRowError }">
                <p>{{ row.message }}</p>
                <p v-if="row.message_en" class="mt-1 text-xs text-slate-500">{{ row.message_en }}</p>
              </template>
            </el-table-column>
          </el-table>
          <el-pagination
            v-if="importPreview.errors.length > importErrorPageSize"
            v-model:current-page="importErrorPage"
            background
            layout="prev, pager, next, total"
            :page-size="importErrorPageSize"
            :total="importPreview.errors.length"
          />
        </div>

        <div v-if="importPreview?.preview_categories.length" class="space-y-3">
          <p class="text-xs text-slate-500">预览明细共 {{ importPreview.preview_categories.length }} 条，每页显示 {{ importPreviewPageSize }} 条。</p>
          <el-table :data="pagedImportCategories" border max-height="320">
            <el-table-column prop="row" label="原始行号" width="100" />
            <el-table-column prop="path" label="类目完整路径" min-width="300" />
            <el-table-column prop="level" label="层级" width="80" />
            <el-table-column prop="sort_order" label="排序" width="90" />
            <el-table-column label="状态" width="100">
              <template #default="{ row }: { row: PlatformCategoryImportItem }">
                <el-tag :type="row.status === 'active' ? 'success' : 'info'">{{ statusLabel(row.status) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作类型" width="100">
              <template #default="{ row }: { row: PlatformCategoryImportItem }">
                <el-tag :type="importActionTagType(row.action)">{{ importActionLabel(row.action) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="规范化" min-width="220">
              <template #default="{ row }: { row: PlatformCategoryImportItem }">
                <el-tag v-if="row.normalized" type="warning" size="small">已自动规范化</el-tag>
                <span v-else class="text-slate-400">-</span>
                <p v-if="row.normalization_note" class="mt-1 text-xs text-amber-700">{{ row.normalization_note }}</p>
              </template>
            </el-table-column>
            <el-table-column prop="error" label="错误信息" min-width="220">
              <template #default="{ row }: { row: PlatformCategoryImportItem }">
                <span :class="row.error ? 'text-red-600' : 'text-slate-400'">{{ row.error || '-' }}</span>
              </template>
            </el-table-column>
          </el-table>
          <el-pagination
            v-if="importPreview.preview_categories.length > importPreviewPageSize"
            v-model:current-page="importPreviewPage"
            background
            layout="prev, pager, next, total"
            :page-size="importPreviewPageSize"
            :total="importPreview.preview_categories.length"
          />
        </div>
      </div>

      <template #footer>
        <el-button :disabled="importConfirming" @click="importDialogVisible = false">取消</el-button>
        <el-button type="primary" :disabled="!importPreview || hasImportErrors || importConfirming || importPreviewLoading" :loading="importConfirming" @click="confirmImportFile">确认导入</el-button>
      </template>
    </el-dialog>
  </section>
</template>
