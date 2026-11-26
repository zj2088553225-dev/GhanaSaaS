package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"saas/internal/middleware"
	"saas/internal/models"
	"saas/internal/pkg/auth"
	"saas/internal/repository"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

type recordingAdminProductRepository struct {
	createdTenantID uint
	product         *models.SPU
	deleteErr       error
}

func (r *recordingAdminProductRepository) CreateAtomic(
	_ context.Context,
	tenantID uint,
	product *models.SPU,
	_ repository.ProductTranslationMutation,
	generateCodes repository.SKUCodeGenerator,
) error {
	r.createdTenantID = tenantID
	if generateCodes != nil {
		if err := generateCodes(func([]string) (map[string]struct{}, error) {
			return map[string]struct{}{}, nil
		}); err != nil {
			return err
		}
	}
	product.ID = 1
	for i := range product.SKUs {
		product.SKUs[i].ID = uint(i + 1)
		product.SKUs[i].ProductID = product.ID
		product.SKUs[i].IsActive = true
	}
	cloned := cloneAdminProductForControllerTest(*product)
	r.product = &cloned
	return nil
}

func (r *recordingAdminProductRepository) CreateManyAtomic(
	ctx context.Context,
	tenantID uint,
	products []models.SPU,
	generateCodes repository.SKUCodeGenerator,
) error {
	if generateCodes != nil {
		if err := generateCodes(func([]string) (map[string]struct{}, error) {
			return map[string]struct{}{}, nil
		}); err != nil {
			return err
		}
	}
	for i := range products {
		if err := r.CreateAtomic(ctx, tenantID, &products[i], repository.ProductTranslationMutation{}, nil); err != nil {
			return err
		}
	}
	return nil
}

func (r *recordingAdminProductRepository) FindByIDWithTenant(_ context.Context, tenantID uint, id uint) (*models.SPU, error) {
	if r.product == nil || r.product.ID != id || r.product.TenantID != tenantID {
		return nil, gorm.ErrRecordNotFound
	}
	cloned := cloneAdminProductForControllerTest(*r.product)
	return &cloned, nil
}

func (r *recordingAdminProductRepository) ListByTenant(_ context.Context, _ repository.AdminProductQuery) ([]models.SPU, int64, error) {
	return nil, 0, nil
}

func (r *recordingAdminProductRepository) FindCategoryByIDWithTenant(_ context.Context, tenantID uint, id uint) (*models.Category, error) {
	return &models.Category{BaseModel: models.BaseModel{ID: id}, TenantID: tenantID, Name: "Category"}, nil
}

func (r *recordingAdminProductRepository) FindPlatformCategoryByID(_ context.Context, id uint) (*models.PlatformCategory, error) {
	return &models.PlatformCategory{
		BaseModel: models.BaseModel{ID: id},
		Name:      "Platform Category",
		Path:      "食品饮料/饮料/碳酸饮料",
		Status:    models.PlatformCategoryStatusActive,
		IsLeaf:    true,
	}, nil
}

func (r *recordingAdminProductRepository) FindPlatformCategoriesByPaths(_ context.Context, _ []string) (map[string]models.PlatformCategory, error) {
	return map[string]models.PlatformCategory{}, nil
}

func (r *recordingAdminProductRepository) HasPlatformCategoryChildren(_ context.Context, _ uint) (bool, error) {
	return false, nil
}

func (r *recordingAdminProductRepository) FindCategoriesByName(_ context.Context, _ uint, _ []string) (map[string]models.Category, error) {
	return map[string]models.Category{}, nil
}

func (r *recordingAdminProductRepository) UpdateAtomic(
	_ context.Context,
	_ uint,
	_ *models.SPU,
	_ repository.ProductTranslationMutation,
	_ repository.SKUCodeGenerator,
) error {
	return nil
}

func (r *recordingAdminProductRepository) UpdateStatusWithTenant(_ context.Context, _ uint, _ uint, _ models.SPUStatus) error {
	return nil
}

func (r *recordingAdminProductRepository) DeleteWithTenant(_ context.Context, _ uint, _ uint) error {
	return r.deleteErr
}

func cloneAdminProductForControllerTest(product models.SPU) models.SPU {
	product.SliderImages = append([]byte(nil), product.SliderImages...)
	product.SKUs = append([]models.SKU(nil), product.SKUs...)
	return product
}

func controllerContainsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func newAdminProductControllerTestRouter(repo *recordingAdminProductRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1")
	group.Use(requireControllerTestAuth("secret"))
	group.Use(middleware.RequireRole(models.UserRoleTenantAdmin))
	NewAdminProductController(service.NewAdminProductService(repo)).RegisterRoutes(group)
	return router
}

func TestAdminProductCreateIgnoresTenantIDFromPayload(t *testing.T) {
	tenantID := uint(1001)
	user := &models.User{
		BaseModel: models.BaseModel{ID: 1},
		TenantID:  &tenantID,
		Role:      models.UserRoleTenantAdmin,
	}
	token, err := auth.GenerateToken("secret", time.Hour, user)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	repo := &recordingAdminProductRepository{}
	router := newAdminProductControllerTestRouter(repo)
	response := httptest.NewRecorder()
	body := `{
		"tenant_id": 9999,
		"platform_category_id": 5001,
		"category_id": 10,
		"name": "Self Operated Product",
		"description": "created by impersonated tenant admin",
		"detail_html": "<p>Long detail</p><img src=\"/uploads/detail.webp\" onerror=\"bad()\"><script>alert(1)</script>",
		"main_image": "https://example.com/product.jpg",
		"video_url": "/uploads/videos/2026/06/product.mp4",
		"material": "Cotton",
		"dimensions": "30 × 20 × 10 cm",
		"weight": "1.2 kg",
		"status": "off_shelf",
		"skus": [{
			"name": "Default SKU",
			"erp_sku_code": "SELF-001",
			"image_url": "/uploads/sku-blue.webp",
			"retail_price": 1000,
			"original_price": 1200,
			"sellable_stock": 5
		}]
	}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/products", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if repo.createdTenantID != tenantID {
		t.Fatalf("created tenant id = %d, want JWT tenant id %d", repo.createdTenantID, tenantID)
	}

	var payload struct {
		Data struct {
			TenantID   uint   `json:"tenant_id"`
			DetailHTML string `json:"detail_html"`
			VideoURL   string `json:"video_url"`
			Material   string `json:"material"`
			Dimensions string `json:"dimensions"`
			Weight     string `json:"weight"`
			SKUs       []struct {
				ERPSKUCode string `json:"erp_sku_code"`
				ImageURL   string `json:"image_url"`
			} `json:"skus"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload.Data.TenantID != tenantID {
		t.Fatalf("response tenant_id = %d, want JWT tenant id %d", payload.Data.TenantID, tenantID)
	}
	if strings.Contains(payload.Data.DetailHTML, "script") || strings.Contains(payload.Data.DetailHTML, "onerror") {
		t.Fatalf("response detail_html was not sanitized: %s", payload.Data.DetailHTML)
	}
	if payload.Data.VideoURL != "/uploads/videos/2026/06/product.mp4" {
		t.Fatalf("response video_url = %q", payload.Data.VideoURL)
	}
	if payload.Data.Material != "Cotton" || payload.Data.Dimensions != "30 × 20 × 10 cm" || payload.Data.Weight != "1.2 kg" {
		t.Fatalf("response product parameters = %q / %q / %q", payload.Data.Material, payload.Data.Dimensions, payload.Data.Weight)
	}
	if len(payload.Data.SKUs) != 1 || payload.Data.SKUs[0].ERPSKUCode == "" || payload.Data.SKUs[0].ERPSKUCode == "SELF-001" {
		t.Fatalf("response SKU code = %+v, want system-generated code", payload.Data.SKUs)
	}
	if payload.Data.SKUs[0].ImageURL != "/uploads/sku-blue.webp" {
		t.Fatalf("response SKU image_url = %q", payload.Data.SKUs[0].ImageURL)
	}
}

func TestAdminProductCreateAllowsMissingShopCategory(t *testing.T) {
	tenantID := uint(1001)
	user := &models.User{
		BaseModel: models.BaseModel{ID: 1},
		TenantID:  &tenantID,
		Role:      models.UserRoleTenantAdmin,
	}
	token, err := auth.GenerateToken("secret", time.Hour, user)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	repo := &recordingAdminProductRepository{}
	router := newAdminProductControllerTestRouter(repo)
	response := httptest.NewRecorder()
	body := `{
		"platform_category_id": 5001,
		"name": "Platform Category Only Product",
		"description": "created without shop category",
		"main_image": "https://example.com/product.jpg",
		"status": "off_shelf",
		"skus": [{
			"name": "Default SKU",
			"retail_price": 1000,
			"original_price": 1200,
			"sellable_stock": 5
		}]
	}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/products", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if repo.product == nil || repo.product.CategoryID != nil {
		t.Fatalf("category_id = %v, want nil", repo.product.CategoryID)
	}
}

func TestAdminProductDeleteReturnsFriendlyOrderRecordError(t *testing.T) {
	tenantID := uint(1001)
	user := &models.User{
		BaseModel: models.BaseModel{ID: 1},
		TenantID:  &tenantID,
		Role:      models.UserRoleTenantAdmin,
	}
	token, err := auth.GenerateToken("secret", time.Hour, user)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	repo := &recordingAdminProductRepository{deleteErr: repository.ErrProductHasOrderRecords}
	router := newAdminProductControllerTestRouter(repo)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/products/99", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusConflict, response.Body.String())
	}
	var payload struct {
		Error   string `json:"error"`
		ErrorEN string `json:"error_en"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload.Error != service.ErrAdminProductHasOrderRecords.Error() ||
		payload.ErrorEN != service.AdminProductHasOrderRecordsMessageEN {
		t.Fatalf("delete errors = %+v", payload)
	}
	if strings.Contains(response.Body.String(), "SQLSTATE") || strings.Contains(response.Body.String(), "fk_order_items_sku") {
		t.Fatalf("response leaked database error: %s", response.Body.String())
	}
}

func TestAdminProductDeleteDoesNotLeakUnexpectedDatabaseError(t *testing.T) {
	tenantID := uint(1001)
	user := &models.User{
		BaseModel: models.BaseModel{ID: 1},
		TenantID:  &tenantID,
		Role:      models.UserRoleTenantAdmin,
	}
	token, err := auth.GenerateToken("secret", time.Hour, user)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	repo := &recordingAdminProductRepository{deleteErr: errors.New("SQLSTATE 23503 other_constraint")}
	router := newAdminProductControllerTestRouter(repo)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/products/99", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if strings.Contains(response.Body.String(), "SQLSTATE") || strings.Contains(response.Body.String(), "other_constraint") {
		t.Fatalf("response leaked database error: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "商品删除失败，请稍后重试") {
		t.Fatalf("body = %s, want generic delete error", response.Body.String())
	}
}

func TestAdminProductImportTemplateReturnsXLSX(t *testing.T) {
	tenantID := uint(1001)
	user := &models.User{
		BaseModel: models.BaseModel{ID: 1},
		TenantID:  &tenantID,
		Role:      models.UserRoleTenantAdmin,
	}
	token, err := auth.GenerateToken("secret", time.Hour, user)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	router := newAdminProductControllerTestRouter(&recordingAdminProductRepository{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products/import/template", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("content-type = %q, want xlsx", response.Header().Get("Content-Type"))
	}
	file, err := excelize.OpenReader(bytes.NewReader(response.Body.Bytes()))
	if err != nil {
		t.Fatalf("OpenReader returned error: %v", err)
	}
	defer func() { _ = file.Close() }()
	if _, err := file.GetRows("商品导入模板"); err != nil {
		t.Fatalf("GetRows returned error: %v", err)
	}
	rows, err := file.GetRows("商品导入模板")
	if err != nil {
		t.Fatalf("GetRows returned error: %v", err)
	}
	if controllerContainsString(rows[0], "店铺分类名称") {
		t.Fatalf("template headers = %+v, should not contain 店铺分类名称", rows[0])
	}
}
