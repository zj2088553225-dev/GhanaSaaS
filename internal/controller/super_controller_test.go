package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"

	"saas/internal/middleware"
	"saas/internal/models"
	"saas/internal/pkg/auth"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type fakeSuperService struct {
	impersonateSuperAdminID uint
	impersonateTenantID     uint
	impersonateTenantIDs    []uint
	impersonateCalls        int
	impersonateResult       *service.AuthResult
	tenants                 []models.Tenant
}

func (s *fakeSuperService) CreateTenantWithAdmin(_ context.Context, _ service.CreateTenantWithAdminInput) (*service.TenantWithAdminResult, error) {
	return nil, service.ErrInvalidInput
}

func (s *fakeSuperService) ListTenants(_ context.Context) ([]models.Tenant, error) {
	return s.tenants, nil
}

func (s *fakeSuperService) GlobalDashboardSummary(_ context.Context) (*service.GlobalDashboardSummary, error) {
	return &service.GlobalDashboardSummary{}, nil
}

func (s *fakeSuperService) ImpersonateTenantAdmin(_ context.Context, superAdminID uint, tenantID uint) (*service.AuthResult, error) {
	s.impersonateCalls++
	s.impersonateSuperAdminID = superAdminID
	s.impersonateTenantID = tenantID
	s.impersonateTenantIDs = append(s.impersonateTenantIDs, tenantID)
	return s.impersonateResult, nil
}

func newSuperControllerTestRouter(superService *fakeSuperService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/super")
	group.Use(requireControllerTestAuth("secret"))
	group.Use(middleware.RequireRole(models.UserRoleSuperAdmin))
	NewSuperController(superService).RegisterRoutes(group)
	return router
}

func TestSuperImpersonateRejectsTenantAdmin(t *testing.T) {
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

	superService := &fakeSuperService{}
	router := newSuperControllerTestRouter(superService)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/super/tenants/2002/impersonate", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if superService.impersonateCalls != 0 {
		t.Fatalf("impersonate calls = %d, want 0", superService.impersonateCalls)
	}
}

func TestSuperListTenantsMarksSelfOperatedTenant(t *testing.T) {
	superUser := &models.User{
		BaseModel: models.BaseModel{ID: 1},
		Role:      models.UserRoleSuperAdmin,
	}
	superToken, err := auth.GenerateToken("secret", time.Hour, superUser)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	superService := &fakeSuperService{
		tenants: []models.Tenant{
			{
				BaseModel: models.BaseModel{ID: 1},
				Name:      "Platform Self Operated",
				Type:      models.TenantTypeSelfOperated,
				Status:    models.TenantStatusActive,
			},
			{
				BaseModel: models.BaseModel{ID: 2},
				Name:      "Demo Merchant",
				Type:      models.TenantTypeThirdParty,
				Status:    models.TenantStatusActive,
			},
		},
	}

	router := newSuperControllerTestRouter(superService)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/super/tenants", nil)
	request.Header.Set("Authorization", "Bearer "+superToken)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}

	var payload struct {
		Data []struct {
			ID             uint              `json:"id"`
			Type           models.TenantType `json:"type"`
			IsSelfOperated bool              `json:"is_self_operated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if len(payload.Data) != 2 {
		t.Fatalf("tenants = %d, want 2", len(payload.Data))
	}
	if !payload.Data[0].IsSelfOperated {
		t.Fatalf("self operated tenant response = %+v, want is_self_operated", payload.Data[0])
	}
	if payload.Data[1].IsSelfOperated {
		t.Fatalf("third party tenant response = %+v, want not self operated", payload.Data[1])
	}
}

func TestSuperImpersonateReturnsTenantAdminTokenForTargetTenant(t *testing.T) {
	superUser := &models.User{
		BaseModel: models.BaseModel{ID: 1},
		Role:      models.UserRoleSuperAdmin,
	}
	superToken, err := auth.GenerateToken("secret", time.Hour, superUser)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	targetTenantID := uint(2002)
	tenantAdmin := &models.User{
		BaseModel: models.BaseModel{ID: 7},
		TenantID:  &targetTenantID,
		Role:      models.UserRoleTenantAdmin,
		Phone:     "13800000001",
		Status:    models.UserStatusActive,
	}
	impersonatedToken, err := auth.GenerateToken("secret", time.Hour, tenantAdmin)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	superService := &fakeSuperService{
		impersonateResult: &service.AuthResult{
			Token: impersonatedToken,
			User: service.AuthUser{
				ID:       tenantAdmin.ID,
				TenantID: tenantAdmin.TenantID,
				Role:     tenantAdmin.Role,
				Phone:    tenantAdmin.Phone,
				Status:   tenantAdmin.Status,
			},
		},
	}

	router := newSuperControllerTestRouter(superService)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/super/tenants/2002/impersonate", nil)
	request.Header.Set("Authorization", "Bearer "+superToken)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if superService.impersonateCalls != 1 || superService.impersonateTenantID != targetTenantID {
		t.Fatalf("impersonate tenant = %d calls = %d, want tenant %d once", superService.impersonateTenantID, superService.impersonateCalls, targetTenantID)
	}
	if superService.impersonateSuperAdminID != superUser.ID {
		t.Fatalf("impersonate super admin id = %d, want %d", superService.impersonateSuperAdminID, superUser.ID)
	}

	var payload struct {
		Data service.AuthResult `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	claims, err := auth.ParseToken("secret", payload.Data.Token)
	if err != nil {
		t.Fatalf("ParseToken returned error: %v", err)
	}
	if claims.TenantID == nil || *claims.TenantID != targetTenantID {
		t.Fatalf("token tenant_id = %v, want %d", claims.TenantID, targetTenantID)
	}
	if claims.Role != models.UserRoleTenantAdmin {
		t.Fatalf("token role = %q, want %q", claims.Role, models.UserRoleTenantAdmin)
	}
}

func TestSuperImpersonateUsesEachRequestedTenantID(t *testing.T) {
	superUser := &models.User{
		BaseModel: models.BaseModel{ID: 1},
		Role:      models.UserRoleSuperAdmin,
	}
	superToken, err := auth.GenerateToken("secret", time.Hour, superUser)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	tenantID := uint(1001)
	tenantAdmin := &models.User{
		BaseModel: models.BaseModel{ID: 7},
		TenantID:  &tenantID,
		Role:      models.UserRoleTenantAdmin,
		Phone:     "13800000001",
		Status:    models.UserStatusActive,
	}
	impersonatedToken, err := auth.GenerateToken("secret", time.Hour, tenantAdmin)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	superService := &fakeSuperService{
		impersonateResult: &service.AuthResult{
			Token: impersonatedToken,
			User: service.AuthUser{
				ID:       tenantAdmin.ID,
				TenantID: tenantAdmin.TenantID,
				Role:     tenantAdmin.Role,
				Phone:    tenantAdmin.Phone,
				Status:   tenantAdmin.Status,
			},
		},
	}

	router := newSuperControllerTestRouter(superService)
	for _, targetTenantID := range []uint{1001, 2002} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/super/tenants/"+strconv.FormatUint(uint64(targetTenantID), 10)+"/impersonate", nil)
		request.Header.Set("Authorization", "Bearer "+superToken)
		router.ServeHTTP(response, request)

		if response.Code != http.StatusOK {
			t.Fatalf("tenant %d status = %d, want %d, body = %s", targetTenantID, response.Code, http.StatusOK, response.Body.String())
		}
	}

	if got, want := superService.impersonateTenantIDs, []uint{1001, 2002}; !reflect.DeepEqual(got, want) {
		t.Fatalf("impersonate tenant ids = %v, want %v", got, want)
	}
}
