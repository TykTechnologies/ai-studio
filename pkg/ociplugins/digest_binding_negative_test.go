package ociplugins

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signedDigestsVerifier accepts only the digests it was told are signed and
// records every digest it was asked about.
type signedDigestsVerifier struct {
	stubVerifier
	signed map[string]bool
	asked  []string
}

func (v *signedDigestsVerifier) VerifySignature(_ context.Context, ref *OCIReference, _ string) error {
	v.asked = append(v.asked, ref.Digest)
	if !v.signed[ref.Digest] {
		return fmt.Errorf("no signature for %s", ref.Digest)
	}
	return nil
}

// A signature on the platform manifest alone does not admit a plugin pulled
// through a tag to an unsigned index: the index is what binds the platform
// manifests, so it is what must be signed (midsommar-clean-c1g.2).
func TestFetchPlugin_TagToUnsignedIndexIsRejectedEvenIfPlatformManifestSigned(t *testing.T) {
	client, _, reg := bindingFixture(t)
	amd64 := reg.PushPlugin("plugins/demo", "", "linux/amd64", []byte("amd64 binary"))
	arm64 := reg.PushPlugin("plugins/demo", "", "linux/arm64", []byte("arm64 binary"))
	index := reg.PushIndex("plugins/demo", "1.0.0", amd64, arm64)

	verifier := &signedDigestsVerifier{signed: map[string]bool{amd64.Digest.String(): true}}
	client.SetSecurityService(verifier)

	ref := &OCIReference{Registry: reg.Host(), Repository: "plugins/demo", Tag: "1.0.0"}
	_, err := client.FetchPlugin(context.Background(), ref, &OCIPluginParams{Architecture: "linux/amd64"})

	var verr *ErrSignatureVerificationFailed
	require.True(t, errors.As(err, &verr), "want ErrSignatureVerificationFailed, got %v", err)
	assert.Equal(t, []string{index.Digest.String()}, verifier.asked)
	assert.False(t, client.HasPlugin(amd64.Digest.String(), "linux/amd64"),
		"a plugin that failed verification must not be cached")
}

// The positive counterpart: with the index signed, a tag pull for either
// platform loads, and each is cached under its own platform digest.
func TestFetchPlugin_TagToSignedIndexLoadsEveryPlatform(t *testing.T) {
	client, _, reg := bindingFixture(t)
	amd64 := reg.PushPlugin("plugins/demo", "", "linux/amd64", []byte("amd64 binary"))
	arm64 := reg.PushPlugin("plugins/demo", "", "linux/arm64", []byte("arm64 binary"))
	index := reg.PushIndex("plugins/demo", "1.0.0", amd64, arm64)

	verifier := &signedDigestsVerifier{signed: map[string]bool{index.Digest.String(): true}}
	client.SetSecurityService(verifier)

	for platform, manifest := range map[string]string{
		"linux/amd64": amd64.Digest.String(),
		"linux/arm64": arm64.Digest.String(),
	} {
		ref := &OCIReference{Registry: reg.Host(), Repository: "plugins/demo", Tag: "1.0.0"}
		local, err := client.FetchPlugin(context.Background(), ref, &OCIPluginParams{Architecture: platform})
		require.NoError(t, err, platform)
		assert.Equal(t, manifest, local.Reference.Digest, platform)
	}
	assert.Equal(t, []string{index.Digest.String(), index.Digest.String()}, verifier.asked)
}

// A single-platform tag whose manifest is unsigned is rejected, as before.
func TestFetchPlugin_TagToUnsignedManifestIsRejected(t *testing.T) {
	client, _, reg := bindingFixture(t)
	manifest := reg.PushPlugin("plugins/single", "1.0.0", "linux/amd64", []byte("binary"))

	verifier := &signedDigestsVerifier{signed: map[string]bool{}}
	client.SetSecurityService(verifier)

	ref := &OCIReference{Registry: reg.Host(), Repository: "plugins/single", Tag: "1.0.0"}
	_, err := client.FetchPlugin(context.Background(), ref, &OCIPluginParams{Architecture: "linux/amd64"})

	var verr *ErrSignatureVerificationFailed
	require.True(t, errors.As(err, &verr), "want ErrSignatureVerificationFailed, got %v", err)
	assert.Equal(t, []string{manifest.Digest.String()}, verifier.asked)
	assert.False(t, client.HasPlugin(manifest.Digest.String(), "linux/amd64"))
}
