package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"saas/internal/middleware"
	"saas/internal/models"
	"saas/internal/repository"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestReservationControllerPublicInterestAndSuperOnlyCRUD(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:reservation-controller-%s?mode=memory&cache=shared", t.Name())),
		&gorm.Config{DisableForeignKeyConstraintWhenMigrating: true, Logger: logger.Default.LogMode(logger.Silent)},
	)
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.SPU{}, &models.ReservationProduct{}, &models.ReservationInterest{},
		&models.ProductReservation{}, &models.ReservationAttemptEvent{},
	); err != nil {
		t.Fatalf("migrate reservation models: %v", err)
	}
	fixedNow := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	reservationService := service.NewReservationService(
		repository.NewReservationProductRepository(db),
		service.ReservationServiceOptions{Now: func() time.Time { return fixedNow }},
	)
	protector, err := service.NewReservationContactProtector(
		"controller-contact-hash-secret-0123456789",
		[]byte("abcdef0123456789abcdef0123456789"),
	)
	if err != nil {
		t.Fatalf("contact protector: %v", err)
	}
	security, err := service.NewReservationRequestSecurity("controller-ip-hash-secret-0123456789abcd")
	if err != nil {
		t.Fatalf("request security: %v", err)
	}
	workflow, err := service.NewReservationWorkflowService(
		repository.NewReservationWorkflowRepository(db),
		service.ReservationWorkflowOptions{
			Now: func() time.Time { return fixedNow }, Protector: protector,
			Turnstile:   service.NewMockTurnstileVerifier(false, func() time.Time { return fixedNow }),
			OTPProvider: service.NewMockOTPProvider(), EnabledChannels: map[string]bool{"sms": true},
			ChallengeTTL: 10 * time.Minute, ResendCooldown: time.Minute, MaxVerifyAttempts: 5,
		},
	)
	if err != nil {
		t.Fatalf("workflow service: %v", err)
	}
	ctl := NewReservationController(reservationService, ReservationControllerOptions{Mode: "otp", Workflow: workflow, Security: security})
	router := gin.New()
	ctl.RegisterPublicRoutes(router.Group("/api/v1"))
	super := router.Group("/api/v1/super")
	super.Use(requireControllerTestAuth("secret"))
	super.Use(middleware.RequireRole(models.UserRoleSuperAdmin))
	ctl.RegisterSuperRoutes(super)

	superToken := holidayControllerToken(t, models.UserRoleSuperAdmin, nil)
	createBody := `{"slug":"controller-product","title_en":"Controller Product","title_zh":"控制器测品","description_en":"<script>bad()</script><p>Safe</p>","status":"open","gallery":[]}`
	createResponse := reservationRequest(router, http.MethodPost, "/api/v1/super/reservation-products", createBody, superToken)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createResponse.Code, createResponse.Body.String())
	}
	var created struct {
		Data service.ReservationProductAdminDTO `json:"data"`
	}
	if err := json.Unmarshal(createResponse.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created product: %v", err)
	}
	if created.Data.ID == 0 || strings.Contains(created.Data.DescriptionEN, "script") {
		t.Fatalf("created product = %+v", created.Data)
	}

	tenantID := uint(99)
	tenantToken := holidayControllerToken(t, models.UserRoleTenantAdmin, &tenantID)
	tenantResponse := reservationRequest(router, http.MethodPost, "/api/v1/super/reservation-products", createBody, tenantToken)
	if tenantResponse.Code != http.StatusForbidden {
		t.Fatalf("tenant create status = %d, want forbidden", tenantResponse.Code)
	}
	for _, role := range []models.UserRole{models.UserRoleFinance, models.UserRolePicker} {
		roleToken := holidayControllerToken(t, role, &tenantID)
		roleResponse := reservationRequest(router, http.MethodGet, "/api/v1/super/reservation-products", "", roleToken)
		if roleResponse.Code != http.StatusForbidden {
			t.Fatalf("%s super reservation status = %d, want forbidden", role, roleResponse.Code)
		}
	}
	unauthenticatedSuper := reservationRequest(router, http.MethodGet, "/api/v1/super/reservation-products", "", "")
	if unauthenticatedSuper.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated super reservation status = %d, want unauthorized", unauthenticatedSuper.Code)
	}

	listResponse := reservationRequest(router, http.MethodGet, "/api/v1/storefront/reservation-products?locale=zh", "", "")
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), "控制器测品") {
		t.Fatalf("public list = %d/%s", listResponse.Code, listResponse.Body.String())
	}
	for _, forbidden := range []string{"exact_interest_count", "exact_verified_count", "linked_product_id", "created_by", "contact_hash", "contact_encrypted", "provider_reference", "price", "stock"} {
		if strings.Contains(listResponse.Body.String(), forbidden) {
			t.Fatalf("public list exposed %q: %s", forbidden, listResponse.Body.String())
		}
	}

	interestBody := `{"visitor_id":"11111111-1111-4111-8111-111111111111","session_id":"22222222-2222-4222-8222-222222222222","idempotency_key":"33333333-3333-4333-8333-333333333333","source_type":"reservation_list","source_id":null}`
	interestPath := fmt.Sprintf("/api/v1/storefront/reservation-products/%d/interest", created.Data.ID)
	interestResponse := reservationRequest(router, http.MethodPost, interestPath, interestBody, "")
	if interestResponse.Code != http.StatusOK || strings.TrimSpace(interestResponse.Body.String()) != `{"interested":true,"created":true}` {
		t.Fatalf("first interest = %d/%s", interestResponse.Code, interestResponse.Body.String())
	}
	repeatResponse := reservationRequest(router, http.MethodPost, interestPath, interestBody, "")
	if repeatResponse.Code != http.StatusOK || strings.TrimSpace(repeatResponse.Body.String()) != `{"interested":true,"created":false}` {
		t.Fatalf("repeat interest = %d/%s", repeatResponse.Code, repeatResponse.Body.String())
	}

	detailResponse := reservationRequest(
		router, http.MethodGet,
		"/api/v1/storefront/reservation-products/controller-product?visitor_id=11111111-1111-4111-8111-111111111111", "", "",
	)
	if detailResponse.Code != http.StatusOK || !strings.Contains(detailResponse.Body.String(), `"interested":true`) || strings.Contains(detailResponse.Body.String(), "exact_interest_count") {
		t.Fatalf("public detail = %d/%s", detailResponse.Code, detailResponse.Body.String())
	}

	analyticsPath := fmt.Sprintf("/api/v1/super/reservation-products/%d/analytics", created.Data.ID)
	analyticsResponse := reservationRequest(router, http.MethodGet, analyticsPath, "", superToken)
	if analyticsResponse.Code != http.StatusOK || !strings.Contains(analyticsResponse.Body.String(), `"unique_interests":1`) {
		t.Fatalf("analytics = %d/%s", analyticsResponse.Code, analyticsResponse.Body.String())
	}
	tenantAnalytics := reservationRequest(router, http.MethodGet, analyticsPath, "", tenantToken)
	if tenantAnalytics.Code != http.StatusForbidden {
		t.Fatalf("tenant analytics status = %d, want forbidden", tenantAnalytics.Code)
	}

	capabilities := reservationRequest(router, http.MethodGet, "/api/v1/storefront/reservation-capabilities", "", "")
	if capabilities.Code != http.StatusOK || !strings.Contains(capabilities.Body.String(), `"reservation_enabled":true`) || !strings.Contains(capabilities.Body.String(), `"otp_channels":["sms"]`) || strings.Contains(capabilities.Body.String(), "whatsapp") {
		t.Fatalf("capabilities = %d/%s", capabilities.Code, capabilities.Body.String())
	}
	otpRequestBody := `{"visitor_id":"11111111-1111-4111-8111-111111111111","idempotency_key":"44444444-4444-4444-8444-444444444444","contact_type":"sms","phone_country":"GH","phone_number":"024 123 4567","turnstile_token":"test-turnstile-controller","source_type":"reservation_detail","source_id":null}`
	otpRequestPath := fmt.Sprintf("/api/v1/storefront/reservation-products/%d/otp/request", created.Data.ID)
	otpResponse := reservationRequest(router, http.MethodPost, otpRequestPath, otpRequestBody, "")
	if otpResponse.Code != http.StatusOK {
		t.Fatalf("OTP request = %d/%s", otpResponse.Code, otpResponse.Body.String())
	}
	for _, secret := range []string{"024 123 4567", "+233241234567", "test-turnstile-controller", "contact_hash", "contact_encrypted"} {
		if strings.Contains(otpResponse.Body.String(), secret) {
			t.Fatalf("OTP response exposed %q: %s", secret, otpResponse.Body.String())
		}
	}
	var challenge service.ReservationOTPRequestResult
	if err := json.Unmarshal(otpResponse.Body.Bytes(), &challenge); err != nil || challenge.ChallengeID == "" {
		t.Fatalf("decode OTP challenge = %+v/%v", challenge, err)
	}
	otpVerifyBody := fmt.Sprintf(`{"challenge_id":%q,"code":"246810","idempotency_key":"55555555-5555-4555-8555-555555555555"}`, challenge.ChallengeID)
	otpVerifyPath := fmt.Sprintf("/api/v1/storefront/reservation-products/%d/otp/verify", created.Data.ID)
	verifyResponse := reservationRequest(router, http.MethodPost, otpVerifyPath, otpVerifyBody, "")
	if verifyResponse.Code != http.StatusOK {
		t.Fatalf("OTP verify = %d/%s", verifyResponse.Code, verifyResponse.Body.String())
	}
	var verified service.ReservationOTPVerifyResult
	if err := json.Unmarshal(verifyResponse.Body.Bytes(), &verified); err != nil || !verified.Reserved || verified.ManageToken == "" {
		t.Fatalf("decode verified reservation = %+v/%v", verified, err)
	}
	if strings.Contains(verifyResponse.Body.String(), "+233") || strings.Contains(verifyResponse.Body.String(), "246810") {
		t.Fatalf("verify response exposed contact or OTP: %s", verifyResponse.Body.String())
	}
	verifiedRetryBody := `{"visitor_id":"77777777-7777-4777-8777-777777777777","idempotency_key":"77777777-7777-4777-8777-777777777778","contact_type":"sms","phone_country":"GH","phone_number":"0241234567","turnstile_token":"test-turnstile-verified-retry","source_type":"reservation_detail","source_id":null}`
	verifiedRetry := reservationRequest(router, http.MethodPost, otpRequestPath, verifiedRetryBody, "")
	if verifiedRetry.Code != http.StatusOK {
		t.Fatalf("verified contact retry = %d/%s", verifiedRetry.Code, verifiedRetry.Body.String())
	}
	verifiedAnalytics := reservationRequest(router, http.MethodGet, analyticsPath, "", superToken)
	if verifiedAnalytics.Code != http.StatusOK || !strings.Contains(verifiedAnalytics.Body.String(), `"verified_reservations":1`) || !strings.Contains(verifiedAnalytics.Body.String(), `"masked_phone":"+233 *** *** 4567"`) || strings.Contains(verifiedAnalytics.Body.String(), "+233241234567") {
		t.Fatalf("verified analytics = %d/%s", verifiedAnalytics.Code, verifiedAnalytics.Body.String())
	}
	cancelBody := fmt.Sprintf(`{"manage_token":%q,"idempotency_key":"66666666-6666-4666-8666-666666666666"}`, verified.ManageToken)
	cancelPath := fmt.Sprintf("/api/v1/storefront/reservation-products/%d/cancel", created.Data.ID)
	cancelResponse := reservationRequest(router, http.MethodPost, cancelPath, cancelBody, "")
	if cancelResponse.Code != http.StatusOK || !strings.Contains(cancelResponse.Body.String(), `"cancelled":true`) || strings.Contains(cancelResponse.Body.String(), verified.ManageToken) {
		t.Fatalf("cancel response = %d/%s", cancelResponse.Code, cancelResponse.Body.String())
	}
	cancelledRetryBody := `{"visitor_id":"88888888-8888-4888-8888-888888888888","idempotency_key":"88888888-8888-4888-8888-888888888889","contact_type":"sms","phone_country":"GH","phone_number":"0241234567","turnstile_token":"test-turnstile-cancelled-retry","source_type":"reservation_detail","source_id":null}`
	cancelledRetry := reservationRequest(router, http.MethodPost, otpRequestPath, cancelledRetryBody, "")
	newContactBody := `{"visitor_id":"99999999-9999-4999-8999-999999999999","idempotency_key":"99999999-9999-4999-8999-999999999998","contact_type":"sms","phone_country":"GH","phone_number":"0201234567","turnstile_token":"test-turnstile-new-contact","source_type":"reservation_detail","source_id":null}`
	newContact := reservationRequest(router, http.MethodPost, otpRequestPath, newContactBody, "")
	for name, response := range map[string]*httptest.ResponseRecorder{"verified": verifiedRetry, "cancelled": cancelledRetry, "new": newContact} {
		if response.Code != http.StatusOK {
			t.Fatalf("%s contact request status/body = %d/%s", name, response.Code, response.Body.String())
		}
		var opaque service.ReservationOTPRequestResult
		if err := json.Unmarshal(response.Body.Bytes(), &opaque); err != nil || len(opaque.ChallengeID) != 43 || opaque.Status != "pending" || opaque.ResendAfterSeconds != 60 {
			t.Fatalf("%s opaque request response = %+v/%v", name, opaque, err)
		}
		for _, sensitive := range []string{"0241234567", "0201234567", "+233", "verified", "cancelled", "contact_hash", "provider"} {
			if strings.Contains(response.Body.String(), sensitive) {
				t.Fatalf("%s request leaked state/contact %q: %s", name, sensitive, response.Body.String())
			}
		}
	}
}

func TestReservationControllerDisabledRegistersOnlyCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:reservation-disabled-controller-%s?mode=memory&cache=shared", t.Name())),
		&gorm.Config{DisableForeignKeyConstraintWhenMigrating: true, Logger: logger.Default.LogMode(logger.Silent)},
	)
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.SPU{}, &models.ReservationProduct{}, &models.ReservationInterest{},
		&models.ProductReservation{}, &models.ReservationAttemptEvent{},
	); err != nil {
		t.Fatalf("migrate reservation models: %v", err)
	}

	ctl := NewReservationController(nil, ReservationControllerOptions{Mode: "disabled"})
	router := gin.New()
	ctl.RegisterPublicRoutes(router.Group("/api/v1"))
	ctl.RegisterSuperRoutes(router.Group("/api/v1/super"))

	capabilities := reservationRequest(router, http.MethodGet, "/api/v1/storefront/reservation-capabilities", "", "")
	if capabilities.Code != http.StatusOK {
		t.Fatalf("capabilities status = %d, body = %s", capabilities.Code, capabilities.Body.String())
	}
	var payload struct {
		Data service.ReservationCapabilities `json:"data"`
	}
	if err := json.Unmarshal(capabilities.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode capabilities: %v", err)
	}
	if payload.Data.ReservationEnabled || len(payload.Data.OTPChannels) != 0 {
		t.Fatalf("disabled capabilities = %+v", payload.Data)
	}
	for _, forbidden := range []string{"mock", "sms", "whatsapp"} {
		if strings.Contains(strings.ToLower(capabilities.Body.String()), forbidden) {
			t.Fatalf("disabled capabilities exposed %q: %s", forbidden, capabilities.Body.String())
		}
	}

	disabledWrites := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/v1/storefront/reservation-products/1/interest", `{}`},
		{http.MethodPost, "/api/v1/storefront/reservation-products/1/otp/request", `{}`},
		{http.MethodPost, "/api/v1/storefront/reservation-products/1/otp/verify", `{}`},
		{http.MethodPost, "/api/v1/storefront/reservation-products/1/cancel", `{}`},
		{http.MethodPost, "/api/v1/super/reservation-products", `{}`},
	}
	for _, request := range disabledWrites {
		response := reservationRequest(router, request.method, request.path, request.body, "")
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s %s status = %d, want 404; body = %s", request.method, request.path, response.Code, response.Body.String())
		}
	}

	for name, model := range map[string]any{
		"products":     &models.ReservationProduct{},
		"interests":    &models.ReservationInterest{},
		"reservations": &models.ProductReservation{},
		"attempts":     &models.ReservationAttemptEvent{},
	} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil {
			t.Fatalf("count %s: %v", name, err)
		}
		if count != 0 {
			t.Fatalf("%s records = %d, want 0", name, count)
		}
	}
}

func TestReservationControllerAnonymousCookieProxyDuplicateMineAndCancel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:reservation-anonymous-controller-%s?mode=memory&cache=shared&_pragma=busy_timeout(5000)", t.Name())),
		&gorm.Config{DisableForeignKeyConstraintWhenMigrating: true, Logger: logger.Default.LogMode(logger.Silent)},
	)
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.SPU{}, &models.ReservationProduct{}, &models.ReservationInterest{},
		&models.ProductReservation{}, &models.ReservationAttemptEvent{},
	); err != nil {
		t.Fatalf("migrate reservation models: %v", err)
	}
	fixedNow := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	productService := service.NewReservationService(
		repository.NewReservationProductRepository(db),
		service.ReservationServiceOptions{Now: func() time.Time { return fixedNow }},
	)
	product, err := productService.CreateProduct(t.Context(), service.ReservationProductInput{
		Slug: "anonymous-controller", TitleEN: "Anonymous product",
		Status: models.ReservationProductStatusOpen, CreatedBy: 1,
	})
	if err != nil {
		t.Fatalf("create reservation product: %v", err)
	}
	anonymous, err := service.NewAnonymousReservationService(
		repository.NewAnonymousReservationRepository(db),
		service.AnonymousReservationOptions{
			Now: func() time.Time { return fixedNow },
			Turnstile: service.NewMockTurnstileVerifier(
				false, func() time.Time { return fixedNow },
			),
			TTL: 24 * time.Hour,
			RateLimits: repository.AnonymousReservationLimits{
				BrowserPerHour: 5, BrowserPerDay: 20, IPPerHour: 20, IPPerDay: 100, IPProductPerDay: 3,
			},
		},
	)
	if err != nil {
		t.Fatalf("anonymous service: %v", err)
	}
	identity, err := service.NewReservationBrowserIdentity("controller-browser-secret-0123456789abcdef")
	if err != nil {
		t.Fatalf("browser identity: %v", err)
	}
	security, err := service.NewReservationRequestSecurity(
		"controller-stable-ip-secret-0123456789abc",
		service.ReservationRequestSecurityOptions{TrustedProxyCIDRs: []string{"172.16.0.0/12"}},
	)
	if err != nil {
		t.Fatalf("request security: %v", err)
	}
	ctl := NewReservationController(productService, ReservationControllerOptions{
		Mode: "anonymous", Anonymous: anonymous, BrowserIdentity: identity, Security: security,
	})
	router := gin.New()
	ctl.RegisterPublicRoutes(router.Group("/api/v1"))

	capabilities := reservationRequest(router, http.MethodGet, "/api/v1/storefront/reservation-capabilities", "", "")
	if capabilities.Code != http.StatusOK ||
		!strings.Contains(capabilities.Body.String(), `"reservation_mode":"anonymous"`) ||
		!strings.Contains(capabilities.Body.String(), `"otp_channels":[]`) {
		t.Fatalf("anonymous capabilities = %d/%s", capabilities.Code, capabilities.Body.String())
	}
	otpRoute := reservationRequest(router, http.MethodPost,
		fmt.Sprintf("/api/v1/storefront/reservation-products/%d/otp/request", product.ID), `{}`, "")
	if otpRoute.Code != http.StatusNotFound {
		t.Fatalf("anonymous mode registered OTP route: %d/%s", otpRoute.Code, otpRoute.Body.String())
	}

	path := fmt.Sprintf("/api/v1/storefront/reservation-products/%d/anonymous?locale=en", product.ID)
	first := httptest.NewRecorder()
	firstRequest := httptest.NewRequest(http.MethodPost, path, strings.NewReader(
		`{"quantity":2,"note":"Please hold","turnstile_token":"test-turnstile-controller-anonymous-1"}`,
	))
	firstRequest.Header.Set("Content-Type", "application/json")
	firstRequest.Header.Set("CF-Connecting-IP", "203.0.113.99")
	firstRequest.Header.Set("X-Forwarded-For", "203.0.113.98")
	router.ServeHTTP(first, firstRequest)
	if first.Code != http.StatusCreated {
		t.Fatalf("anonymous create = %d/%s", first.Code, first.Body.String())
	}
	cookies := first.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("Set-Cookie count = %d", len(cookies))
	}
	deviceCookie := cookies[0]
	if deviceCookie.Name != service.ReservationDeviceCookieName || !deviceCookie.HttpOnly ||
		!deviceCookie.Secure || deviceCookie.SameSite != http.SameSiteLaxMode ||
		deviceCookie.Path != "/" || deviceCookie.MaxAge != service.ReservationDeviceMaxAge {
		t.Fatalf("device Cookie attributes = %+v", deviceCookie)
	}
	var created struct {
		Data service.AnonymousReservationDTO `json:"data"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil || created.Data.ReservationCode == "" {
		t.Fatalf("decode anonymous create = %+v/%v", created, err)
	}
	var stored models.ProductReservation
	if err := db.Where("reservation_code = ?", created.Data.ReservationCode).Take(&stored).Error; err != nil {
		t.Fatalf("load anonymous reservation: %v", err)
	}
	if stored.BrowserHash == nil || *stored.BrowserHash == deviceCookie.Value ||
		stored.IPHash == nil || *stored.IPHash != security.IPHash(firstRequest.RemoteAddr, "") {
		t.Fatalf("stored anonymous identity = browser:%v ip:%v", stored.BrowserHash, stored.IPHash)
	}

	duplicate := httptest.NewRecorder()
	duplicateRequest := httptest.NewRequest(http.MethodPost, path, strings.NewReader(
		`{"quantity":2,"turnstile_token":"test-turnstile-controller-anonymous-2"}`,
	))
	duplicateRequest.Header.Set("Content-Type", "application/json")
	duplicateRequest.AddCookie(deviceCookie)
	router.ServeHTTP(duplicate, duplicateRequest)
	if duplicate.Code != http.StatusConflict ||
		!strings.Contains(duplicate.Body.String(), `"code":"RESERVATION_ALREADY_EXISTS"`) ||
		!strings.Contains(duplicate.Body.String(), created.Data.ReservationCode) ||
		len(duplicate.Result().Cookies()) != 0 {
		t.Fatalf("anonymous duplicate = %d/%s cookies:%v", duplicate.Code, duplicate.Body.String(), duplicate.Result().Cookies())
	}

	mine := httptest.NewRecorder()
	mineRequest := httptest.NewRequest(http.MethodGet, "/api/v1/storefront/my-reservations?locale=en", nil)
	mineRequest.AddCookie(deviceCookie)
	router.ServeHTTP(mine, mineRequest)
	if mine.Code != http.StatusOK || !strings.Contains(mine.Body.String(), created.Data.ReservationCode) {
		t.Fatalf("my reservations = %d/%s", mine.Code, mine.Body.String())
	}

	otherToken, err := identity.NewToken()
	if err != nil {
		t.Fatalf("other token: %v", err)
	}
	cancelPath := "/api/v1/storefront/my-reservations/" + created.Data.ReservationCode + "/cancel"
	otherCancel := httptest.NewRecorder()
	otherCancelRequest := httptest.NewRequest(http.MethodPost, cancelPath, nil)
	otherCancelRequest.AddCookie(&http.Cookie{Name: service.ReservationDeviceCookieName, Value: otherToken})
	router.ServeHTTP(otherCancel, otherCancelRequest)
	if otherCancel.Code != http.StatusNotFound {
		t.Fatalf("other browser cancel = %d/%s", otherCancel.Code, otherCancel.Body.String())
	}
	ownCancel := httptest.NewRecorder()
	ownCancelRequest := httptest.NewRequest(http.MethodPost, cancelPath, nil)
	ownCancelRequest.AddCookie(deviceCookie)
	router.ServeHTTP(ownCancel, ownCancelRequest)
	if ownCancel.Code != http.StatusOK || !strings.Contains(ownCancel.Body.String(), `"status":"cancelled"`) {
		t.Fatalf("own browser cancel = %d/%s", ownCancel.Code, ownCancel.Body.String())
	}
}

func reservationRequest(router http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	router.ServeHTTP(response, request)
	return response
}
