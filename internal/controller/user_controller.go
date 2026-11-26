package controller

import (
	"net/http"

	"saas/internal/middleware"
	"saas/internal/models"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type UserController struct {
	service *service.UserService
}

type userPayload struct {
	TenantID *uint             `json:"tenant_id"`
	Role     models.UserRole   `json:"role"`
	Phone    string            `json:"phone"`
	Password string            `json:"password"`
	Status   models.UserStatus `json:"status"`
}

func (p userPayload) toModel() models.User {
	return models.User{
		TenantID: p.TenantID,
		Role:     p.Role,
		Phone:    p.Phone,
		Status:   p.Status,
	}
}

func NewUserController(service *service.UserService) *UserController {
	return &UserController{service: service}
}

func (ctl *UserController) RegisterRoutes(group *gin.RouterGroup) {
	group.POST("/users", ctl.Create)
	group.GET("/users", ctl.List)
	group.GET("/users/:id", ctl.Get)
	group.PUT("/users/:id", ctl.Update)
	group.DELETE("/users/:id", ctl.Delete)
}

func (ctl *UserController) Create(c *gin.Context) {
	var payload userPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	user := payload.toModel()

	if err := applyUserMutationScope(c, &user); err != nil {
		writeError(c, err)
		return
	}

	if err := ctl.service.Create(c.Request.Context(), &user, payload.Password); err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusCreated, response{Data: user})
}

func (ctl *UserController) Get(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	tenantID, err := userQueryScope(c)
	if err != nil {
		writeError(c, err)
		return
	}

	user, err := ctl.service.FindByID(c.Request.Context(), id, tenantID)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: user})
}

func (ctl *UserController) List(c *gin.Context) {
	tenantID, err := userQueryScope(c)
	if err != nil {
		writeError(c, err)
		return
	}

	users, err := ctl.service.List(c.Request.Context(), tenantID)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: users})
}

func (ctl *UserController) Update(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	tenantID, err := userQueryScope(c)
	if err != nil {
		writeError(c, err)
		return
	}

	var payload userPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	user := payload.toModel()
	user.ID = id

	currentUser, err := ctl.service.FindByID(c.Request.Context(), id, tenantID)
	if err != nil {
		writeError(c, err)
		return
	}
	if err := ensureCanMutateRole(c, currentUser.Role); err != nil {
		writeError(c, err)
		return
	}
	if err := applyUserMutationScope(c, &user); err != nil {
		writeError(c, err)
		return
	}

	var password *string
	if payload.Password != "" {
		password = &payload.Password
	}

	if err := ctl.service.Update(c.Request.Context(), &user, password, tenantID); err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: user})
}

func (ctl *UserController) Delete(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if userID, ok := middleware.UserIDFromContext(c); !ok || userID == id {
		writeError(c, service.ErrForbidden)
		return
	}
	tenantID, err := userQueryScope(c)
	if err != nil {
		writeError(c, err)
		return
	}

	user, err := ctl.service.FindByID(c.Request.Context(), id, tenantID)
	if err != nil {
		writeError(c, err)
		return
	}
	if err := ensureCanMutateRole(c, user.Role); err != nil {
		writeError(c, err)
		return
	}

	if err := ctl.service.Delete(c.Request.Context(), id, tenantID); err != nil {
		writeError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func userQueryScope(c *gin.Context) (*uint, error) {
	role, ok := middleware.RoleFromContext(c)
	if !ok {
		return nil, service.ErrUnauthorized
	}
	if role == models.UserRoleSuperAdmin {
		return nil, nil
	}
	tenantID, ok := middleware.TenantIDFromContext(c)
	if !ok || tenantID == nil {
		return nil, service.ErrForbidden
	}
	return tenantID, nil
}

func applyUserMutationScope(c *gin.Context, user *models.User) error {
	role, ok := middleware.RoleFromContext(c)
	if !ok {
		return service.ErrUnauthorized
	}
	if role == models.UserRoleSuperAdmin {
		return nil
	}
	if user.Role == models.UserRoleSuperAdmin {
		return service.ErrForbidden
	}
	tenantID, ok := middleware.TenantIDFromContext(c)
	if !ok || tenantID == nil {
		return service.ErrForbidden
	}
	user.TenantID = tenantID
	return nil
}

func ensureCanMutateRole(c *gin.Context, role models.UserRole) error {
	if middleware.IsSuperAdmin(c) {
		return nil
	}
	if role == models.UserRoleSuperAdmin {
		return service.ErrForbidden
	}
	return nil
}
