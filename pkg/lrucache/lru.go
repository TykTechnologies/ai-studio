// Package lrucache is a small bounded cache keyed by string: least recently
// used entries are evicted at capacity, and a value that is missing is built
// once even when many goroutines ask for it at the same moment.
package lrucache

import (
	"container/list"
	"sync"

	"golang.org/x/sync/singleflight"
)

// Cache holds up to max values. The zero value is not usable; call New or
// NewWeighted.
type Cache[V any] struct {
	mu    sync.Mutex
	max   int
	order *list.List // front is most recently used
	items map[string]*list.Element
	group singleflight.Group

	// weigh, when set, is the cost of a value; the entries held then also
	// stay within maxWeight in total.
	weigh     func(key string, v V) int64
	maxWeight int64
	weight    int64
}

type entry[V any] struct {
	key    string
	value  V
	weight int64
}

// New returns a cache that keeps at most max entries.
func New[V any](max int) *Cache[V] {
	if max < 1 {
		max = 1
	}
	return &Cache[V]{max: max, order: list.New(), items: make(map[string]*list.Element)}
}

// NewWeighted returns a cache that keeps at most max entries whose weights,
// as weigh reports them, add up to at most maxWeight. A value heavier than
// maxWeight on its own is not stored.
func NewWeighted[V any](max int, maxWeight int64, weigh func(key string, v V) int64) *Cache[V] {
	c := New[V](max)
	c.weigh = weigh
	c.maxWeight = maxWeight
	return c
}

// Get returns the value for key and marks it recently used.
func (c *Cache[V]) Get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*entry[V]).value, true
}

// Add stores v under key, replacing any previous value, and drops the least
// recently used entries until the cache is within capacity.
func (c *Cache[V]) Add(key string, v V) {
	var w int64
	if c.weigh != nil {
		w = c.weigh(key, v)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.removeElement(el)
	}
	if c.weigh != nil && w > c.maxWeight {
		return
	}
	c.items[key] = c.order.PushFront(&entry[V]{key: key, value: v, weight: w})
	c.weight += w
	for c.order.Len() > c.max || (c.weigh != nil && c.weight > c.maxWeight) {
		c.removeElement(c.order.Back())
	}
}

// Remove drops key if present.
func (c *Cache[V]) Remove(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.removeElement(el)
	}
}

// removeElement drops one entry; the caller holds mu.
func (c *Cache[V]) removeElement(el *list.Element) {
	e := el.Value.(*entry[V])
	c.order.Remove(el)
	delete(c.items, e.key)
	c.weight -= e.weight
}

// Len is the number of entries held.
func (c *Cache[V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// Reset drops every entry.
func (c *Cache[V]) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.order.Init()
	c.items = make(map[string]*list.Element)
	c.weight = 0
}

// Weight is the total weight of the entries held (0 for a cache made with
// New).
func (c *Cache[V]) Weight() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.weight
}

// Do returns the value for key, building and storing it with build when it
// is missing. Concurrent callers for the same missing key share one build;
// a build that fails stores nothing and every waiting caller sees the error.
func (c *Cache[V]) Do(key string, build func() (V, error)) (V, error) {
	if v, ok := c.Get(key); ok {
		return v, nil
	}
	v, err, _ := c.group.Do(key, func() (any, error) {
		if v, ok := c.Get(key); ok {
			return v, nil
		}
		v, err := build()
		if err != nil {
			return nil, err
		}
		c.Add(key, v)
		return v, nil
	})
	if err != nil {
		var zero V
		return zero, err
	}
	return v.(V), nil
}
