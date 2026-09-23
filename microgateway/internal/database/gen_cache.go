package database

import (
	"sync"
	"time"
)

// GenCacheTTL bounds how long an entry is served even when the generation has
// not moved. The generation callbacks run when a statement executes, which for
// a statement inside a transaction is before the commit: a reader in that
// window can cache the pre-commit data under the new generation. The TTL caps
// that staleness; the sync paths also bump after they commit.
const GenCacheTTL = 2 * time.Second

// GenCache is a small read-through cache for configuration looked up on the
// request path. An entry is valid while the config generation it was read at
// is current and it is younger than GenCacheTTL.
type GenCache[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]genEntry[V]
}

type genEntry[V any] struct {
	v     V
	gen   uint64
	until time.Time
}

// NewGenCache returns an empty cache.
func NewGenCache[K comparable, V any]() *GenCache[K, V] {
	return &GenCache[K, V]{m: map[K]genEntry[V]{}}
}

// Get returns the cached value if it is still valid. A nil cache is empty.
func (c *GenCache[K, V]) Get(k K) (V, bool) {
	if c == nil {
		var zero V
		return zero, false
	}
	c.mu.RLock()
	e, ok := c.m[k]
	c.mu.RUnlock()
	if !ok || e.gen != ConfigGeneration() || time.Now().After(e.until) {
		var zero V
		return zero, false
	}
	return e.v, true
}

// Load returns the cached value or calls load and caches its result. Errors
// are not cached. The generation is captured before load runs, so a write
// that lands during the read leaves the entry already invalid. A nil cache
// always calls load.
func (c *GenCache[K, V]) Load(k K, load func() (V, error)) (V, error) {
	if c == nil {
		return load()
	}
	if v, ok := c.Get(k); ok {
		return v, nil
	}
	gen := ConfigGeneration()
	v, err := load()
	if err != nil {
		return v, err
	}
	c.mu.Lock()
	if len(c.m) > 10000 {
		// Keys are LLM slugs, LLM IDs and app IDs; far fewer exist. A map
		// this large means keys are not being reused, so start over.
		c.m = map[K]genEntry[V]{}
	}
	c.m[k] = genEntry[V]{v: v, gen: gen, until: time.Now().Add(GenCacheTTL)}
	c.mu.Unlock()
	return v, nil
}
