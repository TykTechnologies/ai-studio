package ociplugins

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// With signatures required and no security service, every fetch is refused
// and nothing is cached: there is no built-in verifier to fall back to.
func TestFetchPlugin_NoSecurityServiceFailsClosed(t *testing.T) {
	client, puller, _, ref, params := fetchFixture(t)
	client.SetSecurityService(nil)

	_, err := client.FetchPlugin(context.Background(), ref, params)
	var verr *ErrSignatureVerificationFailed
	require.True(t, errors.As(err, &verr), "want ErrSignatureVerificationFailed, got %v", err)
	assert.Equal(t, "no signature verifier configured", verr.Reason)
	assert.Equal(t, 1, puller.pulls)
	assert.False(t, client.HasPlugin(ref.Digest, cacheTestArch), "nothing is cached")
	_, err = client.storage.LoadMetadata(ref.Digest, cacheTestArch)
	assert.Error(t, err, "no metadata is stored")
}

// Without signatures required, no security service is needed.
func TestFetchPlugin_NoSecurityServiceSignaturesOptional(t *testing.T) {
	client, _, _, ref, params := fetchFixture(t)
	client.SetSecurityService(nil)
	client.config.RequireSignature = false

	local, err := client.FetchPlugin(context.Background(), ref, params)
	require.NoError(t, err)
	assert.False(t, local.Verified)
}

// Verified means the check ran and passed, whichever key it used: the
// default key (no pubkey parameter) as much as a named one.
func TestFetchPlugin_VerifiedWhicheverKey(t *testing.T) {
	for _, publicKey := range []string{"", "1", "CI"} {
		t.Run("key="+publicKey, func(t *testing.T) {
			client, _, verifier, ref, params := fetchFixture(t)
			params.PublicKey = publicKey

			local, err := client.FetchPlugin(context.Background(), ref, params)
			require.NoError(t, err)
			assert.Equal(t, 1, verifier.calls)
			assert.True(t, local.Verified)

			cached, err := client.GetPlugin(ref, params)
			require.NoError(t, err)
			assert.True(t, cached.Verified)
		})
	}
}
