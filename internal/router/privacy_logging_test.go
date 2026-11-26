package router

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPrivacyLoggerAndRecoveryDoNotExposeRequestSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var accessLog bytes.Buffer
	var recoveryLog bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()
	log.SetOutput(&recoveryLog)
	log.SetFlags(0)
	log.SetPrefix("")
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
	})

	engine := gin.New()
	engine.Use(newPrivacyLogger(&accessLog), newPrivacyRecovery())
	engine.POST("/privacy", func(c *gin.Context) {
		_ = c.Error(errors.New("database rejected +233241234567 contact_hash=abc"))
		c.Status(http.StatusBadRequest)
	})
	engine.GET("/panic", func(_ *gin.Context) {
		panic("opaque-turnstile-token-and-otp-246810")
	})

	request := httptest.NewRequest(http.MethodPost, "/privacy?phone=%2B233241234567&token=opaque-turnstile", strings.NewReader(`{"code":"246810"}`))
	request.RemoteAddr = "203.0.113.8:43210"
	request.Header.Set("Authorization", "Bearer secret-jwt")
	request.Header.Set("X-Forwarded-For", "198.51.100.9")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("privacy response status = %d", response.Code)
	}

	panicRequest := httptest.NewRequest(http.MethodGet, "/panic?manage_token=secret", nil)
	panicRequest.Header.Set("Authorization", "Bearer panic-jwt")
	panicResponse := httptest.NewRecorder()
	engine.ServeHTTP(panicResponse, panicRequest)
	if panicResponse.Code != http.StatusInternalServerError {
		t.Fatalf("panic response status = %d", panicResponse.Code)
	}
	unmatchedRequest := httptest.NewRequest(http.MethodGet, "/+233241234567/opaque-turnstile", nil)
	engine.ServeHTTP(httptest.NewRecorder(), unmatchedRequest)

	combined := accessLog.String() + recoveryLog.String()
	for _, forbidden := range []string{
		"203.0.113.8", "198.51.100.9", "+233241234567", "246810", "opaque-turnstile",
		"secret-jwt", "panic-jwt", "contact_hash", "manage_token", "Authorization",
	} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("privacy logs exposed %q: %s", forbidden, combined)
		}
	}
	if !strings.Contains(accessLog.String(), "method=POST path=/privacy status=400") || !strings.Contains(recoveryLog.String(), "panic recovered method=GET path=/panic") {
		t.Fatalf("privacy logs lost safe operational metadata: access=%q recovery=%q", accessLog.String(), recoveryLog.String())
	}
}
