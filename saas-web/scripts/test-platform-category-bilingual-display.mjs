import { readFileSync } from 'node:fs'
import assert from 'node:assert/strict'

const api = readFileSync(new URL('../src/api/platformCategories.ts', import.meta.url), 'utf8')
const adminView = readFileSync(new URL('../src/views/admin/PlatformCategories.vue', import.meta.url), 'utf8')
const controller = readFileSync(new URL('../../internal/controller/platform_category_controller.go', import.meta.url), 'utf8')
const service = readFileSync(new URL('../../internal/service/platform_category_service.go', import.meta.url), 'utf8')
const repository = readFileSync(new URL('../../internal/repository/platform_category_repository.go', import.meta.url), 'utf8')
const model = readFileSync(new URL('../../internal/models/platform_category.go', import.meta.url), 'utf8')

assert.match(model, /ZhName\s+string\s+`gorm:"-" json:"zh_name,omitempty"`/)
assert.match(model, /ZhPath\s+string\s+`gorm:"-" json:"zh_path,omitempty"`/)

assert.match(controller, /ZhName\s+string\s+`json:"zh_name"`/)
assert.match(controller, /ZhPath\s+string\s+`json:"zh_path"`/)
assert.match(controller, /platformCategoryResponseZhName/)
assert.match(controller, /platformCategoryResponseZhPath/)

assert.match(service, /AttachBilingualDisplay\(ctx,\s*categories\)/)
assert.match(service, /repo\.Search\(ctx,\s*query\.Normalized,\s*limit,\s*activeOnly,\s*requestedLocale\)/)

assert.match(repository, /func \(r \*PlatformCategoryRepository\) AttachBilingualDisplay/)
assert.match(repository, /platform_category_search_indexes|PlatformCategorySearchIndex/)
assert.match(repository, /category_id IN \? AND locale = \?/)
assert.match(repository, /platformCategoryBilingualDisplayFallback/)
assert.match(repository, /platformCategoryZhNameTranslations/)

assert.match(api, /zh_name\?: string/)
assert.match(api, /zh_path\?: string/)

assert.match(adminView, /function zhName\(category: PlatformCategory\)/)
assert.match(adminView, /function zhPath\(category: PlatformCategory\)/)
assert.match(adminView, /function englishPath\(category: PlatformCategory\)/)
assert.match(adminView, /{{ zhName\(row\) }}/)
assert.match(adminView, /{{ zhPath\(row\) }}/)
assert.match(adminView, /{{ englishPath\(row\) }}/)
assert.match(adminView, /未填写中文/)
assert.match(adminView, /未填写中文路径/)
assert.match(adminView, /searchSuperPlatformCategories\(keyword,\s*\{[\s\S]*limit:\s*100,[\s\S]*locale:\s*'all'/)
assert.match(adminView, /rebuildPlatformCategorySearchIndex/)
assert.match(adminView, /exportBilingualTable/)
assert.match(adminView, /confirmPlatformCategoryImport/)

console.log('platform category bilingual display tests passed')
