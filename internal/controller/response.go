package controller

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

type response struct {
	Data    any    `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	Code    string `json:"code,omitempty"`
}

func parseUintParam(c *gin.Context, name string) (uint, bool) {
	value, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || value == 0 {
		c.JSON(http.StatusBadRequest, response{Error: "invalid " + name})
		return 0, false
	}
	return uint(value), true
}

func tenantIDFromRequest(c *gin.Context) (*uint, bool) {
	value := c.GetHeader("X-Tenant-ID")
	if value == "" {
		value = c.Query("tenant_id")
	}
	if value == "" {
		return nil, true
	}

	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		c.JSON(http.StatusBadRequest, response{Error: "invalid tenant_id"})
		return nil, false
	}

	tenantID := uint(parsed)
	return &tenantID, true
}

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, response{Error: err.Error()})
	case errors.Is(err, service.ErrNotFound):
		c.JSON(http.StatusNotFound, response{Error: err.Error()})
	case errors.Is(err, service.ErrUnauthorized):
		c.JSON(http.StatusUnauthorized, response{Error: err.Error()})
	case errors.Is(err, service.ErrForbidden):
		c.JSON(http.StatusForbidden, response{Error: err.Error()})
	case errors.Is(err, service.ErrConflict):
		c.JSON(http.StatusConflict, response{Error: err.Error()})
	case errors.Is(err, service.ErrRateLimited):
		c.JSON(http.StatusTooManyRequests, response{Error: err.Error()})
	case errors.Is(err, service.ErrServiceUnavailable):
		c.JSON(http.StatusServiceUnavailable, response{Error: err.Error()})
	default:
		// Persistence/provider errors can contain driver details or user input.
		// Keep the operational signal without copying the raw error to logs.
		log.Printf("internal server error")
		c.JSON(http.StatusInternalServerError, response{Error: "Internal server error"})
	}
}

func writeStorefrontUnavailable(c *gin.Context) {
	const message = "Storefront checkout is not available yet"
	c.JSON(http.StatusForbidden, response{Message: message, Error: message})
}
