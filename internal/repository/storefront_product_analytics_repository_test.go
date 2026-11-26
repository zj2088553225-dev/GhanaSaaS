package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"saas/internal/analytics"
	"saas/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type productAnalyticsFixture struct {
	db       *gorm.DB
	repo     *StorefrontAnalyticsRepository
	tenantA  models.Tenant
	tenantB  models.Tenant
	category models.PlatformCategory
	productA models.SPU
	offShelf models.SPU
	productB models.SPU
	deleted  models.SPU
	from     time.Time
	to       time.Time
}

func TestProductAnalyticsRepositoryListMetricsPaginationScopeAndHistoricalMetadata(t *testing.T) {
	fixture := newProductAnalyticsFixture(t)
	query := analytics.ProductAnalyticsQuery{
		Sort: analytics.ProductMetricClicks, Order: analytics.SortOrderDescending, Page: 1, PageSize: 1,
	}
	items, total, err := fixture.repo.ProductAnalyticsList(
		context.Background(), analytics.TenantScope(fixture.tenantA.ID), analytics.TimeWindow{FromUTC: fixture.from, ToUTC: fixture.to}, query,
	)
	if err != nil {
		t.Fatalf("ProductAnalyticsList: %v", err)
	}
	if total != 3 || len(items) != 1 || items[0].ProductID != fixture.productA.ID {
		t.Fatalf("first page total/items = %d/%+v", total, items)
	}
	got := items[0]
	if got.Impressions != 3 || got.Clicks != 2 || got.Views != 2 || got.ContactIntents != 2 || got.UniqueVisitors != 2 {
		t.Fatalf("product metrics = %+v", got)
	}
	if !got.CurrentlyExists || got.Status != models.SPUStatusOnShelf || got.TenantID != fixture.tenantA.ID || got.TenantName != fixture.tenantA.Name {
		t.Fatalf("current product metadata = %+v", got.ProductMetadata)
	}

	query.Page = 2
	items, total, err = fixture.repo.ProductAnalyticsList(
		context.Background(), analytics.TenantScope(fixture.tenantA.ID), analytics.TimeWindow{FromUTC: fixture.from, ToUTC: fixture.to}, query,
	)
	if err != nil {
		t.Fatalf("ProductAnalyticsList page 2: %v", err)
	}
	if total != 3 || len(items) != 1 || items[0].ProductID != fixture.deleted.ID || items[0].CurrentlyExists || items[0].ProductName != fmt.Sprintf("Deleted product #%d", fixture.deleted.ID) {
		t.Fatalf("deleted product page = total %d items %+v", total, items)
	}

	query.Page, query.PageSize = 1, 20
	allItems, allTotal, err := fixture.repo.ProductAnalyticsList(
		context.Background(), analytics.AllScope(), analytics.TimeWindow{FromUTC: fixture.from, ToUTC: fixture.to}, query,
	)
	if err != nil {
		t.Fatalf("all-scope ProductAnalyticsList: %v", err)
	}
	if allTotal != 4 || len(allItems) != 4 {
		t.Fatalf("all-scope total/items = %d/%d", allTotal, len(allItems))
	}
	for _, item := range allItems {
		if item.ProductID == fixture.productB.ID && item.TenantID != fixture.tenantB.ID {
			t.Fatalf("tenant B metadata crossed scope: %+v", item)
		}
	}
}

func TestProductAnalyticsRepositoryExcludesBotTraffic(t *testing.T) {
	fixture := newProductAnalyticsFixture(t)
	createProductAnalyticsEvent(
		t, fixture.db, fixture.tenantA.ID, fixture.productA.ID, models.StorefrontEventProductClick,
		"bot-visitor", models.StorefrontSourceMall, "all", fixture.from.Add(17*time.Hour), models.StorefrontTrafficBot,
	)
	createProductAnalyticsEvent(
		t, fixture.db, fixture.tenantA.ID, fixture.productA.ID, models.StorefrontEventProductImpression,
		"bot-visitor", models.StorefrontSourceMall, "all", fixture.from.Add(18*time.Hour), models.StorefrontTrafficBot,
	)
	summary, err := fixture.repo.ProductAnalyticsSummary(
		context.Background(), analytics.TenantScope(fixture.tenantA.ID),
		analytics.TimeWindow{FromUTC: fixture.from, ToUTC: fixture.to}, fixture.productA.ID,
	)
	if err != nil {
		t.Fatalf("ProductAnalyticsSummary: %v", err)
	}
	if summary.Impressions != 3 || summary.Clicks != 2 || summary.UniqueVisitors != 2 {
		t.Fatalf("human-only product summary = %+v", summary)
	}
}

func TestProductAnalyticsRepositorySearchStatusAndDeletedSemantics(t *testing.T) {
	fixture := newProductAnalyticsFixture(t)
	window := analytics.TimeWindow{FromUTC: fixture.from, ToUTC: fixture.to}
	base := analytics.ProductAnalyticsQuery{Sort: analytics.ProductMetricClicks, Order: analytics.SortOrderDescending, Page: 1, PageSize: 20}

	search := base
	search.Search = "alpha-lamp-" + strconv.FormatUint(uint64(fixture.productA.ID), 10)
	items, total, err := fixture.repo.ProductAnalyticsList(context.Background(), analytics.TenantScope(fixture.tenantA.ID), window, search)
	if err != nil || total != 1 || len(items) != 1 || items[0].ProductID != fixture.productA.ID {
		t.Fatalf("slug search = total %d items %+v err=%v", total, items, err)
	}

	search.Search = strconv.FormatUint(uint64(fixture.deleted.ID), 10)
	items, total, err = fixture.repo.ProductAnalyticsList(context.Background(), analytics.TenantScope(fixture.tenantA.ID), window, search)
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("deleted search must not match = total %d items %+v err=%v", total, items, err)
	}

	offShelf := base
	offShelf.Status = analytics.ProductStatusFilter(models.SPUStatusOffShelf)
	items, total, err = fixture.repo.ProductAnalyticsList(context.Background(), analytics.TenantScope(fixture.tenantA.ID), window, offShelf)
	if err != nil || total != 1 || len(items) != 1 || items[0].ProductID != fixture.offShelf.ID || !items[0].CurrentlyExists {
		t.Fatalf("off-shelf filter = total %d items %+v err=%v", total, items, err)
	}

	deleted := base
	deleted.Status = analytics.ProductStatusDeleted
	items, total, err = fixture.repo.ProductAnalyticsList(context.Background(), analytics.TenantScope(fixture.tenantA.ID), window, deleted)
	if err != nil || total != 1 || len(items) != 1 || items[0].ProductID != fixture.deleted.ID || items[0].CurrentlyExists {
		t.Fatalf("deleted filter = total %d items %+v err=%v", total, items, err)
	}
}

func TestProductAnalyticsRepositoryRankingsApplyMinimumDenominators(t *testing.T) {
	fixture := newProductAnalyticsFixture(t)
	window := analytics.TimeWindow{FromUTC: fixture.from, ToUTC: fixture.to}
	scope := analytics.TenantScope(fixture.tenantA.ID)

	topCTR, err := fixture.repo.TopProductAnalytics(context.Background(), scope, window, analytics.ProductRankingQuery{
		Metric: analytics.ProductMetricCTR, Limit: 10, MinImpressions: 2,
	})
	if err != nil || len(topCTR) != 2 || topCTR[0].ProductID != fixture.productA.ID {
		t.Fatalf("top CTR = %+v, err=%v", topCTR, err)
	}
	for _, item := range topCTR {
		if item.Impressions < 2 {
			t.Fatalf("top CTR included low denominator: %+v", item)
		}
	}

	topDetail, err := fixture.repo.TopProductAnalytics(context.Background(), scope, window, analytics.ProductRankingQuery{
		Metric: analytics.ProductMetricDetailViewRate, Limit: 10, MinClicks: 2,
	})
	if err != nil || len(topDetail) != 1 || topDetail[0].ProductID != fixture.productA.ID {
		t.Fatalf("top detail rate = %+v, err=%v", topDetail, err)
	}

	topContact, err := fixture.repo.TopProductAnalytics(context.Background(), scope, window, analytics.ProductRankingQuery{
		Metric: analytics.ProductMetricContactIntentRate, Limit: 10, MinViews: 1,
	})
	if err != nil || len(topContact) != 1 || topContact[0].ProductID != fixture.productA.ID {
		t.Fatalf("top contact rate = %+v, err=%v", topContact, err)
	}

	lowCTR, err := fixture.repo.LowCTRProductAnalytics(context.Background(), scope, window, 2, 10)
	if err != nil || len(lowCTR) != 2 || lowCTR[0].ProductID != fixture.deleted.ID || lowCTR[1].ProductID != fixture.productA.ID {
		t.Fatalf("low CTR = %+v, err=%v", lowCTR, err)
	}
}

func TestProductAnalyticsRepositoryDetailTrendSourcesAndBatchLabels(t *testing.T) {
	fixture := newProductAnalyticsFixture(t)
	window := analytics.TimeWindow{FromUTC: fixture.from, ToUTC: fixture.to}
	scope := analytics.TenantScope(fixture.tenantA.ID)

	summary, err := fixture.repo.ProductAnalyticsSummary(context.Background(), scope, window, fixture.productA.ID)
	if err != nil || summary.Impressions != 3 || summary.Clicks != 2 || summary.Views != 2 || summary.ContactIntents != 2 || summary.UniqueVisitors != 2 {
		t.Fatalf("product summary = %+v, err=%v", summary, err)
	}
	deleted, err := fixture.repo.ProductAnalyticsSummary(context.Background(), scope, window, fixture.deleted.ID)
	if err != nil || deleted.CurrentlyExists || deleted.ProductID != fixture.deleted.ID {
		t.Fatalf("deleted summary = %+v, err=%v", deleted, err)
	}
	if _, err := fixture.repo.CurrentProductAnalyticsMetadata(context.Background(), analytics.TenantScope(fixture.tenantB.ID), fixture.productA.ID); err == nil {
		t.Fatal("tenant B resolved tenant A product metadata")
	}

	dateRange, err := analytics.ParseDateRange(analytics.DateRangeInput{
		From: "2026-07-01", To: "2026-07-03", Timezone: "Africa/Accra",
	}, time.Time{}, analytics.MaxWeeklyRangeDays)
	if err != nil {
		t.Fatalf("ParseDateRange: %v", err)
	}
	trend, err := fixture.repo.ProductAnalyticsTrends(context.Background(), scope, dateRange, analytics.GranularityDay, fixture.productA.ID)
	if err != nil || len(trend) != 2 || trend[0].Bucket != "2026-07-01" || trend[0].UniqueVisitors != 2 || trend[1].Bucket != "2026-07-03" || trend[1].UniqueVisitors != 1 {
		t.Fatalf("product trend = %+v, err=%v", trend, err)
	}

	sources, err := fixture.repo.ProductAnalyticsSources(context.Background(), scope, window, fixture.productA.ID)
	if err != nil {
		t.Fatalf("ProductAnalyticsSources: %v", err)
	}
	bySource := make(map[string]analytics.ProductSourceStat, len(sources))
	for _, source := range sources {
		bySource[source.SourceType] = source
	}
	if mall := bySource["mall"]; mall.Impressions != 2 || mall.Clicks != 1 || mall.Views != 0 {
		t.Fatalf("mall event context = %+v", mall)
	}
	if other := bySource["other"]; other.Views != 1 {
		t.Fatalf("unknown source normalization = %+v", other)
	}

	categoryID := strconv.FormatUint(uint64(fixture.category.ID), 10)
	shopID := strconv.FormatUint(uint64(fixture.tenantA.ID), 10)
	trackingKey := "11111111-1111-4111-8111-111111111111"
	unresolved := "missing"
	labels, err := fixture.repo.ProductAnalyticsSourceLabels(context.Background(), []analytics.ProductSourceStat{
		{SourceType: "category", SourceID: &categoryID},
		{SourceType: "shop", SourceID: &shopID},
		{SourceType: "home_product_floor", SourceID: &trackingKey},
		{SourceType: "category", SourceID: &unresolved},
	})
	if err != nil {
		t.Fatalf("ProductAnalyticsSourceLabels: %v", err)
	}
	resolved := make(map[string]string, len(labels))
	for _, label := range labels {
		resolved[label.SourceType+":"+label.SourceID] = label.Label
	}
	if resolved["category:"+categoryID] != fixture.category.Path || resolved["shop:"+shopID] != "Tenant A Shop" || resolved["home_product_floor:"+trackingKey] != "Featured Products" {
		t.Fatalf("batch source labels = %+v", resolved)
	}
	if _, exists := resolved["category:missing"]; exists {
		t.Fatalf("unresolved source unexpectedly received a label: %+v", resolved)
	}
}

func TestProductAnalyticsPostgreSQLQueryContractsUseSafeFinalFragments(t *testing.T) {
	dateRange, err := analytics.ParseDateRange(analytics.DateRangeInput{
		From: "2026-07-01", To: "2026-07-14", Timezone: "Asia/Shanghai",
	}, time.Time{}, analytics.MaxWeeklyRangeDays)
	if err != nil {
		t.Fatalf("ParseDateRange: %v", err)
	}
	query := analytics.ProductAnalyticsQuery{
		Search: "lamp", Status: analytics.ProductStatusFilter(models.SPUStatusOnShelf),
		Sort: analytics.ProductMetricCTR, Order: analytics.SortOrderAscending, Page: 2, PageSize: 20,
	}
	built, err := buildProductAnalyticsListQueries("postgres", analytics.TenantScope(7), dateRange.Window(), query)
	if err != nil {
		t.Fatalf("buildProductAnalyticsListQueries: %v", err)
	}
	for name, finalQuery := range map[string]storefrontAnalyticsQuery{"items": built.Items, "total": built.Total} {
		normalizedSQL := strings.ToLower(strings.Join(strings.Fields(finalQuery.SQL), " "))
		if !strings.Contains(normalizedSQL, "e.occurred_at >= ?") || !strings.Contains(normalizedSQL, "e.occurred_at < ?") ||
			!strings.Contains(normalizedSQL, "e.tenant_id = ?") || !strings.Contains(normalizedSQL, "e.event_name in (") ||
			!strings.Contains(normalizedSQL, "product_aggregates as") || !strings.Contains(normalizedSQL, "group by product_id, tenant_id") ||
			!strings.Contains(normalizedSQL, "count(distinct visitor_id)") || !strings.Contains(normalizedSQL, "left join spus") ||
			!strings.Contains(normalizedSQL, "filter (where") {
			t.Fatalf("%s final SQL contract = %s", name, finalQuery.SQL)
		}
		if len(finalQuery.Args) < 9 || finalQuery.Args[0] != dateRange.FromUTC || finalQuery.Args[1] != dateRange.ToUTC ||
			finalQuery.Args[2+len(analytics.ProductEventAllowlist)] != models.StorefrontTrafficHuman ||
			finalQuery.Args[3+len(analytics.ProductEventAllowlist)] != uint(7) {
			t.Fatalf("%s final args = %#v", name, finalQuery.Args)
		}
		if strings.Count(finalQuery.SQL, "?") != len(finalQuery.Args) {
			t.Fatalf("%s placeholder/args = %d/%d", name, strings.Count(finalQuery.SQL, "?"), len(finalQuery.Args))
		}
		for _, eventName := range analytics.ProductEventAllowlist {
			if strings.Contains(finalQuery.SQL, "'"+string(eventName)+"'") {
				t.Fatalf("%s interpolated event name %q into SQL: %s", name, eventName, finalQuery.SQL)
			}
		}
	}
	if !strings.Contains(built.Items.SQL, "ORDER BY ctr ASC") || !strings.Contains(built.Items.SQL, "LIMIT ? OFFSET ?") || strings.Contains(built.Items.SQL, query.Search) {
		t.Fatalf("safe list sort/search contract = %s", built.Items.SQL)
	}
	if !strings.Contains(built.Total.SQL, "SELECT COUNT(*) FROM scored_products") {
		t.Fatalf("product total is not calculated in SQL: %s", built.Total.SQL)
	}
	if _, err := buildProductAnalyticsListQueries("postgres", analytics.AllScope(), dateRange.Window(), analytics.ProductAnalyticsQuery{
		Sort: analytics.ProductMetric("clicks; DROP TABLE storefront_events"), Order: analytics.SortOrderDescending, Page: 1, PageSize: 20,
	}); err == nil {
		t.Fatal("unsafe product sort was accepted")
	}
	if _, err := buildProductAnalyticsListQueries("postgres", analytics.AllScope(), dateRange.Window(), analytics.ProductAnalyticsQuery{
		Sort: analytics.ProductMetricClicks, Order: analytics.SortOrder("DESC NULLS FIRST"), Page: 1, PageSize: 20,
	}); err == nil {
		t.Fatal("unsafe product order was accepted")
	}

	for granularity, fragment := range map[analytics.Granularity]string{
		analytics.GranularityHour: "date_trunc('hour'",
		analytics.GranularityDay:  "date_trunc('day'",
		analytics.GranularityWeek: "date_trunc('week'",
	} {
		trend, err := buildProductAnalyticsTrendQuery("postgres", analytics.TenantScope(7), dateRange, granularity, 19)
		if err != nil || !strings.Contains(trend.SQL, fragment) || !strings.Contains(trend.SQL, "AT TIME ZONE ?") || !strings.Contains(trend.SQL, "e.product_id = ?") {
			t.Fatalf("trend contract %q = %s, err=%v", granularity, trend.SQL, err)
		}
	}
	if _, err := buildProductAnalyticsTrendQuery("postgres", analytics.AllScope(), dateRange, analytics.Granularity("month'); DROP TABLE storefront_events; --"), 19); err == nil {
		t.Fatal("unsafe product trend granularity was accepted")
	}

	topCTR, err := buildTopProductAnalyticsQuery("postgres", analytics.TenantScope(7), dateRange.Window(), analytics.ProductRankingQuery{
		Metric: analytics.ProductMetricCTR, Limit: 10, MinImpressions: 100,
	})
	if err != nil || !strings.Contains(topCTR.SQL, "WHERE impressions >= ?") {
		t.Fatalf("top CTR threshold contract = %s, err=%v", topCTR.SQL, err)
	}
	topDetail, err := buildTopProductAnalyticsQuery("postgres", analytics.TenantScope(7), dateRange.Window(), analytics.ProductRankingQuery{
		Metric: analytics.ProductMetricDetailViewRate, Limit: 10, MinClicks: 20,
	})
	if err != nil || !strings.Contains(topDetail.SQL, "WHERE clicks >= ?") {
		t.Fatalf("top detail threshold contract = %s, err=%v", topDetail.SQL, err)
	}
	topContact, err := buildTopProductAnalyticsQuery("postgres", analytics.TenantScope(7), dateRange.Window(), analytics.ProductRankingQuery{
		Metric: analytics.ProductMetricContactIntentRate, Limit: 10, MinViews: 10,
	})
	if err != nil || !strings.Contains(topContact.SQL, "WHERE views >= ?") {
		t.Fatalf("top contact threshold contract = %s, err=%v", topContact.SQL, err)
	}
	lowCTR, err := buildLowCTRProductAnalyticsQuery("postgres", analytics.TenantScope(7), dateRange.Window(), 100, 10)
	if err != nil || !strings.Contains(lowCTR.SQL, "WHERE impressions >= ?") ||
		!strings.Contains(lowCTR.SQL, "ORDER BY ctr ASC, impressions DESC, product_id ASC") {
		t.Fatalf("low CTR threshold/order contract = %s, err=%v", lowCTR.SQL, err)
	}

	assertNoStorefrontEventSlices(t, "storefront_product_analytics_repository.go")
}

func newProductAnalyticsFixture(t *testing.T) productAnalyticsFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Tenant{}, &models.PlatformCategory{}, &models.Category{}, &models.SPU{},
		&models.ShopProfile{}, &models.SitePage{}, &models.StorefrontEvent{},
	); err != nil {
		t.Fatalf("AutoMigrate product analytics: %v", err)
	}
	tenantA := models.Tenant{Name: "Tenant A", Type: models.TenantTypeThirdParty, Status: models.TenantStatusActive, AppKey: "product-a"}
	tenantB := models.Tenant{Name: "Tenant B", Type: models.TenantTypeThirdParty, Status: models.TenantStatusActive, AppKey: "product-b"}
	if err := db.Create(&tenantA).Error; err != nil {
		t.Fatalf("create tenant A: %v", err)
	}
	if err := db.Create(&tenantB).Error; err != nil {
		t.Fatalf("create tenant B: %v", err)
	}
	category := models.PlatformCategory{Name: "Lighting", Path: "Home/Lighting", Level: 2, Status: models.PlatformCategoryStatusActive, IsLeaf: true}
	if err := db.Create(&category).Error; err != nil {
		t.Fatalf("create platform category: %v", err)
	}
	products := []*models.SPU{
		{TenantID: tenantA.ID, PlatformCategoryID: category.ID, Name: "Alpha Lamp", MainImage: "alpha.jpg", Status: models.SPUStatusOnShelf},
		{TenantID: tenantA.ID, PlatformCategoryID: category.ID, Name: "Beta Chair", MainImage: "beta.jpg", Status: models.SPUStatusOffShelf},
		{TenantID: tenantB.ID, PlatformCategoryID: category.ID, Name: "Gamma Table", MainImage: "gamma.jpg", Status: models.SPUStatusOnShelf},
		{TenantID: tenantA.ID, PlatformCategoryID: category.ID, Name: "Removed Shelf", MainImage: "removed.jpg", Status: models.SPUStatusOffShelf},
	}
	for _, product := range products {
		if err := db.Create(product).Error; err != nil {
			t.Fatalf("create product %s: %v", product.Name, err)
		}
	}
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	productA, offShelf, productB, deleted := *products[0], *products[1], *products[2], *products[3]

	createProductAnalyticsEvent(t, db, tenantA.ID, productA.ID, models.StorefrontEventProductImpression, "v1", models.StorefrontSourceMall, "all", from.Add(time.Hour))
	createProductAnalyticsEvent(t, db, tenantA.ID, productA.ID, models.StorefrontEventProductImpression, "v1", models.StorefrontSourceMall, "all", from.Add(2*time.Hour))
	createProductAnalyticsEvent(t, db, tenantA.ID, productA.ID, models.StorefrontEventProductImpression, "v2", models.StorefrontSourceCategory, strconv.FormatUint(uint64(category.ID), 10), from.Add(3*time.Hour))
	createProductAnalyticsEvent(t, db, tenantA.ID, productA.ID, models.StorefrontEventProductClick, "v1", models.StorefrontSourceMall, "all", from.Add(4*time.Hour))
	createProductAnalyticsEvent(t, db, tenantA.ID, productA.ID, models.StorefrontEventProductClick, "v2", models.StorefrontSourceCategory, strconv.FormatUint(uint64(category.ID), 10), from.Add(5*time.Hour))
	createProductAnalyticsEvent(t, db, tenantA.ID, productA.ID, models.StorefrontEventProductView, "v1", models.StorefrontSourceProductDetail, strconv.FormatUint(uint64(productA.ID), 10), from.Add(6*time.Hour))
	createProductAnalyticsEvent(t, db, tenantA.ID, productA.ID, models.StorefrontEventProductView, "v2", models.StorefrontSourceType("legacy_source"), "legacy", from.Add(7*time.Hour))
	createProductAnalyticsEvent(t, db, tenantA.ID, productA.ID, models.StorefrontEventWhatsAppClick, "v1", models.StorefrontSourceProductDetail, strconv.FormatUint(uint64(productA.ID), 10), from.Add(8*time.Hour))
	createProductAnalyticsEvent(t, db, tenantA.ID, productA.ID, models.StorefrontEventPhoneClick, "v2", models.StorefrontSourceShop, strconv.FormatUint(uint64(tenantA.ID), 10), from.AddDate(0, 0, 2).Add(time.Hour))
	createProductAnalyticsEvent(t, db, tenantA.ID, offShelf.ID, models.StorefrontEventProductImpression, "v3", models.StorefrontSourceMall, "all", from.Add(9*time.Hour))
	createProductAnalyticsEvent(t, db, tenantB.ID, productB.ID, models.StorefrontEventProductImpression, "v5", models.StorefrontSourceMall, "all", from.Add(10*time.Hour))
	createProductAnalyticsEvent(t, db, tenantB.ID, productB.ID, models.StorefrontEventProductClick, "v5", models.StorefrontSourceMall, "all", from.Add(11*time.Hour))
	createProductAnalyticsEvent(t, db, tenantA.ID, deleted.ID, models.StorefrontEventProductImpression, "v4", models.StorefrontSourceMall, "all", from.Add(12*time.Hour))
	createProductAnalyticsEvent(t, db, tenantA.ID, deleted.ID, models.StorefrontEventProductImpression, "v4", models.StorefrontSourceMall, "all", from.Add(13*time.Hour))
	createProductAnalyticsEvent(t, db, tenantA.ID, deleted.ID, models.StorefrontEventProductClick, "v4", models.StorefrontSourceMall, "all", from.Add(14*time.Hour))

	outside := from.Add(-time.Second)
	createProductAnalyticsEvent(t, db, tenantA.ID, productA.ID, models.StorefrontEventProductClick, "outside", models.StorefrontSourceMall, "all", outside)
	productID := productA.ID
	if err := db.Create(&models.StorefrontEvent{
		EventName: models.StorefrontEventPageView, VisitorID: "page", SessionID: "page", TenantID: &tenantA.ID,
		ProductID: &productID, SourceType: models.StorefrontSourceDirect, PageType: models.StorefrontPageProduct,
		PagePath: "/en/products", Locale: "en", DeviceType: models.StorefrontDeviceDesktop, OccurredAt: from.Add(15 * time.Hour), CreatedAt: from.Add(15 * time.Hour),
	}).Error; err != nil {
		t.Fatalf("create excluded page event: %v", err)
	}
	if err := db.Create(&models.StorefrontEvent{
		EventName: models.StorefrontEventProductClick, VisitorID: "nil-product", SessionID: "nil-product", TenantID: &tenantA.ID,
		SourceType: models.StorefrontSourceMall, PageType: models.StorefrontPageMall, PagePath: "/en/mall", Locale: "en",
		DeviceType: models.StorefrontDeviceDesktop, OccurredAt: from.Add(16 * time.Hour), CreatedAt: from.Add(16 * time.Hour),
	}).Error; err != nil {
		t.Fatalf("create nil product event: %v", err)
	}
	if err := db.Delete(&deleted).Error; err != nil {
		t.Fatalf("hard-delete product: %v", err)
	}
	shop := models.ShopProfile{TenantID: tenantA.ID, Slug: "tenant-a", ShopName: "Tenant A Shop", IsPublic: true}
	if err := db.Create(&shop).Error; err != nil {
		t.Fatalf("create shop: %v", err)
	}
	page := models.SitePage{
		PageKey: "home", Title: "Home", Status: models.SitePageStatusPublished,
		DraftContent:     datatypes.JSON(`{"blocks":[]}`),
		PublishedContent: datatypes.JSON(`{"blocks":[{"type":"product_floor","tracking_key":"11111111-1111-4111-8111-111111111111","title":"Featured Products"}]}`),
	}
	if err := db.Create(&page).Error; err != nil {
		t.Fatalf("create site page: %v", err)
	}
	return productAnalyticsFixture{
		db: db, repo: NewStorefrontAnalyticsRepository(db), tenantA: tenantA, tenantB: tenantB,
		category: category, productA: productA, offShelf: offShelf, productB: productB, deleted: deleted, from: from, to: to,
	}
}

func createProductAnalyticsEvent(
	t *testing.T,
	db *gorm.DB,
	tenantID uint,
	productID uint,
	eventName models.StorefrontEventName,
	visitorID string,
	sourceType models.StorefrontSourceType,
	sourceID string,
	occurredAt time.Time,
	trafficTypes ...models.StorefrontTrafficType,
) {
	t.Helper()
	sourceIDCopy := sourceID
	event := models.StorefrontEvent{
		EventName: eventName, VisitorID: visitorID, SessionID: "session-" + visitorID, TenantID: &tenantID,
		ProductID: &productID, SourceType: sourceType, SourceID: &sourceIDCopy, PageType: models.StorefrontPageProduct,
		PagePath: "/en/products/test", Locale: "en", DeviceType: models.StorefrontDeviceDesktop,
		OccurredAt: occurredAt.UTC(), CreatedAt: occurredAt.UTC(),
	}
	if len(trafficTypes) > 0 {
		event.TrafficType = trafficTypes[0]
	}
	if err := db.Create(&event).Error; err != nil {
		t.Fatalf("create product analytics event %s: %v", eventName, err)
	}
}
