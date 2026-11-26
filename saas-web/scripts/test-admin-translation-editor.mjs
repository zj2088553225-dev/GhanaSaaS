import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const productForm = readFileSync(new URL('../src/views/admin/products/ProductForm.vue', import.meta.url), 'utf8')
const shopProfile = readFileSync(new URL('../src/views/admin/ShopProfile.vue', import.meta.url), 'utf8')
const platformCategories = readFileSync(new URL('../src/views/admin/PlatformCategories.vue', import.meta.url), 'utf8')
const adminProductsApi = readFileSync(new URL('../src/api/adminProducts.ts', import.meta.url), 'utf8')
const shopProfileApi = readFileSync(new URL('../src/api/adminShopProfile.ts', import.meta.url), 'utf8')
const platformCategoriesApi = readFileSync(new URL('../src/api/platformCategories.ts', import.meta.url), 'utf8')
const adminProductController = readFileSync(new URL('../../internal/controller/admin_product_controller.go', import.meta.url), 'utf8')
const shopProfileController = readFileSync(new URL('../../internal/controller/shop_profile_controller.go', import.meta.url), 'utf8')
const platformCategoryController = readFileSync(new URL('../../internal/controller/platform_category_controller.go', import.meta.url), 'utf8')
const contentTranslationService = readFileSync(new URL('../../internal/service/content_translation_service.go', import.meta.url), 'utf8')

for (const source of [productForm, shopProfile, platformCategories]) {
  assert.match(source, /adminTranslations\.english/)
  assert.match(source, /adminTranslations\.chinese/)
  assert.match(source, /translations:\s*\{\s*zh:/)
}

assert.match(productForm, /translationForm\.detail_html/)
assert.match(productForm, /RichTextDetailEditor v-model="translationForm\.detail_html"/)
assert.match(productForm, /sanitizeProductDetailHtml\(translationForm\.detail_html\)/)
assert.match(shopProfile, /translationForm\.shop_name/)
assert.match(shopProfile, /translationForm\.business_hours/)
assert.match(platformCategories, /zh_name/)

assert.match(adminProductsApi, /AdminProductTranslations/)
assert.match(shopProfileApi, /ShopProfileTranslations/)
assert.match(platformCategoriesApi, /PlatformCategoryTranslations/)

assert.match(adminProductController, /Translations\s+\*adminProductTranslationsPayload/)
assert.match(shopProfileController, /Translations\s+\*shopProfileTranslationsPayload/)
assert.match(platformCategoryController, /Translations\s+\*platformCategoryTranslationsPayload/)
assert.match(contentTranslationService, /SaveTranslationsForEntity/)
assert.match(contentTranslationService, /Delete\(ctx,\s*entityType,\s*entityID,\s*fieldKey,\s*requestedLocale\)/)
assert.match(contentTranslationService, /allowedTranslationFields/)

console.log('admin translation editor tests passed')
