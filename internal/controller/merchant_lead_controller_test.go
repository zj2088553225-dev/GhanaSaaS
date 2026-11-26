package controller

import (
	"context"
	"encoding/json"
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
)

type fakeMerchantLeadService struct {
	createCalls  int
	createInput  service.MerchantLeadInput
	createErr    error
	listCalls    int
	updateCalls  int
	updateID     uint
	updateStatus models.MerchantLeadStatus
	leads        []models.MerchantLead
}

func (s *fakeMerchantLeadService) Create(_ context.Context, input service.MerchantLeadInput) (*models.MerchantLead, error) {
	s.createCalls++
	s.createInput = input
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &models.MerchantLead{BaseModel: models.BaseModel{ID: 1}, Status: models.MerchantLeadStatusNew}, nil
}

func (s *fakeMerchantLeadService) List(context.Context) ([]models.MerchantLead, error) {
	s.listCalls++
	return s.leads, nil
}

func (s *fakeMerchantLeadService) UpdateStatus(_ context.Context, id uint, status models.MerchantLeadStatus) (*models.MerchantLead, error) {
	s.updateCalls++
	s.updateID = id
	s.updateStatus = status
	return &models.MerchantLead{BaseModel: models.BaseModel{ID: id}, Status: status}, nil
}

func newMerchantLeadPublicTestRouter(service *fakeMerchantLeadService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewMerchantLeadController(service).RegisterPublicRoutes(router.Group("/api/v1"))
	return router
}

func newMerchantLeadSuperTestRouter(service *fakeMerchantLeadService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/super")
	group.Use(requireControllerTestAuth("secret"))
	group.Use(middleware.RequireRole(models.UserRoleSuperAdmin))
	NewMerchantLeadController(service).RegisterSuperRoutes(group)
	return router
}

func TestMerchantLeadCreateIsPublicAndReturnsSuccessMessage(t *testing.T) {
	service := &fakeMerchantLeadService{}
	router := newMerchantLeadPublicTestRouter(service)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/storefront/merchant-leads", strings.NewReader(`{
		"contact_name":"John",
		"shop_name":"ABC Store",
		"whatsapp":"+233500000000",
		"category":"Electronics",
		"has_offline_shop":true
	}`))
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if service.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", service.createCalls)
	}
	if service.createInput.Whatsapp != "+233500000000" || !service.createInput.HasOfflineShop {
		t.Fatalf("unexpected input: %+v", service.createInput)
	}
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if payload.Message != "Application submitted successfully" {
		t.Fatalf("message = %q", payload.Message)
	}
}

func TestMerchantLeadCreateRejectsInvalidJSON(t *testing.T) {
	service := &fakeMerchantLeadService{}
	router := newMerchantLeadPublicTestRouter(service)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/storefront/merchant-leads", strings.NewReader(`{`))
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if service.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", service.createCalls)
	}
}

func TestMerchantLeadCreateReturnsInvalidInputForMissingRequiredFields(t *testing.T) {
	service := &fakeMerchantLeadService{createErr: service.ErrInvalidInput}
	router := newMerchantLeadPublicTestRouter(service)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/storefront/merchant-leads", strings.NewReader(`{"contact_name":"John"}`))
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), "invalid input") {
		t.Fatalf("body = %s, want invalid input", response.Body.String())
	}
}

func TestMerchantLeadCreateRateLimitsSameIP(t *testing.T) {
	service := &fakeMerchantLeadService{}
	router := newMerchantLeadPublicTestRouter(service)

	for i := 0; i < 5; i++ {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/storefront/merchant-leads", strings.NewReader(`{
			"contact_name":"John",
			"shop_name":"ABC Store",
			"whatsapp":"+233500000000",
			"category":"Electronics"
		}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)

		if response.Code != http.StatusCreated {
			t.Fatalf("request %d status = %d, want %d, body = %s", i+1, response.Code, http.StatusCreated, response.Body.String())
		}
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/storefront/merchant-leads", strings.NewReader(`{
		"contact_name":"John",
		"shop_name":"ABC Store",
		"whatsapp":"+233500000000",
		"category":"Electronics"
	}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusTooManyRequests, response.Body.String())
	}
	if service.createCalls != 5 {
		t.Fatalf("create calls = %d, want 5", service.createCalls)
	}
}

func TestMerchantLeadSuperRoutesRequireSuperAdmin(t *testing.T) {
	tenantID := uint(88)
	tenantAdmin := &models.User{
		BaseModel: models.BaseModel{ID: 2},
		TenantID:  &tenantID,
		Role:      models.UserRoleTenantAdmin,
	}
	token, err := auth.GenerateToken("secret", time.Hour, tenantAdmin)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	service := &fakeMerchantLeadService{}
	router := newMerchantLeadSuperTestRouter(service)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/super/merchant-leads", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if service.listCalls != 0 {
		t.Fatalf("list calls = %d, want 0", service.listCalls)
	}
}

func TestMerchantLeadSuperListAndPatchStatus(t *testing.T) {
	superAdmin := &models.User{BaseModel: models.BaseModel{ID: 1}, Role: models.UserRoleSuperAdmin}
	token, err := auth.GenerateToken("secret", time.Hour, superAdmin)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	service := &fakeMerchantLeadService{
		leads: []models.MerchantLead{{
			BaseModel:   models.BaseModel{ID: 3},
			ContactName: "Alice",
			ShopName:    "ABC Store",
			Whatsapp:    "+233500000000",
			Category:    "Retail",
			Status:      models.MerchantLeadStatusNew,
		}},
	}
	router := newMerchantLeadSuperTestRouter(service)

	listResponse := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/super/merchant-leads", nil)
	listRequest.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d, body = %s", listResponse.Code, http.StatusOK, listResponse.Body.String())
	}
	if service.listCalls != 1 {
		t.Fatalf("list calls = %d, want 1", service.listCalls)
	}

	patchResponse := httptest.NewRecorder()
	patchRequest := httptest.NewRequest(http.MethodPatch, "/api/v1/super/merchant-leads/3/status", strings.NewReader(`{"status":"contacted"}`))
	patchRequest.Header.Set("Authorization", "Bearer "+token)
	patchRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(patchResponse, patchRequest)
	if patchResponse.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want %d, body = %s", patchResponse.Code, http.StatusOK, patchResponse.Body.String())
	}
	if service.updateCalls != 1 || service.updateID != 3 || service.updateStatus != models.MerchantLeadStatusContacted {
		t.Fatalf("update = calls %d id %d status %q", service.updateCalls, service.updateID, service.updateStatus)
	}
}
