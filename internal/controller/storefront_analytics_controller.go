package controller

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"saas/internal/analytics"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type storefrontAnalyticsReader interface {
	OverviewForSuper(context.Context, *uint, service.AnalyticsOverviewRequest) (*service.AnalyticsOverviewResult, error)
	OverviewForTenant(context.Context, uint, service.AnalyticsOverviewRequest) (*service.AnalyticsOverviewResult, error)
	TrendsForSuper(context.Context, *uint, service.AnalyticsTrendsRequest) (*service.AnalyticsTrendsResult, error)
	TrendsForTenant(context.Context, uint, service.AnalyticsTrendsRequest) (*service.AnalyticsTrendsResult, error)
	SourcesForSuper(context.Context, *uint, service.AnalyticsSourcesRequest) (*service.AnalyticsSourcesResult, error)
	SourcesForTenant(context.Context, uint, service.AnalyticsSourcesRequest) (*service.AnalyticsSourcesResult, error)
}

type storefrontAnalyticsStatusReader interface {
	StatusForSuper(context.Context) (*analytics.DataStatus, error)
}

type StorefrontAnalyticsController struct {
	service        storefrontAnalyticsReader
	productService storefrontProductAnalyticsReader
	contentService storefrontContentAnalyticsReader
	statusService  storefrontAnalyticsStatusReader
	popularity     service.ProductPopularityProvider
}

func NewStorefrontAnalyticsController(service storefrontAnalyticsReader) *StorefrontAnalyticsController {
	controller := &StorefrontAnalyticsController{service: service}
	if productService, ok := service.(storefrontProductAnalyticsReader); ok {
		controller.productService = productService
	}
	if contentService, ok := service.(storefrontContentAnalyticsReader); ok {
		controller.contentService = contentService
	}
	if statusService, ok := service.(storefrontAnalyticsStatusReader); ok {
		controller.statusService = statusService
	}
	return controller
}

func (ctl *StorefrontAnalyticsController) WithPopularity(provider service.ProductPopularityProvider) *StorefrontAnalyticsController {
	ctl.popularity = provider
	return ctl
}

func (ctl *StorefrontAnalyticsController) RegisterSuperRoutes(group *gin.RouterGroup) {
	analyticsGroup := group.Group("/analytics")
	analyticsGroup.GET("/overview", ctl.SuperOverview)
	analyticsGroup.GET("/trends", ctl.SuperTrends)
	analyticsGroup.GET("/sources", ctl.SuperSources)
	if ctl.statusService != nil {
		analyticsGroup.GET("/status", ctl.SuperStatus)
	}
	ctl.registerSuperProductAnalyticsRoutes(analyticsGroup)
	ctl.registerSuperContentAnalyticsRoutes(analyticsGroup)
	if ctl.popularity != nil {
		analyticsGroup.GET("/popularity", ctl.SuperPopularity)
	}
}

func (ctl *StorefrontAnalyticsController) SuperStatus(c *gin.Context) {
	if _, exists := c.GetQuery("tenant_id"); exists {
		c.JSON(http.StatusBadRequest, response{Error: service.ErrInvalidInput.Error()})
		return
	}
	result, err := ctl.statusService.StatusForSuper(c.Request.Context())
	writeAnalyticsResult(c, result, err)
}

func (ctl *StorefrontAnalyticsController) RegisterAdminRoutes(group *gin.RouterGroup) {
	analyticsGroup := group.Group("/admin/analytics")
	analyticsGroup.GET("/overview", ctl.AdminOverview)
	analyticsGroup.GET("/trends", ctl.AdminTrends)
	analyticsGroup.GET("/sources", ctl.AdminSources)
	ctl.registerAdminProductAnalyticsRoutes(analyticsGroup)
	if ctl.popularity != nil {
		analyticsGroup.GET("/popularity", ctl.AdminPopularity)
	}
}

func (ctl *StorefrontAnalyticsController) SuperOverview(c *gin.Context) {
	tenantID, ok := analyticsSuperTenantID(c)
	if !ok {
		return
	}
	request, ok := analyticsOverviewRequest(c)
	if !ok {
		return
	}
	result, err := ctl.service.OverviewForSuper(c.Request.Context(), tenantID, request)
	writeAnalyticsResult(c, result, err)
}

func (ctl *StorefrontAnalyticsController) AdminOverview(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}
	request, ok := analyticsOverviewRequest(c)
	if !ok {
		return
	}
	result, err := ctl.service.OverviewForTenant(c.Request.Context(), tenantID, request)
	writeAnalyticsResult(c, result, err)
}

func (ctl *StorefrontAnalyticsController) SuperTrends(c *gin.Context) {
	tenantID, ok := analyticsSuperTenantID(c)
	if !ok {
		return
	}
	result, err := ctl.service.TrendsForSuper(c.Request.Context(), tenantID, analyticsTrendsRequest(c))
	writeAnalyticsResult(c, result, err)
}

func (ctl *StorefrontAnalyticsController) AdminTrends(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}
	result, err := ctl.service.TrendsForTenant(c.Request.Context(), tenantID, analyticsTrendsRequest(c))
	writeAnalyticsResult(c, result, err)
}

func (ctl *StorefrontAnalyticsController) SuperSources(c *gin.Context) {
	tenantID, ok := analyticsSuperTenantID(c)
	if !ok {
		return
	}
	request, ok := analyticsSourcesRequest(c)
	if !ok {
		return
	}
	result, err := ctl.service.SourcesForSuper(c.Request.Context(), tenantID, request)
	writeAnalyticsResult(c, result, err)
}

func (ctl *StorefrontAnalyticsController) AdminSources(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}
	request, ok := analyticsSourcesRequest(c)
	if !ok {
		return
	}
	result, err := ctl.service.SourcesForTenant(c.Request.Context(), tenantID, request)
	writeAnalyticsResult(c, result, err)
}

func analyticsDateRangeInput(c *gin.Context) analytics.DateRangeInput {
	return analytics.DateRangeInput{
		From:     c.Query("from"),
		To:       c.Query("to"),
		Timezone: c.Query("timezone"),
	}
}

func analyticsOverviewRequest(c *gin.Context) (service.AnalyticsOverviewRequest, bool) {
	compare := false
	if value := strings.TrimSpace(c.Query("compare")); value != "" {
		switch value {
		case "true":
			compare = true
		case "false":
		default:
			c.JSON(http.StatusBadRequest, response{Error: service.ErrInvalidInput.Error()})
			return service.AnalyticsOverviewRequest{}, false
		}
	}
	return service.AnalyticsOverviewRequest{DateRange: analyticsDateRangeInput(c), Compare: compare}, true
}

func analyticsTrendsRequest(c *gin.Context) service.AnalyticsTrendsRequest {
	return service.AnalyticsTrendsRequest{
		DateRange:   analyticsDateRangeInput(c),
		Granularity: strings.TrimSpace(c.Query("granularity")),
	}
}

func analyticsSourcesRequest(c *gin.Context) (service.AnalyticsSourcesRequest, bool) {
	limit, ok := analyticsOptionalPositiveInt(c, "limit")
	if !ok {
		return service.AnalyticsSourcesRequest{}, false
	}
	return service.AnalyticsSourcesRequest{
		DateRange: analyticsDateRangeInput(c),
		Sort:      strings.TrimSpace(c.Query("sort")),
		Limit:     limit,
	}, true
}

func analyticsSuperTenantID(c *gin.Context) (*uint, bool) {
	value := strings.TrimSpace(c.Query("tenant_id"))
	if value == "" {
		return nil, true
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		c.JSON(http.StatusBadRequest, response{Error: service.ErrInvalidInput.Error()})
		return nil, false
	}
	tenantID := uint(parsed)
	return &tenantID, true
}

func analyticsOptionalPositiveInt(c *gin.Context, key string) (int, bool) {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return 0, true
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		c.JSON(http.StatusBadRequest, response{Error: service.ErrInvalidInput.Error()})
		return 0, false
	}
	return parsed, true
}

func writeAnalyticsResult(c *gin.Context, result any, err error) {
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: result})
}

func writeAnalyticsCSV(c *gin.Context, result *service.AnalyticsCSVExport, err error) {
	if err != nil {
		writeError(c, err)
		return
	}
	if result == nil {
		writeError(c, service.ErrNotFound)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, result.Filename))
	c.Data(http.StatusOK, "text/csv; charset=utf-8", result.Content)
}
