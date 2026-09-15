//go:build enterprise

package scripting

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
)

func cachedGuardrail(id uint, updated time.Time) *models.Filter {
	f := &models.Filter{
		ID:   id,
		Name: "cached",
		Kind: models.FilterKindGuardrail,
		Config: models.JSONMap{
			"provider":  "builtin",
			"detectors": []any{map[string]any{"name": "secrets"}},
			"on_detect": "block",
		},
	}
	f.UpdatedAt = updated
	return f
}

func runOnce(t *testing.T, f *models.Filter) *ScriptOutput {
	t.Helper()
	out, err := NewFilterRunner(f).RunScript(&ScriptInput{
		RawInput: `{"messages":[{"role":"user","content":"key AKIAIOSFODNN7EXAMPLE"}]}`,
		Messages: []llms.MessageContent{{Role: llms.ChatMessageTypeHuman, Parts: []llms.ContentPart{llms.TextPart("key AKIAIOSFODNN7EXAMPLE")}}},
	}, nil)
	require.NoError(t, err)
	return out
}

// A saved filter is prepared once per version and reused; a new version, or
// a filter with no identity (the test endpoint's temporary one), is not
// served from the cache.
func TestGuardrailRunner_PreparesASavedFilterOncePerVersion(t *testing.T) {
	preparedMu.Lock()
	prepared = map[string]*preparedGuardrail{}
	preparedMu.Unlock()

	v1 := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	f := cachedGuardrail(7, v1)

	require.True(t, runOnce(t, f).Block)
	first, ok := lookupPrepared(preparedKey(f))
	require.True(t, ok, "the first run stores the prepared entry")

	require.True(t, runOnce(t, f).Block)
	second, ok := lookupPrepared(preparedKey(f))
	require.True(t, ok)
	assert.Same(t, first, second, "the second run reuses the entry rather than preparing again")

	// Editing the filter bumps UpdatedAt: a different key, a fresh entry.
	edited := cachedGuardrail(7, v1.Add(time.Minute))
	require.True(t, runOnce(t, edited).Block)
	third, ok := lookupPrepared(preparedKey(edited))
	require.True(t, ok)
	assert.NotSame(t, first, third)
	_, ok = lookupPrepared(preparedKey(f))
	assert.True(t, ok, "the previous version stays until it expires; nothing else refers to it")

	// A temporary filter has no key and leaves nothing behind.
	temp := cachedGuardrail(0, time.Time{})
	before := len(prepared)
	require.True(t, runOnce(t, temp).Block)
	assert.Equal(t, "", preparedKey(temp))
	assert.Equal(t, before, len(prepared))

	// An expired entry is prepared again.
	preparedMu.Lock()
	prepared[preparedKey(f)].at = time.Now().Add(-2 * preparedTTL)
	preparedMu.Unlock()
	_, ok = lookupPrepared(preparedKey(f))
	assert.False(t, ok)
	require.True(t, runOnce(t, f).Block)
	fresh, ok := lookupPrepared(preparedKey(f))
	require.True(t, ok)
	assert.NotSame(t, first, fresh)
}

// A misconfigured filter is reported every time and never cached, so fixing
// it takes effect on the next request.
func TestGuardrailRunner_DoesNotCacheFailures(t *testing.T) {
	f := cachedGuardrail(8, time.Now())
	f.Config = models.JSONMap{"provider": "builtin", "on_detect": "block"} // no detectors

	_, err := NewFilterRunner(f).RunScript(&ScriptInput{RawInput: "x"}, nil)
	require.Error(t, err)
	_, ok := lookupPrepared(preparedKey(f))
	assert.False(t, ok)
}
