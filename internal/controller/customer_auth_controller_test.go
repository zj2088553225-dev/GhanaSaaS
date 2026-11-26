package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"saas/internal/models"
	"saas/internal/pkg/auth"
	"saas/internal/repository"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newCustomerAuthControllerTestRouterWithFlags(
	t *testing.T,
	authEnabled bool,
	passwordResetEnabled bool,
) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dsn := fmt.Sprintf("file:customer-auth-controller-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Tenant{},
		&models.User{},
		&models.Customer{},
		&models.PasswordResetToken{},
	); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	customerRepo := repository.NewCustomerRepository(db)
	customerService := service.NewCustomerAuthService(customerRepo, "secret", time.Hour)
	adminService := service.NewAuthService(repository.NewUserRepository(db), "secret", time.Hour)
	router := gin.New()
	v1 := router.Group("/api/v1")
	NewAuthController(adminService, customerService).
		WithCustomerAuthEnabled(authEnabled).
		RegisterRoutes(v1)
	NewCustomerAuthController(customerService, "secret", authEnabled, passwordResetEnabled).
		WithDatabase(db).
		RegisterRoutes(v1)
	return router, db
}

func newCustomerAuthControllerTestRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	return newCustomerAuthControllerTestRouterWithFlags(t, true, true)
}

func performJSONRequest(router http.Handler, method string, path string, body string) *httptest.ResponseRecorder {
	responseRecorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(responseRecorder, request)
	return responseRecorder
}

func assertPrivateResponseHeaders(t *testing.T, responseRecorder *httptest.ResponseRecorder) {
	t.Helper()
	if responseRecorder.Header().Get("Cache-Control") != "no-store, private" ||
		responseRecorder.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("private response cache headers = %q/%q",
			responseRecorder.Header().Get("Cache-Control"),
			responseRecorder.Header().Get("Pragma"))
	}
}

func TestCustomerAuthRegisterEndpointNormalizesEmailAndDoesNotLogin(t *testing.T) {
	router, db := newCustomerAuthControllerTestRouter(t)
	responseRecorder := performJSONRequest(
		router,
		http.MethodPost,
		"/api/v1/auth/register",
		`{"email":" Buyer@Example.COM ","password":"strong-password"}`,
	)

	if responseRecorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", responseRecorder.Code, http.StatusCreated, responseRecorder.Body.String())
	}
	assertPrivateResponseHeaders(t, responseRecorder)
	if strings.Contains(responseRecorder.Body.String(), `"token"`) ||
		strings.Contains(responseRecorder.Body.String(), "password_hash") {
		t.Fatalf("registration response exposed a secret: %s", responseRecorder.Body.String())
	}

	var customer models.Customer
	if err := db.First(&customer).Error; err != nil {
		t.Fatalf("load registered customer: %v", err)
	}
	if customer.Email != "buyer@example.com" {
		t.Fatalf("stored email = %q, want normalized lowercase", customer.Email)
	}
}

func TestCustomerAuthLoginReturnsUnifiedInvalidCredentials(t *testing.T) {
	router, db := newCustomerAuthControllerTestRouter(t)
	performJSONRequest(router, http.MethodPost, "/api/v1/auth/register",
		`{"email":"buyer@example.com","password":"strong-password"}`)

	tests := []string{
		`{"email":"missing@example.com","password":"wrong-password"}`,
		`{"email":"buyer@example.com","password":"wrong-password"}`,
	}
	for _, body := range tests {
		responseRecorder := performJSONRequest(router, http.MethodPost, "/api/v1/auth/login", body)
		if responseRecorder.Code != http.StatusUnauthorized ||
			!strings.Contains(responseRecorder.Body.String(), `"code":"INVALID_CREDENTIALS"`) {
			t.Fatalf("status/body = %d/%s, want unified invalid credentials", responseRecorder.Code, responseRecorder.Body.String())
		}
	}

	if err := db.Model(&models.Customer{}).
		Where("email = ?", "buyer@example.com").
		Update("status", models.CustomerStatusDisabled).Error; err != nil {
		t.Fatalf("disable customer: %v", err)
	}
	responseRecorder := performJSONRequest(router, http.MethodPost, "/api/v1/auth/login",
		`{"email":"buyer@example.com","password":"strong-password"}`)
	if responseRecorder.Code != http.StatusUnauthorized ||
		!strings.Contains(responseRecorder.Body.String(), `"code":"INVALID_CREDENTIALS"`) {
		t.Fatalf("disabled status/body = %d/%s, want unified invalid credentials", responseRecorder.Code, responseRecorder.Body.String())
	}
}

func TestCustomerAuthLoginAndMeEndpoints(t *testing.T) {
	router, _ := newCustomerAuthControllerTestRouter(t)
	performJSONRequest(router, http.MethodPost, "/api/v1/auth/register",
		`{"email":"buyer@example.com","password":"strong-password"}`)
	loginResponse := performJSONRequest(router, http.MethodPost, "/api/v1/auth/login",
		`{"email":"buyer@example.com","password":"strong-password"}`)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", loginResponse.Code, loginResponse.Body.String())
	}
	assertPrivateResponseHeaders(t, loginResponse)

	var loginPayload struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(loginResponse.Body.Bytes(), &loginPayload); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if loginPayload.Data.Token == "" {
		t.Fatal("login response returned an empty JWT")
	}

	meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meRequest.Header.Set("Authorization", "Bearer "+loginPayload.Data.Token)
	meResponse := httptest.NewRecorder()
	router.ServeHTTP(meResponse, meRequest)
	if meResponse.Code != http.StatusOK ||
		!strings.Contains(meResponse.Body.String(), `"email":"buyer@example.com"`) {
		t.Fatalf("me status/body = %d/%s", meResponse.Code, meResponse.Body.String())
	}
	assertPrivateResponseHeaders(t, meResponse)
	for _, forbidden := range []string{"password_hash", "token_hash", "last_login_at", `"status"`} {
		if strings.Contains(meResponse.Body.String(), forbidden) {
			t.Fatalf("me response contains %q: %s", forbidden, meResponse.Body.String())
		}
	}

	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated me status = %d, want 401", unauthenticated.Code)
	}
}

func TestPasswordResetRequestResponseDoesNotRevealAccountExistence(t *testing.T) {
	router, db := newCustomerAuthControllerTestRouter(t)
	performJSONRequest(router, http.MethodPost, "/api/v1/auth/register",
		`{"email":"buyer@example.com","password":"strong-password"}`)

	existing := performJSONRequest(router, http.MethodPost, "/api/v1/auth/password/request",
		`{"email":"buyer@example.com"}`)
	missing := performJSONRequest(router, http.MethodPost, "/api/v1/auth/password/request",
		`{"email":"missing@example.com"}`)
	if existing.Code != http.StatusOK || missing.Code != http.StatusOK ||
		existing.Body.String() != missing.Body.String() ||
		!strings.Contains(existing.Body.String(), passwordResetRequestMessage) {
		t.Fatalf("reset request responses differ: existing=%d/%s missing=%d/%s",
			existing.Code, existing.Body.String(), missing.Code, missing.Body.String())
	}
	assertPrivateResponseHeaders(t, existing)
	assertPrivateResponseHeaders(t, missing)
	if strings.Contains(existing.Body.String(), "token") {
		t.Fatalf("reset request response exposed token data: %s", existing.Body.String())
	}

	var resetToken models.PasswordResetToken
	if err := db.First(&resetToken).Error; err != nil {
		t.Fatalf("load reset token: %v", err)
	}
	if len(resetToken.TokenHash) != 64 {
		t.Fatalf("token hash length = %d, want 64", len(resetToken.TokenHash))
	}
}

func TestPasswordResetDisabledDoesNotCreateToken(t *testing.T) {
	router, db := newCustomerAuthControllerTestRouterWithFlags(t, true, false)
	performJSONRequest(router, http.MethodPost, "/api/v1/auth/register",
		`{"email":"buyer@example.com","password":"strong-password"}`)

	responseRecorder := performJSONRequest(router, http.MethodPost, "/api/v1/auth/password/request",
		`{"email":"buyer@example.com"}`)
	if responseRecorder.Code != http.StatusServiceUnavailable ||
		!strings.Contains(responseRecorder.Body.String(), `"code":"CUSTOMER_PASSWORD_RESET_DISABLED"`) {
		t.Fatalf("reset request status/body = %d/%s", responseRecorder.Code, responseRecorder.Body.String())
	}

	var count int64
	if err := db.Model(&models.PasswordResetToken{}).Count(&count).Error; err != nil {
		t.Fatalf("count reset tokens: %v", err)
	}
	if count != 0 {
		t.Fatalf("password reset token count = %d, want 0", count)
	}
}

func TestCustomerLoginFailureRateLimit(t *testing.T) {
	router, _ := newCustomerAuthControllerTestRouter(t)
	for attempt := 0; attempt < 10; attempt++ {
		responseRecorder := performJSONRequest(router, http.MethodPost, "/api/v1/auth/login",
			`{"email":"limited@example.com","password":"wrong-password"}`)
		if responseRecorder.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", attempt+1, responseRecorder.Code)
		}
	}

	limited := performJSONRequest(router, http.MethodPost, "/api/v1/auth/login",
		`{"email":"limited@example.com","password":"wrong-password"}`)
	if limited.Code != http.StatusTooManyRequests ||
		!strings.Contains(limited.Body.String(), `"code":"LOGIN_RATE_LIMITED"`) ||
		limited.Header().Get("Retry-After") == "" {
		t.Fatalf("limited status/body/header = %d/%s/%q", limited.Code, limited.Body.String(), limited.Header().Get("Retry-After"))
	}
}

func TestExistingAdminPhoneLoginStillWorks(t *testing.T) {
	router, db := newCustomerAuthControllerTestRouter(t)
	passwordHash, err := auth.HashPassword("admin-password")
	if err != nil {
		t.Fatalf("hash admin password: %v", err)
	}
	admin := models.User{
		Role:         models.UserRoleSuperAdmin,
		Phone:        "233500000000",
		PasswordHash: passwordHash,
		Status:       models.UserStatusActive,
	}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}

	responseRecorder := performJSONRequest(router, http.MethodPost, "/api/v1/auth/login",
		`{"phone":"233500000000","password":"admin-password"}`)
	if responseRecorder.Code != http.StatusOK ||
		!strings.Contains(responseRecorder.Body.String(), `"role":"super_admin"`) {
		t.Fatalf("admin login status/body = %d/%s", responseRecorder.Code, responseRecorder.Body.String())
	}
	assertPrivateResponseHeaders(t, responseRecorder)
}

func TestStorefrontAuthDisabledRejectsCustomerEndpointsButAllowsAdminLogin(t *testing.T) {
	router, db := newCustomerAuthControllerTestRouterWithFlags(t, false, false)
	passwordHash, err := auth.HashPassword("admin-password")
	if err != nil {
		t.Fatalf("hash admin password: %v", err)
	}
	if err := db.Create(&models.User{
		Role:         models.UserRoleSuperAdmin,
		Phone:        "233500000001",
		PasswordHash: passwordHash,
		Status:       models.UserStatusActive,
	}).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}

	customerCases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/v1/auth/register", `{"email":"buyer@example.com","password":"strong-password"}`},
		{http.MethodPost, "/api/v1/auth/login", `{"email":"buyer@example.com","password":"strong-password"}`},
		{http.MethodGet, "/api/v1/auth/me", ""},
		{http.MethodPost, "/api/v1/auth/logout", `{}`},
		{http.MethodPost, "/api/v1/auth/password/request", `{"email":"buyer@example.com"}`},
		{http.MethodPost, "/api/v1/auth/password/reset", `{"token":"token","new_password":"strong-password"}`},
	}
	for _, testCase := range customerCases {
		responseRecorder := performJSONRequest(router, testCase.method, testCase.path, testCase.body)
		if responseRecorder.Code != http.StatusServiceUnavailable ||
			!strings.Contains(responseRecorder.Body.String(), `"code":"CUSTOMER_AUTH_DISABLED"`) {
			t.Fatalf("%s %s status/body = %d/%s, want disabled contract",
				testCase.method, testCase.path, responseRecorder.Code, responseRecorder.Body.String())
		}
	}

	adminLogin := performJSONRequest(router, http.MethodPost, "/api/v1/auth/login",
		`{"phone":"233500000001","password":"admin-password"}`)
	if adminLogin.Code != http.StatusOK ||
		!strings.Contains(adminLogin.Body.String(), `"role":"super_admin"`) {
		t.Fatalf("admin login status/body = %d/%s", adminLogin.Code, adminLogin.Body.String())
	}
}

func TestAuthClientIPUsesExplicitTrustedProxyResolver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resolver, err := service.NewClientIPResolver([]string{"172.16.0.0/12"})
	if err != nil {
		t.Fatal(err)
	}
	controller := NewAuthController(nil).WithClientIPResolver(resolver)

	privateRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	privateRequest.RemoteAddr = "172.19.0.10:43210"
	privateRequest.Header.Set("X-Forwarded-For", "203.0.113.50, 172.19.0.10")
	privateContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	privateContext.Request = privateRequest
	if got := controller.clientIP(privateContext); got != "203.0.113.50" {
		t.Fatalf("private proxy client IP = %q, want forwarded public IP", got)
	}

	publicRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	publicRequest.RemoteAddr = "198.51.100.20:43210"
	publicRequest.Header.Set("X-Forwarded-For", "203.0.113.60")
	publicContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	publicContext.Request = publicRequest
	if got := controller.clientIP(publicContext); got != "198.51.100.20" {
		t.Fatalf("public peer client IP = %q, want direct peer", got)
	}
}

func TestAuthRateLimitSeparatesClientsBehindTrustedProxy(t *testing.T) {
	resolver, err := service.NewClientIPResolver([]string{"172.16.0.0/12"})
	if err != nil {
		t.Fatal(err)
	}

	// Build an isolated router so the configured resolver is used by login.
	dsn := fmt.Sprintf("file:customer-auth-proxy-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Customer{}); err != nil {
		t.Fatal(err)
	}
	customerService := service.NewCustomerAuthService(repository.NewCustomerRepository(db), "secret", time.Hour)
	authController := NewAuthController(nil, customerService).
		WithCustomerAuthEnabled(true).
		WithClientIPResolver(resolver)
	router := gin.New()
	authController.RegisterRoutes(router.Group("/api/v1"))

	for client := 1; client <= 2; client++ {
		for attempt := 0; attempt < 10; attempt++ {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
				strings.NewReader(fmt.Sprintf(`{"email":"client-%d-%d@example.com","password":"wrong-password"}`, client, attempt)))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", client))
			request.RemoteAddr = "172.19.0.10:43210"
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("client %d attempt %d status = %d", client, attempt, recorder.Code)
			}
		}
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"email":"third@example.com","password":"wrong-password"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Forwarded-For", "203.0.113.3")
	request.RemoteAddr = "172.19.0.10:43210"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("third proxied client status = %d, want separate IP bucket", recorder.Code)
	}
}
