package controller

import (
	"net/http"

	"saas/internal/models"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type TenantController struct {
	service *service.TenantService
}

func NewTenantController(service *service.TenantService) *TenantController {
	return &TenantController{service: service}
}

func (ctl *TenantController) RegisterRoutes(group *gin.RouterGroup) {
	group.POST("/tenants", ctl.Create)
	group.GET("/tenants", ctl.List)
	group.GET("/tenants/:id", ctl.Get)
	group.PUT("/tenants/:id", ctl.Update)
	group.DELETE("/tenants/:id", ctl.Delete)
}

func (ctl *TenantController) Create(c *gin.Context) {
	var tenant models.Tenant
	if err := c.ShouldBindJSON(&tenant); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	if err := ctl.service.Create(c.Request.Context(), &tenant); err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusCreated, response{Data: tenant})
}

func (ctl *TenantController) Get(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	tenant, err := ctl.service.FindByID(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: tenant})
}

func (ctl *TenantController) List(c *gin.Context) {
	tenants, err := ctl.service.List(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: tenants})
}

func (ctl *TenantController) Update(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var tenant models.Tenant
	if err := c.ShouldBindJSON(&tenant); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	tenant.ID = id

	if err := ctl.service.Update(c.Request.Context(), &tenant); err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: tenant})
}

func (ctl *TenantController) Delete(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	if err := ctl.service.Delete(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}
