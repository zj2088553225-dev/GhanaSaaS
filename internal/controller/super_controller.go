package controller

import (
	"context"
	"net/http"
	"time"

	"saas/internal/middleware"
	"saas/internal/models"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type SuperController struct {
	service superService
}

type superService interface {
	CreateTenantWithAdmin(ctx context.Context, input service.CreateTenantWithAdminInput) (*service.TenantWithAdminResult, error)
	ListTenants(ctx context.Context) ([]models.Tenant, error)
	GlobalDashboardSummary(ctx context.Context) (*service.GlobalDashboardSummary, error)
	ImpersonateTenantAdmin(ctx context.Context, superAdminID uint, tenantID uint) (*service.AuthResult, error)
}

type createTenantPayload struct {
	Name          string              `json:"name" binding:"required"`
	Type          models.TenantType   `json:"type"`
	Status        models.TenantStatus `json:"status"`
	AdminPhone    string              `json:"admin_phone" binding:"required"`
	AdminPassword string              `json:"admin_password" binding:"required"`
}

type superTenantResponse struct {
	ID             uint                `json:"id"`
	Name           string              `json:"name"`
	Type           models.TenantType   `json:"type"`
	Status         models.TenantStatus `json:"status"`
	IsSelfOperated bool                `json:"is_self_operated"`
	AppKey         string              `json:"app_key"`
	AppSecret      string              `json:"app_secret"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
}

type superUserResponse struct {
	ID        uint              `json:"id"`
	TenantID  *uint             `json:"tenant_id,omitempty"`
	Role      models.UserRole   `json:"role"`
	Phone     string            `json:"phone"`
	Status    models.UserStatus `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type createTenantResponse struct {
	Tenant superTenantResponse `json:"tenant"`
	Admin  superUserResponse   `json:"admin"`
}

func NewSuperController(service superService) *SuperController {
	return &SuperController{service: service}
}

func (ctl *SuperController) RegisterRoutes(group *gin.RouterGroup) {
	dashboard := group.Group("/dashboard")
	dashboard.GET("/summary", ctl.DashboardSummary)

	tenants := group.Group("/tenants")
	tenants.POST("", ctl.CreateTenant)
	tenants.GET("", ctl.ListTenants)
	tenants.POST("/:id/impersonate", ctl.ImpersonateTenant)
}

func (ctl *SuperController) CreateTenant(c *gin.Context) {
	var payload createTenantPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	result, err := ctl.service.CreateTenantWithAdmin(c.Request.Context(), service.CreateTenantWithAdminInput{
		Name:          payload.Name,
		Type:          payload.Type,
		Status:        payload.Status,
		AdminPhone:    payload.AdminPhone,
		AdminPassword: payload.AdminPassword,
	})
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusCreated, response{Data: createTenantResponse{
		Tenant: toSuperTenantResponse(result.Tenant),
		Admin:  toSuperUserResponse(result.Admin),
	}})
}

func (ctl *SuperController) ListTenants(c *gin.Context) {
	tenants, err := ctl.service.ListTenants(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}

	items := make([]superTenantResponse, 0, len(tenants))
	for _, tenant := range tenants {
		items = append(items, toSuperTenantResponse(tenant))
	}
	c.JSON(http.StatusOK, response{Data: items})
}

func (ctl *SuperController) DashboardSummary(c *gin.Context) {
	summary, err := ctl.service.GlobalDashboardSummary(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: summary})
}

func (ctl *SuperController) ImpersonateTenant(c *gin.Context) {
	tenantID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	superAdminID, ok := middleware.UserIDFromContext(c)
	if !ok || superAdminID == 0 {
		writeError(c, service.ErrForbidden)
		return
	}

	result, err := ctl.service.ImpersonateTenantAdmin(c.Request.Context(), superAdminID, tenantID)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: result})
}

func toSuperTenantResponse(tenant models.Tenant) superTenantResponse {
	return superTenantResponse{
		ID:             tenant.ID,
		Name:           tenant.Name,
		Type:           tenant.Type,
		Status:         tenant.Status,
		IsSelfOperated: tenant.Type == models.TenantTypeSelfOperated,
		AppKey:         tenant.AppKey,
		AppSecret:      tenant.AppSecret,
		CreatedAt:      tenant.CreatedAt,
		UpdatedAt:      tenant.UpdatedAt,
	}
}

func toSuperUserResponse(user models.User) superUserResponse {
	return superUserResponse{
		ID:        user.ID,
		TenantID:  user.TenantID,
		Role:      user.Role,
		Phone:     user.Phone,
		Status:    user.Status,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}
