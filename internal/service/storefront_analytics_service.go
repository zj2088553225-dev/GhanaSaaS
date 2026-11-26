package service

import (
	"context"
	"errors"
	"time"

	"saas/internal/analytics"
)

type storefrontAnalyticsRepository interface {
	TenantExists(ctx context.Context, tenantID uint) (bool, error)
	Metrics(ctx context.Context, scope analytics.Scope, window analytics.TimeWindow) (analytics.EventCounts, error)
	Trends(ctx context.Context, scope analytics.Scope, dateRange analytics.DateRange, granularity analytics.Granularity) ([]analytics.SparseTrendPoint, error)
	Sources(ctx context.Context, scope analytics.Scope, window analytics.TimeWindow, sort analytics.SourceSort, limit int) ([]analytics.SourceStat, error)
}

type storefrontAnalyticsStatusRepository interface {
	AnalyticsStatus(ctx context.Context, now time.Time) (analytics.DataStatus, error)
}

type StorefrontAnalyticsService struct {
	repo            storefrontAnalyticsRepository
	productRepo     storefrontProductAnalyticsRepository
	contentRepo     storefrontContentAnalyticsRepository
	contentResolver storefrontMarketingContentResolver
	now             func() time.Time
}

type StorefrontAnalyticsOptions struct {
	Now             func() time.Time
	ContentResolver storefrontMarketingContentResolver
}

func NewStorefrontAnalyticsService(repo storefrontAnalyticsRepository, options ...StorefrontAnalyticsOptions) *StorefrontAnalyticsService {
	service := &StorefrontAnalyticsService{repo: repo, now: time.Now}
	if productRepo, ok := repo.(storefrontProductAnalyticsRepository); ok {
		service.productRepo = productRepo
	}
	if contentRepo, ok := repo.(storefrontContentAnalyticsRepository); ok {
		service.contentRepo = contentRepo
	}
	if len(options) > 0 {
		if options[0].Now != nil {
			service.now = options[0].Now
		}
		service.contentResolver = options[0].ContentResolver
	}
	return service
}

type AnalyticsMetric struct {
	Value         float64  `json:"value"`
	PreviousValue *float64 `json:"previous_value"`
	ChangeRate    *float64 `json:"change_rate"`
}

type AnalyticsOverviewMetrics struct {
	Visitors           AnalyticsMetric `json:"visitors"`
	Sessions           AnalyticsMetric `json:"sessions"`
	PageViews          AnalyticsMetric `json:"page_views"`
	ProductImpressions AnalyticsMetric `json:"product_impressions"`
	ProductClicks      AnalyticsMetric `json:"product_clicks"`
	ProductViews       AnalyticsMetric `json:"product_views"`
	ContactIntents     AnalyticsMetric `json:"contact_intents"`
	ProductCTR         AnalyticsMetric `json:"product_ctr"`
	DetailViewRate     AnalyticsMetric `json:"detail_view_rate"`
	ContactIntentRate  AnalyticsMetric `json:"contact_intent_rate"`
}

type AnalyticsOverviewRequest struct {
	DateRange analytics.DateRangeInput
	Compare   bool
}

type AnalyticsOverviewResult struct {
	Range   analytics.DateRange      `json:"range"`
	Metrics AnalyticsOverviewMetrics `json:"metrics"`
}

type AnalyticsTrendsRequest struct {
	DateRange   analytics.DateRangeInput
	Granularity string
}

type AnalyticsTrendsResult struct {
	Granularity analytics.Granularity        `json:"granularity"`
	Timezone    string                       `json:"timezone"`
	Series      []analytics.SparseTrendPoint `json:"series"`
}

type AnalyticsSourcesRequest struct {
	DateRange analytics.DateRangeInput
	Sort      string
	Limit     int
}

type AnalyticsSourcesResult struct {
	Range   analytics.DateRange    `json:"range"`
	Sources []analytics.SourceStat `json:"sources"`
}

func (s *StorefrontAnalyticsService) OverviewForSuper(
	ctx context.Context,
	tenantID *uint,
	request AnalyticsOverviewRequest,
) (*AnalyticsOverviewResult, error) {
	scope, err := s.superScope(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return s.overview(ctx, scope, request)
}

func (s *StorefrontAnalyticsService) OverviewForTenant(
	ctx context.Context,
	tenantID uint,
	request AnalyticsOverviewRequest,
) (*AnalyticsOverviewResult, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}
	return s.overview(ctx, analytics.TenantScope(tenantID), request)
}

func (s *StorefrontAnalyticsService) TrendsForSuper(
	ctx context.Context,
	tenantID *uint,
	request AnalyticsTrendsRequest,
) (*AnalyticsTrendsResult, error) {
	scope, err := s.superScope(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return s.trends(ctx, scope, request)
}

func (s *StorefrontAnalyticsService) TrendsForTenant(
	ctx context.Context,
	tenantID uint,
	request AnalyticsTrendsRequest,
) (*AnalyticsTrendsResult, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}
	return s.trends(ctx, analytics.TenantScope(tenantID), request)
}

func (s *StorefrontAnalyticsService) SourcesForSuper(
	ctx context.Context,
	tenantID *uint,
	request AnalyticsSourcesRequest,
) (*AnalyticsSourcesResult, error) {
	scope, err := s.superScope(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return s.sources(ctx, scope, request)
}

func (s *StorefrontAnalyticsService) SourcesForTenant(
	ctx context.Context,
	tenantID uint,
	request AnalyticsSourcesRequest,
) (*AnalyticsSourcesResult, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}
	return s.sources(ctx, analytics.TenantScope(tenantID), request)
}

func (s *StorefrontAnalyticsService) StatusForSuper(ctx context.Context) (*analytics.DataStatus, error) {
	repository, ok := s.repo.(storefrontAnalyticsStatusRepository)
	if !ok {
		return nil, errors.New("analytics status repository is not configured")
	}
	status, err := repository.AnalyticsStatus(ctx, s.now().UTC())
	if err != nil {
		return nil, err
	}
	return &status, nil
}

func (s *StorefrontAnalyticsService) superScope(ctx context.Context, tenantID *uint) (analytics.Scope, error) {
	if tenantID == nil {
		return analytics.AllScope(), nil
	}
	if *tenantID == 0 {
		return analytics.Scope{}, ErrInvalidInput
	}
	exists, err := s.repo.TenantExists(ctx, *tenantID)
	if err != nil {
		return analytics.Scope{}, err
	}
	if !exists {
		return analytics.Scope{}, ErrNotFound
	}
	return analytics.TenantScope(*tenantID), nil
}

func (s *StorefrontAnalyticsService) overview(
	ctx context.Context,
	scope analytics.Scope,
	request AnalyticsOverviewRequest,
) (*AnalyticsOverviewResult, error) {
	dateRange, err := s.parseDateRange(request.DateRange, analytics.MaxStandardRangeDays)
	if err != nil {
		return nil, err
	}
	current, err := s.repo.Metrics(ctx, scope, dateRange.Window())
	if err != nil {
		return nil, err
	}
	var previous *analytics.EventCounts
	if request.Compare {
		value, err := s.repo.Metrics(ctx, scope, dateRange.PreviousWindow())
		if err != nil {
			return nil, err
		}
		previous = &value
	}
	return &AnalyticsOverviewResult{
		Range:   dateRange,
		Metrics: buildOverviewMetrics(current, previous),
	}, nil
}

func (s *StorefrontAnalyticsService) trends(
	ctx context.Context,
	scope analytics.Scope,
	request AnalyticsTrendsRequest,
) (*AnalyticsTrendsResult, error) {
	dateRange, err := s.parseDateRange(request.DateRange, analytics.MaxWeeklyRangeDays)
	if err != nil {
		return nil, err
	}
	granularity, ok := analytics.ResolveGranularity(request.Granularity, dateRange.Days)
	if !ok {
		return nil, ErrInvalidInput
	}
	sparse, err := s.repo.Trends(ctx, scope, dateRange, granularity)
	if err != nil {
		return nil, err
	}
	return &AnalyticsTrendsResult{
		Granularity: granularity,
		Timezone:    dateRange.Timezone,
		Series:      fillTrendBuckets(dateRange, granularity, sparse),
	}, nil
}

func (s *StorefrontAnalyticsService) sources(
	ctx context.Context,
	scope analytics.Scope,
	request AnalyticsSourcesRequest,
) (*AnalyticsSourcesResult, error) {
	dateRange, err := s.parseDateRange(request.DateRange, analytics.MaxStandardRangeDays)
	if err != nil {
		return nil, err
	}
	sort, ok := analytics.ParseSourceSort(request.Sort)
	if !ok {
		return nil, ErrInvalidInput
	}
	limit := request.Limit
	if limit == 0 {
		limit = analytics.DefaultSourceLimit
	}
	if limit < 1 || limit > analytics.MaxSourceLimit {
		return nil, ErrInvalidInput
	}
	stats, err := s.repo.Sources(ctx, scope, dateRange.Window(), sort, limit)
	if err != nil {
		return nil, err
	}
	return &AnalyticsSourcesResult{Range: dateRange, Sources: stats}, nil
}

func (s *StorefrontAnalyticsService) parseDateRange(input analytics.DateRangeInput, maxDays int) (analytics.DateRange, error) {
	dateRange, err := analytics.ParseDateRange(input, s.now(), maxDays)
	if err != nil {
		return analytics.DateRange{}, ErrInvalidInput
	}
	return dateRange, nil
}

func buildOverviewMetrics(current analytics.EventCounts, previous *analytics.EventCounts) AnalyticsOverviewMetrics {
	currentRates := current.Rates()
	if previous == nil {
		return AnalyticsOverviewMetrics{
			Visitors:           analyticsMetric(float64(current.Visitors), nil),
			Sessions:           analyticsMetric(float64(current.Sessions), nil),
			PageViews:          analyticsMetric(float64(current.PageViews), nil),
			ProductImpressions: analyticsMetric(float64(current.ProductImpressions), nil),
			ProductClicks:      analyticsMetric(float64(current.ProductClicks), nil),
			ProductViews:       analyticsMetric(float64(current.ProductViews), nil),
			ContactIntents:     analyticsMetric(float64(current.ContactIntents), nil),
			ProductCTR:         analyticsMetric(currentRates.ProductCTR, nil),
			DetailViewRate:     analyticsMetric(currentRates.DetailViewRate, nil),
			ContactIntentRate:  analyticsMetric(currentRates.ContactIntentRate, nil),
		}
	}
	previousRates := previous.Rates()
	return AnalyticsOverviewMetrics{
		Visitors:           analyticsMetric(float64(current.Visitors), floatPointer(float64(previous.Visitors))),
		Sessions:           analyticsMetric(float64(current.Sessions), floatPointer(float64(previous.Sessions))),
		PageViews:          analyticsMetric(float64(current.PageViews), floatPointer(float64(previous.PageViews))),
		ProductImpressions: analyticsMetric(float64(current.ProductImpressions), floatPointer(float64(previous.ProductImpressions))),
		ProductClicks:      analyticsMetric(float64(current.ProductClicks), floatPointer(float64(previous.ProductClicks))),
		ProductViews:       analyticsMetric(float64(current.ProductViews), floatPointer(float64(previous.ProductViews))),
		ContactIntents:     analyticsMetric(float64(current.ContactIntents), floatPointer(float64(previous.ContactIntents))),
		ProductCTR:         analyticsMetric(currentRates.ProductCTR, floatPointer(previousRates.ProductCTR)),
		DetailViewRate:     analyticsMetric(currentRates.DetailViewRate, floatPointer(previousRates.DetailViewRate)),
		ContactIntentRate:  analyticsMetric(currentRates.ContactIntentRate, floatPointer(previousRates.ContactIntentRate)),
	}
}

func analyticsMetric(value float64, previous *float64) AnalyticsMetric {
	metric := AnalyticsMetric{Value: value, PreviousValue: previous}
	if previous != nil {
		metric.ChangeRate = analytics.ChangeRate(value, *previous)
	}
	return metric
}

func floatPointer(value float64) *float64 {
	result := value
	return &result
}

func fillTrendBuckets(
	dateRange analytics.DateRange,
	granularity analytics.Granularity,
	sparse []analytics.SparseTrendPoint,
) []analytics.SparseTrendPoint {
	byBucket := make(map[string]analytics.SparseTrendPoint, len(sparse))
	for _, point := range sparse {
		byBucket[point.Bucket] = point
	}

	buckets := analyticsBucketKeys(dateRange, granularity)
	series := make([]analytics.SparseTrendPoint, 0, len(buckets))
	for _, bucket := range buckets {
		point, ok := byBucket[bucket]
		if !ok {
			point = analytics.SparseTrendPoint{Bucket: bucket}
		}
		series = append(series, point)
	}
	return series
}

func analyticsBucketKeys(dateRange analytics.DateRange, granularity analytics.Granularity) []string {
	current := dateRange.FromUTC.In(dateRange.Location)
	current = time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, dateRange.Location)
	if granularity == analytics.GranularityWeek {
		weekdayOffset := (int(current.Weekday()) + 6) % 7
		current = current.AddDate(0, 0, -weekdayOffset)
	}

	buckets := make([]string, 0)
	for current.UTC().Before(dateRange.ToUTC) {
		buckets = append(buckets, formatTrendBucket(current, granularity))
		switch granularity {
		case analytics.GranularityHour:
			current = current.Add(time.Hour)
		case analytics.GranularityWeek:
			current = current.AddDate(0, 0, 7)
		default:
			current = current.AddDate(0, 0, 1)
		}
	}
	return buckets
}

func formatTrendBucket(value time.Time, granularity analytics.Granularity) string {
	if granularity == analytics.GranularityHour {
		return value.Format("2006-01-02T15:00:00")
	}
	return value.Format("2006-01-02")
}
