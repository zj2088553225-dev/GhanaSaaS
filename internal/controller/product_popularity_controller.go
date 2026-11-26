package controller

import (
	"net/http"
	"strconv"
	"strings"

	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

// storefrontPopularProduct is deliberately separate from the administrator
// ranking DTO. Public responses expose only normal product-card fields.
type storefrontPopularProduct struct {
	ProductID          uint   `json:"product_id"`
	CategoryID         *uint  `json:"category_id"`
	PlatformCategoryID uint   `json:"platform_category_id"`
	Name               string `json:"name"`
	MainImage          string `json:"main_image"`
	MinPrice           int64  `json:"min_price"`
	MaxPrice           int64  `json:"max_price"`
	OriginalPrice      int64  `json:"original_price"`
}

func (ctl *StorefrontController) PopularProducts(c *gin.Context) {
	limit, categoryID, ok := popularityParameters(c)
	if !ok {
		return
	}
	products, err := ctl.popularityProvider.CalculatePopularProducts(c.Request.Context(), service.ProductPopularityRequest{
		Limit:      limit,
		CategoryID: categoryID,
		Public:     true,
		Locale:     parseLocaleQuery(c),
	})
	if err != nil {
		writeError(c, err)
		return
	}

	result := make([]storefrontPopularProduct, 0, len(products))
	for _, product := range products {
		result = append(result, storefrontPopularProduct{
			ProductID:          product.ProductID,
			CategoryID:         product.CategoryID,
			PlatformCategoryID: product.PlatformCategoryID,
			Name:               product.Name,
			MainImage:          product.MainImage,
			MinPrice:           product.MinPrice,
			MaxPrice:           product.MaxPrice,
			OriginalPrice:      product.OriginalPrice,
		})
	}
	c.JSON(http.StatusOK, response{Data: result})
}

func (ctl *StorefrontAnalyticsController) SuperPopularity(c *gin.Context) {
	tenantID, ok := analyticsSuperTenantID(c)
	if !ok {
		return
	}
	ctl.popularityResult(c, service.ProductPopularityRequest{
		TenantID:   tenantID,
		SuperAdmin: true,
	})
}

func (ctl *StorefrontAnalyticsController) AdminPopularity(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}
	ctl.popularityResult(c, service.ProductPopularityRequest{TenantID: &tenantID})
}

func (ctl *StorefrontAnalyticsController) popularityResult(c *gin.Context, request service.ProductPopularityRequest) {
	limit, categoryID, ok := popularityParameters(c)
	if !ok {
		return
	}
	request.Limit = limit
	request.CategoryID = categoryID
	request.Locale = parseLocaleQuery(c)
	result, err := ctl.popularity.CalculatePopularProducts(c.Request.Context(), request)
	writeAnalyticsResult(c, result, err)
}

func popularityParameters(c *gin.Context) (int, *uint, bool) {
	limit := service.DefaultProductPopularityLimit
	if value := strings.TrimSpace(c.Query("limit")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > service.MaxProductPopularityLimit {
			c.JSON(http.StatusBadRequest, response{Error: service.ErrInvalidInput.Error()})
			return 0, nil, false
		}
		limit = parsed
	}

	var categoryID *uint
	if value := strings.TrimSpace(c.Query("category_id")); value != "" {
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil || parsed == 0 {
			c.JSON(http.StatusBadRequest, response{Error: service.ErrInvalidInput.Error()})
			return 0, nil, false
		}
		normalized := uint(parsed)
		categoryID = &normalized
	}
	return limit, categoryID, true
}
