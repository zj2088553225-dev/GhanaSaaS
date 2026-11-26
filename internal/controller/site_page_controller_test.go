package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"saas/internal/middleware"
	"saas/internal/models"
	"saas/internal/pkg/auth"
	"saas/internal/repository"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type noopSitePageRepository struct {
	page *models.SitePage
}

func (r *noopSitePageRepository) FindByKey(_ context.Context, pageKey string) (*models.SitePage, error) {
	if r.page == nil || r.page.PageKey != pageKey {
		return nil, gorm.ErrRecordNotFound
	}
	cloned := *r.page
	return &cloned, nil
}

func (r *noopSitePageRepository) SaveDraftAtomic(_ context.Context, pageKey string, title string, content datatypes.JSON, userID uint, _ repository.SitePageTranslationMutation) (*models.SitePage, error) {
	page := &models.SitePage{
		BaseModel:    models.BaseModel{ID: 1},
		PageKey:      pageKey,
		Title:        title,
		DraftContent: append(datatypes.JSON(nil), content...),
		Status:       models.SitePageStatusDraft,
		CreatedBy:    &userID,
		UpdatedBy:    &userID,
	}
	r.page = page
	cloned := *page
	return &cloned, nil
}

func (r *noopSitePageRepository) PublishAtomic(_ context.Context, pageKey string, userID uint, _ repository.SitePagePublishTranslationBuilder) (*models.SitePage, error) {
	if r.page == nil || r.page.PageKey != pageKey {
		return nil, gorm.ErrRecordNotFound
	}
	r.page.PublishedContent = append(datatypes.JSON(nil), r.page.DraftContent...)
	r.page.Status = models.SitePageStatusPublished
	r.page.UpdatedBy = &userID
	cloned := *r.page
	return &cloned, nil
}

func newSitePageAdminTestRouter(repo *noopSitePageRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1")
	group.Use(requireControllerTestAuth("secret"))
	group.Use(middleware.RequireRole(models.UserRoleSuperAdmin))
	controller := NewSitePageController(service.NewSitePageService(repo))
	controller.RegisterAdminRoutes(group)
	return router
}

func TestSitePageAdminRouteRejectsUnauthenticatedRequest(t *testing.T) {
	router := newSitePageAdminTestRouter(&noopSitePageRepository{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/site-pages/home", nil)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestSitePageAdminRouteRejectsTenantAdmin(t *testing.T) {
	tenantID := uint(1001)
	user := &models.User{
		BaseModel: models.BaseModel{ID: 1},
		TenantID:  &tenantID,
		Role:      models.UserRoleTenantAdmin,
	}
	token, err := auth.GenerateToken("secret", time.Hour, user)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	router := newSitePageAdminTestRouter(&noopSitePageRepository{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/site-pages/home/draft", strings.NewReader(`{"title":"Home","content":{"blocks":[]}}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestSitePageAdminRouteReturnsDefaultDraftWhenMissing(t *testing.T) {
	user := &models.User{
		BaseModel: models.BaseModel{ID: 1},
		Role:      models.UserRoleSuperAdmin,
	}
	token, err := auth.GenerateToken("secret", time.Hour, user)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	router := newSitePageAdminTestRouter(&noopSitePageRepository{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/site-pages/home", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}

	var payload struct {
		Data service.SitePageAdminDTO `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v, body = %s", err, response.Body.String())
	}
	if payload.Data.PageKey != service.SitePageHomeKey {
		t.Fatalf("page_key = %q, want %q", payload.Data.PageKey, service.SitePageHomeKey)
	}
	if payload.Data.Title != "平台主页" {
		t.Fatalf("title = %q, want 平台主页", payload.Data.Title)
	}
	if payload.Data.Status != models.SitePageStatusDraft {
		t.Fatalf("status = %q, want %q", payload.Data.Status, models.SitePageStatusDraft)
	}
	if len(payload.Data.DraftContent.Blocks) != 0 {
		t.Fatalf("blocks len = %d, want 0", len(payload.Data.DraftContent.Blocks))
	}
	if payload.Data.PublishedContent != nil {
		t.Fatalf("published_content = %+v, want nil", payload.Data.PublishedContent)
	}
}

func TestSitePageAdminRouteAllowsSuperAdminDraftSave(t *testing.T) {
	user := &models.User{
		BaseModel: models.BaseModel{ID: 1},
		Role:      models.UserRoleSuperAdmin,
	}
	token, err := auth.GenerateToken("secret", time.Hour, user)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	router := newSitePageAdminTestRouter(&noopSitePageRepository{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/site-pages/home/draft", strings.NewReader(`{"title":"Home","content":{"blocks":[]}}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, http.StatusOK, response.Body.String())
	}
}

func TestSitePageAdminRouteSupportsHeroCarouselDraftSaveAndPublish(t *testing.T) {
	user := &models.User{
		BaseModel: models.BaseModel{ID: 1},
		Role:      models.UserRoleSuperAdmin,
	}
	token, err := auth.GenerateToken("secret", time.Hour, user)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	repo := &noopSitePageRepository{}
	router := newSitePageAdminTestRouter(repo)

	saveRequest := httptest.NewRequest(http.MethodPut, "/api/v1/admin/site-pages/home/draft", strings.NewReader(`{
		"title":"Mall Home",
		"content":{
			"blocks":[
				{
					"type":"hero_carousel",
					"autoplay":true,
					"interval":4500,
					"height":"large",
					"fit":"cover",
					"slides":[
						{"image":"/uploads/banner.webp","link":"/mall","enabled":true}
					]
				}
			]
		}
	}`))
	saveRequest.Header.Set("Authorization", "Bearer "+token)
	saveRequest.Header.Set("Content-Type", "application/json")

	saveResponse := httptest.NewRecorder()
	router.ServeHTTP(saveResponse, saveRequest)

	if saveResponse.Code != http.StatusOK {
		t.Fatalf("save status = %d, want %d, body = %s", saveResponse.Code, http.StatusOK, saveResponse.Body.String())
	}

	publishRequest := httptest.NewRequest(http.MethodPost, "/api/v1/admin/site-pages/home/publish", nil)
	publishRequest.Header.Set("Authorization", "Bearer "+token)

	publishResponse := httptest.NewRecorder()
	router.ServeHTTP(publishResponse, publishRequest)

	if publishResponse.Code != http.StatusOK {
		t.Fatalf("publish status = %d, want %d, body = %s", publishResponse.Code, http.StatusOK, publishResponse.Body.String())
	}

	var payload struct {
		Data service.SitePageAdminDTO `json:"data"`
	}
	if err := json.Unmarshal(publishResponse.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v, body = %s", err, publishResponse.Body.String())
	}
	if payload.Data.Status != models.SitePageStatusPublished {
		t.Fatalf("status = %q, want %q", payload.Data.Status, models.SitePageStatusPublished)
	}
	if payload.Data.PublishedContent == nil || len(payload.Data.PublishedContent.Blocks) != 1 {
		t.Fatalf("published_content = %+v, want one block", payload.Data.PublishedContent)
	}
	if payload.Data.PublishedContent.Blocks[0]["type"] != "hero_carousel" {
		t.Fatalf("published block = %+v, want hero_carousel", payload.Data.PublishedContent.Blocks[0])
	}
	if payload.Data.PublishedContent.Blocks[0]["fit"] != "cover" {
		t.Fatalf("published fit = %v, want cover", payload.Data.PublishedContent.Blocks[0]["fit"])
	}
}
