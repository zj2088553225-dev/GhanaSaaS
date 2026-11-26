package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"saas/internal/analytics"
	"saas/internal/middleware"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type capturingAnalyticsReader struct {
	superTenantID *uint
	adminTenantID uint
}

type capturingStatusAnalyticsReader struct {
	capturingAnalyticsReader
	statusCalls int
}

func (reader *capturingStatusAnalyticsReader) StatusForSuper(_ context.Context) (*analytics.DataStatus, error) {
	reader.statusCalls++
	return &analytics.DataStatus{TotalEvents: 12, HumanEvents: 10, BotEvents: 2}, nil
}

func (reader *capturingAnalyticsReader) OverviewForSuper(_ context.Context, tenantID *uint, _ service.AnalyticsOverviewRequest) (*service.AnalyticsOverviewResult, error) {
	reader.superTenantID = tenantID
	return &service.AnalyticsOverviewResult{}, nil
}

func (reader *capturingAnalyticsReader) OverviewForTenant(_ context.Context, tenantID uint, _ service.AnalyticsOverviewRequest) (*service.AnalyticsOverviewResult, error) {
	reader.adminTenantID = tenantID
	return &service.AnalyticsOverviewResult{}, nil
}

func (reader *capturingAnalyticsReader) TrendsForSuper(_ context.Context, tenantID *uint, _ service.AnalyticsTrendsRequest) (*service.AnalyticsTrendsResult, error) {
	reader.superTenantID = tenantID
	return &service.AnalyticsTrendsResult{}, nil
}

func (reader *capturingAnalyticsReader) TrendsForTenant(_ context.Context, tenantID uint, _ service.AnalyticsTrendsRequest) (*service.AnalyticsTrendsResult, error) {
	reader.adminTenantID = tenantID
	return &service.AnalyticsTrendsResult{}, nil
}

func (reader *capturingAnalyticsReader) SourcesForSuper(_ context.Context, tenantID *uint, _ service.AnalyticsSourcesRequest) (*service.AnalyticsSourcesResult, error) {
	reader.superTenantID = tenantID
	return &service.AnalyticsSourcesResult{}, nil
}

func (reader *capturingAnalyticsReader) SourcesForTenant(_ context.Context, tenantID uint, _ service.AnalyticsSourcesRequest) (*service.AnalyticsSourcesResult, error) {
	reader.adminTenantID = tenantID
	return &service.AnalyticsSourcesResult{}, nil
}

func TestAdminAnalyticsIgnoresTenantQueryOverrideAndUsesAuthScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &capturingAnalyticsReader{}
	controller := NewStorefrontAnalyticsController(reader)
	router := gin.New()
	tenantID := uint(7)
	group := router.Group("/api/v1")
	group.Use(func(c *gin.Context) {
		c.Set(middleware.ContextTenantIDKey, &tenantID)
		c.Next()
	})
	controller.RegisterAdminRoutes(group)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/analytics/overview?from=2026-07-01&to=2026-07-02&tenant_id=999", nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	if reader.adminTenantID != tenantID {
		t.Fatalf("admin tenant = %d, want JWT tenant %d", reader.adminTenantID, tenantID)
	}
}

func TestSuperAnalyticsParsesOptionalTenantAndRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &capturingAnalyticsReader{}
	controller := NewStorefrontAnalyticsController(reader)
	router := gin.New()
	controller.RegisterSuperRoutes(router.Group("/api/v1/super"))

	for _, path := range []string{
		"/api/v1/super/analytics/overview?tenant_id=9",
		"/api/v1/super/analytics/trends?tenant_id=9",
		"/api/v1/super/analytics/sources?tenant_id=9",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body=%s", path, response.Code, response.Body.String())
		}
		if reader.superTenantID == nil || *reader.superTenantID != 9 {
			t.Fatalf("%s super tenant = %v, want 9", path, reader.superTenantID)
		}
	}
}

func TestAnalyticsStatusIsSuperOnlyAndRejectsTenantScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &capturingStatusAnalyticsReader{}
	superRouter := gin.New()
	NewStorefrontAnalyticsController(reader).RegisterSuperRoutes(superRouter.Group("/api/v1/super"))
	response := httptest.NewRecorder()
	superRouter.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/super/analytics/status", nil))
	if response.Code != http.StatusOK || reader.statusCalls != 1 || !containsJSONNull(response.Body.String(), `"total_events":12`) {
		t.Fatalf("super status = %d calls=%d body=%s", response.Code, reader.statusCalls, response.Body.String())
	}
	response = httptest.NewRecorder()
	superRouter.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/super/analytics/status?tenant_id=7", nil))
	if response.Code != http.StatusBadRequest || reader.statusCalls != 1 {
		t.Fatalf("tenant-scoped status = %d calls=%d body=%s", response.Code, reader.statusCalls, response.Body.String())
	}

	adminRouter := gin.New()
	NewStorefrontAnalyticsController(reader).RegisterAdminRoutes(adminRouter.Group("/api/v1"))
	response = httptest.NewRecorder()
	adminRouter.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/analytics/status", nil))
	if response.Code != http.StatusNotFound || reader.statusCalls != 1 {
		t.Fatalf("admin status route = %d calls=%d", response.Code, reader.statusCalls)
	}
}

func TestAnalyticsControllerRejectsInvalidQueryValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &capturingAnalyticsReader{}
	router := gin.New()
	NewStorefrontAnalyticsController(reader).RegisterSuperRoutes(router.Group("/api/v1/super"))

	for _, path := range []string{
		"/api/v1/super/analytics/overview?tenant_id=invalid",
		"/api/v1/super/analytics/overview?compare=1",
		"/api/v1/super/analytics/sources?limit=0",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400, body=%s", path, response.Code, response.Body.String())
		}
	}
}

func TestAnalyticsMetricJSONKeepsUndefinedGrowthAsNull(t *testing.T) {
	metric := service.AnalyticsMetric{Value: 1, PreviousValue: floatTestPointer(0), ChangeRate: nil}
	result := service.AnalyticsOverviewResult{
		Range:   analytics.DateRange{From: "2026-07-01", To: "2026-07-01", Timezone: analytics.DefaultTimezone},
		Metrics: service.AnalyticsOverviewMetrics{PageViews: metric},
	}
	reader := &staticOverviewAnalyticsReader{result: &result}
	router := gin.New()
	NewStorefrontAnalyticsController(reader).RegisterSuperRoutes(router.Group("/api/v1/super"))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/super/analytics/overview", nil))
	if response.Code != http.StatusOK || !containsJSONNull(response.Body.String(), `"change_rate":null`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

type staticOverviewAnalyticsReader struct {
	capturingAnalyticsReader
	result *service.AnalyticsOverviewResult
}

func (reader *staticOverviewAnalyticsReader) OverviewForSuper(_ context.Context, _ *uint, _ service.AnalyticsOverviewRequest) (*service.AnalyticsOverviewResult, error) {
	return reader.result, nil
}

func floatTestPointer(value float64) *float64 {
	return &value
}

func containsJSONNull(value string, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
