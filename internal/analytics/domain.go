package analytics

import (
	"math"

	"saas/internal/models"
)

const (
	DefaultTimezone      = "Africa/Accra"
	DefaultDateRangeDays = 7
	MaxStandardRangeDays = 366
	MaxWeeklyRangeDays   = 732
	MaxHourlyRangeDays   = 31
	DefaultSourceLimit   = 20
	MaxSourceLimit       = 50
)

type ScopeMode string

const (
	ScopeAll    ScopeMode = "all"
	ScopeTenant ScopeMode = "tenant"
)

// Scope is the single authorization boundary used by every analytics query.
// ScopeAll deliberately has no tenant predicate and therefore includes rows
// whose tenant_id is NULL. ScopeTenant always uses an exact tenant predicate.
type Scope struct {
	Mode     ScopeMode
	TenantID uint
}

func AllScope() Scope {
	return Scope{Mode: ScopeAll}
}

func TenantScope(tenantID uint) Scope {
	return Scope{Mode: ScopeTenant, TenantID: tenantID}
}

func (scope Scope) IsValid() bool {
	switch scope.Mode {
	case ScopeAll:
		return scope.TenantID == 0
	case ScopeTenant:
		return scope.TenantID > 0
	default:
		return false
	}
}

var EventAllowlist = []models.StorefrontEventName{
	models.StorefrontEventPageView,
	models.StorefrontEventProductImpression,
	models.StorefrontEventProductClick,
	models.StorefrontEventProductView,
	models.StorefrontEventContentImpression,
	models.StorefrontEventContentClick,
	models.StorefrontEventWhatsAppClick,
	models.StorefrontEventPhoneClick,
}

var SourceAllowlist = []models.StorefrontSourceType{
	models.StorefrontSourceHomeProductFloor,
	models.StorefrontSourceMall,
	models.StorefrontSourceCategory,
	models.StorefrontSourceSearch,
	models.StorefrontSourceShop,
	models.StorefrontSourceProductDetail,
	models.StorefrontSourceMarketingContent,
	models.StorefrontSourceDirect,
}

func NormalizeSource(value string) string {
	for _, allowed := range SourceAllowlist {
		if value == string(allowed) {
			return value
		}
	}
	return "other"
}

// EventCounts is the shared raw metric contract. Repositories obtain these
// values with conditional SQL aggregation; services derive all rates from it.
type EventCounts struct {
	Visitors           int64 `json:"visitors"`
	Sessions           int64 `json:"sessions"`
	PageViews          int64 `json:"page_views"`
	ProductImpressions int64 `json:"product_impressions"`
	ProductClicks      int64 `json:"product_clicks"`
	ProductViews       int64 `json:"product_views"`
	ContactIntents     int64 `json:"contact_intents"`
}

type FunnelRates struct {
	ProductCTR        float64 `json:"product_ctr"`
	DetailViewRate    float64 `json:"detail_view_rate"`
	ContactIntentRate float64 `json:"contact_intent_rate"`
}

func (counts EventCounts) Rates() FunnelRates {
	return FunnelRates{
		ProductCTR:        Ratio(counts.ProductClicks, counts.ProductImpressions),
		DetailViewRate:    Ratio(counts.ProductViews, counts.ProductClicks),
		ContactIntentRate: Ratio(counts.ContactIntents, counts.ProductViews),
	}
}

func Ratio(numerator int64, denominator int64) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

// ChangeRate returns nil only for a positive current value against a zero
// previous value. It never returns NaN or infinity.
func ChangeRate(value float64, previous float64) *float64 {
	if previous == 0 {
		if value == 0 {
			zero := float64(0)
			return &zero
		}
		return nil
	}
	change := (value - previous) / previous
	if math.IsNaN(change) || math.IsInf(change, 0) {
		return nil
	}
	return &change
}

type Granularity string

const (
	GranularityHour Granularity = "hour"
	GranularityDay  Granularity = "day"
	GranularityWeek Granularity = "week"
)

func ResolveGranularity(value string, rangeDays int) (Granularity, bool) {
	if value == "" {
		if rangeDays <= 2 {
			return GranularityHour, true
		}
		return GranularityDay, true
	}
	granularity := Granularity(value)
	switch granularity {
	case GranularityHour:
		return granularity, rangeDays <= MaxHourlyRangeDays
	case GranularityDay:
		return granularity, rangeDays <= MaxStandardRangeDays
	case GranularityWeek:
		return granularity, rangeDays <= MaxWeeklyRangeDays
	default:
		return "", false
	}
}

type SourceSort string

const (
	SourceSortImpressions    SourceSort = "impressions"
	SourceSortClicks         SourceSort = "clicks"
	SourceSortCTR            SourceSort = "ctr"
	SourceSortContactIntents SourceSort = "contact_intents"
)

func ParseSourceSort(value string) (SourceSort, bool) {
	if value == "" {
		return SourceSortClicks, true
	}
	sort := SourceSort(value)
	switch sort {
	case SourceSortImpressions, SourceSortClicks, SourceSortCTR, SourceSortContactIntents:
		return sort, true
	default:
		return "", false
	}
}

type SparseTrendPoint struct {
	Bucket             string `json:"bucket"`
	PageViews          int64  `json:"page_views"`
	ProductImpressions int64  `json:"product_impressions"`
	ProductClicks      int64  `json:"product_clicks"`
	ProductViews       int64  `json:"product_views"`
	ContactIntents     int64  `json:"contact_intents"`
}

type SourceStat struct {
	SourceType     string  `json:"source_type"`
	Impressions    int64   `json:"impressions"`
	Clicks         int64   `json:"clicks"`
	ProductViews   int64   `json:"product_views"`
	ContactIntents int64   `json:"contact_intents"`
	CTR            float64 `json:"ctr"`
}
