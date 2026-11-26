package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"saas/internal/repository"
)

type recordingProductPopularityRepository struct {
	mu      sync.Mutex
	queries []repository.ProductPopularityQuery
	rows    []repository.ProductPopularityRow
	err     error
}

func (repositoryMock *recordingProductPopularityRepository) ListPopularProducts(
	_ context.Context,
	query repository.ProductPopularityQuery,
) ([]repository.ProductPopularityRow, error) {
	repositoryMock.mu.Lock()
	defer repositoryMock.mu.Unlock()
	repositoryMock.queries = append(repositoryMock.queries, query)
	if repositoryMock.err != nil {
		return nil, repositoryMock.err
	}
	return append([]repository.ProductPopularityRow(nil), repositoryMock.rows...), nil
}

func (repositoryMock *recordingProductPopularityRepository) lastQuery() repository.ProductPopularityQuery {
	repositoryMock.mu.Lock()
	defer repositoryMock.mu.Unlock()
	return repositoryMock.queries[len(repositoryMock.queries)-1]
}

func (repositoryMock *recordingProductPopularityRepository) queryCount() int {
	repositoryMock.mu.Lock()
	defer repositoryMock.mu.Unlock()
	return len(repositoryMock.queries)
}

func TestProductPopularityServiceScopesLimitsAndEmptyResults(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	repositoryMock := &recordingProductPopularityRepository{rows: []repository.ProductPopularityRow{}}
	popularityService := NewProductPopularityService(repositoryMock, ProductPopularityOptions{
		PriceVisible: true,
		Now:          func() time.Time { return now },
	})

	result, err := popularityService.CalculatePopularProducts(t.Context(), ProductPopularityRequest{Public: true})
	if err != nil {
		t.Fatalf("public popularity: %v", err)
	}
	if result == nil || len(result) != 0 {
		t.Fatalf("empty popularity result = %#v, want non-nil empty slice", result)
	}
	publicQuery := repositoryMock.lastQuery()
	if publicQuery.Scope != repository.ProductPopularityScopePublic ||
		publicQuery.Limit != DefaultProductPopularityLimit ||
		!publicQuery.ScoredAt.Equal(now) {
		t.Fatalf("public query = %+v", publicQuery)
	}

	tenantID := uint(42)
	categoryID := uint(7)
	if _, err := popularityService.CalculatePopularProducts(t.Context(), ProductPopularityRequest{
		TenantID: &tenantID, CategoryID: &categoryID, Limit: 100,
	}); err != nil {
		t.Fatalf("tenant popularity: %v", err)
	}
	tenantQuery := repositoryMock.lastQuery()
	if tenantQuery.Scope != repository.ProductPopularityScopeTenant ||
		tenantQuery.TenantID == nil || *tenantQuery.TenantID != tenantID ||
		tenantQuery.CategoryID == nil || *tenantQuery.CategoryID != categoryID ||
		tenantQuery.Limit != MaxProductPopularityLimit {
		t.Fatalf("tenant query = %+v", tenantQuery)
	}

	if _, err := popularityService.CalculatePopularProducts(t.Context(), ProductPopularityRequest{
		SuperAdmin: true, Limit: 1,
	}); err != nil {
		t.Fatalf("super popularity: %v", err)
	}
	if query := repositoryMock.lastQuery(); query.Scope != repository.ProductPopularityScopeAll {
		t.Fatalf("super query scope = %q, want all", query.Scope)
	}

	for _, request := range []ProductPopularityRequest{
		{Limit: -1, Public: true},
		{Limit: 101, Public: true},
		{},
		{CategoryID: new(uint), Public: true},
	} {
		_, err := popularityService.CalculatePopularProducts(t.Context(), request)
		if !errors.Is(err, ErrInvalidInput) && !errors.Is(err, ErrForbidden) {
			t.Fatalf("invalid request %+v error = %v", request, err)
		}
	}
}

func TestProductPopularityServiceHidesPricesWhenStorefrontPricesAreDisabled(t *testing.T) {
	repositoryMock := &recordingProductPopularityRepository{rows: []repository.ProductPopularityRow{{
		ProductID: 1, TenantID: 2, Name: "Phone", MinPrice: 100, MaxPrice: 200, OriginalPrice: 250, HotScore: 9,
	}}}
	popularityService := NewProductPopularityService(repositoryMock, ProductPopularityOptions{PriceVisible: false})
	result, err := popularityService.CalculatePopularProducts(t.Context(), ProductPopularityRequest{Public: true})
	if err != nil || len(result) != 1 {
		t.Fatalf("hidden-price popularity = %+v, %v", result, err)
	}
	if result[0].MinPrice != 0 || result[0].MaxPrice != 0 || result[0].OriginalPrice != 0 || result[0].HotScore != 9 {
		t.Fatalf("hidden-price result = %+v", result[0])
	}
}

func TestProductPopularityServiceCacheSeparatesScopeCategoryLimitAndMode(t *testing.T) {
	categoryID := uint(7)
	tenantID := uint(9)
	repositoryMock := &recordingProductPopularityRepository{rows: []repository.ProductPopularityRow{{
		ProductID: 1, TenantID: tenantID, Name: "Phone", HotScore: 9,
	}}}
	cache := NewMemoryProductPopularityCache()
	popularityService := NewProductPopularityService(repositoryMock, ProductPopularityOptions{
		PriceVisible: true,
		Cache:        cache,
		CacheTTL:     2 * time.Minute,
	})

	publicRequest := ProductPopularityRequest{Public: true, Limit: 10, Locale: "en"}
	first, err := popularityService.CalculatePopularProducts(t.Context(), publicRequest)
	if err != nil {
		t.Fatal(err)
	}
	first[0].Name = "mutated by caller"
	second, err := popularityService.CalculatePopularProducts(t.Context(), publicRequest)
	if err != nil {
		t.Fatal(err)
	}
	if repositoryMock.queryCount() != 1 || second[0].Name != "Phone" {
		t.Fatalf("public cache count/result = %d/%+v", repositoryMock.queryCount(), second)
	}

	requests := []ProductPopularityRequest{
		{Public: true, Limit: 11, Locale: "en"},
		{Public: true, Limit: 10, Locale: "zh"},
		{Public: true, Limit: 10, CategoryID: &categoryID, Locale: "en"},
		{TenantID: &tenantID, Limit: 10, Locale: "en"},
		{TenantID: &tenantID, SuperAdmin: true, Limit: 10, Locale: "en"},
	}
	for _, request := range requests {
		if _, err := popularityService.CalculatePopularProducts(t.Context(), request); err != nil {
			t.Fatal(err)
		}
	}
	if repositoryMock.queryCount() != 1+len(requests) {
		t.Fatalf("cache keys collapsed distinct request modes: queries=%d", repositoryMock.queryCount())
	}
	stats := cache.Stats()
	if stats.Hits != 1 || stats.Misses != uint64(1+len(requests)) {
		t.Fatalf("cache stats = %+v", stats)
	}
}

type failingProductPopularityCache struct{}

func (failingProductPopularityCache) Get(context.Context, string) ([]ProductPopularity, bool, error) {
	return nil, false, errors.New("cache unavailable")
}

func (failingProductPopularityCache) Set(context.Context, string, []ProductPopularity, time.Duration) error {
	return errors.New("cache unavailable")
}

func TestProductPopularityServiceCacheFailureDoesNotBlockDatabaseQuery(t *testing.T) {
	repositoryMock := &recordingProductPopularityRepository{rows: []repository.ProductPopularityRow{}}
	popularityService := NewProductPopularityService(repositoryMock, ProductPopularityOptions{
		Cache: failingProductPopularityCache{},
	})
	result, err := popularityService.CalculatePopularProducts(t.Context(), ProductPopularityRequest{Public: true})
	if err != nil || result == nil || repositoryMock.queryCount() != 1 {
		t.Fatalf("cache failure result/error/queries = %#v/%v/%d", result, err, repositoryMock.queryCount())
	}
}
