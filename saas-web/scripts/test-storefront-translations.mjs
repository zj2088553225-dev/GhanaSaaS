import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const storefrontApi = readFileSync(new URL('../src/api/storefront.ts', import.meta.url), 'utf8')
const requestSource = readFileSync(new URL('../src/utils/request.ts', import.meta.url), 'utf8')
const storefrontService = readFileSync(new URL('../../internal/service/storefront_service.go', import.meta.url), 'utf8')
const translationHelpers = readFileSync(new URL('../../internal/service/storefront_translation_helpers.go', import.meta.url), 'utf8')
const contentTranslationService = readFileSync(new URL('../../internal/service/content_translation_service.go', import.meta.url), 'utf8')
const platformCategoryService = readFileSync(new URL('../../internal/service/platform_category_service.go', import.meta.url), 'utf8')

assert.match(contentTranslationService, /TranslationEntityProduct\s*=\s*"product"/)
assert.match(contentTranslationService, /TranslationEntityShopProfile\s*=\s*"shop_profile"/)
assert.match(contentTranslationService, /TranslationEntityCategory\s*=\s*"category"/)
assert.match(contentTranslationService, /TranslationEntityPlatformCategory\s*=\s*"platform_category"/)

assert.match(storefrontService, /Translations\s+TranslationReader/)
assert.match(storefrontService, /applyProductListTranslations/)
assert.match(storefrontService, /applyProductDetailTranslations/)
assert.match(storefrontService, /applyShopProfileTranslations/)
assert.match(translationHelpers, /GetTranslations\(ctx,\s*TranslationEntityProduct/)
assert.match(translationHelpers, /GetTranslations\(ctx,\s*TranslationEntityShopProfile/)
assert.match(translationHelpers, /GetTranslations\(ctx,\s*TranslationEntityCategory/)
assert.match(translationHelpers, /localizePlatformCategories/)
assert.match(platformCategoryService, /TranslationEntityPlatformCategory/)

assert.match(storefrontApi, /description:\s*string/)
assert.match(storefrontApi, /display_description:\s*string/)
assert.match(storefrontApi, /description\?:\s*unknown/)
assert.match(requestSource, /attachStorefrontLocale\(config\)/)

console.log('storefront translation tests passed')
