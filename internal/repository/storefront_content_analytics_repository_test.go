package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"saas/internal/analytics"
	"saas/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type contentAnalyticsFixture struct {
	db        *gorm.DB
	repo      *StorefrontAnalyticsRepository
	primary   string
	deleted   string
	lowVolume string
	pages     string
	from      time.Time
	to        time.Time
}

func TestContentAnalyticsRepositoryListAggregatesOnlyMarketingContentEventsAndPaginatesInDB(t *testing.T) {
	fixture := newContentAnalyticsFixture(t)
	query := analytics.ContentAnalyticsQuery{
		Sort: analytics.ContentMetricClicks, Order: analytics.SortOrderDescending,
		Page: 1, PageSize: 1, Paginate: true,
	}
	items, total, err := fixture.repo.ListContentAnalytics(
		context.Background(), analytics.TimeWindow{FromUTC: fixture.from, ToUTC: fixture.to}, query,
	)
	if err != nil {
		t.Fatalf("ListContentAnalytics: %v", err)
	}
	if total != 4 || len(items) != 1 || items[0].TrackingKey != fixture.pages {
		t.Fatalf("page 1 total/items = %d/%+v", total, items)
	}
	query.Page, query.PageSize = 1, 20
	items, total, err = fixture.repo.ListContentAnalytics(
		context.Background(), analytics.TimeWindow{FromUTC: fixture.from, ToUTC: fixture.to}, query,
	)
	if err != nil || total != 4 || len(items) != 4 {
		t.Fatalf("all content aggregates = total %d items %+v err=%v", total, items, err)
	}
	byKey := make(map[string]analytics.ContentAggregate, len(items))
	for _, item := range items {
		byKey[item.TrackingKey] = item
	}
	primary := byKey[fixture.primary]
	if primary.Impressions != 3 || primary.Clicks != 2 || primary.UniqueVisitors != 2 {
		t.Fatalf("primary counts = %+v", primary)
	}
	if _, exists := byKey["product-event-only"]; exists {
		t.Fatalf("product event leaked into content analytics: %+v", byKey)
	}
	if _, exists := byKey["wrong-entity"]; exists {
		t.Fatalf("wrong entity type leaked into content analytics: %+v", byKey)
	}
	if _, exists := byKey["outside-boundary"]; exists {
		t.Fatalf("exclusive upper-bound event leaked into content analytics: %+v", byKey)
	}
}

func TestContentAnalyticsRepositoryExcludesBotTraffic(t *testing.T) {
	fixture := newContentAnalyticsFixture(t)
	botOnly := "55555555-5555-4555-8555-555555555555"
	createContentAnalyticsEvent(
		t, fixture.db, fixture.primary, models.StorefrontEventContentClick, "bot-visitor", "en",
		models.StorefrontDeviceDesktop, "/en", fixture.from.Add(10*time.Hour),
		models.StorefrontEntityMarketingContent, models.StorefrontTrafficBot,
	)
	createContentAnalyticsEvent(
		t, fixture.db, botOnly, models.StorefrontEventContentClick, "bot-only", "en",
		models.StorefrontDeviceDesktop, "/en", fixture.from.Add(12*time.Hour),
		models.StorefrontEntityMarketingContent, models.StorefrontTrafficBot,
	)
	createContentAnalyticsEvent(
		t, fixture.db, fixture.primary, models.StorefrontEventContentImpression, "bot-visitor", "en",
		models.StorefrontDeviceDesktop, "/en", fixture.from.Add(11*time.Hour),
		models.StorefrontEntityMarketingContent, models.StorefrontTrafficBot,
	)
	summary, err := fixture.repo.GetContentSummary(
		context.Background(), analytics.TimeWindow{FromUTC: fixture.from, ToUTC: fixture.to}, fixture.primary,
	)
	if err != nil {
		t.Fatalf("GetContentSummary: %v", err)
	}
	if summary.Impressions != 3 || summary.Clicks != 2 || summary.UniqueVisitors != 2 {
		t.Fatalf("human-only content summary = %+v", summary)
	}
	if everExisted, err := fixture.repo.ContentEverExisted(context.Background(), botOnly); err != nil || everExisted {
		t.Fatalf("bot-only ContentEverExisted = %v err=%v, want false", everExisted, err)
	}
}

func TestContentAnalyticsRepositoryCTRThresholdUnpaginatedAggregatePathAndSummary(t *testing.T) {
	fixture := newContentAnalyticsFixture(t)
	query := analytics.ContentAnalyticsQuery{
		Sort: analytics.ContentMetricCTR, Order: analytics.SortOrderAscending,
		Page: 1, PageSize: 20, MinImpressions: 2, Paginate: false,
	}
	items, total, err := fixture.repo.ListContentAnalytics(
		context.Background(), analytics.TimeWindow{FromUTC: fixture.from, ToUTC: fixture.to}, query,
	)
	if err != nil {
		t.Fatalf("ListContentAnalytics aggregate-row path: %v", err)
	}
	if total != int64(len(items)) || len(items) != 3 {
		t.Fatalf("threshold aggregate rows total/items = %d/%+v", total, items)
	}
	for _, item := range items {
		if item.Impressions < 2 || item.TrackingKey == fixture.lowVolume {
			t.Fatalf("CTR threshold failed: %+v", item)
		}
	}

	summary, err := fixture.repo.GetContentSummary(
		context.Background(), analytics.TimeWindow{FromUTC: fixture.from, ToUTC: fixture.to}, fixture.primary,
	)
	if err != nil || summary.Impressions != 3 || summary.Clicks != 2 || summary.UniqueVisitors != 2 {
		t.Fatalf("summary = %+v err=%v", summary, err)
	}
	missing, err := fixture.repo.GetContentSummary(
		context.Background(), analytics.TimeWindow{FromUTC: fixture.from, ToUTC: fixture.to}, "99999999-9999-4999-8999-999999999999",
	)
	if err != nil || missing != (analytics.ContentCounts{}) {
		t.Fatalf("zero summary = %+v err=%v", missing, err)
	}
	for key, want := range map[string]bool{fixture.deleted: true, "99999999-9999-4999-8999-999999999999": false} {
		got, err := fixture.repo.ContentEverExisted(context.Background(), key)
		if err != nil || got != want {
			t.Fatalf("ContentEverExisted(%s) = %v err=%v, want %v", key, got, err, want)
		}
	}
}

func TestContentAnalyticsRepositoryTrendLocaleDeviceAndPageBreakdowns(t *testing.T) {
	fixture := newContentAnalyticsFixture(t)
	dateRange, err := analytics.ParseDateRange(analytics.DateRangeInput{
		From: "2026-07-01", To: "2026-07-03", Timezone: "Africa/Accra",
	}, time.Time{}, analytics.MaxWeeklyRangeDays)
	if err != nil {
		t.Fatalf("ParseDateRange: %v", err)
	}
	trend, err := fixture.repo.GetContentTrend(context.Background(), dateRange, analytics.GranularityDay, fixture.primary)
	if err != nil || len(trend) != 2 || trend[0].Bucket != "2026-07-01" || trend[0].UniqueVisitors != 1 || trend[1].Bucket != "2026-07-03" || trend[1].UniqueVisitors != 1 {
		t.Fatalf("trend = %+v err=%v", trend, err)
	}

	window := analytics.TimeWindow{FromUTC: fixture.from, ToUTC: fixture.to}
	locales, err := fixture.repo.GetContentLocaleBreakdown(context.Background(), window, fixture.primary)
	if err != nil {
		t.Fatalf("GetContentLocaleBreakdown: %v", err)
	}
	localeMap := make(map[string]analytics.ContentLocaleStat, len(locales))
	for _, stat := range locales {
		localeMap[stat.Locale] = stat
	}
	if en := localeMap["en"]; en.Impressions != 1 || en.Clicks != 1 || en.UniqueVisitors != 1 {
		t.Fatalf("en locale = %+v", en)
	}
	if zh := localeMap["zh"]; zh.Impressions != 1 || zh.Clicks != 1 || zh.UniqueVisitors != 2 {
		t.Fatalf("zh locale = %+v", zh)
	}
	if other := localeMap["other"]; other.Impressions != 1 || other.Clicks != 0 || other.UniqueVisitors != 1 {
		t.Fatalf("other locale = %+v", other)
	}

	devices, err := fixture.repo.GetContentDeviceBreakdown(context.Background(), window, fixture.primary)
	if err != nil {
		t.Fatalf("GetContentDeviceBreakdown: %v", err)
	}
	deviceMap := make(map[string]analytics.ContentDeviceStat, len(devices))
	for _, stat := range devices {
		deviceMap[stat.DeviceType] = stat
	}
	if desktop := deviceMap["desktop"]; desktop.Impressions != 1 || desktop.Clicks != 1 {
		t.Fatalf("desktop device = %+v", desktop)
	}
	if mobile := deviceMap["mobile"]; mobile.Impressions != 1 || mobile.Clicks != 0 {
		t.Fatalf("mobile device = %+v", mobile)
	}
	if unknown := deviceMap["unknown"]; unknown.Impressions != 1 || unknown.Clicks != 1 {
		t.Fatalf("unknown device = %+v", unknown)
	}

	pages, err := fixture.repo.GetContentPageBreakdown(context.Background(), window, fixture.pages)
	if err != nil || len(pages) != analytics.ContentPageBreakdownLimit {
		t.Fatalf("page breakdown len=%d err=%v: %+v", len(pages), err, pages)
	}
	if pages[0].Clicks < pages[len(pages)-1].Clicks {
		t.Fatalf("pages are not click-desc sorted: %+v", pages)
	}
	for _, page := range pages {
		if page.PagePath == "" {
			t.Fatalf("empty page path was not normalized: %+v", pages)
		}
	}
}

func TestContentAnalyticsPostgreSQLFinalQueryContractsAndNoRawEventSlice(t *testing.T) {
	dateRange, err := analytics.ParseDateRange(analytics.DateRangeInput{
		From: "2026-07-01", To: "2026-07-14", Timezone: "Asia/Shanghai",
	}, time.Time{}, analytics.MaxWeeklyRangeDays)
	if err != nil {
		t.Fatalf("ParseDateRange: %v", err)
	}
	built, err := buildContentAnalyticsListQueries("postgres", dateRange.Window(), analytics.ContentAnalyticsQuery{
		Sort: analytics.ContentMetricCTR, Order: analytics.SortOrderAscending,
		Page: 2, PageSize: 20, MinImpressions: 100, Paginate: true,
	})
	if err != nil {
		t.Fatalf("buildContentAnalyticsListQueries: %v", err)
	}
	for name, finalQuery := range map[string]storefrontAnalyticsQuery{"items": built.Items, "total": *built.Total} {
		normalizedSQL := strings.ToLower(strings.Join(strings.Fields(finalQuery.SQL), " "))
		for _, fragment := range []string{
			"e.occurred_at >= ?", "e.occurred_at < ?", "e.event_name in (?,?)",
			"e.entity_type = ?", "e.entity_id is not null", "e.traffic_type = ?", "content_aggregates as",
			"group by entity_id", "count(distinct visitor_id)", "filter (where",
		} {
			if !strings.Contains(strings.ReplaceAll(normalizedSQL, "?, ?", "?,?"), fragment) {
				t.Fatalf("%s missing %q: %s", name, fragment, finalQuery.SQL)
			}
		}
		if strings.Contains(normalizedSQL, "tenant_id") {
			t.Fatalf("content all-platform query unexpectedly accepted tenant scope: %s", finalQuery.SQL)
		}
		if strings.Count(finalQuery.SQL, "?") != len(finalQuery.Args) || finalQuery.Args[0] != dateRange.FromUTC || finalQuery.Args[1] != dateRange.ToUTC ||
			finalQuery.Args[2+len(analytics.ContentEventAllowlist)] != models.StorefrontTrafficHuman {
			t.Fatalf("%s placeholder/final args = %d/%#v", name, strings.Count(finalQuery.SQL, "?"), finalQuery.Args)
		}
		for _, eventName := range analytics.ContentEventAllowlist {
			if strings.Contains(finalQuery.SQL, "'"+string(eventName)+"'") {
				t.Fatalf("%s interpolated event name %q: %s", name, eventName, finalQuery.SQL)
			}
		}
	}
	if !strings.Contains(built.Items.SQL, "WHERE impressions >= ?") || !strings.Contains(built.Items.SQL, "ORDER BY ctr ASC, tracking_key ASC") || !strings.Contains(built.Items.SQL, "LIMIT ? OFFSET ?") {
		t.Fatalf("CTR threshold/sort/pagination contract = %s", built.Items.SQL)
	}
	if built.Total == nil || !strings.Contains(built.Total.SQL, "SELECT COUNT(*) FROM scored_content") {
		t.Fatalf("DB total contract = %+v", built.Total)
	}
	aggregateOnly, err := buildContentAnalyticsListQueries("postgres", dateRange.Window(), analytics.ContentAnalyticsQuery{
		Sort: analytics.ContentMetricClicks, Order: analytics.SortOrderDescending,
		Page: 1, PageSize: 20, Paginate: false,
	})
	if err != nil || aggregateOnly.Total != nil || strings.Contains(aggregateOnly.Items.SQL, "LIMIT ?") {
		t.Fatalf("metadata-filter aggregate-row contract = %+v err=%v", aggregateOnly, err)
	}
	if _, err := buildContentAnalyticsListQueries("postgres", dateRange.Window(), analytics.ContentAnalyticsQuery{
		Sort: analytics.ContentMetric("clicks; DROP TABLE storefront_events"), Order: analytics.SortOrderDescending, Page: 1, PageSize: 20, Paginate: true,
	}); err == nil {
		t.Fatal("unsafe content sort was accepted")
	}
	if _, err := buildContentAnalyticsListQueries("postgres", dateRange.Window(), analytics.ContentAnalyticsQuery{
		Sort: analytics.ContentMetricClicks, Order: analytics.SortOrder("DESC NULLS FIRST"), Page: 1, PageSize: 20, Paginate: true,
	}); err == nil {
		t.Fatal("unsafe content order was accepted")
	}

	for granularity, fragment := range map[analytics.Granularity]string{
		analytics.GranularityHour: "date_trunc('hour'",
		analytics.GranularityDay:  "date_trunc('day'",
		analytics.GranularityWeek: "date_trunc('week'",
	} {
		trend, err := buildContentAnalyticsTrendQuery("postgres", dateRange, granularity, "11111111-1111-4111-8111-111111111111")
		if err != nil || !strings.Contains(trend.SQL, fragment) || !strings.Contains(trend.SQL, "AT TIME ZONE ?") || !strings.Contains(trend.SQL, "e.entity_id = ?") {
			t.Fatalf("trend contract %q = %s err=%v", granularity, trend.SQL, err)
		}
	}
	if _, err := buildContentAnalyticsTrendQuery("postgres", dateRange, analytics.Granularity("month'); DROP TABLE storefront_events; --"), "key"); err == nil {
		t.Fatal("unsafe content granularity was accepted")
	}
	localeQuery, _ := buildContentLocaleBreakdownQuery("postgres", dateRange.Window(), "key")
	deviceQuery, _ := buildContentDeviceBreakdownQuery("postgres", dateRange.Window(), "key")
	pageQuery, _ := buildContentPageBreakdownQuery("postgres", dateRange.Window(), "key")
	if !strings.Contains(localeQuery.SQL, "GROUP BY dimension_value") || !strings.Contains(deviceQuery.SQL, "GROUP BY dimension_value") ||
		!strings.Contains(pageQuery.SQL, "GROUP BY page_path") || !strings.Contains(pageQuery.SQL, "LIMIT ?") || strings.Contains(pageQuery.SQL, "key") {
		t.Fatalf("breakdown contracts locale=%s device=%s page=%s", localeQuery.SQL, deviceQuery.SQL, pageQuery.SQL)
	}
	assertNoStorefrontEventSlices(t, "storefront_content_analytics_repository.go")
}

func newContentAnalyticsFixture(t *testing.T) contentAnalyticsFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	if err := db.AutoMigrate(&models.StorefrontEvent{}); err != nil {
		t.Fatalf("AutoMigrate content analytics: %v", err)
	}
	fixture := contentAnalyticsFixture{
		db:        db,
		repo:      NewStorefrontAnalyticsRepository(db),
		primary:   "11111111-1111-4111-8111-111111111111",
		deleted:   "22222222-2222-4222-8222-222222222222",
		lowVolume: "33333333-3333-4333-8333-333333333333",
		pages:     "44444444-4444-4444-8444-444444444444",
		from:      time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		to:        time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC),
	}
	createContentAnalyticsEvent(t, db, fixture.primary, models.StorefrontEventContentImpression, "v1", "en", models.StorefrontDeviceDesktop, "/en", fixture.from.Add(time.Hour), models.StorefrontEntityMarketingContent)
	createContentAnalyticsEvent(t, db, fixture.primary, models.StorefrontEventContentImpression, "v1", "zh", models.StorefrontDeviceMobile, "/zh", fixture.from.Add(2*time.Hour), models.StorefrontEntityMarketingContent)
	createContentAnalyticsEvent(t, db, fixture.primary, models.StorefrontEventContentImpression, "v2", "fr", models.StorefrontDeviceType("smart_tv"), "", fixture.from.AddDate(0, 0, 2).Add(time.Hour), models.StorefrontEntityMarketingContent)
	createContentAnalyticsEvent(t, db, fixture.primary, models.StorefrontEventContentClick, "v1", "en", models.StorefrontDeviceDesktop, "/en", fixture.from.Add(3*time.Hour), models.StorefrontEntityMarketingContent)
	createContentAnalyticsEvent(t, db, fixture.primary, models.StorefrontEventContentClick, "v2", "zh", models.StorefrontDeviceType("smart_tv"), "/zh", fixture.from.AddDate(0, 0, 2).Add(2*time.Hour), models.StorefrontEntityMarketingContent)

	createContentAnalyticsEvent(t, db, fixture.deleted, models.StorefrontEventContentImpression, "v3", "en", models.StorefrontDeviceMobile, "/en/deleted", fixture.from.Add(4*time.Hour), models.StorefrontEntityMarketingContent)
	createContentAnalyticsEvent(t, db, fixture.deleted, models.StorefrontEventContentImpression, "v4", "zh", models.StorefrontDeviceMobile, "/zh/deleted", fixture.from.Add(5*time.Hour), models.StorefrontEntityMarketingContent)
	createContentAnalyticsEvent(t, db, fixture.lowVolume, models.StorefrontEventContentImpression, "v5", "en", models.StorefrontDeviceDesktop, "/en/low", fixture.from.Add(6*time.Hour), models.StorefrontEntityMarketingContent)
	createContentAnalyticsEvent(t, db, fixture.lowVolume, models.StorefrontEventContentClick, "v5", "en", models.StorefrontDeviceDesktop, "/en/low", fixture.from.Add(7*time.Hour), models.StorefrontEntityMarketingContent)

	for index := 0; index < 25; index++ {
		pagePath := fmt.Sprintf("/en/page-%02d", index)
		createContentAnalyticsEvent(t, db, fixture.pages, models.StorefrontEventContentImpression, fmt.Sprintf("p-%d", index), "en", models.StorefrontDeviceDesktop, pagePath, fixture.from.Add(time.Duration(index+8)*time.Minute), models.StorefrontEntityMarketingContent)
		for click := 0; click < index%4; click++ {
			createContentAnalyticsEvent(t, db, fixture.pages, models.StorefrontEventContentClick, fmt.Sprintf("p-%d", index), "en", models.StorefrontDeviceDesktop, pagePath, fixture.from.Add(time.Duration(index+click+40)*time.Minute), models.StorefrontEntityMarketingContent)
		}
	}

	createContentAnalyticsEvent(t, db, "product-event-only", models.StorefrontEventProductClick, "ignored", "en", models.StorefrontDeviceDesktop, "/en", fixture.from.Add(8*time.Hour), models.StorefrontEntityMarketingContent)
	createContentAnalyticsEvent(t, db, "page-event-only", models.StorefrontEventPageView, "ignored", "en", models.StorefrontDeviceDesktop, "/en", fixture.from.Add(8*time.Hour), models.StorefrontEntityMarketingContent)
	createContentAnalyticsEvent(t, db, "wrong-entity", models.StorefrontEventContentClick, "ignored", "en", models.StorefrontDeviceDesktop, "/en", fixture.from.Add(9*time.Hour), models.StorefrontEntityProduct)
	createContentAnalyticsEvent(t, db, "outside-boundary", models.StorefrontEventContentClick, "ignored", "en", models.StorefrontDeviceDesktop, "/en", fixture.to, models.StorefrontEntityMarketingContent)
	return fixture
}

func createContentAnalyticsEvent(
	t *testing.T,
	db *gorm.DB,
	trackingKey string,
	eventName models.StorefrontEventName,
	visitor string,
	locale string,
	device models.StorefrontDeviceType,
	pagePath string,
	occurredAt time.Time,
	entityType models.StorefrontEntityType,
	trafficTypes ...models.StorefrontTrafficType,
) {
	t.Helper()
	entityID := trackingKey
	sourceID := trackingKey
	event := models.StorefrontEvent{
		EventName: eventName, VisitorID: visitor, SessionID: "session-" + visitor,
		EntityType: entityType, EntityID: &entityID,
		SourceType: models.StorefrontSourceMarketingContent, SourceID: &sourceID,
		PageType: models.StorefrontPageHome, PagePath: pagePath, Locale: locale, DeviceType: device,
		OccurredAt: occurredAt, CreatedAt: occurredAt,
	}
	if len(trafficTypes) > 0 {
		event.TrafficType = trafficTypes[0]
	}
	if err := db.Create(&event).Error; err != nil {
		t.Fatalf("create content event %s/%s: %v", trackingKey, eventName, err)
	}
}
