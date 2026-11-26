package controller

import (
	"net/http"

	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type StorefrontTrackingController struct {
	service *service.StorefrontTrackingService
}

func NewStorefrontTrackingController(service *service.StorefrontTrackingService) *StorefrontTrackingController {
	return &StorefrontTrackingController{service: service}
}

func (ctl *StorefrontTrackingController) RegisterSuperRoutes(group *gin.RouterGroup) {
	group.GET("/storefront-tracking", ctl.GetAdmin)
	group.PUT("/storefront-tracking", ctl.Save)
}

func (ctl *StorefrontTrackingController) RegisterPublicRoutes(group *gin.RouterGroup) {
	group.GET("/storefront/tracking", ctl.GetPublic)
}

func (ctl *StorefrontTrackingController) GetAdmin(c *gin.Context) {
	setting, err := ctl.service.GetAdmin(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: setting})
}

func (ctl *StorefrontTrackingController) Save(c *gin.Context) {
	var input service.StorefrontTrackingInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	setting, err := ctl.service.Save(c.Request.Context(), input)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: setting})
}

func (ctl *StorefrontTrackingController) GetPublic(c *gin.Context) {
	setting, err := ctl.service.GetPublic(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: setting})
}
