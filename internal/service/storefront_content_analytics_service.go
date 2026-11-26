package service

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"saas/internal/analytics"
	"saas/internal/pkg/trackingkey"
)

type storefrontContentAnalyticsRepository interface {
	ListContentAnalytics(ctx context.Context, window analytics.TimeWindow, query analytics.ContentAnalyticsQuery) ([]analytics.ContentAggregate, int64, error)
	GetContentSummary(ctx context.Context, window analytics.TimeWindow, trackingKey string) (analytics.ContentCounts, error)
	ContentEverExisted(ctx context.Context, trackingKey string) (bool, error)
	GetContentTrend(ctx context.Context, dateRange analytics.DateRange, granularity analytics.Granularity, trackingKey string) ([]analytics.ContentTrendPoint, error)
	GetContentLocaleBreakdown(ctx context.Context, window analytics.TimeWindow, trackingKey string) ([]analytics.ContentLocaleStat, error)
	GetContentDeviceBreakdown(ctx context.Context, window analytics.TimeWindow, trackingKey string) ([]analytics.ContentDeviceStat, error)
	GetContentPageBreakdown(ctx context.Context, window analytics.TimeWindow, trackingKey string) ([]analytics.ContentPageStat, error)
}

type storefrontMarketingContentResolver interface {
	Resolve(ctx context.Context) (map[string]analytics.MarketingContentMetadata, error)
}

type ContentAnalyticsListRequest struct {
	DateRange       analytics.DateRangeInput
	Page            int
	PageSize        int
	Search          string
	ContentType     string
	CurrentlyExists *bool
	Sort            string
	Order           string
	MinImpressions  *int
}

type ContentAnalyticsDetailRequest struct {
	DateRange   analytics.DateRangeInput
	Granularity string
}

type MarketingContentView struct {
	analytics.MarketingContentMetadata
	CurrentlyExists bool `json:"currently_exists"`
}

type ContentAnalyticsItem struct {
	MarketingContentView
	analytics.ContentCounts
	CTR float64 `json:"ctr"`
}

type ContentAnalyticsSummary struct {
	analytics.ContentCounts
	CTR float64 `json:"ctr"`
}

type ContentAnalyticsListResult struct {
	Range    analytics.DateRange    `json:"range"`
	Items    []ContentAnalyticsItem `json:"items"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
}

type ContentAnalyticsDetailResult struct {
	Range       analytics.DateRange           `json:"range"`
	Granularity analytics.Granularity         `json:"granularity"`
	Content     MarketingContentView          `json:"content"`
	Summary     ContentAnalyticsSummary       `json:"summary"`
	Trend       []analytics.ContentTrendPoint `json:"trend"`
	Locales     []analytics.ContentLocaleStat `json:"locales"`
	Devices     []analytics.ContentDeviceStat `json:"devices"`
	Pages       []analytics.ContentPageStat   `json:"pages"`
}

type contentMetadataFilter struct {
	Search          string
	ContentType     analytics.ContentType
	CurrentlyExists *bool
}

func (filter contentMetadataFilter) Active() bool {
	return filter.Search != "" || filter.ContentType != "" || filter.CurrentlyExists != nil
}

func (s *StorefrontAnalyticsService) ContentForSuper(
	ctx context.Context,
	request ContentAnalyticsListRequest,
) (*ContentAnalyticsListResult, error) {
	if s.contentRepo == nil || s.contentResolver == nil {
		return nil, errors.New("content analytics is not configured")
	}
	dateRange, err := s.parseDateRange(request.DateRange, analytics.MaxStandardRangeDays)
	if err != nil {
		return nil, err
	}
	query, metadataFilter, err := normalizeContentAnalyticsListRequest(request)
	if err != nil {
		return nil, err
	}
	metadata, err := s.contentResolver.Resolve(ctx)
	if err != nil {
		return nil, err
	}

	// The metadata-filter path intentionally disables SQL pagination, but the
	// repository still returns only entity_id aggregate rows. Raw events never
	// cross into Go. The default path remains fully DB-paginated.
	query.Paginate = !metadataFilter.Active()
	rows, total, err := s.contentRepo.ListContentAnalytics(ctx, dateRange.Window(), query)
	if err != nil {
		return nil, err
	}
	items := contentAnalyticsItems(rows, metadata)
	if metadataFilter.Active() {
		items = filterContentAnalyticsItems(items, metadataFilter)
		total = int64(len(items))
		items = paginateContentAnalyticsItems(items, query.Page, query.PageSize)
	}
	return &ContentAnalyticsListResult{
		Range: dateRange, Items: items, Total: total, Page: query.Page, PageSize: query.PageSize,
	}, nil
}

func (s *StorefrontAnalyticsService) ContentDetailForSuper(
	ctx context.Context,
	trackingKey string,
	request ContentAnalyticsDetailRequest,
) (*ContentAnalyticsDetailResult, error) {
	if s.contentRepo == nil || s.contentResolver == nil {
		return nil, errors.New("content analytics is not configured")
	}
	trackingKey = trackingkey.Normalize(trackingKey)
	if !trackingkey.IsValid(trackingKey) {
		return nil, ErrInvalidInput
	}
	dateRange, err := s.parseDateRange(request.DateRange, analytics.MaxWeeklyRangeDays)
	if err != nil {
		return nil, err
	}
	granularity, ok := analytics.ResolveGranularity(strings.TrimSpace(request.Granularity), dateRange.Days)
	if !ok {
		return nil, ErrInvalidInput
	}
	metadata, err := s.contentResolver.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	currentMetadata, currentlyExists := metadata[trackingKey]
	if !currentlyExists {
		everExisted, err := s.contentRepo.ContentEverExisted(ctx, trackingKey)
		if err != nil {
			return nil, err
		}
		if !everExisted {
			return nil, ErrNotFound
		}
		currentMetadata = deletedMarketingContentMetadata(trackingKey)
	}

	counts, err := s.contentRepo.GetContentSummary(ctx, dateRange.Window(), trackingKey)
	if err != nil {
		return nil, err
	}
	trend, err := s.contentRepo.GetContentTrend(ctx, dateRange, granularity, trackingKey)
	if err != nil {
		return nil, err
	}
	locales, err := s.contentRepo.GetContentLocaleBreakdown(ctx, dateRange.Window(), trackingKey)
	if err != nil {
		return nil, err
	}
	devices, err := s.contentRepo.GetContentDeviceBreakdown(ctx, dateRange.Window(), trackingKey)
	if err != nil {
		return nil, err
	}
	pages, err := s.contentRepo.GetContentPageBreakdown(ctx, dateRange.Window(), trackingKey)
	if err != nil {
		return nil, err
	}

	return &ContentAnalyticsDetailResult{
		Range:       dateRange,
		Granularity: granularity,
		Content:     MarketingContentView{MarketingContentMetadata: currentMetadata, CurrentlyExists: currentlyExists},
		Summary:     ContentAnalyticsSummary{ContentCounts: counts, CTR: counts.CTR()},
		Trend:       fillContentTrendBuckets(dateRange, granularity, trend),
		Locales:     normalizeContentLocaleStats(locales),
		Devices:     normalizeContentDeviceStats(devices),
		Pages:       pages,
	}, nil
}

func normalizeContentAnalyticsListRequest(
	request ContentAnalyticsListRequest,
) (analytics.ContentAnalyticsQuery, contentMetadataFilter, error) {
	page := request.Page
	if page == 0 {
		page = 1
	}
	pageSize := request.PageSize
	if pageSize == 0 {
		pageSize = analytics.DefaultReportPageSize
	}
	if page < 1 || pageSize < 1 || pageSize > analytics.MaxReportPageSize {
		return analytics.ContentAnalyticsQuery{}, contentMetadataFilter{}, ErrInvalidInput
	}
	search := strings.TrimSpace(request.Search)
	if utf8.RuneCountInString(search) > analytics.MaxContentAnalyticsSearch {
		return analytics.ContentAnalyticsQuery{}, contentMetadataFilter{}, ErrInvalidInput
	}
	contentType, ok := analytics.ParseContentType(request.ContentType)
	if !ok {
		return analytics.ContentAnalyticsQuery{}, contentMetadataFilter{}, ErrInvalidInput
	}
	sort, ok := analytics.ParseContentMetric(request.Sort)
	if !ok {
		return analytics.ContentAnalyticsQuery{}, contentMetadataFilter{}, ErrInvalidInput
	}
	order, ok := analytics.ParseSortOrder(request.Order)
	if !ok {
		return analytics.ContentAnalyticsQuery{}, contentMetadataFilter{}, ErrInvalidInput
	}
	minImpressions := int64(0)
	if sort == analytics.ContentMetricCTR {
		minImpressions = analytics.DefaultMinImpressions
		if request.MinImpressions != nil {
			if *request.MinImpressions < 0 || *request.MinImpressions > analytics.MaxContentAnalyticsThreshold {
				return analytics.ContentAnalyticsQuery{}, contentMetadataFilter{}, ErrInvalidInput
			}
			minImpressions = int64(*request.MinImpressions)
		}
	} else if request.MinImpressions != nil && (*request.MinImpressions < 0 || *request.MinImpressions > analytics.MaxContentAnalyticsThreshold) {
		return analytics.ContentAnalyticsQuery{}, contentMetadataFilter{}, ErrInvalidInput
	}
	return analytics.ContentAnalyticsQuery{
			Sort: sort, Order: order, Page: page, PageSize: pageSize, MinImpressions: minImpressions,
		}, contentMetadataFilter{
			Search: strings.ToLower(search), ContentType: contentType, CurrentlyExists: request.CurrentlyExists,
		}, nil
}

func contentAnalyticsItems(
	rows []analytics.ContentAggregate,
	metadata map[string]analytics.MarketingContentMetadata,
) []ContentAnalyticsItem {
	items := make([]ContentAnalyticsItem, 0, len(rows))
	for _, row := range rows {
		current, exists := metadata[row.TrackingKey]
		if !exists {
			current = deletedMarketingContentMetadata(row.TrackingKey)
		}
		items = append(items, ContentAnalyticsItem{
			MarketingContentView: MarketingContentView{MarketingContentMetadata: current, CurrentlyExists: exists},
			ContentCounts:        row.ContentCounts,
			CTR:                  row.ContentCounts.CTR(),
		})
	}
	return items
}

func deletedMarketingContentMetadata(trackingKey string) analytics.MarketingContentMetadata {
	return analytics.MarketingContentMetadata{
		TrackingKey: trackingKey,
		ContentType: analytics.ContentTypeOther,
		Title:       "Deleted marketing content",
		TargetType:  analytics.ContentTargetNone,
		Published:   false,
	}
}

func filterContentAnalyticsItems(items []ContentAnalyticsItem, filter contentMetadataFilter) []ContentAnalyticsItem {
	filtered := make([]ContentAnalyticsItem, 0, len(items))
	for _, item := range items {
		if filter.CurrentlyExists != nil && item.CurrentlyExists != *filter.CurrentlyExists {
			continue
		}
		if filter.ContentType != "" && (!item.CurrentlyExists || item.ContentType != filter.ContentType) {
			continue
		}
		if filter.Search != "" {
			trackingKey := strings.ToLower(item.TrackingKey)
			title := ""
			if item.CurrentlyExists {
				title = strings.ToLower(item.Title)
			}
			if !strings.Contains(trackingKey, filter.Search) && !strings.Contains(title, filter.Search) {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func paginateContentAnalyticsItems(items []ContentAnalyticsItem, page int, pageSize int) []ContentAnalyticsItem {
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []ContentAnalyticsItem{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func fillContentTrendBuckets(
	dateRange analytics.DateRange,
	granularity analytics.Granularity,
	sparse []analytics.ContentTrendPoint,
) []analytics.ContentTrendPoint {
	byBucket := make(map[string]analytics.ContentTrendPoint, len(sparse))
	for _, point := range sparse {
		byBucket[point.Bucket] = point
	}
	buckets := analyticsBucketKeys(dateRange, granularity)
	series := make([]analytics.ContentTrendPoint, 0, len(buckets))
	for _, bucket := range buckets {
		point, ok := byBucket[bucket]
		if !ok {
			point = analytics.ContentTrendPoint{Bucket: bucket}
		}
		series = append(series, point)
	}
	return series
}

func normalizeContentLocaleStats(stats []analytics.ContentLocaleStat) []analytics.ContentLocaleStat {
	byLocale := make(map[string]analytics.ContentCounts, 3)
	for _, stat := range stats {
		locale := stat.Locale
		if locale != "en" && locale != "zh" {
			locale = "other"
		}
		counts := byLocale[locale]
		counts.Impressions += stat.Impressions
		counts.Clicks += stat.Clicks
		counts.UniqueVisitors += stat.UniqueVisitors
		byLocale[locale] = counts
	}
	result := make([]analytics.ContentLocaleStat, 0, 3)
	for _, locale := range []string{"en", "zh", "other"} {
		counts := byLocale[locale]
		result = append(result, analytics.ContentLocaleStat{Locale: locale, ContentCounts: counts, CTR: counts.CTR()})
	}
	return result
}

func normalizeContentDeviceStats(stats []analytics.ContentDeviceStat) []analytics.ContentDeviceStat {
	byDevice := make(map[string]analytics.ContentCounts, 4)
	for _, stat := range stats {
		device := stat.DeviceType
		switch device {
		case "desktop", "mobile", "tablet":
		default:
			device = "unknown"
		}
		counts := byDevice[device]
		counts.Impressions += stat.Impressions
		counts.Clicks += stat.Clicks
		counts.UniqueVisitors += stat.UniqueVisitors
		byDevice[device] = counts
	}
	result := make([]analytics.ContentDeviceStat, 0, 4)
	for _, device := range []string{"desktop", "mobile", "tablet", "unknown"} {
		counts := byDevice[device]
		result = append(result, analytics.ContentDeviceStat{DeviceType: device, ContentCounts: counts, CTR: counts.CTR()})
	}
	return result
}
