package service

import (
	"testing"
	"time"
)

func TestLoginAttemptLimiterEnforcesEmailAndIPLimits(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	limiter := NewLoginAttemptLimiter()
	limiter.now = func() time.Time { return now }

	for attempt := 0; attempt < 10; attempt++ {
		if allowed, _ := limiter.Allow("203.0.113.10", "buyer@example.com"); !allowed {
			t.Fatalf("email attempt %d was limited before 10 failures", attempt+1)
		}
		limiter.RecordFailure("203.0.113.10", "buyer@example.com")
	}
	if allowed, retryAfter := limiter.Allow("203.0.113.10", "buyer@example.com"); allowed || retryAfter < 1 {
		t.Fatalf("email limit = %v/%d, want blocked with retry", allowed, retryAfter)
	}

	ipLimiter := NewLoginAttemptLimiter()
	ipLimiter.now = func() time.Time { return now }
	for attempt := 0; attempt < 20; attempt++ {
		email := "buyer-" + string(rune('a'+attempt)) + "@example.com"
		if allowed, _ := ipLimiter.Allow("203.0.113.20", email); !allowed {
			t.Fatalf("IP attempt %d was limited before 20 failures", attempt+1)
		}
		ipLimiter.RecordFailure("203.0.113.20", email)
	}
	if allowed, retryAfter := ipLimiter.Allow("203.0.113.20", "new@example.com"); allowed || retryAfter < 1 {
		t.Fatalf("IP limit = %v/%d, want blocked with retry", allowed, retryAfter)
	}

	now = now.Add(time.Minute + time.Second)
	if allowed, _ := ipLimiter.Allow("203.0.113.20", "new@example.com"); !allowed {
		t.Fatal("IP limit did not expire after one minute")
	}
}

func TestLoginAttemptLimiterClearsEmailFailuresAfterSuccess(t *testing.T) {
	limiter := NewLoginAttemptLimiter()
	for attempt := 0; attempt < 10; attempt++ {
		limiter.RecordFailure("203.0.113.30", "buyer@example.com")
	}
	limiter.RecordSuccess("buyer@example.com")
	if allowed, _ := limiter.Allow("203.0.113.31", "buyer@example.com"); !allowed {
		t.Fatal("successful login did not clear the email failure bucket")
	}
}
