package service

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

const (
	loginAttemptWindow          = time.Minute
	loginAttemptIPLimit         = 20
	loginAttemptIdentifierLimit = 10
)

type LoginAttemptLimiter struct {
	mu              sync.Mutex
	attempts        map[string][]time.Time
	now             func() time.Time
	window          time.Duration
	ipLimit         int
	identifierLimit int
	lastCleanup     time.Time
}

func NewLoginAttemptLimiter() *LoginAttemptLimiter {
	return &LoginAttemptLimiter{
		attempts:        make(map[string][]time.Time),
		now:             time.Now,
		window:          loginAttemptWindow,
		ipLimit:         loginAttemptIPLimit,
		identifierLimit: loginAttemptIdentifierLimit,
	}
}

func (l *LoginAttemptLimiter) Allow(ip string, identifier string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.cleanupExpired(now)
	if allowed, retryAfter := l.allowKey("ip", ip, l.ipLimit, now); !allowed {
		return false, retryAfter
	}
	if identifier != "" {
		if allowed, retryAfter := l.allowKey("identifier", identifier, l.identifierLimit, now); !allowed {
			return false, retryAfter
		}
	}
	return true, 0
}

func (l *LoginAttemptLimiter) RecordFailure(ip string, identifier string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.recordKey("ip", ip, now)
	if identifier != "" {
		l.recordKey("identifier", identifier, now)
	}
}

func (l *LoginAttemptLimiter) RecordSuccess(identifier string) {
	if identifier == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, limiterKey("identifier", identifier))
}

func (l *LoginAttemptLimiter) allowKey(kind string, value string, limit int, now time.Time) (bool, int) {
	key := limiterKey(kind, value)
	attempts := pruneLoginAttempts(l.attempts[key], now.Add(-l.window))
	if len(attempts) == 0 {
		delete(l.attempts, key)
		return true, 0
	}
	l.attempts[key] = attempts
	if len(attempts) < limit {
		return true, 0
	}
	retryAfter := int(attempts[0].Add(l.window).Sub(now).Seconds())
	if retryAfter < 1 {
		retryAfter = 1
	}
	return false, retryAfter
}

func (l *LoginAttemptLimiter) recordKey(kind string, value string, now time.Time) {
	key := limiterKey(kind, value)
	l.attempts[key] = append(pruneLoginAttempts(l.attempts[key], now.Add(-l.window)), now)
}

func (l *LoginAttemptLimiter) cleanupExpired(now time.Time) {
	if !l.lastCleanup.IsZero() && now.Sub(l.lastCleanup) < l.window {
		return
	}
	cutoff := now.Add(-l.window)
	for key, attempts := range l.attempts {
		active := pruneLoginAttempts(attempts, cutoff)
		if len(active) == 0 {
			delete(l.attempts, key)
			continue
		}
		l.attempts[key] = active
	}
	l.lastCleanup = now
}

func pruneLoginAttempts(attempts []time.Time, cutoff time.Time) []time.Time {
	firstActive := 0
	for firstActive < len(attempts) && !attempts[firstActive].After(cutoff) {
		firstActive++
	}
	return attempts[firstActive:]
}

func limiterKey(kind string, value string) string {
	sum := sha256.Sum256([]byte(value))
	return kind + ":" + hex.EncodeToString(sum[:])
}
