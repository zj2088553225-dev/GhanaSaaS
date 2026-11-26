package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"saas/internal/models"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	maxStorefrontEventBodyBytes       int64 = 8 * 1024
	maxStorefrontEventBatchBodyBytes  int64 = 64 * 1024
	storefrontEventVisitorLimit             = 120
	storefrontEventIPRequestLimit           = 600
	storefrontEventGlobalRequestLimit       = 6000
	storefrontEventGlobalEventLimit         = 6000
	storefrontEventRateLimitWindow          = time.Minute
)

type storefrontEventTracker interface {
	Track(ctx context.Context, input service.StorefrontEventInput, trafficTypes ...models.StorefrontTrafficType) error
	TrackBatch(ctx context.Context, input service.StorefrontEventBatchInput, trafficTypes ...models.StorefrontTrafficType) (service.StorefrontEventBatchResult, error)
}

type StorefrontEventController struct {
	service          storefrontEventTracker
	limiter          *storefrontEventRateLimiter
	clientIPResolver *service.ClientIPResolver
}

func NewStorefrontEventController(tracker storefrontEventTracker) *StorefrontEventController {
	clientIPResolver, _ := service.NewClientIPResolver(nil)
	return &StorefrontEventController{
		service:          tracker,
		limiter:          newStorefrontEventRateLimiter(),
		clientIPResolver: clientIPResolver,
	}
}

func (ctl *StorefrontEventController) WithClientIPResolver(resolver *service.ClientIPResolver) *StorefrontEventController {
	if resolver != nil {
		ctl.clientIPResolver = resolver
	}
	return ctl
}

func (ctl *StorefrontEventController) RegisterRoutes(group *gin.RouterGroup) {
	group.POST("/storefront/events", ctl.Create)
	group.POST("/storefront/events/batch", ctl.CreateBatch)
}

func (ctl *StorefrontEventController) Create(c *gin.Context) {
	now := time.Now()
	if !ctl.limiter.AllowRequest(now, ctl.clientIP(c)) {
		writeStorefrontEventRateLimit(c)
		return
	}

	var input service.StorefrontEventInput
	if err := decodeStorefrontEventJSON(c, maxStorefrontEventBodyBytes, &input); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: service.ErrInvalidInput.Error()})
		return
	}
	if !ctl.limiter.AllowEvents([]string{input.VisitorID}, now) {
		writeStorefrontEventRateLimit(c)
		return
	}

	if err := ctl.service.Track(c.Request.Context(), input, classifyStorefrontTraffic(c.GetHeader("User-Agent"))); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, response{Data: gin.H{"accepted": true}})
}

func (ctl *StorefrontEventController) CreateBatch(c *gin.Context) {
	now := time.Now()
	if !ctl.limiter.AllowRequest(now, ctl.clientIP(c)) {
		writeStorefrontEventRateLimit(c)
		return
	}

	var input service.StorefrontEventBatchInput
	if err := decodeStorefrontEventJSON(c, maxStorefrontEventBatchBodyBytes, &input); err != nil ||
		len(input.Events) == 0 || len(input.Events) > service.MaxStorefrontEventBatchSize {
		c.JSON(http.StatusBadRequest, response{Error: service.ErrInvalidInput.Error()})
		return
	}
	visitorIDs := make([]string, len(input.Events))
	for index, event := range input.Events {
		visitorIDs[index] = event.VisitorID
	}
	if !ctl.limiter.AllowEvents(visitorIDs, now) {
		writeStorefrontEventRateLimit(c)
		return
	}

	result, err := ctl.service.TrackBatch(c.Request.Context(), input, classifyStorefrontTraffic(c.GetHeader("User-Agent")))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, response{Data: gin.H{
		"accepted": result.Accepted, "duplicates": result.Duplicates, "rejected": 0,
	}})
}

func (ctl *StorefrontEventController) clientIP(c *gin.Context) string {
	return ctl.clientIPResolver.Resolve(
		c.Request.RemoteAddr,
		c.GetHeader("CF-Connecting-IP"),
		c.GetHeader("X-Real-IP"),
		c.Request.Header.Values("X-Forwarded-For")...,
	)
}

func decodeStorefrontEventJSON(c *gin.Context, maxBytes int64, target any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil || !utf8.Valid(body) {
		return service.ErrInvalidInput
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(target); err != nil {
		return service.ErrInvalidInput
	}
	return ensureSingleJSONValue(decoder)
}

func writeStorefrontEventRateLimit(c *gin.Context) {
	c.Header("Retry-After", "60")
	c.JSON(http.StatusTooManyRequests, response{Error: "rate limit exceeded"})
}

var storefrontBotUserAgentTokens = []string{
	"bot", "crawler", "spider", "slurp", "headlesschrome",
	"googlebot", "bingbot", "duckduckbot", "baiduspider", "yandexbot",
	"facebookexternalhit",
}

// classifyStorefrontTraffic intentionally recognizes only obvious crawler
// signatures. Empty and unfamiliar User-Agent values remain human so this
// basic quality filter does not pretend to be comprehensive fraud detection.
func classifyStorefrontTraffic(userAgent string) models.StorefrontTrafficType {
	normalized := strings.ToLower(strings.TrimSpace(userAgent))
	for _, token := range storefrontBotUserAgentTokens {
		if strings.Contains(normalized, token) {
			return models.StorefrontTrafficBot
		}
	}
	return models.StorefrontTrafficHuman
}

func ensureSingleJSONValue(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return service.ErrInvalidInput
	}
	return nil
}

type storefrontEventRateLimiter struct {
	mu           sync.Mutex
	windowStart  time.Time
	requestCount int
	eventCount   int
	visitors     map[[sha256.Size]byte]int
	clientIPs    map[[sha256.Size]byte]int
}

func newStorefrontEventRateLimiter() *storefrontEventRateLimiter {
	return &storefrontEventRateLimiter{
		visitors:  make(map[[sha256.Size]byte]int),
		clientIPs: make(map[[sha256.Size]byte]int),
	}
}

func (limiter *storefrontEventRateLimiter) Allow(visitorID string, now time.Time) bool {
	return limiter.AllowMany([]string{visitorID}, now)
}

func (limiter *storefrontEventRateLimiter) AllowMany(visitorIDs []string, now time.Time) bool {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	limiter.resetWindow(now)
	if limiter.requestCount+1 > storefrontEventGlobalRequestLimit || !limiter.allowEvents(visitorIDs) {
		return false
	}
	limiter.requestCount++
	return true
}

// AllowRequest is deliberately called before reading or decoding the public
// request body. Malformed JSON therefore cannot bypass the process-wide
// request-volume boundary.
func (limiter *storefrontEventRateLimiter) AllowRequest(now time.Time, clientIPs ...string) bool {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	limiter.resetWindow(now)
	if limiter.requestCount+1 > storefrontEventGlobalRequestLimit {
		return false
	}
	if len(clientIPs) > 0 && strings.TrimSpace(clientIPs[0]) != "" {
		clientIPKey := sha256.Sum256([]byte(strings.TrimSpace(clientIPs[0])))
		if limiter.clientIPs[clientIPKey]+1 > storefrontEventIPRequestLimit {
			return false
		}
		limiter.clientIPs[clientIPKey]++
	}
	limiter.requestCount++
	return true
}

func (limiter *storefrontEventRateLimiter) AllowEvents(visitorIDs []string, now time.Time) bool {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	limiter.resetWindow(now)
	return limiter.allowEvents(visitorIDs)
}

func (limiter *storefrontEventRateLimiter) resetWindow(now time.Time) {
	if limiter.windowStart.IsZero() || now.Sub(limiter.windowStart) >= storefrontEventRateLimitWindow || now.Before(limiter.windowStart) {
		limiter.windowStart = now
		limiter.requestCount = 0
		limiter.eventCount = 0
		limiter.visitors = make(map[[sha256.Size]byte]int)
		limiter.clientIPs = make(map[[sha256.Size]byte]int)
	}
}

func (limiter *storefrontEventRateLimiter) allowEvents(visitorIDs []string) bool {
	if len(visitorIDs) == 0 || limiter.eventCount+len(visitorIDs) > storefrontEventGlobalEventLimit {
		return false
	}

	batchVisitors := make(map[[sha256.Size]byte]int)
	for _, visitorID := range visitorIDs {
		visitorKey := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(visitorID))))
		batchVisitors[visitorKey]++
	}
	for visitorKey, count := range batchVisitors {
		if limiter.visitors[visitorKey]+count > storefrontEventVisitorLimit {
			return false
		}
	}
	for visitorKey, count := range batchVisitors {
		limiter.visitors[visitorKey] += count
	}
	limiter.eventCount += len(visitorIDs)
	return true
}
