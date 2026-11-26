package controller

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"saas/internal/service"

	"github.com/gin-gonic/gin"
)

func TestWriteErrorHidesInternalErrorDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previousWriter) })

	router := gin.New()
	router.GET("/internal", func(c *gin.Context) {
		writeError(c, errors.New("SQLSTATE 23505: duplicate key on table secret_orders"))
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/internal", nil)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	body := response.Body.String()
	if !strings.Contains(body, "Internal server error") {
		t.Fatalf("body = %s, want generic internal error", body)
	}
	if strings.Contains(body, "SQLSTATE") || strings.Contains(body, "secret_orders") {
		t.Fatalf("body = %s, leaked internal error detail", body)
	}
	if strings.Contains(logs.String(), "SQLSTATE") || strings.Contains(logs.String(), "secret_orders") {
		t.Fatalf("log leaked internal error detail: %s", logs.String())
	}
}

func TestWriteErrorKeepsBusinessErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/invalid", func(c *gin.Context) {
		writeError(c, service.ErrInvalidInput)
	})
	router.GET("/forbidden", func(c *gin.Context) {
		writeError(c, service.ErrForbidden)
	})
	router.GET("/not-found", func(c *gin.Context) {
		writeError(c, service.ErrNotFound)
	})
	router.GET("/analytics-export-too-large", func(c *gin.Context) {
		writeError(c, service.ErrAnalyticsExportTooLarge)
	})

	tests := []struct {
		path   string
		status int
		body   string
	}{
		{path: "/invalid", status: http.StatusBadRequest, body: "invalid input"},
		{path: "/analytics-export-too-large", status: http.StatusBadRequest, body: "Export result exceeds 10,000 rows. Narrow the date range or filters."},
		{path: "/forbidden", status: http.StatusForbidden, body: "forbidden"},
		{path: "/not-found", status: http.StatusNotFound, body: "not found"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			router.ServeHTTP(response, request)

			if response.Code != tt.status {
				t.Fatalf("status = %d, want %d", response.Code, tt.status)
			}
			if !strings.Contains(response.Body.String(), tt.body) {
				t.Fatalf("body = %s, want %q", response.Body.String(), tt.body)
			}
		})
	}
}
