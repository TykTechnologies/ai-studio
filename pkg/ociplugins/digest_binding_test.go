package ociplugins

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins/ocitest"
)

// recordingVerifier accepts every artifact and records the digests it was
// asked to verify.
type recordingVerifier struct {
	stubVerifier
	digests []string
}

func (v *recordingVerifier) VerifySignature(_ context.Context, ref *OCIReference, _ string) error {
	v.digests = append(v.digests, ref.Digest)
	return nil
}

// bindingFixture returns a client with signatures required that pulls from
// an in-memory registry through the real ORAS fetcher.
func bindingFixture(t *testing.T) (*OCIPluginClient, *recordingVerifier, *ocitest.Registry) {
	t.Helper()
	config := DefaultOCIConfig()
	config.CacheDir = t.TempDir()
	config.GCInterval = 0
	config.RequireSignature = true
	client, err := NewOCIPluginClient(config)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	verifier := &recordingVerifier{}
	client.SetSecurityService(verifier)
	return client, verifier, ocitest.NewRegistry(t)
}

func TestFetchPlugin_TagToIndexVerifiesIndexDigest(t *testing.T) {
	client, verifier, reg := bindingFixture(t)
	amd64 := reg.PushPlugin("plugins/demo", "", "linux/amd64", []byte("amd64 binary"))
	arm64 := reg.PushPlugin("plugins/demo", "", "linux/arm64", []byte("arm64 binary"))
	index := reg.PushIndex("plugins/demo", "1.0.0", amd64, arm64)

	ref := &OCIReference{Registry: reg.Host(), Repository: "plugins/demo", Tag: "1.0.0"}
	local, err := client.FetchPlugin(context.Background(), ref, &OCIPluginParams{Architecture: "linux/arm64"})
	require.NoError(t, err)

	assert.Equal(t, []string{index.Digest.String()}, verifier.digests,
		"a tag to an index must be verified against the signed index digest, not the platform manifest")
	assert.Equal(t, arm64.Digest.String(), local.Reference.Digest,
		"the cache stays keyed by the platform manifest digest")

	meta, err := client.storage.LoadMetadata(arm64.Digest.String(), "linux/arm64")
	require.NoError(t, err)
	assert.Equal(t, index.Digest.String(), meta.SignedDigest)
}

func TestFetchPlugin_TagToManifestVerifiesManifestDigest(t *testing.T) {
	client, verifier, reg := bindingFixture(t)
	manifest := reg.PushPlugin("plugins/single", "1.0.0", "linux/amd64", []byte("binary"))

	ref := &OCIReference{Registry: reg.Host(), Repository: "plugins/single", Tag: "1.0.0"}
	local, err := client.FetchPlugin(context.Background(), ref, &OCIPluginParams{Architecture: "linux/amd64"})
	require.NoError(t, err)

	assert.Equal(t, []string{manifest.Digest.String()}, verifier.digests)
	assert.Equal(t, manifest.Digest.String(), local.Reference.Digest)
}

func TestFetchPlugin_DigestToIndexVerifiesIndexDigest(t *testing.T) {
	client, verifier, reg := bindingFixture(t)
	amd64 := reg.PushPlugin("plugins/demo", "", "linux/amd64", []byte("amd64 binary"))
	arm64 := reg.PushPlugin("plugins/demo", "", "linux/arm64", []byte("arm64 binary"))
	index := reg.PushIndex("plugins/demo", "", amd64, arm64)

	ref := &OCIReference{Registry: reg.Host(), Repository: "plugins/demo", Digest: index.Digest.String()}
	local, err := client.FetchPlugin(context.Background(), ref, &OCIPluginParams{Architecture: "linux/amd64"})
	require.NoError(t, err)

	assert.Equal(t, []string{index.Digest.String()}, verifier.digests)
	assert.Equal(t, index.Digest.String(), local.Reference.Digest,
		"a digest reference is cached under the digest it names, as before")
}
