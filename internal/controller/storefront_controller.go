package controller

import (
	"net/http"
	"strconv"
	"strings"

	"saas/internal/pkg/locale"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type StorefrontController struct {
	service            *service.StorefrontService
	popularityProvider service.ProductPopularityProvider
}

func NewStorefrontController(service *service.StorefrontService) *StorefrontController {
	return &StorefrontController{service: service}
}

func (ctl *StorefrontController) WithPopularity(provider service.ProductPopularityProvider) *StorefrontController {
	ctl.popularityProvider = provider
	return ctl
}

func (ctl *StorefrontController) RegisterRoutes(group *gin.RouterGroup) {
	storefront := group.Group("/storefront")
	storefront.GET("/categories", ctl.CategoryTree)
	storefront.GET("/products", ctl.Products)
	if ctl.popularityProvider != nil {
		storefront.GET("/products/popular", ctl.PopularProducts)
	}
	storefront.GET("/products/:id", ctl.ProductDetail)
	storefront.GET("/shops", ctl.Shops)
	storefront.GET("/shops/:slug", ctl.ShopProfile)
	storefront.GET("/shops/:slug/products", ctl.ShopProducts)
}

func (ctl *StorefrontController) Sitemap(c *gin.Context) {
	sitemap, err := ctl.service.SitemapXML(c.Request.Context(), "https://www.ghanachinatown.com")
	if err != nil {
		writeError(c, err)
		return
	}

	c.Data(http.StatusOK, "application/xml; charset=utf-8", sitemap)
}

func (ctl *StorefrontController) CategoryTree(c *gin.Context) {
	categories, err := ctl.service.CategoryTreeWithLocale(c.Request.Context(), parseLocaleQuery(c))
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: categories})
}

func (ctl *StorefrontController) Products(c *gin.Context) {
	query, ok := parseStorefrontProductQuery(c)
	if !ok {
		return
	}

	products, err := ctl.service.Products(c.Request.Context(), query)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: products})
}

func (ctl *StorefrontController) ProductDetail(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	product, err := ctl.service.ProductDetailWithLocale(c.Request.Context(), id, parseLocaleQuery(c))
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: product})
}

func (ctl *StorefrontController) ShopProfile(c *gin.Context) {
	profile, err := ctl.service.PublicShopProfileWithLocale(c.Request.Context(), c.Param("slug"), parseLocaleQuery(c))
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: profile})
}

func (ctl *StorefrontController) Shops(c *gin.Context) {
	shops, err := ctl.service.PublicShops(c.Request.Context(), service.StorefrontShopQuery{
		Locale:   parseLocaleQuery(c),
		Page:     parsePositiveInt(c.Query("page"), 1),
		PageSize: parsePositiveInt(c.Query("page_size"), parsePositiveInt(c.Query("limit"), 20)),
	})
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: shops})
}

func (ctl *StorefrontController) ShopProducts(c *gin.Context) {
	query, ok := parseShopProductQuery(c)
	if !ok {
		return
	}

	products, err := ctl.service.ShopProducts(c.Request.Context(), c.Param("slug"), query)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: products})
}

func parseStorefrontProductQuery(c *gin.Context) (service.StorefrontProductQuery, bool) {
	query := service.StorefrontProductQuery{
		Keyword:  c.Query("keyword"),
		Locale:   parseLocaleQuery(c),
		Page:     parsePositiveInt(c.Query("page"), 1),
		PageSize: parsePositiveInt(c.Query("page_size"), parsePositiveInt(c.Query("limit"), 20)),
	}

	if categoryIDValue := c.Query("category_id"); categoryIDValue != "" {
		categoryID, err := strconv.ParseUint(categoryIDValue, 10, 64)
		if err != nil || categoryID == 0 {
			c.JSON(http.StatusBadRequest, response{Error: "invalid category_id"})
			return query, false
		}
		parsed := uint(categoryID)
		query.CategoryID = &parsed
	}

	if platformCategoryIDValue := c.Query("platform_category_id"); platformCategoryIDValue != "" {
		platformCategoryID, err := strconv.ParseUint(platformCategoryIDValue, 10, 64)
		if err != nil || platformCategoryID == 0 {
			c.JSON(http.StatusBadRequest, response{Error: "invalid platform_category_id"})
			return query, false
		}
		parsed := uint(platformCategoryID)
		query.PlatformCategoryID = &parsed
	}

	if isSelfOperatedValue := c.Query("is_self_operated"); isSelfOperatedValue != "" {
		isSelfOperated, err := strconv.ParseBool(isSelfOperatedValue)
		if err != nil {
			c.JSON(http.StatusBadRequest, response{Error: "invalid is_self_operated"})
			return query, false
		}
		query.IsSelfOperated = isSelfOperated
	}
	source, ok := parseProductSource(c)
	if !ok {
		return query, false
	}
	if source != "" {
		query.Source = source
	} else if query.IsSelfOperated {
		query.Source = "t6"
	}
	sort, ok := parseProductSort(c)
	if !ok {
		return query, false
	}
	query.Sort = sort

	return query, true
}

func parseShopProductQuery(c *gin.Context) (service.StorefrontProductQuery, bool) {
	query := service.StorefrontProductQuery{
		Keyword:  c.Query("keyword"),
		Locale:   parseLocaleQuery(c),
		Page:     parsePositiveInt(c.Query("page"), 1),
		PageSize: parsePositiveInt(c.Query("page_size"), parsePositiveInt(c.Query("limit"), 20)),
	}

	if categoryIDValue := c.Query("category_id"); categoryIDValue != "" {
		categoryID, err := strconv.ParseUint(categoryIDValue, 10, 64)
		if err != nil || categoryID == 0 {
			c.JSON(http.StatusBadRequest, response{Error: "invalid category_id"})
			return query, false
		}
		parsed := uint(categoryID)
		query.CategoryID = &parsed
	}

	if platformCategoryIDValue := c.Query("platform_category_id"); platformCategoryIDValue != "" {
		platformCategoryID, err := strconv.ParseUint(platformCategoryIDValue, 10, 64)
		if err != nil || platformCategoryID == 0 {
			c.JSON(http.StatusBadRequest, response{Error: "invalid platform_category_id"})
			return query, false
		}
		parsed := uint(platformCategoryID)
		query.PlatformCategoryID = &parsed
	}
	sort, ok := parseProductSort(c)
	if !ok {
		return query, false
	}
	query.Sort = sort

	return query, true
}

func parseProductSource(c *gin.Context) (string, bool) {
	source := strings.TrimSpace(strings.ToLower(c.Query("source")))
	if source == "" || source == "all" {
		return source, true
	}
	switch source {
	case "t6", "merchant":
		return source, true
	default:
		c.JSON(http.StatusBadRequest, response{Error: "invalid source"})
		return "", false
	}
}

func parseProductSort(c *gin.Context) (string, bool) {
	sort := strings.TrimSpace(c.Query("sort"))
	if sort == "" {
		return "", true
	}
	switch sort {
	case "latest", "hot", "price_asc", "price_desc":
		return sort, true
	default:
		c.JSON(http.StatusBadRequest, response{Error: "invalid sort"})
		return "", false
	}
}

func parseLocaleQuery(c *gin.Context) string {
	return locale.NormalizeLocale(c.Query("locale"))
}

func parsePositiveInt(value string, fallback int) int {
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
