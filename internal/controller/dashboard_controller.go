package controller

import (
	"net/http"

	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type DashboardController struct {
	service *service.DashboardService
}

func NewDashboardController(service *service.DashboardService) *DashboardController {
	return &DashboardController{service: service}
}

func (ctl *DashboardController) RegisterRoutes(group *gin.RouterGroup) {
	dashboard := group.Group("/admin/dashboard")
	dashboard.GET("/summary", ctl.Summary)
}

func (ctl *DashboardController) Summary(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	summary, err := ctl.service.Summary(c.Request.Context(), tenantID)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: summary})
}
