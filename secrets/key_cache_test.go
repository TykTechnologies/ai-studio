package secrets

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Visor review follow-up: the derived-key cache must evict least-recently-used
// entries rather than discarding the whole cache when full, so a working set
// larger than the cap degrades gradually instead of causing scrypt stampedes.

func TestKeyCacheLRUEviction(t *testing.T) {
	c := newKeyCache(3)

	c.put("a", []byte{1})
	c.put("b", []byte{2})
	c.put("c", []byte{3})

	// Touch "a" so "b" becomes the least recently used
	_, ok := c.get("a")
	assert.True(t, ok)

	// Inserting a fourth entry evicts only "b"
	c.put("d", []byte{4})

	_, ok = c.get("b")
	assert.False(t, ok, "least-recently-used entry should be evicted")
	for _, k := range []string{"a", "c", "d"} {
		_, ok := c.get(k)
		assert.True(t, ok, "entry %q should have survived eviction", k)
	}
}

func TestKeyCacheNoFullFlush(t *testing.T) {
	cap := 8
	c := newKeyCache(cap)

	// Insert 3x capacity; the cache must stay full rather than being
	// repeatedly flushed to empty.
	for i := 0; i < cap*3; i++ {
		c.put(fmt.Sprintf("k%d", i), []byte{byte(i)})
	}
	assert.Equal(t, cap, c.len(), "cache should remain at capacity, not be flushed")

	// The most recent `cap` entries are retained
	for i := cap * 2; i < cap*3; i++ {
		_, ok := c.get(fmt.Sprintf("k%d", i))
		assert.True(t, ok, "recent entry k%d should be cached", i)
	}
}

func TestKeyCacheUpdateExisting(t *testing.T) {
	c := newKeyCache(2)
	c.put("a", []byte{1})
	c.put("a", []byte{9})
	v, ok := c.get("a")
	assert.True(t, ok)
	assert.Equal(t, []byte{9}, v)
	assert.Equal(t, 1, c.len())
}
