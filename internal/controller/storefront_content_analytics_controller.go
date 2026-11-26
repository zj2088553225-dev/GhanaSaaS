package controller

import (
	"context"
	"net/http"
	"strings"

	"saas/internal/pkg/trackingkey"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type storefrontContentAnalyticsReader interface {
	ContentForSuper(context.Context, service.ContentAnalyticsListRequest) (*service.ContentAnalyticsListResult, error)
	ContentDetailForSuper(context.Context, string, service.ContentAnalyticsDetailRequest) (*service.ContentAnalyticsDetailResult, error)
}

type storefrontContentAnalyticsExporter interface {
	ContentExportForSuper(context.Context, service.ContentAnalyticsListRequest) (*service.AnalyticsCSVExport, error)
}

func (ctl *StorefrontAnalyticsController) registerSuperContentAnalyticsRoutes(group *gin.RouterGroup) {
	if ctl.contentService == nil {
		return
	}
	group.GET("/content", ctl.SuperContentAnalytics)
	if exporter, ok := ctl.contentService.(storefrontContentAnalyticsExporter); ok {
		group.GET("/content/export", func(c *gin.Context) { ctl.SuperContentAnalyticsExport(c, exporter) })
	}
	group.GET("/content/:trackingKey", ctl.SuperContentAnalyticsDetail)
}

func (ctl *StorefrontAnalyticsController) SuperContentAnalyticsExport(c *gin.Context, exporter storefrontContentAnalyticsExporter) {
	if !rejectContentTenantOverride(c) {
		return
	}
	request, ok := contentAnalyticsListRequest(c)
	if !ok {
		return
	}
	result, err := exporter.ContentExportForSuper(c.Request.Context(), request)
	writeAnalyticsCSV(c, result, err)
}

func (ctl *StorefrontAnalyticsController) SuperContentAnalytics(c *gin.Context) {
	if !rejectContentTenantOverride(c) {
		return
	}
	request, ok := contentAnalyticsListRequest(c)
	if !ok {
		return
	}
	result, err := ctl.contentService.ContentForSuper(c.Request.Context(), request)
	writeAnalyticsResult(c, result, err)
}

func (ctl *StorefrontAnalyticsController) SuperContentAnalyticsDetail(c *gin.Context) {
	if !rejectContentTenantOverride(c) {
		return
	}
	trackingKey := trackingkey.Normalize(c.Param("trackingKey"))
	if !trackingkey.IsValid(trackingKey) {
		c.JSON(http.StatusBadRequest, response{Error: service.ErrInvalidInput.Error()})
		return
	}
	result, err := ctl.contentService.ContentDetailForSuper(
		c.Request.Context(),
		trackingKey,
		service.ContentAnalyticsDetailRequest{
			DateRange: analyticsDateRangeInput(c), Granularity: strings.TrimSpace(c.Query("granularity")),
		},
	)
	writeAnalyticsResult(c, result, err)
}

func contentAnalyticsListRequest(c *gin.Context) (service.ContentAnalyticsListRequest, bool) {
	page, ok := analyticsOptionalPositiveInt(c, "page")
	if !ok {
		return service.ContentAnalyticsListRequest{}, false
	}
	pageSize, ok := analyticsOptionalPositiveInt(c, "page_size")
	if !ok {
		return service.ContentAnalyticsListRequest{}, false
	}
	minImpressions, ok := analyticsOptionalNonNegativeInt(c, "min_impressions")
	if !ok {
		return service.ContentAnalyticsListRequest{}, false
	}
	currentlyExists, ok := analyticsOptionalBool(c, "currently_exists")
	if !ok {
		return service.ContentAnalyticsListRequest{}, false
	}
	return service.ContentAnalyticsListRequest{
		DateRange:       analyticsDateRangeInput(c),
		Page:            page,
		PageSize:        pageSize,
		Search:          strings.TrimSpace(c.Query("search")),
		ContentType:     strings.TrimSpace(c.Query("content_type")),
		CurrentlyExists: currentlyExists,
		Sort:            strings.TrimSpace(c.Query("sort")),
		Order:           strings.TrimSpace(c.Query("order")),
		MinImpressions:  minImpressions,
	}, true
}

func analyticsOptionalBool(c *gin.Context, key string) (*bool, bool) {
	value, exists := c.GetQuery(key)
	if !exists {
		return nil, true
	}
	switch strings.TrimSpace(value) {
	case "true":
		parsed := true
		return &parsed, true
	case "false":
		parsed := false
		return &parsed, true
	default:
		c.JSON(http.StatusBadRequest, response{Error: service.ErrInvalidInput.Error()})
		return nil, false
	}
}

func rejectContentTenantOverride(c *gin.Context) bool {
	if _, exists := c.GetQuery("tenant_id"); exists {
		c.JSON(http.StatusBadRequest, response{Error: service.ErrInvalidInput.Error()})
		return false
	}
	return true
}
