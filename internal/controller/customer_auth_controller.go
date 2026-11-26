package controller

import (
	"errors"
	"net/http"
	"strings"

	"saas/internal/middleware"
	"saas/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const passwordResetRequestMessage = "If the email exists, reset instructions will be sent."

type CustomerAuthController struct {
	service              *service.CustomerAuthService
	secret               string
	db                   *gorm.DB
	authEnabled          bool
	passwordResetEnabled bool
}

type customerRegisterPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type passwordResetRequestPayload struct {
	Email string `json:"email"`
}

type passwordResetPayload struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

func NewCustomerAuthController(
	service *service.CustomerAuthService,
	secret string,
	flags ...bool,
) *CustomerAuthController {
	controller := &CustomerAuthController{service: service, secret: secret}
	if len(flags) > 0 {
		controller.authEnabled = flags[0]
	}
	if len(flags) > 1 {
		controller.passwordResetEnabled = flags[1]
	}
	return controller
}

func (ctl *CustomerAuthController) WithDatabase(db *gorm.DB) *CustomerAuthController {
	ctl.db = db
	return ctl
}

func (ctl *CustomerAuthController) RegisterRoutes(group *gin.RouterGroup) {
	authGroup := group.Group("/auth")
	authGroup.Use(privateResponseHeaders())
	authGroup.POST("/register", ctl.RequireCustomerAuthEnabled(), ctl.Register)

	password := authGroup.Group("/password")
	password.Use(ctl.RequireCustomerAuthEnabled(), ctl.RequirePasswordResetEnabled())
	password.POST("/request", ctl.RequestPasswordReset)
	password.POST("/reset", ctl.ResetPassword)

	protected := authGroup.Group("")
	protected.Use(ctl.RequireCustomerAuthEnabled(), middleware.RequireCustomerAuth(ctl.secret, ctl.db))
	protected.GET("/me", ctl.Me)
	protected.POST("/logout", ctl.Logout)
}

func (ctl *CustomerAuthController) RequireCustomerAuthEnabled() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !ctl.authEnabled {
			writeCustomerAuthDisabled(c)
			c.Abort()
			return
		}
		c.Next()
	}
}

func (ctl *CustomerAuthController) RequirePasswordResetEnabled() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !ctl.passwordResetEnabled {
			c.JSON(http.StatusServiceUnavailable, response{
				Error: "Customer password reset is temporarily unavailable.",
				Code:  "CUSTOMER_PASSWORD_RESET_DISABLED",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

func writeCustomerAuthDisabled(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, response{
		Error: "Customer authentication is temporarily unavailable.",
		Code:  "CUSTOMER_AUTH_DISABLED",
	})
}

func (ctl *CustomerAuthController) Register(c *gin.Context) {
	var payload customerRegisterPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: "Invalid request", Code: "INVALID_REQUEST"})
		return
	}

	profile, err := ctl.service.Register(c.Request.Context(), service.RegisterCustomerInput{
		Email:    payload.Email,
		Password: payload.Password,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrEmailExists):
			c.JSON(http.StatusConflict, response{Error: err.Error(), Code: "EMAIL_ALREADY_REGISTERED"})
		case errors.Is(err, service.ErrInvalidEmail):
			c.JSON(http.StatusBadRequest, response{Error: err.Error(), Code: "INVALID_EMAIL"})
		case errors.Is(err, service.ErrWeakPassword):
			c.JSON(http.StatusBadRequest, response{Error: err.Error(), Code: "WEAK_PASSWORD"})
		default:
			writeError(c, err)
		}
		return
	}

	c.JSON(http.StatusCreated, response{
		Data:    gin.H{"user": profile},
		Message: "Registration successful. Please log in.",
	})
}

func (ctl *CustomerAuthController) RequestPasswordReset(c *gin.Context) {
	var payload passwordResetRequestPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusOK, response{Message: passwordResetRequestMessage})
		return
	}

	// The raw token is intentionally not returned or logged. A future email
	// delivery adapter will receive it from the service.
	if _, err := ctl.service.RequestPasswordReset(c.Request.Context(), payload.Email); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, response{Message: passwordResetRequestMessage})
}

func (ctl *CustomerAuthController) ResetPassword(c *gin.Context) {
	var payload passwordResetPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, response{Error: "Invalid request", Code: "INVALID_REQUEST"})
		return
	}

	err := ctl.service.ResetPassword(c.Request.Context(), strings.TrimSpace(payload.Token), payload.NewPassword)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWeakPassword):
			c.JSON(http.StatusBadRequest, response{Error: err.Error(), Code: "WEAK_PASSWORD"})
		case errors.Is(err, service.ErrInvalidResetToken):
			c.JSON(http.StatusBadRequest, response{Error: err.Error(), Code: "INVALID_RESET_TOKEN"})
		default:
			writeError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, response{Message: "Password reset successful. You can now log in."})
}

func (ctl *CustomerAuthController) Me(c *gin.Context) {
	customerID, ok := middleware.CustomerIDFromContext(c)
	if !ok || customerID == 0 {
		writeError(c, service.ErrUnauthorized)
		return
	}

	profile, err := ctl.service.Me(c.Request.Context(), customerID)
	if err != nil {
		writeError(c, err)
		return
	}

	c.JSON(http.StatusOK, response{Data: profile})
}

func (ctl *CustomerAuthController) Logout(c *gin.Context) {
	c.JSON(http.StatusOK, response{Message: "Logged out successfully."})
}
