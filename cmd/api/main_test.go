package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"saas/internal/config"

	"github.com/gin-gonic/gin"
)

func TestDisabledReservationInitializationNeedsNoDatabaseOrProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reservationController, err := initializeReservationController(
		config.ReservationConfig{Mode: config.ReservationModeDisabled},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("initializeReservationController() returned error: %v", err)
	}

	router := gin.New()
	reservationController.RegisterPublicRoutes(router.Group("/api/v1"))
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/storefront/reservation-capabilities", nil)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("capabilities status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"reservation_enabled":false`) ||
		!strings.Contains(response.Body.String(), `"otp_channels":[]`) {
		t.Fatalf("disabled capabilities = %s", response.Body.String())
	}
}
