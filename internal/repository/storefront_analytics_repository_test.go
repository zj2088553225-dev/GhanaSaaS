package repository

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
	"time"

	"saas/internal/analytics"
	"saas/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestStorefrontAnalyticsMetricsUseDistinctIdentityCountsAndScope(t *testing.T) {
	db := newAnalyticsRepositoryTestDB(t)
	tenantA := createAnalyticsTenant(t, db, "Tenant A")
	tenantB := createAnalyticsTenant(t, db, "Tenant B")
	from := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)

	createAnalyticsEvent(t, db, nil, models.StorefrontEventPageView, "platform-visitor", "platform-session", models.StorefrontSourceDirect, from.Add(time.Hour))
	createAnalyticsEvent(t, db, &tenantA.ID, models.StorefrontEventPageView, "visitor-a", "session-a", models.StorefrontSourceDirect, from)
	createAnalyticsEvent(t, db, &tenantA.ID, models.StorefrontEventProductImpression, "visitor-a", "session-a", models.StorefrontSourceMall, from.Add(2*time.Hour))
	createAnalyticsEvent(t, db, &tenantA.ID, models.StorefrontEventProductImpression, "visitor-a", "session-a", models.StorefrontSourceMall, from.Add(3*time.Hour))
	createAnalyticsEvent(t, db, &tenantA.ID, models.StorefrontEventProductClick, "visitor-a", "session-a", models.StorefrontSourceMall, from.Add(4*time.Hour))
	createAnalyticsEvent(t, db, &tenantA.ID, models.StorefrontEventProductView, "visitor-a", "session-b", models.StorefrontSourceProductDetail, from.Add(5*time.Hour))
	createAnalyticsEvent(t, db, &tenantA.ID, models.StorefrontEventWhatsAppClick, "visitor-b", "session-b", models.StorefrontSourceShop, from.Add(6*time.Hour))
	createAnalyticsEvent(t, db, &tenantA.ID, models.StorefrontEventPhoneClick, "visitor-b", "session-b", models.StorefrontSourceShop, from.Add(7*time.Hour))
	createAnalyticsEvent(t, db, &tenantB.ID, models.StorefrontEventProductClick, "visitor-c", "session-c", models.StorefrontSourceSearch, from.Add(8*time.Hour))
	createAnalyticsEvent(t, db, &tenantA.ID, models.StorefrontEventPageView, "excluded-end", "excluded-end", models.StorefrontSourceDirect, to)

	repo := NewStorefrontAnalyticsRepository(db)
	tenantCounts, err := repo.Metrics(context.Background(), analytics.TenantScope(tenantA.ID), analytics.TimeWindow{FromUTC: from, ToUTC: to})
	if err != nil {
		t.Fatalf("tenant Metrics: %v", err)
	}
	wantTenant := analytics.EventCounts{
		Visitors: 2, Sessions: 2, PageViews: 1, ProductImpressions: 2,
		ProductClicks: 1, ProductViews: 1, ContactIntents: 2,
	}
	if tenantCounts != wantTenant {
		t.Fatalf("tenant metrics = %+v, want %+v", tenantCounts, wantTenant)
	}

	allCounts, err := repo.Metrics(context.Background(), analytics.AllScope(), analytics.TimeWindow{FromUTC: from, ToUTC: to})
	if err != nil {
		t.Fatalf("all Metrics: %v", err)
	}
	wantAll := analytics.EventCounts{
		Visitors: 4, Sessions: 4, PageViews: 2, ProductImpressions: 2,
		ProductClicks: 2, ProductViews: 1, ContactIntents: 2,
	}
	if allCounts != wantAll {
		t.Fatalf("all metrics = %+v, want %+v", allCounts, wantAll)
	}
}

func TestStorefrontAnalyticsMetricsHonorNonUTCCalendarBoundary(t *testing.T) {
	db := newAnalyticsRepositoryTestDB(t)
	tenant := createAnalyticsTenant(t, db, "Shanghai Tenant")
	dateRange, err := analytics.ParseDateRange(analytics.DateRangeInput{
		From: "2026-07-01", To: "2026-07-01", Timezone: "Asia/Shanghai",
	}, time.Time{}, analytics.MaxStandardRangeDays)
	if err != nil {
		t.Fatalf("ParseDateRange: %v", err)
	}
	createAnalyticsEvent(t, db, &tenant.ID, models.StorefrontEventPageView, "before", "before", models.StorefrontSourceDirect, dateRange.FromUTC.Add(-time.Nanosecond))
	createAnalyticsEvent(t, db, &tenant.ID, models.StorefrontEventPageView, "start", "start", models.StorefrontSourceDirect, dateRange.FromUTC)
	createAnalyticsEvent(t, db, &tenant.ID, models.StorefrontEventPageView, "end", "end", models.StorefrontSourceDirect, dateRange.ToUTC.Add(-time.Nanosecond))
	createAnalyticsEvent(t, db, &tenant.ID, models.StorefrontEventPageView, "after", "after", models.StorefrontSourceDirect, dateRange.ToUTC)

	counts, err := NewStorefrontAnalyticsRepository(db).Metrics(context.Background(), analytics.TenantScope(tenant.ID), dateRange.Window())
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if counts.PageViews != 2 || counts.Visitors != 2 {
		t.Fatalf("boundary metrics = %+v, want two included events", counts)
	}
}

func TestStorefrontAnalyticsCurrentAndPreviousWindowsAreAdjacentAndNonOverlapping(t *testing.T) {
	db := newAnalyticsRepositoryTestDB(t)
	tenant := createAnalyticsTenant(t, db, "Period Tenant")
	dateRange, err := analytics.ParseDateRange(analytics.DateRangeInput{
		From: "2026-07-08", To: "2026-07-14", Timezone: "Africa/Accra",
	}, time.Time{}, analytics.MaxStandardRangeDays)
	if err != nil {
		t.Fatalf("ParseDateRange: %v", err)
	}
	current := dateRange.Window()
	previous := dateRange.PreviousWindow()
	if !previous.ToUTC.Equal(current.FromUTC) {
		t.Fatalf("previous/current boundary = %s/%s, want adjacent windows", previous.ToUTC, current.FromUTC)
	}

	createAnalyticsEvent(t, db, &tenant.ID, models.StorefrontEventPageView, "previous-start", "previous-start", models.StorefrontSourceDirect, previous.FromUTC)
	createAnalyticsEvent(t, db, &tenant.ID, models.StorefrontEventPageView, "before-current", "before-current", models.StorefrontSourceDirect, current.FromUTC.Add(-time.Second))
	createAnalyticsEvent(t, db, &tenant.ID, models.StorefrontEventPageView, "current-start", "current-start", models.StorefrontSourceDirect, current.FromUTC)
	createAnalyticsEvent(t, db, &tenant.ID, models.StorefrontEventPageView, "before-current-end", "before-current-end", models.StorefrontSourceDirect, current.ToUTC.Add(-time.Second))
	createAnalyticsEvent(t, db, &tenant.ID, models.StorefrontEventPageView, "current-end", "current-end", models.StorefrontSourceDirect, current.ToUTC)

	repo := NewStorefrontAnalyticsRepository(db)
	previousCounts, err := repo.Metrics(context.Background(), analytics.TenantScope(tenant.ID), previous)
	if err != nil {
		t.Fatalf("previous Metrics: %v", err)
	}
	currentCounts, err := repo.Metrics(context.Background(), analytics.TenantScope(tenant.ID), current)
	if err != nil {
		t.Fatalf("current Metrics: %v", err)
	}
	if previousCounts.PageViews != 2 || currentCounts.PageViews != 2 {
		t.Fatalf("previous/current page views = %d/%d, want 2/2", previousCounts.PageViews, currentCounts.PageViews)
	}
}

func TestStorefrontAnalyticsTrendQueriesHourDayAndWeek(t *testing.T) {
	db := newAnalyticsRepositoryTestDB(t)
	tenant := createAnalyticsTenant(t, db, "Trend Tenant")
	dateRange, err := analytics.ParseDateRange(analytics.DateRangeInput{
		From: "2026-07-08", To: "2026-07-10", Timezone: "Africa/Accra",
	}, time.Time{}, analytics.MaxWeeklyRangeDays)
	if err != nil {
		t.Fatalf("ParseDateRange: %v", err)
	}
	createAnalyticsEvent(t, db, &tenant.ID, models.StorefrontEventPageView, "one", "one", models.StorefrontSourceDirect, time.Date(2026, 7, 8, 10, 15, 0, 0, time.UTC))
	createAnalyticsEvent(t, db, &tenant.ID, models.StorefrontEventProductClick, "two", "two", models.StorefrontSourceMall, time.Date(2026, 7, 10, 14, 30, 0, 0, time.UTC))
	repo := NewStorefrontAnalyticsRepository(db)

	tests := []struct {
		granularity analytics.Granularity
		wantFirst   string
	}{
		{analytics.GranularityHour, "2026-07-08T10:00:00"},
		{analytics.GranularityDay, "2026-07-08"},
		{analytics.GranularityWeek, "2026-07-06"},
	}
	for _, test := range tests {
		points, err := repo.Trends(context.Background(), analytics.TenantScope(tenant.ID), dateRange, test.granularity)
		if err != nil {
			t.Fatalf("Trends(%s): %v", test.granularity, err)
		}
		if len(points) == 0 || points[0].Bucket != test.wantFirst {
			t.Fatalf("Trends(%s) = %+v, want first bucket %s", test.granularity, points, test.wantFirst)
		}
	}
}

func TestStorefrontAnalyticsSourcesNormalizeUnknownAndAggregateInSQL(t *testing.T) {
	db := newAnalyticsRepositoryTestDB(t)
	tenant := createAnalyticsTenant(t, db, "Source Tenant")
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 7)
	createAnalyticsEvent(t, db, &tenant.ID, models.StorefrontEventProductImpression, "a", "a", models.StorefrontSourceMall, from.Add(time.Hour))
	createAnalyticsEvent(t, db, &tenant.ID, models.StorefrontEventProductClick, "a", "a", models.StorefrontSourceMall, from.Add(2*time.Hour))
	createAnalyticsEvent(t, db, &tenant.ID, models.StorefrontEventProductClick, "b", "b", models.StorefrontSourceType("legacy_dynamic_source"), from.Add(3*time.Hour))

	stats, err := NewStorefrontAnalyticsRepository(db).Sources(
		context.Background(), analytics.TenantScope(tenant.ID), analytics.TimeWindow{FromUTC: from, ToUTC: to}, analytics.SourceSortClicks, 50,
	)
	if err != nil {
		t.Fatalf("Sources: %v", err)
	}
	bySource := make(map[string]analytics.SourceStat, len(stats))
	for _, stat := range stats {
		bySource[stat.SourceType] = stat
	}
	if mall := bySource["mall"]; mall.Impressions != 1 || mall.Clicks != 1 || mall.CTR != 1 {
		t.Fatalf("mall stat = %+v", mall)
	}
	if other := bySource["other"]; other.Clicks != 1 {
		t.Fatalf("other stat = %+v", other)
	}
}

func TestStorefrontAnalyticsOverviewTrendsAndSourcesExcludeBotTraffic(t *testing.T) {
	db := newAnalyticsRepositoryTestDB(t)
	tenant := createAnalyticsTenant(t, db, "Human Only Tenant")
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 1)
	createAnalyticsEvent(t, db, &tenant.ID, models.StorefrontEventProductImpression, "human", "human-session", models.StorefrontSourceMall, from.Add(time.Hour))
	createAnalyticsEvent(t, db, &tenant.ID, models.StorefrontEventProductClick, "human", "human-session", models.StorefrontSourceMall, from.Add(2*time.Hour))
	createAnalyticsEvent(t, db, &tenant.ID, models.StorefrontEventPageView, "bot", "bot-session", models.StorefrontSourceMall, from.Add(3*time.Hour), models.StorefrontTrafficBot)
	createAnalyticsEvent(t, db, &tenant.ID, models.StorefrontEventProductClick, "bot", "bot-session", models.StorefrontSourceMall, from.Add(4*time.Hour), models.StorefrontTrafficBot)

	repo := NewStorefrontAnalyticsRepository(db)
	window := analytics.TimeWindow{FromUTC: from, ToUTC: to}
	metrics, err := repo.Metrics(context.Background(), analytics.TenantScope(tenant.ID), window)
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if metrics.Visitors != 1 || metrics.Sessions != 1 || metrics.PageViews != 0 || metrics.ProductImpressions != 1 || metrics.ProductClicks != 1 {
		t.Fatalf("human-only metrics = %+v", metrics)
	}
	dateRange, err := analytics.ParseDateRange(analytics.DateRangeInput{
		From: "2026-07-01", To: "2026-07-01", Timezone: "Africa/Accra",
	}, time.Time{}, analytics.MaxWeeklyRangeDays)
	if err != nil {
		t.Fatalf("ParseDateRange: %v", err)
	}
	trend, err := repo.Trends(context.Background(), analytics.TenantScope(tenant.ID), dateRange, analytics.GranularityDay)
	if err != nil || len(trend) != 1 || trend[0].PageViews != 0 || trend[0].ProductClicks != 1 {
		t.Fatalf("human-only trend = %+v err=%v", trend, err)
	}
	sources, err := repo.Sources(context.Background(), analytics.TenantScope(tenant.ID), window, analytics.SourceSortClicks, 10)
	if err != nil || len(sources) != 1 || sources[0].Clicks != 1 || sources[0].Impressions != 1 {
		t.Fatalf("human-only sources = %+v err=%v", sources, err)
	}
}

func TestStorefrontAnalyticsPostgreSQLContractsUseConditionalAggregationAndSafeFragments(t *testing.T) {
	dateRange, err := analytics.ParseDateRange(analytics.DateRangeInput{
		From: "2026-07-01", To: "2026-07-02", Timezone: "Africa/Accra",
	}, time.Time{}, analytics.MaxWeeklyRangeDays)
	if err != nil {
		t.Fatalf("ParseDateRange: %v", err)
	}
	tenantScope := analytics.TenantScope(42)

	overview, err := buildOverviewQuery("postgres", tenantScope, dateRange.Window())
	if err != nil {
		t.Fatalf("buildOverviewQuery: %v", err)
	}
	assertFinalAnalyticsRange(t, overview, dateRange.Window(), 6)
	assertTenantBoundAfterFilter(t, overview, 6, uint(42))
	if !strings.Contains(overview.SQL, "COUNT(DISTINCT e.visitor_id)") ||
		!strings.Contains(overview.SQL, "COUNT(DISTINCT e.session_id)") ||
		strings.Count(overview.SQL, "COUNT(*) FILTER") != 5 ||
		strings.Count(overview.SQL, "FROM storefront_events") != 1 {
		t.Fatalf("PostgreSQL overview is not one conditional aggregation query: %s", overview.SQL)
	}
	if !strings.Contains(overview.SQL, "e.traffic_type = ?") || !containsAnalyticsArg(overview.Args, models.StorefrontTrafficHuman) {
		t.Fatalf("overview does not enforce bound human traffic: %s args=%#v", overview.SQL, overview.Args)
	}

	platformOverview, err := buildOverviewQuery("postgres", analytics.AllScope(), dateRange.Window())
	if err != nil {
		t.Fatalf("buildOverviewQuery platform scope: %v", err)
	}
	assertFinalAnalyticsRange(t, platformOverview, dateRange.Window(), 6)
	if strings.Contains(platformOverview.SQL, "e.tenant_id") {
		t.Fatalf("platform scope unexpectedly excludes tenant_id NULL rows: %s", platformOverview.SQL)
	}

	granularityFragments := map[analytics.Granularity]string{
		analytics.GranularityHour: "date_trunc('hour'",
		analytics.GranularityDay:  "date_trunc('day'",
		analytics.GranularityWeek: "date_trunc('week'",
	}
	for granularity, fragment := range granularityFragments {
		trend, err := buildTrendQuery("postgres", tenantScope, dateRange, granularity)
		if err != nil {
			t.Fatalf("buildTrendQuery(%q): %v", granularity, err)
		}
		if !strings.Contains(trend.SQL, fragment) || !strings.Contains(trend.SQL, "AT TIME ZONE ?") {
			t.Fatalf("PostgreSQL trend SQL for %q = %s", granularity, trend.SQL)
		}
		if len(trend.Args) == 0 || trend.Args[0] != dateRange.Timezone {
			t.Fatalf("trend timezone args = %#v", trend.Args)
		}
		assertFinalAnalyticsRange(t, trend, dateRange.Window(), 1)
		assertTenantBoundAfterFilter(t, trend, 1, uint(42))
	}
	unsafeGranularity := analytics.Granularity("day'); DROP TABLE storefront_events; --")
	if _, err := buildTrendQuery("postgres", tenantScope, dateRange, unsafeGranularity); err == nil {
		t.Fatal("unsafe trend granularity was accepted")
	}

	sortFragments := map[analytics.SourceSort]string{
		analytics.SourceSortImpressions:    "ORDER BY impressions DESC",
		analytics.SourceSortClicks:         "ORDER BY clicks DESC",
		analytics.SourceSortCTR:            "ORDER BY ctr DESC",
		analytics.SourceSortContactIntents: "ORDER BY contact_intents DESC",
	}
	for sort, fragment := range sortFragments {
		sources, err := buildSourceQuery("postgres", tenantScope, dateRange.Window(), sort, 25)
		if err != nil {
			t.Fatalf("buildSourceQuery(%q): %v", sort, err)
		}
		if !strings.Contains(sources.SQL, fragment) {
			t.Fatalf("PostgreSQL source sort for %q = %s", sort, sources.SQL)
		}
		assertFinalAnalyticsRange(t, sources, dateRange.Window(), len(analytics.SourceAllowlist))
		assertTenantBoundAfterFilter(t, sources, len(analytics.SourceAllowlist), uint(42))
		if len(sources.Args) == 0 || sources.Args[len(sources.Args)-1] != 25 {
			t.Fatalf("source limit args = %#v", sources.Args)
		}
	}
	if _, err := buildSourceQuery("postgres", tenantScope, dateRange.Window(), analytics.SourceSort("clicks; DROP TABLE storefront_events"), 25); err == nil {
		t.Fatal("unsafe source sort was accepted")
	}

	assertNoStorefrontEventSlices(t, "storefront_analytics_repository.go")
}

func assertNoStorefrontEventSlices(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			t.Fatalf("parse repository source %s: %v", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			array, ok := node.(*ast.ArrayType)
			if !ok || array.Len != nil {
				return true
			}
			selector, ok := array.Elt.(*ast.SelectorExpr)
			if !ok || selector.Sel == nil || selector.Sel.Name != "StorefrontEvent" {
				return true
			}
			packageName, ok := selector.X.(*ast.Ident)
			if !ok || packageName.Name != "models" {
				return true
			}
			position := fileSet.Position(array.Pos())
			t.Errorf("analytics repository declares []models.StorefrontEvent at %s", position)
			return true
		})
	}
}

func assertFinalAnalyticsRange(t *testing.T, query storefrontAnalyticsQuery, window analytics.TimeWindow, rangeArgIndex int) {
	t.Helper()
	if !strings.Contains(query.SQL, "e.occurred_at >= ?") || !strings.Contains(query.SQL, "e.occurred_at < ?") {
		t.Fatalf("final analytics SQL is missing half-open occurred_at range: %s", query.SQL)
	}
	if strings.Contains(query.SQL, window.FromUTC.Format(time.RFC3339)) || strings.Contains(query.SQL, window.ToUTC.Format(time.RFC3339)) {
		t.Fatalf("analytics time was interpolated into SQL instead of bound: %s", query.SQL)
	}
	if got, want := strings.Count(query.SQL, "?"), len(query.Args); got != want {
		t.Fatalf("placeholder/arg count = %d/%d for SQL %s with args %#v", got, want, query.SQL, query.Args)
	}
	if len(query.Args) <= rangeArgIndex+1 {
		t.Fatalf("analytics args are missing range at index %d: %#v", rangeArgIndex, query.Args)
	}
	from, fromOK := query.Args[rangeArgIndex].(time.Time)
	to, toOK := query.Args[rangeArgIndex+1].(time.Time)
	if !fromOK || !toOK || !from.Equal(window.FromUTC) || !to.Equal(window.ToUTC) {
		t.Fatalf("bound range args = %#v/%#v, want %s/%s", query.Args[rangeArgIndex], query.Args[rangeArgIndex+1], window.FromUTC, window.ToUTC)
	}
}

func assertTenantBoundAfterFilter(t *testing.T, query storefrontAnalyticsQuery, rangeArgIndex int, tenantID uint) {
	t.Helper()
	if !strings.Contains(query.SQL, "e.tenant_id = ?") {
		t.Fatalf("tenant scope is missing from final SQL: %s", query.SQL)
	}
	tenantArgIndex := rangeArgIndex + 2 + len(analytics.EventAllowlist) + 1
	if len(query.Args) <= tenantArgIndex {
		t.Fatalf("tenant arg is missing at index %d; all args %#v", tenantArgIndex, query.Args)
	}
	if query.Args[tenantArgIndex] != tenantID {
		t.Fatalf("tenant arg at index %d = %#v, want %d; all args %#v", tenantArgIndex, query.Args[tenantArgIndex], tenantID, query.Args)
	}
}

func containsAnalyticsArg(args []any, want any) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func newAnalyticsRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Tenant{}, &models.StorefrontEvent{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}

func createAnalyticsTenant(t *testing.T, db *gorm.DB, name string) models.Tenant {
	t.Helper()
	tenant := models.Tenant{Name: name, Type: models.TenantTypeThirdParty, Status: models.TenantStatusActive, AppKey: strings.ToLower(strings.ReplaceAll(name, " ", "-"))}
	if err := db.Create(&tenant).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	return tenant
}

func createAnalyticsEvent(
	t *testing.T,
	db *gorm.DB,
	tenantID *uint,
	eventName models.StorefrontEventName,
	visitorID string,
	sessionID string,
	sourceType models.StorefrontSourceType,
	occurredAt time.Time,
	trafficTypes ...models.StorefrontTrafficType,
) models.StorefrontEvent {
	t.Helper()
	event := models.StorefrontEvent{
		EventName:  eventName,
		VisitorID:  visitorID,
		SessionID:  sessionID,
		TenantID:   tenantID,
		SourceType: sourceType,
		PageType:   models.StorefrontPageOther,
		PagePath:   "/en/test",
		Locale:     "en",
		DeviceType: models.StorefrontDeviceDesktop,
		OccurredAt: occurredAt.UTC(),
		CreatedAt:  occurredAt.UTC(),
	}
	if len(trafficTypes) > 0 {
		event.TrafficType = trafficTypes[0]
	}
	if err := db.Create(&event).Error; err != nil {
		t.Fatalf("create event %s: %v", eventName, err)
	}
	return event
}
