import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

function source(path) {
  return readFileSync(new URL(path, import.meta.url), 'utf8')
}

const mall = source('../src/views/MallProductList.vue')
const en = source('../src/locales/en.ts')
const zh = source('../src/locales/zh.ts')
const storefrontServiceTest = source('../../internal/service/storefront_service_test.go')

assert.match(mall, /const expandedPlatformCategoryId = ref<number \| null>\(null\)/, 'Mall should track one expanded root category')
assert.match(mall, /rootCategoryOptions = computed\(\(\) => sortedCategories\(platformCategories\.value\)\)/, 'Mall sidebar should render root categories by default')
assert.match(mall, /function toggleCategoryExpansion\(category: PlatformCategory\)/, 'Mall should expose category accordion toggle')
assert.match(mall, /expandedPlatformCategoryId\.value = isCategoryExpanded\(category\) \? null : category\.id/, 'Accordion toggle should collapse current or open the clicked root')
assert.match(mall, /function collapseAllCategories\(\)/, 'Mall should expose collapse all behavior')
assert.match(mall, /@click="collapseAllCategories"/, 'Mall should wire the collapse all button')
assert.match(mall, /selectedRootPlatformCategoryId = computed/, 'Mall should derive selected root category from the URL category')
assert.match(mall, /watch\(\s*\[\s*selectedRootPlatformCategoryId,\s*\(\) => platformCategories\.value\]/, 'Mall should auto-expand the selected root after route or category data changes')
assert.match(mall, /expandedPlatformCategoryId\.value = rootCategoryId \?\? null/, 'Mall should collapse when no category is selected')

assert.match(mall, /v-for="category in rootCategoryOptions"/, 'Sidebar should list root categories')
assert.match(mall, /v-if="isCategoryExpanded\(category\)"/, 'Second-level category drawer should render only when expanded')
assert.match(mall, /max-h-56 overflow-y-auto/, 'Second-level category drawer should have internal scrolling')
assert.match(mall, /@click="selectCategory\(category\.id\)"/, 'Clicking a root category name should filter by that parent category')
assert.match(mall, /@click="selectCategory\(child\.id\)"/, 'Clicking a child category should filter by that second-level category')
assert.match(mall, /:class="isCategoryActive\(category\)/, 'Root category should be highlighted when selected')
assert.match(mall, /:class="isCategoryActive\(child\)/, 'Child category should be highlighted when selected')
assert.match(mall, /getStorefrontPlatformCategories\(\{\s*maxLevel:\s*MALL_CATEGORY_MAX_LEVEL\s*\}\)/, 'Mall should keep max_level=2 category loading')

assert.match(mall, /\{\{ category\.name \}\}/, 'Mall should render localized category.name returned by the storefront API')
assert.match(mall, /\{\{ child\.name \}\}/, 'Mall should render localized child.name returned by the storefront API')
assert.doesNotMatch(mall, /zh_name|zh_path/, 'Mall should not bypass storefront locale content with admin bilingual fields')
assert.match(en, /collapseAll: 'Collapse All'/, 'English locale should include collapse all text')
assert.match(zh, /collapseAll: '收起全部'/, 'Chinese locale should include collapse all text')

assert.match(storefrontServiceTest, /TestStorefrontServiceProductsFilterBySecondLevelIncludesDeepDescendantsAndSource/, 'Backend tests should cover parent category filtering into descendants')
assert.match(storefrontServiceTest, /PlatformCategoryID: &secondLevelID/, 'Parent category filter test should use a second-level category')
assert.match(storefrontServiceTest, /Source:\s+"t6"[\s\S]*Source:\s+"merchant"/, 'Parent category filter test should cover source=t6 and source=merchant')

console.log('mall category accordion tests passed')
