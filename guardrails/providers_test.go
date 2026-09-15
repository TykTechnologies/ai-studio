package guardrails

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubProvider struct{ id int }

func (stubProvider) Classify(context.Context, Input) (Verdict, error) { return Verdict{}, nil }
func (stubProvider) Capabilities() Capabilities                       { return Capabilities{} }

// Every edit of a guardrail filter is a new config key, so the cache must
// stay bounded and drop the configurations nobody runs any more.
func TestProviderCache_BoundedLeastRecentlyUsed(t *testing.T) {
	c := newProviderLRU(3)

	c.add("a", stubProvider{1})
	c.add("b", stubProvider{2})
	c.add("c", stubProvider{3})
	require.Equal(t, 3, c.len())

	// Touch "a" so "b" is the least recently used, then overflow.
	_, ok := c.get("a")
	require.True(t, ok)
	c.add("d", stubProvider{4})

	assert.Equal(t, 3, c.len())
	_, ok = c.get("b")
	assert.False(t, ok, "the least recently used entry is the one evicted")
	for _, k := range []string{"a", "c", "d"} {
		_, ok := c.get(k)
		assert.True(t, ok, k)
	}

	// A concurrent second build of the same key does not replace the first
	// instance: callers share one provider per configuration.
	first := c.add("e", stubProvider{5})
	second := c.add("e", stubProvider{6})
	assert.Equal(t, first, second)
	assert.Equal(t, stubProvider{5}, second)
}

func TestProviderCache_StaysUnderCapacityUnderChurn(t *testing.T) {
	c := newProviderLRU(providerCacheSize)
	for i := 0; i < providerCacheSize*4; i++ {
		c.add(fmt.Sprintf("cfg-%d", i), stubProvider{i})
	}
	assert.Equal(t, providerCacheSize, c.len())
	_, ok := c.get("cfg-0")
	assert.False(t, ok)
	_, ok = c.get(fmt.Sprintf("cfg-%d", providerCacheSize*4-1))
	assert.True(t, ok)
}
