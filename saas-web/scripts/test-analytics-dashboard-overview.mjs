import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

function source(path) {
  return readFileSync(new URL(path, import.meta.url), 'utf8')
}

const api = source('../src/api/analytics.ts')
const router = source('../src/router/index.ts')
const layout = source('../src/layouts/AdminLayout.vue')
const overview = source('../src/views/admin/analytics/AnalyticsOverview.vue')
const navigation = source('../src/components/analytics/AnalyticsNavigation.vue')
const dateFilter = source('../src/components/analytics/AnalyticsDateRangeFilter.vue')
const tenantFilter = source('../src/components/analytics/TenantAnalyticsFilter.vue')
const chart = source('../src/components/analytics/AnalyticsTrendChart.vue')
const format = source('../src/utils/analyticsFormat.ts')

// 1-3. Analytics is a first-level menu and both roles can enter the overview;
// content remains a super-only tab.
assert.match(layout, /adminPath\('\/analytics'\), label: '数据分析'/)
assert.match(router, /super_admin:[\s\S]*adminPath\('\/analytics'\)/)
assert.match(router, /tenant_admin:[^\n]*adminPath\('\/analytics'\)/)
assert.match(router, /name: 'admin-analytics-overview'/)
assert.match(navigation, /v-if="isSuperAdmin" label="内容分析"/)

// 4-6. The API chooses scope from the current role, never sends tenant scope for
// an admin, fixes business timezone, and overview always requests compare.
assert.match(api, /getStoredUser\(\)\?\.role === 'super_admin'/)
assert.match(api, /'\/super\/analytics' : '\/admin\/analytics'/)
assert.match(api, /key === 'tenant_id' && \(!allowTenant \|\| !usesSuperAnalyticsEndpoint\(\)\)/)
assert.match(api, /ANALYTICS_TIMEZONE = 'Africa\/Accra'/)
assert.match(api, /getAnalyticsOverview[\s\S]*compare: true/)
assert.match(dateFilter, /GMT \/ Accra Time/)

// 7-11. Operations see the accumulation boundary, all ten canonical metrics,
// New semantics, 0-1 rate formatting, and no order-conversion claim.
assert.match(overview, /数据自行为分析功能上线后开始累计，不包含上线前历史访问。/)
for (const metric of [
  'visitors', 'sessions', 'page_views', 'product_impressions', 'product_clicks',
  'product_views', 'contact_intents', 'product_ctr', 'detail_view_rate', 'contact_intent_rate',
]) {
  assert.match(overview, new RegExp(`key: '${metric}'`))
}
assert.match(format, /return 'New'/)
assert.match(format, /rate \* 100/)
assert.doesNotMatch(overview, /订单转化率|转化率/)

// 12-13. ECharts has a real lifecycle and a bounded metric selector.
assert.match(chart, /new ResizeObserver/)
assert.match(chart, /resizeObserver\?\.disconnect\(\)/)
assert.match(chart, /chart\?\.dispose\(\)/)
assert.match(chart, /if \(!chart\)/)
assert.match(chart, /:max="5"/)
for (const metric of ['页面浏览', '商品曝光', '商品点击', '商品详情访问', '联系意向']) {
  assert.match(overview, new RegExp(metric))
}

// 14-16. Source names and both existing product ranking APIs are reused.
assert.match(format, /home_product_floor: '首页商品楼层'/)
assert.match(format, /marketing_content: '营销内容'/)
assert.match(overview, /getTopProducts/)
assert.match(overview, /metric: 'clicks'/)
assert.match(overview, /getLowCTRProducts/)
assert.match(overview, /min_impressions: 100/)

// 17-20. Empty data is honest, stale requests are aborted/sequence guarded,
// and tenant controls are rendered only for super admins.
assert.match(overview, /AnalyticsEmptyState/)
assert.doesNotMatch(overview, /mock|fake|Math\.random/)
assert.match(overview, /requestSequence/)
assert.match(overview, /requestController\?\.abort\(\)/)
assert.match(overview, /sequence === requestSequence/)
assert.match(overview, /<TenantAnalyticsFilter[\s\S]*v-if="isSuperAdmin"/)
assert.match(tenantFilter, /getSuperTenants\(\)/)
assert.match(tenantFilter, /slice\(0, 50\)/)

console.log('analytics overview dashboard contract tests passed')
