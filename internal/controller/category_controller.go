package controller

import (
	"net/http"

	"saas/internal/models"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type CategoryController struct {
	service *service.CategoryService
}

func NewCategoryController(service *service.CategoryService) *CategoryController {
	return &CategoryController{service: service}
}

func (ctl *CategoryController) RegisterRoutes(group *gin.RouterGroup) {
	group.POST("/categories", ctl.Create)
	group.GET("/categories", ctl.List)
	group.GET("/categories/:id", ctl.Get)
	group.PUT("/categories/:id", ctl.Update)
	group.DELETE("/categories/:id", ctl.Delete)
}

func (ctl *CategoryController) Create(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	var category models.Category
	if err := c.ShouldBindJSON(&category); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	if err := ctl.service.Create(c.Request.Context(), tenantID, &category); err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusCreated, response{Data: category})
}

func (ctl *CategoryController) Get(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	category, err := ctl.service.FindByID(c.Request.Context(), tenantID, id)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: category})
}

func (ctl *CategoryController) List(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	categories, err := ctl.service.List(c.Request.Context(), tenantID)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: categories})
}

func (ctl *CategoryController) Update(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	var category models.Category
	if err := c.ShouldBindJSON(&category); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
		return
	}
	category.ID = id

	if err := ctl.service.Update(c.Request.Context(), tenantID, &category); err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: category})
}

func (ctl *CategoryController) Delete(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	if err := ctl.service.Delete(c.Request.Context(), tenantID, id); err != nil {
		writeError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}
