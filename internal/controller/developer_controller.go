package controller

import (
	"net/http"

	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type DeveloperController struct {
	service *service.TenantService
}

type developerCredentialsResponse struct {
	AppKey    string `json:"app_key"`
	AppSecret string `json:"app_secret"`
}

func NewDeveloperController(service *service.TenantService) *DeveloperController {
	return &DeveloperController{service: service}
}

func (ctl *DeveloperController) RegisterRoutes(group *gin.RouterGroup) {
	settings := group.Group("/admin/settings/developer")
	settings.GET("", ctl.GetCredentials)
	settings.POST("/regenerate-secret", ctl.RegenerateSecret)
}

func (ctl *DeveloperController) GetCredentials(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	tenant, err := ctl.service.DeveloperCredentials(c.Request.Context(), tenantID)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: developerCredentialsResponse{
		AppKey:    tenant.AppKey,
		AppSecret: tenant.AppSecret,
	}})
}

func (ctl *DeveloperController) RegenerateSecret(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	tenant, err := ctl.service.RegenerateAppSecret(c.Request.Context(), tenantID)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: developerCredentialsResponse{
		AppKey:    tenant.AppKey,
		AppSecret: tenant.AppSecret,
	}})
}
