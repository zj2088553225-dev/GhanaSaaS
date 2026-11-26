package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"saas/internal/models"

	"github.com/google/uuid"
)

const (
	twilioVerifyBaseURL        = "https://verify.twilio.com/v2/Services"
	maxOTPProviderResponseBody = 64 << 10
	mockOTPCode                = "246810"
)

var ErrOTPProviderUnavailable = &contextualError{
	message: "verification service is temporarily unavailable",
	target:  ErrServiceUnavailable,
}

type OTPVerificationResult struct {
	Approved bool
	Status   string
}

type OTPProvider interface {
	Name() string
	StartVerification(ctx context.Context, e164 string, channel models.ReservationContactType, locale string) (string, error)
	CheckVerification(ctx context.Context, e164, code string) (OTPVerificationResult, error)
}

type MockOTPProvider struct{}

func NewMockOTPProvider() *MockOTPProvider { return &MockOTPProvider{} }
func (p *MockOTPProvider) Name() string    { return "mock" }

func (p *MockOTPProvider) StartVerification(_ context.Context, _ string, _ models.ReservationContactType, _ string) (string, error) {
	return "mock-" + uuid.NewString(), nil
}

func (p *MockOTPProvider) CheckVerification(_ context.Context, _ string, code string) (OTPVerificationResult, error) {
	if strings.TrimSpace(code) == mockOTPCode {
		return OTPVerificationResult{Approved: true, Status: "approved"}, nil
	}
	return OTPVerificationResult{Approved: false, Status: "pending"}, nil
}

type TwilioVerifyOTPProvider struct {
	client     *http.Client
	baseURL    string
	accountSID string
	authToken  string
	serviceSID string
}

type TwilioVerifyOptions struct {
	Client     *http.Client
	BaseURL    string
	AccountSID string
	AuthToken  string
	ServiceSID string
	Timeout    time.Duration
}

type twilioVerificationResponse struct {
	SID    string `json:"sid"`
	Status string `json:"status"`
}

func NewTwilioVerifyOTPProvider(options TwilioVerifyOptions) (*TwilioVerifyOTPProvider, error) {
	accountSID := strings.TrimSpace(options.AccountSID)
	authToken := strings.TrimSpace(options.AuthToken)
	serviceSID := strings.TrimSpace(options.ServiceSID)
	if accountSID == "" || authToken == "" || serviceSID == "" {
		return nil, errors.New("Twilio Verify credentials are required")
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	client := options.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	} else {
		copy := *client
		copy.Timeout = timeout
		client = &copy
	}
	baseURL := strings.TrimRight(strings.TrimSpace(options.BaseURL), "/")
	if baseURL == "" {
		baseURL = twilioVerifyBaseURL
	}
	return &TwilioVerifyOTPProvider{
		client: client, baseURL: baseURL, accountSID: accountSID, authToken: authToken, serviceSID: serviceSID,
	}, nil
}

func (p *TwilioVerifyOTPProvider) Name() string { return "twilio_verify" }

func (p *TwilioVerifyOTPProvider) StartVerification(
	ctx context.Context,
	e164 string,
	channel models.ReservationContactType,
	requestedLocale string,
) (string, error) {
	if !validNormalizedReservationE164(e164) || !channel.IsValid() {
		return "", ErrOTPProviderUnavailable
	}
	form := url.Values{"To": {e164}, "Channel": {string(channel)}}
	if requestedLocale = strings.TrimSpace(requestedLocale); requestedLocale != "" {
		form.Set("Locale", requestedLocale)
	}
	var response twilioVerificationResponse
	if err := p.postForm(ctx, "/Verifications", form, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.SID) == "" || !strings.EqualFold(strings.TrimSpace(response.Status), "pending") {
		return "", ErrOTPProviderUnavailable
	}
	return strings.TrimSpace(response.SID), nil
}

func (p *TwilioVerifyOTPProvider) CheckVerification(ctx context.Context, e164, code string) (OTPVerificationResult, error) {
	code = strings.TrimSpace(code)
	if !validNormalizedReservationE164(e164) || len(code) < 4 || len(code) > 10 || !reservationDigitsOnly(code) {
		return OTPVerificationResult{}, ErrOTPProviderUnavailable
	}
	form := url.Values{"To": {e164}, "Code": {code}}
	var response twilioVerificationResponse
	if err := p.postForm(ctx, "/VerificationCheck", form, &response); err != nil {
		return OTPVerificationResult{}, err
	}
	status := strings.ToLower(strings.TrimSpace(response.Status))
	switch status {
	case "approved":
		return OTPVerificationResult{Approved: true, Status: status}, nil
	case "pending", "canceled", "expired", "failed":
		return OTPVerificationResult{Approved: false, Status: status}, nil
	default:
		return OTPVerificationResult{}, ErrOTPProviderUnavailable
	}
}

func (p *TwilioVerifyOTPProvider) postForm(ctx context.Context, suffix string, form url.Values, target any) error {
	endpoint := p.baseURL + "/" + url.PathEscape(p.serviceSID) + suffix
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return ErrOTPProviderUnavailable
	}
	request.SetBasicAuth(p.accountSID, p.authToken)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := p.client.Do(request)
	if err != nil {
		return ErrOTPProviderUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxOTPProviderResponseBody))
		return ErrOTPProviderUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxOTPProviderResponseBody+1))
	if err != nil || len(body) > maxOTPProviderResponseBody {
		return ErrOTPProviderUnavailable
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(target); err != nil {
		return ErrOTPProviderUnavailable
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ErrOTPProviderUnavailable
	}
	return nil
}
