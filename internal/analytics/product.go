package analytics

import (
	"strings"

	"saas/internal/models"
)

const (
	DefaultReportPageSize      = 20
	MaxReportPageSize          = 100
	DefaultProductRankingLimit = 10
	MaxProductRankingLimit     = 100
	DefaultMinImpressions      = 100
	DefaultMinClicks           = 20
	DefaultMinViews            = 10
	MaxProductRankingThreshold = 1_000_000_000
	MaxProductAnalyticsSearch  = 200
)

var ProductEventAllowlist = []models.StorefrontEventName{
	models.StorefrontEventProductImpression,
	models.StorefrontEventProductClick,
	models.StorefrontEventProductView,
	models.StorefrontEventWhatsAppClick,
	models.StorefrontEventPhoneClick,
}

type SortOrder string

const (
	SortOrderAscending  SortOrder = "asc"
	SortOrderDescending SortOrder = "desc"
)

func ParseSortOrder(value string) (SortOrder, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(SortOrderDescending):
		return SortOrderDescending, true
	case string(SortOrderAscending):
		return SortOrderAscending, true
	default:
		return "", false
	}
}

type ProductMetric string

const (
	ProductMetricImpressions       ProductMetric = "impressions"
	ProductMetricClicks            ProductMetric = "clicks"
	ProductMetricViews             ProductMetric = "views"
	ProductMetricContactIntents    ProductMetric = "contact_intents"
	ProductMetricUniqueVisitors    ProductMetric = "unique_visitors"
	ProductMetricCTR               ProductMetric = "ctr"
	ProductMetricDetailViewRate    ProductMetric = "detail_view_rate"
	ProductMetricContactIntentRate ProductMetric = "contact_intent_rate"
)

func ParseProductMetric(value string) (ProductMetric, bool) {
	if strings.TrimSpace(value) == "" {
		return ProductMetricClicks, true
	}
	metric := ProductMetric(strings.ToLower(strings.TrimSpace(value)))
	switch metric {
	case ProductMetricImpressions,
		ProductMetricClicks,
		ProductMetricViews,
		ProductMetricContactIntents,
		ProductMetricUniqueVisitors,
		ProductMetricCTR,
		ProductMetricDetailViewRate,
		ProductMetricContactIntentRate:
		return metric, true
	default:
		return "", false
	}
}

type ProductStatusFilter string

const ProductStatusDeleted ProductStatusFilter = "deleted"

func ParseProductStatusFilter(value string) (ProductStatusFilter, bool) {
	status := ProductStatusFilter(strings.ToLower(strings.TrimSpace(value)))
	switch status {
	case "", ProductStatusFilter(models.SPUStatusOnShelf), ProductStatusFilter(models.SPUStatusOffShelf), ProductStatusDeleted:
		return status, true
	default:
		return "", false
	}
}

type ProductAnalyticsQuery struct {
	Search   string
	Status   ProductStatusFilter
	Sort     ProductMetric
	Order    SortOrder
	Page     int
	PageSize int
}

type ProductRankingQuery struct {
	Metric         ProductMetric
	Limit          int
	MinImpressions int64
	MinClicks      int64
	MinViews       int64
}

type ProductMetadata struct {
	ProductID       uint             `json:"product_id"`
	ProductName     string           `json:"product_name"`
	ProductSlug     string           `json:"product_slug"`
	TenantID        uint             `json:"tenant_id"`
	TenantName      string           `json:"tenant_name"`
	Status          models.SPUStatus `json:"status"`
	MainImageURL    string           `json:"main_image_url"`
	CurrentlyExists bool             `json:"currently_exists"`
}

type ProductCounts struct {
	Impressions    int64 `json:"impressions"`
	Clicks         int64 `json:"clicks"`
	Views          int64 `json:"views"`
	ContactIntents int64 `json:"contact_intents"`
	UniqueVisitors int64 `json:"unique_visitors"`
}

func (counts ProductCounts) Rates() FunnelRates {
	return FunnelRates{
		ProductCTR:        Ratio(counts.Clicks, counts.Impressions),
		DetailViewRate:    Ratio(counts.Views, counts.Clicks),
		ContactIntentRate: Ratio(counts.ContactIntents, counts.Views),
	}
}

type ProductAggregate struct {
	ProductMetadata
	ProductCounts
}

type ProductTrendPoint struct {
	Bucket string `json:"bucket"`
	ProductCounts
}

type ProductSourceStat struct {
	SourceType  string  `json:"source_type"`
	SourceID    *string `json:"source_id"`
	SourceLabel *string `json:"source_label"`
	ProductCounts
	CTR float64 `json:"ctr"`
}

type ProductSourceLabel struct {
	SourceType string
	SourceID   string
	Label      string
}
