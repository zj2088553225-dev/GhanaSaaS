package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"saas/internal/analytics"
	"saas/internal/models"

	"gorm.io/gorm"
)

var ErrInvalidAnalyticsScope = errors.New("invalid analytics scope")

var errInvalidAnalyticsQueryTemplate = errors.New("invalid analytics query template")

type storefrontAnalyticsQuery struct {
	SQL  string
	Args []any
}

type StorefrontAnalyticsRepository struct {
	db *gorm.DB
}

func NewStorefrontAnalyticsRepository(db *gorm.DB) *StorefrontAnalyticsRepository {
	return &StorefrontAnalyticsRepository{db: db}
}

func (r *StorefrontAnalyticsRepository) TenantExists(ctx context.Context, tenantID uint) (bool, error) {
	if tenantID == 0 {
		return false, nil
	}
	var tenant models.Tenant
	err := r.db.WithContext(ctx).Select("id").First(&tenant, tenantID).Error
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return false, nil
	default:
		return false, err
	}
}

func (r *StorefrontAnalyticsRepository) Metrics(
	ctx context.Context,
	scope analytics.Scope,
	window analytics.TimeWindow,
) (analytics.EventCounts, error) {
	query, err := buildOverviewQuery(r.db.Dialector.Name(), scope, window)
	if err != nil {
		return analytics.EventCounts{}, err
	}
	var counts analytics.EventCounts
	err = r.db.WithContext(ctx).Raw(query.SQL, query.Args...).Scan(&counts).Error
	return counts, err
}

func (r *StorefrontAnalyticsRepository) Trends(
	ctx context.Context,
	scope analytics.Scope,
	dateRange analytics.DateRange,
	granularity analytics.Granularity,
) ([]analytics.SparseTrendPoint, error) {
	query, err := buildTrendQuery(r.db.Dialector.Name(), scope, dateRange, granularity)
	if err != nil {
		return nil, err
	}
	points := make([]analytics.SparseTrendPoint, 0)
	if err := r.db.WithContext(ctx).Raw(query.SQL, query.Args...).Scan(&points).Error; err != nil {
		return nil, err
	}
	return points, nil
}

func (r *StorefrontAnalyticsRepository) Sources(
	ctx context.Context,
	scope analytics.Scope,
	window analytics.TimeWindow,
	sort analytics.SourceSort,
	limit int,
) ([]analytics.SourceStat, error) {
	query, err := buildSourceQuery(r.db.Dialector.Name(), scope, window, sort, limit)
	if err != nil {
		return nil, err
	}
	stats := make([]analytics.SourceStat, 0)
	if err := r.db.WithContext(ctx).Raw(query.SQL, query.Args...).Scan(&stats).Error; err != nil {
		return nil, err
	}
	return stats, nil
}

func buildOverviewQuery(dialect string, scope analytics.Scope, window analytics.TimeWindow) (storefrontAnalyticsQuery, error) {
	filter, filterArgs, err := analyticsEventFilter(scope, window)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	template, selectArgs := analyticsMetricsSQL(dialect)
	query, err := applyAnalyticsFilter(template, filter)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	args := append(selectArgs, filterArgs...)
	return storefrontAnalyticsQuery{SQL: query, Args: args}, nil
}

func buildTrendQuery(
	dialect string,
	scope analytics.Scope,
	dateRange analytics.DateRange,
	granularity analytics.Granularity,
) (storefrontAnalyticsQuery, error) {
	filter, filterArgs, err := analyticsEventFilter(scope, dateRange.Window())
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	template, prefixArgs, aggregateArgs, err := analyticsTrendSQL(dialect, granularity, dateRange)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	query, err := applyAnalyticsFilter(template, filter)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	args := append(prefixArgs, filterArgs...)
	args = append(args, aggregateArgs...)
	return storefrontAnalyticsQuery{SQL: query, Args: args}, nil
}

func buildSourceQuery(
	dialect string,
	scope analytics.Scope,
	window analytics.TimeWindow,
	sort analytics.SourceSort,
	limit int,
) (storefrontAnalyticsQuery, error) {
	filter, filterArgs, err := analyticsEventFilter(scope, window)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	template, sourceArgs, aggregateArgs, err := analyticsSourcesSQL(dialect, sort)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	query, err := applyAnalyticsFilter(template, filter)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	args := append(sourceArgs, filterArgs...)
	args = append(args, aggregateArgs...)
	args = append(args, limit)
	return storefrontAnalyticsQuery{SQL: query, Args: args}, nil
}

func applyAnalyticsFilter(query string, filter string) (string, error) {
	if strings.Count(query, "%s") != 1 {
		return "", errInvalidAnalyticsQueryTemplate
	}
	return strings.Replace(query, "%s", filter, 1), nil
}

func analyticsEventFilter(scope analytics.Scope, window analytics.TimeWindow) (string, []any, error) {
	return analyticsEventFilterFor(scope, window, analytics.EventAllowlist)
}

func analyticsEventFilterFor(
	scope analytics.Scope,
	window analytics.TimeWindow,
	eventNames []models.StorefrontEventName,
) (string, []any, error) {
	if !scope.IsValid() || window.FromUTC.IsZero() || !window.FromUTC.Before(window.ToUTC) || len(eventNames) == 0 {
		return "", nil, ErrInvalidAnalyticsScope
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(eventNames)), ",")
	filter := "e.occurred_at >= ? AND e.occurred_at < ? AND e.event_name IN (" + placeholders + ")"
	args := []any{window.FromUTC, window.ToUTC}
	for _, eventName := range eventNames {
		if !eventName.IsValid() {
			return "", nil, ErrInvalidAnalyticsScope
		}
		args = append(args, eventName)
	}
	filter += " AND e.traffic_type = ?"
	args = append(args, models.StorefrontTrafficHuman)
	if scope.Mode == analytics.ScopeTenant {
		filter += " AND e.tenant_id = ?"
		args = append(args, scope.TenantID)
	}
	return filter, args, nil
}

func analyticsMetricsSQL(dialect string) (string, []any) {
	args := []any{
		models.StorefrontEventPageView,
		models.StorefrontEventProductImpression,
		models.StorefrontEventProductClick,
		models.StorefrontEventProductView,
		models.StorefrontEventWhatsAppClick,
		models.StorefrontEventPhoneClick,
	}
	if dialect == "postgres" {
		return `
SELECT
    COUNT(DISTINCT e.visitor_id) AS visitors,
    COUNT(DISTINCT e.session_id) AS sessions,
    COUNT(*) FILTER (WHERE e.event_name = ?) AS page_views,
    COUNT(*) FILTER (WHERE e.event_name = ?) AS product_impressions,
    COUNT(*) FILTER (WHERE e.event_name = ?) AS product_clicks,
    COUNT(*) FILTER (WHERE e.event_name = ?) AS product_views,
    COUNT(*) FILTER (WHERE e.event_name IN (?, ?)) AS contact_intents
FROM storefront_events e
WHERE %s`, args
	}
	return `
SELECT
    COUNT(DISTINCT e.visitor_id) AS visitors,
    COUNT(DISTINCT e.session_id) AS sessions,
    COALESCE(SUM(CASE WHEN e.event_name = ? THEN 1 ELSE 0 END), 0) AS page_views,
    COALESCE(SUM(CASE WHEN e.event_name = ? THEN 1 ELSE 0 END), 0) AS product_impressions,
    COALESCE(SUM(CASE WHEN e.event_name = ? THEN 1 ELSE 0 END), 0) AS product_clicks,
    COALESCE(SUM(CASE WHEN e.event_name = ? THEN 1 ELSE 0 END), 0) AS product_views,
    COALESCE(SUM(CASE WHEN e.event_name IN (?, ?) THEN 1 ELSE 0 END), 0) AS contact_intents
FROM storefront_events e
WHERE %s`, args
}

func analyticsTrendSQL(
	dialect string,
	granularity analytics.Granularity,
	dateRange analytics.DateRange,
) (string, []any, []any, error) {
	aggregateArgs := []any{
		models.StorefrontEventPageView,
		models.StorefrontEventProductImpression,
		models.StorefrontEventProductClick,
		models.StorefrontEventProductView,
		models.StorefrontEventWhatsAppClick,
		models.StorefrontEventPhoneClick,
	}
	if dialect == "postgres" {
		var truncUnit string
		var outputFormat string
		switch granularity {
		case analytics.GranularityHour:
			truncUnit = "hour"
			outputFormat = "YYYY-MM-DD\"T\"HH24:00:00"
		case analytics.GranularityDay:
			truncUnit = "day"
			outputFormat = "YYYY-MM-DD"
		case analytics.GranularityWeek:
			truncUnit = "week"
			outputFormat = "YYYY-MM-DD"
		default:
			return "", nil, nil, ErrInvalidAnalyticsScope
		}
		query := fmt.Sprintf(`
WITH filtered AS (
    SELECT
        e.event_name,
        date_trunc('%s', e.occurred_at AT TIME ZONE ?) AS bucket_local
    FROM storefront_events e
    WHERE %%s
)
SELECT
    TO_CHAR(bucket_local, '%s') AS bucket,
    COUNT(*) FILTER (WHERE event_name = ?) AS page_views,
    COUNT(*) FILTER (WHERE event_name = ?) AS product_impressions,
    COUNT(*) FILTER (WHERE event_name = ?) AS product_clicks,
    COUNT(*) FILTER (WHERE event_name = ?) AS product_views,
    COUNT(*) FILTER (WHERE event_name IN (?, ?)) AS contact_intents
FROM filtered
GROUP BY bucket_local
ORDER BY bucket_local ASC`, truncUnit, outputFormat)
		return query, []any{dateRange.Timezone}, aggregateArgs, nil
	}

	bucketExpression, prefixArgs, err := sqliteTrendBucketExpression(granularity, dateRange)
	if err != nil {
		return "", nil, nil, err
	}
	query := fmt.Sprintf(`
WITH filtered AS (
    SELECT e.event_name, %s AS bucket
    FROM storefront_events e
    WHERE %%s
)
SELECT
    bucket,
    COALESCE(SUM(CASE WHEN event_name = ? THEN 1 ELSE 0 END), 0) AS page_views,
    COALESCE(SUM(CASE WHEN event_name = ? THEN 1 ELSE 0 END), 0) AS product_impressions,
    COALESCE(SUM(CASE WHEN event_name = ? THEN 1 ELSE 0 END), 0) AS product_clicks,
    COALESCE(SUM(CASE WHEN event_name = ? THEN 1 ELSE 0 END), 0) AS product_views,
    COALESCE(SUM(CASE WHEN event_name IN (?, ?) THEN 1 ELSE 0 END), 0) AS contact_intents
FROM filtered
GROUP BY bucket
ORDER BY bucket ASC`, bucketExpression)
	return query, prefixArgs, aggregateArgs, nil
}

func sqliteTrendBucketExpression(granularity analytics.Granularity, dateRange analytics.DateRange) (string, []any, error) {
	_, offset := dateRange.FromUTC.In(dateRange.Location).Zone()
	modifier := sqliteTimezoneModifier(offset)
	switch granularity {
	case analytics.GranularityHour:
		return `strftime('%Y-%m-%dT%H:00:00', e.occurred_at, ?)`, []any{modifier}, nil
	case analytics.GranularityDay:
		return `strftime('%Y-%m-%d', e.occurred_at, ?)`, []any{modifier}, nil
	case analytics.GranularityWeek:
		return `date(e.occurred_at, ?, '-' || ((CAST(strftime('%w', e.occurred_at, ?) AS integer) + 6) % 7) || ' days')`, []any{modifier, modifier}, nil
	default:
		return "", nil, ErrInvalidAnalyticsScope
	}
}

func sqliteTimezoneModifier(offsetSeconds int) string {
	sign := "+"
	if offsetSeconds < 0 {
		sign = "-"
		offsetSeconds = -offsetSeconds
	}
	hours := offsetSeconds / 3600
	minutes := (offsetSeconds % 3600) / 60
	return fmt.Sprintf("%s%02d:%02d", sign, hours, minutes)
}

func analyticsSourcesSQL(dialect string, sort analytics.SourceSort) (string, []any, []any, error) {
	orderExpression := "clicks DESC"
	switch sort {
	case analytics.SourceSortImpressions:
		orderExpression = "impressions DESC"
	case analytics.SourceSortClicks:
		orderExpression = "clicks DESC"
	case analytics.SourceSortCTR:
		orderExpression = "ctr DESC"
	case analytics.SourceSortContactIntents:
		orderExpression = "contact_intents DESC"
	default:
		return "", nil, nil, ErrInvalidAnalyticsScope
	}

	sourceArgs := make([]any, 0, len(analytics.SourceAllowlist))
	for _, sourceType := range analytics.SourceAllowlist {
		sourceArgs = append(sourceArgs, sourceType)
	}
	sourcePlaceholders := strings.TrimRight(strings.Repeat("?,", len(sourceArgs)), ",")
	aggregateArgs := []any{
		models.StorefrontEventProductImpression,
		models.StorefrontEventContentImpression,
		models.StorefrontEventProductClick,
		models.StorefrontEventContentClick,
		models.StorefrontEventProductView,
		models.StorefrontEventWhatsAppClick,
		models.StorefrontEventPhoneClick,
	}

	var aggregateSQL string
	if dialect == "postgres" {
		aggregateSQL = `
        COUNT(*) FILTER (WHERE event_name IN (?, ?)) AS impressions,
        COUNT(*) FILTER (WHERE event_name IN (?, ?)) AS clicks,
        COUNT(*) FILTER (WHERE event_name = ?) AS product_views,
        COUNT(*) FILTER (WHERE event_name IN (?, ?)) AS contact_intents`
	} else {
		aggregateSQL = `
        COALESCE(SUM(CASE WHEN event_name IN (?, ?) THEN 1 ELSE 0 END), 0) AS impressions,
        COALESCE(SUM(CASE WHEN event_name IN (?, ?) THEN 1 ELSE 0 END), 0) AS clicks,
        COALESCE(SUM(CASE WHEN event_name = ? THEN 1 ELSE 0 END), 0) AS product_views,
        COALESCE(SUM(CASE WHEN event_name IN (?, ?) THEN 1 ELSE 0 END), 0) AS contact_intents`
	}
	query := fmt.Sprintf(`
WITH filtered AS (
    SELECT
        CASE WHEN e.source_type IN (%s) THEN e.source_type ELSE 'other' END AS source_type,
        e.event_name
    FROM storefront_events e
    WHERE %%s
), source_stats AS (
    SELECT
        source_type,%s
    FROM filtered
    GROUP BY source_type
)
SELECT
    source_type,
    impressions,
    clicks,
    product_views,
    contact_intents,
    CASE WHEN impressions = 0 THEN 0 ELSE CAST(clicks AS DOUBLE PRECISION) / impressions END AS ctr
FROM source_stats
WHERE impressions > 0 OR clicks > 0 OR product_views > 0 OR contact_intents > 0
ORDER BY %s, source_type ASC
LIMIT ?`, sourcePlaceholders, aggregateSQL, orderExpression)
	return query, sourceArgs, aggregateArgs, nil
}
