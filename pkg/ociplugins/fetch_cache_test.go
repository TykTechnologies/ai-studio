package ociplugins

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingPuller serves one plugin binary and counts registry pulls.
type countingPuller struct {
	manifestDigest string
	binary         []byte
	pulls          int
}

func (p *countingPuller) Pull(context.Context, *OCIReference, *OCIPluginParams) (*ocispec.Descriptor, *PluginConfig, []byte, error) {
	p.pulls++
	return &ocispec.Descriptor{Digest: digest.Digest(p.manifestDigest)}, &PluginConfig{Version: "1.0.0"}, p.binary, nil
}

// stubVerifier stands in for the signature service; err fails verification.
type stubVerifier struct {
	err   error
	calls int
}

func (v *stubVerifier) VerifySignature(context.Context, *OCIReference, string) error {
	v.calls++
	return v.err
}
func (v *stubVerifier) VerifyBundle(context.Context, *OCIReference, string, string) error {
	return nil
}
func (v *stubVerifier) VerifyWithPolicy(context.Context, *OCIReference, string) error {
	return nil
}

// fetchFixture returns a client with signatures required, whose registry and
// signature check are the given fakes, and a digest-pinned reference.
func fetchFixture(t *testing.T) (*OCIPluginClient, *countingPuller, *stubVerifier, *OCIReference, *OCIPluginParams) {
	t.Helper()
	config := DefaultOCIConfig()
	config.CacheDir = t.TempDir()
	config.RequireSignature = true
	client, err := NewOCIPluginClient(config)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	manifest := digestOf([]byte("manifest"))
	puller := &countingPuller{manifestDigest: manifest, binary: []byte("signed plugin binary")}
	verifier := &stubVerifier{}
	client.fetcher = puller
	client.SetSecurityService(verifier)

	ref := &OCIReference{Registry: "registry.example.com", Repository: "plugins/demo", Digest: manifest}
	return client, puller, verifier, ref, &OCIPluginParams{Architecture: cacheTestArch}
}

func TestFetchPlugin_CacheHitDoesNotPullAgain(t *testing.T) {
	client, puller, _, ref, params := fetchFixture(t)

	_, err := client.FetchPlugin(context.Background(), ref, params)
	require.NoError(t, err)
	meta, err := client.storage.LoadMetadata(ref.Digest, cacheTestArch)
	require.NoError(t, err)
	assert.NotEmpty(t, meta.BinarySHA256, "the fetch must record the binary hash")
	assert.True(t, meta.SignatureChecked, "the fetch must record that the signature was checked")

	_, err = client.FetchPlugin(context.Background(), ref, params)
	require.NoError(t, err)
	assert.Equal(t, 1, puller.pulls, "an intact cached plugin must load without pulling again")
}

func TestFetchPlugin_TamperedBinaryIsFetchedAgain(t *testing.T) {
	client, puller, verifier, ref, params := fetchFixture(t)
	local, err := client.FetchPlugin(context.Background(), ref, params)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(local.ExecutablePath, []byte("tampered"), 0o700))

	local, err = client.FetchPlugin(context.Background(), ref, params)
	require.NoError(t, err)
	assert.Equal(t, 2, puller.pulls)
	assert.Equal(t, 2, verifier.calls, "the refetch must verify the signature again")
	got, err := os.ReadFile(local.ExecutablePath)
	require.NoError(t, err)
	assert.Equal(t, string(puller.binary), string(got), "the refetch must replace the tampered file")

	_, err = client.GetPlugin(ref, params)
	require.NoError(t, err, "the replaced file must pass the cache check")
}

func TestFetchPlugin_TamperedBinaryWithFailedRefetchIsNotLoaded(t *testing.T) {
	client, _, verifier, ref, params := fetchFixture(t)
	local, err := client.FetchPlugin(context.Background(), ref, params)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(local.ExecutablePath, []byte("tampered"), 0o700))

	verifier.err = errors.New("signature does not verify")
	_, err = client.FetchPlugin(context.Background(), ref, params)
	var sigErr *ErrSignatureVerificationFailed
	require.True(t, errors.As(err, &sigErr), "FetchPlugin error = %v, want a signature failure", err)

	_, err = client.GetPlugin(ref, params)
	requireCacheCorrupted(t, err)
}

func TestFetchPlugin_LegacyEntryIsFetchedOnceThenCached(t *testing.T) {
	client, puller, _, ref, params := fetchFixture(t)
	// An entry cached by an older release: binary and metadata, no hash.
	_, err := client.storage.StoreExecutable(ref.Digest, cacheTestArch, []byte("old binary"))
	require.NoError(t, err)
	require.NoError(t, client.storage.StoreMetadata(ref.Digest, cacheTestArch, &PluginMetadata{Reference: ref, Params: params, Verified: true}))

	_, err = client.FetchPlugin(context.Background(), ref, params)
	require.NoError(t, err)
	_, err = client.FetchPlugin(context.Background(), ref, params)
	require.NoError(t, err)
	assert.Equal(t, 1, puller.pulls, "a legacy entry is fetched once, then served from the cache")
}
