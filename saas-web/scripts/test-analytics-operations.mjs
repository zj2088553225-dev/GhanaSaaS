import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const api = readFileSync(new URL('../src/api/analytics.ts', import.meta.url), 'utf8')
const productView = readFileSync(new URL('../src/views/admin/analytics/AnalyticsProducts.vue', import.meta.url), 'utf8')
const contentView = readFileSync(new URL('../src/views/admin/analytics/AnalyticsContent.vue', import.meta.url), 'utf8')
const overviewView = readFileSync(new URL('../src/views/admin/analytics/AnalyticsOverview.vue', import.meta.url), 'utf8')
const downloader = readFileSync(new URL('../src/utils/analyticsDownload.ts', import.meta.url), 'utf8')
const exportService = readFileSync(new URL('../../internal/service/storefront_analytics_export_service.go', import.meta.url), 'utf8')
const statusRepository = readFileSync(new URL('../../internal/repository/storefront_analytics_status_repository.go', import.meta.url), 'utf8')
const productController = readFileSync(new URL('../../internal/controller/storefront_product_analytics_controller.go', import.meta.url), 'utf8')
const contentController = readFileSync(new URL('../../internal/controller/storefront_content_analytics_controller.go', import.meta.url), 'utf8')
const analyticsController = readFileSync(new URL('../../internal/controller/storefront_analytics_controller.go', import.meta.url), 'utf8')
const operationsDocs = readFileSync(new URL('../../docs/storefront-analytics-operations.md', import.meta.url), 'utf8')

// CSV APIs reuse role-scoped authenticated analytics endpoints and current filters.
assert.match(api, /analyticsCSVExport[\s\S]*request\.get<Blob>/)
assert.match(api, /responseType:\s*'blob'/)
assert.match(api, /'\/products\/export'/)
assert.match(api, /'\/content\/export'/)
assert.match(api, /exportContentAnalytics[\s\S]*true,/)
assert.match(api, /cleanAnalyticsParams\(params, !superOnly\)/)
assert.match(api, /\.\.\.dateParams\(query\)/)
assert.doesNotMatch(api.slice(api.indexOf('async function analyticsCSVExport')), /token[=:]/i)
assert.match(productController, /group\.GET\("\/products\/export"/)
assert.match(contentController, /group\.GET\("\/content\/export"/)

for (const [source, label] of [[productView, 'product'], [contentView, 'content']]) {
  assert.match(source, /导出 CSV/)
  assert.match(source, /if \(exportLoading\.value\) return/)
  assert.match(source, /:loading="exportLoading"/)
  assert.match(source, /downloadAnalyticsCSV\(download\)/)
  assert.match(source, /search:/)
  assert.match(source, /sort:/)
  assert.match(source, /order:/)
  assert.doesNotMatch(source, /visitor_id|session_id|referrer/, `${label} export UI must not expose event identities`)
}
assert.match(productView, /tenant_id:\s*isSuperAdmin\.value/)
assert.match(productView, /status:\s*status\.value/)
assert.match(contentView, /content_type:\s*contentType\.value/)
assert.match(contentView, /currently_exists:/)
assert.match(downloader, /URL\.createObjectURL\(download\.blob\)/)
assert.match(downloader, /link\.download = download\.filename/)
assert.match(downloader, /URL\.revokeObjectURL\(url\)/)

// Backend export is bounded, Excel-friendly, RFC CSV encoded, and formula-safe.
assert.match(exportService, /AnalyticsExportMaxRows\s*=\s*10_000/)
assert.match(exportService, /AnalyticsExportMaxRows \+ 1/)
assert.match(exportService, /ErrAnalyticsExportTooLarge/)
assert.match(exportService, /encoding\/csv/)
assert.match(exportService, /0xEF, 0xBB, 0xBF/)
assert.match(exportService, /case '=', '\+', '-', '@':/)
assert.match(exportService, /"Product ID"[\s\S]*"Contact Intent Rate"/)
assert.match(exportService, /"Tracking Key"[\s\S]*"Unique Visitors"[\s\S]*"CTR"/)
assert.doesNotMatch(exportService, /visitor_id|session_id|referrer/)

// Status is aggregate-only, super-only, with an honest empty/stale UI.
assert.match(analyticsController, /analyticsGroup\.GET\("\/status", ctl\.SuperStatus\)/)
assert.match(api, /getAnalyticsStatus[\s\S]*'\/status'[\s\S]*true/)
assert.match(overviewView, /v-if="isSuperAdmin"[\s\S]*数据状态/)
assert.match(overviewView, /尚未收到商城行为事件/)
assert.match(overviewView, /最近 24 小时未收到新的商城行为事件，请检查前台埋点或访问情况。/)
assert.match(overviewView, /Date\.now\(\) - latest > 24 \* 60 \* 60 \* 1000/)
for (const field of ['total_events', 'human_events', 'bot_events', 'events_last_24h', 'oldest_event_at', 'latest_event_at']) {
  assert.match(overviewView, new RegExp(field))
}
assert.match(statusRepository, /COUNT\(\*\) AS total_events/)
assert.match(statusRepository, /traffic_type = \?/)
assert.match(statusRepository, /occurred_at >= \? AND occurred_at < \?/)
assert.match(statusRepository, /Group\("event_name"\)/)
assert.match(statusRepository, /EventsByName/)
assert.doesNotMatch(statusRepository, /visitor_id|session_id|referrer/)

// Retention remains manual and no premature daily aggregate is introduced.
assert.match(operationsDocs, /retained by default/i)
assert.match(operationsDocs, /does not[\s\S]*automatic purge/i)
assert.match(operationsDocs, /HIGH RISK/)
assert.match(operationsDocs, /permanently remove/i)
assert.match(operationsDocs, /EXPLAIN \(ANALYZE, BUFFERS\)/)
assert.match(operationsDocs, /Daily aggregation is explicitly not\s+implemented/i)
assert.match(operationsDocs, /hour\/date, tenant, event name,[\s\S]*locale, and device/i)
assert.doesNotMatch(exportService + statusRepository, /DELETE FROM storefront_events/i)

console.log('analytics exports and data operations contract tests passed')
