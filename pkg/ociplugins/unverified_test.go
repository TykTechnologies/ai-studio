package ociplugins

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nonVerifyingStub is a security service that cannot verify signatures, as
// the Community Edition service reports.
type nonVerifyingStub struct{ stubVerifier }

func (*nonVerifyingStub) VerifiesSignatures() bool { return false }

// A service that cannot verify signatures loads plugins but never records
// them as verified, and its cache entries are reused without pulling again.
func TestFetchPlugin_NonVerifyingServiceRecordsUnverified(t *testing.T) {
	client, puller, _, ref, params := fetchFixture(t)
	client.SetSecurityService(&nonVerifyingStub{})

	local, err := client.FetchPlugin(context.Background(), ref, params)
	require.NoError(t, err)
	assert.False(t, local.Verified)
	meta, err := client.storage.LoadMetadata(ref.Digest, cacheTestArch)
	require.NoError(t, err)
	assert.False(t, meta.Verified)
	assert.False(t, meta.SignatureChecked, "nothing checked the signature")
	assert.Empty(t, meta.SignedDigest)

	local, err = client.FetchPlugin(context.Background(), ref, params)
	require.NoError(t, err)
	assert.False(t, local.Verified)
	assert.Equal(t, 1, puller.pulls, "the unverified entry is served from the cache")

	// A verifying service sharing the cache refuses the entry and verifies it.
	verifier := &stubVerifier{}
	client.SetSecurityService(verifier)
	_, err = client.FetchPlugin(context.Background(), ref, params)
	require.NoError(t, err)
	assert.Equal(t, 2, puller.pulls)
	assert.Equal(t, 1, verifier.calls)
	meta, err = client.storage.LoadMetadata(ref.Digest, cacheTestArch)
	require.NoError(t, err)
	assert.True(t, meta.SignatureChecked)
	assert.Equal(t, ref.Digest, meta.SignedDigest)
}
