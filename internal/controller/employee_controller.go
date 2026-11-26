package controller

import (
	"net/http"
	"time"

	"saas/internal/models"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type EmployeeController struct {
	service *service.UserService
}

type createEmployeePayload struct {
	Role     models.UserRole `json:"role" binding:"required"`
	Phone    string          `json:"phone" binding:"required"`
	Password string          `json:"password" binding:"required"`
}

type employeeResponse struct {
	ID        uint              `json:"id"`
	TenantID  *uint             `json:"tenant_id,omitempty"`
	Role      models.UserRole   `json:"role"`
	Phone     string            `json:"phone"`
	Status    models.UserStatus `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

func NewEmployeeController(service *service.UserService) *EmployeeController {
	return &EmployeeController{service: service}
}

func (ctl *EmployeeController) RegisterRoutes(group *gin.RouterGroup) {
	employees := group.Group("/admin/employees")
	employees.POST("", ctl.Create)
	employees.GET("", ctl.List)
}

func (ctl *EmployeeController) Create(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	var payload createEmployeePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	user, err := ctl.service.CreateEmployee(c.Request.Context(), tenantID, service.CreateEmployeeInput{
		Role:     payload.Role,
		Phone:    payload.Phone,
		Password: payload.Password,
	})
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusCreated, response{Data: toEmployeeResponse(*user)})
}

func (ctl *EmployeeController) List(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	users, err := ctl.service.ListEmployees(c.Request.Context(), tenantID)
	if err != nil {
		writeError(c, err)
		return
	}

	items := make([]employeeResponse, 0, len(users))
	for _, user := range users {
		items = append(items, toEmployeeResponse(user))
	}
	c.JSON(http.StatusOK, response{Data: items})
}

func toEmployeeResponse(user models.User) employeeResponse {
	return employeeResponse{
		ID:        user.ID,
		TenantID:  user.TenantID,
		Role:      user.Role,
		Phone:     user.Phone,
		Status:    user.Status,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}
