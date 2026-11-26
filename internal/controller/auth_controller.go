package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type AuthController struct {
	adminService        *service.AuthService
	customerService     *service.CustomerAuthService
	customerAuthEnabled bool
	loginLimiter        LoginAttemptLimiter
	clientIPResolver    *service.ClientIPResolver
}

// LoginAttemptLimiter is intentionally injectable so a shared Redis-backed
// implementation can replace the single-instance limiter without controller
// changes.
type LoginAttemptLimiter interface {
	Allow(ip string, identifier string) (bool, int)
	RecordFailure(ip string, identifier string)
	RecordSuccess(identifier string)
}

type loginPayload struct {
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func NewAuthController(
	adminService *service.AuthService,
	customerServices ...*service.CustomerAuthService,
) *AuthController {
	var customerService *service.CustomerAuthService
	if len(customerServices) > 0 {
		customerService = customerServices[0]
	}
	clientIPResolver, _ := service.NewClientIPResolver(nil)
	return &AuthController{
		adminService:     adminService,
		customerService:  customerService,
		loginLimiter:     service.NewLoginAttemptLimiter(),
		clientIPResolver: clientIPResolver,
	}
}

func (ctl *AuthController) WithClientIPResolver(resolver *service.ClientIPResolver) *AuthController {
	if resolver != nil {
		ctl.clientIPResolver = resolver
	}
	return ctl
}

func (ctl *AuthController) WithLoginAttemptLimiter(limiter LoginAttemptLimiter) *AuthController {
	if limiter != nil {
		ctl.loginLimiter = limiter
	}
	return ctl
}

func (ctl *AuthController) WithCustomerAuthEnabled(enabled bool) *AuthController {
	ctl.customerAuthEnabled = enabled
	return ctl
}

func (ctl *AuthController) RegisterRoutes(group *gin.RouterGroup) {
	authGroup := group.Group("/auth")
	authGroup.POST("/login", privateResponseHeaders(), ctl.Login)
}

func (ctl *AuthController) Login(c *gin.Context) {
	var payload loginPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: "Invalid request", Code: "INVALID_REQUEST"})
		return
	}

	email := strings.ToLower(strings.TrimSpace(payload.Email))
	phone := strings.TrimSpace(payload.Phone)
	clientIP := ctl.clientIP(c)
	if allowed, retryAfter := ctl.loginLimiter.Allow(clientIP, email); !allowed {
		c.Header("Retry-After", strconv.Itoa(retryAfter))
		c.JSON(http.StatusTooManyRequests, response{
			Error: "Too many login attempts. Please try again later.",
			Code:  "LOGIN_RATE_LIMITED",
		})
		return
	}

	switch {
	case email != "" && phone == "":
		if !ctl.customerAuthEnabled {
			writeCustomerAuthDisabled(c)
			return
		}
		ctl.loginCustomer(c, clientIP, email, payload.Password)
	case phone != "" && email == "":
		ctl.loginAdmin(c, clientIP, phone, payload.Password)
	default:
		ctl.loginLimiter.RecordFailure(clientIP, email)
		writeInvalidCredentials(c)
	}
}

func (ctl *AuthController) loginCustomer(c *gin.Context, clientIP string, email string, password string) {
	if ctl.customerService == nil {
		ctl.loginLimiter.RecordFailure(clientIP, email)
		writeInvalidCredentials(c)
		return
	}

	result, err := ctl.customerService.Login(c.Request.Context(), service.CustomerLoginInput{
		Identifier: service.AuthIdentifier{Type: service.AuthIdentifierEmail, Value: email},
		Password:   password,
	})
	if err != nil {
		if errors.Is(err, service.ErrUnauthorized) || errors.Is(err, service.ErrInvalidInput) {
			ctl.loginLimiter.RecordFailure(clientIP, email)
			writeInvalidCredentials(c)
			return
		}
		writeError(c, err)
		return
	}

	ctl.loginLimiter.RecordSuccess(email)
	c.JSON(http.StatusOK, response{Data: result})
}

func (ctl *AuthController) loginAdmin(c *gin.Context, clientIP string, phone string, password string) {
	if ctl.adminService == nil {
		ctl.loginLimiter.RecordFailure(clientIP, "")
		writeInvalidCredentials(c)
		return
	}

	result, err := ctl.adminService.Login(c.Request.Context(), service.LoginInput{
		Phone:    phone,
		Password: password,
	})
	if err != nil {
		if errors.Is(err, service.ErrUnauthorized) || errors.Is(err, service.ErrInvalidInput) {
			ctl.loginLimiter.RecordFailure(clientIP, "")
			writeInvalidCredentials(c)
			return
		}
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: result})
}

func writeInvalidCredentials(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, response{
		Error: "Invalid credentials",
		Code:  "INVALID_CREDENTIALS",
	})
}

func privateResponseHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store, private")
		c.Header("Pragma", "no-cache")
		c.Next()
	}
}

func (ctl *AuthController) clientIP(c *gin.Context) string {
	return ctl.clientIPResolver.Resolve(
		c.Request.RemoteAddr,
		c.GetHeader("CF-Connecting-IP"),
		c.GetHeader("X-Real-IP"),
		c.Request.Header.Values("X-Forwarded-For")...,
	)
}
