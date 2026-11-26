package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"saas/internal/middleware"
	"saas/internal/models"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type productPopularityControllerProvider struct {
	requests []service.ProductPopularityRequest
	result   []service.ProductPopularity
}

func (provider *productPopularityControllerProvider) CalculatePopularProducts(
	_ context.Context,
	request service.ProductPopularityRequest,
) ([]service.ProductPopularity, error) {
	provider.requests = append(provider.requests, request)
	return provider.result, nil
}

func TestStorefrontPopularProductsResponseAndValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &productPopularityControllerProvider{result: []service.ProductPopularity{{
		ProductID: 1, TenantID: 99, Name: "Phone", HotScore: 42,
		Views: 5, Clicks: 3, Contacts: 2, InterestCount: 4, VerifiedCount: 1,
	}}}
	controller := (&StorefrontController{}).WithPopularity(provider)
	router := gin.New()
	router.GET("/api/v1/storefront/products/popular", controller.PopularProducts)

	responseRecorder := httptest.NewRecorder()
	router.ServeHTTP(responseRecorder, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/storefront/products/popular?limit=10&category_id=7&locale=zh",
		nil,
	))
	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("popular products status/body = %d/%s", responseRecorder.Code, responseRecorder.Body.String())
	}
	var payload struct {
		Data []struct {
			ProductID uint `json:"product_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(responseRecorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode popularity response: %v", err)
	}
	if len(payload.Data) != 1 || payload.Data[0].ProductID != 1 {
		t.Fatalf("popular response = %+v", payload.Data)
	}
	for _, internalField := range []string{
		`"tenant_id"`, `"hot_score"`, `"views"`, `"clicks"`, `"contacts"`,
		`"reservations"`, `"interest_count"`, `"verified_count"`, `"metrics"`,
	} {
		if strings.Contains(responseRecorder.Body.String(), internalField) {
			t.Fatalf("public popularity response exposed %s: %s", internalField, responseRecorder.Body.String())
		}
	}
	if len(provider.requests) != 1 || !provider.requests[0].Public ||
		provider.requests[0].CategoryID == nil || *provider.requests[0].CategoryID != 7 ||
		provider.requests[0].Locale != "zh" {
		t.Fatalf("public popularity request = %+v", provider.requests)
	}

	for _, path := range []string{
		"/api/v1/storefront/products/popular?limit=101",
		"/api/v1/storefront/products/popular?category_id=invalid",
	} {
		invalidRecorder := httptest.NewRecorder()
		router.ServeHTTP(invalidRecorder, httptest.NewRequest(http.MethodGet, path, nil))
		if invalidRecorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid popularity request %q status = %d", path, invalidRecorder.Code)
		}
	}
}

func TestAnalyticsPopularityTenantScopeAndRoleProtection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	provider := &productPopularityControllerProvider{result: []service.ProductPopularity{}}
	controller := (&StorefrontAnalyticsController{}).WithPopularity(provider)

	adminRouter := gin.New()
	adminGroup := adminRouter.Group("/api/v1")
	adminGroup.Use(func(c *gin.Context) {
		tenantID := uint(23)
		c.Set(middleware.ContextTenantIDKey, &tenantID)
		c.Set(middleware.ContextRoleKey, models.UserRoleTenantAdmin)
		c.Next()
	})
	adminGroup.Use(middleware.RequireRole(models.UserRoleSuperAdmin, models.UserRoleTenantAdmin))
	adminGroup.GET("/admin/analytics/popularity", controller.AdminPopularity)

	adminRecorder := httptest.NewRecorder()
	adminRouter.ServeHTTP(adminRecorder, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/admin/analytics/popularity?tenant_id=999&limit=5",
		nil,
	))
	if adminRecorder.Code != http.StatusOK {
		t.Fatalf("tenant popularity status/body = %d/%s", adminRecorder.Code, adminRecorder.Body.String())
	}
	if len(provider.requests) != 1 || provider.requests[0].TenantID == nil ||
		*provider.requests[0].TenantID != 23 || provider.requests[0].Limit != 5 {
		t.Fatalf("tenant popularity request = %+v", provider.requests)
	}

	customerRouter := gin.New()
	customerGroup := customerRouter.Group("/api/v1")
	customerGroup.Use(func(c *gin.Context) {
		c.Set(middleware.ContextRoleKey, models.UserRoleCustomer)
		c.Next()
	})
	customerGroup.Use(middleware.RequireRole(models.UserRoleSuperAdmin, models.UserRoleTenantAdmin))
	customerGroup.GET("/admin/analytics/popularity", controller.AdminPopularity)

	customerRecorder := httptest.NewRecorder()
	customerRouter.ServeHTTP(customerRecorder, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/admin/analytics/popularity",
		nil,
	))
	if customerRecorder.Code != http.StatusForbidden {
		t.Fatalf("customer popularity status = %d, want forbidden", customerRecorder.Code)
	}
}
