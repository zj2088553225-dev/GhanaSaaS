package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"saas/internal/analytics"
)

type fakeContentAnalyticsRepository struct {
	fakeStorefrontAnalyticsRepository
	rows             []analytics.ContentAggregate
	total            int64
	listWindow       analytics.TimeWindow
	listQuery        analytics.ContentAnalyticsQuery
	summary          analytics.ContentCounts
	summaryWindow    analytics.TimeWindow
	detailKey        string
	everExisted      bool
	everCalls        int
	trend            []analytics.ContentTrendPoint
	trendGranularity analytics.Granularity
	locales          []analytics.ContentLocaleStat
	devices          []analytics.ContentDeviceStat
	pages            []analytics.ContentPageStat
	contentErr       error
}

func (repo *fakeContentAnalyticsRepository) ListContentAnalytics(
	_ context.Context,
	window analytics.TimeWindow,
	query analytics.ContentAnalyticsQuery,
) ([]analytics.ContentAggregate, int64, error) {
	repo.listWindow, repo.listQuery = window, query
	return repo.rows, repo.total, repo.contentErr
}

func (repo *fakeContentAnalyticsRepository) GetContentSummary(
	_ context.Context,
	window analytics.TimeWindow,
	trackingKey string,
) (analytics.ContentCounts, error) {
	repo.summaryWindow, repo.detailKey = window, trackingKey
	return repo.summary, repo.contentErr
}

func (repo *fakeContentAnalyticsRepository) ContentEverExisted(_ context.Context, trackingKey string) (bool, error) {
	repo.everCalls++
	repo.detailKey = trackingKey
	return repo.everExisted, repo.contentErr
}

func (repo *fakeContentAnalyticsRepository) GetContentTrend(
	_ context.Context,
	_ analytics.DateRange,
	granularity analytics.Granularity,
	trackingKey string,
) ([]analytics.ContentTrendPoint, error) {
	repo.trendGranularity, repo.detailKey = granularity, trackingKey
	return repo.trend, repo.contentErr
}

func (repo *fakeContentAnalyticsRepository) GetContentLocaleBreakdown(
	_ context.Context,
	_ analytics.TimeWindow,
	trackingKey string,
) ([]analytics.ContentLocaleStat, error) {
	repo.detailKey = trackingKey
	return repo.locales, repo.contentErr
}

func (repo *fakeContentAnalyticsRepository) GetContentDeviceBreakdown(
	_ context.Context,
	_ analytics.TimeWindow,
	trackingKey string,
) ([]analytics.ContentDeviceStat, error) {
	repo.detailKey = trackingKey
	return repo.devices, repo.contentErr
}

func (repo *fakeContentAnalyticsRepository) GetContentPageBreakdown(
	_ context.Context,
	_ analytics.TimeWindow,
	trackingKey string,
) ([]analytics.ContentPageStat, error) {
	repo.detailKey = trackingKey
	return repo.pages, repo.contentErr
}

type fakeMarketingContentResolver struct {
	metadata map[string]analytics.MarketingContentMetadata
	err      error
	calls    int
}

func (resolver *fakeMarketingContentResolver) Resolve(_ context.Context) (map[string]analytics.MarketingContentMetadata, error) {
	resolver.calls++
	return resolver.metadata, resolver.err
}

func newFixedContentAnalyticsService(
	repo *fakeContentAnalyticsRepository,
	resolver *fakeMarketingContentResolver,
) *StorefrontAnalyticsService {
	return NewStorefrontAnalyticsService(repo, StorefrontAnalyticsOptions{
		Now: func() time.Time {
			return time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
		},
		ContentResolver: resolver,
	})
}

func TestContentAnalyticsServiceNormalListUsesDBPaginationSortRangeAndMetadataMerge(t *testing.T) {
	const (
		currentKey = "11111111-1111-4111-8111-111111111111"
		deletedKey = "22222222-2222-4222-8222-222222222222"
	)
	repo := &fakeContentAnalyticsRepository{
		rows: []analytics.ContentAggregate{
			{TrackingKey: currentKey, ContentCounts: analytics.ContentCounts{Impressions: 200, Clicks: 25, UniqueVisitors: 18}},
			{TrackingKey: deletedKey, ContentCounts: analytics.ContentCounts{Impressions: 50, Clicks: 5, UniqueVisitors: 9}},
		},
		total: 2,
	}
	resolver := &fakeMarketingContentResolver{metadata: map[string]analytics.MarketingContentMetadata{
		currentKey: {
			TrackingKey: currentKey, ContentType: analytics.ContentTypePromoBanner, Title: "English Promo",
			ImageURL: "/promo.webp", TargetType: analytics.ContentTargetInternal, TargetValue: "/mall", Published: true,
		},
	}}
	service := newFixedContentAnalyticsService(repo, resolver)
	result, err := service.ContentForSuper(context.Background(), ContentAnalyticsListRequest{
		DateRange: analytics.DateRangeInput{From: "2026-07-01", To: "2026-07-14", Timezone: "Asia/Shanghai"},
	})
	if err != nil {
		t.Fatalf("ContentForSuper: %v", err)
	}
	if resolver.calls != 1 || !repo.listQuery.Paginate || repo.listQuery.Page != 1 || repo.listQuery.PageSize != 20 || repo.listQuery.Sort != analytics.ContentMetricClicks || repo.listQuery.Order != analytics.SortOrderDescending {
		t.Fatalf("normal path resolver/query calls=%d query=%+v", resolver.calls, repo.listQuery)
	}
	if got, want := repo.listWindow.FromUTC.Format(time.RFC3339), "2026-06-30T16:00:00Z"; got != want {
		t.Fatalf("FromUTC = %s, want %s", got, want)
	}
	if result.Total != 2 || len(result.Items) != 2 || result.Items[0].CTR != 0.125 || !result.Items[0].CurrentlyExists || !result.Items[0].Published {
		t.Fatalf("content list = %+v", result)
	}
	deleted := result.Items[1]
	if deleted.TrackingKey != deletedKey || deleted.CurrentlyExists || deleted.Published || deleted.ContentType != analytics.ContentTypeOther || deleted.Title != "Deleted marketing content" || deleted.CTR != 0.1 {
		t.Fatalf("deleted content item = %+v", deleted)
	}
}

func TestContentAnalyticsServiceMetadataFilterUsesOnlyAggregateRowsThenPaginates(t *testing.T) {
	const (
		heroKey    = "11111111-1111-4111-8111-111111111111"
		promoKey   = "22222222-2222-4222-8222-222222222222"
		deletedKey = "33333333-3333-4333-8333-333333333333"
	)
	repo := &fakeContentAnalyticsRepository{rows: []analytics.ContentAggregate{
		{TrackingKey: heroKey, ContentCounts: analytics.ContentCounts{Impressions: 300, Clicks: 30}},
		{TrackingKey: promoKey, ContentCounts: analytics.ContentCounts{Impressions: 200, Clicks: 10}},
		{TrackingKey: deletedKey, ContentCounts: analytics.ContentCounts{Impressions: 150, Clicks: 1}},
	}}
	resolver := &fakeMarketingContentResolver{metadata: map[string]analytics.MarketingContentMetadata{
		heroKey:  {TrackingKey: heroKey, ContentType: analytics.ContentTypeHero, Title: "Summer Hero", Published: true},
		promoKey: {TrackingKey: promoKey, ContentType: analytics.ContentTypePromoBanner, Title: "Summer Sale", Published: false},
	}}
	service := newFixedContentAnalyticsService(repo, resolver)
	exists := true
	result, err := service.ContentForSuper(context.Background(), ContentAnalyticsListRequest{
		Page: 1, PageSize: 1, Search: "sale", ContentType: "promo_banner", CurrentlyExists: &exists,
	})
	if err != nil {
		t.Fatalf("metadata-filter ContentForSuper: %v", err)
	}
	if repo.listQuery.Paginate || result.Total != 1 || len(result.Items) != 1 || result.Items[0].TrackingKey != promoKey || result.Items[0].Published {
		t.Fatalf("metadata aggregate-row path query=%+v result=%+v", repo.listQuery, result)
	}
	if resolver.calls != 1 {
		t.Fatalf("resolver calls = %d, want once per request", resolver.calls)
	}

	resolver.calls = 0
	notExists := false
	result, err = service.ContentForSuper(context.Background(), ContentAnalyticsListRequest{
		Search: deletedKey[:12], CurrentlyExists: &notExists,
	})
	if err != nil || result.Total != 1 || len(result.Items) != 1 || result.Items[0].TrackingKey != deletedKey || result.Items[0].CurrentlyExists {
		t.Fatalf("deleted tracking-key search = %+v err=%v", result, err)
	}
	if resolver.calls != 1 {
		t.Fatalf("deleted search resolver calls = %d", resolver.calls)
	}
}

func TestContentAnalyticsServiceCTRThresholdAndInputAllowlists(t *testing.T) {
	repo := &fakeContentAnalyticsRepository{}
	resolver := &fakeMarketingContentResolver{metadata: map[string]analytics.MarketingContentMetadata{}}
	service := newFixedContentAnalyticsService(repo, resolver)
	if _, err := service.ContentForSuper(context.Background(), ContentAnalyticsListRequest{Sort: "ctr"}); err != nil {
		t.Fatalf("default CTR request: %v", err)
	}
	if repo.listQuery.MinImpressions != analytics.DefaultMinImpressions {
		t.Fatalf("default min impressions = %d", repo.listQuery.MinImpressions)
	}
	zero := 0
	if _, err := service.ContentForSuper(context.Background(), ContentAnalyticsListRequest{Sort: "ctr", MinImpressions: &zero}); err != nil || repo.listQuery.MinImpressions != 0 {
		t.Fatalf("explicit zero threshold query=%+v err=%v", repo.listQuery, err)
	}
	tooHigh := analytics.MaxContentAnalyticsThreshold + 1
	for _, request := range []ContentAnalyticsListRequest{
		{PageSize: analytics.MaxReportPageSize + 1},
		{ContentType: "hero_slide;drop"},
		{Sort: "clicks;drop"},
		{Order: "desc nulls first"},
		{MinImpressions: &tooHigh},
	} {
		if _, err := service.ContentForSuper(context.Background(), request); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("invalid request %+v error=%v", request, err)
		}
	}
}

func TestContentAnalyticsServiceDetailCurrentDeletedHistoricalAndNeverExisted(t *testing.T) {
	const key = "11111111-1111-4111-8111-111111111111"
	repo := &fakeContentAnalyticsRepository{
		summary: analytics.ContentCounts{Impressions: 10, Clicks: 2, UniqueVisitors: 4},
		trend: []analytics.ContentTrendPoint{
			{Bucket: "2026-07-01", ContentCounts: analytics.ContentCounts{Impressions: 3, UniqueVisitors: 2}},
			{Bucket: "2026-07-03", ContentCounts: analytics.ContentCounts{Clicks: 1, UniqueVisitors: 1}},
		},
		locales: []analytics.ContentLocaleStat{
			{Locale: "en", ContentCounts: analytics.ContentCounts{Impressions: 8, Clicks: 2, UniqueVisitors: 3}},
			{Locale: "legacy", ContentCounts: analytics.ContentCounts{Impressions: 2, UniqueVisitors: 1}},
		},
		devices: []analytics.ContentDeviceStat{
			{DeviceType: "mobile", ContentCounts: analytics.ContentCounts{Impressions: 7, Clicks: 2, UniqueVisitors: 3}},
			{DeviceType: "television", ContentCounts: analytics.ContentCounts{Impressions: 3, UniqueVisitors: 1}},
		},
		pages: []analytics.ContentPageStat{{PagePath: "/en", Impressions: 10, Clicks: 2}},
	}
	resolver := &fakeMarketingContentResolver{metadata: map[string]analytics.MarketingContentMetadata{
		key: {TrackingKey: key, ContentType: analytics.ContentTypeHeroSlide, Title: "Current", Published: false},
	}}
	service := newFixedContentAnalyticsService(repo, resolver)
	result, err := service.ContentDetailForSuper(context.Background(), key, ContentAnalyticsDetailRequest{
		DateRange: analytics.DateRangeInput{From: "2026-07-01", To: "2026-07-03"}, Granularity: "day",
	})
	if err != nil {
		t.Fatalf("current ContentDetailForSuper: %v", err)
	}
	if resolver.calls != 1 || repo.everCalls != 0 || !result.Content.CurrentlyExists || result.Content.Published || result.Summary.CTR != 0.2 {
		t.Fatalf("current detail = %+v resolver=%d ever=%d", result, resolver.calls, repo.everCalls)
	}
	if len(result.Trend) != 3 || result.Trend[1].Bucket != "2026-07-02" || result.Trend[1].Impressions != 0 || result.Trend[0].UniqueVisitors != 2 {
		t.Fatalf("filled trend = %+v", result.Trend)
	}
	if len(result.Locales) != 3 || result.Locales[0].Locale != "en" || result.Locales[1].Locale != "zh" || result.Locales[2].Locale != "other" || result.Locales[2].Impressions != 2 {
		t.Fatalf("fixed locale breakdown = %+v", result.Locales)
	}
	if len(result.Devices) != 4 || result.Devices[0].DeviceType != "desktop" || result.Devices[1].DeviceType != "mobile" || result.Devices[3].DeviceType != "unknown" || result.Devices[3].Impressions != 3 {
		t.Fatalf("fixed device breakdown = %+v", result.Devices)
	}

	// Current metadata alone is sufficient for a 200 response when the selected
	// period has no events.
	repo.summary = analytics.ContentCounts{}
	repo.trend, repo.locales, repo.devices, repo.pages = nil, nil, nil, nil
	resolver.calls, repo.everCalls = 0, 0
	currentZero, err := service.ContentDetailForSuper(context.Background(), key, ContentAnalyticsDetailRequest{
		DateRange: analytics.DateRangeInput{From: "2026-07-08", To: "2026-07-14"}, Granularity: "day",
	})
	if err != nil || !currentZero.Content.CurrentlyExists || currentZero.Summary != (ContentAnalyticsSummary{}) || repo.everCalls != 0 || resolver.calls != 1 {
		t.Fatalf("current zero-range detail=%+v err=%v resolver=%d ever=%d", currentZero, err, resolver.calls, repo.everCalls)
	}

	resolver.metadata = map[string]analytics.MarketingContentMetadata{}
	resolver.calls, repo.everCalls = 0, 0
	repo.everExisted = true
	deleted, err := service.ContentDetailForSuper(context.Background(), key, ContentAnalyticsDetailRequest{
		DateRange: analytics.DateRangeInput{From: "2026-07-08", To: "2026-07-14"}, Granularity: "day",
	})
	if err != nil || deleted.Content.CurrentlyExists || deleted.Content.Published || deleted.Summary != (ContentAnalyticsSummary{}) || repo.everCalls != 1 || resolver.calls != 1 {
		t.Fatalf("historical deleted zero-range detail=%+v err=%v resolver=%d ever=%d", deleted, err, resolver.calls, repo.everCalls)
	}

	repo.everExisted = false
	if _, err := service.ContentDetailForSuper(context.Background(), key, ContentAnalyticsDetailRequest{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("never-existed detail error=%v", err)
	}
	if _, err := service.ContentDetailForSuper(context.Background(), "not-a-tracking-key", ContentAnalyticsDetailRequest{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid tracking key error=%v", err)
	}
}
