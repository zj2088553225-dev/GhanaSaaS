import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

function source(path) {
  return readFileSync(new URL(path, import.meta.url), 'utf8')
}

const api = source('../src/api/analytics.ts')
const router = source('../src/router/index.ts')
const navigation = source('../src/components/analytics/AnalyticsNavigation.vue')
const list = source('../src/views/admin/analytics/AnalyticsProducts.vue')
const detail = source('../src/views/admin/analytics/AnalyticsProductDetail.vue')
const format = source('../src/utils/analyticsFormat.ts')

// 1-3. Both permitted roles use one frontend route while API scope follows the
// active role; only super admins can render/send a tenant filter.
assert.match(router, /name: 'admin-analytics-products'/)
assert.match(router, /name: 'admin-analytics-product-detail'/)
assert.match(navigation, /admin-analytics-products/)
assert.match(api, /'\/super\/analytics' : '\/admin\/analytics'/)
assert.match(api, /key === 'tenant_id' && \(!allowTenant \|\| !usesSuperAnalyticsEndpoint\(\)\)/)
assert.match(list, /<TenantAnalyticsFilter v-if="isSuperAdmin"/)

// 4-7. List paging, sorting, search and the real status enum are sent to the
// backend; no all-row client pagination exists.
assert.match(list, /getProductAnalytics\([\s\S]*page: page\.value[\s\S]*page_size: pageSize\.value/)
assert.match(list, /sort: sort\.value[\s\S]*order: order\.value/)
assert.match(list, /search: search\.value\.trim\(\)/)
for (const status of ['on_shelf', 'off_shelf', 'deleted']) assert.match(list, new RegExp(status))
assert.match(list, /list\?\.total \|\| 0/)
assert.doesNotMatch(list, /\.slice\(.*page|clientPag/i)

// 8-11. Deleted/off-shelf metadata and every canonical metric/rate are rendered
// through shared safe formatters.
assert.match(list, /Deleted product #\$\{product\.product_id\}/)
assert.match(list, /!product\.currently_exists/)
assert.match(list, /product\.status === 'on_shelf' \? '上架' : '下架'/)
for (const metric of [
  'impressions', 'clicks', 'views', 'contact_intents', 'unique_visitors',
  'ctr', 'detail_view_rate', 'contact_intent_rate',
]) assert.match(list, new RegExp(metric))
assert.match(format, /Number\.isFinite/)

// 12-14. Existing ranking APIs are reused, with an adjustable minimum exposure
// for low CTR and no frontend metric recomputation.
assert.match(list, /getTopProducts/)
assert.match(list, /getLowCTRProducts/)
assert.match(list, /lowMinImpressions = ref\(100\)/)
assert.match(list, /min_impressions: lowMinImpressions\.value/)
assert.match(list, /metric: topMetric\.value/)

// 15-18. Detail preserves URL context, reuses the trend component, presents the
// event-context source table and states the attribution boundary verbatim.
assert.match(list, /name: 'admin-analytics-product-detail'/)
assert.match(detail, /getProductAnalyticsDetail/)
assert.match(detail, /<AnalyticsTrendChart/)
assert.match(detail, /detail\.sources/)
assert.match(detail, /来源数据表示事件发生时的页面上下文，不代表严格转化归因。/)
assert.doesNotMatch(detail, /转化来源|带来.*转化|conversion source/i)

// 19-23. URL filter state is carried into detail/back navigation and stale
// responses are canceled/sequence guarded without fabricated data.
for (const key of ['from', 'to', 'tenant_id', 'search', 'status', 'sort', 'order', 'page', 'page_size']) {
  assert.match(list + detail, new RegExp(key))
}
assert.match(detail, /name: 'admin-analytics-products', query: preservedQuery\(\)/)
assert.match(list, /listController\?\.abort\(\)/)
assert.match(list, /sequence === listSequence/)
assert.match(detail, /requestController\?\.abort\(\)/)
assert.match(detail, /sequence === requestSequence/)
assert.doesNotMatch(list + detail, /mock|fake|Math\.random/)

console.log('product analytics dashboard contract tests passed')
