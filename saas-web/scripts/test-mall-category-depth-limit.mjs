import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

function source(path) {
  return readFileSync(new URL(path, import.meta.url), 'utf8')
}

const mall = source('../src/views/MallProductList.vue')
const api = source('../src/api/platformCategories.ts')
const platformHome = source('../src/views/PlatformHome.vue')
const homeCategoryNav = source('../src/components/storefront/HomeCategoryNav.vue')
const homeBlockRenderer = source('../src/components/site-blocks/HomeBlockRenderer.vue')
const controller = source('../../internal/controller/platform_category_controller.go')
const service = source('../../internal/service/platform_category_service.go')
const repository = source('../../internal/repository/platform_category_repository.go')
const storefrontService = source('../../internal/service/storefront_service.go')
const storefrontRepository = source('../../internal/repository/storefront_repository.go')

assert.match(mall, /const MALL_CATEGORY_MAX_LEVEL = 2/, 'MallProductList should define a 2-level category limit')
assert.match(mall, /getStorefrontPlatformCategories\(\{\s*maxLevel:\s*MALL_CATEGORY_MAX_LEVEL\s*\}\)/, 'MallProductList should request max_level=2 categories')
assert.match(mall, /depth >= MALL_CATEGORY_MAX_LEVEL - 1/, 'MallProductList should not flatten level 3+ categories')

assert.match(api, /getStorefrontPlatformCategories\(options: PlatformCategoryListOptions = \{\}\)/, 'storefront category list API should accept options')
assert.match(api, /max_level: options\.maxLevel/, 'storefront category list API should send max_level')
assert.match(api, /\$\{currentStorefrontLocale\(\)\}:\$\{parentId \?\? 0\}:\$\{options\.maxLevel \?\? 'all'\}/, 'storefront category children cache key should include maxLevel')

assert.match(platformHome, /getStorefrontPlatformCategoryChildren\(null,\s*\{\s*maxLevel:\s*2\s*\}\)/, 'PlatformHome root category nav should request max_level=2')
assert.match(homeCategoryNav, /getStorefrontPlatformCategoryChildren\(category\.id,\s*\{\s*maxLevel:\s*2\s*\}\)/, 'HomeCategoryNav lazy children should request max_level=2')
assert.match(homeBlockRenderer, /getStorefrontPlatformCategoryChildren\(block\.platform_category_id \|\| null,\s*\{\s*maxLevel:\s*2\s*\}\)/, 'HomeBlockRenderer dynamic category nav should request max_level=2')

assert.match(controller, /parsePlatformCategoryMaxLevelQuery/, 'public platform category controller should parse max_level')
assert.match(controller, /ListWithLocaleMaxLevel/, 'public platform category controller should call max-level service list')
assert.match(service, /ListWithLocaleMaxLevel/, 'platform category service should expose max-level localized list')
assert.match(repository, /ListByMaxLevel/, 'platform category repository should query by max level')
assert.match(repository, /Where\("level <= \?", maxLevel\)/, 'platform category repository should filter by level')

assert.match(storefrontService, /applyPlatformCategoryScope/, 'storefront service should apply platform category scope')
assert.match(storefrontService, /ListPlatformCategoryIDsByRoot/, 'storefront service should expand parent platform category IDs')
assert.match(storefrontRepository, /Where\("id = \? OR path LIKE \?", id, root\.Path\+"\/%"\)/, 'storefront repository should include descendant platform categories')

console.log('mall category depth limit tests passed')
