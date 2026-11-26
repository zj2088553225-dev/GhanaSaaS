package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"saas/internal/middleware"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type capturingProductAnalyticsReader struct {
	capturingAnalyticsReader
	endpoint      string
	productID     uint
	listRequest   service.ProductAnalyticsListRequest
	topRequest    service.TopProductAnalyticsRequest
	lowRequest    service.LowCTRProductAnalyticsRequest
	detailRequest service.ProductAnalyticsDetailRequest
}

func (reader *capturingProductAnalyticsReader) ProductsForSuper(
	_ context.Context,
	tenantID *uint,
	request service.ProductAnalyticsListRequest,
) (*service.ProductAnalyticsListResult, error) {
	reader.endpoint, reader.superTenantID, reader.listRequest = "products", tenantID, request
	if strings.Contains(request.Sort, ";") || strings.Contains(request.Order, " ") {
		return nil, service.ErrInvalidInput
	}
	return &service.ProductAnalyticsListResult{}, nil
}

func (reader *capturingProductAnalyticsReader) ProductsForTenant(
	_ context.Context,
	tenantID uint,
	request service.ProductAnalyticsListRequest,
) (*service.ProductAnalyticsListResult, error) {
	reader.endpoint, reader.adminTenantID, reader.listRequest = "products", tenantID, request
	return &service.ProductAnalyticsListResult{}, nil
}

func (reader *capturingProductAnalyticsReader) TopProductsForSuper(
	_ context.Context,
	tenantID *uint,
	request service.TopProductAnalyticsRequest,
) (*service.TopProductAnalyticsResult, error) {
	reader.endpoint, reader.superTenantID, reader.topRequest = "top", tenantID, request
	return &service.TopProductAnalyticsResult{}, nil
}

func (reader *capturingProductAnalyticsReader) TopProductsForTenant(
	_ context.Context,
	tenantID uint,
	request service.TopProductAnalyticsRequest,
) (*service.TopProductAnalyticsResult, error) {
	reader.endpoint, reader.adminTenantID, reader.topRequest = "top", tenantID, request
	return &service.TopProductAnalyticsResult{}, nil
}

func (reader *capturingProductAnalyticsReader) LowCTRProductsForSuper(
	_ context.Context,
	tenantID *uint,
	request service.LowCTRProductAnalyticsRequest,
) (*service.LowCTRProductAnalyticsResult, error) {
	reader.endpoint, reader.superTenantID, reader.lowRequest = "low-ctr", tenantID, request
	return &service.LowCTRProductAnalyticsResult{}, nil
}

func (reader *capturingProductAnalyticsReader) LowCTRProductsForTenant(
	_ context.Context,
	tenantID uint,
	request service.LowCTRProductAnalyticsRequest,
) (*service.LowCTRProductAnalyticsResult, error) {
	reader.endpoint, reader.adminTenantID, reader.lowRequest = "low-ctr", tenantID, request
	return &service.LowCTRProductAnalyticsResult{}, nil
}

func (reader *capturingProductAnalyticsReader) ProductDetailForSuper(
	_ context.Context,
	tenantID *uint,
	productID uint,
	request service.ProductAnalyticsDetailRequest,
) (*service.ProductAnalyticsDetailResult, error) {
	reader.endpoint, reader.superTenantID, reader.productID, reader.detailRequest = "detail", tenantID, productID, request
	return &service.ProductAnalyticsDetailResult{}, nil
}

func (reader *capturingProductAnalyticsReader) ProductDetailForTenant(
	_ context.Context,
	tenantID uint,
	productID uint,
	request service.ProductAnalyticsDetailRequest,
) (*service.ProductAnalyticsDetailResult, error) {
	reader.endpoint, reader.adminTenantID, reader.productID, reader.detailRequest = "detail", tenantID, productID, request
	return &service.ProductAnalyticsDetailResult{}, nil
}

func (reader *capturingProductAnalyticsReader) ProductExportForSuper(
	_ context.Context,
	tenantID *uint,
	request service.ProductAnalyticsListRequest,
) (*service.AnalyticsCSVExport, error) {
	reader.endpoint, reader.superTenantID, reader.listRequest = "export", tenantID, request
	return &service.AnalyticsCSVExport{Filename: "product-analytics-20260701-20260714.csv", Content: []byte{0xEF, 0xBB, 0xBF, 'x'}}, nil
}

func (reader *capturingProductAnalyticsReader) ProductExportForTenant(
	_ context.Context,
	tenantID uint,
	request service.ProductAnalyticsListRequest,
) (*service.AnalyticsCSVExport, error) {
	reader.endpoint, reader.adminTenantID, reader.listRequest = "export", tenantID, request
	return &service.AnalyticsCSVExport{Filename: "product-analytics-20260701-20260714.csv", Content: []byte{0xEF, 0xBB, 0xBF, 'x'}}, nil
}

func TestProductAnalyticsStaticRoutesAreRegisteredBeforeProductID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &capturingProductAnalyticsReader{}
	router := gin.New()
	NewStorefrontAnalyticsController(reader).RegisterSuperRoutes(router.Group("/api/v1/super"))

	for _, test := range []struct {
		path     string
		endpoint string
	}{
		{"/api/v1/super/analytics/products/export?tenant_id=9&search=lamp&status=off_shelf&sort=ctr&order=asc", "export"},
		{"/api/v1/super/analytics/products/top?tenant_id=9", "top"},
		{"/api/v1/super/analytics/products/low-ctr?tenant_id=9", "low-ctr"},
		{"/api/v1/super/analytics/products/42?tenant_id=9", "detail"},
	} {
		reader.endpoint = ""
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != http.StatusOK || reader.endpoint != test.endpoint {
			t.Fatalf("%s = status %d endpoint %q body=%s", test.path, response.Code, reader.endpoint, response.Body.String())
		}
		if reader.superTenantID == nil || *reader.superTenantID != 9 {
			t.Fatalf("%s tenant scope = %v", test.path, reader.superTenantID)
		}
	}
	if reader.productID != 42 {
		t.Fatalf("detail product ID = %d", reader.productID)
	}
}

func TestAdminProductAnalyticsExportUsesJWTenantAndReturnsCSV(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &capturingProductAnalyticsReader{}
	router := gin.New()
	tenantID := uint(7)
	group := router.Group("/api/v1")
	group.Use(func(c *gin.Context) {
		c.Set(middleware.ContextTenantIDKey, &tenantID)
		c.Next()
	})
	NewStorefrontAnalyticsController(reader).RegisterAdminRoutes(group)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/api/v1/admin/analytics/products/export?tenant_id=999&search=lamp&status=off_shelf&sort=ctr&order=asc", nil))
	if response.Code != http.StatusOK || reader.endpoint != "export" || reader.adminTenantID != 7 ||
		reader.listRequest.Search != "lamp" || reader.listRequest.Status != "off_shelf" ||
		reader.listRequest.Sort != "ctr" || reader.listRequest.Order != "asc" {
		t.Fatalf("admin export = status %d endpoint %q tenant %d request %+v body=%q", response.Code, reader.endpoint, reader.adminTenantID, reader.listRequest, response.Body.Bytes())
	}
	if response.Header().Get("Content-Type") != "text/csv; charset=utf-8" ||
		!strings.Contains(response.Header().Get("Content-Disposition"), "product-analytics-20260701-20260714.csv") ||
		!strings.HasPrefix(response.Body.String(), string([]byte{0xEF, 0xBB, 0xBF})) {
		t.Fatalf("export headers/body = %#v/% x", response.Header(), response.Body.Bytes())
	}
}

func TestAdminProductAnalyticsUsesJWTenantAndIgnoresQueryOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &capturingProductAnalyticsReader{}
	router := gin.New()
	tenantID := uint(7)
	group := router.Group("/api/v1")
	group.Use(func(c *gin.Context) {
		c.Set(middleware.ContextTenantIDKey, &tenantID)
		c.Next()
	})
	NewStorefrontAnalyticsController(reader).RegisterAdminRoutes(group)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/analytics/products/42?tenant_id=999&granularity=day", nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || reader.endpoint != "detail" || reader.adminTenantID != 7 || reader.productID != 42 {
		t.Fatalf("admin detail = status %d endpoint %q tenant %d product %d body=%s", response.Code, reader.endpoint, reader.adminTenantID, reader.productID, response.Body.String())
	}
}

func TestProductAnalyticsControllerRejectsMalformedNumericAndUnsafeQueryValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &capturingProductAnalyticsReader{}
	router := gin.New()
	NewStorefrontAnalyticsController(reader).RegisterSuperRoutes(router.Group("/api/v1/super"))

	for _, path := range []string{
		"/api/v1/super/analytics/products?page=0",
		"/api/v1/super/analytics/products?sort=clicks%3Bdrop",
		"/api/v1/super/analytics/products?order=desc%20nulls%20first",
		"/api/v1/super/analytics/products/top?min_impressions=-1",
		"/api/v1/super/analytics/products/invalid",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400; body=%s", path, response.Code, response.Body.String())
		}
	}
}
