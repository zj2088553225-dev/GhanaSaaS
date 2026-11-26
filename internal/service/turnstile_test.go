package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCloudflareTurnstileVerifierValidatesActionHostnameAndForm(t *testing.T) {
	var received url.Values
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		received, _ = url.ParseQuery(string(body))
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"success":true,"action":"reservation_otp","hostname":"shop.example.com"}`))
	}))
	defer server.Close()
	fixedNow := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	verifier, err := NewCloudflareTurnstileVerifier(CloudflareTurnstileOptions{
		Endpoint: server.URL, Secret: "turnstile-secret", ExpectedAction: "reservation_otp",
		ExpectedHostname: "shop.example.com", Timeout: time.Second, Now: func() time.Time { return fixedNow },
	})
	if err != nil {
		t.Fatalf("NewCloudflareTurnstileVerifier: %v", err)
	}
	result, err := verifier.Verify(t.Context(), "opaque-token", "203.0.113.5", "11111111-1111-4111-8111-111111111111")
	if err != nil || !result.VerifiedAt.Equal(fixedNow) {
		t.Fatalf("Verify = %+v/%v", result, err)
	}
	for key, expected := range map[string]string{
		"secret": "turnstile-secret", "response": "opaque-token", "remoteip": "203.0.113.5",
		"idempotency_key": "11111111-1111-4111-8111-111111111111",
	} {
		if received.Get(key) != expected {
			t.Fatalf("form[%s] = %q, want %q", key, received.Get(key), expected)
		}
	}
}

func TestCloudflareTurnstileVerifierRejectsInvalidActionHostnameAndTimeout(t *testing.T) {
	tests := []struct {
		name     string
		response string
	}{
		{name: "unsuccessful", response: `{"success":false,"error-codes":["invalid-input-response"]}`},
		{name: "action mismatch", response: `{"success":true,"action":"login","hostname":"shop.example.com"}`},
		{name: "hostname mismatch", response: `{"success":true,"action":"reservation_otp","hostname":"evil.example"}`},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				_, _ = response.Write([]byte(testCase.response))
			}))
			defer server.Close()
			verifier, _ := NewCloudflareTurnstileVerifier(CloudflareTurnstileOptions{
				Endpoint: server.URL, Secret: "secret", ExpectedAction: "reservation_otp", ExpectedHostname: "shop.example.com",
			})
			if _, err := verifier.Verify(t.Context(), "token", "", "key"); !errors.Is(err, ErrTurnstileRejected) {
				t.Fatalf("Verify error = %v", err)
			}
		})
	}

	timeoutServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = response.Write([]byte(`{"success":true,"action":"reservation_otp"}`))
	}))
	defer timeoutServer.Close()
	verifier, _ := NewCloudflareTurnstileVerifier(CloudflareTurnstileOptions{
		Endpoint: timeoutServer.URL, Secret: "secret", ExpectedAction: "reservation_otp", Timeout: 5 * time.Millisecond,
	})
	if _, err := verifier.Verify(context.Background(), "token", "", "key"); !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("timeout error = %v", err)
	}
	if _, err := verifier.Verify(t.Context(), strings.Repeat("x", maxTurnstileTokenLength+1), "", "key"); !errors.Is(err, ErrTurnstileRejected) {
		t.Fatalf("oversized token error = %v", err)
	}
	if _, err := verifier.Verify(t.Context(), "   ", "", "key"); !errors.Is(err, ErrTurnstileRejected) {
		t.Fatalf("blank token error = %v", err)
	}
	if _, err := verifier.Verify(t.Context(), "token", "", ""); !errors.Is(err, ErrTurnstileRejected) {
		t.Fatalf("blank idempotency error = %v", err)
	}
}

func TestCloudflareTurnstileReplayGuardIsConcurrentAndRetrySafe(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var starts sync.Once
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		starts.Do(func() { close(started) })
		<-release
		_, _ = response.Write([]byte(`{"success":true,"action":"reservation_otp","hostname":"shop.example.com"}`))
	}))
	defer server.Close()
	verifier, err := NewCloudflareTurnstileVerifier(CloudflareTurnstileOptions{
		Endpoint: server.URL, Secret: "secret", ExpectedAction: "reservation_otp", ExpectedHostname: "shop.example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct{ err error }
	results := make(chan outcome, 2)
	for index := 0; index < 2; index++ {
		go func() {
			_, verifyErr := verifier.Verify(t.Context(), "same-token", "", "same-request")
			results <- outcome{err: verifyErr}
		}()
	}
	<-started
	if _, err := verifier.Verify(t.Context(), "same-token", "", "different-request"); !errors.Is(err, ErrTurnstileRejected) {
		t.Fatalf("cross-request replay error = %v", err)
	}
	close(release)
	for index := 0; index < 2; index++ {
		if result := <-results; result.err != nil {
			t.Fatalf("stable concurrent retry = %v", result.err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("Siteverify calls = %d, want 1", calls.Load())
	}

	var retryCalls atomic.Int64
	retryServer := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		if retryCalls.Add(1) == 1 {
			response.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = response.Write([]byte(`{"success":true,"action":"reservation_otp"}`))
	}))
	defer retryServer.Close()
	retryVerifier, _ := NewCloudflareTurnstileVerifier(CloudflareTurnstileOptions{
		Endpoint: retryServer.URL, Secret: "secret", ExpectedAction: "reservation_otp",
	})
	if _, err := retryVerifier.Verify(t.Context(), "retry-token", "", "retry-key"); !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("first unavailable result = %v", err)
	}
	if _, err := retryVerifier.Verify(t.Context(), "retry-token", "", "retry-key"); err != nil {
		t.Fatalf("retry after unavailable = %v", err)
	}
	if retryCalls.Load() != 2 {
		t.Fatalf("retry calls = %d, want 2", retryCalls.Load())
	}
}

func TestCloudflareTurnstileMalformedResponsesFailClosed(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "non-2xx", status: http.StatusBadGateway, body: `{}`},
		{name: "malformed", status: http.StatusOK, body: `{`},
		{name: "trailing JSON", status: http.StatusOK, body: `{"success":true,"action":"reservation_otp"} {}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(testCase.status)
				_, _ = response.Write([]byte(testCase.body))
			}))
			defer server.Close()
			verifier, _ := NewCloudflareTurnstileVerifier(CloudflareTurnstileOptions{Endpoint: server.URL, Secret: "secret", ExpectedAction: "reservation_otp"})
			if _, err := verifier.Verify(t.Context(), "token-"+testCase.name, "", "request"); !errors.Is(err, ErrServiceUnavailable) {
				t.Fatalf("fail-closed error = %v", err)
			}
		})
	}
}

func TestMockTurnstileRequiresFreshTokenButAllowsStableRetry(t *testing.T) {
	verifier := NewMockTurnstileVerifier(false)
	if _, err := verifier.Verify(t.Context(), "test-turnstile-one", "", "request-one"); err != nil {
		t.Fatalf("first Verify: %v", err)
	}
	if _, err := verifier.Verify(t.Context(), "test-turnstile-one", "", "request-one"); err != nil {
		t.Fatalf("stable retry Verify: %v", err)
	}
	if _, err := verifier.Verify(t.Context(), "test-turnstile-one", "", "request-two"); !errors.Is(err, ErrTurnstileRejected) {
		t.Fatalf("reused token error = %v", err)
	}
	if _, err := verifier.Verify(t.Context(), "not-a-test-token", "", "request-three"); !errors.Is(err, ErrTurnstileRejected) {
		t.Fatalf("invalid mock token error = %v", err)
	}
}
