package service

import (
	"fmt"
	"math"
	"sync"
	"time"
)

type ReservationRateLimits struct {
	InterestVisitorPerMinute  int
	InterestIPPerMinute       int
	InterestGlobalPerMinute   int
	OTPRequestContactPer30Min int
	OTPRequestContactPerDay   int
	OTPRequestVisitorPerHour  int
	OTPRequestIPPerHour       int
	OTPRequestGlobalPerMinute int
	OTPVerifyVisitorPer10Min  int
	OTPVerifyIPPer10Min       int
	OTPVerifyGlobalPerMinute  int
	CancelIPPerMinute         int
	CancelGlobalPerMinute     int
}

type reservationLimitBucket struct {
	windowStartedAt time.Time
	lastSeenAt      time.Time
	count           int
}

type ReservationMemoryLimiter struct {
	mu          sync.Mutex
	buckets     map[string]reservationLimitBucket
	now         func() time.Time
	lastCleanup time.Time
	maxBuckets  int
}

const defaultReservationLimiterMaxBuckets = 100_000

func NewReservationMemoryLimiter(now ...func() time.Time) *ReservationMemoryLimiter {
	clock := time.Now
	if len(now) > 0 && now[0] != nil {
		clock = now[0]
	}
	current := clock().UTC()
	return &ReservationMemoryLimiter{
		buckets: make(map[string]reservationLimitBucket), now: clock, lastCleanup: current,
		maxBuckets: defaultReservationLimiterMaxBuckets,
	}
}

func (l *ReservationMemoryLimiter) Allow(scope, key string, limit int, window time.Duration) (bool, time.Duration) {
	if l == nil || key == "" || limit <= 0 || window <= 0 {
		return true, 0
	}
	now := l.now().UTC()
	bucketKey := fmt.Sprintf("%s\x00%s\x00%d", scope, key, window)
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.lastCleanup) >= time.Minute {
		l.cleanupLocked(now)
	}
	bucket, exists := l.buckets[bucketKey]
	if !exists && len(l.buckets) >= l.maxBuckets {
		return false, window
	}
	if bucket.windowStartedAt.IsZero() || now.Sub(bucket.windowStartedAt) >= window || now.Before(bucket.windowStartedAt) {
		bucket = reservationLimitBucket{windowStartedAt: now, lastSeenAt: now}
	}
	bucket.lastSeenAt = now
	if bucket.count >= limit {
		l.buckets[bucketKey] = bucket
		retry := window - now.Sub(bucket.windowStartedAt)
		if retry < time.Second {
			retry = time.Second
		}
		return false, retry
	}
	bucket.count++
	l.buckets[bucketKey] = bucket
	return true, 0
}

func (l *ReservationMemoryLimiter) Size() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

func (l *ReservationMemoryLimiter) cleanupLocked(now time.Time) {
	const maxBucketIdle = 48 * time.Hour
	for key, bucket := range l.buckets {
		if now.Before(bucket.lastSeenAt) || now.Sub(bucket.lastSeenAt) > maxBucketIdle {
			delete(l.buckets, key)
		}
	}
	l.lastCleanup = now
}

func reservationRetryAfterError(retry time.Duration) error {
	seconds := int(math.Ceil(retry.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	return &RateLimitError{RetryAfterSeconds: seconds}
}
