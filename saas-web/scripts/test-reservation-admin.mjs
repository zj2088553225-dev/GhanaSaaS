import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const [router, layout, list, form, analytics, records, api] = await Promise.all([
  readFile(new URL('../src/router/index.ts', import.meta.url), 'utf8'),
  readFile(new URL('../src/layouts/AdminLayout.vue', import.meta.url), 'utf8'),
  readFile(new URL('../src/views/admin/reservations/ReservationProductList.vue', import.meta.url), 'utf8'),
  readFile(new URL('../src/views/admin/reservations/ReservationProductForm.vue', import.meta.url), 'utf8'),
  readFile(new URL('../src/views/admin/reservations/ReservationProductAnalytics.vue', import.meta.url), 'utf8'),
  readFile(new URL('../src/views/admin/reservations/ReservationRecordList.vue', import.meta.url), 'utf8'),
  readFile(new URL('../src/api/reservations.ts', import.meta.url), 'utf8'),
])

assert.match(router, /adminPath\('\/reservation-products'\)/)
const reservationRoutes = router.match(/path: 'reservation-products[\s\S]*?path: 'dashboard'/)?.[0] || ''
assert.ok(reservationRoutes)
assert.equal((reservationRoutes.match(/roles: \['super_admin'\]/g) || []).length, 5)
assert.doesNotMatch(router.match(/tenant_admin: \[([^\n]+)/)?.[1] || '', /reservation-products/)
assert.match(layout, /label: '预约商品'/)
assert.match(layout, /label: '预约记录'/)
assert.match(router, /adminPath\('\/reservations'\)/)
assert.match(layout, /role === 'super_admin'/)

assert.match(list, /exact_interest_count/)
assert.match(list, /exact_verified_count/)
assert.match(list, /automatic_heat_level/)
assert.match(list, /effective_heat_level/)
assert.match(list, /archiveSuperReservationProduct/)
assert.match(list, /历史预约与统计会保留/)

assert.match(form, /createSuperReservationProduct/)
assert.match(form, /updateSuperReservationProduct/)
assert.match(form, /linked_product_id/)
assert.match(form, /value="draft"/)
assert.match(form, /value="scheduled"/)
assert.match(form, /value="open"/)
assert.match(form, /value="closed"/)
assert.match(form, /value="archived"/)
assert.match(form, /heat_level_override/)
assert.match(form, /RichTextDetailEditor v-model="form\.description_en"/)
assert.match(form, /RichTextDetailEditor v-model="form\.description_zh"/)
assert.match(form, /admin\/uploads\/image/)
assert.match(form, /:limit="12"/)
assert.match(form, /v-model="previewOpen"/)
assert.match(form, /sanitizeProductDetailHtml/)
assert.doesNotMatch(form, /retail_price|sellable_stock|shopping|checkout/)

for (const metric of [
  'page_views', 'unique_interests', 'reservation_submissions', 'verified_reservations',
  'verification_rate', 'cancelled_reservations', 'blocked_attempts',
]) {
  assert.match(analytics, new RegExp(`analytics(?:\\.value)?\\.${metric}`))
  assert.match(api, new RegExp(`${metric}: number`))
}
assert.match(analytics, /masked_phone/)
assert.match(analytics, /不提供明文号码或 CSV 导出/)
assert.doesNotMatch(analytics, /contact_encrypted|contact_hash|provider_reference|ip_risk_key/)
assert.match(analytics, /既有 Storefront Analytics 指标定义保持不变/)
assert.match(records, /browser_hash_prefix/)
assert.match(records, /ip_hash_prefix/)
assert.match(records, /rate_limit_flagged/)
assert.match(records, /setSuperReservationStatus/)
assert.match(records, /setSuperReservationRisk/)
assert.doesNotMatch(records, /raw_ip|cookie_token|browser_hash(?!_prefix)|ip_hash(?!_prefix)/)

console.log('reservation super admin behavior tests passed')
