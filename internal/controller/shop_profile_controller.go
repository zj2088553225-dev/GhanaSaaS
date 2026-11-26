package controller

import (
	"net/http"

	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type ShopProfileController struct {
	service *service.ShopProfileService
}

type shopProfilePayload struct {
	Slug          string                          `json:"slug"`
	ShopName      string                          `json:"shop_name"`
	LogoURL       string                          `json:"logo_url"`
	BannerURL     string                          `json:"banner_url"`
	Description   string                          `json:"description"`
	Phone         string                          `json:"phone"`
	Whatsapp      string                          `json:"whatsapp"`
	Address       string                          `json:"address"`
	BusinessHours string                          `json:"business_hours"`
	IsPublic      bool                            `json:"is_public"`
	Translations  *shopProfileTranslationsPayload `json:"translations"`
}

type shopProfileTranslationsPayload struct {
	Zh service.ShopProfileTranslationInput `json:"zh"`
}

func NewShopProfileController(service *service.ShopProfileService) *ShopProfileController {
	return &ShopProfileController{service: service}
}

func (ctl *ShopProfileController) RegisterRoutes(group *gin.RouterGroup) {
	profile := group.Group("/admin/shop-profile")
	profile.GET("", ctl.Get)
	profile.PUT("", ctl.Update)
}

func (ctl *ShopProfileController) Get(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	profile, err := ctl.service.GetForTenant(c.Request.Context(), tenantID)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: profile})
}

func (ctl *ShopProfileController) Update(c *gin.Context) {
	tenantID, err := tenantIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	var payload shopProfilePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	profile, err := ctl.service.UpdateForTenant(c.Request.Context(), tenantID, payload.toInput())
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: profile})
}

func (p shopProfilePayload) toInput() service.ShopProfileInput {
	return service.ShopProfileInput{
		Slug:          p.Slug,
		ShopName:      p.ShopName,
		LogoURL:       p.LogoURL,
		BannerURL:     p.BannerURL,
		Description:   p.Description,
		Phone:         p.Phone,
		Whatsapp:      p.Whatsapp,
		Address:       p.Address,
		BusinessHours: p.BusinessHours,
		IsPublic:      p.IsPublic,
		Translations:  p.toTranslationInput(),
	}
}

func (p shopProfilePayload) toTranslationInput() *service.ShopProfileTranslationInput {
	if p.Translations == nil {
		return nil
	}
	return &service.ShopProfileTranslationInput{
		ShopName:      p.Translations.Zh.ShopName,
		Description:   p.Translations.Zh.Description,
		Address:       p.Translations.Zh.Address,
		BusinessHours: p.Translations.Zh.BusinessHours,
	}
}
