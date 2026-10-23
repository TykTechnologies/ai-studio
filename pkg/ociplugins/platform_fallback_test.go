package ociplugins

import (
	"context"
	"errors"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins/ocitest"
)

// fallbackFixture pushes a two-platform plugin under tag 1.0.0 and returns a
// client verifying through a signedDigestsVerifier with nothing signed yet.
func fallbackFixture(t *testing.T) (client *OCIPluginClient, verifier *signedDigestsVerifier, reg *ocitest.Registry, index, amd64, arm64 ocispec.Descriptor) {
	t.Helper()
	client, _, reg = bindingFixture(t)
	amd64 = reg.PushPlugin("plugins/demo", "", "linux/amd64", []byte("amd64 binary"))
	arm64 = reg.PushPlugin("plugins/demo", "", "linux/arm64", []byte("arm64 binary"))
	index = reg.PushIndex("plugins/demo", "1.0.0", amd64, arm64)
	verifier = &signedDigestsVerifier{signed: map[string]bool{}}
	client.SetSecurityService(verifier)
	return client, verifier, reg, index, amd64, arm64
}

func tagRef(reg *ocitest.Registry) *OCIReference {
	return &OCIReference{Registry: reg.Host(), Repository: "plugins/demo", Tag: "1.0.0"}
}

func amd64Params() *OCIPluginParams {
	return &OCIPluginParams{Architecture: "linux/amd64", PublicKey: "1"}
}

func signedDigestOf(t *testing.T, client *OCIPluginClient, cacheDigest string) string {
	t.Helper()
	meta, err := client.storage.LoadMetadata(cacheDigest, "linux/amd64")
	require.NoError(t, err)
	return meta.SignedDigest
}

// (a) Only the index is signed: verified on the index.
func TestPlatformFallback_IndexSigned(t *testing.T) {
	client, verifier, reg, index, amd64, _ := fallbackFixture(t)
	verifier.signed[index.Digest.String()] = true

	_, err := client.FetchPlugin(context.Background(), tagRef(reg), amd64Params())
	require.NoError(t, err)
	assert.Equal(t, []string{index.Digest.String()}, verifier.asked)
	assert.Equal(t, index.Digest.String(), signedDigestOf(t, client, amd64.Digest.String()))
}

// (b) Only the pulled platform manifest is signed: verified on it.
func TestPlatformFallback_PulledPlatformSigned(t *testing.T) {
	client, verifier, reg, index, amd64, _ := fallbackFixture(t)
	verifier.signed[amd64.Digest.String()] = true

	local, err := client.FetchPlugin(context.Background(), tagRef(reg), amd64Params())
	require.NoError(t, err)
	assert.True(t, local.Verified)
	assert.Equal(t, []string{index.Digest.String(), amd64.Digest.String()}, verifier.asked)
	assert.Equal(t, amd64.Digest.String(), signedDigestOf(t, client, amd64.Digest.String()))
}

// (c) Nothing signed: fails, naming both attempts, and caches nothing.
func TestPlatformFallback_NothingSigned(t *testing.T) {
	client, verifier, reg, index, amd64, _ := fallbackFixture(t)

	_, err := client.FetchPlugin(context.Background(), tagRef(reg), amd64Params())
	var verr *ErrSignatureVerificationFailed
	require.True(t, errors.As(err, &verr), "want ErrSignatureVerificationFailed, got %v", err)
	assert.Contains(t, verr.Reason, index.Digest.String())
	assert.Contains(t, verr.Reason, amd64.Digest.String())
	assert.Equal(t, []string{index.Digest.String(), amd64.Digest.String()}, verifier.asked)
	assert.False(t, client.HasPlugin(amd64.Digest.String(), "linux/amd64"), "nothing is cached on failure")
}

// (d) Only another platform's manifest is signed: fails without consulting it.
func TestPlatformFallback_OtherPlatformSigned(t *testing.T) {
	client, verifier, reg, index, amd64, arm64 := fallbackFixture(t)
	verifier.signed[arm64.Digest.String()] = true

	_, err := client.FetchPlugin(context.Background(), tagRef(reg), amd64Params())
	var verr *ErrSignatureVerificationFailed
	require.True(t, errors.As(err, &verr), "want ErrSignatureVerificationFailed, got %v", err)
	assert.Equal(t, []string{index.Digest.String(), amd64.Digest.String()}, verifier.asked)
}

// (e) Both signed: passes on the index without a second check.
func TestPlatformFallback_BothSigned(t *testing.T) {
	client, verifier, reg, index, amd64, _ := fallbackFixture(t)
	verifier.signed[index.Digest.String()] = true
	verifier.signed[amd64.Digest.String()] = true

	_, err := client.FetchPlugin(context.Background(), tagRef(reg), amd64Params())
	require.NoError(t, err)
	assert.Equal(t, []string{index.Digest.String()}, verifier.asked)
}

// A single-manifest reference is checked once, with no fallback.
func TestPlatformFallback_ManifestReferenceHasNoFallback(t *testing.T) {
	client, _, reg := bindingFixture(t)
	manifest := reg.PushPlugin("plugins/single", "1.0.0", "linux/amd64", []byte("binary"))
	verifier := &signedDigestsVerifier{signed: map[string]bool{}}
	client.SetSecurityService(verifier)

	ref := &OCIReference{Registry: reg.Host(), Repository: "plugins/single", Tag: "1.0.0"}
	_, err := client.FetchPlugin(context.Background(), ref, amd64Params())
	require.Error(t, err)
	assert.Equal(t, []string{manifest.Digest.String()}, verifier.asked)
}

// A puller that reports no resolved digest fails closed: the verifier is
// never handed a tag-only reference.
func TestFetchPlugin_NoResolvedDigestFailsClosed(t *testing.T) {
	client, puller, verifier, ref, params := fetchFixture(t)
	client.fetcher = &noDigestPuller{binary: puller.binary}

	tagOnly := *ref
	tagOnly.Digest, tagOnly.Tag = "", "1.0.0"
	_, err := client.FetchPlugin(context.Background(), &tagOnly, params)
	var verr *ErrSignatureVerificationFailed
	require.True(t, errors.As(err, &verr), "want ErrSignatureVerificationFailed, got %v", err)
	assert.Equal(t, 0, verifier.calls)
}

type noDigestPuller struct{ binary []byte }

func (p *noDigestPuller) Pull(context.Context, *OCIReference, *OCIPluginParams) (*PullResult, error) {
	return &PullResult{Config: &PluginConfig{}, Binary: p.binary}, nil
}

// A tag fetch caches a multi-arch plugin under the platform digest P, but
// verified it on the index. A later pin to @P gives the same answer as a
// fresh fetch of @P would: it is verified afresh (on P alone), not served
// from the tag fetch's entry (midsommar-clean-c1g.18).
func TestPlatformFallback_PinToPlatformAfterTagFetchVerifiesAfresh(t *testing.T) {
	client, verifier, reg, index, amd64, _ := fallbackFixture(t)
	verifier.signed[index.Digest.String()] = true

	_, err := client.FetchPlugin(context.Background(), tagRef(reg), amd64Params())
	require.NoError(t, err)
	require.True(t, client.HasPlugin(amd64.Digest.String(), "linux/amd64"))

	pin := &OCIReference{Registry: reg.Host(), Repository: "plugins/demo", Digest: amd64.Digest.String()}
	_, err = client.FetchPlugin(context.Background(), pin, amd64Params())
	require.Error(t, err, "P carries no signature of its own, so a fresh fetch of @P fails; so must the cache")
	assert.Equal(t, []string{index.Digest.String(), amd64.Digest.String()}, verifier.asked)

	// Once P is signed, the pin verifies on P and is then served from the cache.
	verifier.signed[amd64.Digest.String()] = true
	_, err = client.FetchPlugin(context.Background(), pin, amd64Params())
	require.NoError(t, err)
	_, err = client.FetchPlugin(context.Background(), pin, amd64Params())
	require.NoError(t, err)
	assert.Equal(t, []string{index.Digest.String(), amd64.Digest.String(), amd64.Digest.String()}, verifier.asked)
}

// A digest pin to the index is served from the cache after the first fetch,
// whether the index or (fallback) the platform manifest verified.
func TestPlatformFallback_IndexPinIsCached(t *testing.T) {
	for name, signPlatform := range map[string]bool{"index signed": false, "platform signed": true} {
		t.Run(name, func(t *testing.T) {
			client, verifier, reg, index, amd64, _ := fallbackFixture(t)
			if signPlatform {
				verifier.signed[amd64.Digest.String()] = true
			} else {
				verifier.signed[index.Digest.String()] = true
			}
			pin := &OCIReference{Registry: reg.Host(), Repository: "plugins/demo", Digest: index.Digest.String()}

			_, err := client.FetchPlugin(context.Background(), pin, amd64Params())
			require.NoError(t, err)
			asked := len(verifier.asked)
			manifestPulls := reg.Requests("GET", "manifests")

			_, err = client.FetchPlugin(context.Background(), pin, amd64Params())
			require.NoError(t, err)
			assert.Len(t, verifier.asked, asked, "the second load is a cache hit")
			assert.Equal(t, manifestPulls, reg.Requests("GET", "manifests"))
		})
	}
}
