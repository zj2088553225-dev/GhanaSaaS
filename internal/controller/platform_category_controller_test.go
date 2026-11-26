package controller

import (
	"context"
	"encoding/json"
	"io"
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
)

type fakePlatformCategoryService struct {
	createCalls    int
	listActive     bool
	categories     []models.PlatformCategory
	listParentID   *uint
	searchKeyword  string
	searchLimit    int
	lastMaxLevel   int
	lastCreate     service.PlatformCategoryInput
	updateErr      error
	confirmResult  *service.PlatformCategoryImportResult
	confirmPreview *service.PlatformCategoryImportPreview
	confirmErr     error
	deleteCalls    int
	deleteErr      error
	rebuildCalls   int
}

func (s *fakePlatformCategoryService) Create(_ context.Context, input service.PlatformCategoryInput) (*models.PlatformCategory, error) {
	s.createCalls++
	s.lastCreate = input
	return &models.PlatformCategory{
		BaseModel: models.BaseModel{ID: 10},
		ParentID:  input.ParentID,
		Name:      input.Name,
		Path:      input.Name,
		Level:     1,
		SortOrder: input.SortOrder,
		Status:    models.PlatformCategoryStatusActive,
		IsLeaf:    true,
	}, nil
}

func (s *fakePlatformCategoryService) FindByID(_ context.Context, id uint) (*models.PlatformCategory, error) {
	for i := range s.categories {
		if s.categories[i].ID == id {
			category := s.categories[i]
			return &category, nil
		}
	}
	return &models.PlatformCategory{BaseModel: models.BaseModel{ID: id}}, nil
}

func (s *fakePlatformCategoryService) FindByIDWithBilingualDisplay(ctx context.Context, id uint) (*models.PlatformCategory, error) {
	return s.FindByID(ctx, id)
}

func (s *fakePlatformCategoryService) TranslationFields(_ context.Context, _ uint, _ string) (service.PlatformCategoryTranslationInput, error) {
	return service.PlatformCategoryTranslationInput{}, nil
}

func (s *fakePlatformCategoryService) List(_ context.Context, activeOnly bool) ([]models.PlatformCategory, error) {
	s.listActive = activeOnly
	return append([]models.PlatformCategory(nil), s.categories...), nil
}

func (s *fakePlatformCategoryService) ListWithLocale(ctx context.Context, activeOnly bool, _ string) ([]models.PlatformCategory, error) {
	return s.List(ctx, activeOnly)
}

func (s *fakePlatformCategoryService) ListWithLocaleMaxLevel(ctx context.Context, activeOnly bool, _ string, maxLevel int) ([]models.PlatformCategory, error) {
	s.lastMaxLevel = maxLevel
	categories, err := s.List(ctx, activeOnly)
	if err != nil || maxLevel <= 0 {
		return categories, err
	}
	items := make([]models.PlatformCategory, 0, len(categories))
	for _, category := range categories {
		if category.Level <= maxLevel {
			items = append(items, category)
		}
	}
	return items, nil
}

func (s *fakePlatformCategoryService) ListChildren(_ context.Context, parentID *uint, activeOnly bool) ([]models.PlatformCategory, error) {
	s.listActive = activeOnly
	s.listParentID = parentID
	items := make([]models.PlatformCategory, 0)
	for _, category := range s.categories {
		if parentID == nil {
			if category.ParentID == nil {
				items = append(items, category)
			}
			continue
		}
		if category.ParentID != nil && *category.ParentID == *parentID {
			items = append(items, category)
		}
	}
	return items, nil
}

func (s *fakePlatformCategoryService) ListChildrenWithLocale(ctx context.Context, parentID *uint, activeOnly bool, _ string) ([]models.PlatformCategory, error) {
	return s.ListChildren(ctx, parentID, activeOnly)
}

func (s *fakePlatformCategoryService) Search(_ context.Context, keyword string, limit int, activeOnly bool, _ string) ([]models.PlatformCategory, error) {
	s.listActive = activeOnly
	s.searchKeyword = keyword
	s.searchLimit = limit
	if len(s.categories) > limit {
		return append([]models.PlatformCategory(nil), s.categories[:limit]...), nil
	}
	return append([]models.PlatformCategory(nil), s.categories...), nil
}

func (s *fakePlatformCategoryService) Update(_ context.Context, id uint, input service.PlatformCategoryInput) (*models.PlatformCategory, error) {
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	return &models.PlatformCategory{BaseModel: models.BaseModel{ID: id}, Name: input.Name}, nil
}

func (s *fakePlatformCategoryService) Delete(_ context.Context, _ uint) error {
	s.deleteCalls++
	return s.deleteErr
}

func (s *fakePlatformCategoryService) PreviewImport(_ context.Context, _ io.Reader) (*service.PlatformCategoryImportPreview, error) {
	return &service.PlatformCategoryImportPreview{}, nil
}

func (s *fakePlatformCategoryService) ConfirmImport(_ context.Context, _ io.Reader) (*service.PlatformCategoryImportResult, *service.PlatformCategoryImportPreview, error) {
	if s.confirmResult != nil || s.confirmPreview != nil || s.confirmErr != nil {
		return s.confirmResult, s.confirmPreview, s.confirmErr
	}
	return &service.PlatformCategoryImportResult{}, &service.PlatformCategoryImportPreview{}, nil
}

func (s *fakePlatformCategoryService) ExportBilingualImportWorkbook(_ context.Context) ([]byte, error) {
	return service.BuildPlatformCategoryPathImportTemplate()
}

func (s *fakePlatformCategoryService) RebuildSearchIndex(_ context.Context) (repository.PlatformCategorySearchMaintenanceSummary, error) {
	s.rebuildCalls++
	return repository.PlatformCategorySearchMaintenanceSummary{Verified: true}, nil
}

func newPlatformCategorySuperRouter(platformService *fakePlatformCategoryService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/super")
	group.Use(requireControllerTestAuth("secret"))
	group.Use(middleware.RequireRole(models.UserRoleSuperAdmin))
	NewPlatformCategoryController(platformService).RegisterSuperRoutes(group)
	return router
}

func newPlatformCategoryAdminRouter(platformService *fakePlatformCategoryService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1")
	group.Use(requireControllerTestAuth("secret"))
	group.Use(middleware.RequireRole(models.UserRoleSuperAdmin, models.UserRoleTenantAdmin))
	NewPlatformCategoryController(platformService).RegisterAdminRoutes(group)
	return router
}

func TestPlatformCategorySuperCreateAllowsSuperAdmin(t *testing.T) {
	superUser := &models.User{BaseModel: models.BaseModel{ID: 1}, Role: models.UserRoleSuperAdmin}
	token, err := auth.GenerateToken("secret", time.Hour, superUser)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	platformService := &fakePlatformCategoryService{}
	router := newPlatformCategorySuperRouter(platformService)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/super/platform-categories", strings.NewReader(`{"name":"食品饮料"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if platformService.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", platformService.createCalls)
	}
}

func TestPlatformCategoryImportTemplateRequiresSuperAdmin(t *testing.T) {
	superUser := &models.User{BaseModel: models.BaseModel{ID: 1}, Role: models.UserRoleSuperAdmin}
	superToken, err := auth.GenerateToken("secret", time.Hour, superUser)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	router := newPlatformCategorySuperRouter(&fakePlatformCategoryService{})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/super/platform-categories/import/template", nil)
	request.Header.Set("Authorization", "Bearer "+superToken)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("super admin status = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Header().Get("Content-Type"), "spreadsheetml") {
		t.Fatalf("content type = %q, want xlsx", response.Header().Get("Content-Type"))
	}
	if !strings.Contains(response.Header().Get("Content-Disposition"), "levels") {
		t.Fatalf("content disposition = %q, want default levels template", response.Header().Get("Content-Disposition"))
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/super/platform-categories/import/template?format=path", nil)
	request.Header.Set("Authorization", "Bearer "+superToken)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Header().Get("Content-Disposition"), "path") {
		t.Fatalf("path template status = %d, disposition = %q", response.Code, response.Header().Get("Content-Disposition"))
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/super/platform-categories/import-template", nil)
	request.Header.Set("Authorization", "Bearer "+superToken)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Header().Get("Content-Disposition"), "bilingual-template") {
		t.Fatalf("bilingual template status = %d, disposition = %q", response.Code, response.Header().Get("Content-Disposition"))
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/super/platform-categories/export", nil)
	request.Header.Set("Authorization", "Bearer "+superToken)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Header().Get("Content-Disposition"), "bilingual-export") {
		t.Fatalf("bilingual export status = %d, disposition = %q", response.Code, response.Header().Get("Content-Disposition"))
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/super/platform-categories/rebuild-search-index", nil)
	request.Header.Set("Authorization", "Bearer "+superToken)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("super admin rebuild status = %d, want %d", response.Code, http.StatusOK)
	}

	tenantID := uint(1001)
	tenantUser := &models.User{BaseModel: models.BaseModel{ID: 2}, TenantID: &tenantID, Role: models.UserRoleTenantAdmin}
	tenantToken, err := auth.GenerateToken("secret", time.Hour, tenantUser)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/super/platform-categories/import/template", nil)
	request.Header.Set("Authorization", "Bearer "+tenantToken)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("tenant admin status = %d, want %d", response.Code, http.StatusForbidden)
	}

	body, contentType := multipartBody(t, "file", "categories.xlsx", []byte("xlsx"))
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/super/platform-categories/import", body)
	request.Header.Set("Authorization", "Bearer "+tenantToken)
	request.Header.Set("Content-Type", contentType)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("tenant admin import status = %d, want %d", response.Code, http.StatusForbidden)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/super/platform-categories/rebuild-search-index", nil)
	request.Header.Set("Authorization", "Bearer "+tenantToken)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("tenant admin rebuild status = %d, want %d", response.Code, http.StatusForbidden)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/super/platform-categories/search?keyword=食品", nil)
	request.Header.Set("Authorization", "Bearer "+tenantToken)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("tenant admin super search status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestPlatformCategoryImportConfirmReturnsLocalizedRowLimitError(t *testing.T) {
	superUser := &models.User{BaseModel: models.BaseModel{ID: 1}, Role: models.UserRoleSuperAdmin}
	token, err := auth.GenerateToken("secret", time.Hour, superUser)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	platformService := &fakePlatformCategoryService{
		confirmPreview: &service.PlatformCategoryImportPreview{
			Errors: []service.PlatformCategoryImportRowError{{
				Row:       service.PlatformCategoryImportMaxRows + 2,
				Field:     "行数",
				Message:   service.PlatformCategoryImportTooManyRowsMessageZH,
				MessageEN: service.PlatformCategoryImportTooManyRowsMessageEN,
			}},
			InvalidRows: 1,
		},
		confirmErr: service.ErrInvalidInput,
	}
	router := newPlatformCategorySuperRouter(platformService)
	body, contentType := multipartBody(t, "file", "categories.xlsx", []byte("xlsx"))
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/super/platform-categories/import/confirm", body)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", contentType)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	var payload struct {
		Error   string `json:"error"`
		ErrorEN string `json:"error_en"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload.Error != service.PlatformCategoryImportTooManyRowsMessageZH ||
		payload.ErrorEN != service.PlatformCategoryImportTooManyRowsMessageEN {
		t.Fatalf("localized errors = %+v", payload)
	}
}

func TestPlatformCategorySuperCreateIgnoresLeafFlagInRequestBody(t *testing.T) {
	superUser := &models.User{BaseModel: models.BaseModel{ID: 1}, Role: models.UserRoleSuperAdmin}
	token, err := auth.GenerateToken("secret", time.Hour, superUser)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	platformService := &fakePlatformCategoryService{}
	router := newPlatformCategorySuperRouter(platformService)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/super/platform-categories", strings.NewReader(`{"name":"食品饮料","is_leaf":false}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if platformService.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", platformService.createCalls)
	}
	if platformService.lastCreate.Name != "食品饮料" {
		t.Fatalf("last create input = %+v, want name from request and no leaf control", platformService.lastCreate)
	}
	var payload struct {
		Data platformCategoryResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if !payload.Data.IsLeaf {
		t.Fatalf("response IsLeaf = false, want service-maintained true")
	}
}

func TestPlatformCategorySuperCreateTreatsZeroParentAsRoot(t *testing.T) {
	superUser := &models.User{BaseModel: models.BaseModel{ID: 1}, Role: models.UserRoleSuperAdmin}
	token, err := auth.GenerateToken("secret", time.Hour, superUser)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	platformService := &fakePlatformCategoryService{}
	router := newPlatformCategorySuperRouter(platformService)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/super/platform-categories", strings.NewReader(`{"parent_id":0,"name":"女装"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if platformService.lastCreate.ParentID != nil {
		t.Fatalf("last create parent_id = %v, want nil root parent", *platformService.lastCreate.ParentID)
	}
}

func TestPlatformCategorySuperListAndSearchReturnBilingualDisplay(t *testing.T) {
	superUser := &models.User{BaseModel: models.BaseModel{ID: 1}, Role: models.UserRoleSuperAdmin}
	token, err := auth.GenerateToken("secret", time.Hour, superUser)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	platformService := &fakePlatformCategoryService{
		categories: []models.PlatformCategory{
			{
				BaseModel: models.BaseModel{ID: 1},
				Name:      "Electronics",
				Path:      "Electronics",
				ZhName:    "电子产品",
				ZhPath:    "电子产品",
				Status:    models.PlatformCategoryStatusActive,
			},
			{
				BaseModel: models.BaseModel{ID: 2},
				ParentID:  controllerUintPtr(1),
				Name:      "Mobile Accessories",
				Path:      "Electronics/Mobile Accessories",
				ZhName:    "手机配件",
				ZhPath:    "电子产品 > 手机配件",
				Status:    models.PlatformCategoryStatusActive,
				IsLeaf:    true,
			},
		},
	}
	router := newPlatformCategorySuperRouter(platformService)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/super/platform-categories?parent_id=0", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	var listPayload struct {
		Data []platformCategoryResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("Unmarshal list returned error: %v", err)
	}
	if len(listPayload.Data) != 1 || listPayload.Data[0].ZhName != "电子产品" || listPayload.Data[0].ZhPath != "电子产品" {
		t.Fatalf("list payload = %+v, want bilingual display fields", listPayload.Data)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/super/platform-categories/search?keyword=Mobile&locale=all", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("search status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	var searchPayload struct {
		Data []platformCategoryResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &searchPayload); err != nil {
		t.Fatalf("Unmarshal search returned error: %v", err)
	}
	if len(searchPayload.Data) < 2 || searchPayload.Data[1].ZhName != "手机配件" || searchPayload.Data[1].ZhPath != "电子产品 > 手机配件" {
		t.Fatalf("search payload = %+v, want bilingual display fields", searchPayload.Data)
	}
}

func TestPlatformCategorySuperUpdateNotFoundMessage(t *testing.T) {
	superUser := &models.User{BaseModel: models.BaseModel{ID: 1}, Role: models.UserRoleSuperAdmin}
	token, err := auth.GenerateToken("secret", time.Hour, superUser)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	platformService := &fakePlatformCategoryService{updateErr: service.ErrPlatformCategoryNotFound}
	router := newPlatformCategorySuperRouter(platformService)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/super/platform-categories/12", strings.NewReader(`{"name":"女装"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusNotFound, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "类目不存在，请刷新后重试") {
		t.Fatalf("body = %s, want refresh message", response.Body.String())
	}
}

func TestPlatformCategorySuperCreateRejectsTenantAdmin(t *testing.T) {
	tenantID := uint(1001)
	tenantUser := &models.User{BaseModel: models.BaseModel{ID: 1}, TenantID: &tenantID, Role: models.UserRoleTenantAdmin}
	token, err := auth.GenerateToken("secret", time.Hour, tenantUser)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	platformService := &fakePlatformCategoryService{}
	router := newPlatformCategorySuperRouter(platformService)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/super/platform-categories", strings.NewReader(`{"name":"食品饮料"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if platformService.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", platformService.createCalls)
	}
}

func TestPlatformCategorySuperDeleteAllowsSuperAdmin(t *testing.T) {
	superUser := &models.User{BaseModel: models.BaseModel{ID: 1}, Role: models.UserRoleSuperAdmin}
	token, err := auth.GenerateToken("secret", time.Hour, superUser)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	platformService := &fakePlatformCategoryService{}
	router := newPlatformCategorySuperRouter(platformService)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/super/platform-categories/12", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusNoContent, response.Body.String())
	}
	if platformService.deleteCalls != 1 {
		t.Fatalf("delete calls = %d, want 1", platformService.deleteCalls)
	}
}

func TestPlatformCategorySuperDeleteReturnsProductBindingMessages(t *testing.T) {
	superUser := &models.User{BaseModel: models.BaseModel{ID: 1}, Role: models.UserRoleSuperAdmin}
	token, err := auth.GenerateToken("secret", time.Hour, superUser)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	platformService := &fakePlatformCategoryService{deleteErr: service.ErrPlatformCategoryHasProducts}
	router := newPlatformCategorySuperRouter(platformService)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/super/platform-categories/12", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	var payload struct {
		Error   string `json:"error"`
		ErrorEN string `json:"error_en"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload.Error != service.ErrPlatformCategoryHasProducts.Error() ||
		payload.ErrorEN != service.PlatformCategoryHasProductsMessageEN {
		t.Fatalf("product binding errors = %+v", payload)
	}
}

func TestPlatformCategorySuperDeleteMissingReturnsNotFound(t *testing.T) {
	superUser := &models.User{BaseModel: models.BaseModel{ID: 1}, Role: models.UserRoleSuperAdmin}
	token, err := auth.GenerateToken("secret", time.Hour, superUser)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	platformService := &fakePlatformCategoryService{deleteErr: service.ErrPlatformCategoryNotFound}
	router := newPlatformCategorySuperRouter(platformService)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/super/platform-categories/9999", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusNotFound, response.Body.String())
	}
}

func TestPlatformCategorySuperDeleteRejectsTenantAdmin(t *testing.T) {
	tenantID := uint(1001)
	tenantUser := &models.User{BaseModel: models.BaseModel{ID: 1}, TenantID: &tenantID, Role: models.UserRoleTenantAdmin}
	token, err := auth.GenerateToken("secret", time.Hour, tenantUser)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	platformService := &fakePlatformCategoryService{}
	router := newPlatformCategorySuperRouter(platformService)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/super/platform-categories/12", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if platformService.deleteCalls != 0 {
		t.Fatalf("delete calls = %d, want 0", platformService.deleteCalls)
	}
}

func TestPlatformCategoryAdminListReturnsActiveTree(t *testing.T) {
	tenantID := uint(1001)
	tenantUser := &models.User{BaseModel: models.BaseModel{ID: 1}, TenantID: &tenantID, Role: models.UserRoleTenantAdmin}
	token, err := auth.GenerateToken("secret", time.Hour, tenantUser)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	platformService := &fakePlatformCategoryService{
		categories: []models.PlatformCategory{
			{BaseModel: models.BaseModel{ID: 1}, Name: "食品饮料", Path: "食品饮料", Level: 1, Status: models.PlatformCategoryStatusActive},
			{BaseModel: models.BaseModel{ID: 2}, ParentID: controllerUintPtr(1), Name: "饮料", Path: "食品饮料/饮料", Level: 2, Status: models.PlatformCategoryStatusActive, IsLeaf: true},
		},
	}
	router := newPlatformCategoryAdminRouter(platformService)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-categories", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if !platformService.listActive {
		t.Fatal("admin list should request active platform categories")
	}
	var payload struct {
		Data []platformCategoryResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if len(payload.Data) != 1 || len(payload.Data[0].Children) != 1 {
		t.Fatalf("tree = %+v, want one root with one child", payload.Data)
	}
}

func TestPlatformCategoryAdminLazyListReturnsOnlyDirectChildren(t *testing.T) {
	tenantID := uint(1001)
	tenantUser := &models.User{BaseModel: models.BaseModel{ID: 1}, TenantID: &tenantID, Role: models.UserRoleTenantAdmin}
	token, err := auth.GenerateToken("secret", time.Hour, tenantUser)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	platformService := &fakePlatformCategoryService{
		categories: []models.PlatformCategory{
			{BaseModel: models.BaseModel{ID: 1}, Name: "食品饮料", Path: "食品饮料", Level: 1, Status: models.PlatformCategoryStatusActive, IsLeaf: false},
			{BaseModel: models.BaseModel{ID: 2}, ParentID: controllerUintPtr(1), Name: "饮料", Path: "食品饮料/饮料", Level: 2, Status: models.PlatformCategoryStatusActive, IsLeaf: false},
			{BaseModel: models.BaseModel{ID: 3}, ParentID: controllerUintPtr(2), Name: "碳酸饮料", Path: "食品饮料/饮料/碳酸饮料", Level: 3, Status: models.PlatformCategoryStatusActive, IsLeaf: true},
		},
	}
	router := newPlatformCategoryAdminRouter(platformService)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-categories?parent_id=1", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if platformService.listParentID == nil || *platformService.listParentID != 1 {
		t.Fatalf("parent ID = %v, want 1", platformService.listParentID)
	}
	var payload struct {
		Data []platformCategoryResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if len(payload.Data) != 1 || payload.Data[0].ID != 2 {
		t.Fatalf("data = %+v, want only direct child ID 2", payload.Data)
	}
	if !payload.Data[0].HasChildren || len(payload.Data[0].Children) != 0 {
		t.Fatalf("lazy child = %+v, want has_children without recursive children", payload.Data[0])
	}
}

func TestPlatformCategoryAdminLazyRootAndSearchLimit(t *testing.T) {
	tenantID := uint(1001)
	tenantUser := &models.User{BaseModel: models.BaseModel{ID: 1}, TenantID: &tenantID, Role: models.UserRoleTenantAdmin}
	token, err := auth.GenerateToken("secret", time.Hour, tenantUser)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	platformService := &fakePlatformCategoryService{
		categories: []models.PlatformCategory{
			{BaseModel: models.BaseModel{ID: 1}, Name: "食品饮料", Path: "食品饮料", Level: 1, Status: models.PlatformCategoryStatusActive, IsLeaf: false},
			{BaseModel: models.BaseModel{ID: 2}, ParentID: controllerUintPtr(1), Name: "饮料", Path: "食品饮料/饮料", Level: 2, Status: models.PlatformCategoryStatusActive, IsLeaf: true},
		},
	}
	router := newPlatformCategoryAdminRouter(platformService)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-categories?parent_id=0", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("root status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if platformService.listParentID != nil {
		t.Fatalf("root parent ID = %v, want nil", platformService.listParentID)
	}
	var rootPayload struct {
		Data []platformCategoryResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &rootPayload); err != nil {
		t.Fatalf("Unmarshal root returned error: %v", err)
	}
	if len(rootPayload.Data) != 1 || rootPayload.Data[0].ID != 1 {
		t.Fatalf("root data = %+v, want only root ID 1", rootPayload.Data)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-categories/search?keyword=%E9%A5%AE%E6%96%99&limit=1000&leaf_only=true", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("search status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if platformService.searchKeyword != "饮料" || platformService.searchLimit != service.PlatformCategorySearchMaxLimit {
		t.Fatalf("search call keyword=%q limit=%d", platformService.searchKeyword, platformService.searchLimit)
	}
	var searchPayload struct {
		Data []platformCategoryResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &searchPayload); err != nil {
		t.Fatalf("Unmarshal search returned error: %v", err)
	}
	if len(searchPayload.Data) != 1 || searchPayload.Data[0].ID != 2 {
		t.Fatalf("search data = %+v, want only leaf category ID 2", searchPayload.Data)
	}
}

func TestPlatformCategoryPublicListReturnsActiveTreeWithoutAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	platformService := &fakePlatformCategoryService{
		categories: []models.PlatformCategory{
			{BaseModel: models.BaseModel{ID: 1}, Name: "Phone Parts", Path: "Phone Parts", Level: 1, Status: models.PlatformCategoryStatusActive},
			{BaseModel: models.BaseModel{ID: 2}, ParentID: controllerUintPtr(1), Name: "Screens", Path: "Phone Parts/Screens", Level: 2, Status: models.PlatformCategoryStatusActive, IsLeaf: true},
		},
	}
	router := gin.New()
	NewPlatformCategoryController(platformService).RegisterPublicRoutes(router.Group("/api/v1"))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/storefront/platform-categories", nil)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if !platformService.listActive {
		t.Fatal("public list should request active platform categories")
	}
	var payload struct {
		Data []platformCategoryResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if len(payload.Data) != 1 || len(payload.Data[0].Children) != 1 {
		t.Fatalf("tree = %+v, want one public root with one child", payload.Data)
	}
}

func TestPlatformCategoryPublicListHonorsMaxLevelWithoutAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	platformService := &fakePlatformCategoryService{
		categories: []models.PlatformCategory{
			{BaseModel: models.BaseModel{ID: 1}, Name: "Home Appliances", Path: "Home Appliances", Level: 1, Status: models.PlatformCategoryStatusActive, IsLeaf: false},
			{BaseModel: models.BaseModel{ID: 2}, ParentID: controllerUintPtr(1), Name: "Kitchen Appliances", Path: "Home Appliances/Kitchen Appliances", Level: 2, Status: models.PlatformCategoryStatusActive, IsLeaf: false},
			{BaseModel: models.BaseModel{ID: 3}, ParentID: controllerUintPtr(2), Name: "Rice Cookers", Path: "Home Appliances/Kitchen Appliances/Rice Cookers", Level: 3, Status: models.PlatformCategoryStatusActive, IsLeaf: true},
		},
	}
	router := gin.New()
	NewPlatformCategoryController(platformService).RegisterPublicRoutes(router.Group("/api/v1"))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/storefront/platform-categories?locale=zh&max_level=2", nil)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if platformService.lastMaxLevel != 2 {
		t.Fatalf("max level = %d, want 2", platformService.lastMaxLevel)
	}
	var payload struct {
		Data []platformCategoryResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if len(payload.Data) != 1 || len(payload.Data[0].Children) != 1 {
		t.Fatalf("tree = %+v, want level 1 and 2 only", payload.Data)
	}
	if len(payload.Data[0].Children[0].Children) != 0 {
		t.Fatalf("level 3 category leaked into public max_level tree: %+v", payload.Data[0].Children[0].Children)
	}
}

func controllerUintPtr(value uint) *uint {
	return &value
}
