import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { homeFloorDiagnosticKey } from '../src/components/site-blocks/homeFloorDiagnostics.ts'

const editor = readFileSync(new URL('../src/views/admin/SitePageHomeEditor.vue', import.meta.url), 'utf8')
const settings = readFileSync(new URL('../src/components/site-blocks/editor/BlockSettings.vue', import.meta.url), 'utf8')
const preview = readFileSync(new URL('../src/components/site-blocks/HomeBlockPreview.vue', import.meta.url), 'utf8')
const renderer = readFileSync(new URL('../src/components/site-blocks/HomeBlockRenderer.vue', import.meta.url), 'utf8')

assert.equal(homeFloorDiagnosticKey({ type: 'product_floor', platform_category_id: 42, source: 't6' }), '42:t6')
assert.equal(homeFloorDiagnosticKey({ type: 'product_floor', platform_category_id: 42 }), '42:all')
assert.equal(homeFloorDiagnosticKey({ type: 'product_floor' }), null)

assert.match(editor, /getAdminPlatformCategoryById\(Number\(block\.platform_category_id\)\)/)
assert.match(editor, /getProducts\(\{[\s\S]*platform_category_id:\s*category\.id[\s\S]*source:\s*productSourceParam\(block\.source\)/)
assert.match(editor, /categoryPath:\s*category\.path/)
assert.match(settings, /绑定的平台类目已不存在，请重新选择/)
assert.match(settings, /type="error"/)
assert.match(preview, /当前类目暂无可展示商品/)
assert.match(renderer, /import\.meta\.env\.DEV/)
assert.match(renderer, /console\.warn\('\[HomeBlockRenderer\] Product block request failed'/)
assert.match(renderer, /platformCategoryID:\s*block\.platform_category_id/)

console.log('home floor category validation tests passed')
