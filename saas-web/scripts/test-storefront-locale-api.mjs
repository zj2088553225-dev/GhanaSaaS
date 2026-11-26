import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { storefrontLocaleParam } from '../src/utils/localePath.ts'

const requestSource = readFileSync(new URL('../src/utils/request.ts', import.meta.url), 'utf8')
const platformCategoriesSource = readFileSync(new URL('../src/api/platformCategories.ts', import.meta.url), 'utf8')

assert.deepEqual(storefrontLocaleParam({ params: { locale: 'zh' }, path: '/zh/mall' }), { locale: 'zh' })
assert.deepEqual(storefrontLocaleParam({ params: { locale: 'en' }, path: '/en/products/apple-iphone-12-14' }), { locale: 'en' })
assert.deepEqual(storefrontLocaleParam({ params: {}, path: '/mall' }), { locale: 'en' })

assert.match(requestSource, /localeFromPath/)
assert.match(requestSource, /function isPublicStorefrontAPIRequest/)
assert.match(requestSource, /requestPath\(url\)\.includes\('\/api\/v1\/storefront'\)/)
assert.match(requestSource, /attachStorefrontLocale\(config\)/)
assert.match(requestSource, /config\.params = \{ \.\.\.params, locale \}/)
assert.match(requestSource, /function isAdminRequest/)
assert.match(requestSource, /path\.includes\('\/api\/v1\/admin'\)/)
assert.match(requestSource, /path\.includes\('\/api\/v1\/super'\)/)

assert.match(platformCategoriesSource, /currentStorefrontLocale/)
assert.match(platformCategoriesSource, /\$\{currentStorefrontLocale\(\)\}:\$\{parentId \?\? 0\}:\$\{options\.maxLevel \?\? 'all'\}/)
assert.match(platformCategoriesSource, /max_level: options\.maxLevel/)

console.log('storefront locale API tests passed')
