package httpapi

import (
	"sync"
	"time"
)

type loginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string]attemptWindow
	limit    int
	window   time.Duration
	now      func() time.Time
}

type attemptWindow struct {
	count   int
	resetAt time.Time
}

func newLoginRateLimiter(limit int, window time.Duration) *loginRateLimiter {
	return &loginRateLimiter{attempts: make(map[string]attemptWindow), limit: limit, window: window, now: time.Now}
}

func (l *loginRateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	entry := l.attempts[key]
	if entry.resetAt.IsZero() || !now.Before(entry.resetAt) {
		entry = attemptWindow{resetAt: now.Add(l.window)}
	}
	if entry.count >= l.limit {
		l.attempts[key] = entry
		return false
	}
	entry.count++
	l.attempts[key] = entry
	return true
}

func (l *loginRateLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}
