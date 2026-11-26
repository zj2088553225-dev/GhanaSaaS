package service

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestReservationMemoryLimiterIsConcurrentAndCleansExpiredBuckets(t *testing.T) {
	current := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	limiter := NewReservationMemoryLimiter(func() time.Time { return current })
	const workers = 100
	var accepted atomic.Int64
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if allowed, _ := limiter.Allow("scope", "same-key", 10, time.Minute); allowed {
				accepted.Add(1)
			}
		}()
	}
	wait.Wait()
	if accepted.Load() != 10 {
		t.Fatalf("accepted = %d, want 10", accepted.Load())
	}
	if allowed, retry := limiter.Allow("scope", "same-key", 10, time.Minute); allowed || retry <= 0 {
		t.Fatalf("limited result = %v/%v", allowed, retry)
	}
	current = current.Add(49 * time.Hour)
	if allowed, _ := limiter.Allow("scope", "new-key", 1, time.Minute); !allowed {
		t.Fatal("new window was unexpectedly limited")
	}
	if limiter.Size() != 1 {
		t.Fatalf("limiter retained expired buckets, size=%d", limiter.Size())
	}
}

func TestReservationMemoryLimiterFailsClosedAtBoundAndRecoversAfterCleanup(t *testing.T) {
	current := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	limiter := NewReservationMemoryLimiter(func() time.Time { return current })
	limiter.maxBuckets = 2
	if allowed, _ := limiter.Allow("verify", "one", 1, time.Minute); !allowed {
		t.Fatal("first bucket rejected")
	}
	if allowed, _ := limiter.Allow("verify", "two", 1, time.Minute); !allowed {
		t.Fatal("second bucket rejected")
	}
	if allowed, retry := limiter.Allow("verify", "random-attacker-key", 1, time.Minute); allowed || retry <= 0 {
		t.Fatalf("bucket cap did not fail closed: %v/%v", allowed, retry)
	}
	if limiter.Size() != 2 {
		t.Fatalf("bucket map grew past cap: %d", limiter.Size())
	}
	current = current.Add(49 * time.Hour)
	if allowed, _ := limiter.Allow("verify", "after-cleanup", 1, time.Minute); !allowed {
		t.Fatal("limiter did not recover after stale-bucket cleanup")
	}
}
