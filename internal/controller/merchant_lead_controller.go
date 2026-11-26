package controller

import (
	"context"
	"net/http"
	"sync"
	"time"

	"saas/internal/models"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type MerchantLeadController struct {
	service     merchantLeadService
	rateLimiter *merchantLeadRateLimiter
}

type merchantLeadService interface {
	Create(ctx context.Context, input service.MerchantLeadInput) (*models.MerchantLead, error)
	List(ctx context.Context) ([]models.MerchantLead, error)
	UpdateStatus(ctx context.Context, id uint, status models.MerchantLeadStatus) (*models.MerchantLead, error)
}

type updateMerchantLeadStatusPayload struct {
	Status models.MerchantLeadStatus `json:"status"`
}

func NewMerchantLeadController(service merchantLeadService) *MerchantLeadController {
	return &MerchantLeadController{
		service:     service,
		rateLimiter: newMerchantLeadRateLimiter(10*time.Minute, 5),
	}
}

func (ctl *MerchantLeadController) RegisterPublicRoutes(group *gin.RouterGroup) {
	storefront := group.Group("/storefront")
	storefront.POST("/merchant-leads", ctl.Create)
}

func (ctl *MerchantLeadController) RegisterSuperRoutes(group *gin.RouterGroup) {
	leads := group.Group("/merchant-leads")
	leads.GET("", ctl.List)
	leads.PATCH("/:id/status", ctl.UpdateStatus)
}

func (ctl *MerchantLeadController) Create(c *gin.Context) {
	if ctl.rateLimiter != nil && !ctl.rateLimiter.Allow(c.ClientIP(), time.Now()) {
		const message = "Too many applications. Please try again later"
		c.JSON(http.StatusTooManyRequests, response{Message: message, Error: message})
		return
	}

	var input service.MerchantLeadInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: service.ErrInvalidInput.Error()})
		return
	}

	if _, err := ctl.service.Create(c.Request.Context(), input); err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Application submitted successfully"})
}

func (ctl *MerchantLeadController) List(c *gin.Context) {
	leads, err := ctl.service.List(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: leads})
}

func (ctl *MerchantLeadController) UpdateStatus(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	var payload updateMerchantLeadStatusPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: service.ErrInvalidInput.Error()})
		return
	}

	lead, err := ctl.service.UpdateStatus(c.Request.Context(), id, payload.Status)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: lead})
}

type merchantLeadRateLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	limit   int
	records map[string][]time.Time
}

func newMerchantLeadRateLimiter(window time.Duration, limit int) *merchantLeadRateLimiter {
	return &merchantLeadRateLimiter{
		window:  window,
		limit:   limit,
		records: make(map[string][]time.Time),
	}
}

func (l *merchantLeadRateLimiter) Allow(key string, now time.Time) bool {
	if l == nil || l.limit <= 0 || l.window <= 0 {
		return true
	}
	if key == "" {
		key = "unknown"
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := now.Add(-l.window)
	recent := l.records[key][:0]
	for _, timestamp := range l.records[key] {
		if timestamp.After(cutoff) {
			recent = append(recent, timestamp)
		}
	}
	if len(recent) >= l.limit {
		l.records[key] = recent
		return false
	}

	recent = append(recent, now)
	l.records[key] = recent
	return true
}
