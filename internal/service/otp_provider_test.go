package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"saas/internal/models"
)

func TestTwilioVerifyProviderStartsAndChecksWithoutLeakingRawErrors(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		username, password, ok := request.BasicAuth()
		if !ok || username != "AC-test" || password != "auth-token" {
			t.Errorf("BasicAuth = %q/%q/%v", username, password, ok)
		}
		if err := request.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		switch {
		case strings.HasSuffix(request.URL.Path, "/Verifications"):
			if request.Form.Get("To") != "+233241234567" || request.Form.Get("Channel") != "whatsapp" || request.Form.Get("Locale") != "zh" {
				t.Errorf("verification form = %v", url.Values(request.Form))
			}
			_, _ = response.Write([]byte(`{"sid":"VE-safe-reference","status":"pending"}`))
		case strings.HasSuffix(request.URL.Path, "/VerificationCheck"):
			if request.Form.Get("To") != "+233241234567" || request.Form.Get("Code") != "123456" {
				t.Errorf("check form = %v", url.Values(request.Form))
			}
			_, _ = response.Write([]byte(`{"sid":"VE-safe-reference","status":"approved"}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	provider, err := NewTwilioVerifyOTPProvider(TwilioVerifyOptions{
		BaseURL: server.URL, AccountSID: "AC-test", AuthToken: "auth-token", ServiceSID: "VA-test", Client: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewTwilioVerifyOTPProvider: %v", err)
	}
	reference, err := provider.StartVerification(t.Context(), "+233241234567", models.ReservationContactWhatsApp, "zh")
	if err != nil || reference != "VE-safe-reference" {
		t.Fatalf("StartVerification = %q/%v", reference, err)
	}
	result, err := provider.CheckVerification(t.Context(), "+233241234567", "123456")
	if err != nil || !result.Approved || result.Status != "approved" || requests != 2 {
		t.Fatalf("CheckVerification = %+v/%v, requests=%d", result, err, requests)
	}
}

func TestTwilioAndMockProviderErrorSemantics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusBadGateway)
		_, _ = response.Write([]byte(`{"message":"provider-secret-detail"}`))
	}))
	defer server.Close()
	provider, _ := NewTwilioVerifyOTPProvider(TwilioVerifyOptions{
		BaseURL: server.URL, AccountSID: "AC-test", AuthToken: "auth-token", ServiceSID: "VA-test", Client: server.Client(),
	})
	if _, err := provider.StartVerification(t.Context(), "+233241234567", models.ReservationContactSMS, "en"); !errors.Is(err, ErrServiceUnavailable) || strings.Contains(err.Error(), "provider-secret-detail") {
		t.Fatalf("provider error = %v", err)
	}
	mock := NewMockOTPProvider()
	if result, err := mock.CheckVerification(t.Context(), "+233241234567", mockOTPCode); err != nil || !result.Approved {
		t.Fatalf("mock approved = %+v/%v", result, err)
	}
	if result, err := mock.CheckVerification(t.Context(), "+233241234567", "000000"); err != nil || result.Approved {
		t.Fatalf("mock invalid = %+v/%v", result, err)
	}
}

func TestTwilioVerifyProviderFailsClosedOnInputsStatusesAndBodies(t *testing.T) {
	requests := 0
	status := "pending"
	bodySuffix := ""
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		_, _ = response.Write([]byte(`{"sid":"VE-safe","status":"` + status + `"}` + bodySuffix))
	}))
	defer server.Close()
	provider, err := NewTwilioVerifyOTPProvider(TwilioVerifyOptions{
		BaseURL: server.URL, AccountSID: "AC-test", AuthToken: "auth-token", ServiceSID: "VA-test", Client: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.StartVerification(t.Context(), "+233241234567", models.ReservationContactType("email"), "en"); !errors.Is(err, ErrServiceUnavailable) || requests != 0 {
		t.Fatalf("invalid channel result/requests = %v/%d", err, requests)
	}
	if _, err := provider.CheckVerification(t.Context(), "+233241234567", "12ab"); !errors.Is(err, ErrServiceUnavailable) || requests != 0 {
		t.Fatalf("invalid code result/requests = %v/%d", err, requests)
	}
	status = "approved"
	if _, err := provider.StartVerification(t.Context(), "+233241234567", models.ReservationContactSMS, "en"); !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("non-pending start status error = %v", err)
	}
	for _, providerStatus := range []string{"pending", "canceled", "expired", "failed"} {
		status = providerStatus
		result, err := provider.CheckVerification(t.Context(), "+233241234567", "123456")
		if err != nil || result.Approved || result.Status != providerStatus {
			t.Fatalf("status %q = %+v/%v", providerStatus, result, err)
		}
	}
	status = "mystery"
	if _, err := provider.CheckVerification(t.Context(), "+233241234567", "123456"); !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("unknown status error = %v", err)
	}
	status = "approved"
	bodySuffix = ` {}`
	if _, err := provider.CheckVerification(t.Context(), "+233241234567", "123456"); !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("trailing provider body error = %v", err)
	}

	timeoutServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		time.Sleep(30 * time.Millisecond)
		_, _ = response.Write([]byte(`{"sid":"VE-safe","status":"pending"}`))
	}))
	defer timeoutServer.Close()
	timeoutProvider, _ := NewTwilioVerifyOTPProvider(TwilioVerifyOptions{
		BaseURL: timeoutServer.URL, AccountSID: "AC-test", AuthToken: "auth-token", ServiceSID: "VA-test",
		Client: timeoutServer.Client(), Timeout: 5 * time.Millisecond,
	})
	if _, err := timeoutProvider.StartVerification(t.Context(), "+233241234567", models.ReservationContactSMS, "en"); !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("provider timeout error = %v", err)
	}
}
