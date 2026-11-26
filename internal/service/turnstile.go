package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	cloudflareTurnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	maxTurnstileTokenLength      = 2048
	maxTurnstileResponseBytes    = 64 << 10
	maxTurnstileReplayEntries    = 50_000
	turnstileReplayTTL           = 15 * time.Minute
)

var (
	ErrTurnstileRejected    = &contextualError{message: "human verification failed", target: ErrInvalidInput}
	ErrTurnstileUnavailable = &contextualError{
		message: "human verification service is temporarily unavailable",
		target:  ErrServiceUnavailable,
	}
)

type TurnstileVerification struct {
	VerifiedAt time.Time
}

type TurnstileVerifier interface {
	Verify(ctx context.Context, token, remoteIP, idempotencyKey string) (TurnstileVerification, error)
}

type CloudflareTurnstileVerifier struct {
	client           *http.Client
	endpoint         string
	secret           string
	expectedAction   string
	expectedHostname string
	now              func() time.Time
	replay           *turnstileReplayGuard
}

type CloudflareTurnstileOptions struct {
	Client           *http.Client
	Endpoint         string
	Secret           string
	ExpectedAction   string
	ExpectedHostname string
	Timeout          time.Duration
	Now              func() time.Time
}

type cloudflareTurnstileResponse struct {
	Success    bool     `json:"success"`
	Action     string   `json:"action"`
	Hostname   string   `json:"hostname"`
	ErrorCodes []string `json:"error-codes"`
}

func NewCloudflareTurnstileVerifier(options CloudflareTurnstileOptions) (*CloudflareTurnstileVerifier, error) {
	secret := strings.TrimSpace(options.Secret)
	action := strings.TrimSpace(options.ExpectedAction)
	if secret == "" || action == "" {
		return nil, errors.New("Turnstile secret and expected action are required")
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
	endpoint := strings.TrimSpace(options.Endpoint)
	if endpoint == "" {
		endpoint = cloudflareTurnstileVerifyURL
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &CloudflareTurnstileVerifier{
		client: client, endpoint: endpoint, secret: secret, expectedAction: action,
		expectedHostname: strings.ToLower(strings.TrimSpace(options.ExpectedHostname)), now: now,
		replay: newTurnstileReplayGuard(now, maxTurnstileReplayEntries),
	}, nil
}

func (v *CloudflareTurnstileVerifier) Verify(
	ctx context.Context,
	token, remoteIP, idempotencyKey string,
) (TurnstileVerification, error) {
	token = strings.TrimSpace(token)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if token == "" || len(token) > maxTurnstileTokenLength || idempotencyKey == "" || len(idempotencyKey) > 64 {
		return TurnstileVerification{}, ErrTurnstileRejected
	}
	return v.replay.verify(ctx, token, idempotencyKey, func() (TurnstileVerification, error) {
		return v.verifyRemote(ctx, token, remoteIP, idempotencyKey)
	})
}

func (v *CloudflareTurnstileVerifier) verifyRemote(
	ctx context.Context,
	token, remoteIP, idempotencyKey string,
) (TurnstileVerification, error) {
	form := url.Values{
		"secret":          {v.secret},
		"response":        {token},
		"idempotency_key": {strings.TrimSpace(idempotencyKey)},
	}
	if remoteIP = strings.TrimSpace(remoteIP); remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, v.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return TurnstileVerification{}, ErrTurnstileUnavailable
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := v.client.Do(request)
	if err != nil {
		var networkError net.Error
		if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &networkError) {
			return TurnstileVerification{}, ErrTurnstileUnavailable
		}
		return TurnstileVerification{}, ErrTurnstileUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxTurnstileResponseBytes))
		return TurnstileVerification{}, ErrTurnstileUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxTurnstileResponseBytes+1))
	if err != nil || len(body) > maxTurnstileResponseBytes {
		return TurnstileVerification{}, ErrTurnstileUnavailable
	}
	var result cloudflareTurnstileResponse
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&result); err != nil {
		return TurnstileVerification{}, ErrTurnstileUnavailable
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return TurnstileVerification{}, ErrTurnstileUnavailable
	}
	if !result.Success || result.Action != v.expectedAction {
		return TurnstileVerification{}, ErrTurnstileRejected
	}
	if v.expectedHostname != "" && strings.ToLower(strings.TrimSpace(result.Hostname)) != v.expectedHostname {
		return TurnstileVerification{}, ErrTurnstileRejected
	}
	return TurnstileVerification{VerifiedAt: v.now().UTC()}, nil
}

type turnstileReplayEntry struct {
	idempotencyKey string
	createdAt      time.Time
	done           chan struct{}
	result         TurnstileVerification
	err            error
}

type turnstileReplayGuard struct {
	mu         sync.Mutex
	entries    map[[sha256.Size]byte]*turnstileReplayEntry
	now        func() time.Time
	maxEntries int
}

func newTurnstileReplayGuard(now func() time.Time, maxEntries int) *turnstileReplayGuard {
	return &turnstileReplayGuard{
		entries: make(map[[sha256.Size]byte]*turnstileReplayEntry), now: now, maxEntries: maxEntries,
	}
}

func (g *turnstileReplayGuard) verify(
	ctx context.Context,
	token, idempotencyKey string,
	verify func() (TurnstileVerification, error),
) (TurnstileVerification, error) {
	digest := sha256.Sum256([]byte(token))
	now := g.now().UTC()
	g.mu.Lock()
	for key, entry := range g.entries {
		if now.Sub(entry.createdAt) > turnstileReplayTTL {
			select {
			case <-entry.done:
				delete(g.entries, key)
			default:
			}
		}
	}
	if existing, ok := g.entries[digest]; ok {
		if existing.idempotencyKey != idempotencyKey {
			g.mu.Unlock()
			return TurnstileVerification{}, ErrTurnstileRejected
		}
		done := existing.done
		g.mu.Unlock()
		select {
		case <-done:
			return existing.result, existing.err
		case <-ctx.Done():
			return TurnstileVerification{}, ErrTurnstileUnavailable
		}
	}
	if len(g.entries) >= g.maxEntries {
		g.mu.Unlock()
		return TurnstileVerification{}, ErrTurnstileUnavailable
	}
	entry := &turnstileReplayEntry{idempotencyKey: idempotencyKey, createdAt: now, done: make(chan struct{})}
	g.entries[digest] = entry
	g.mu.Unlock()

	entry.result, entry.err = verify()
	g.mu.Lock()
	close(entry.done)
	if errors.Is(entry.err, ErrServiceUnavailable) {
		delete(g.entries, digest)
	}
	g.mu.Unlock()
	return entry.result, entry.err
}

type MockTurnstileVerifier struct {
	mu         sync.Mutex
	consumed   map[[sha256.Size]byte]turnstileMockEntry
	allowEmpty bool
	now        func() time.Time
}

type turnstileMockEntry struct {
	idempotencyKey string
	createdAt      time.Time
}

func NewMockTurnstileVerifier(allowEmpty bool, now ...func() time.Time) *MockTurnstileVerifier {
	clock := time.Now
	if len(now) > 0 && now[0] != nil {
		clock = now[0]
	}
	return &MockTurnstileVerifier{consumed: make(map[[sha256.Size]byte]turnstileMockEntry), allowEmpty: allowEmpty, now: clock}
}

func (v *MockTurnstileVerifier) Verify(_ context.Context, token, _ string, idempotencyKey string) (TurnstileVerification, error) {
	token = strings.TrimSpace(token)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if token == "" && !v.allowEmpty {
		return TurnstileVerification{}, ErrTurnstileRejected
	}
	if token == "" {
		return TurnstileVerification{VerifiedAt: v.now().UTC()}, nil
	}
	if len(token) > maxTurnstileTokenLength || (token != "" && !strings.HasPrefix(token, "test-turnstile-")) {
		return TurnstileVerification{}, ErrTurnstileRejected
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	now := v.now().UTC()
	for digest, entry := range v.consumed {
		if now.Sub(entry.createdAt) > turnstileReplayTTL {
			delete(v.consumed, digest)
		}
	}
	digest := sha256.Sum256([]byte(token))
	if previous, exists := v.consumed[digest]; exists && previous.idempotencyKey != idempotencyKey {
		return TurnstileVerification{}, ErrTurnstileRejected
	}
	if _, exists := v.consumed[digest]; !exists && len(v.consumed) >= maxTurnstileReplayEntries {
		return TurnstileVerification{}, ErrTurnstileUnavailable
	}
	v.consumed[digest] = turnstileMockEntry{idempotencyKey: idempotencyKey, createdAt: now}
	return TurnstileVerification{VerifiedAt: now}, nil
}
