package database

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// GenCacheTTL bounds how long an entry is served even when the generation has
// not moved. The generation callbacks run when a statement executes, which for
// a statement inside a transaction is before the commit: a reader in that
// window can cache the pre-commit data under the new generation. The TTL caps
// that staleness; the sync paths also bump after they commit.
const GenCacheTTL = 2 * time.Second

// genCacheMaxEntries is a safety bound. Keys are LLM slugs, LLM IDs and app
// IDs, of which a gateway has far fewer; reaching it means keys are not being
// reused.
const genCacheMaxEntries = 10000

// GenCache is a small read-through cache for configuration looked up on the
// request path. An entry is valid while the config generation it was read at
// is current and it is younger than GenCacheTTL.
//
// Values are shared between callers. Callers that hand them on to code that
// may modify them must copy them first (see DeepCopy).
type GenCache[K comparable, V any] struct {
	mu     sync.RWMutex
	m      map[K]genEntry[V]
	flight singleflight.Group
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
	if !ok || !e.valid(time.Now()) {
		var zero V
		return zero, false
	}
	return e.v, true
}

func (e genEntry[V]) valid(now time.Time) bool {
	return e.gen == ConfigGeneration() && !now.After(e.until)
}

// Load returns the cached value or calls load and caches its result. Errors
// are not cached. Concurrent misses for the same key share one load, so a
// configuration change does not send every in-flight request to the database
// at once. The generation is captured before load runs, so a write that
// lands during the read leaves the entry already invalid. A nil cache always
// calls load.
func (c *GenCache[K, V]) Load(k K, load func() (V, error)) (V, error) {
	if c == nil {
		return load()
	}
	if v, ok := c.Get(k); ok {
		return v, nil
	}
	res, err, _ := c.flight.Do(fmt.Sprint(k), func() (interface{}, error) {
		gen := ConfigGeneration()
		v, err := load()
		if err != nil {
			return v, err
		}
		c.store(k, genEntry[V]{v: v, gen: gen, until: time.Now().Add(GenCacheTTL)})
		return v, nil
	})
	v, _ := res.(V)
	return v, err
}

func (c *GenCache[K, V]) store(k K, e genEntry[V]) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= genCacheMaxEntries {
		// Entries live for GenCacheTTL, so sweeping the invalid ones
		// normally frees nearly everything. Start over only if it does not.
		now := time.Now()
		for key, old := range c.m {
			if !old.valid(now) {
				delete(c.m, key)
			}
		}
		if len(c.m) >= genCacheMaxEntries {
			c.m = map[K]genEntry[V]{}
		}
	}
	c.m[k] = e
}
