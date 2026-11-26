package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"saas/internal/middleware"
	"saas/internal/models"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	maxReservationAdminBodyBytes    = 4 << 20
	maxReservationInterestBodyBytes = 16 << 10
)

type reservationService interface {
	CreateProduct(ctx context.Context, input service.ReservationProductInput) (*service.ReservationProductAdminDTO, error)
	UpdateProduct(ctx context.Context, id uint64, input service.ReservationProductInput) (*service.ReservationProductAdminDTO, error)
	ArchiveProduct(ctx context.Context, id uint64) error
	AdminProduct(ctx context.Context, id uint64) (*service.ReservationProductAdminDTO, error)
	AdminProducts(ctx context.Context, query service.ReservationAdminQuery) (*service.ReservationProductAdminPage, error)
	PublicProducts(ctx context.Context, query service.ReservationPublicQuery) (*service.ReservationProductPublicPage, error)
	PublicProduct(ctx context.Context, slug, locale, visitorID string) (*service.ReservationProductPublicDTO, error)
	RecordInterest(ctx context.Context, productID uint64, input service.ReservationInterestInput) (service.ReservationInterestResult, error)
	ProductAnalytics(ctx context.Context, productID uint64) (*service.ReservationProductAnalyticsDTO, error)
}

type ReservationController struct {
	mode            string
	service         reservationService
	workflow        reservationWorkflow
	anonymous       anonymousReservationWorkflow
	browserIdentity *service.ReservationBrowserIdentity
	security        *service.ReservationRequestSecurity
}

type reservationWorkflow interface {
	Capabilities() service.ReservationCapabilities
	RequestOTP(ctx context.Context, productID uint64, input service.ReservationOTPRequestInput) (service.ReservationOTPRequestResult, error)
	VerifyOTP(ctx context.Context, productID uint64, input service.ReservationOTPVerifyInput) (service.ReservationOTPVerifyResult, error)
	Cancel(ctx context.Context, productID uint64, input service.ReservationCancelInput) (service.ReservationCancelResult, error)
}

type anonymousReservationWorkflow interface {
	Create(ctx context.Context, input service.AnonymousReservationCreateInput) (service.AnonymousReservationDTO, error)
	Mine(ctx context.Context, browserHash, requestedLocale string) ([]service.AnonymousReservationDTO, error)
	Cancel(ctx context.Context, reservationCode, browserHash, requestedLocale string) (service.AnonymousReservationDTO, error)
	ExpireDue(ctx context.Context, batchSize int) (int64, error)
	AdminReservations(ctx context.Context, query service.AnonymousReservationAdminQuery) (*service.AnonymousReservationAdminPage, error)
	AdminSetStatus(ctx context.Context, reservationCode, status string) (service.AnonymousReservationAdminDTO, error)
	AdminSetRisk(ctx context.Context, reservationCode string, flagged bool) (service.AnonymousReservationAdminDTO, error)
}

type ReservationControllerOptions struct {
	Mode            string
	Workflow        reservationWorkflow
	Anonymous       anonymousReservationWorkflow
	BrowserIdentity *service.ReservationBrowserIdentity
	Security        *service.ReservationRequestSecurity
}

type reservationProductPayload struct {
	LinkedProductID    *uint                           `json:"linked_product_id"`
	Slug               string                          `json:"slug"`
	TitleEN            string                          `json:"title_en"`
	TitleZH            *string                         `json:"title_zh"`
	SummaryEN          string                          `json:"summary_en"`
	SummaryZH          *string                         `json:"summary_zh"`
	DescriptionEN      string                          `json:"description_en"`
	DescriptionZH      *string                         `json:"description_zh"`
	MainImageURL       string                          `json:"main_image_url"`
	Gallery            []string                        `json:"gallery"`
	Status             models.ReservationProductStatus `json:"status"`
	ReservationStartAt *time.Time                      `json:"reservation_start_at"`
	ReservationEndAt   *time.Time                      `json:"reservation_end_at"`
	DisplayOrder       int                             `json:"display_order"`
	IsFeatured         bool                            `json:"is_featured"`
	HeatLevelOverride  *models.ReservationHeatLevel    `json:"heat_level_override"`
}

type reservationInterestPayload struct {
	VisitorID      string                       `json:"visitor_id"`
	SessionID      *string                      `json:"session_id"`
	IdempotencyKey string                       `json:"idempotency_key"`
	SourceType     models.ReservationSourceType `json:"source_type"`
	SourceID       *string                      `json:"source_id"`
}

type reservationOTPRequestPayload struct {
	VisitorID      string                        `json:"visitor_id"`
	IdempotencyKey string                        `json:"idempotency_key"`
	ContactType    models.ReservationContactType `json:"contact_type"`
	PhoneCountry   string                        `json:"phone_country"`
	PhoneNumber    string                        `json:"phone_number"`
	TurnstileToken string                        `json:"turnstile_token"`
	SourceType     models.ReservationSourceType  `json:"source_type"`
	SourceID       *string                       `json:"source_id"`
}

type reservationOTPVerifyPayload struct {
	ChallengeID    string `json:"challenge_id"`
	Code           string `json:"code"`
	IdempotencyKey string `json:"idempotency_key"`
}

type reservationCancelPayload struct {
	ManageToken    string `json:"manage_token"`
	IdempotencyKey string `json:"idempotency_key"`
}

type anonymousReservationCreatePayload struct {
	ProductID            uint64 `json:"product_id"`
	ReservationProductID uint64 `json:"reservation_product_id"`
	Quantity             int    `json:"quantity"`
	Note                 string `json:"note"`
	TurnstileToken       string `json:"turnstile_token"`
}

type anonymousReservationStatusPayload struct {
	Status string `json:"status"`
}

type anonymousReservationRiskPayload struct {
	Flagged bool `json:"flagged"`
}

func NewReservationController(reservationService reservationService, options ...ReservationControllerOptions) *ReservationController {
	controller := &ReservationController{service: reservationService, mode: "disabled"}
	if len(options) > 0 {
		controller.mode = strings.ToLower(strings.TrimSpace(options[0].Mode))
		if controller.mode != "anonymous" && controller.mode != "otp" {
			controller.mode = "disabled"
		}
		controller.workflow = options[0].Workflow
		controller.anonymous = options[0].Anonymous
		controller.browserIdentity = options[0].BrowserIdentity
		controller.security = options[0].Security
	}
	return controller
}

func (ctl *ReservationController) RegisterPublicRoutes(group *gin.RouterGroup) {
	group.GET("/storefront/reservation-capabilities", ctl.Capabilities)
	if ctl.mode == "disabled" {
		return
	}
	products := group.Group("/storefront/reservation-products")
	products.GET("", ctl.PublicList)
	products.GET("/:slug", ctl.PublicDetail)
	products.POST("/:id/interest", ctl.Interest)
	if ctl.mode == "otp" {
		products.POST("/:id/otp/request", ctl.RequestOTP)
		products.POST("/:id/otp/verify", ctl.VerifyOTP)
		products.POST("/:id/cancel", ctl.Cancel)
	}
	if ctl.mode == "anonymous" {
		products.POST("/:id/anonymous", ctl.CreateAnonymous)
		group.POST("/storefront/reservations", ctl.CreateAnonymous)
		group.GET("/storefront/my-reservations", ctl.MyAnonymousReservations)
		group.POST("/storefront/my-reservations/:code/cancel", ctl.CancelAnonymous)
	}
}

func (ctl *ReservationController) RegisterSuperRoutes(group *gin.RouterGroup) {
	if ctl.mode == "disabled" {
		return
	}
	products := group.Group("/reservation-products")
	products.GET("", ctl.AdminList)
	products.POST("", ctl.Create)
	products.GET("/:id", ctl.AdminGet)
	products.PUT("/:id", ctl.Update)
	products.DELETE("/:id", ctl.Archive)
	products.GET("/:id/analytics", ctl.Analytics)
	if ctl.anonymous != nil {
		group.GET("/reservations", ctl.AdminReservations)
		group.POST("/reservations/expire", ctl.AdminExpireReservations)
		group.PATCH("/reservations/:code/status", ctl.AdminReservationStatus)
		group.PATCH("/reservations/:code/risk", ctl.AdminReservationRisk)
	}
}

func (ctl *ReservationController) PublicList(c *gin.Context) {
	featured, ok := parseOptionalReservationBool(c, "featured")
	if !ok {
		return
	}
	items, err := ctl.service.PublicProducts(c.Request.Context(), service.ReservationPublicQuery{
		Page: parsePositiveInt(c.Query("page"), 1), PageSize: parsePositiveInt(c.Query("page_size"), 20),
		Search: c.Query("search"), Featured: featured, Locale: parseLocaleQuery(c), VisitorID: reservationVisitorID(c),
	})
	if err != nil {
		writeReservationError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: items})
}

func (ctl *ReservationController) PublicDetail(c *gin.Context) {
	item, err := ctl.service.PublicProduct(c.Request.Context(), c.Param("slug"), parseLocaleQuery(c), reservationVisitorID(c))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: item})
}

func (ctl *ReservationController) Interest(c *gin.Context) {
	id, ok := parseUint64Param(c, "id")
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxReservationInterestBodyBytes)
	var payload reservationInterestPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: "invalid request"})
		return
	}
	result, err := ctl.service.RecordInterest(c.Request.Context(), id, service.ReservationInterestInput{
		VisitorID: payload.VisitorID, SessionID: reservationOptionalPayloadString(payload.SessionID),
		IdempotencyKey: payload.IdempotencyKey, SourceType: payload.SourceType,
		SourceID:  reservationOptionalPayloadString(payload.SourceID),
		IPRiskKey: ctl.ipRiskKey(c),
	})
	if err != nil {
		writeReservationError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (ctl *ReservationController) Capabilities(c *gin.Context) {
	if ctl.mode == "disabled" {
		c.JSON(http.StatusOK, response{Data: service.ReservationCapabilities{
			ReservationEnabled: false,
			ReservationMode:    "disabled",
			OTPChannels:        []models.ReservationContactType{},
			TurnstileEnabled:   false,
		}})
		return
	}
	if ctl.mode == "anonymous" {
		c.JSON(http.StatusOK, response{Data: service.ReservationCapabilities{
			ReservationEnabled: true,
			ReservationMode:    "anonymous",
			OTPChannels:        []models.ReservationContactType{},
			TurnstileEnabled:   true,
		}})
		return
	}
	if ctl.workflow == nil {
		c.JSON(http.StatusServiceUnavailable, response{Error: "service unavailable"})
		return
	}
	c.JSON(http.StatusOK, response{Data: ctl.workflow.Capabilities()})
}

func (ctl *ReservationController) CreateAnonymous(c *gin.Context) {
	if ctl.mode != "anonymous" || ctl.anonymous == nil {
		c.JSON(http.StatusNotFound, response{Error: "reservation feature is disabled", Code: "RESERVATION_DISABLED"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxReservationInterestBodyBytes)
	var payload anonymousReservationCreatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: "invalid request", Code: "INVALID_REQUEST"})
		return
	}
	productID := payload.ReservationProductID
	if productID == 0 {
		productID = payload.ProductID
	}
	if pathID := strings.TrimSpace(c.Param("id")); pathID != "" {
		parsed, err := strconv.ParseUint(pathID, 10, 64)
		if err != nil || parsed == 0 ||
			(productID != 0 && productID != parsed) {
			c.JSON(http.StatusBadRequest, response{Error: "invalid product id", Code: "INVALID_REQUEST"})
			return
		}
		productID = parsed
	}
	if productID == 0 {
		c.JSON(http.StatusBadRequest, response{Error: "invalid product id", Code: "INVALID_REQUEST"})
		return
	}
	browserHash, ok := ctl.anonymousBrowserHash(c)
	if !ok {
		return
	}
	remoteIP := ctl.clientIP(c)
	result, err := ctl.anonymous.Create(c.Request.Context(), service.AnonymousReservationCreateInput{
		ProductID: productID, Quantity: payload.Quantity, Note: payload.Note,
		TurnstileToken: payload.TurnstileToken, BrowserHash: browserHash,
		IPHash: ctl.ipHash(c), RemoteIP: remoteIP, Locale: parseLocaleQuery(c),
	})
	if err != nil {
		writeAnonymousReservationError(c, err)
		return
	}
	c.JSON(http.StatusCreated, response{Data: result})
}

func (ctl *ReservationController) MyAnonymousReservations(c *gin.Context) {
	if ctl.mode != "anonymous" || ctl.anonymous == nil {
		c.JSON(http.StatusNotFound, response{Error: "reservation feature is disabled", Code: "RESERVATION_DISABLED"})
		return
	}
	browserHash, ok := ctl.anonymousBrowserHash(c)
	if !ok {
		return
	}
	items, err := ctl.anonymous.Mine(c.Request.Context(), browserHash, parseLocaleQuery(c))
	if err != nil {
		writeAnonymousReservationError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: items})
}

func (ctl *ReservationController) CancelAnonymous(c *gin.Context) {
	if ctl.mode != "anonymous" || ctl.anonymous == nil {
		c.JSON(http.StatusNotFound, response{Error: "reservation feature is disabled", Code: "RESERVATION_DISABLED"})
		return
	}
	browserHash, ok := ctl.anonymousBrowserHash(c)
	if !ok {
		return
	}
	result, err := ctl.anonymous.Cancel(
		c.Request.Context(), c.Param("code"), browserHash, parseLocaleQuery(c),
	)
	if err != nil {
		writeAnonymousReservationError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: result})
}

func (ctl *ReservationController) RequestOTP(c *gin.Context) {
	id, ok := parseUint64Param(c, "id")
	if !ok {
		return
	}
	if ctl.workflow == nil {
		c.JSON(http.StatusServiceUnavailable, response{Error: "service unavailable"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxReservationInterestBodyBytes)
	var payload reservationOTPRequestPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: "invalid request"})
		return
	}
	result, err := ctl.workflow.RequestOTP(c.Request.Context(), id, service.ReservationOTPRequestInput{
		VisitorID: payload.VisitorID, IdempotencyKey: payload.IdempotencyKey,
		ContactType: payload.ContactType, PhoneCountry: payload.PhoneCountry, PhoneNumber: payload.PhoneNumber,
		TurnstileToken: payload.TurnstileToken, SourceType: payload.SourceType,
		SourceID: reservationOptionalPayloadString(payload.SourceID), Locale: parseLocaleQuery(c),
		IPRiskKey: ctl.ipRiskKey(c), RemoteIP: ctl.remoteIP(c),
	})
	if err != nil {
		writeReservationError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (ctl *ReservationController) VerifyOTP(c *gin.Context) {
	id, ok := parseUint64Param(c, "id")
	if !ok {
		return
	}
	if ctl.workflow == nil {
		c.JSON(http.StatusServiceUnavailable, response{Error: "service unavailable"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxReservationInterestBodyBytes)
	var payload reservationOTPVerifyPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: "invalid request"})
		return
	}
	result, err := ctl.workflow.VerifyOTP(c.Request.Context(), id, service.ReservationOTPVerifyInput{
		ChallengeID: payload.ChallengeID, Code: payload.Code, IdempotencyKey: payload.IdempotencyKey,
		IPRiskKey: ctl.ipRiskKey(c),
	})
	if err != nil {
		writeReservationError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (ctl *ReservationController) Cancel(c *gin.Context) {
	id, ok := parseUint64Param(c, "id")
	if !ok {
		return
	}
	if ctl.workflow == nil {
		c.JSON(http.StatusServiceUnavailable, response{Error: "service unavailable"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxReservationInterestBodyBytes)
	var payload reservationCancelPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: "invalid request"})
		return
	}
	result, err := ctl.workflow.Cancel(c.Request.Context(), id, service.ReservationCancelInput{
		ManageToken: payload.ManageToken, IdempotencyKey: payload.IdempotencyKey, IPRiskKey: ctl.ipRiskKey(c),
	})
	if err != nil {
		writeReservationError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (ctl *ReservationController) AdminList(c *gin.Context) {
	var status *models.ReservationProductStatus
	if value := strings.TrimSpace(c.Query("status")); value != "" {
		parsed := models.ReservationProductStatus(value)
		status = &parsed
	}
	items, err := ctl.service.AdminProducts(c.Request.Context(), service.ReservationAdminQuery{
		Page: parsePositiveInt(c.Query("page"), 1), PageSize: parsePositiveInt(c.Query("page_size"), 20),
		Search: c.Query("search"), Status: status,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: items})
}

func (ctl *ReservationController) AdminGet(c *gin.Context) {
	id, ok := parseUint64Param(c, "id")
	if !ok {
		return
	}
	item, err := ctl.service.AdminProduct(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: item})
}

func (ctl *ReservationController) Create(c *gin.Context) {
	userID, ok := middleware.UserIDFromContext(c)
	if !ok || userID == 0 {
		writeError(c, service.ErrForbidden)
		return
	}
	payload, ok := bindReservationProductPayload(c)
	if !ok {
		return
	}
	item, err := ctl.service.CreateProduct(c.Request.Context(), payload.toInput(userID))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, response{Data: item})
}

func (ctl *ReservationController) Update(c *gin.Context) {
	id, ok := parseUint64Param(c, "id")
	if !ok {
		return
	}
	userID, authenticated := middleware.UserIDFromContext(c)
	if !authenticated || userID == 0 {
		writeError(c, service.ErrForbidden)
		return
	}
	payload, ok := bindReservationProductPayload(c)
	if !ok {
		return
	}
	item, err := ctl.service.UpdateProduct(c.Request.Context(), id, payload.toInput(userID))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: item})
}

func (ctl *ReservationController) Archive(c *gin.Context) {
	id, ok := parseUint64Param(c, "id")
	if !ok {
		return
	}
	if err := ctl.service.ArchiveProduct(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (ctl *ReservationController) Analytics(c *gin.Context) {
	id, ok := parseUint64Param(c, "id")
	if !ok {
		return
	}
	analytics, err := ctl.service.ProductAnalytics(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: analytics})
}

func (ctl *ReservationController) AdminReservations(c *gin.Context) {
	if ctl.anonymous == nil {
		c.JSON(http.StatusServiceUnavailable, response{Error: "service unavailable"})
		return
	}
	var riskFlagged *bool
	if value := strings.TrimSpace(c.Query("risk_flagged")); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			c.JSON(http.StatusBadRequest, response{Error: "invalid risk_flagged"})
			return
		}
		riskFlagged = &parsed
	}
	items, err := ctl.anonymous.AdminReservations(c.Request.Context(), service.AnonymousReservationAdminQuery{
		Page: parsePositiveInt(c.Query("page"), 1), PageSize: parsePositiveInt(c.Query("page_size"), 20),
		IdentityType: c.Query("identity_type"), Status: c.Query("status"), RiskFlagged: riskFlagged,
	})
	if err != nil {
		writeAnonymousReservationError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: items})
}

func (ctl *ReservationController) AdminExpireReservations(c *gin.Context) {
	if ctl.anonymous == nil {
		c.JSON(http.StatusServiceUnavailable, response{Error: "service unavailable"})
		return
	}
	expired, err := ctl.anonymous.ExpireDue(c.Request.Context(), 1000)
	if err != nil {
		writeAnonymousReservationError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: gin.H{"expired": expired}})
}

func (ctl *ReservationController) AdminReservationStatus(c *gin.Context) {
	if ctl.anonymous == nil {
		c.JSON(http.StatusServiceUnavailable, response{Error: "service unavailable"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxReservationInterestBodyBytes)
	var payload anonymousReservationStatusPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: "invalid request"})
		return
	}
	item, err := ctl.anonymous.AdminSetStatus(c.Request.Context(), c.Param("code"), payload.Status)
	if err != nil {
		writeAnonymousReservationError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: item})
}

func (ctl *ReservationController) AdminReservationRisk(c *gin.Context) {
	if ctl.anonymous == nil {
		c.JSON(http.StatusServiceUnavailable, response{Error: "service unavailable"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxReservationInterestBodyBytes)
	var payload anonymousReservationRiskPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: "invalid request"})
		return
	}
	item, err := ctl.anonymous.AdminSetRisk(c.Request.Context(), c.Param("code"), payload.Flagged)
	if err != nil {
		writeAnonymousReservationError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Data: item})
}

func bindReservationProductPayload(c *gin.Context) (reservationProductPayload, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxReservationAdminBodyBytes)
	var payload reservationProductPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: "invalid request"})
		return reservationProductPayload{}, false
	}
	return payload, true
}

func (payload reservationProductPayload) toInput(userID uint) service.ReservationProductInput {
	return service.ReservationProductInput{
		LinkedProductID: payload.LinkedProductID, Slug: payload.Slug, TitleEN: payload.TitleEN, TitleZH: payload.TitleZH,
		SummaryEN: payload.SummaryEN, SummaryZH: payload.SummaryZH, DescriptionEN: payload.DescriptionEN,
		DescriptionZH: payload.DescriptionZH, MainImageURL: payload.MainImageURL, Gallery: payload.Gallery,
		Status: payload.Status, ReservationStartAt: payload.ReservationStartAt, ReservationEndAt: payload.ReservationEndAt,
		DisplayOrder: payload.DisplayOrder, IsFeatured: payload.IsFeatured, HeatLevelOverride: payload.HeatLevelOverride,
		CreatedBy: userID,
	}
}

func parseUint64Param(c *gin.Context, name string) (uint64, bool) {
	value, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || value == 0 {
		c.JSON(http.StatusBadRequest, response{Error: "invalid " + name})
		return 0, false
	}
	return value, true
}

func parseOptionalReservationBool(c *gin.Context, name string) (*bool, bool) {
	value := strings.TrimSpace(c.Query(name))
	if value == "" {
		return nil, true
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		c.JSON(http.StatusBadRequest, response{Error: "invalid " + name})
		return nil, false
	}
	return &parsed, true
}

func reservationVisitorID(c *gin.Context) string {
	if value := strings.TrimSpace(c.Query("visitor_id")); value != "" {
		return value
	}
	return strings.TrimSpace(c.GetHeader("X-GC-Visitor-ID"))
}

func reservationOptionalPayloadString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (ctl *ReservationController) ipRiskKey(c *gin.Context) string {
	if ctl.security == nil {
		return ""
	}
	return ctl.security.IPRiskKeyForRequest(
		c.Request.RemoteAddr,
		c.GetHeader("CF-Connecting-IP"),
		c.GetHeader("X-Real-IP"),
		c.Request.Header.Values("X-Forwarded-For")...,
	)
}

func (ctl *ReservationController) remoteIP(c *gin.Context) string {
	return ctl.clientIP(c)
}

func (ctl *ReservationController) clientIP(c *gin.Context) string {
	if ctl.security == nil {
		return ""
	}
	return ctl.security.ClientIPWithHeaders(
		c.Request.RemoteAddr,
		c.GetHeader("CF-Connecting-IP"),
		c.GetHeader("X-Real-IP"),
		c.Request.Header.Values("X-Forwarded-For")...,
	)
}

func (ctl *ReservationController) ipHash(c *gin.Context) string {
	if ctl.security == nil {
		return ""
	}
	return ctl.security.IPHashWithHeaders(
		c.Request.RemoteAddr,
		c.GetHeader("CF-Connecting-IP"),
		c.GetHeader("X-Real-IP"),
		c.Request.Header.Values("X-Forwarded-For")...,
	)
}

func (ctl *ReservationController) anonymousBrowserHash(c *gin.Context) (string, bool) {
	if ctl.browserIdentity == nil {
		c.JSON(http.StatusServiceUnavailable, response{Error: "service unavailable"})
		return "", false
	}
	token := ""
	if cookie, err := c.Request.Cookie(service.ReservationDeviceCookieName); err == nil {
		token = cookie.Value
	}
	if !ctl.browserIdentity.ValidToken(token) {
		var err error
		token, err = ctl.browserIdentity.NewToken()
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, response{Error: "service unavailable"})
			return "", false
		}
		http.SetCookie(c.Writer, &http.Cookie{
			Name: service.ReservationDeviceCookieName, Value: token,
			Path: "/", MaxAge: service.ReservationDeviceMaxAge,
			HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
		})
	}
	hash := ctl.browserIdentity.HashToken(token)
	if hash == "" {
		c.JSON(http.StatusServiceUnavailable, response{Error: "service unavailable"})
		return "", false
	}
	return hash, true
}

func writeReservationError(c *gin.Context, err error) {
	var limitError *service.RateLimitError
	if errors.As(err, &limitError) && limitError.RetryAfterSeconds > 0 {
		c.Header("Retry-After", strconv.Itoa(limitError.RetryAfterSeconds))
	}
	writeError(c, err)
}

func writeAnonymousReservationError(c *gin.Context, err error) {
	var duplicate *service.AnonymousReservationDuplicateError
	if errors.As(err, &duplicate) {
		c.JSON(http.StatusConflict, response{
			Data: duplicate.Existing, Error: err.Error(), Code: "RESERVATION_ALREADY_EXISTS",
		})
		return
	}
	var limit *service.AnonymousReservationRateLimitError
	if errors.As(err, &limit) {
		if limit.RetryAfterSeconds > 0 {
			c.Header("Retry-After", strconv.Itoa(limit.RetryAfterSeconds))
		}
		code := "IP_RATE_LIMITED"
		if limit.Scope == "browser" {
			code = "BROWSER_RATE_LIMITED"
		}
		c.JSON(http.StatusTooManyRequests, response{Error: err.Error(), Code: code})
		return
	}
	switch {
	case errors.Is(err, service.ErrTurnstileRejected):
		c.JSON(http.StatusBadRequest, response{Error: err.Error(), Code: "TURNSTILE_FAILED"})
	case errors.Is(err, service.ErrAnonymousProductNotReservable):
		c.JSON(http.StatusConflict, response{Error: err.Error(), Code: "PRODUCT_NOT_RESERVABLE"})
	case errors.Is(err, service.ErrAnonymousReservationNotPending):
		c.JSON(http.StatusConflict, response{Error: err.Error(), Code: "RESERVATION_NOT_PENDING"})
	case errors.Is(err, service.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, response{Error: err.Error(), Code: "INVALID_REQUEST"})
	default:
		writeError(c, err)
	}
}
