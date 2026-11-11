package services

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/credhash"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testEncryptionKey is the edge ENCRYPTION_KEY the service tests hash with.
const testEncryptionKey = "0123456789abcdef0123456789abcdef"

func testCredentialHasher(t testing.TB) *credhash.Hasher {
	t.Helper()
	h, err := credhash.New(testEncryptionKey)
	require.NoError(t, err)
	return h
}

// The edge keys its token cache with the hash the hub computes: the vectors
// here are the ones fixed in pkg/credhash/credhash_test.go, which the root
// module (the hub) asserts. Both modules build pkg/credhash from the same
// source, so a failure here means the two would no longer agree.
func TestCacheKeyIsTheCredentialHashTheHubComputes(t *testing.T) {
	h := newStaleGraceService(t, 0)
	for token, want := range map[string]string{
		"tyk-test-credential": "v1:03c819a3f64b42c74f216528f1141cd52aedf44c251b106b18c42a32b904ce45",
		"5f2b1c9e7a4d4e0f9b3a6c8d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f": "v1:f1b41918681329798b132b8035ad7607044fd01d970ae48fd5ce08a5c5cb28eb",
	} {
		assert.Equal(t, want, h.cacheKey(token))
	}
}

// The token cache holds no raw token: not as a map key and not anywhere in
// an entry. Entries are checked by printing them in full (%+v follows the
// result and its App), so a field added later that carried the token would
// fail here.
func TestTokenCacheHoldsNoRawToken(t *testing.T) {
	const token = "5f2b1c9e7a4d4e0f9b3a6c8d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f"
	h, client := newRefreshTestService(t, time.Minute)

	res, err := h.ValidateAPIToken(token)
	require.NoError(t, err)
	require.NotNil(t, res)
	// Served from the cache the second time: the entry is the one under test.
	_, err = h.ValidateAPIToken(token)
	require.NoError(t, err)
	require.EqualValues(t, 1, client.calls.Load(), "the second validation must be a cache hit")

	h.cacheMutex.RLock()
	defer h.cacheMutex.RUnlock()
	require.Len(t, h.tokenCache, 1)
	for key, entry := range h.tokenCache {
		assert.True(t, credhash.IsHash(key), "cache key %q is not a credential hash", key)
		assert.Equal(t, h.cacheKey(token), key)
		assert.NotContains(t, key, token)
		assert.NotContains(t, key, token[:8], "not even the logged prefix is in the key")
		dump := fmt.Sprintf("%+v %+v %+v", entry, entry.Result, entry.Result.App)
		assert.False(t, strings.Contains(dump, token), "a cache entry holds the raw token: %s", dump)
	}
}

// A rejection evicts the entry stored under the hash, and a stale entry is
// found under it: every path uses the same key.
func TestTokenCachePathsShareTheHashedKey(t *testing.T) {
	const token = "tok-hashed-paths"
	h, client := newRefreshTestService(t, time.Minute)
	h.cacheConfig.TokenCacheStaleGrace = time.Hour

	_, err := h.ValidateAPIToken(token)
	require.NoError(t, err)
	require.NotNil(t, h.getFromCache(h.cacheKey(token)))
	require.Nil(t, h.getFromCache(token), "nothing is stored under the raw token")

	// Expired, hub unreachable: the stale entry is served.
	h.cacheMutex.Lock()
	h.tokenCache[h.cacheKey(token)].ExpiresAt = time.Now().Add(-time.Minute)
	h.cacheMutex.Unlock()
	h.SetEdgeClient(&fakeEdgeClient{err: assert.AnError})
	stale, err := h.ValidateAPIToken(token)
	require.NoError(t, err)
	require.NotNil(t, stale)

	// The hub rejects it: gone.
	h.SetEdgeClient(&fakeEdgeClient{resp: &pb.TokenValidationResponse{Valid: false, ErrorMessage: "revoked"}})
	_, err = h.ValidateAPIToken(token)
	require.Error(t, err)
	h.cacheMutex.RLock()
	assert.Empty(t, h.tokenCache)
	h.cacheMutex.RUnlock()
	_ = client
}
