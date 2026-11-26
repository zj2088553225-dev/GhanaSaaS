package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"saas/internal/analytics"
	"saas/internal/models"

	"gorm.io/gorm"
)

type productAnalyticsListQueries struct {
	Items storefrontAnalyticsQuery
	Total storefrontAnalyticsQuery
}

func (r *StorefrontAnalyticsRepository) ProductAnalyticsList(
	ctx context.Context,
	scope analytics.Scope,
	window analytics.TimeWindow,
	query analytics.ProductAnalyticsQuery,
) ([]analytics.ProductAggregate, int64, error) {
	queries, err := buildProductAnalyticsListQueries(r.db.Dialector.Name(), scope, window, query)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := r.db.WithContext(ctx).Raw(queries.Total.SQL, queries.Total.Args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]analytics.ProductAggregate, 0)
	if err := r.db.WithContext(ctx).Raw(queries.Items.SQL, queries.Items.Args...).Scan(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *StorefrontAnalyticsRepository) TopProductAnalytics(
	ctx context.Context,
	scope analytics.Scope,
	window analytics.TimeWindow,
	query analytics.ProductRankingQuery,
) ([]analytics.ProductAggregate, error) {
	built, err := buildTopProductAnalyticsQuery(r.db.Dialector.Name(), scope, window, query)
	if err != nil {
		return nil, err
	}
	items := make([]analytics.ProductAggregate, 0)
	if err := r.db.WithContext(ctx).Raw(built.SQL, built.Args...).Scan(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *StorefrontAnalyticsRepository) LowCTRProductAnalytics(
	ctx context.Context,
	scope analytics.Scope,
	window analytics.TimeWindow,
	minImpressions int64,
	limit int,
) ([]analytics.ProductAggregate, error) {
	built, err := buildLowCTRProductAnalyticsQuery(r.db.Dialector.Name(), scope, window, minImpressions, limit)
	if err != nil {
		return nil, err
	}
	items := make([]analytics.ProductAggregate, 0)
	if err := r.db.WithContext(ctx).Raw(built.SQL, built.Args...).Scan(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *StorefrontAnalyticsRepository) CurrentProductAnalyticsMetadata(
	ctx context.Context,
	scope analytics.Scope,
	productID uint,
) (*analytics.ProductMetadata, error) {
	if !scope.IsValid() || productID == 0 {
		return nil, ErrInvalidAnalyticsScope
	}
	query := `
SELECT
    p.id AS product_id,
    p.name AS product_name,
    p.tenant_id,
    COALESCE(t.name, '') AS tenant_name,
    p.status,
    COALESCE(p.main_image, '') AS main_image_url,
    TRUE AS currently_exists
FROM spus p
LEFT JOIN tenants t ON t.id = p.tenant_id
WHERE p.id = ?`
	args := []any{productID}
	if scope.Mode == analytics.ScopeTenant {
		query += " AND p.tenant_id = ?"
		args = append(args, scope.TenantID)
	}
	var metadata analytics.ProductMetadata
	result := r.db.WithContext(ctx).Raw(query, args...).Scan(&metadata)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 || metadata.ProductID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &metadata, nil
}

func (r *StorefrontAnalyticsRepository) ProductAnalyticsSummary(
	ctx context.Context,
	scope analytics.Scope,
	window analytics.TimeWindow,
	productID uint,
) (*analytics.ProductAggregate, error) {
	built, err := buildProductAnalyticsSummaryQuery(r.db.Dialector.Name(), scope, window, productID)
	if err != nil {
		return nil, err
	}
	var item analytics.ProductAggregate
	result := r.db.WithContext(ctx).Raw(built.SQL, built.Args...).Scan(&item)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 || item.ProductID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &item, nil
}

func (r *StorefrontAnalyticsRepository) ProductAnalyticsTrends(
	ctx context.Context,
	scope analytics.Scope,
	dateRange analytics.DateRange,
	granularity analytics.Granularity,
	productID uint,
) ([]analytics.ProductTrendPoint, error) {
	built, err := buildProductAnalyticsTrendQuery(r.db.Dialector.Name(), scope, dateRange, granularity, productID)
	if err != nil {
		return nil, err
	}
	points := make([]analytics.ProductTrendPoint, 0)
	if err := r.db.WithContext(ctx).Raw(built.SQL, built.Args...).Scan(&points).Error; err != nil {
		return nil, err
	}
	return points, nil
}

func (r *StorefrontAnalyticsRepository) ProductAnalyticsSources(
	ctx context.Context,
	scope analytics.Scope,
	window analytics.TimeWindow,
	productID uint,
) ([]analytics.ProductSourceStat, error) {
	built, err := buildProductAnalyticsSourcesQuery(r.db.Dialector.Name(), scope, window, productID)
	if err != nil {
		return nil, err
	}
	stats := make([]analytics.ProductSourceStat, 0)
	if err := r.db.WithContext(ctx).Raw(built.SQL, built.Args...).Scan(&stats).Error; err != nil {
		return nil, err
	}
	return stats, nil
}

func (r *StorefrontAnalyticsRepository) ProductAnalyticsSourceLabels(
	ctx context.Context,
	sources []analytics.ProductSourceStat,
) ([]analytics.ProductSourceLabel, error) {
	categoryIDs := make([]uint, 0)
	shopIDs := make([]uint, 0)
	shopSlugs := make([]string, 0)
	contentKeys := make(map[string]struct{})
	requested := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		if source.SourceID == nil || strings.TrimSpace(*source.SourceID) == "" {
			continue
		}
		sourceID := strings.TrimSpace(*source.SourceID)
		requested[productSourceLabelKey(source.SourceType, sourceID)] = struct{}{}
		switch source.SourceType {
		case string(models.StorefrontSourceCategory):
			if value, err := strconv.ParseUint(sourceID, 10, 64); err == nil && value > 0 {
				categoryIDs = append(categoryIDs, uint(value))
			}
		case string(models.StorefrontSourceShop):
			if value, err := strconv.ParseUint(sourceID, 10, 64); err == nil && value > 0 {
				shopIDs = append(shopIDs, uint(value))
			} else {
				shopSlugs = append(shopSlugs, sourceID)
			}
		case string(models.StorefrontSourceHomeProductFloor), string(models.StorefrontSourceMarketingContent):
			contentKeys[sourceID] = struct{}{}
		}
	}

	labels := make(map[string]string)
	if len(categoryIDs) > 0 {
		var categories []models.PlatformCategory
		if err := r.db.WithContext(ctx).Where("id IN ?", uniqueUintValues(categoryIDs)).Find(&categories).Error; err != nil {
			return nil, err
		}
		for _, category := range categories {
			label := strings.TrimSpace(category.Path)
			if label == "" {
				label = strings.TrimSpace(category.Name)
			}
			if label != "" {
				labels[productSourceLabelKey(string(models.StorefrontSourceCategory), strconv.FormatUint(uint64(category.ID), 10))] = label
			}
		}
	}
	if len(shopIDs) > 0 || len(shopSlugs) > 0 {
		var shops []models.ShopProfile
		query := r.db.WithContext(ctx).Model(&models.ShopProfile{})
		switch {
		case len(shopIDs) > 0 && len(shopSlugs) > 0:
			query = query.Where("tenant_id IN ? OR slug IN ?", uniqueUintValues(shopIDs), uniqueStringValues(shopSlugs))
		case len(shopIDs) > 0:
			query = query.Where("tenant_id IN ?", uniqueUintValues(shopIDs))
		default:
			query = query.Where("slug IN ?", uniqueStringValues(shopSlugs))
		}
		if err := query.Find(&shops).Error; err != nil {
			return nil, err
		}
		for _, shop := range shops {
			label := strings.TrimSpace(shop.ShopName)
			if label == "" {
				continue
			}
			labels[productSourceLabelKey(string(models.StorefrontSourceShop), strconv.FormatUint(uint64(shop.TenantID), 10))] = label
			labels[productSourceLabelKey(string(models.StorefrontSourceShop), shop.Slug)] = label
		}
	}
	if len(contentKeys) > 0 {
		var page models.SitePage
		err := r.db.WithContext(ctx).Where("page_key = ?", "home").First(&page).Error
		switch {
		case err == nil:
			cmsLabels := make(map[string]string)
			collectAnalyticsCMSLabels(page.PublishedContent, contentKeys, cmsLabels)
			collectAnalyticsCMSLabels(page.DraftContent, contentKeys, cmsLabels)
			for trackingKey, label := range cmsLabels {
				labels[productSourceLabelKey(string(models.StorefrontSourceHomeProductFloor), trackingKey)] = label
				labels[productSourceLabelKey(string(models.StorefrontSourceMarketingContent), trackingKey)] = label
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
		default:
			return nil, err
		}
	}

	result := make([]analytics.ProductSourceLabel, 0, len(labels))
	for key, label := range labels {
		if _, ok := requested[key]; !ok {
			continue
		}
		sourceType, sourceID, ok := splitProductSourceLabelKey(key)
		if !ok {
			continue
		}
		result = append(result, analytics.ProductSourceLabel{SourceType: sourceType, SourceID: sourceID, Label: label})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SourceType == result[j].SourceType {
			return result[i].SourceID < result[j].SourceID
		}
		return result[i].SourceType < result[j].SourceType
	})
	return result, nil
}

func buildProductAnalyticsListQueries(
	dialect string,
	scope analytics.Scope,
	window analytics.TimeWindow,
	query analytics.ProductAnalyticsQuery,
) (productAnalyticsListQueries, error) {
	base, err := buildProductAnalyticsBaseQuery(dialect, scope, window, nil, query.Search, query.Status)
	if err != nil {
		return productAnalyticsListQueries{}, err
	}
	metric, err := productMetricSQL(query.Sort)
	if err != nil {
		return productAnalyticsListQueries{}, err
	}
	order, err := analyticsOrderSQL(query.Order)
	if err != nil {
		return productAnalyticsListQueries{}, err
	}
	if query.Page < 1 || query.PageSize < 1 {
		return productAnalyticsListQueries{}, ErrInvalidAnalyticsScope
	}
	itemsSQL := base.SQL + "\n" + productAnalyticsSelectSQL + "\nORDER BY " + metric + " " + order + ", product_id ASC\nLIMIT ? OFFSET ?"
	itemsArgs := append(append([]any{}, base.Args...), query.PageSize, (query.Page-1)*query.PageSize)
	totalSQL := base.SQL + "\nSELECT COUNT(*) FROM scored_products"
	return productAnalyticsListQueries{
		Items: storefrontAnalyticsQuery{SQL: itemsSQL, Args: itemsArgs},
		Total: storefrontAnalyticsQuery{SQL: totalSQL, Args: append([]any{}, base.Args...)},
	}, nil
}

func buildTopProductAnalyticsQuery(
	dialect string,
	scope analytics.Scope,
	window analytics.TimeWindow,
	query analytics.ProductRankingQuery,
) (storefrontAnalyticsQuery, error) {
	base, err := buildProductAnalyticsBaseQuery(dialect, scope, window, nil, "", "")
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	metric, err := productMetricSQL(query.Metric)
	if err != nil || query.Limit < 1 {
		return storefrontAnalyticsQuery{}, ErrInvalidAnalyticsScope
	}
	thresholdSQL := ""
	thresholdArgs := make([]any, 0, 1)
	switch query.Metric {
	case analytics.ProductMetricCTR:
		thresholdSQL = "\nWHERE impressions >= ?"
		thresholdArgs = append(thresholdArgs, query.MinImpressions)
	case analytics.ProductMetricDetailViewRate:
		thresholdSQL = "\nWHERE clicks >= ?"
		thresholdArgs = append(thresholdArgs, query.MinClicks)
	case analytics.ProductMetricContactIntentRate:
		thresholdSQL = "\nWHERE views >= ?"
		thresholdArgs = append(thresholdArgs, query.MinViews)
	}
	querySQL := base.SQL + "\n" + productAnalyticsSelectSQL + thresholdSQL + "\nORDER BY " + metric + " DESC, impressions DESC, product_id ASC\nLIMIT ?"
	args := append(append([]any{}, base.Args...), thresholdArgs...)
	args = append(args, query.Limit)
	return storefrontAnalyticsQuery{SQL: querySQL, Args: args}, nil
}

func buildLowCTRProductAnalyticsQuery(
	dialect string,
	scope analytics.Scope,
	window analytics.TimeWindow,
	minImpressions int64,
	limit int,
) (storefrontAnalyticsQuery, error) {
	if minImpressions < 0 || limit < 1 {
		return storefrontAnalyticsQuery{}, ErrInvalidAnalyticsScope
	}
	base, err := buildProductAnalyticsBaseQuery(dialect, scope, window, nil, "", "")
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	querySQL := base.SQL + "\n" + productAnalyticsSelectSQL + `
WHERE impressions >= ?
ORDER BY ctr ASC, impressions DESC, product_id ASC
LIMIT ?`
	args := append(append([]any{}, base.Args...), minImpressions, limit)
	return storefrontAnalyticsQuery{SQL: querySQL, Args: args}, nil
}

func buildProductAnalyticsSummaryQuery(
	dialect string,
	scope analytics.Scope,
	window analytics.TimeWindow,
	productID uint,
) (storefrontAnalyticsQuery, error) {
	if productID == 0 {
		return storefrontAnalyticsQuery{}, ErrInvalidAnalyticsScope
	}
	base, err := buildProductAnalyticsBaseQuery(dialect, scope, window, &productID, "", "")
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	return storefrontAnalyticsQuery{SQL: base.SQL + "\n" + productAnalyticsSelectSQL + "\nLIMIT 1", Args: base.Args}, nil
}

const productAnalyticsSelectSQL = `SELECT
    product_id,
    product_name,
    tenant_id,
    tenant_name,
    status,
    main_image_url,
    currently_exists,
    impressions,
    clicks,
    views,
    contact_intents,
    unique_visitors
FROM scored_products`

func buildProductAnalyticsBaseQuery(
	dialect string,
	scope analytics.Scope,
	window analytics.TimeWindow,
	productID *uint,
	search string,
	status analytics.ProductStatusFilter,
) (storefrontAnalyticsQuery, error) {
	filter, filterArgs, err := analyticsEventFilterFor(scope, window, analytics.ProductEventAllowlist)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	filter += " AND e.product_id IS NOT NULL"
	if productID != nil {
		if *productID == 0 {
			return storefrontAnalyticsQuery{}, ErrInvalidAnalyticsScope
		}
		filter += " AND e.product_id = ?"
		filterArgs = append(filterArgs, *productID)
	}

	aggregateSQL, aggregateArgs := productAnalyticsAggregateSQL(dialect)
	metadataWhere := ""
	metadataArgs := make([]any, 0, 3)
	search = strings.ToLower(strings.TrimSpace(search))
	if search != "" {
		metadataWhere += ` AND p.id IS NOT NULL AND (
        LOWER(p.name) LIKE ? OR
        LOWER(REPLACE(TRIM(p.name), ' ', '-') || '-' || CAST(p.id AS TEXT)) LIKE ?
    )`
		pattern := "%" + search + "%"
		metadataArgs = append(metadataArgs, pattern, pattern)
	}
	switch status {
	case "":
	case analytics.ProductStatusDeleted:
		metadataWhere += " AND p.id IS NULL"
	case analytics.ProductStatusFilter(models.SPUStatusOnShelf), analytics.ProductStatusFilter(models.SPUStatusOffShelf):
		metadataWhere += " AND p.id IS NOT NULL AND p.status = ?"
		metadataArgs = append(metadataArgs, status)
	default:
		return storefrontAnalyticsQuery{}, ErrInvalidAnalyticsScope
	}

	template := fmt.Sprintf(`
WITH filtered_events AS (
    SELECT e.product_id, e.tenant_id, e.event_name, e.visitor_id
    FROM storefront_events e
    WHERE %%s
), product_aggregates AS (
    SELECT
        product_id,
        tenant_id,%s
    FROM filtered_events
    GROUP BY product_id, tenant_id
), joined_products AS (
    SELECT
        pa.product_id,
        COALESCE(NULLIF(p.name, ''), 'Deleted product #' || CAST(pa.product_id AS TEXT)) AS product_name,
        COALESCE(pa.tenant_id, 0) AS tenant_id,
        COALESCE(t.name, '') AS tenant_name,
        COALESCE(p.status, '') AS status,
        COALESCE(p.main_image, '') AS main_image_url,
        CASE WHEN p.id IS NULL THEN FALSE ELSE TRUE END AS currently_exists,
        pa.impressions,
        pa.clicks,
        pa.views,
        pa.contact_intents,
        pa.unique_visitors
    FROM product_aggregates pa
    LEFT JOIN spus p ON p.id = pa.product_id AND p.tenant_id = pa.tenant_id
    LEFT JOIN tenants t ON t.id = pa.tenant_id
    WHERE 1 = 1%s
), scored_products AS (
    SELECT
        *,
        CASE WHEN impressions = 0 THEN 0 ELSE CAST(clicks AS DOUBLE PRECISION) / impressions END AS ctr,
        CASE WHEN clicks = 0 THEN 0 ELSE CAST(views AS DOUBLE PRECISION) / clicks END AS detail_view_rate,
        CASE WHEN views = 0 THEN 0 ELSE CAST(contact_intents AS DOUBLE PRECISION) / views END AS contact_intent_rate
    FROM joined_products
)`, aggregateSQL, metadataWhere)
	query, err := applyAnalyticsFilter(template, filter)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	args := append(append([]any{}, filterArgs...), aggregateArgs...)
	args = append(args, metadataArgs...)
	return storefrontAnalyticsQuery{SQL: query, Args: args}, nil
}

func productAnalyticsAggregateSQL(dialect string) (string, []any) {
	args := []any{
		models.StorefrontEventProductImpression,
		models.StorefrontEventProductClick,
		models.StorefrontEventProductView,
		models.StorefrontEventWhatsAppClick,
		models.StorefrontEventPhoneClick,
	}
	if dialect == "postgres" {
		return `
        COUNT(*) FILTER (WHERE event_name = ?) AS impressions,
        COUNT(*) FILTER (WHERE event_name = ?) AS clicks,
        COUNT(*) FILTER (WHERE event_name = ?) AS views,
        COUNT(*) FILTER (WHERE event_name IN (?, ?)) AS contact_intents,
        COUNT(DISTINCT visitor_id) AS unique_visitors`, args
	}
	return `
        COALESCE(SUM(CASE WHEN event_name = ? THEN 1 ELSE 0 END), 0) AS impressions,
        COALESCE(SUM(CASE WHEN event_name = ? THEN 1 ELSE 0 END), 0) AS clicks,
        COALESCE(SUM(CASE WHEN event_name = ? THEN 1 ELSE 0 END), 0) AS views,
        COALESCE(SUM(CASE WHEN event_name IN (?, ?) THEN 1 ELSE 0 END), 0) AS contact_intents,
        COUNT(DISTINCT visitor_id) AS unique_visitors`, args
}

func productMetricSQL(metric analytics.ProductMetric) (string, error) {
	switch metric {
	case analytics.ProductMetricImpressions:
		return "impressions", nil
	case analytics.ProductMetricClicks:
		return "clicks", nil
	case analytics.ProductMetricViews:
		return "views", nil
	case analytics.ProductMetricContactIntents:
		return "contact_intents", nil
	case analytics.ProductMetricUniqueVisitors:
		return "unique_visitors", nil
	case analytics.ProductMetricCTR:
		return "ctr", nil
	case analytics.ProductMetricDetailViewRate:
		return "detail_view_rate", nil
	case analytics.ProductMetricContactIntentRate:
		return "contact_intent_rate", nil
	default:
		return "", ErrInvalidAnalyticsScope
	}
}

func analyticsOrderSQL(order analytics.SortOrder) (string, error) {
	switch order {
	case analytics.SortOrderAscending:
		return "ASC", nil
	case analytics.SortOrderDescending:
		return "DESC", nil
	default:
		return "", ErrInvalidAnalyticsScope
	}
}

func buildProductAnalyticsTrendQuery(
	dialect string,
	scope analytics.Scope,
	dateRange analytics.DateRange,
	granularity analytics.Granularity,
	productID uint,
) (storefrontAnalyticsQuery, error) {
	if productID == 0 {
		return storefrontAnalyticsQuery{}, ErrInvalidAnalyticsScope
	}
	filter, filterArgs, err := analyticsEventFilterFor(scope, dateRange.Window(), analytics.ProductEventAllowlist)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	filter += " AND e.product_id = ?"
	filterArgs = append(filterArgs, productID)

	aggregateSQL, aggregateArgs := productTrendAggregateSQL(dialect)
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

func postgresTrendFragments(granularity analytics.Granularity) (string, string, error) {
	switch granularity {
	case analytics.GranularityHour:
		return "hour", `YYYY-MM-DD"T"HH24:00:00`, nil
	case analytics.GranularityDay:
		return "day", "YYYY-MM-DD", nil
	case analytics.GranularityWeek:
		return "week", "YYYY-MM-DD", nil
	default:
		return "", "", ErrInvalidAnalyticsScope
	}
}

func productTrendAggregateSQL(dialect string) (string, []any) {
	args := []any{
		models.StorefrontEventProductImpression,
		models.StorefrontEventProductClick,
		models.StorefrontEventProductView,
		models.StorefrontEventWhatsAppClick,
		models.StorefrontEventPhoneClick,
	}
	if dialect == "postgres" {
		return `
    COUNT(*) FILTER (WHERE event_name = ?) AS impressions,
    COUNT(*) FILTER (WHERE event_name = ?) AS clicks,
    COUNT(*) FILTER (WHERE event_name = ?) AS views,
    COUNT(*) FILTER (WHERE event_name IN (?, ?)) AS contact_intents,
    COUNT(DISTINCT visitor_id) AS unique_visitors`, args
	}
	return `
    COALESCE(SUM(CASE WHEN event_name = ? THEN 1 ELSE 0 END), 0) AS impressions,
    COALESCE(SUM(CASE WHEN event_name = ? THEN 1 ELSE 0 END), 0) AS clicks,
    COALESCE(SUM(CASE WHEN event_name = ? THEN 1 ELSE 0 END), 0) AS views,
    COALESCE(SUM(CASE WHEN event_name IN (?, ?) THEN 1 ELSE 0 END), 0) AS contact_intents,
    COUNT(DISTINCT visitor_id) AS unique_visitors`, args
}

func buildProductAnalyticsSourcesQuery(
	dialect string,
	scope analytics.Scope,
	window analytics.TimeWindow,
	productID uint,
) (storefrontAnalyticsQuery, error) {
	if productID == 0 {
		return storefrontAnalyticsQuery{}, ErrInvalidAnalyticsScope
	}
	filter, filterArgs, err := analyticsEventFilterFor(scope, window, analytics.ProductEventAllowlist)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	filter += " AND e.product_id = ?"
	filterArgs = append(filterArgs, productID)
	sourceArgs := make([]any, 0, len(analytics.SourceAllowlist))
	for _, sourceType := range analytics.SourceAllowlist {
		sourceArgs = append(sourceArgs, sourceType)
	}
	sourcePlaceholders := strings.TrimRight(strings.Repeat("?,", len(sourceArgs)), ",")
	aggregateSQL, aggregateArgs := productTrendAggregateSQL(dialect)
	var ctrSQL string
	if dialect == "postgres" {
		ctrSQL = "CASE WHEN COUNT(*) FILTER (WHERE event_name = ?) = 0 THEN 0 ELSE CAST(COUNT(*) FILTER (WHERE event_name = ?) AS DOUBLE PRECISION) / COUNT(*) FILTER (WHERE event_name = ?) END AS ctr"
	} else {
		ctrSQL = "CASE WHEN SUM(CASE WHEN event_name = ? THEN 1 ELSE 0 END) = 0 THEN 0 ELSE CAST(SUM(CASE WHEN event_name = ? THEN 1 ELSE 0 END) AS DOUBLE PRECISION) / SUM(CASE WHEN event_name = ? THEN 1 ELSE 0 END) END AS ctr"
	}
	template := fmt.Sprintf(`
WITH filtered AS (
    SELECT
        CASE WHEN e.source_type IN (%s) THEN e.source_type ELSE 'other' END AS source_type,
        e.source_id,
        e.event_name,
        e.visitor_id
    FROM storefront_events e
    WHERE %%s
)
SELECT
    source_type,
    source_id,%s,
    %s
FROM filtered
GROUP BY source_type, source_id
ORDER BY clicks DESC, impressions DESC, source_type ASC, source_id ASC`, sourcePlaceholders, aggregateSQL, ctrSQL)
	query, err := applyAnalyticsFilter(template, filter)
	if err != nil {
		return storefrontAnalyticsQuery{}, err
	}
	args := append(append([]any{}, sourceArgs...), filterArgs...)
	args = append(args, aggregateArgs...)
	args = append(args,
		models.StorefrontEventProductImpression,
		models.StorefrontEventProductClick,
		models.StorefrontEventProductImpression,
	)
	return storefrontAnalyticsQuery{SQL: query, Args: args}, nil
}

func productSourceLabelKey(sourceType string, sourceID string) string {
	return sourceType + "\x00" + sourceID
}

func splitProductSourceLabelKey(value string) (string, string, bool) {
	parts := strings.SplitN(value, "\x00", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func uniqueUintValues(values []uint) []uint {
	seen := make(map[uint]struct{}, len(values))
	result := make([]uint, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func uniqueStringValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func collectAnalyticsCMSLabels(raw []byte, wanted map[string]struct{}, labels map[string]string) {
	if len(raw) == 0 || len(wanted) == 0 {
		return
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return
	}
	var walk func(any)
	walk = func(node any) {
		switch typed := node.(type) {
		case map[string]any:
			trackingKey, _ := typed["tracking_key"].(string)
			trackingKey = strings.ToLower(strings.TrimSpace(trackingKey))
			if _, ok := wanted[trackingKey]; ok {
				if _, exists := labels[trackingKey]; !exists {
					for _, field := range []string{"title", "name", "alt", "buttonText", "button_text"} {
						if label, ok := typed[field].(string); ok && strings.TrimSpace(label) != "" {
							labels[trackingKey] = strings.TrimSpace(label)
							break
						}
					}
				}
			}
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
}
