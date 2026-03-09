package ratelimit

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

var nextID atomic.Uint64

// Result holds the outcome of a rate limit check.
type Result struct {
	Allowed    bool
	RetryAfter time.Duration
}

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
func (l *Limiter) Allow(ctx context.Context, key string) (Result, error) {
	prefixed := l.prefix + key

	count, err := l.backend.Record(ctx, prefixed, l.window)
	if err != nil {
		return Result{}, fmt.Errorf("ratelimit record: %w", err)
	}

	if count > l.limit {
		oldest, err := l.backend.Oldest(ctx, prefixed, l.window)
		if err != nil {
			return Result{Allowed: false, RetryAfter: l.window}, fmt.Errorf("ratelimit oldest: %w", err)
		}
		retryAfter := l.window
		if !oldest.IsZero() {
			retryAfter = max(time.Until(oldest.Add(l.window)), 0)
		}
		return Result{Allowed: false, RetryAfter: retryAfter}, nil
	}

	return Result{Allowed: true}, nil
}

// Reset clears the rate limit state for a key.
func (l *Limiter) Reset(ctx context.Context, key string) error {
	return l.backend.Reset(ctx, l.prefix+key)
}
