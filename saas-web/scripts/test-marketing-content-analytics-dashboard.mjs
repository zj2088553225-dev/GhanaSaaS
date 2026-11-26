import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

function source(path) {
  return readFileSync(new URL(path, import.meta.url), 'utf8')
}

const api = source('../src/api/analytics.ts')
const router = source('../src/router/index.ts')
const navigation = source('../src/components/analytics/AnalyticsNavigation.vue')
const list = source('../src/views/admin/analytics/AnalyticsContent.vue')
const detail = source('../src/views/admin/analytics/AnalyticsContentDetail.vue')
const format = source('../src/utils/analyticsFormat.ts')
const image = source('../src/components/ProductImage.vue')

// 1-4. Content has two super-only routes, no admin analytics endpoint or tenant
// selector, and the tab is hidden from tenant admins.
assert.match(router, /name: 'admin-analytics-content'[\s\S]*meta: \{ roles: \['super_admin'\] \}/)
assert.match(router, /name: 'admin-analytics-content-detail'[\s\S]*meta: \{ roles: \['super_admin'\] \}/)
assert.match(router, /Array\.isArray\(routeRoles\)[\s\S]*includes\(adminRole\)/)
assert.match(navigation, /v-if="isSuperAdmin" label="内容分析"/)
assert.match(api, /superOnly && !usesSuperAnalyticsEndpoint\(\)/)
assert.doesNotMatch(list + detail, /TenantAnalyticsFilter|tenant_id/)

// 5-9. The list exposes the fixed resolver types, CMS/current-state filters,
// server pagination, quick image-performance sorting and CTR threshold.
for (const type of [
  'hero', 'hero_banner', 'hero_slide', 'promo_banner', 'image', 'image_text',
  'contact', 'category_item', 'feature_grid', 'link', 'other',
]) assert.match(list, new RegExp(`'${type}'`))
assert.match(list, /currently_exists:/)
assert.match(list, /page: page\.value[\s\S]*page_size: pageSize\.value/)
for (const preset of ['点击最高', '曝光最高', 'CTR 最高', 'CTR 最低']) assert.match(list, new RegExp(preset))
assert.match(list, /minImpressions = ref\([^)]*100/)
assert.match(list, /min_impressions: usesCTRThreshold\.value \? minImpressions\.value/)

// 10-13. Images, English title/tracking key, current/deleted and published/draft
// states remain visible; unsafe HTML/script execution is absent.
assert.match(list, /<ProductImage/)
assert.match(image, /@error="handleError"/)
assert.match(list, /scope\.row\.tracking_key/)
assert.match(list, /'已删除内容'/)
assert.match(list, /scope\.row\.published \? '已发布' : '草稿'/)
assert.doesNotMatch(list + detail, /v-html|window\.open|innerHTML|eval\(/)

// 14-19. Detail reuses trend and shared metrics, renders all three aggregate
// breakdowns, and preserves deleted/draft historical semantics.
assert.match(detail, /getContentAnalyticsDetail/)
assert.match(detail, /<AnalyticsTrendChart/)
assert.match(detail, /detail\.locales/)
assert.match(detail, /detail\.devices/)
assert.match(detail, /detail\.pages/)
assert.match(detail, /出现页面 TOP 20/)
assert.match(detail, /该内容当前已删除，以下为历史行为数据。/)
assert.match(detail, /当前仅存在于草稿，未发布。/)
assert.match(detail, /trackingKeyPattern/)

// 20-23. Locale merging and anonymous/bucket semantics are explicit, raw event
// identity/referrer fields never appear, and requests are stale-safe/no fake data.
assert.match(list + detail, /中英文内容通过同一个 tracking key 合并统计，语言维度仅作为行为拆分。/)
assert.match(detail, /各时间桶的独立访客不可相加为总独立访客/)
assert.doesNotMatch(list + detail, /visitor_id|session_id|referrer|原始事件 JSON/)
assert.match(list, /requestController\?\.abort\(\)/)
assert.match(detail, /requestController\?\.abort\(\)/)
assert.match(list + detail, /sequence === requestSequence/)
assert.doesNotMatch(list + detail, /mock|fake|Math\.random/)

// 24. Shared labels are fixed allowlists rather than raw unbounded values.
assert.match(format, /hero_slide: '轮播图'/)
assert.match(format, /promo_banner: '推广 Banner'/)
assert.match(format, /feature_grid: '功能宫格'/)

console.log('marketing content analytics dashboard contract tests passed')
