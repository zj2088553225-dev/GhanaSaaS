package analytics

import (
	"strings"

	"saas/internal/models"
)

const (
	MaxContentAnalyticsSearch    = 200
	MaxContentAnalyticsThreshold = 1_000_000_000
	ContentPageBreakdownLimit    = 20
)

var ContentEventAllowlist = []models.StorefrontEventName{
	models.StorefrontEventContentImpression,
	models.StorefrontEventContentClick,
}

type ContentType string

const (
	ContentTypeHero         ContentType = "hero"
	ContentTypeHeroBanner   ContentType = "hero_banner"
	ContentTypeHeroSlide    ContentType = "hero_slide"
	ContentTypePromoBanner  ContentType = "promo_banner"
	ContentTypeImage        ContentType = "image"
	ContentTypeImageText    ContentType = "image_text"
	ContentTypeContact      ContentType = "contact"
	ContentTypeCategoryItem ContentType = "category_item"
	ContentTypeFeatureGrid  ContentType = "feature_grid"
	ContentTypeLink         ContentType = "link"
	ContentTypeOther        ContentType = "other"
)

func ParseContentType(value string) (ContentType, bool) {
	contentType := ContentType(strings.ToLower(strings.TrimSpace(value)))
	switch contentType {
	case "":
		return "", true
	case ContentTypeHero,
		ContentTypeHeroBanner,
		ContentTypeHeroSlide,
		ContentTypePromoBanner,
		ContentTypeImage,
		ContentTypeImageText,
		ContentTypeContact,
		ContentTypeCategoryItem,
		ContentTypeFeatureGrid,
		ContentTypeLink,
		ContentTypeOther:
		return contentType, true
	default:
		return "", false
	}
}

type ContentTargetType string

const (
	ContentTargetProduct  ContentTargetType = "product"
	ContentTargetShop     ContentTargetType = "shop"
	ContentTargetCategory ContentTargetType = "category"
	ContentTargetInternal ContentTargetType = "internal"
	ContentTargetExternal ContentTargetType = "external"
	ContentTargetWhatsApp ContentTargetType = "whatsapp"
	ContentTargetPhone    ContentTargetType = "phone"
	ContentTargetNone     ContentTargetType = "none"
)

type MarketingContentMetadata struct {
	TrackingKey string            `json:"tracking_key"`
	ContentType ContentType       `json:"content_type"`
	Title       string            `json:"title"`
	ImageURL    string            `json:"image_url"`
	TargetType  ContentTargetType `json:"target_type"`
	TargetValue string            `json:"target_value"`
	Published   bool              `json:"published"`
}

type ContentMetric string

const (
	ContentMetricImpressions    ContentMetric = "impressions"
	ContentMetricClicks         ContentMetric = "clicks"
	ContentMetricUniqueVisitors ContentMetric = "unique_visitors"
	ContentMetricCTR            ContentMetric = "ctr"
)

func ParseContentMetric(value string) (ContentMetric, bool) {
	if strings.TrimSpace(value) == "" {
		return ContentMetricClicks, true
	}
	metric := ContentMetric(strings.ToLower(strings.TrimSpace(value)))
	switch metric {
	case ContentMetricImpressions, ContentMetricClicks, ContentMetricUniqueVisitors, ContentMetricCTR:
		return metric, true
	default:
		return "", false
	}
}

// ContentAnalyticsQuery controls SQL aggregation. When Paginate is false the
// repository still returns only GROUP BY entity_id rows; it never returns raw
// storefront events. The service uses that path for CMS metadata filters.
type ContentAnalyticsQuery struct {
	Sort           ContentMetric
	Order          SortOrder
	Page           int
	PageSize       int
	MinImpressions int64
	Paginate       bool
}

type ContentCounts struct {
	Impressions    int64 `json:"impressions"`
	Clicks         int64 `json:"clicks"`
	UniqueVisitors int64 `json:"unique_visitors"`
}

func (counts ContentCounts) CTR() float64 {
	return Ratio(counts.Clicks, counts.Impressions)
}

type ContentAggregate struct {
	TrackingKey string `json:"tracking_key"`
	ContentCounts
}

type ContentTrendPoint struct {
	Bucket string `json:"bucket"`
	ContentCounts
}

type ContentLocaleStat struct {
	Locale string `json:"locale"`
	ContentCounts
	CTR float64 `json:"ctr"`
}

type ContentDeviceStat struct {
	DeviceType string `json:"device_type"`
	ContentCounts
	CTR float64 `json:"ctr"`
}

type ContentPageStat struct {
	PagePath    string `json:"page_path"`
	Impressions int64  `json:"impressions"`
	Clicks      int64  `json:"clicks"`
}
