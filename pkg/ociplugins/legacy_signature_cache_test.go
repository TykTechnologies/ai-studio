package ociplugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A cache entry written before SignedDigest existed claims a signature check
// it cannot prove: Community builds before c1g.16 wrote SignatureChecked=true
// without verifying anything. With signatures enforced, such an entry is
// fetched and verified once more instead of being served from the cache
// (midsommar-clean-c1g.19).
func TestFetchPlugin_PreSignedDigestEntryIsVerifiedAgain(t *testing.T) {
	for _, publicKey := range []string{"", "1"} {
		t.Run("key="+publicKey, func(t *testing.T) {
			client, puller, verifier, ref, params := fetchFixture(t)
			params.PublicKey = publicKey

			// The old format: intact binary with its hash, a recorded
			// signature check with the same key, and no SignedDigest.
			binary := []byte("binary cached by an unverifying release")
			sum := sha256.Sum256(binary)
			_, err := client.storage.StoreExecutable(ref.Digest, cacheTestArch, binary)
			require.NoError(t, err)
			require.NoError(t, client.storage.StoreMetadata(ref.Digest, cacheTestArch, &PluginMetadata{
				Reference:        ref,
				Params:           params,
				Verified:         true,
				BinarySHA256:     hex.EncodeToString(sum[:]),
				SignatureChecked: true,
				SignatureKey:     publicKey,
			}))

			_, err = client.FetchPlugin(context.Background(), ref, params)
			require.NoError(t, err)
			assert.Equal(t, 1, puller.pulls, "the old-format entry must be fetched again, not served from the cache")
			assert.Equal(t, 1, verifier.calls, "the refetch must verify the signature exactly once")

			meta, err := client.storage.LoadMetadata(ref.Digest, cacheTestArch)
			require.NoError(t, err)
			assert.Equal(t, ref.Digest, meta.SignedDigest, "the refetch records what it verified")

			// Now in the new format, the entry is served from the cache.
			_, err = client.FetchPlugin(context.Background(), ref, params)
			require.NoError(t, err)
			assert.Equal(t, 1, puller.pulls)
			assert.Equal(t, 1, verifier.calls)
		})
	}
}
