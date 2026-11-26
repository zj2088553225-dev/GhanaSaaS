package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"saas/internal/middleware"
	"saas/internal/models"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

type AdminProductController struct {
	service *service.AdminProductService
}

type adminProductPayload struct {
	PlatformCategoryID uint                             `json:"platform_category_id"`
	CategoryID         *uint                            `json:"category_id"`
	ShopCategoryID     *uint                            `json:"shop_category_id"`
	Name               string                           `json:"name"`
	Description        string                           `json:"description"`
	DetailHTML         string                           `json:"detail_html"`
	MainImage          string                           `json:"main_image"`
	VideoURL           string                           `json:"video_url"`
	Material           string                           `json:"material"`
	Dimensions         string                           `json:"dimensions"`
	Weight             string                           `json:"weight"`
	SliderImages       json.RawMessage                  `json:"slider_images"`
	SpecTemplate       json.RawMessage                  `json:"spec_template"`
	Status             models.SPUStatus                 `json:"status"`
	SKUs               []adminSKUPayload                `json:"skus"`
	Translations       *adminProductTranslationsPayload `json:"translations"`
}

type adminProductTranslationsPayload struct {
	Zh adminProductTranslationPayload `json:"zh"`
}

type adminProductTranslationPayload struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	DetailHTML  string `json:"detail_html"`
	Material    string `json:"material"`
	Dimensions  string `json:"dimensions"`
	Weight      string `json:"weight"`
}

type adminSKUPayload struct {
	ID            uint            `json:"id"`
	Name          string          `json:"name"`
	ERPSKUCode    string          `json:"erp_sku_code"`
	ImageURL      string          `json:"image_url"`
	Specs         json.RawMessage `json:"specs"`
	RetailPrice   int64           `json:"retail_price"`
	OriginalPrice int64           `json:"original_price"`
	SellableStock int64           `json:"sellable_stock"`
}

type adminProductStatusPayload struct {
	Status models.SPUStatus `json:"status" binding:"required"`
}

type adminProductResponse struct {
	ID                 uint                             `json:"id"`
	TenantID           uint                             `json:"tenant_id"`
	PlatformCategoryID uint                             `json:"platform_category_id"`
	CategoryID         *uint                            `json:"category_id"`
	ShopCategoryID     *uint                            `json:"shop_category_id,omitempty"`
	Name               string                           `json:"name"`
	Description        string                           `json:"description"`
	DetailHTML         string                           `json:"detail_html"`
	MainImage          string                           `json:"main_image"`
	VideoURL           string                           `json:"video_url"`
	Material           string                           `json:"material"`
	Dimensions         string                           `json:"dimensions"`
	Weight             string                           `json:"weight"`
	SliderImages       datatypes.JSON                   `json:"slider_images"`
	SpecTemplate       datatypes.JSON                   `json:"spec_template"`
	Status             models.SPUStatus                 `json:"status"`
	SKUs               []adminSKUResponse               `json:"skus"`
	Translations       adminProductTranslationsResponse `json:"translations"`
	CreatedAt          time.Time                        `json:"created_at"`
	UpdatedAt          time.Time                        `json:"updated_at"`
}

type adminProductTranslationsResponse struct {
	Zh adminProductTranslationPayload `json:"zh"`
}

type adminSKUResponse struct {
	ID            uint           `json:"id"`
	ProductID     uint           `json:"product_id"`
	Name          string         `json:"name"`
	ERPSKUCode    string         `json:"erp_sku_code"`
	ImageURL      string         `json:"image_url"`
	Specs         datatypes.JSON `json:"specs"`
	RetailPrice   int64          `json:"retail_price"`
	OriginalPrice int64          `json:"original_price"`
	SellableStock int64          `json:"sellable_stock"`
	IsActive      bool           `json:"is_active"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type adminProductPageResponse struct {
	Items    []adminProductResponse `json:"items"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
}

const maxProductImportFileBytes = 10 << 20

func NewAdminProductController(service *service.AdminProductService) *AdminProductController {
	return &AdminProductController{service: service}
}

func (ctl *AdminProductController) RegisterRoutes(group *gin.RouterGroup) {
	products := group.Group("/admin/products")
	products.POST("", ctl.Create)
	products.GET("", ctl.List)
	products.GET("/import/template", ctl.ImportTemplate)
	products.POST("/import/preview", ctl.ImportPreview)
	products.POST("/import/confirm", ctl.ImportConfirm)
	products.GET("/:id", ctl.Get)
	products.PUT("/:id", ctl.Update)
	products.PATCH("/:id/status", ctl.UpdateStatus)
	products.DELETE("/:id", ctl.Delete)
}

func (ctl *AdminProductController) ImportTemplate(c *gin.Context) {
	content, err := service.BuildProductImportTemplate()
	if err != nil {
		writeError(c, err)
		return
	}

	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", `attachment; filename="product-import-template.xlsx"`)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", content)
}

func (ctl *AdminProductController) ImportPreview(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}
	file, ok := openProductImportFile(c)
	if !ok {
		return
	}
	defer func() { _ = file.Close() }()

	preview, err := ctl.service.PreviewImport(c.Request.Context(), tenantID, file)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: preview})
}

func (ctl *AdminProductController) ImportConfirm(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}
	file, ok := openProductImportFile(c)
	if !ok {
		return
	}
	defer func() { _ = file.Close() }()

	result, preview, err := ctl.service.ConfirmImport(c.Request.Context(), tenantID, file)
	if err != nil {
		if preview != nil {
			c.JSON(http.StatusBadRequest, response{Data: preview, Error: "导入校验失败"})
			return
		}
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: result})
}

func (ctl *AdminProductController) Create(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	input, ok := bindAdminProductInput(c)
	if !ok {
		return
	}

	product, err := ctl.service.Create(c.Request.Context(), tenantID, input)
	if err != nil {
		writeError(c, err)
		return
	}

	translations, err := ctl.productTranslations(c, tenantID, product.ID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, response{Data: toAdminProductResponse(product, translations)})
}

func (ctl *AdminProductController) Get(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	product, err := ctl.service.FindByID(c.Request.Context(), tenantID, id)
	if err != nil {
		writeError(c, err)
		return
	}

	translations, err := ctl.productTranslations(c, tenantID, product.ID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: toAdminProductResponse(product, translations)})
}

func (ctl *AdminProductController) List(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}
	query, ok := parseAdminProductQuery(c)
	if !ok {
		return
	}

	page, err := ctl.service.List(c.Request.Context(), tenantID, query)
	if err != nil {
		writeError(c, err)
		return
	}

	items := make([]adminProductResponse, 0, len(page.Items))
	for i := range page.Items {
		items = append(items, toAdminProductResponse(&page.Items[i], service.ProductTranslationInput{}))
	}

	c.JSON(http.StatusOK, response{Data: adminProductPageResponse{
		Items:    items,
		Total:    page.Total,
		Page:     page.Page,
		PageSize: page.PageSize,
	}})
}

func (ctl *AdminProductController) Update(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	input, ok := bindAdminProductInput(c)
	if !ok {
		return
	}

	product, err := ctl.service.Update(c.Request.Context(), tenantID, id, input)
	if err != nil {
		writeError(c, err)
		return
	}

	translations, err := ctl.productTranslations(c, tenantID, product.ID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: toAdminProductResponse(product, translations)})
}

func (ctl *AdminProductController) UpdateStatus(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	var payload adminProductStatusPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	product, err := ctl.service.UpdateStatus(c.Request.Context(), tenantID, id, payload.Status)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: toAdminProductResponse(product, service.ProductTranslationInput{})})
}

func (ctl *AdminProductController) Delete(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	if err := ctl.service.Delete(c.Request.Context(), tenantID, id); err != nil {
		if errors.Is(err, service.ErrAdminProductHasOrderRecords) {
			c.JSON(http.StatusConflict, gin.H{
				"error":    err.Error(),
				"error_en": service.AdminProductHasOrderRecordsMessageEN,
			})
			return
		}
		if errors.Is(err, service.ErrNotFound) || errors.Is(err, service.ErrInvalidInput) {
			writeError(c, err)
			return
		}
		c.JSON(http.StatusInternalServerError, response{Error: "商品删除失败，请稍后重试"})
		return
	}

	c.Status(http.StatusNoContent)
}

func openProductImportFile(c *gin.Context) (multipartFile, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxProductImportFileBytes)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, response{Error: "请上传 xlsx 文件"})
		return nil, false
	}
	if fileHeader.Size > maxProductImportFileBytes {
		c.JSON(http.StatusBadRequest, response{Error: "Excel 文件不能超过 10MB"})
		return nil, false
	}
	if strings.ToLower(filepath.Ext(fileHeader.Filename)) != ".xlsx" {
		c.JSON(http.StatusBadRequest, response{Error: "仅支持 .xlsx 文件"})
		return nil, false
	}
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, response{Error: "Excel 文件读取失败"})
		return nil, false
	}
	return file, true
}

type multipartFile interface {
	Read([]byte) (int, error)
	Close() error
}

func bindAdminProductInput(c *gin.Context) (service.AdminProductInput, bool) {
	var payload adminProductPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
		return service.AdminProductInput{}, false
	}
	input, err := payload.toInput()
	if err != nil {
		writeError(c, err)
		return service.AdminProductInput{}, false
	}
	return input, true
}

func (p adminProductPayload) toInput() (service.AdminProductInput, error) {
	var sliderImages *datatypes.JSON
	if len(p.SliderImages) > 0 {
		if !json.Valid(p.SliderImages) {
			return service.AdminProductInput{}, service.ErrInvalidInput
		}
		value := datatypes.JSON(append([]byte(nil), p.SliderImages...))
		sliderImages = &value
	}
	var specTemplate *datatypes.JSON
	if len(p.SpecTemplate) > 0 {
		if !json.Valid(p.SpecTemplate) {
			return service.AdminProductInput{}, service.ErrInvalidInput
		}
		value := datatypes.JSON(append([]byte(nil), p.SpecTemplate...))
		specTemplate = &value
	}

	skus := make([]service.AdminSKUInput, 0, len(p.SKUs))
	for _, sku := range p.SKUs {
		var specs *datatypes.JSON
		if len(sku.Specs) > 0 {
			if !json.Valid(sku.Specs) {
				return service.AdminProductInput{}, service.ErrInvalidInput
			}
			value := datatypes.JSON(append([]byte(nil), sku.Specs...))
			specs = &value
		}
		skus = append(skus, service.AdminSKUInput{
			ID:            sku.ID,
			Name:          sku.Name,
			ERPSKUCode:    sku.ERPSKUCode,
			ImageURL:      sku.ImageURL,
			Specs:         specs,
			RetailPrice:   sku.RetailPrice,
			OriginalPrice: sku.OriginalPrice,
			SellableStock: sku.SellableStock,
		})
	}

	categoryID := p.CategoryID
	if p.ShopCategoryID != nil {
		categoryID = p.ShopCategoryID
	}

	return service.AdminProductInput{
		PlatformCategoryID: p.PlatformCategoryID,
		CategoryID:         categoryID,
		Name:               p.Name,
		Description:        p.Description,
		DetailHTML:         p.DetailHTML,
		MainImage:          p.MainImage,
		VideoURL:           p.VideoURL,
		Material:           p.Material,
		Dimensions:         p.Dimensions,
		Weight:             p.Weight,
		SliderImages:       sliderImages,
		SpecTemplate:       specTemplate,
		Status:             p.Status,
		SKUs:               skus,
		Translations:       p.toTranslationInput(),
	}, nil
}

func (p adminProductPayload) toTranslationInput() *service.ProductTranslationInput {
	if p.Translations == nil {
		return nil
	}
	return &service.ProductTranslationInput{
		Name:        p.Translations.Zh.Name,
		Description: p.Translations.Zh.Description,
		DetailHTML:  p.Translations.Zh.DetailHTML,
		Material:    p.Translations.Zh.Material,
		Dimensions:  p.Translations.Zh.Dimensions,
		Weight:      p.Translations.Zh.Weight,
	}
}

func parseAdminProductQuery(c *gin.Context) (service.AdminProductQuery, bool) {
	query := service.AdminProductQuery{
		Keyword:  c.Query("keyword"),
		Page:     parsePositiveInt(c.Query("page"), 1),
		PageSize: parsePositiveInt(c.Query("page_size"), 20),
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
		categoryID, err := strconv.ParseUint(platformCategoryIDValue, 10, 64)
		if err != nil || categoryID == 0 {
			c.JSON(http.StatusBadRequest, response{Error: "invalid platform_category_id"})
			return query, false
		}
		parsed := uint(categoryID)
		query.PlatformCategoryID = &parsed
	}

	if statusValue := c.Query("status"); statusValue != "" {
		status := models.SPUStatus(statusValue)
		query.Status = &status
	}

	return query, true
}

func tenantIDFromAuthContext(c *gin.Context) (uint, error) {
	tenantID, ok := middleware.TenantIDFromContext(c)
	if !ok || tenantID == nil || *tenantID == 0 {
		return 0, service.ErrForbidden
	}
	return *tenantID, nil
}

func (ctl *AdminProductController) productTranslations(c *gin.Context, tenantID uint, productID uint) (service.ProductTranslationInput, error) {
	return ctl.service.TranslationFields(c.Request.Context(), tenantID, productID, "zh")
}

func toAdminProductResponse(product *models.SPU, translations service.ProductTranslationInput) adminProductResponse {
	if product == nil {
		return adminProductResponse{}
	}

	skus := make([]adminSKUResponse, 0, len(product.SKUs))
	for _, sku := range product.SKUs {
		skus = append(skus, adminSKUResponse{
			ID:            sku.ID,
			ProductID:     sku.ProductID,
			Name:          sku.Name,
			ERPSKUCode:    sku.ERPSKUCode,
			ImageURL:      sku.ImageURL,
			Specs:         sku.Specs,
			RetailPrice:   sku.RetailPrice,
			OriginalPrice: sku.OriginalPrice,
			SellableStock: sku.SellableStock,
			IsActive:      sku.IsActive,
			CreatedAt:     sku.CreatedAt,
			UpdatedAt:     sku.UpdatedAt,
		})
	}

	return adminProductResponse{
		ID:                 product.ID,
		TenantID:           product.TenantID,
		PlatformCategoryID: product.PlatformCategoryID,
		CategoryID:         product.CategoryID,
		ShopCategoryID:     product.CategoryID,
		Name:               product.Name,
		Description:        product.Description,
		DetailHTML:         product.DetailHTML,
		MainImage:          product.MainImage,
		VideoURL:           product.VideoURL,
		Material:           product.Material,
		Dimensions:         product.Dimensions,
		Weight:             product.Weight,
		SliderImages:       product.SliderImages,
		SpecTemplate:       product.SpecTemplate,
		Status:             product.Status,
		SKUs:               skus,
		Translations: adminProductTranslationsResponse{
			Zh: adminProductTranslationPayload{
				Name:        translations.Name,
				Description: translations.Description,
				DetailHTML:  translations.DetailHTML,
				Material:    translations.Material,
				Dimensions:  translations.Dimensions,
				Weight:      translations.Weight,
			},
		},
		CreatedAt: product.CreatedAt,
		UpdatedAt: product.UpdatedAt,
	}
}
