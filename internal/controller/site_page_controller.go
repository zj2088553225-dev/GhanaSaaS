package controller

import (
	"net/http"

	"saas/internal/middleware"
	"saas/internal/pkg/locale"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type SitePageController struct {
	service *service.SitePageService
}

type sitePageDraftPayload struct {
	Title        string                       `json:"title"`
	Content      map[string]any               `json:"content" binding:"required"`
	Translations *sitePageTranslationsPayload `json:"translations"`
}

type sitePageTranslationsPayload struct {
	Zh *sitePageLocaleTranslationPayload `json:"zh"`
}

type sitePageLocaleTranslationPayload struct {
	DraftContent map[string]any `json:"draft_content"`
}

func NewSitePageController(service *service.SitePageService) *SitePageController {
	return &SitePageController{service: service}
}

func (ctl *SitePageController) RegisterAdminRoutes(group *gin.RouterGroup) {
	home := group.Group("/admin/site-pages/home")
	home.GET("", ctl.GetHome)
	home.PUT("/draft", ctl.SaveHomeDraft)
	home.POST("/publish", ctl.PublishHome)
}

func (ctl *SitePageController) RegisterPublicRoutes(group *gin.RouterGroup) {
	pages := group.Group("/storefront/pages")
	pages.GET("/home", ctl.PublicHome)
}

func (ctl *SitePageController) GetHome(c *gin.Context) {
	page, err := ctl.service.GetHomeAdmin(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: page})
}

func (ctl *SitePageController) SaveHomeDraft(c *gin.Context) {
	userID, err := userIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	var payload sitePageDraftPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
		return
	}

	page, err := ctl.service.SaveHomeDraft(c.Request.Context(), userID, service.SitePageDraftInput{
		Title:        payload.Title,
		Content:      payload.Content,
		Translations: sitePageTranslationInput(payload.Translations),
	})
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: page})
}

func sitePageTranslationInput(payload *sitePageTranslationsPayload) *service.SitePageDraftTranslationsInput {
	if payload == nil || payload.Zh == nil {
		return nil
	}
	return &service.SitePageDraftTranslationsInput{
		Zh: &service.SitePageLocaleDraftInput{
			DraftContent: payload.Zh.DraftContent,
		},
	}
}

func (ctl *SitePageController) PublishHome(c *gin.Context) {
	userID, err := userIDFromAuthContext(c)
	if err != nil {
		writeError(c, err)
		return
	}

	page, err := ctl.service.PublishHome(c.Request.Context(), userID)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: page})
}

func (ctl *SitePageController) PublicHome(c *gin.Context) {
	page, err := ctl.service.PublicHomeWithLocale(c.Request.Context(), locale.NormalizeLocale(c.Query("locale")))
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: page})
}

func userIDFromAuthContext(c *gin.Context) (uint, error) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok || userID == 0 {
		return 0, service.ErrUnauthorized
	}
	return userID, nil
}
