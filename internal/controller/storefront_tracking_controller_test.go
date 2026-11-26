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
	"saas/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type noopStorefrontTrackingRepository struct {
	setting *models.StorefrontTrackingSetting
}

func (r *noopStorefrontTrackingRepository) Get(_ context.Context) (*models.StorefrontTrackingSetting, error) {
	if r.setting == nil {
		return nil, gorm.ErrRecordNotFound
	}
	cloned := *r.setting
	return &cloned, nil
}

func (r *noopStorefrontTrackingRepository) Save(
	_ context.Context,
	setting models.StorefrontTrackingSetting,
) (*models.StorefrontTrackingSetting, error) {
	setting.ID = 1
	r.setting = &setting
	cloned := setting
	return &cloned, nil
}

func newStorefrontTrackingTestRouter(repo *noopStorefrontTrackingRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	controller := NewStorefrontTrackingController(service.NewStorefrontTrackingService(repo))

	v1 := router.Group("/api/v1")
	controller.RegisterPublicRoutes(v1)

	super := v1.Group("/super")
	super.Use(requireControllerTestAuth("secret"))
	super.Use(middleware.RequireRole(models.UserRoleSuperAdmin))
	controller.RegisterSuperRoutes(super)
	return router
}

func TestStorefrontTrackingSuperRoutesRejectTenantAdmin(t *testing.T) {
	router := newStorefrontTrackingTestRouter(&noopStorefrontTrackingRepository{})
	tenantID := uint(1001)
	token, err := auth.GenerateToken("secret", time.Hour, &models.User{
		BaseModel: models.BaseModel{ID: 1},
		TenantID:  &tenantID,
		Role:      models.UserRoleTenantAdmin,
	})
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/super/storefront-tracking",
		strings.NewReader(`{"meta_pixel_enabled":true,"meta_pixel_id":"123456789"}`),
	)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusForbidden, response.Body.String())
	}
}

func TestStorefrontTrackingSuperAdminCanSaveAndPublicCanRead(t *testing.T) {
	router := newStorefrontTrackingTestRouter(&noopStorefrontTrackingRepository{})
	token, err := auth.GenerateToken("secret", time.Hour, &models.User{
		BaseModel: models.BaseModel{ID: 1},
		Role:      models.UserRoleSuperAdmin,
	})
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	saveResponse := httptest.NewRecorder()
	saveRequest := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/super/storefront-tracking",
		strings.NewReader(`{
			"meta_pixel_enabled":true,
			"meta_pixel_id":"123456789",
			"custom_head_enabled":true,
			"custom_head_html":"<meta name=\"verification\" content=\"abc\">"
		}`),
	)
	saveRequest.Header.Set("Authorization", "Bearer "+token)
	saveRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(saveResponse, saveRequest)
	if saveResponse.Code != http.StatusOK {
		t.Fatalf("save status = %d, want %d, body = %s", saveResponse.Code, http.StatusOK, saveResponse.Body.String())
	}

	publicResponse := httptest.NewRecorder()
	publicRequest := httptest.NewRequest(http.MethodGet, "/api/v1/storefront/tracking", nil)
	router.ServeHTTP(publicResponse, publicRequest)
	if publicResponse.Code != http.StatusOK {
		t.Fatalf("public status = %d, want %d, body = %s", publicResponse.Code, http.StatusOK, publicResponse.Body.String())
	}
	for _, expected := range []string{"123456789", "verification", "custom_head_enabled"} {
		if !strings.Contains(publicResponse.Body.String(), expected) {
			t.Fatalf("public body = %s, want %q", publicResponse.Body.String(), expected)
		}
	}
}
