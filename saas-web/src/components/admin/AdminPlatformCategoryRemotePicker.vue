<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import {
  getAdminPlatformCategoryById,
  searchAdminPlatformCategories,
  type PlatformCategory,
} from '../../api/platformCategories'
import {
  isCurrentPlatformCategorySearch,
  shouldSchedulePlatformCategorySearch,
} from '../../utils/platformCategorySearch.ts'

const props = withDefaults(defineProps<{
  modelValue?: number
  modelPath?: string
  disabled?: boolean
  placeholder?: string
}>(), {
  modelValue: undefined,
  modelPath: '',
  disabled: false,
  placeholder: '搜索英文/中文类目名称或完整路径',
})

const emit = defineEmits<{
  'update:modelValue': [value: number | undefined]
  'update:modelPath': [value: string]
}>()

const CATEGORY_SEARCH_LIMIT = 50
const CATEGORY_SEARCH_DEBOUNCE_MS = 250
const SELECTED_CATEGORY_CACHE_LIMIT = 50
const selectedCategoryCache = new Map<number, Promise<PlatformCategory>>()

const keyword = ref('')
const results = ref<PlatformCategory[]>([])
const selectedCategory = ref<PlatformCategory | null>(null)
const popoverVisible = ref(false)
const searchLoading = ref(false)
const unavailableCategoryID = ref<number | null>(null)
const isComposing = ref(false)
let searchTimer: ReturnType<typeof setTimeout> | undefined
let searchAbortController: AbortController | undefined
let searchSequence = 0
let selectionSequence = 0

const normalizedKeyword = computed(() => keyword.value.trim())
const selectedLabel = computed(() => {
  if (selectedCategory.value) {
    return categorySelectedLabel(selectedCategory.value)
  }
  if (props.modelValue) {
    return props.modelPath?.trim() || `Category ID ${props.modelValue}`
  }
  return ''
})

watch(
  () => props.modelValue,
  (categoryID) => {
    void loadSelectedCategory(categoryID)
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  if (searchTimer) {
    clearTimeout(searchTimer)
  }
  searchAbortController?.abort()
  searchSequence += 1
  selectionSequence += 1
})

function categoryEnglishPath(category: PlatformCategory) {
  return category.path.split('/').map((part) => part.trim()).filter(Boolean).join(' / ')
}

function categoryZhName(category: PlatformCategory) {
  return category.zh_name?.trim() || category.translations?.zh?.name?.trim() || category.name
}

function categoryZhPath(category: PlatformCategory) {
  return category.zh_path?.trim() || categoryEnglishPath(category)
}

function categoryTitle(category: PlatformCategory) {
  const zhName = categoryZhName(category)
  return zhName === category.name ? category.name : `${zhName} / ${category.name}`
}

function categorySelectedLabel(category: PlatformCategory) {
  const zhPath = categoryZhPath(category)
  const englishPath = categoryEnglishPath(category)
  return zhPath === englishPath ? englishPath : `${zhPath} / ${englishPath}`
}

function cacheCategory(category: PlatformCategory) {
  if (!selectedCategoryCache.has(category.id) && selectedCategoryCache.size >= SELECTED_CATEGORY_CACHE_LIMIT) {
    const oldestKey = selectedCategoryCache.keys().next().value
    if (oldestKey !== undefined) {
      selectedCategoryCache.delete(oldestKey)
    }
  }
  selectedCategoryCache.set(category.id, Promise.resolve(category))
}

function getCachedCategoryByID(categoryID: number) {
  const cached = selectedCategoryCache.get(categoryID)
  if (cached) {
    return cached
  }
  if (selectedCategoryCache.size >= SELECTED_CATEGORY_CACHE_LIMIT) {
    const oldestKey = selectedCategoryCache.keys().next().value
    if (oldestKey !== undefined) {
      selectedCategoryCache.delete(oldestKey)
    }
  }
  const request = getAdminPlatformCategoryById(categoryID).catch((error) => {
    selectedCategoryCache.delete(categoryID)
    throw error
  })
  selectedCategoryCache.set(categoryID, request)
  return request
}

async function loadSelectedCategory(categoryID?: number) {
  const sequence = ++selectionSequence
  unavailableCategoryID.value = null
  if (!categoryID) {
    selectedCategory.value = null
    return
  }
  if (selectedCategory.value?.id === categoryID) {
    return
  }
  try {
    const category = await getCachedCategoryByID(categoryID)
    if (sequence === selectionSequence && props.modelValue === categoryID) {
      selectedCategory.value = category
    }
  } catch {
    if (sequence === selectionSequence && props.modelValue === categoryID) {
      selectedCategory.value = null
      unavailableCategoryID.value = categoryID
    }
  }
}

function handleInput(value: string) {
  keyword.value = value
  popoverVisible.value = true
  if (searchTimer) {
    clearTimeout(searchTimer)
    searchTimer = undefined
  }
  searchAbortController?.abort()
  searchAbortController = undefined
  const sequence = ++searchSequence
  if (!shouldSchedulePlatformCategorySearch(value, isComposing.value)) {
    searchLoading.value = false
    if (!isComposing.value) {
      results.value = []
    }
    return
  }
  const nextKeyword = value.trim()
  searchTimer = setTimeout(() => {
    void searchOptions(nextKeyword, sequence)
  }, CATEGORY_SEARCH_DEBOUNCE_MS)
}

async function searchOptions(searchKeyword: string, sequence: number) {
  const controller = new AbortController()
  searchAbortController = controller
  searchLoading.value = true
  try {
    const categories = await searchAdminPlatformCategories(searchKeyword, {
      limit: CATEGORY_SEARCH_LIMIT,
      locale: 'all',
      leafOnly: false,
      signal: controller.signal,
    })
    if (isCurrentPlatformCategorySearch(sequence, searchSequence, searchKeyword, keyword.value)) {
      results.value = categories
    }
  } catch (error) {
    if (sequence === searchSequence) {
      ElMessage.error(error instanceof Error ? error.message : '平台类目搜索失败')
    }
  } finally {
    if (sequence === searchSequence) {
      searchLoading.value = false
    }
    if (searchAbortController === controller) {
      searchAbortController = undefined
    }
  }
}

function handleCompositionStart() {
  isComposing.value = true
  if (searchTimer) {
    clearTimeout(searchTimer)
    searchTimer = undefined
  }
  searchAbortController?.abort()
  searchAbortController = undefined
  searchSequence += 1
  searchLoading.value = false
}

function handleCompositionEnd(event: CompositionEvent) {
  isComposing.value = false
  const input = event.target as HTMLInputElement | null
  handleInput(input?.value ?? keyword.value)
}

function selectCategory(category: PlatformCategory) {
  selectedCategory.value = category
  unavailableCategoryID.value = null
  cacheCategory(category)
  emit('update:modelValue', category.id)
  emit('update:modelPath', category.path)
  clearSearch()
  popoverVisible.value = false
}

function clearSelection() {
  selectedCategory.value = null
  unavailableCategoryID.value = null
  emit('update:modelValue', undefined)
  emit('update:modelPath', '')
  clearSearch()
}

function clearSearch() {
  isComposing.value = false
  keyword.value = ''
  results.value = []
  searchSequence += 1
  searchLoading.value = false
  searchAbortController?.abort()
  searchAbortController = undefined
  if (searchTimer) {
    clearTimeout(searchTimer)
    searchTimer = undefined
  }
}

function handlePopoverHide() {
  clearSearch()
}
</script>

<template>
  <el-popover
    v-model:visible="popoverVisible"
    placement="bottom-start"
    trigger="click"
    :width="640"
    :disabled="disabled"
    @hide="handlePopoverHide"
  >
    <template #reference>
      <el-input
        :model-value="popoverVisible ? keyword : selectedLabel"
        :placeholder="placeholder"
        :disabled="disabled"
        @focus="popoverVisible = true"
        @input="handleInput"
        @compositionstart="handleCompositionStart"
        @compositionend="handleCompositionEnd"
      />
    </template>

    <div class="space-y-3">
      <div class="flex items-center justify-between gap-3 border-b border-slate-100 pb-2">
        <p class="text-sm font-bold text-slate-800">
          {{ normalizedKeyword ? `搜索“${normalizedKeyword}”` : '输入英文或中文名称、完整路径搜索' }}
        </p>
        <div class="flex items-center gap-2">
          <el-button v-if="normalizedKeyword" link size="small" @click="clearSearch">清空搜索</el-button>
          <el-button v-if="modelValue" link type="danger" size="small" @click="clearSelection">清除已选</el-button>
          <p class="text-xs text-slate-400">父级与最末级类目均可选择</p>
        </div>
      </div>

      <p v-if="unavailableCategoryID" class="rounded-md bg-red-50 px-3 py-2 text-xs text-red-700">
        Category ID {{ unavailableCategoryID }} 已不存在，请重新选择
      </p>

      <div v-if="normalizedKeyword" v-loading="searchLoading" class="min-h-24 max-h-80 overflow-y-auto pr-1">
        <button
          v-for="category in results"
          :key="category.id"
          type="button"
          class="flex w-full items-start justify-between gap-3 rounded-lg px-3 py-2 text-left hover:bg-emerald-50"
          @click="selectCategory(category)"
        >
          <span class="min-w-0 space-y-0.5">
            <span class="block truncate text-sm font-semibold text-slate-800">{{ categoryTitle(category) }}</span>
            <span class="block truncate text-xs text-slate-600">{{ categoryZhPath(category) }}</span>
            <span class="block truncate text-xs text-slate-400">{{ categoryEnglishPath(category) }}</span>
          </span>
          <span class="shrink-0 text-right text-xs text-slate-400">
            <span class="block">ID {{ category.id }} · Level {{ category.level }}</span>
            <span class="block">{{ category.is_leaf ? '最末级类目' : 'Parent category' }}</span>
          </span>
        </button>
        <el-empty v-if="!searchLoading && !results.length" description="未找到匹配的平台类目" :image-size="72" />
      </div>
      <p v-else class="py-6 text-center text-sm text-slate-400">输入关键词后仅加载最多 50 条匹配结果</p>
    </div>
  </el-popover>
</template>
