package ratelimit

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

var nextID atomic.Uint64

// Limiter implements rate limiting logic using a pluggable Backend for storage.
// Each Limiter instance has a unique key prefix to avoid collisions
// when multiple limiters share the same Backend.
type Limiter struct {
	backend Backend
	limit   int
	window  time.Duration
	prefix  string
}

// NewLimiter creates a rate limiter that allows limit requests per window.
func NewLimiter(backend Backend, limit int, window time.Duration) *Limiter {
	return &Limiter{
		backend: backend,
		limit:   limit,
		window:  window,
		prefix:  fmt.Sprintf("rl%d:", nextID.Add(1)),
	}
}

// Allow records a hit and checks if the request is within the rate limit.
// Returns whether the request is allowed and, if denied, how long to wait.
func (l *Limiter) Allow(ctx context.Context, key string) (bool, time.Duration) {
	prefixed := l.prefix + key

	count, err := l.backend.Record(ctx, prefixed, l.window)
	if err != nil {
		// Fail open: if backend is unavailable, allow the request.
		return true, 0
	}

	if count > l.limit {
		oldest, err := l.backend.Oldest(ctx, prefixed, l.window)
		if err != nil || oldest.IsZero() {
			return false, l.window
		}
		retryAfter := max(time.Until(oldest.Add(l.window)), 0)
		return false, retryAfter
	}

	return true, 0
}

// Reset clears the rate limit state for a key.
func (l *Limiter) Reset(ctx context.Context, key string) {
	l.backend.Reset(ctx, l.prefix+key) //nolint:errcheck
}
