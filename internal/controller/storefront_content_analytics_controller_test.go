package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"saas/internal/analytics"
	"saas/internal/middleware"
	"saas/internal/models"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type capturingContentAnalyticsReader struct {
	capturingAnalyticsReader
	endpoint      string
	trackingKey   string
	listRequest   service.ContentAnalyticsListRequest
	detailRequest service.ContentAnalyticsDetailRequest
	result        *service.ContentAnalyticsDetailResult
}

func (reader *capturingContentAnalyticsReader) ContentForSuper(
	_ context.Context,
	request service.ContentAnalyticsListRequest,
) (*service.ContentAnalyticsListResult, error) {
	reader.endpoint, reader.listRequest = "content", request
	if strings.Contains(request.Sort, ";") || strings.Contains(request.Order, " ") || strings.Contains(request.ContentType, ";") {
		return nil, service.ErrInvalidInput
	}
	return &service.ContentAnalyticsListResult{}, nil
}

func (reader *capturingContentAnalyticsReader) ContentDetailForSuper(
	_ context.Context,
	trackingKey string,
	request service.ContentAnalyticsDetailRequest,
) (*service.ContentAnalyticsDetailResult, error) {
	reader.endpoint, reader.trackingKey, reader.detailRequest = "detail", trackingKey, request
	if reader.result != nil {
		return reader.result, nil
	}
	return &service.ContentAnalyticsDetailResult{}, nil
}

func (reader *capturingContentAnalyticsReader) ContentExportForSuper(
	_ context.Context,
	request service.ContentAnalyticsListRequest,
) (*service.AnalyticsCSVExport, error) {
	reader.endpoint, reader.listRequest = "export", request
	return &service.AnalyticsCSVExport{Filename: "content-analytics-20260701-20260714.csv", Content: []byte{0xEF, 0xBB, 0xBF, 'x'}}, nil
}

func TestSuperContentAnalyticsRoutesParseFixedScopeQueriesAndDetailKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &capturingContentAnalyticsReader{}
	router := gin.New()
	NewStorefrontAnalyticsController(reader).RegisterSuperRoutes(router.Group("/api/v1/super"))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/api/v1/super/analytics/content?from=2026-07-01&to=2026-07-14&timezone=Asia%2FShanghai&page=2&page_size=10&search=Sale&content_type=promo_banner&currently_exists=false&sort=ctr&order=asc&min_impressions=12", nil))
	if response.Code != http.StatusOK || reader.endpoint != "content" {
		t.Fatalf("content list = status %d endpoint %q body=%s", response.Code, reader.endpoint, response.Body.String())
	}
	request := reader.listRequest
	if request.DateRange.Timezone != "Asia/Shanghai" || request.Page != 2 || request.PageSize != 10 || request.Search != "Sale" || request.ContentType != "promo_banner" || request.CurrentlyExists == nil || *request.CurrentlyExists || request.Sort != "ctr" || request.Order != "asc" || request.MinImpressions == nil || *request.MinImpressions != 12 {
		t.Fatalf("parsed content list request = %+v", request)
	}

	const key = "11111111-1111-4111-8111-111111111111"
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/api/v1/super/analytics/content/"+key+"?granularity=week", nil))
	if response.Code != http.StatusOK || reader.endpoint != "detail" || reader.trackingKey != key || reader.detailRequest.Granularity != "week" {
		t.Fatalf("content detail = status %d endpoint=%q key=%q request=%+v body=%s", response.Code, reader.endpoint, reader.trackingKey, reader.detailRequest, response.Body.String())
	}
}

func TestSuperContentAnalyticsExportPropagatesFiltersAndReturnsCSV(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &capturingContentAnalyticsReader{}
	router := gin.New()
	NewStorefrontAnalyticsController(reader).RegisterSuperRoutes(router.Group("/api/v1/super"))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/api/v1/super/analytics/content/export?from=2026-07-01&to=2026-07-14&search=Sale&content_type=promo_banner&currently_exists=false&sort=ctr&order=asc&min_impressions=12", nil))
	if response.Code != http.StatusOK || reader.endpoint != "export" || reader.listRequest.Search != "Sale" ||
		reader.listRequest.ContentType != "promo_banner" || reader.listRequest.CurrentlyExists == nil || *reader.listRequest.CurrentlyExists ||
		reader.listRequest.Sort != "ctr" || reader.listRequest.Order != "asc" || reader.listRequest.MinImpressions == nil || *reader.listRequest.MinImpressions != 12 {
		t.Fatalf("content export = status %d endpoint %q request %+v body=%q", response.Code, reader.endpoint, reader.listRequest, response.Body.Bytes())
	}
	if response.Header().Get("Content-Type") != "text/csv; charset=utf-8" ||
		!strings.Contains(response.Header().Get("Content-Disposition"), "content-analytics-20260701-20260714.csv") ||
		!strings.HasPrefix(response.Body.String(), string([]byte{0xEF, 0xBB, 0xBF})) {
		t.Fatalf("content export headers/body = %#v/% x", response.Header(), response.Body.Bytes())
	}
}

func TestContentAnalyticsControllerRejectsTenantOverrideMalformedValuesAndUnsafeInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &capturingContentAnalyticsReader{}
	router := gin.New()
	NewStorefrontAnalyticsController(reader).RegisterSuperRoutes(router.Group("/api/v1/super"))
	for _, path := range []string{
		"/api/v1/super/analytics/content?tenant_id=7",
		"/api/v1/super/analytics/content?page=0",
		"/api/v1/super/analytics/content?currently_exists=1",
		"/api/v1/super/analytics/content?sort=clicks%3Bdrop",
		"/api/v1/super/analytics/content?order=desc%20nulls%20first",
		"/api/v1/super/analytics/content?content_type=hero%3Bdrop",
		"/api/v1/super/analytics/content/not-a-uuid",
		"/api/v1/super/analytics/content/11111111-1111-4111-8111-111111111111?tenant_id=7",
	} {
		reader.endpoint = ""
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}

func TestTenantAdminHasNoContentAnalyticsRouteIncludingImpersonatedContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &capturingContentAnalyticsReader{}
	router := gin.New()
	tenantID := uint(7)
	admin := router.Group("/api/v1")
	admin.Use(func(c *gin.Context) {
		c.Set(middleware.ContextTenantIDKey, &tenantID)
		// Impersonation issues an ordinary tenant-admin token, so the resulting
		// authorization context is intentionally indistinguishable here.
		c.Set(middleware.ContextRoleKey, models.UserRoleTenantAdmin)
		c.Next()
	})
	NewStorefrontAnalyticsController(reader).RegisterAdminRoutes(admin)

	for _, path := range []string{
		"/api/v1/admin/analytics/content",
		"/api/v1/admin/analytics/content/export",
		"/api/v1/admin/analytics/content/11111111-1111-4111-8111-111111111111",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound || reader.endpoint != "" {
			t.Fatalf("tenant content route %s = status %d endpoint %q", path, response.Code, reader.endpoint)
		}
	}
}

func TestContentAnalyticsDetailResponseContainsAggregatesWithoutVisitorSessionOrReferrer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const key = "11111111-1111-4111-8111-111111111111"
	reader := &capturingContentAnalyticsReader{result: &service.ContentAnalyticsDetailResult{
		Content: service.MarketingContentView{MarketingContentMetadata: analytics.MarketingContentMetadata{
			TrackingKey: key, ContentType: analytics.ContentTypeHeroSlide, Title: "Slide", TargetType: analytics.ContentTargetInternal,
		}, CurrentlyExists: true},
		Summary: service.ContentAnalyticsSummary{ContentCounts: analytics.ContentCounts{Impressions: 3, Clicks: 1, UniqueVisitors: 2}, CTR: 1.0 / 3.0},
		Pages:   []analytics.ContentPageStat{{PagePath: "/en", Impressions: 3, Clicks: 1}},
	}}
	router := gin.New()
	NewStorefrontAnalyticsController(reader).RegisterSuperRoutes(router.Group("/api/v1/super"))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/super/analytics/content/"+key, nil))
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"tracking_key":"`+key+`"`) {
		t.Fatalf("detail response = %d %s", response.Code, body)
	}
	for _, forbidden := range []string{"visitor_id", "session_id", "referrer"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %s: %s", forbidden, body)
		}
	}
}
