package secrets

import (
	"container/list"
	"sync"
)

// keyCache is a small thread-safe LRU cache for scrypt-derived keys. scrypt
// is deliberately expensive, so each (secret, salt) derivation is cached;
// LRU eviction means a working set larger than the capacity degrades
// gradually instead of triggering a full-cache flush and a stampede of
// re-derivations.
type keyCache struct {
	mu       sync.Mutex
	capacity int
	order    *list.List // front = most recently used
	entries  map[string]*list.Element
}

type keyCacheEntry struct {
	key   string
	value []byte
}

func newKeyCache(capacity int) *keyCache {
	return &keyCache{
		capacity: capacity,
		order:    list.New(),
		entries:  make(map[string]*list.Element, capacity),
	}
}

func (c *keyCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*keyCacheEntry).value, true
}

func (c *keyCache) put(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.entries[key]; ok {
		el.Value.(*keyCacheEntry).value = value
		c.order.MoveToFront(el)
		return
	}

	c.entries[key] = c.order.PushFront(&keyCacheEntry{key: key, value: value})

	if c.order.Len() > c.capacity {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			delete(c.entries, oldest.Value.(*keyCacheEntry).key)
		}
	}
}

func (c *keyCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}
