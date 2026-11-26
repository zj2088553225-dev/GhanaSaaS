package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"saas/internal/analytics"

	"gorm.io/gorm"
)

type storefrontProductAnalyticsRepository interface {
	ProductAnalyticsList(context.Context, analytics.Scope, analytics.TimeWindow, analytics.ProductAnalyticsQuery) ([]analytics.ProductAggregate, int64, error)
	TopProductAnalytics(context.Context, analytics.Scope, analytics.TimeWindow, analytics.ProductRankingQuery) ([]analytics.ProductAggregate, error)
	LowCTRProductAnalytics(context.Context, analytics.Scope, analytics.TimeWindow, int64, int) ([]analytics.ProductAggregate, error)
	CurrentProductAnalyticsMetadata(context.Context, analytics.Scope, uint) (*analytics.ProductMetadata, error)
	ProductAnalyticsSummary(context.Context, analytics.Scope, analytics.TimeWindow, uint) (*analytics.ProductAggregate, error)
	ProductAnalyticsTrends(context.Context, analytics.Scope, analytics.DateRange, analytics.Granularity, uint) ([]analytics.ProductTrendPoint, error)
	ProductAnalyticsSources(context.Context, analytics.Scope, analytics.TimeWindow, uint) ([]analytics.ProductSourceStat, error)
	ProductAnalyticsSourceLabels(context.Context, []analytics.ProductSourceStat) ([]analytics.ProductSourceLabel, error)
}

type ProductAnalyticsListRequest struct {
	DateRange analytics.DateRangeInput
	Page      int
	PageSize  int
	Search    string
	Status    string
	Sort      string
	Order     string
}

type TopProductAnalyticsRequest struct {
	DateRange      analytics.DateRangeInput
	Metric         string
	Limit          int
	MinImpressions *int
	MinClicks      *int
	MinViews       *int
}

type LowCTRProductAnalyticsRequest struct {
	DateRange      analytics.DateRangeInput
	MinImpressions *int
	Limit          int
}

type ProductAnalyticsDetailRequest struct {
	DateRange   analytics.DateRangeInput
	Granularity string
}

type ProductAnalyticsItem struct {
	analytics.ProductMetadata
	analytics.ProductCounts
	CTR               float64 `json:"ctr"`
	DetailViewRate    float64 `json:"detail_view_rate"`
	ContactIntentRate float64 `json:"contact_intent_rate"`
}

type ProductAnalyticsSummary struct {
	analytics.ProductCounts
	CTR               float64 `json:"ctr"`
	DetailViewRate    float64 `json:"detail_view_rate"`
	ContactIntentRate float64 `json:"contact_intent_rate"`
}

type ProductAnalyticsListResult struct {
	Range    analytics.DateRange    `json:"range"`
	Items    []ProductAnalyticsItem `json:"items"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
}

type TopProductAnalyticsResult struct {
	Range  analytics.DateRange     `json:"range"`
	Metric analytics.ProductMetric `json:"metric"`
	Items  []ProductAnalyticsItem  `json:"items"`
}

type LowCTRProductAnalyticsResult struct {
	Range          analytics.DateRange    `json:"range"`
	MinImpressions int64                  `json:"min_impressions"`
	Items          []ProductAnalyticsItem `json:"items"`
}

type ProductAnalyticsDetailResult struct {
	Range       analytics.DateRange           `json:"range"`
	Granularity analytics.Granularity         `json:"granularity"`
	Product     analytics.ProductMetadata     `json:"product"`
	Summary     ProductAnalyticsSummary       `json:"summary"`
	Trend       []analytics.ProductTrendPoint `json:"trend"`
	Sources     []analytics.ProductSourceStat `json:"sources"`
}

func (s *StorefrontAnalyticsService) ProductsForSuper(
	ctx context.Context,
	tenantID *uint,
	request ProductAnalyticsListRequest,
) (*ProductAnalyticsListResult, error) {
	scope, err := s.superScope(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return s.products(ctx, scope, request)
}

func (s *StorefrontAnalyticsService) ProductsForTenant(
	ctx context.Context,
	tenantID uint,
	request ProductAnalyticsListRequest,
) (*ProductAnalyticsListResult, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}
	return s.products(ctx, analytics.TenantScope(tenantID), request)
}

func (s *StorefrontAnalyticsService) TopProductsForSuper(
	ctx context.Context,
	tenantID *uint,
	request TopProductAnalyticsRequest,
) (*TopProductAnalyticsResult, error) {
	scope, err := s.superScope(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return s.topProducts(ctx, scope, request)
}

func (s *StorefrontAnalyticsService) TopProductsForTenant(
	ctx context.Context,
	tenantID uint,
	request TopProductAnalyticsRequest,
) (*TopProductAnalyticsResult, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}
	return s.topProducts(ctx, analytics.TenantScope(tenantID), request)
}

func (s *StorefrontAnalyticsService) LowCTRProductsForSuper(
	ctx context.Context,
	tenantID *uint,
	request LowCTRProductAnalyticsRequest,
) (*LowCTRProductAnalyticsResult, error) {
	scope, err := s.superScope(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return s.lowCTRProducts(ctx, scope, request)
}

func (s *StorefrontAnalyticsService) LowCTRProductsForTenant(
	ctx context.Context,
	tenantID uint,
	request LowCTRProductAnalyticsRequest,
) (*LowCTRProductAnalyticsResult, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}
	return s.lowCTRProducts(ctx, analytics.TenantScope(tenantID), request)
}

func (s *StorefrontAnalyticsService) ProductDetailForSuper(
	ctx context.Context,
	tenantID *uint,
	productID uint,
	request ProductAnalyticsDetailRequest,
) (*ProductAnalyticsDetailResult, error) {
	scope, err := s.superScope(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return s.productDetail(ctx, scope, productID, request)
}

func (s *StorefrontAnalyticsService) ProductDetailForTenant(
	ctx context.Context,
	tenantID uint,
	productID uint,
	request ProductAnalyticsDetailRequest,
) (*ProductAnalyticsDetailResult, error) {
	if tenantID == 0 {
		return nil, ErrForbidden
	}
	return s.productDetail(ctx, analytics.TenantScope(tenantID), productID, request)
}

func (s *StorefrontAnalyticsService) products(
	ctx context.Context,
	scope analytics.Scope,
	request ProductAnalyticsListRequest,
) (*ProductAnalyticsListResult, error) {
	if s.productRepo == nil {
		return nil, errors.New("product analytics repository is not configured")
	}
	dateRange, err := s.parseDateRange(request.DateRange, analytics.MaxStandardRangeDays)
	if err != nil {
		return nil, err
	}
	query, err := normalizeProductAnalyticsListRequest(request)
	if err != nil {
		return nil, err
	}
	rows, total, err := s.productRepo.ProductAnalyticsList(ctx, scope, dateRange.Window(), query)
	if err != nil {
		return nil, err
	}
	return &ProductAnalyticsListResult{
		Range:    dateRange,
		Items:    productAnalyticsItems(rows),
		Total:    total,
		Page:     query.Page,
		PageSize: query.PageSize,
	}, nil
}

func (s *StorefrontAnalyticsService) topProducts(
	ctx context.Context,
	scope analytics.Scope,
	request TopProductAnalyticsRequest,
) (*TopProductAnalyticsResult, error) {
	if s.productRepo == nil {
		return nil, errors.New("product analytics repository is not configured")
	}
	dateRange, err := s.parseDateRange(request.DateRange, analytics.MaxStandardRangeDays)
	if err != nil {
		return nil, err
	}
	query, err := normalizeTopProductAnalyticsRequest(request)
	if err != nil {
		return nil, err
	}
	rows, err := s.productRepo.TopProductAnalytics(ctx, scope, dateRange.Window(), query)
	if err != nil {
		return nil, err
	}
	return &TopProductAnalyticsResult{Range: dateRange, Metric: query.Metric, Items: productAnalyticsItems(rows)}, nil
}

func (s *StorefrontAnalyticsService) lowCTRProducts(
	ctx context.Context,
	scope analytics.Scope,
	request LowCTRProductAnalyticsRequest,
) (*LowCTRProductAnalyticsResult, error) {
	if s.productRepo == nil {
		return nil, errors.New("product analytics repository is not configured")
	}
	dateRange, err := s.parseDateRange(request.DateRange, analytics.MaxStandardRangeDays)
	if err != nil {
		return nil, err
	}
	minImpressions, err := normalizeProductThreshold(request.MinImpressions, analytics.DefaultMinImpressions)
	if err != nil {
		return nil, err
	}
	limit, err := normalizeProductRankingLimit(request.Limit)
	if err != nil {
		return nil, err
	}
	rows, err := s.productRepo.LowCTRProductAnalytics(ctx, scope, dateRange.Window(), minImpressions, limit)
	if err != nil {
		return nil, err
	}
	return &LowCTRProductAnalyticsResult{
		Range: dateRange, MinImpressions: minImpressions, Items: productAnalyticsItems(rows),
	}, nil
}

func (s *StorefrontAnalyticsService) productDetail(
	ctx context.Context,
	scope analytics.Scope,
	productID uint,
	request ProductAnalyticsDetailRequest,
) (*ProductAnalyticsDetailResult, error) {
	if s.productRepo == nil {
		return nil, errors.New("product analytics repository is not configured")
	}
	if productID == 0 {
		return nil, ErrInvalidInput
	}
	dateRange, err := s.parseDateRange(request.DateRange, analytics.MaxWeeklyRangeDays)
	if err != nil {
		return nil, err
	}
	granularity, ok := analytics.ResolveGranularity(request.Granularity, dateRange.Days)
	if !ok {
		return nil, ErrInvalidInput
	}

	metadata, metadataErr := s.productRepo.CurrentProductAnalyticsMetadata(ctx, scope, productID)
	if metadataErr != nil && !errors.Is(metadataErr, gorm.ErrRecordNotFound) {
		return nil, metadataErr
	}
	aggregate, aggregateErr := s.productRepo.ProductAnalyticsSummary(ctx, scope, dateRange.Window(), productID)
	if aggregateErr != nil && !errors.Is(aggregateErr, gorm.ErrRecordNotFound) {
		return nil, aggregateErr
	}
	if metadata == nil && aggregate == nil {
		return nil, ErrNotFound
	}
	if aggregate == nil {
		aggregate = &analytics.ProductAggregate{ProductMetadata: *metadata}
	} else if metadata != nil {
		aggregate.ProductMetadata = *metadata
	}

	trend, err := s.productRepo.ProductAnalyticsTrends(ctx, scope, dateRange, granularity, productID)
	if err != nil {
		return nil, err
	}
	sources, err := s.productRepo.ProductAnalyticsSources(ctx, scope, dateRange.Window(), productID)
	if err != nil {
		return nil, err
	}
	labels, err := s.productRepo.ProductAnalyticsSourceLabels(ctx, sources)
	if err != nil {
		return nil, err
	}
	applyProductSourceLabels(sources, labels)

	item := productAnalyticsItem(*aggregate)
	return &ProductAnalyticsDetailResult{
		Range:       dateRange,
		Granularity: granularity,
		Product:     item.ProductMetadata,
		Summary:     productAnalyticsSummary(item.ProductCounts),
		Trend:       fillProductTrendBuckets(dateRange, granularity, trend),
		Sources:     normalizeProductSourceStats(sources),
	}, nil
}

func normalizeProductAnalyticsListRequest(request ProductAnalyticsListRequest) (analytics.ProductAnalyticsQuery, error) {
	page := request.Page
	if page == 0 {
		page = 1
	}
	pageSize := request.PageSize
	if pageSize == 0 {
		pageSize = analytics.DefaultReportPageSize
	}
	if page < 1 || pageSize < 1 || pageSize > analytics.MaxReportPageSize {
		return analytics.ProductAnalyticsQuery{}, ErrInvalidInput
	}
	search := strings.TrimSpace(request.Search)
	if utf8.RuneCountInString(search) > analytics.MaxProductAnalyticsSearch {
		return analytics.ProductAnalyticsQuery{}, ErrInvalidInput
	}
	status, ok := analytics.ParseProductStatusFilter(request.Status)
	if !ok {
		return analytics.ProductAnalyticsQuery{}, ErrInvalidInput
	}
	sort, ok := analytics.ParseProductMetric(request.Sort)
	if !ok {
		return analytics.ProductAnalyticsQuery{}, ErrInvalidInput
	}
	order, ok := analytics.ParseSortOrder(request.Order)
	if !ok {
		return analytics.ProductAnalyticsQuery{}, ErrInvalidInput
	}
	return analytics.ProductAnalyticsQuery{
		Search: search, Status: status, Sort: sort, Order: order, Page: page, PageSize: pageSize,
	}, nil
}

func normalizeTopProductAnalyticsRequest(request TopProductAnalyticsRequest) (analytics.ProductRankingQuery, error) {
	metric, ok := analytics.ParseProductMetric(request.Metric)
	if !ok {
		return analytics.ProductRankingQuery{}, ErrInvalidInput
	}
	limit, err := normalizeProductRankingLimit(request.Limit)
	if err != nil {
		return analytics.ProductRankingQuery{}, err
	}
	minImpressions, err := normalizeProductThreshold(request.MinImpressions, analytics.DefaultMinImpressions)
	if err != nil {
		return analytics.ProductRankingQuery{}, err
	}
	minClicks, err := normalizeProductThreshold(request.MinClicks, analytics.DefaultMinClicks)
	if err != nil {
		return analytics.ProductRankingQuery{}, err
	}
	minViews, err := normalizeProductThreshold(request.MinViews, analytics.DefaultMinViews)
	if err != nil {
		return analytics.ProductRankingQuery{}, err
	}
	return analytics.ProductRankingQuery{
		Metric: metric, Limit: limit, MinImpressions: minImpressions, MinClicks: minClicks, MinViews: minViews,
	}, nil
}

func normalizeProductRankingLimit(value int) (int, error) {
	if value == 0 {
		return analytics.DefaultProductRankingLimit, nil
	}
	if value < 1 || value > analytics.MaxProductRankingLimit {
		return 0, ErrInvalidInput
	}
	return value, nil
}

func normalizeProductThreshold(value *int, defaultValue int) (int64, error) {
	if value == nil {
		return int64(defaultValue), nil
	}
	if *value < 0 || *value > analytics.MaxProductRankingThreshold {
		return 0, ErrInvalidInput
	}
	return int64(*value), nil
}

func productAnalyticsItems(rows []analytics.ProductAggregate) []ProductAnalyticsItem {
	items := make([]ProductAnalyticsItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, productAnalyticsItem(row))
	}
	return items
}

func productAnalyticsItem(row analytics.ProductAggregate) ProductAnalyticsItem {
	if !row.CurrentlyExists {
		row.ProductSlug = ""
		if strings.TrimSpace(row.ProductName) == "" {
			row.ProductName = fmt.Sprintf("Deleted product #%d", row.ProductID)
		}
	} else {
		row.ProductSlug = BuildProductSEOPathSegment(row.ProductName, row.ProductID)
	}
	rates := row.ProductCounts.Rates()
	return ProductAnalyticsItem{
		ProductMetadata:   row.ProductMetadata,
		ProductCounts:     row.ProductCounts,
		CTR:               rates.ProductCTR,
		DetailViewRate:    rates.DetailViewRate,
		ContactIntentRate: rates.ContactIntentRate,
	}
}

func productAnalyticsSummary(counts analytics.ProductCounts) ProductAnalyticsSummary {
	rates := counts.Rates()
	return ProductAnalyticsSummary{
		ProductCounts:     counts,
		CTR:               rates.ProductCTR,
		DetailViewRate:    rates.DetailViewRate,
		ContactIntentRate: rates.ContactIntentRate,
	}
}

func fillProductTrendBuckets(
	dateRange analytics.DateRange,
	granularity analytics.Granularity,
	sparse []analytics.ProductTrendPoint,
) []analytics.ProductTrendPoint {
	byBucket := make(map[string]analytics.ProductTrendPoint, len(sparse))
	for _, point := range sparse {
		byBucket[point.Bucket] = point
	}
	buckets := analyticsBucketKeys(dateRange, granularity)
	series := make([]analytics.ProductTrendPoint, 0, len(buckets))
	for _, bucket := range buckets {
		point, ok := byBucket[bucket]
		if !ok {
			point = analytics.ProductTrendPoint{Bucket: bucket}
		}
		series = append(series, point)
	}
	return series
}

func applyProductSourceLabels(stats []analytics.ProductSourceStat, labels []analytics.ProductSourceLabel) {
	bySource := make(map[string]string, len(labels))
	for _, label := range labels {
		bySource[label.SourceType+"\x00"+label.SourceID] = label.Label
	}
	for index := range stats {
		if stats[index].SourceID == nil {
			continue
		}
		if label, ok := bySource[stats[index].SourceType+"\x00"+*stats[index].SourceID]; ok {
			value := label
			stats[index].SourceLabel = &value
		}
	}
}

func normalizeProductSourceStats(stats []analytics.ProductSourceStat) []analytics.ProductSourceStat {
	for index := range stats {
		stats[index].CTR = analytics.Ratio(stats[index].Clicks, stats[index].Impressions)
	}
	return stats
}
