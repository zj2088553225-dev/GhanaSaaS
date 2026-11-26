package controller

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"saas/internal/models"
	"saas/internal/pkg/locale"
	"saas/internal/repository"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type PlatformCategoryController struct {
	service platformCategoryService
}

type platformCategoryService interface {
	Create(ctx context.Context, input service.PlatformCategoryInput) (*models.PlatformCategory, error)
	FindByID(ctx context.Context, id uint) (*models.PlatformCategory, error)
	FindByIDWithBilingualDisplay(ctx context.Context, id uint) (*models.PlatformCategory, error)
	TranslationFields(ctx context.Context, id uint, requestedLocale string) (service.PlatformCategoryTranslationInput, error)
	List(ctx context.Context, activeOnly bool) ([]models.PlatformCategory, error)
	ListWithLocale(ctx context.Context, activeOnly bool, requestedLocale string) ([]models.PlatformCategory, error)
	ListWithLocaleMaxLevel(ctx context.Context, activeOnly bool, requestedLocale string, maxLevel int) ([]models.PlatformCategory, error)
	ListChildren(ctx context.Context, parentID *uint, activeOnly bool) ([]models.PlatformCategory, error)
	ListChildrenWithLocale(ctx context.Context, parentID *uint, activeOnly bool, requestedLocale string) ([]models.PlatformCategory, error)
	Search(ctx context.Context, keyword string, limit int, activeOnly bool, requestedLocale string) ([]models.PlatformCategory, error)
	Update(ctx context.Context, id uint, input service.PlatformCategoryInput) (*models.PlatformCategory, error)
	Delete(ctx context.Context, id uint) error
	PreviewImport(ctx context.Context, reader io.Reader) (*service.PlatformCategoryImportPreview, error)
	ConfirmImport(ctx context.Context, reader io.Reader) (*service.PlatformCategoryImportResult, *service.PlatformCategoryImportPreview, error)
	ExportBilingualImportWorkbook(ctx context.Context) ([]byte, error)
	RebuildSearchIndex(ctx context.Context) (repository.PlatformCategorySearchMaintenanceSummary, error)
}

type platformCategoryPayload struct {
	ParentID     *uint                                `json:"parent_id"`
	Name         string                               `json:"name" binding:"required"`
	SortOrder    int                                  `json:"sort_order"`
	Status       models.PlatformCategoryStatus        `json:"status"`
	Translations *platformCategoryTranslationsPayload `json:"translations"`
}

type platformCategoryTranslationsPayload struct {
	Zh service.PlatformCategoryTranslationInput `json:"zh"`
}

type platformCategoryResponse struct {
	ID           uint                                `json:"id"`
	ParentID     *uint                               `json:"parent_id"`
	Name         string                              `json:"name"`
	Path         string                              `json:"path"`
	ZhName       string                              `json:"zh_name"`
	ZhPath       string                              `json:"zh_path"`
	Level        int                                 `json:"level"`
	SortOrder    int                                 `json:"sort_order"`
	Status       models.PlatformCategoryStatus       `json:"status"`
	IsLeaf       bool                                `json:"is_leaf"`
	HasChildren  bool                                `json:"has_children"`
	Children     []platformCategoryResponse          `json:"children,omitempty"`
	Translations platformCategoryTranslationsPayload `json:"translations"`
	CreatedAt    time.Time                           `json:"created_at"`
	UpdatedAt    time.Time                           `json:"updated_at"`
}

const maxPlatformCategoryImportFileBytes = 10 << 20

func NewPlatformCategoryController(service platformCategoryService) *PlatformCategoryController {
	return &PlatformCategoryController{service: service}
}

func (ctl *PlatformCategoryController) RegisterSuperRoutes(group *gin.RouterGroup) {
	categories := group.Group("/platform-categories")
	categories.GET("", ctl.ListAll)
	categories.GET("/search", ctl.SearchAll)
	categories.GET("/import-template", ctl.BilingualImportTemplate)
	categories.POST("/import", ctl.ImportDirect)
	categories.GET("/export", ctl.ExportBilingual)
	categories.POST("/rebuild-search-index", ctl.RebuildSearchIndex)
	categories.GET("/import/template", ctl.ImportTemplate)
	categories.POST("/import/preview", ctl.ImportPreview)
	categories.POST("/import/confirm", ctl.ImportConfirm)
	categories.POST("", ctl.Create)
	categories.GET("/:id", ctl.GetByID)
	categories.PUT("/:id", ctl.Update)
	categories.DELETE("/:id", ctl.Delete)
}

func (ctl *PlatformCategoryController) BilingualImportTemplate(c *gin.Context) {
	content, err := service.BuildPlatformCategoryPathImportTemplate()
	if err != nil {
		writeError(c, err)
		return
	}
	writePlatformCategoryWorkbook(c, "platform-category-bilingual-template.xlsx", content)
}

func (ctl *PlatformCategoryController) ImportTemplate(c *gin.Context) {
	format := strings.ToLower(strings.TrimSpace(c.Query("format")))
	var (
		content  []byte
		filename string
		err      error
	)
	switch format {
	case "", "levels", "multi":
		content, err = service.BuildPlatformCategoryImportTemplate()
		filename = "platform-category-import-template-levels.xlsx"
	case "path":
		content, err = service.BuildPlatformCategoryPathImportTemplate()
		filename = "platform-category-import-template-path.xlsx"
	default:
		c.JSON(http.StatusBadRequest, response{Error: "模板格式仅支持 levels 或 path"})
		return
	}
	if err != nil {
		writeError(c, err)
		return
	}
	writePlatformCategoryWorkbook(c, filename, content)
}

func (ctl *PlatformCategoryController) ImportPreview(c *gin.Context) {
	file, ok := openPlatformCategoryImportFile(c)
	if !ok {
		return
	}
	defer func() { _ = file.Close() }()

	preview, err := ctl.service.PreviewImport(c.Request.Context(), file)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: preview})
}

func (ctl *PlatformCategoryController) ImportConfirm(c *gin.Context) {
	file, ok := openPlatformCategoryImportFile(c)
	if !ok {
		return
	}
	defer func() { _ = file.Close() }()

	result, preview, err := ctl.service.ConfirmImport(c.Request.Context(), file)
	if err != nil {
		if preview != nil {
			errorMessage := "导入校验失败"
			errorMessageEN := ""
			if len(preview.Errors) > 0 {
				errorMessage = preview.Errors[0].Message
				errorMessageEN = preview.Errors[0].MessageEN
			}
			payload := gin.H{"data": preview, "error": errorMessage}
			if errorMessageEN != "" {
				payload["error_en"] = errorMessageEN
			}
			c.JSON(http.StatusBadRequest, payload)
			return
		}
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: result})
}

func (ctl *PlatformCategoryController) ImportDirect(c *gin.Context) {
	ctl.ImportConfirm(c)
}

func (ctl *PlatformCategoryController) ExportBilingual(c *gin.Context) {
	content, err := ctl.service.ExportBilingualImportWorkbook(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	writePlatformCategoryWorkbook(c, "platform-category-bilingual-export.xlsx", content)
}

func (ctl *PlatformCategoryController) RebuildSearchIndex(c *gin.Context) {
	summary, err := ctl.service.RebuildSearchIndex(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: summary})
}

func writePlatformCategoryWorkbook(c *gin.Context, filename string, content []byte) {
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", content)
}

func openPlatformCategoryImportFile(c *gin.Context) (multipartFile, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPlatformCategoryImportFileBytes)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, response{Error: "请上传 xlsx 文件"})
		return nil, false
	}
	if fileHeader.Size > maxPlatformCategoryImportFileBytes {
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

func (ctl *PlatformCategoryController) RegisterAdminRoutes(group *gin.RouterGroup) {
	group.GET("/admin/platform-categories", ctl.ListActive)
	group.GET("/admin/platform-categories/search", ctl.SearchActive)
	group.GET("/admin/platform-categories/:id", ctl.GetByID)
}

func (ctl *PlatformCategoryController) RegisterPublicRoutes(group *gin.RouterGroup) {
	group.GET("/storefront/platform-categories", ctl.PublicListActive)
}

func (ctl *PlatformCategoryController) ListAll(c *gin.Context) {
	if strings.TrimSpace(c.Query("keyword")) != "" {
		ctl.search(c, false)
		return
	}
	if _, exists := c.GetQuery("parent_id"); exists {
		ctl.listChildren(c, false)
		return
	}
	ctl.list(c, false)
}

func (ctl *PlatformCategoryController) ListActive(c *gin.Context) {
	if strings.TrimSpace(c.Query("keyword")) != "" {
		ctl.search(c, true)
		return
	}
	if _, exists := c.GetQuery("parent_id"); exists {
		ctl.listChildren(c, true)
		return
	}
	ctl.list(c, true)
}

func (ctl *PlatformCategoryController) PublicListActive(c *gin.Context) {
	if _, exists := c.GetQuery("parent_id"); exists {
		ctl.publicListChildren(c)
		return
	}
	ctl.publicList(c)
}

func (ctl *PlatformCategoryController) SearchAll(c *gin.Context) {
	ctl.search(c, false)
}

func (ctl *PlatformCategoryController) SearchActive(c *gin.Context) {
	ctl.search(c, true)
}

func (ctl *PlatformCategoryController) GetByID(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	category, err := ctl.service.FindByIDWithBilingualDisplay(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	translations, err := ctl.platformCategoryTranslations(c, category.ID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: toPlatformCategoryResponse(category, translations)})
}

func (ctl *PlatformCategoryController) Create(c *gin.Context) {
	var payload platformCategoryPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	category, err := ctl.service.Create(c.Request.Context(), payload.toInput())
	if err != nil {
		writeError(c, err)
		return
	}
	translations, err := ctl.platformCategoryTranslations(c, category.ID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, response{Data: toPlatformCategoryResponse(category, translations)})
}

func (ctl *PlatformCategoryController) Update(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var payload platformCategoryPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	category, err := ctl.service.Update(c.Request.Context(), id, payload.toInput())
	if err != nil {
		writeError(c, err)
		return
	}
	translations, err := ctl.platformCategoryTranslations(c, category.ID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: toPlatformCategoryResponse(category, translations)})
}

func (ctl *PlatformCategoryController) Delete(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := ctl.service.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, service.ErrPlatformCategoryHasProducts) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":    err.Error(),
				"error_en": service.PlatformCategoryHasProductsMessageEN,
			})
			return
		}
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (ctl *PlatformCategoryController) list(c *gin.Context, activeOnly bool) {
	categories, err := ctl.service.List(c.Request.Context(), activeOnly)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: buildPlatformCategoryTree(categories)})
}

func (ctl *PlatformCategoryController) listChildren(c *gin.Context, activeOnly bool) {
	parentID, ok := parsePlatformCategoryParentIDQuery(c)
	if !ok {
		return
	}
	categories, err := ctl.service.ListChildren(c.Request.Context(), parentID, activeOnly)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: toPlatformCategoryResponses(categories)})
}

func (ctl *PlatformCategoryController) publicList(c *gin.Context) {
	maxLevel, ok := parsePlatformCategoryMaxLevelQuery(c)
	if !ok {
		return
	}
	categories, err := ctl.service.ListWithLocaleMaxLevel(
		c.Request.Context(),
		true,
		locale.NormalizeLocale(c.Query("locale")),
		maxLevel,
	)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: buildPlatformCategoryTree(categories)})
}

func (ctl *PlatformCategoryController) publicListChildren(c *gin.Context) {
	parentID, ok := parsePlatformCategoryParentIDQuery(c)
	if !ok {
		return
	}
	maxLevel, ok := parsePlatformCategoryMaxLevelQuery(c)
	if !ok {
		return
	}
	categories, err := ctl.service.ListChildrenWithLocale(c.Request.Context(), parentID, true, locale.NormalizeLocale(c.Query("locale")))
	if err != nil {
		writeError(c, err)
		return
	}
	if maxLevel > 0 {
		categories = filterPlatformCategoriesByMaxLevel(categories, maxLevel)
	}
	c.JSON(http.StatusOK, response{Data: toPlatformCategoryResponses(categories)})
}

func (ctl *PlatformCategoryController) search(c *gin.Context, activeOnly bool) {
	limit := service.PlatformCategorySearchDefaultLimit
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, response{Error: "limit 必须为正整数"})
			return
		}
		limit = parsed
	}
	if limit > service.PlatformCategorySearchMaxLimit {
		limit = service.PlatformCategorySearchMaxLimit
	}
	categories, err := ctl.service.Search(c.Request.Context(), c.Query("keyword"), limit, activeOnly, c.Query("locale"))
	if err != nil {
		writeError(c, err)
		return
	}
	if platformCategoryTruthyQuery(c.Query("leaf_only")) {
		categories = filterLeafPlatformCategories(categories)
	}
	c.JSON(http.StatusOK, response{Data: toPlatformCategoryResponses(categories)})
}

func parsePlatformCategoryMaxLevelQuery(c *gin.Context) (int, bool) {
	rawMaxLevel := strings.TrimSpace(c.Query("max_level"))
	if rawMaxLevel == "" {
		return 0, true
	}
	parsed, err := strconv.Atoi(rawMaxLevel)
	if err != nil || parsed <= 0 {
		c.JSON(http.StatusBadRequest, response{Error: "max_level 必须为正整数"})
		return 0, false
	}
	return parsed, true
}

func platformCategoryTruthyQuery(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "1" || value == "true" || value == "yes"
}

func filterPlatformCategoriesByMaxLevel(categories []models.PlatformCategory, maxLevel int) []models.PlatformCategory {
	if maxLevel <= 0 {
		return categories
	}
	filtered := make([]models.PlatformCategory, 0, len(categories))
	for _, category := range categories {
		if category.Level <= maxLevel {
			filtered = append(filtered, category)
		}
	}
	return filtered
}

func filterLeafPlatformCategories(categories []models.PlatformCategory) []models.PlatformCategory {
	filtered := make([]models.PlatformCategory, 0, len(categories))
	for _, category := range categories {
		if category.IsLeaf {
			filtered = append(filtered, category)
		}
	}
	return filtered
}

func parsePlatformCategoryParentIDQuery(c *gin.Context) (*uint, bool) {
	rawParentID := strings.TrimSpace(c.Query("parent_id"))
	if rawParentID == "" || rawParentID == "0" {
		return nil, true
	}
	parsed, err := strconv.ParseUint(rawParentID, 10, 64)
	if err != nil || parsed == 0 {
		c.JSON(http.StatusBadRequest, response{Error: "parent_id 必须为非负整数"})
		return nil, false
	}
	parentID := uint(parsed)
	return &parentID, true
}

func (p platformCategoryPayload) toInput() service.PlatformCategoryInput {
	parentID := p.ParentID
	if parentID != nil && *parentID == 0 {
		parentID = nil
	}
	return service.PlatformCategoryInput{
		ParentID:     parentID,
		Name:         p.Name,
		SortOrder:    p.SortOrder,
		Status:       p.Status,
		Translations: p.toTranslationInput(),
	}
}

func (p platformCategoryPayload) toTranslationInput() *service.PlatformCategoryTranslationInput {
	if p.Translations == nil {
		return nil
	}
	return &service.PlatformCategoryTranslationInput{Name: p.Translations.Zh.Name}
}

func (ctl *PlatformCategoryController) platformCategoryTranslations(c *gin.Context, id uint) (service.PlatformCategoryTranslationInput, error) {
	return ctl.service.TranslationFields(c.Request.Context(), id, "zh")
}

func toPlatformCategoryResponse(category *models.PlatformCategory, translations service.PlatformCategoryTranslationInput) platformCategoryResponse {
	if category == nil {
		return platformCategoryResponse{}
	}
	hasChildren := !category.IsLeaf
	if category.HasChildren != nil {
		hasChildren = *category.HasChildren
	}
	return platformCategoryResponse{
		ID:          category.ID,
		ParentID:    category.ParentID,
		Name:        category.Name,
		Path:        category.Path,
		ZhName:      platformCategoryResponseZhName(category, translations),
		ZhPath:      platformCategoryResponseZhPath(category),
		Level:       category.Level,
		SortOrder:   category.SortOrder,
		Status:      category.Status,
		IsLeaf:      category.IsLeaf,
		HasChildren: hasChildren,
		Translations: platformCategoryTranslationsPayload{
			Zh: translations,
		},
		CreatedAt: category.CreatedAt,
		UpdatedAt: category.UpdatedAt,
	}
}

func platformCategoryResponseZhName(category *models.PlatformCategory, translations service.PlatformCategoryTranslationInput) string {
	if category == nil {
		return ""
	}
	if value := strings.TrimSpace(category.ZhName); value != "" {
		return value
	}
	if value := strings.TrimSpace(translations.Name); value != "" {
		return value
	}
	return strings.TrimSpace(category.Name)
}

func platformCategoryResponseZhPath(category *models.PlatformCategory) string {
	if category == nil {
		return ""
	}
	if value := strings.TrimSpace(category.ZhPath); value != "" {
		return value
	}
	return strings.ReplaceAll(strings.TrimSpace(category.Path), "/", " > ")
}

func toPlatformCategoryResponses(categories []models.PlatformCategory) []platformCategoryResponse {
	items := make([]platformCategoryResponse, 0, len(categories))
	for i := range categories {
		items = append(items, toPlatformCategoryResponse(&categories[i], service.PlatformCategoryTranslationInput{}))
	}
	return items
}

func buildPlatformCategoryTree(categories []models.PlatformCategory) []platformCategoryResponse {
	childrenByParent := make(map[uint][]models.PlatformCategory, len(categories))
	for i := range categories {
		parentID := uint(0)
		if categories[i].ParentID != nil {
			parentID = *categories[i].ParentID
		}
		childrenByParent[parentID] = append(childrenByParent[parentID], categories[i])
	}

	var build func(parentID uint) []platformCategoryResponse
	build = func(parentID uint) []platformCategoryResponse {
		children := childrenByParent[parentID]
		items := make([]platformCategoryResponse, 0, len(children))
		for i := range children {
			child := children[i]
			item := toPlatformCategoryResponse(&child, service.PlatformCategoryTranslationInput{})
			item.Children = build(child.ID)
			items = append(items, item)
		}
		return items
	}
	return build(0)
}
