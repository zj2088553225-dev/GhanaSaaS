import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const api = readFileSync(new URL('../src/api/platformCategories.ts', import.meta.url), 'utf8')
const adminView = readFileSync(new URL('../src/views/admin/PlatformCategories.vue', import.meta.url), 'utf8')
const controller = readFileSync(new URL('../../internal/controller/platform_category_controller.go', import.meta.url), 'utf8')
const importService = readFileSync(new URL('../../internal/service/platform_category_import.go', import.meta.url), 'utf8')
const repository = readFileSync(new URL('../../internal/repository/platform_category_repository.go', import.meta.url), 'utf8')
const translationRepository = readFileSync(new URL('../../internal/repository/content_translation_repository.go', import.meta.url), 'utf8')
const webNginx = readFileSync(new URL('../nginx.conf', import.meta.url), 'utf8')
const hostNginx = readFileSync(new URL('../../deploy/nginx-host.conf', import.meta.url), 'utf8')

assert.match(importService, /"English Path",\s*"中文路径",\s*"Sort Order",\s*"Status"/)
assert.match(importService, /"Category ID",\s*"English Path",\s*"中文路径",\s*"Sort Order",\s*"Status"/)
assert.match(importService, /中文路径层级数量必须和 English Path 一致/)
assert.match(importService, /TranslationEntityPlatformCategory/)
assert.match(importService, /ZhProvided/)
assert.match(importService, /ExportBilingualImportWorkbook/)
assert.doesNotMatch(importService, /SaveTranslationsForEntity\(/, 'import service must not save translations outside the repository transaction')

assert.match(translationRepository, /func \(r \*ContentTranslationRepository\) BatchUpsert/)
assert.match(translationRepository, /CreateInBatches\(rows,\s*500\)/)

assert.match(repository, /ID\s+\*uint/)
assert.match(repository, /ZhName\s+string/)
assert.match(repository, /platformCategoryImportExistingCategory/)
assert.match(repository, /func \(r \*PlatformCategoryRepository\) Import[\s\S]*Transaction\(func\(tx \*gorm\.DB\) error/)
assert.match(repository, /savePlatformCategoryImportTranslations\(tx,/)
assert.match(repository, /rebuildPlatformCategorySearchIndex\(tx,\s*nil\)/)
assert.match(repository, /substr\(path, length\(\?\) \+ 1\)/)

assert.match(controller, /GET\("\/import-template",\s*ctl\.BilingualImportTemplate\)/)
assert.match(controller, /POST\("\/import",\s*ctl\.ImportDirect\)/)
assert.match(controller, /GET\("\/export",\s*ctl\.ExportBilingual\)/)

assert.match(api, /\/super\/platform-categories\/import-template/)
assert.match(api, /\/super\/platform-categories\/import'/)
assert.match(api, /\/super\/platform-categories\/export/)
assert.match(api, /exportPlatformCategoryBilingualTable/)
assert.match(api, /PLATFORM_CATEGORY_IMPORT_TIMEOUT_MS\s*=\s*180000/)
assert.match(api, /timeout:\s*PLATFORM_CATEGORY_IMPORT_TIMEOUT_MS/)

assert.match(adminView, /下载双语类目模板/)
assert.match(adminView, /导入双语类目 Excel/)
assert.match(adminView, /导出当前双语表/)
assert.match(adminView, /Category ID 可用于原地迁移历史类目/)
assert.match(adminView, /导入处理时间较长，请稍后刷新类目列表确认结果；请勿重复提交。/)
assert.match(adminView, /:disabled="!importPreview \|\| hasImportErrors \|\| importConfirming \|\| importPreviewLoading"/)
assert.match(adminView, /:show-close="!importConfirming"/)

for (const [name, nginx] of [['web', webNginx], ['host', hostNginx]]) {
  assert.match(nginx, /proxy_connect_timeout\s+300s;/, `${name} nginx should extend API connect timeout`)
  assert.match(nginx, /proxy_send_timeout\s+300s;/, `${name} nginx should extend API send timeout`)
  assert.match(nginx, /proxy_read_timeout\s+300s;/, `${name} nginx should extend API read timeout`)
  assert.match(nginx, /send_timeout\s+300s;/, `${name} nginx should extend response send timeout`)
}

console.log('platform category bilingual import tests passed')
