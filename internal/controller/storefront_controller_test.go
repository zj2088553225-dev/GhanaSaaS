package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"saas/internal/models"
	"saas/internal/repository"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type sitemapControllerTestRepo struct{}

func (s sitemapControllerTestRepo) ListActiveCategories(context.Context) ([]models.Category, error) {
	return nil, nil
}

func (s sitemapControllerTestRepo) ListProducts(context.Context, repository.ProductQuery) ([]models.SPU, int64, error) {
	return nil, 0, nil
}

func (s sitemapControllerTestRepo) FindProductByID(context.Context, uint) (*models.SPU, error) {
	return nil, nil
}

func (s sitemapControllerTestRepo) FindPublicShopProfileByTenantID(context.Context, uint) (*models.ShopProfile, error) {
	return nil, nil
}

func (s sitemapControllerTestRepo) FindPlatformCategoryByID(context.Context, uint) (*models.PlatformCategory, error) {
	return nil, nil
}

func (s sitemapControllerTestRepo) ListPlatformCategories(context.Context, bool) ([]models.PlatformCategory, error) {
	return nil, nil
}

func (s sitemapControllerTestRepo) ListPlatformCategoryIDsByRoot(context.Context, uint) ([]uint, error) {
	return nil, nil
}

func (s sitemapControllerTestRepo) FindPublicShopProfileBySlug(context.Context, string) (*models.ShopProfile, error) {
	return nil, nil
}

func (s sitemapControllerTestRepo) ListPublicShopProfiles(context.Context, repository.PublicShopProfileQuery) ([]models.ShopProfile, int64, error) {
	return nil, 0, nil
}

func (s sitemapControllerTestRepo) ListSitemapShops(context.Context) ([]repository.SitemapShopURL, error) {
	return []repository.SitemapShopURL{}, nil
}

func (s sitemapControllerTestRepo) ListSitemapProducts(context.Context) ([]repository.SitemapProductURL, error) {
	return []repository.SitemapProductURL{}, nil
}

func TestParseProductSortAcceptsStorefrontSortOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, sort := range []string{"", "latest", "hot", "price_asc", "price_desc"} {
		t.Run(sort, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest("GET", "/api/v1/storefront/products?sort="+sort, nil)

			got, ok := parseProductSort(context)
			if !ok {
				t.Fatalf("parseProductSort(%q) rejected a supported sort", sort)
			}
			if got != sort {
				t.Fatalf("parseProductSort(%q) = %q", sort, got)
			}
		})
	}
}

func TestParseProductSortRejectsUnsupportedValue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("GET", "/api/v1/storefront/products?sort=unknown", nil)

	if _, ok := parseProductSort(context); ok {
		t.Fatal("parseProductSort accepted an unsupported sort")
	}
	if recorder.Code != 400 {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestParseProductSourceAcceptsSupportedValues(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, source := range []string{"", "all", "t6", "merchant"} {
		t.Run(source, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest("GET", "/api/v1/storefront/products?source="+source, nil)

			got, ok := parseProductSource(context)
			if !ok {
				t.Fatalf("parseProductSource(%q) rejected a supported source", source)
			}
			if got != source {
				t.Fatalf("parseProductSource(%q) = %q", source, got)
			}
		})
	}
}

func TestParseProductSourceRejectsUnsupportedValue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("GET", "/api/v1/storefront/products?source=self_operated", nil)

	if _, ok := parseProductSource(context); ok {
		t.Fatal("parseProductSource accepted an unsupported source")
	}
	if recorder.Code != 400 {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestParseLocaleQueryNormalizesStorefrontLocale(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		url  string
		want string
	}{
		{url: "/api/v1/storefront/products", want: "en"},
		{url: "/api/v1/storefront/products?locale=zh", want: "zh"},
		{url: "/api/v1/storefront/products?locale=bad", want: "en"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest("GET", tt.url, nil)

			if got := parseLocaleQuery(context); got != tt.want {
				t.Fatalf("parseLocaleQuery(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestParseStorefrontProductQueryIncludesNormalizedLocale(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("GET", "/api/v1/storefront/products?locale=zh&page=2", nil)

	query, ok := parseStorefrontProductQuery(context)
	if !ok {
		t.Fatal("parseStorefrontProductQuery rejected supported locale")
	}
	if query.Locale != "zh" {
		t.Fatalf("query.Locale = %q, want zh", query.Locale)
	}
}

func TestStorefrontSitemapReturnsXMLContentType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewStorefrontController(service.NewStorefrontService(sitemapControllerTestRepo{})).RegisterRoutes(router.Group("/api/v1"))
	router.GET("/sitemap.xml", NewStorefrontController(service.NewStorefrontService(sitemapControllerTestRepo{})).Sitemap)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/xml; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want application/xml; charset=utf-8", got)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "<urlset") || !strings.Contains(body, "/merchant/join") || strings.Contains(body, "/admin") {
		t.Fatalf("unexpected sitemap body: %s", body)
	}
}
