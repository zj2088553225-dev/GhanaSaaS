package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"saas/internal/analytics"
)

type fakeStorefrontAnalyticsRepository struct {
	tenantExists map[uint]bool
	metrics      []analytics.EventCounts
	metricScopes []analytics.Scope
	windows      []analytics.TimeWindow
	trend        []analytics.SparseTrendPoint
	trendScope   analytics.Scope
	trendRange   analytics.DateRange
	trendGran    analytics.Granularity
	sources      []analytics.SourceStat
	sourceScope  analytics.Scope
	sourceSort   analytics.SourceSort
	sourceLimit  int
	status       analytics.DataStatus
	statusNow    time.Time
	err          error
}

func (repo *fakeStorefrontAnalyticsRepository) AnalyticsStatus(_ context.Context, now time.Time) (analytics.DataStatus, error) {
	repo.statusNow = now
	return repo.status, repo.err
}

func (repo *fakeStorefrontAnalyticsRepository) TenantExists(_ context.Context, tenantID uint) (bool, error) {
	if repo.err != nil {
		return false, repo.err
	}
	return repo.tenantExists[tenantID], nil
}

func (repo *fakeStorefrontAnalyticsRepository) Metrics(_ context.Context, scope analytics.Scope, window analytics.TimeWindow) (analytics.EventCounts, error) {
	if repo.err != nil {
		return analytics.EventCounts{}, repo.err
	}
	repo.metricScopes = append(repo.metricScopes, scope)
	repo.windows = append(repo.windows, window)
	index := len(repo.metricScopes) - 1
	if index >= len(repo.metrics) {
		return analytics.EventCounts{}, nil
	}
	return repo.metrics[index], nil
}

func (repo *fakeStorefrontAnalyticsRepository) Trends(_ context.Context, scope analytics.Scope, dateRange analytics.DateRange, granularity analytics.Granularity) ([]analytics.SparseTrendPoint, error) {
	if repo.err != nil {
		return nil, repo.err
	}
	repo.trendScope = scope
	repo.trendRange = dateRange
	repo.trendGran = granularity
	return repo.trend, nil
}

func (repo *fakeStorefrontAnalyticsRepository) Sources(_ context.Context, scope analytics.Scope, _ analytics.TimeWindow, sort analytics.SourceSort, limit int) ([]analytics.SourceStat, error) {
	if repo.err != nil {
		return nil, repo.err
	}
	repo.sourceScope = scope
	repo.sourceSort = sort
	repo.sourceLimit = limit
	return repo.sources, nil
}

func TestStorefrontAnalyticsOverviewUsesUnifiedMetricsAndCompareSemantics(t *testing.T) {
	repo := &fakeStorefrontAnalyticsRepository{metrics: []analytics.EventCounts{
		{Visitors: 3, Sessions: 4, PageViews: 0, ProductImpressions: 10, ProductClicks: 2, ProductViews: 1, ContactIntents: 1},
		{},
	}}
	service := newFixedAnalyticsService(repo)
	result, err := service.OverviewForSuper(context.Background(), nil, AnalyticsOverviewRequest{
		DateRange: analytics.DateRangeInput{From: "2026-07-08", To: "2026-07-14"},
		Compare:   true,
	})
	if err != nil {
		t.Fatalf("OverviewForSuper: %v", err)
	}
	if len(repo.metricScopes) != 2 || repo.metricScopes[0].Mode != analytics.ScopeAll || repo.metricScopes[1].Mode != analytics.ScopeAll {
		t.Fatalf("metric scopes = %+v", repo.metricScopes)
	}
	if !repo.windows[0].FromUTC.Equal(time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)) ||
		!repo.windows[0].ToUTC.Equal(time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("current window = %+v", repo.windows[0])
	}
	if !repo.windows[1].FromUTC.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) ||
		!repo.windows[1].ToUTC.Equal(time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("previous window = %+v", repo.windows[1])
	}
	if result.Metrics.ProductCTR.Value != 0.2 || result.Metrics.DetailViewRate.Value != 0.5 || result.Metrics.ContactIntentRate.Value != 1 {
		t.Fatalf("funnel rates = %+v", result.Metrics)
	}
	if result.Metrics.ProductClicks.PreviousValue == nil || *result.Metrics.ProductClicks.PreviousValue != 0 || result.Metrics.ProductClicks.ChangeRate != nil {
		t.Fatalf("new clicks comparison = %+v", result.Metrics.ProductClicks)
	}
	if result.Metrics.PageViews.ChangeRate == nil || *result.Metrics.PageViews.ChangeRate != 0 {
		t.Fatalf("zero-to-zero comparison = %+v", result.Metrics.PageViews)
	}
}

func TestStorefrontAnalyticsStatusUsesServerUTCClock(t *testing.T) {
	latest := time.Date(2026, 7, 14, 11, 0, 0, 0, time.UTC)
	repo := &fakeStorefrontAnalyticsRepository{status: analytics.DataStatus{
		TotalEvents: 10, HumanEvents: 8, BotEvents: 2, LatestEventAt: &latest,
	}}
	tracker := newFixedAnalyticsService(repo)
	result, err := tracker.StatusForSuper(context.Background())
	if err != nil {
		t.Fatalf("StatusForSuper: %v", err)
	}
	if result.TotalEvents != 10 || result.HumanEvents != 8 || result.BotEvents != 2 {
		t.Fatalf("status result = %+v", result)
	}
	wantNow := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	if !repo.statusNow.Equal(wantNow) || repo.statusNow.Location() != time.UTC {
		t.Fatalf("status now = %s, want %s UTC", repo.statusNow, wantNow)
	}
}

func TestStorefrontAnalyticsOverviewTenantScopeCannotEscape(t *testing.T) {
	repo := &fakeStorefrontAnalyticsRepository{tenantExists: map[uint]bool{7: true}}
	service := newFixedAnalyticsService(repo)
	request := AnalyticsOverviewRequest{DateRange: analytics.DateRangeInput{From: "2026-07-08", To: "2026-07-08"}}

	if _, err := service.OverviewForTenant(context.Background(), 7, request); err != nil {
		t.Fatalf("OverviewForTenant: %v", err)
	}
	if len(repo.metricScopes) != 1 || repo.metricScopes[0] != analytics.TenantScope(7) {
		t.Fatalf("tenant metric scope = %+v", repo.metricScopes)
	}

	repo.metricScopes = nil
	tenantID := uint(7)
	if _, err := service.OverviewForSuper(context.Background(), &tenantID, request); err != nil {
		t.Fatalf("OverviewForSuper tenant: %v", err)
	}
	if len(repo.metricScopes) != 1 || repo.metricScopes[0] != analytics.TenantScope(7) {
		t.Fatalf("super tenant scope = %+v", repo.metricScopes)
	}

	missing := uint(8)
	if _, err := service.OverviewForSuper(context.Background(), &missing, request); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing tenant error = %v, want ErrNotFound", err)
	}
}

func TestStorefrontAnalyticsDateValidationIsShared(t *testing.T) {
	repo := &fakeStorefrontAnalyticsRepository{}
	service := newFixedAnalyticsService(repo)
	tests := []analytics.DateRangeInput{
		{From: "2026-07-14", To: "2026-07-01"},
		{From: "2026-07-01", To: "2027-07-02"},
		{From: "2026-07-01", To: "2026-07-02", Timezone: "Invalid/Timezone"},
	}
	for _, dateRange := range tests {
		_, err := service.OverviewForSuper(context.Background(), nil, AnalyticsOverviewRequest{DateRange: dateRange})
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("overview date range %+v error = %v", dateRange, err)
		}
		_, err = service.SourcesForSuper(context.Background(), nil, AnalyticsSourcesRequest{DateRange: dateRange})
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("sources date range %+v error = %v", dateRange, err)
		}
	}
}

func TestStorefrontAnalyticsTrendsFillMissingBucketsAndResolveGranularity(t *testing.T) {
	repo := &fakeStorefrontAnalyticsRepository{trend: []analytics.SparseTrendPoint{
		{Bucket: "2026-07-08", PageViews: 2},
		{Bucket: "2026-07-10", ProductClicks: 1},
	}}
	service := newFixedAnalyticsService(repo)
	result, err := service.TrendsForTenant(context.Background(), 7, AnalyticsTrendsRequest{
		DateRange:   analytics.DateRangeInput{From: "2026-07-08", To: "2026-07-10", Timezone: "Africa/Accra"},
		Granularity: "day",
	})
	if err != nil {
		t.Fatalf("TrendsForTenant: %v", err)
	}
	if repo.trendScope != analytics.TenantScope(7) || repo.trendGran != analytics.GranularityDay {
		t.Fatalf("trend query = scope %+v granularity %q", repo.trendScope, repo.trendGran)
	}
	if len(result.Series) != 3 || result.Series[1].Bucket != "2026-07-09" || result.Series[1].PageViews != 0 || result.Series[1].ProductClicks != 0 {
		t.Fatalf("filled trend = %+v", result.Series)
	}

	repo.trend = nil
	defaultResult, err := service.TrendsForSuper(context.Background(), nil, AnalyticsTrendsRequest{
		DateRange: analytics.DateRangeInput{From: "2026-07-08", To: "2026-07-09"},
	})
	if err != nil || defaultResult.Granularity != analytics.GranularityHour || len(defaultResult.Series) != 48 {
		t.Fatalf("default hourly trend = %+v, err=%v", defaultResult, err)
	}
}

func TestStorefrontAnalyticsTrendsRejectInvalidGranularityAndLimits(t *testing.T) {
	service := newFixedAnalyticsService(&fakeStorefrontAnalyticsRepository{})
	tests := []AnalyticsTrendsRequest{
		{DateRange: analytics.DateRangeInput{From: "2026-07-01", To: "2026-07-02"}, Granularity: "month"},
		{DateRange: analytics.DateRangeInput{From: "2026-01-01", To: "2026-02-01"}, Granularity: "hour"},
		{DateRange: analytics.DateRangeInput{From: "2025-01-01", To: "2027-01-03"}, Granularity: "week"},
	}
	for _, request := range tests {
		if _, err := service.TrendsForSuper(context.Background(), nil, request); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("TrendsForSuper(%+v) error = %v", request, err)
		}
	}
}

func TestStorefrontAnalyticsSourcesValidateSortAndLimit(t *testing.T) {
	repo := &fakeStorefrontAnalyticsRepository{sources: []analytics.SourceStat{{SourceType: "mall", Clicks: 4}}}
	service := newFixedAnalyticsService(repo)
	result, err := service.SourcesForTenant(context.Background(), 7, AnalyticsSourcesRequest{
		DateRange: analytics.DateRangeInput{From: "2026-07-01", To: "2026-07-02"},
		Sort:      "ctr",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("SourcesForTenant: %v", err)
	}
	if repo.sourceScope != analytics.TenantScope(7) || repo.sourceSort != analytics.SourceSortCTR || repo.sourceLimit != 10 || len(result.Sources) != 1 {
		t.Fatalf("source query/result = scope=%+v sort=%q limit=%d result=%+v", repo.sourceScope, repo.sourceSort, repo.sourceLimit, result)
	}
	for _, request := range []AnalyticsSourcesRequest{
		{DateRange: analytics.DateRangeInput{From: "2026-07-01", To: "2026-07-02"}, Sort: "ctr;drop"},
		{DateRange: analytics.DateRangeInput{From: "2026-07-01", To: "2026-07-02"}, Limit: analytics.MaxSourceLimit + 1},
	} {
		if _, err := service.SourcesForTenant(context.Background(), 7, request); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("SourcesForTenant(%+v) error = %v", request, err)
		}
	}
}

func TestStorefrontAnalyticsRepositoryErrorsPropagate(t *testing.T) {
	databaseError := errors.New("database unavailable")
	service := newFixedAnalyticsService(&fakeStorefrontAnalyticsRepository{err: databaseError})
	_, err := service.OverviewForSuper(context.Background(), nil, AnalyticsOverviewRequest{
		DateRange: analytics.DateRangeInput{From: "2026-07-01", To: "2026-07-02"},
	})
	if !errors.Is(err, databaseError) {
		t.Fatalf("OverviewForSuper error = %v", err)
	}
}

func newFixedAnalyticsService(repo storefrontAnalyticsRepository) *StorefrontAnalyticsService {
	return NewStorefrontAnalyticsService(repo, StorefrontAnalyticsOptions{
		Now: func() time.Time { return time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC) },
	})
}
