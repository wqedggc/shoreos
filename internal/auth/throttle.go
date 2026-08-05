package auth

import (
	"strings"
	"sync"
	"time"
)

// LoginThrottle is a small process-local attempt limiter used by identity endpoints.
// The edge proxy still supplies an independent IP-based limit in production.
type LoginThrottle struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	now      func() time.Time
	failures map[string][]time.Time
}

func NewLoginThrottle(limit int, window time.Duration) *LoginThrottle {
	return newLoginThrottle(limit, window, time.Now)
}

func newLoginThrottle(limit int, window time.Duration, now func() time.Time) *LoginThrottle {
	if limit < 1 {
		limit = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	return &LoginThrottle{limit: limit, window: window, now: now, failures: make(map[string][]time.Time)}
}

func (t *LoginThrottle) Allow(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	key = normalizeKey(key)
	t.prune(key)
	return len(t.failures[key]) < t.limit
}

func (t *LoginThrottle) RecordFailure(key string) {
	t.RecordAttempt(key)
}

func (t *LoginThrottle) RecordAttempt(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	key = normalizeKey(key)
	t.prune(key)
	t.failures[key] = append(t.failures[key], t.now())
}

func (t *LoginThrottle) Reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, normalizeKey(key))
}

func (t *LoginThrottle) prune(key string) {
	cutoff := t.now().Add(-t.window)
	failures := t.failures[key]
	kept := failures[:0]
	for _, failedAt := range failures {
		if failedAt.After(cutoff) {
			kept = append(kept, failedAt)
		}
	}
	if len(kept) == 0 {
		delete(t.failures, key)
		return
	}
	t.failures[key] = kept
}

func normalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}
