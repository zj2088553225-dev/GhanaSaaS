package controller

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type storefrontProductAnalyticsReader interface {
	ProductsForSuper(context.Context, *uint, service.ProductAnalyticsListRequest) (*service.ProductAnalyticsListResult, error)
	ProductsForTenant(context.Context, uint, service.ProductAnalyticsListRequest) (*service.ProductAnalyticsListResult, error)
	TopProductsForSuper(context.Context, *uint, service.TopProductAnalyticsRequest) (*service.TopProductAnalyticsResult, error)
	TopProductsForTenant(context.Context, uint, service.TopProductAnalyticsRequest) (*service.TopProductAnalyticsResult, error)
	LowCTRProductsForSuper(context.Context, *uint, service.LowCTRProductAnalyticsRequest) (*service.LowCTRProductAnalyticsResult, error)
	LowCTRProductsForTenant(context.Context, uint, service.LowCTRProductAnalyticsRequest) (*service.LowCTRProductAnalyticsResult, error)
	ProductDetailForSuper(context.Context, *uint, uint, service.ProductAnalyticsDetailRequest) (*service.ProductAnalyticsDetailResult, error)
	ProductDetailForTenant(context.Context, uint, uint, service.ProductAnalyticsDetailRequest) (*service.ProductAnalyticsDetailResult, error)
}

type storefrontProductAnalyticsExporter interface {
	ProductExportForSuper(context.Context, *uint, service.ProductAnalyticsListRequest) (*service.AnalyticsCSVExport, error)
	ProductExportForTenant(context.Context, uint, service.ProductAnalyticsListRequest) (*service.AnalyticsCSVExport, error)
}

func (ctl *StorefrontAnalyticsController) registerSuperProductAnalyticsRoutes(group *gin.RouterGroup) {
	if ctl.productService == nil {
		return
	}
	group.GET("/products", ctl.SuperProducts)
	if exporter, ok := ctl.productService.(storefrontProductAnalyticsExporter); ok {
		group.GET("/products/export", func(c *gin.Context) { ctl.SuperProductExport(c, exporter) })
	}
	group.GET("/products/top", ctl.SuperTopProducts)
	group.GET("/products/low-ctr", ctl.SuperLowCTRProducts)
	group.GET("/products/:id", ctl.SuperProductDetail)
}

func (ctl *StorefrontAnalyticsController) registerAdminProductAnalyticsRoutes(group *gin.RouterGroup) {
	if ctl.productService == nil {
		return
	}
	group.GET("/products", ctl.AdminProducts)
	if exporter, ok := ctl.productService.(storefrontProductAnalyticsExporter); ok {
		group.GET("/products/export", func(c *gin.Context) { ctl.AdminProductExport(c, exporter) })
	}
	group.GET("/products/top", ctl.AdminTopProducts)
	group.GET("/products/low-ctr", ctl.AdminLowCTRProducts)
	group.GET("/products/:id", ctl.AdminProductDetail)
}

func (ctl *StorefrontAnalyticsController) SuperProductExport(c *gin.Context, exporter storefrontProductAnalyticsExporter) {
	tenantID, ok := analyticsSuperTenantID(c)
	if !ok {
		return
	}
	request, ok := productAnalyticsListRequest(c)
	if !ok {
		return
	}
	result, err := exporter.ProductExportForSuper(c.Request.Context(), tenantID, request)
	writeAnalyticsCSV(c, result, err)
}

func (ctl *StorefrontAnalyticsController) AdminProductExport(c *gin.Context, exporter storefrontProductAnalyticsExporter) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}
	request, ok := productAnalyticsListRequest(c)
	if !ok {
		return
	}
	result, err := exporter.ProductExportForTenant(c.Request.Context(), tenantID, request)
	writeAnalyticsCSV(c, result, err)
}

func (ctl *StorefrontAnalyticsController) SuperProducts(c *gin.Context) {
	tenantID, ok := analyticsSuperTenantID(c)
	if !ok {
		return
	}
	request, ok := productAnalyticsListRequest(c)
	if !ok {
		return
	}
	result, err := ctl.productService.ProductsForSuper(c.Request.Context(), tenantID, request)
	writeAnalyticsResult(c, result, err)
}

func (ctl *StorefrontAnalyticsController) AdminProducts(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}
	request, ok := productAnalyticsListRequest(c)
	if !ok {
		return
	}
	result, err := ctl.productService.ProductsForTenant(c.Request.Context(), tenantID, request)
	writeAnalyticsResult(c, result, err)
}

func (ctl *StorefrontAnalyticsController) SuperTopProducts(c *gin.Context) {
	tenantID, ok := analyticsSuperTenantID(c)
	if !ok {
		return
	}
	request, ok := topProductAnalyticsRequest(c)
	if !ok {
		return
	}
	result, err := ctl.productService.TopProductsForSuper(c.Request.Context(), tenantID, request)
	writeAnalyticsResult(c, result, err)
}

func (ctl *StorefrontAnalyticsController) AdminTopProducts(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}
	request, ok := topProductAnalyticsRequest(c)
	if !ok {
		return
	}
	result, err := ctl.productService.TopProductsForTenant(c.Request.Context(), tenantID, request)
	writeAnalyticsResult(c, result, err)
}

func (ctl *StorefrontAnalyticsController) SuperLowCTRProducts(c *gin.Context) {
	tenantID, ok := analyticsSuperTenantID(c)
	if !ok {
		return
	}
	request, ok := lowCTRProductAnalyticsRequest(c)
	if !ok {
		return
	}
	result, err := ctl.productService.LowCTRProductsForSuper(c.Request.Context(), tenantID, request)
	writeAnalyticsResult(c, result, err)
}

func (ctl *StorefrontAnalyticsController) AdminLowCTRProducts(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}
	request, ok := lowCTRProductAnalyticsRequest(c)
	if !ok {
		return
	}
	result, err := ctl.productService.LowCTRProductsForTenant(c.Request.Context(), tenantID, request)
	writeAnalyticsResult(c, result, err)
}

func (ctl *StorefrontAnalyticsController) SuperProductDetail(c *gin.Context) {
	productID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	tenantID, ok := analyticsSuperTenantID(c)
	if !ok {
		return
	}
	result, err := ctl.productService.ProductDetailForSuper(c.Request.Context(), tenantID, productID, productAnalyticsDetailRequest(c))
	writeAnalyticsResult(c, result, err)
}

func (ctl *StorefrontAnalyticsController) AdminProductDetail(c *gin.Context) {
	productID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}
	result, err := ctl.productService.ProductDetailForTenant(c.Request.Context(), tenantID, productID, productAnalyticsDetailRequest(c))
	writeAnalyticsResult(c, result, err)
}

func productAnalyticsListRequest(c *gin.Context) (service.ProductAnalyticsListRequest, bool) {
	page, ok := analyticsOptionalPositiveInt(c, "page")
	if !ok {
		return service.ProductAnalyticsListRequest{}, false
	}
	pageSize, ok := analyticsOptionalPositiveInt(c, "page_size")
	if !ok {
		return service.ProductAnalyticsListRequest{}, false
	}
	return service.ProductAnalyticsListRequest{
		DateRange: analyticsDateRangeInput(c),
		Page:      page,
		PageSize:  pageSize,
		Search:    strings.TrimSpace(c.Query("search")),
		Status:    strings.TrimSpace(c.Query("status")),
		Sort:      strings.TrimSpace(c.Query("sort")),
		Order:     strings.TrimSpace(c.Query("order")),
	}, true
}

func topProductAnalyticsRequest(c *gin.Context) (service.TopProductAnalyticsRequest, bool) {
	limit, ok := analyticsOptionalPositiveInt(c, "limit")
	if !ok {
		return service.TopProductAnalyticsRequest{}, false
	}
	minImpressions, ok := analyticsOptionalNonNegativeInt(c, "min_impressions")
	if !ok {
		return service.TopProductAnalyticsRequest{}, false
	}
	minClicks, ok := analyticsOptionalNonNegativeInt(c, "min_clicks")
	if !ok {
		return service.TopProductAnalyticsRequest{}, false
	}
	minViews, ok := analyticsOptionalNonNegativeInt(c, "min_views")
	if !ok {
		return service.TopProductAnalyticsRequest{}, false
	}
	return service.TopProductAnalyticsRequest{
		DateRange:      analyticsDateRangeInput(c),
		Metric:         strings.TrimSpace(c.Query("metric")),
		Limit:          limit,
		MinImpressions: minImpressions,
		MinClicks:      minClicks,
		MinViews:       minViews,
	}, true
}

func lowCTRProductAnalyticsRequest(c *gin.Context) (service.LowCTRProductAnalyticsRequest, bool) {
	limit, ok := analyticsOptionalPositiveInt(c, "limit")
	if !ok {
		return service.LowCTRProductAnalyticsRequest{}, false
	}
	minImpressions, ok := analyticsOptionalNonNegativeInt(c, "min_impressions")
	if !ok {
		return service.LowCTRProductAnalyticsRequest{}, false
	}
	return service.LowCTRProductAnalyticsRequest{
		DateRange: analyticsDateRangeInput(c), MinImpressions: minImpressions, Limit: limit,
	}, true
}

func productAnalyticsDetailRequest(c *gin.Context) service.ProductAnalyticsDetailRequest {
	return service.ProductAnalyticsDetailRequest{
		DateRange: analyticsDateRangeInput(c), Granularity: strings.TrimSpace(c.Query("granularity")),
	}
}

func analyticsOptionalNonNegativeInt(c *gin.Context, key string) (*int, bool) {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return nil, true
	}
	parsed, err := parseAnalyticsInt(value)
	if err != nil || parsed < 0 {
		c.JSON(http.StatusBadRequest, response{Error: service.ErrInvalidInput.Error()})
		return nil, false
	}
	return &parsed, true
}

func parseAnalyticsInt(value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, service.ErrInvalidInput
	}
	return parsed, nil
}
