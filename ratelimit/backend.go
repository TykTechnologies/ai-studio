package ratelimit

import (
	"context"
	"time"
)

// Backend abstracts the storage layer for rate limiting.
// Implementations handle recording hits and counting them within a time window.
// All methods accept a context for cancellation and timeout (e.g. Redis calls).
type Backend interface {
	// Record adds a hit for key and returns the current count within window.
	// The backend is responsible for expiring old entries (e.g. TTL in Redis,
	// timestamp pruning in memory).
	Record(ctx context.Context, key string, window time.Duration) (count int, err error)

	// Count returns the current hit count for key within window without recording a new hit.
	Count(ctx context.Context, key string, window time.Duration) (int, error)

	// Reset clears all state for a key.
	Reset(ctx context.Context, key string) error

	// Oldest returns the timestamp of the oldest hit within window for key.
	// Returns zero time if no hits exist. Used by Limiter to compute Retry-After.
	Oldest(ctx context.Context, key string, window time.Duration) (time.Time, error)
}
