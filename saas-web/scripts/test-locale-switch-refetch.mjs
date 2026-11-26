import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

function source(path) {
  return readFileSync(new URL(path, import.meta.url), 'utf8')
}

function assertContains(file, pattern, message) {
  assert.match(source(file), pattern, message)
}

function assertRefetchesOnLocale(file, loaderName, apiPattern) {
  const content = source(file)
  assert.match(content, /activeLocale\s*=\s*computed\(\(\)\s*=>\s*currentLocale\(route\)\)/, `${file} should derive activeLocale from the route`)
  assert.match(content, new RegExp(`watch\\(\\s*activeLocale[\\s\\S]*${loaderName}\\(`), `${file} should watch activeLocale and call ${loaderName}`)
  assert.match(content, apiPattern, `${file} should still call the expected storefront API`)
}

assertRefetchesOnLocale('../src/views/ProductDetail.vue', 'loadProduct', /getProductDetail\(/)
assertContains('../src/views/ProductDetail.vue', /let\s+loadSequence\s*=\s*0/, 'ProductDetail should guard stale locale-switch requests')
assertContains('../src/views/ProductDetail.vue', /replaceWithCanonicalProductPath\(detail\)/, 'ProductDetail should keep canonical SEO path replacement after reload')

assertRefetchesOnLocale('../src/views/MallProductList.vue', 'loadProducts', /getProducts\(/)
assertContains('../src/views/MallProductList.vue', /watch\(\s*activeLocale[\s\S]*loadPlatformCategories\(\)[\s\S]*loadProducts\(\)/, 'MallProductList should refetch categories and products on locale switch')
assertContains('../src/views/MallProductList.vue', /routeQuery\(\{ source,\s*page:\s*1 \}\)/, 'MallProductList should keep source query handling when switching source tabs')

assertRefetchesOnLocale('../src/views/PlatformHome.vue', 'reloadPageData', /getPublishedHomeSitePage\(/)
assertContains('../src/views/PlatformHome.vue', /watch\(\s*activeLocale[\s\S]*reloadPageData\(\)/, 'PlatformHome should reload homepage CMS on locale switch')
assertContains('../src/views/PlatformHome.vue', /getStorefrontPlatformCategoryChildren\([^)]*\)/, 'PlatformHome should reload localized root categories')

assertRefetchesOnLocale('../src/views/ShopList.vue', 'loadShops', /getPublicShops\(/)

assertRefetchesOnLocale('../src/views/ShopHome.vue', 'loadShop', /getPublicShopProfile\(/)
assertContains('../src/views/ShopHome.vue', /getPublicShopProducts\(/, 'ShopHome should still reload shop products')
assertContains('../src/views/ShopHome.vue', /watch\(slug,[\s\S]*query\.page\s*=\s*1[\s\S]*loadShop\(\)/, 'ShopHome should preserve slug watch behavior')

const blockRenderer = source('../src/components/site-blocks/HomeBlockRenderer.vue')
assert.match(blockRenderer, /activeLocale\s*=\s*computed\(\(\)\s*=>\s*currentLocale\(route\)\)/, 'HomeBlockRenderer should derive activeLocale from route')
assert.match(blockRenderer, /watch\(\s*\[\(\)\s*=>\s*props\.blocks,\s*\(\)\s*=>\s*activeLocale\.value\]/, 'HomeBlockRenderer should reload dynamic blocks when locale changes')
assert.match(blockRenderer, /const\s+locale\s*=\s*activeLocale\.value/, 'HomeBlockRenderer should capture active locale for requests')
assert.match(blockRenderer, /JSON\.stringify\(\{\s*locale,\s*params\s*\}\)/, 'HomeBlockRenderer product request cache key should include locale')
assert.match(blockRenderer, /getPublicShops\(/, 'HomeBlockRenderer should still load shop_grid data')
assert.match(blockRenderer, /getStorefrontPlatformCategoryChildren\(/, 'HomeBlockRenderer should still load dynamic category_nav data')

const homeCategoryNav = source('../src/components/storefront/HomeCategoryNav.vue')
assert.match(homeCategoryNav, /watch\(\s*activeLocale[\s\S]*childrenByParent\.value\s*=\s*\{\}/, 'HomeCategoryNav should clear lazy children cache on locale switch')
assert.match(homeCategoryNav, /withLocalePath\(activeLocale\.value,\s*`\/mall\?platform_category_id=\$\{category\.id\}`\)/, 'HomeCategoryNav category links should use active locale')

assertRefetchesOnLocale('../src/views/Cart.vue', 'hydrateCart', /getProductDetail\(/)
assertRefetchesOnLocale('../src/views/Checkout.vue', 'hydrateProducts', /getProductDetail\(/)

const request = source('../src/utils/request.ts')
assert.match(request, /function\s+attachStorefrontLocale/, 'request interceptor should attach storefront locale')
assert.match(request, /localeFromPath\(window\.location\.pathname\)/, 'request interceptor should read locale from current URL')
assert.match(request, /config\.params\s*=\s*\{\s*\.\.\.params,\s*locale\s*\}/, 'request interceptor should add locale query parameter')
assert.doesNotMatch(request, /isAdminRequest[\s\S]*attachStorefrontLocale\(config\)[\s\S]*\/api\/v1\/super/, 'admin requests should not be localized by storefront locale helper')

console.log('locale switch refetch tests passed')
