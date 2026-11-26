package controller

import (
	"context"
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
	"gorm.io/gorm"
)

type noopShopProfileRepository struct{}

func (r *noopShopProfileRepository) FindByTenant(_ context.Context, _ uint) (*models.ShopProfile, error) {
	return nil, gorm.ErrRecordNotFound
}

func (r *noopShopProfileRepository) SaveAtomic(_ context.Context, _ uint, profile *models.ShopProfile, _ repository.ShopProfileTranslationMutation) (*models.ShopProfile, error) {
	return profile, nil
}

type recordingShopProfileRepository struct {
	savedTenantID uint
}

func (r *recordingShopProfileRepository) FindByTenant(_ context.Context, _ uint) (*models.ShopProfile, error) {
	return nil, gorm.ErrRecordNotFound
}

func (r *recordingShopProfileRepository) SaveAtomic(_ context.Context, tenantID uint, profile *models.ShopProfile, _ repository.ShopProfileTranslationMutation) (*models.ShopProfile, error) {
	r.savedTenantID = tenantID
	saved := *profile
	saved.TenantID = tenantID
	return &saved, nil
}

func newShopProfileTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1")
	group.Use(requireControllerTestAuth("secret"))
	group.Use(middleware.RequireRole(models.UserRoleTenantAdmin))
	controller := NewShopProfileController(service.NewShopProfileService(&noopShopProfileRepository{}))
	controller.RegisterRoutes(group)
	return router
}

func newShopProfileTestRouterWithRecordingRepo(repo *recordingShopProfileRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1")
	group.Use(requireControllerTestAuth("secret"))
	group.Use(middleware.RequireRole(models.UserRoleTenantAdmin))
	controller := NewShopProfileController(service.NewShopProfileService(repo))
	controller.RegisterRoutes(group)
	return router
}

func TestShopProfileAdminRouteRejectsUnauthenticatedRequest(t *testing.T) {
	router := newShopProfileTestRouter()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/shop-profile", nil)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestShopProfileAdminRouteRejectsSuperAdmin(t *testing.T) {
	tenantID := uint(1001)
	user := &models.User{
		BaseModel: models.BaseModel{ID: 1},
		TenantID:  &tenantID,
		Role:      models.UserRoleSuperAdmin,
	}
	token, err := auth.GenerateToken("secret", time.Hour, user)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	router := newShopProfileTestRouter()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/shop-profile", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestShopProfileAdminRouteIgnoresTenantIDFromPayload(t *testing.T) {
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

	repo := &recordingShopProfileRepository{}
	router := newShopProfileTestRouterWithRecordingRepo(repo)
	response := httptest.NewRecorder()
	body := `{
		"tenant_id": 9999,
		"slug": "tenant-shop",
		"shop_name": "Tenant Shop",
		"is_public": true
	}`
	request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/shop-profile", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if repo.savedTenantID != tenantID {
		t.Fatalf("saved tenant id = %d, want JWT tenant id %d", repo.savedTenantID, tenantID)
	}
}
