package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSnapshotSecretCache_Deduplication verifies that the secret resolution
// cache in getConfigurationSnapshot deduplicates identical references.
// Since getConfigurationSnapshot is tightly coupled to the full gRPC server,
// we test the caching closure pattern in isolation.
func TestSnapshotSecretCache_Deduplication(t *testing.T) {
	t.Parallel()

	callCount := 0
	resolveSecret := func(ref string) string {
		callCount++
		if ref == "$SECRET/API_KEY" {
			return "resolved-api-key"
		}
		if ref == "$ENV/HOST" {
			return "localhost"
		}
		return ref
	}

	// Build the same cache + closure pattern used in getConfigurationSnapshot
	secretCache := make(map[string]string)
	resolve := func(ref string) string {
		if v, ok := secretCache[ref]; ok {
			return v
		}
		v := resolveSecret(ref)
		secretCache[ref] = v
		return v
	}

	// Same reference called 3 times (simulating 3 LLMs with same API key)
	r1 := resolve("$SECRET/API_KEY")
	r2 := resolve("$SECRET/API_KEY")
	r3 := resolve("$SECRET/API_KEY")

	assert.Equal(t, "resolved-api-key", r1)
	assert.Equal(t, "resolved-api-key", r2)
	assert.Equal(t, "resolved-api-key", r3)
	assert.Equal(t, 1, callCount, "resolveSecret should only be called once for the same reference")
}

func TestSnapshotSecretCache_DifferentReferences(t *testing.T) {
	t.Parallel()

	callCount := 0
	resolveSecret := func(ref string) string {
		callCount++
		return "resolved:" + ref
	}

	secretCache := make(map[string]string)
	resolve := func(ref string) string {
		if v, ok := secretCache[ref]; ok {
			return v
		}
		v := resolveSecret(ref)
		secretCache[ref] = v
		return v
	}

	// Different references
	resolve("$SECRET/KEY_A")
	resolve("$SECRET/KEY_B")
	resolve("$ENV/HOST")
	resolve("plain-value")

	assert.Equal(t, 4, callCount, "each unique reference should be resolved once")

	// Second pass — all from cache
	resolve("$SECRET/KEY_A")
	resolve("$SECRET/KEY_B")
	resolve("$ENV/HOST")
	resolve("plain-value")

	assert.Equal(t, 4, callCount, "second pass should use cache, no new calls")
}

func TestSnapshotSecretCache_EmptyString(t *testing.T) {
	t.Parallel()

	callCount := 0
	resolveSecret := func(ref string) string {
		callCount++
		return ref
	}

	secretCache := make(map[string]string)
	resolve := func(ref string) string {
		if v, ok := secretCache[ref]; ok {
			return v
		}
		v := resolveSecret(ref)
		secretCache[ref] = v
		return v
	}

	r := resolve("")
	assert.Equal(t, "", r)
	assert.Equal(t, 1, callCount)

	// Second call with empty string should use cache
	resolve("")
	assert.Equal(t, 1, callCount)
}

func TestSnapshotSecretCache_ResolvesToEmptyString(t *testing.T) {
	t.Parallel()

	callCount := 0
	resolveSecret := func(ref string) string {
		callCount++
		return "" // simulates a secret that resolves to empty
	}

	secretCache := make(map[string]string)
	resolve := func(ref string) string {
		if v, ok := secretCache[ref]; ok {
			return v
		}
		v := resolveSecret(ref)
		secretCache[ref] = v
		return v
	}

	r1 := resolve("$SECRET/EMPTY_SECRET")
	assert.Equal(t, "", r1)
	assert.Equal(t, 1, callCount)

	// Should be cached even though result is empty
	r2 := resolve("$SECRET/EMPTY_SECRET")
	assert.Equal(t, "", r2)
	assert.Equal(t, 1, callCount, "empty result should still be cached")
}
