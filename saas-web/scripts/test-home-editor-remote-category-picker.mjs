import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

function source(path) {
  return readFileSync(new URL(path, import.meta.url), 'utf8')
}

const settings = source('../src/components/site-blocks/editor/BlockSettings.vue')
const picker = source('../src/components/admin/AdminPlatformCategoryRemotePicker.vue')
const productForm = source('../src/views/admin/products/ProductForm.vue')
const editor = source('../src/views/admin/SitePageHomeEditor.vue')
const controller = source('../../internal/controller/platform_category_controller.go')

assert.doesNotMatch(settings, /getAdminPlatformCategories/, 'BlockSettings must not request the complete category list')
assert.doesNotMatch(settings, /onMounted/, 'BlockSettings must not load categories when mounted')
assert.doesNotMatch(settings, /<el-tree-select|platformCategories|platformCategoryTreeProps/, 'BlockSettings must not render or retain a complete category tree')
assert.equal((settings.match(/<AdminPlatformCategoryRemotePicker/g) || []).length, 3, 'all homepage category fields must use the remote picker')

assert.match(picker, /searchAdminPlatformCategories\(searchKeyword,\s*\{[\s\S]*limit:\s*CATEGORY_SEARCH_LIMIT,[\s\S]*locale:\s*'all',[\s\S]*leafOnly:\s*false/, 'picker must use bilingual server-side search without leaf-only filtering')
assert.match(picker, /const CATEGORY_SEARCH_LIMIT = 50/, 'homepage category search must request at most 50 results')
assert.match(picker, /!shouldSchedulePlatformCategorySearch\(value, isComposing\.value\)[\s\S]*results\.value = \[\][\s\S]*return/, 'an empty or composing keyword must not execute category search')
assert.match(picker, /const CATEGORY_SEARCH_DEBOUNCE_MS = 250/, 'category search must be debounced by 250ms')
assert.match(picker, /isCurrentPlatformCategorySearch\(sequence, searchSequence, searchKeyword, keyword\.value\)/, 'stale search responses must not replace current results')
assert.match(picker, /onBeforeUnmount\(\(\) => \{[\s\S]*clearTimeout\(searchTimer\)[\s\S]*searchSequence \+= 1/, 'unmount must clear the timer and invalidate pending searches')
assert.match(picker, /ElMessage\.error\(error instanceof Error \? error\.message : '平台类目搜索失败'\)/, 'search failures must show an explicit message')
assert.match(picker, /@compositionstart="handleCompositionStart"/, 'IME composition must pause remote search')
assert.match(picker, /@compositionend="handleCompositionEnd"/, 'committed IME input must restart remote search')
assert.match(picker, /searchAbortController\?\.abort\(\)/, 'obsolete remote searches must be cancelled')

assert.match(picker, /watch\([\s\S]*\(\) => props\.modelValue[\s\S]*loadSelectedCategory\(categoryID\)[\s\S]*immediate:\s*true/, 'saved category IDs must be loaded individually')
assert.match(picker, /getAdminPlatformCategoryById\(categoryID\)/, 'ID display must use the admin get-by-ID endpoint')
assert.match(picker, /SELECTED_CATEGORY_CACHE_LIMIT = 50/, 'ID display cache must remain bounded')
assert.match(picker, /unavailableCategoryID\.value = categoryID/, 'a missing category must retain its ID for diagnosis')
assert.doesNotMatch(picker, /emit\('update:modelValue', undefined\)[\s\S]*unavailableCategoryID\.value = categoryID/, 'a failed ID lookup must not automatically clear the saved ID')

assert.match(picker, /emit\('update:modelValue', category\.id\)/, 'selection must save the category ID')
assert.match(picker, /emit\('update:modelPath', category\.path\)/, 'selection must save the English main-table path')
assert.match(picker, /父级与最末级类目均可选择/, 'homepage category picker must allow parent categories')
assert.doesNotMatch(picker, /if \(!category\.is_leaf\)/, 'homepage category selection must not reject parent categories')
assert.doesNotMatch(picker, /<el-tree|children/, 'remote results must not create a category tree DOM')

assert.match(productForm, /if \(!category\.is_leaf\)/, 'ProductForm must continue to reject parent categories')
assert.match(productForm, /仅最末级类目可选/, 'ProductForm must continue to explain its leaf-only rule')

assert.match(settings, /绑定的平台类目已不存在，请重新选择/, 'missing-category floor warning must remain')
assert.match(editor, /platform_category_id:\s*block\.platform_category_id,\s*source:\s*block\.source \|\| 'all'/, 'floor diagnosis must react to category and source')
assert.match(editor, /const sequence = \+\+floorValidationSequence[\s\S]*if \(sequence !== floorValidationSequence\)/, 'stale floor diagnostics must not overwrite a newer selection')
assert.match(editor, /structure-readonly/, 'the Chinese CMS tab must not edit category structure fields')
assert.match(editor, /const translatableCmsFields = new Set\(\['title',[\s\S]*'address'\]\)/, 'Chinese CMS content must remain restricted to text fields')

const simulatedCategoryCount = 30_000
assert.equal(simulatedCategoryCount > CATEGORY_RESULT_BOUND_FROM_SOURCE(picker), true)
assert.equal(CATEGORY_RESULT_BOUND_FROM_SOURCE(picker), 50, 'initial data size and result DOM must stay independent of the 30,000-category dataset')

function CATEGORY_RESULT_BOUND_FROM_SOURCE(componentSource) {
  const match = componentSource.match(/const CATEGORY_SEARCH_LIMIT = (\d+)/)
  assert.ok(match, 'category search limit constant must exist')
  return Number(match[1])
}

assert.match(controller, /if limit > service\.PlatformCategorySearchMaxLimit \{[\s\S]*limit = service\.PlatformCategorySearchMaxLimit/, 'backend must enforce its category search limit')

console.log('home editor remote category picker tests passed')
