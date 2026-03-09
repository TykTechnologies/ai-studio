package ratelimit

import "time"

// Backend abstracts the storage and counting mechanism for rate limiting.
// The default implementation is in-memory, but this can be swapped for
// Redis or any other distributed store.
type Backend interface {
	// Allow checks whether a request identified by key is within the rate limit.
	// Returns whether the request is allowed and, if denied, how long until a slot opens.
	Allow(key string, limit int, window time.Duration) (allowed bool, retryAfter time.Duration)

	// Reset clears the rate limit state for a key.
	Reset(key string)
}
