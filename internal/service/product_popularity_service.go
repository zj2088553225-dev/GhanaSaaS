package service

import (
	"context"
	"fmt"
	"time"

	"saas/internal/models"
	"saas/internal/pkg/locale"
	"saas/internal/repository"
)

const (
	DefaultProductPopularityLimit    = 20
	MaxProductPopularityLimit        = 100
	DefaultProductPopularityCacheTTL = 120 * time.Second
)

type ProductPopularityRequest struct {
	Limit      int
	CategoryID *uint
	TenantID   *uint
	SuperAdmin bool
	Public     bool
	Locale     string
}

type ProductPopularity struct {
	ProductID          uint    `json:"product_id"`
	TenantID           uint    `json:"tenant_id,omitempty"`
	CategoryID         *uint   `json:"category_id"`
	PlatformCategoryID uint    `json:"platform_category_id"`
	Name               string  `json:"name"`
	MainImage          string  `json:"main_image"`
	MinPrice           int64   `json:"min_price"`
	MaxPrice           int64   `json:"max_price"`
	OriginalPrice      int64   `json:"original_price"`
	HotScore           float64 `json:"hot_score"`
	Views              int64   `json:"views"`
	Clicks             int64   `json:"clicks"`
	Contacts           int64   `json:"contacts"`
	InterestCount      int64   `json:"interest_count"`
	VerifiedCount      int64   `json:"verified_count"`
}

// ProductPopularityProvider is the service boundary used by HTTP consumers.
// A short-lived in-memory or Redis-backed decorator can be added later without
// changing controllers or the SQL-backed implementation.
type ProductPopularityProvider interface {
	CalculatePopularProducts(context.Context, ProductPopularityRequest) ([]ProductPopularity, error)
}

type productPopularityRepository interface {
	ListPopularProducts(context.Context, repository.ProductPopularityQuery) ([]repository.ProductPopularityRow, error)
}

type ProductPopularityService struct {
	repository   productPopularityRepository
	translations TranslationReader
	priceVisible bool
	cache        ProductPopularityCache
	cacheTTL     time.Duration
	now          func() time.Time
}

type ProductPopularityOptions struct {
	Translations TranslationReader
	PriceVisible bool
	Cache        ProductPopularityCache
	CacheTTL     time.Duration
	Now          func() time.Time
}

func NewProductPopularityService(
	popularityRepository productPopularityRepository,
	options ...ProductPopularityOptions,
) *ProductPopularityService {
	service := &ProductPopularityService{
		repository:   popularityRepository,
		priceVisible: true,
		cache:        NewMemoryProductPopularityCache(),
		cacheTTL:     DefaultProductPopularityCacheTTL,
		now:          time.Now,
	}
	if len(options) > 0 {
		service.translations = options[0].Translations
		service.priceVisible = options[0].PriceVisible
		if options[0].Cache != nil {
			service.cache = options[0].Cache
		}
		if options[0].CacheTTL > 0 {
			service.cacheTTL = options[0].CacheTTL
		}
		if options[0].Now != nil {
			service.now = options[0].Now
		}
	}
	return service
}

func (s *ProductPopularityService) CalculatePopularProducts(
	ctx context.Context,
	request ProductPopularityRequest,
) ([]ProductPopularity, error) {
	if s == nil || s.repository == nil {
		return nil, ErrServiceUnavailable
	}
	limit := request.Limit
	if limit == 0 {
		limit = DefaultProductPopularityLimit
	}
	if limit < 1 || limit > MaxProductPopularityLimit {
		return nil, ErrInvalidInput
	}
	if request.CategoryID != nil && *request.CategoryID == 0 {
		return nil, ErrInvalidInput
	}

	scope := repository.ProductPopularityScopeTenant
	switch {
	case request.Public:
		scope = repository.ProductPopularityScopePublic
		request.TenantID = nil
	case request.SuperAdmin:
		if request.TenantID == nil {
			scope = repository.ProductPopularityScopeAll
		}
	case request.TenantID == nil || *request.TenantID == 0:
		return nil, ErrForbidden
	}

	cacheKey := productPopularityCacheKey(request, scope, limit)
	if cached, found, cacheErr := s.cache.Get(ctx, cacheKey); cacheErr == nil && found {
		return cached, nil
	}

	rows, err := s.repository.ListPopularProducts(ctx, repository.ProductPopularityQuery{
		Scope:      scope,
		TenantID:   request.TenantID,
		CategoryID: request.CategoryID,
		Limit:      limit,
		ScoredAt:   s.now().UTC(),
	})
	if err != nil {
		return nil, err
	}

	translations, err := s.productTranslations(ctx, rows, locale.NormalizeLocale(request.Locale))
	if err != nil {
		return nil, err
	}
	result := make([]ProductPopularity, 0, len(rows))
	for _, row := range rows {
		name := translatedText(row.Name, translations[row.ProductID], TranslationFieldName)
		item := ProductPopularity{
			ProductID:          row.ProductID,
			TenantID:           row.TenantID,
			CategoryID:         row.CategoryID,
			PlatformCategoryID: row.PlatformCategoryID,
			Name:               name,
			MainImage:          row.MainImage,
			MinPrice:           row.MinPrice,
			MaxPrice:           row.MaxPrice,
			OriginalPrice:      row.OriginalPrice,
			HotScore:           row.HotScore,
			Views:              row.Views,
			Clicks:             row.Clicks,
			Contacts:           row.Contacts,
			InterestCount:      row.InterestCount,
			VerifiedCount:      row.VerifiedCount,
		}
		if !s.priceVisible {
			item.MinPrice = 0
			item.MaxPrice = 0
			item.OriginalPrice = 0
		}
		result = append(result, item)
	}
	_ = s.cache.Set(ctx, cacheKey, result, s.cacheTTL)
	return result, nil
}

func productPopularityCacheKey(
	request ProductPopularityRequest,
	scope repository.ProductPopularityScope,
	limit int,
) string {
	tenantID := uint(0)
	if request.TenantID != nil {
		tenantID = *request.TenantID
	}
	categoryID := uint(0)
	if request.CategoryID != nil {
		categoryID = *request.CategoryID
	}
	mode := "admin"
	if request.Public {
		mode = "storefront"
	} else if request.SuperAdmin {
		mode = "super"
	}
	return fmt.Sprintf(
		"product-popularity:v1:%s:%s:tenant=%d:category=%d:limit=%d:locale=%s",
		mode,
		scope,
		tenantID,
		categoryID,
		limit,
		locale.NormalizeLocale(request.Locale),
	)
}

func (s *ProductPopularityService) productTranslations(
	ctx context.Context,
	rows []repository.ProductPopularityRow,
	requestedLocale string,
) (map[uint]map[string]models.ContentTranslation, error) {
	if s.translations == nil || len(rows) == 0 {
		return map[uint]map[string]models.ContentTranslation{}, nil
	}
	productIDs := make([]uint, 0, len(rows))
	for _, row := range rows {
		productIDs = append(productIDs, row.ProductID)
	}
	return s.translations.GetTranslations(ctx, TranslationEntityProduct, productIDs, requestedLocale)
}
