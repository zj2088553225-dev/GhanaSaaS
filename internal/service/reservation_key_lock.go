package service

import "sync"

// reservationKeyLocker serializes expensive provider operations for one
// pseudonymous key without retaining an unbounded key map after callers leave.
// It is an in-process safeguard; the database constraints remain the final
// cross-instance concurrency boundary.
type reservationKeyLocker struct {
	mu    sync.Mutex
	locks map[string]*reservationKeyLock
}

type reservationKeyLock struct {
	mu   sync.Mutex
	refs int
}

func newReservationKeyLocker() *reservationKeyLocker {
	return &reservationKeyLocker{locks: make(map[string]*reservationKeyLock)}
}

func (l *reservationKeyLocker) acquire(key string) func() {
	l.mu.Lock()
	item := l.locks[key]
	if item == nil {
		item = &reservationKeyLock{}
		l.locks[key] = item
	}
	item.refs++
	l.mu.Unlock()

	item.mu.Lock()
	return func() {
		item.mu.Unlock()
		l.mu.Lock()
		item.refs--
		if item.refs == 0 {
			delete(l.locks, key)
		}
		l.mu.Unlock()
	}
}
