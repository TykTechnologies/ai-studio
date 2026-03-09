package ratelimit

import (
	"fmt"
	"sync/atomic"
	"time"
)

var nextID atomic.Uint64

// Limiter applies a rate limit using a pluggable Backend.
// Each Limiter instance has a unique prefix to avoid key collisions
// when multiple limiters share the same Backend.
type Limiter struct {
	backend Backend
	limit   int
	window  time.Duration
	prefix  string
}

// NewLimiter creates a rate limiter that allows limit requests per window,
// backed by the provided Backend.
func NewLimiter(backend Backend, limit int, window time.Duration) *Limiter {
	return &Limiter{
		backend: backend,
		limit:   limit,
		window:  window,
		prefix:  fmt.Sprintf("rl%d:", nextID.Add(1)),
	}
}

// Allow checks if a request identified by key is within the rate limit.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	return l.backend.Allow(l.prefix+key, l.limit, l.window)
}

// Reset clears the rate limit state for a key.
func (l *Limiter) Reset(key string) {
	l.backend.Reset(l.prefix + key)
}
