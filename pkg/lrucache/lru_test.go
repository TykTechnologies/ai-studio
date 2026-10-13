package lrucache

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_BoundedLeastRecentlyUsed(t *testing.T) {
	c := New[int](3)
	c.Add("a", 1)
	c.Add("b", 2)
	c.Add("c", 3)
	require.Equal(t, 3, c.Len())

	// Touch "a" so "b" is the least recently used, then overflow.
	_, ok := c.Get("a")
	require.True(t, ok)
	c.Add("d", 4)

	assert.Equal(t, 3, c.Len())
	_, ok = c.Get("b")
	assert.False(t, ok, "the least recently used entry is the one evicted")
	for _, k := range []string{"a", "c", "d"} {
		_, ok := c.Get(k)
		assert.True(t, ok, k)
	}

	// Add replaces in place and keeps the count.
	c.Add("a", 10)
	v, _ := c.Get("a")
	assert.Equal(t, 10, v)
	assert.Equal(t, 3, c.Len())

	c.Remove("a")
	_, ok = c.Get("a")
	assert.False(t, ok)
	c.Reset()
	assert.Equal(t, 0, c.Len())
}

func TestCache_StaysUnderCapacityUnderChurn(t *testing.T) {
	c := New[int](256)
	for i := 0; i < 256*4; i++ {
		c.Add(fmt.Sprintf("k-%d", i), i)
	}
	assert.Equal(t, 256, c.Len())
	_, ok := c.Get("k-0")
	assert.False(t, ok)
	_, ok = c.Get("k-1023")
	assert.True(t, ok)
}

// Many goroutines missing the same key at once share one build, so a cache
// miss under load does not fan out into a herd of identical builds.
func TestCache_DoBuildsOnceUnderConcurrentMisses(t *testing.T) {
	c := New[string](8)
	var builds int32
	release := make(chan struct{})

	var wg sync.WaitGroup
	results := make([]string, 50)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, err := c.Do("shared", func() (string, error) {
				atomic.AddInt32(&builds, 1)
				<-release
				return "built", nil
			})
			require.NoError(t, err)
			results[i] = v
		}(i)
	}
	close(release)
	wg.Wait()

	assert.Equal(t, int32(1), atomic.LoadInt32(&builds))
	for _, r := range results {
		assert.Equal(t, "built", r)
	}
	assert.Equal(t, 1, c.Len())
}

func TestCache_DoDoesNotCacheFailures(t *testing.T) {
	c := New[string](8)
	boom := errors.New("boom")
	_, err := c.Do("k", func() (string, error) { return "", boom })
	assert.ErrorIs(t, err, boom)
	assert.Equal(t, 0, c.Len())

	v, err := c.Do("k", func() (string, error) { return "ok", nil })
	require.NoError(t, err)
	assert.Equal(t, "ok", v)
	v, err = c.Do("k", func() (string, error) { t.Fatal("must not rebuild a cached key"); return "", nil })
	require.NoError(t, err)
	assert.Equal(t, "ok", v)
}

func TestCache_WeightedBound(t *testing.T) {
	c := NewWeighted[string](10, 10, func(_ string, v string) int64 { return int64(len(v)) })
	c.Add("a", "1234")
	c.Add("b", "1234")
	require.Equal(t, int64(8), c.Weight())

	// Over the weight bound: the least recently used entry goes, even though
	// the entry count is well within its bound.
	c.Add("c", "1234")
	assert.Equal(t, 2, c.Len())
	assert.Equal(t, int64(8), c.Weight())
	_, ok := c.Get("a")
	assert.False(t, ok)

	// Replacing a value re-weighs it.
	c.Add("b", "1")
	assert.Equal(t, int64(5), c.Weight())

	// A value heavier than the whole bound is not stored and evicts nothing.
	c.Add("huge", "12345678901")
	_, ok = c.Get("huge")
	assert.False(t, ok)
	assert.Equal(t, 2, c.Len())

	c.Remove("b")
	assert.Equal(t, int64(4), c.Weight())
	c.Reset()
	assert.Equal(t, int64(0), c.Weight())
	assert.Equal(t, 0, c.Len())
}

func TestCache_WeightedReplaceWithHeavyValueDropsOld(t *testing.T) {
	c := NewWeighted[string](10, 5, func(_ string, v string) int64 { return int64(len(v)) })
	c.Add("a", "12")
	c.Add("a", "123456")
	_, ok := c.Get("a")
	assert.False(t, ok, "an oversized replacement must not leave the stale value behind")
	assert.Equal(t, int64(0), c.Weight())
}
