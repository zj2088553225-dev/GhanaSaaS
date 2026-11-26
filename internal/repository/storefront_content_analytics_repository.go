package repository

import (
	"context"
	"fmt"
	"strings"

	"saas/internal/analytics"
	"saas/internal/models"
)

type contentAnalyticsListQueries struct {
	Items storefrontAnalyticsQuery
	Total *storefrontAnalyticsQuery
}

func (r *StorefrontAnalyticsRepository) ListContentAnalytics(
	ctx context.Context,
	window analytics.TimeWindow,
	query analytics.ContentAnalyticsQuery,
) ([]analytics.ContentAggregate, int64, error) {
	built, err := buildContentAnalyticsListQueries(r.db.Dialector.Name(), window, query)
	if err != nil {
		return nil, 0, err
	}
	items := make([]analytics.ContentAggregate, 0)
	if err := r.db.WithContext(ctx).Raw(built.Items.SQL, built.Items.Args...).Scan(&items).Error; err != nil {
		return nil, 0, err
	}
	if built.Total == nil {
		return items, int64(len(items)), nil
	}
	var total int64
	if err := r.db.WithContext(ctx).Raw(built.Total.SQL, built.Total.Args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *StorefrontAnalyticsRepository) GetContentSummary(
	ctx context.Context,
	window analytics.TimeWindow,
	trackingKey string,
) (analytics.ContentCounts, error) {
	query, err := buildContentAnalyticsSummaryQuery(r.db.Dialector.Name(), window, trackingKey)
	if err != nil {
		return analytics.ContentCounts{}, err
	}
	var counts analytics.ContentCounts
	err = r.db.WithContext(ctx).Raw(query.SQL, query.Args...).Scan(&counts).Error
	return counts, err
}

func (r *StorefrontAnalyticsRepository) ContentEverExisted(ctx context.Context, trackingKey string) (bool, error) {
	query, err := buildContentEverExistedQuery(trackingKey)
	if err != nil {
		return false, err
	}
	var exists bool
	err = r.db.WithContext(ctx).Raw(query.SQL, query.Args...).Scan(&exists).Error
	return exists, err
}

func (r *StorefrontAnalyticsRepository) GetContentTrend(
	ctx context.Context,
	dateRange analytics.DateRange,
	granularity analytics.Granularity,
	trackingKey string,
) ([]analytics.ContentTrendPoint, error) {
	query, err := buildContentAnalyticsTrendQuery(r.db.Dialector.Name(), dateRange, granularity, trackingKey)
	if err != nil {
		return nil, err
	}
	points := make([]analytics.ContentTrendPoint, 0)
	if err := r.db.WithContext(ctx).Raw(query.SQL, query.Args...).Scan(&points).Error; err != nil {
		return nil, err
	}
	return points, nil
}

func (r *StorefrontAnalyticsRepository) GetContentLocaleBreakdown(
	ctx context.Context,
	window analytics.TimeWindow,
	trackingKey string,
) ([]analytics.ContentLocaleStat, error) {
	query, err := buildContentLocaleBreakdownQuery(r.db.Dialector.Name(), window, trackingKey)
	if err != nil {
		return nil, err
	}
	stats := make([]analytics.ContentLocaleStat, 0)
	if err := r.db.WithContext(ctx).Raw(query.SQL, query.Args...).Scan(&stats).Error; err != nil {
		return nil, err
	}
	return stats, nil
}

func (r *StorefrontAnalyticsRepository) GetContentDeviceBreakdown(
	ctx context.Context,
	window analytics.TimeWindow,
	trackingKey string,
) ([]analytics.ContentDeviceStat, error) {
	query, err := buildContentDeviceBreakdownQuery(r.db.Dialector.Name(), window, trackingKey)
	if err != nil {
		return nil, err
	}
	stats := make([]analytics.ContentDeviceStat, 0)
	if err := r.db.WithContext(ctx).Raw(query.SQL, query.Args...).Scan(&stats).Error; err != nil {
		return nil, err
	}
	return stats, nil
}

func (r *StorefrontAnalyticsRepository) GetContentPageBreakdown(
	ctx context.Context,
	window analytics.TimeWindow,
	trackingKey string,
) ([]analytics.ContentPageStat, error) {
	query, err := buildContentPageBreakdownQuery(r.db.Dialector.Name(), window, trackingKey)
	if err != nil {
		return nil, err
	}
	stats := make([]analytics.ContentPageStat, 0)
	if err := r.db.WithContext(ctx).Raw(query.SQL, query.Args...).Scan(&stats).Error; err != nil {
		return nil, err
	}
	return stats, nil
}

func buildContentAnalyticsListQueries(
	dialect string,
	window analytics.TimeWindow,
	query analytics.ContentAnalyticsQuery,
) (contentAnalyticsListQueries, error) {
	base, err := buildContentAnalyticsBaseQuery(dialect, window)
	if err != nil {
		return contentAnalyticsListQueries{}, err
	}
	metric, err := contentMetricSQL(query.Sort)
	if err != nil {
		return contentAnalyticsListQueries{}, err
	}
	order, err := analyticsOrderSQL(query.Order)
	if err != nil || query.MinImpressions < 0 {
		return contentAnalyticsListQueries{}, ErrInvalidAnalyticsScope
	}
	thresholdSQL := ""
	thresholdArgs := make([]any, 0, 1)
	if query.Sort == analytics.ContentMetricCTR {
		thresholdSQL = "\nWHERE impressions >= ?"
		thresholdArgs = append(thresholdArgs, query.MinImpressions)
	}

	itemsSQL := base.SQL + "\n" + contentAnalyticsSelectSQL + thresholdSQL +
		"\nORDER BY " + metric + " " + order + ", tracking_key ASC"
	itemsArgs := append(append([]any{}, base.Args...), thresholdArgs...)
	result := contentAnalyticsListQueries{}
	if query.Paginate {
		if query.Page < 1 || query.PageSize < 1 {
			return contentAnalyticsListQueries{}, ErrInvalidAnalyticsScope
		}
		itemsSQL += "\nLIMIT ? OFFSET ?"
		itemsArgs = append(itemsArgs, query.PageSize, (query.Page-1)*query.PageSize)
		totalSQL := base.SQL + "\nSELECT COUNT(*) FROM scored_content" + thresholdSQL
		totalArgs := append(append([]any{}, base.Args...), thresholdArgs...)
		result.Total = &storefrontAnalyticsQuery{SQL: totalSQL, Args: totalArgs}
	}
	result.Items = storefrontAnalyticsQuery{SQL: itemsSQL, Args: itemsArgs}
	return result, nil
}

const contentAnalyticsSelectSQL = `SELECT
    tracking_key,
    impressions,
    clicks,
    unique_visitors
FROM scored_content`

func buildContentAnalyticsBaseQuery(dialect string, window analytics.TimeWindow) (storefrontAnalyticsQuery, error) {
	filter, filterArgs, err := contentAnalyticsEventFilter(window, "")
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	aggregateSQL, aggregateArgs := contentAnalyticsAggregateSQL(dialect)
	template := fmt.Sprintf(`
WITH filtered_events AS (
    SELECT e.entity_id, e.event_name, e.visitor_id
    FROM storefront_events e
    WHERE %%s
), content_aggregates AS (
    SELECT
        entity_id AS tracking_key,%s
    FROM filtered_events
    GROUP BY entity_id
), scored_content AS (
    SELECT
        *,
        CASE WHEN impressions = 0 THEN 0 ELSE CAST(clicks AS DOUBLE PRECISION) / impressions END AS ctr
    FROM content_aggregates
)`, aggregateSQL)
	query, err := applyAnalyticsFilter(template, filter)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	args := append(append([]any{}, filterArgs...), aggregateArgs...)
	return storefrontAnalyticsQuery{SQL: query, Args: args}, nil
}

func buildContentAnalyticsSummaryQuery(
	dialect string,
	window analytics.TimeWindow,
	trackingKey string,
) (storefrontAnalyticsQuery, error) {
	filter, filterArgs, err := contentAnalyticsEventFilter(window, trackingKey)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	aggregateSQL, aggregateArgs := contentAnalyticsAggregateSQL(dialect)
	template := fmt.Sprintf(`
WITH filtered_events AS (
    SELECT e.event_name, e.visitor_id
    FROM storefront_events e
    WHERE %%s
)
SELECT%s
FROM filtered_events`, aggregateSQL)
	query, err := applyAnalyticsFilter(template, filter)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	args := append(append([]any{}, filterArgs...), aggregateArgs...)
	return storefrontAnalyticsQuery{SQL: query, Args: args}, nil
}

func buildContentEverExistedQuery(trackingKey string) (storefrontAnalyticsQuery, error) {
	trackingKey = strings.TrimSpace(trackingKey)
	if trackingKey == "" {
		return storefrontAnalyticsQuery{}, ErrInvalidAnalyticsScope
	}
	return storefrontAnalyticsQuery{
		SQL: `SELECT EXISTS (
    SELECT 1
    FROM storefront_events e
    WHERE e.event_name IN (?, ?)
      AND e.entity_type = ?
      AND e.entity_id = ?
	  AND e.traffic_type = ?
)`,
		Args: []any{
			models.StorefrontEventContentImpression,
			models.StorefrontEventContentClick,
			models.StorefrontEntityMarketingContent,
			trackingKey,
			models.StorefrontTrafficHuman,
		},
	}, nil
}

func buildContentAnalyticsTrendQuery(
	dialect string,
	dateRange analytics.DateRange,
	granularity analytics.Granularity,
	trackingKey string,
) (storefrontAnalyticsQuery, error) {
	filter, filterArgs, err := contentAnalyticsEventFilter(dateRange.Window(), trackingKey)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	aggregateSQL, aggregateArgs := contentAnalyticsAggregateSQL(dialect)
	var template string
	var prefixArgs []any
	if dialect == "postgres" {
		truncUnit, outputFormat, err := postgresTrendFragments(granularity)
		if err != nil {
			return storefrontAnalyticsQuery{}, err
		}
		template = fmt.Sprintf(`
WITH filtered AS (
    SELECT
        e.event_name,
        e.visitor_id,
        date_trunc('%s', e.occurred_at AT TIME ZONE ?) AS bucket_local
    FROM storefront_events e
    WHERE %%s
)
SELECT
    TO_CHAR(bucket_local, '%s') AS bucket,%s
FROM filtered
GROUP BY bucket_local
ORDER BY bucket_local ASC`, truncUnit, outputFormat, aggregateSQL)
		prefixArgs = []any{dateRange.Timezone}
	} else {
		bucketExpression, bucketArgs, err := sqliteTrendBucketExpression(granularity, dateRange)
		if err != nil {
			return storefrontAnalyticsQuery{}, err
		}
		template = fmt.Sprintf(`
WITH filtered AS (
    SELECT e.event_name, e.visitor_id, %s AS bucket
    FROM storefront_events e
    WHERE %%s
)
SELECT
    bucket,%s
FROM filtered
GROUP BY bucket
ORDER BY bucket ASC`, bucketExpression, aggregateSQL)
		prefixArgs = bucketArgs
	}
	query, err := applyAnalyticsFilter(template, filter)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	args := append(append([]any{}, prefixArgs...), filterArgs...)
	args = append(args, aggregateArgs...)
	return storefrontAnalyticsQuery{SQL: query, Args: args}, nil
}

func buildContentLocaleBreakdownQuery(
	dialect string,
	window analytics.TimeWindow,
	trackingKey string,
) (storefrontAnalyticsQuery, error) {
	return buildContentDimensionBreakdownQuery(
		dialect,
		window,
		trackingKey,
		"CASE WHEN LOWER(TRIM(e.locale)) = 'en' THEN 'en' WHEN LOWER(TRIM(e.locale)) = 'zh' THEN 'zh' ELSE 'other' END",
		"locale",
	)
}

func buildContentDeviceBreakdownQuery(
	dialect string,
	window analytics.TimeWindow,
	trackingKey string,
) (storefrontAnalyticsQuery, error) {
	return buildContentDimensionBreakdownQuery(
		dialect,
		window,
		trackingKey,
		"CASE WHEN LOWER(TRIM(e.device_type)) IN ('desktop', 'mobile', 'tablet') THEN LOWER(TRIM(e.device_type)) ELSE 'unknown' END",
		"device_type",
	)
}

func buildContentDimensionBreakdownQuery(
	dialect string,
	window analytics.TimeWindow,
	trackingKey string,
	dimensionExpression string,
	dimensionAlias string,
) (storefrontAnalyticsQuery, error) {
	if dimensionAlias != "locale" && dimensionAlias != "device_type" {
		return storefrontAnalyticsQuery{}, ErrInvalidAnalyticsScope
	}
	filter, filterArgs, err := contentAnalyticsEventFilter(window, trackingKey)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	aggregateSQL, aggregateArgs := contentAnalyticsAggregateSQL(dialect)
	template := fmt.Sprintf(`
WITH filtered AS (
    SELECT
        %s AS dimension_value,
        e.event_name,
        e.visitor_id
    FROM storefront_events e
    WHERE %%s
)
SELECT
    dimension_value AS %s,%s
FROM filtered
GROUP BY dimension_value
ORDER BY dimension_value ASC`, dimensionExpression, dimensionAlias, aggregateSQL)
	query, err := applyAnalyticsFilter(template, filter)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	args := append(append([]any{}, filterArgs...), aggregateArgs...)
	return storefrontAnalyticsQuery{SQL: query, Args: args}, nil
}

func buildContentPageBreakdownQuery(
	dialect string,
	window analytics.TimeWindow,
	trackingKey string,
) (storefrontAnalyticsQuery, error) {
	filter, filterArgs, err := contentAnalyticsEventFilter(window, trackingKey)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	countsSQL, countArgs := contentPageCountsSQL(dialect)
	template := fmt.Sprintf(`
WITH filtered AS (
    SELECT
        COALESCE(NULLIF(TRIM(e.page_path), ''), 'unknown') AS page_path,
        e.event_name
    FROM storefront_events e
    WHERE %%s
)
SELECT
    page_path,%s
FROM filtered
GROUP BY page_path
ORDER BY clicks DESC, impressions DESC, page_path ASC
LIMIT ?`, countsSQL)
	query, err := applyAnalyticsFilter(template, filter)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	args := append(append([]any{}, filterArgs...), countArgs...)
	args = append(args, analytics.ContentPageBreakdownLimit)
	return storefrontAnalyticsQuery{SQL: query, Args: args}, nil
}

func contentAnalyticsEventFilter(window analytics.TimeWindow, trackingKey string) (string, []any, error) {
	filter, args, err := analyticsEventFilterFor(analytics.AllScope(), window, analytics.ContentEventAllowlist)
	if err != nil {
		return "", nil, err
	}
	filter += " AND e.entity_type = ? AND e.entity_id IS NOT NULL"
	args = append(args, models.StorefrontEntityMarketingContent)
	trackingKey = strings.TrimSpace(trackingKey)
	if trackingKey != "" {
		filter += " AND e.entity_id = ?"
		args = append(args, trackingKey)
	}
	return filter, args, nil
}

func contentAnalyticsAggregateSQL(dialect string) (string, []any) {
	args := []any{models.StorefrontEventContentImpression, models.StorefrontEventContentClick}
	if dialect == "postgres" {
		return `
        COUNT(*) FILTER (WHERE event_name = ?) AS impressions,
        COUNT(*) FILTER (WHERE event_name = ?) AS clicks,
        COUNT(DISTINCT visitor_id) AS unique_visitors`, args
	}
	return `
        COALESCE(SUM(CASE WHEN event_name = ? THEN 1 ELSE 0 END), 0) AS impressions,
        COALESCE(SUM(CASE WHEN event_name = ? THEN 1 ELSE 0 END), 0) AS clicks,
        COUNT(DISTINCT visitor_id) AS unique_visitors`, args
}

func contentPageCountsSQL(dialect string) (string, []any) {
	args := []any{models.StorefrontEventContentImpression, models.StorefrontEventContentClick}
	if dialect == "postgres" {
		return `
    COUNT(*) FILTER (WHERE event_name = ?) AS impressions,
    COUNT(*) FILTER (WHERE event_name = ?) AS clicks`, args
	}
	return `
    COALESCE(SUM(CASE WHEN event_name = ? THEN 1 ELSE 0 END), 0) AS impressions,
    COALESCE(SUM(CASE WHEN event_name = ? THEN 1 ELSE 0 END), 0) AS clicks`, args
}

func contentMetricSQL(metric analytics.ContentMetric) (string, error) {
	switch metric {
	case analytics.ContentMetricImpressions:
		return "impressions", nil
	case analytics.ContentMetricClicks:
		return "clicks", nil
	case analytics.ContentMetricUniqueVisitors:
		return "unique_visitors", nil
	case analytics.ContentMetricCTR:
		return "ctr", nil
	default:
		return "", ErrInvalidAnalyticsScope
	}
}
