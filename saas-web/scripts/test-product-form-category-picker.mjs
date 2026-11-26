import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

function source(path) {
  return readFileSync(new URL(path, import.meta.url), 'utf8')
}

const productForm = source('../src/views/admin/products/ProductForm.vue')
const api = source('../src/api/platformCategories.ts')
const controller = source('../../internal/controller/platform_category_controller.go')

assert.doesNotMatch(productForm, /getAdminPlatformCategoryChildren/, 'ProductForm should not load the platform category tree')
assert.doesNotMatch(productForm, /<el-tree/, 'ProductForm should not render a deep platform category tree')
assert.doesNotMatch(productForm, /loadPlatformCategoryNode/, 'ProductForm should not lazy-expand category tree nodes')

assert.match(productForm, /searchAdminPlatformCategories\(keyword,\s*\{[\s\S]*limit:\s*50,[\s\S]*locale:\s*'all',[\s\S]*leafOnly:\s*false,[\s\S]*signal:\s*controller\.signal/, 'ProductForm should use cancellable admin server-side bilingual category search')
assert.match(productForm, /搜索英文\/中文类目名称或完整路径，选择最末级类目/, 'ProductForm should show Chinese bilingual search placeholder')
assert.match(productForm, /Search English\/Chinese category name or full path, choose the most specific category/, 'ProductForm should include English search placeholder')
assert.match(productForm, /category\.zh_name/, 'ProductForm should render zh_name')
assert.match(productForm, /category\.zh_path/, 'ProductForm should render zh_path')
assert.match(productForm, /platformCategoryEnglishPathLabel/, 'ProductForm should render English path')
assert.match(productForm, /if \(!category\.is_leaf\)/, 'ProductForm should reject non-leaf selection')
assert.match(productForm, /parentPlatformCategorySearchResults/, 'ProductForm should separate non-leaf search results')
assert.match(productForm, /disabled[\s\S]*请选择最末级类目/, 'ProductForm should show disabled parent category rows')
assert.match(productForm, /getAdminPlatformCategoryById\(product\.platform_category_id\)/, 'ProductForm edit mode should load selected category by ID')
assert.match(productForm, /@compositionstart="handlePlatformCategoryCompositionStart"/, 'ProductForm should pause during IME composition')
assert.match(productForm, /@compositionend="handlePlatformCategoryCompositionEnd"/, 'ProductForm should search committed IME input')
assert.match(productForm, /platformCategorySearchAbortController\?\.abort\(\)/, 'ProductForm should cancel obsolete search requests')

assert.match(api, /\/admin\/platform-categories\/search/, 'admin platform category search API should exist')
assert.match(api, /buildPlatformCategorySearchParams/, 'admin platform category search API should build leaf_only parameters centrally')
assert.match(controller, /RegisterAdminRoutes[\s\S]*\/admin\/platform-categories\/search/, 'admin platform category search route should be registered')
assert.match(controller, /platformCategoryTruthyQuery\(c\.Query\("leaf_only"\)\)/, 'backend search should handle leaf_only')

console.log('product form category picker tests passed')
