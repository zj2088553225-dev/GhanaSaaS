package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"saas/internal/analytics"
	"saas/internal/models"

	"gorm.io/gorm"
)

type fakeProductAnalyticsRepository struct {
	fakeStorefrontAnalyticsRepository
	listRows       []analytics.ProductAggregate
	listTotal      int64
	listScope      analytics.Scope
	listWindow     analytics.TimeWindow
	listQuery      analytics.ProductAnalyticsQuery
	topRows        []analytics.ProductAggregate
	topScope       analytics.Scope
	topQuery       analytics.ProductRankingQuery
	lowRows        []analytics.ProductAggregate
	lowScope       analytics.Scope
	lowMinimum     int64
	lowLimit       int
	metadata       *analytics.ProductMetadata
	metadataErr    error
	summary        *analytics.ProductAggregate
	summaryErr     error
	detailScope    analytics.Scope
	detailProduct  uint
	productTrend   []analytics.ProductTrendPoint
	productSources []analytics.ProductSourceStat
	sourceLabels   []analytics.ProductSourceLabel
	productErr     error
}

func (repo *fakeProductAnalyticsRepository) ProductAnalyticsList(
	_ context.Context,
	scope analytics.Scope,
	window analytics.TimeWindow,
	query analytics.ProductAnalyticsQuery,
) ([]analytics.ProductAggregate, int64, error) {
	repo.listScope, repo.listWindow, repo.listQuery = scope, window, query
	return repo.listRows, repo.listTotal, repo.productErr
}

func (repo *fakeProductAnalyticsRepository) TopProductAnalytics(
	_ context.Context,
	scope analytics.Scope,
	_ analytics.TimeWindow,
	query analytics.ProductRankingQuery,
) ([]analytics.ProductAggregate, error) {
	repo.topScope, repo.topQuery = scope, query
	return repo.topRows, repo.productErr
}

func (repo *fakeProductAnalyticsRepository) LowCTRProductAnalytics(
	_ context.Context,
	scope analytics.Scope,
	_ analytics.TimeWindow,
	minimum int64,
	limit int,
) ([]analytics.ProductAggregate, error) {
	repo.lowScope, repo.lowMinimum, repo.lowLimit = scope, minimum, limit
	return repo.lowRows, repo.productErr
}

func (repo *fakeProductAnalyticsRepository) CurrentProductAnalyticsMetadata(
	_ context.Context,
	scope analytics.Scope,
	productID uint,
) (*analytics.ProductMetadata, error) {
	repo.detailScope, repo.detailProduct = scope, productID
	return repo.metadata, repo.metadataErr
}

func (repo *fakeProductAnalyticsRepository) ProductAnalyticsSummary(
	_ context.Context,
	scope analytics.Scope,
	_ analytics.TimeWindow,
	productID uint,
) (*analytics.ProductAggregate, error) {
	repo.detailScope, repo.detailProduct = scope, productID
	return repo.summary, repo.summaryErr
}

func (repo *fakeProductAnalyticsRepository) ProductAnalyticsTrends(
	_ context.Context,
	_ analytics.Scope,
	_ analytics.DateRange,
	_ analytics.Granularity,
	_ uint,
) ([]analytics.ProductTrendPoint, error) {
	return repo.productTrend, repo.productErr
}

func (repo *fakeProductAnalyticsRepository) ProductAnalyticsSources(
	_ context.Context,
	_ analytics.Scope,
	_ analytics.TimeWindow,
	_ uint,
) ([]analytics.ProductSourceStat, error) {
	return repo.productSources, repo.productErr
}

func (repo *fakeProductAnalyticsRepository) ProductAnalyticsSourceLabels(
	_ context.Context,
	_ []analytics.ProductSourceStat,
) ([]analytics.ProductSourceLabel, error) {
	return repo.sourceLabels, repo.productErr
}

func TestProductAnalyticsServiceListUsesSharedRangeScopePaginationAndRates(t *testing.T) {
	repo := &fakeProductAnalyticsRepository{
		listRows: []analytics.ProductAggregate{{
			ProductMetadata: analytics.ProductMetadata{
				ProductID: 19, ProductName: "Alpha Lamp", TenantID: 7, TenantName: "Tenant", Status: models.SPUStatusOnShelf, CurrentlyExists: true,
			},
			ProductCounts: analytics.ProductCounts{Impressions: 200, Clicks: 25, Views: 10, ContactIntents: 2, UniqueVisitors: 18},
		}},
		listTotal: 1,
	}
	service := newFixedAnalyticsService(repo)
	result, err := service.ProductsForTenant(context.Background(), 7, ProductAnalyticsListRequest{
		DateRange: analytics.DateRangeInput{From: "2026-07-01", To: "2026-07-14", Timezone: "Asia/Shanghai"},
	})
	if err != nil {
		t.Fatalf("ProductsForTenant: %v", err)
	}
	if repo.listScope != analytics.TenantScope(7) || repo.listQuery.Page != 1 || repo.listQuery.PageSize != 20 || repo.listQuery.Sort != analytics.ProductMetricClicks || repo.listQuery.Order != analytics.SortOrderDescending {
		t.Fatalf("normalized product list query = scope %+v query %+v", repo.listScope, repo.listQuery)
	}
	if got, want := repo.listWindow.FromUTC.Format(time.RFC3339), "2026-06-30T16:00:00Z"; got != want {
		t.Fatalf("list FromUTC = %s, want %s", got, want)
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].ProductSlug != "alpha-lamp-19" || result.Items[0].CTR != 0.125 || result.Items[0].DetailViewRate != 0.4 || result.Items[0].ContactIntentRate != 0.2 {
		t.Fatalf("product list result = %+v", result)
	}

	repo.tenantExists = map[uint]bool{9: true}
	tenantID := uint(9)
	if _, err := service.ProductsForSuper(context.Background(), &tenantID, ProductAnalyticsListRequest{}); err != nil {
		t.Fatalf("ProductsForSuper tenant: %v", err)
	}
	if repo.listScope != analytics.TenantScope(9) {
		t.Fatalf("super tenant list scope = %+v", repo.listScope)
	}
}

func TestProductAnalyticsServiceRejectsUnsafeListAndRankingInputs(t *testing.T) {
	service := newFixedAnalyticsService(&fakeProductAnalyticsRepository{})
	for _, request := range []ProductAnalyticsListRequest{
		{Sort: "clicks;drop"},
		{Order: "desc nulls first"},
		{Status: "deleted' OR 1=1"},
		{Page: -1},
		{PageSize: analytics.MaxReportPageSize + 1},
	} {
		if _, err := service.ProductsForTenant(context.Background(), 7, request); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("ProductsForTenant(%+v) error = %v", request, err)
		}
	}
	overLimit := analytics.MaxProductRankingThreshold + 1
	negative := -1
	for _, request := range []TopProductAnalyticsRequest{
		{Metric: "orders"},
		{Limit: analytics.MaxProductRankingLimit + 1},
		{MinImpressions: &negative},
		{MinClicks: &overLimit},
	} {
		if _, err := service.TopProductsForTenant(context.Background(), 7, request); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("TopProductsForTenant(%+v) error = %v", request, err)
		}
	}
}

func TestProductAnalyticsServiceRankingDefaultsAndThresholds(t *testing.T) {
	repo := &fakeProductAnalyticsRepository{}
	service := newFixedAnalyticsService(repo)
	if _, err := service.TopProductsForTenant(context.Background(), 7, TopProductAnalyticsRequest{Metric: "ctr"}); err != nil {
		t.Fatalf("TopProductsForTenant CTR: %v", err)
	}
	if repo.topScope != analytics.TenantScope(7) || repo.topQuery.Limit != 10 || repo.topQuery.MinImpressions != 100 || repo.topQuery.MinClicks != 20 || repo.topQuery.MinViews != 10 {
		t.Fatalf("top defaults = scope %+v query %+v", repo.topScope, repo.topQuery)
	}
	zero := 0
	if _, err := service.TopProductsForTenant(context.Background(), 7, TopProductAnalyticsRequest{
		Metric: "detail_view_rate", MinClicks: &zero,
	}); err != nil || repo.topQuery.MinClicks != 0 {
		t.Fatalf("explicit zero min clicks query = %+v err=%v", repo.topQuery, err)
	}
	if _, err := service.LowCTRProductsForTenant(context.Background(), 7, LowCTRProductAnalyticsRequest{}); err != nil {
		t.Fatalf("LowCTRProductsForTenant: %v", err)
	}
	if repo.lowScope != analytics.TenantScope(7) || repo.lowMinimum != 100 || repo.lowLimit != 10 {
		t.Fatalf("low CTR defaults = scope %+v min %d limit %d", repo.lowScope, repo.lowMinimum, repo.lowLimit)
	}
}

func TestProductAnalyticsServiceDetailSupportsDeletedHistoryGapFillAndEventContextSources(t *testing.T) {
	sourceID := "all"
	repo := &fakeProductAnalyticsRepository{
		metadataErr: gorm.ErrRecordNotFound,
		summary: &analytics.ProductAggregate{
			ProductMetadata: analytics.ProductMetadata{ProductID: 19, ProductName: "Deleted product #19", TenantID: 7, CurrentlyExists: false},
			ProductCounts:   analytics.ProductCounts{Impressions: 10, Clicks: 2, Views: 1, ContactIntents: 1, UniqueVisitors: 4},
		},
		productTrend: []analytics.ProductTrendPoint{
			{Bucket: "2026-07-01", ProductCounts: analytics.ProductCounts{Impressions: 3, UniqueVisitors: 2}},
			{Bucket: "2026-07-03", ProductCounts: analytics.ProductCounts{Clicks: 1, UniqueVisitors: 1}},
		},
		productSources: []analytics.ProductSourceStat{{
			SourceType: "mall", SourceID: &sourceID,
			ProductCounts: analytics.ProductCounts{Impressions: 10, Clicks: 2, UniqueVisitors: 4},
		}},
		sourceLabels: []analytics.ProductSourceLabel{{SourceType: "mall", SourceID: "all", Label: "Mall"}},
	}
	service := newFixedAnalyticsService(repo)
	result, err := service.ProductDetailForTenant(context.Background(), 7, 19, ProductAnalyticsDetailRequest{
		DateRange: analytics.DateRangeInput{From: "2026-07-01", To: "2026-07-03"}, Granularity: "day",
	})
	if err != nil {
		t.Fatalf("ProductDetailForTenant: %v", err)
	}
	if repo.detailScope != analytics.TenantScope(7) || repo.detailProduct != 19 || result.Product.CurrentlyExists || result.Product.ProductSlug != "" {
		t.Fatalf("deleted product detail = scope %+v product %+v", repo.detailScope, result.Product)
	}
	if result.Summary.CTR != 0.2 || result.Summary.DetailViewRate != 0.5 || result.Summary.ContactIntentRate != 1 {
		t.Fatalf("detail rates = %+v", result.Summary)
	}
	if len(result.Trend) != 3 || result.Trend[1].Bucket != "2026-07-02" || result.Trend[1].Impressions != 0 || result.Trend[0].UniqueVisitors != 2 {
		t.Fatalf("filled product trend = %+v", result.Trend)
	}
	if len(result.Sources) != 1 || result.Sources[0].CTR != 0.2 || result.Sources[0].SourceLabel == nil || *result.Sources[0].SourceLabel != "Mall" {
		t.Fatalf("event-context sources = %+v", result.Sources)
	}
}

func TestProductAnalyticsServiceDetailReturnsZeroForCurrentProductAnd404ForUnknown(t *testing.T) {
	repo := &fakeProductAnalyticsRepository{
		metadata:     &analytics.ProductMetadata{ProductID: 22, ProductName: "Current", TenantID: 7, CurrentlyExists: true},
		summaryErr:   gorm.ErrRecordNotFound,
		productTrend: []analytics.ProductTrendPoint{},
	}
	service := newFixedAnalyticsService(repo)
	result, err := service.ProductDetailForTenant(context.Background(), 7, 22, ProductAnalyticsDetailRequest{
		DateRange: analytics.DateRangeInput{From: "2026-07-01", To: "2026-07-01"}, Granularity: "day",
	})
	if err != nil || result.Summary.Impressions != 0 || result.Product.ProductSlug != "current-22" {
		t.Fatalf("current zero-event detail = %+v err=%v", result, err)
	}

	repo.metadata, repo.metadataErr, repo.summaryErr = nil, gorm.ErrRecordNotFound, gorm.ErrRecordNotFound
	if _, err := service.ProductDetailForTenant(context.Background(), 7, 999, ProductAnalyticsDetailRequest{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown product detail error = %v, want ErrNotFound", err)
	}
}
