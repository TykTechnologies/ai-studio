package ociplugins

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// The cache-only path (GetPlugin, used to load without a registry round
// trip) applies the same rule as FetchPlugin: a pin to the platform digest
// is not served from a tag fetch's entry that was verified on the index
// (midsommar-clean-c1g.18/.20).
func TestPlatformFallback_GetPluginDoesNotServePlatformPinFromTagEntry(t *testing.T) {
	client, verifier, reg, index, amd64, _ := fallbackFixture(t)
	verifier.signed[index.Digest.String()] = true

	_, err := client.FetchPlugin(context.Background(), tagRef(reg), amd64Params())
	require.NoError(t, err)

	pin := &OCIReference{Registry: reg.Host(), Repository: "plugins/demo", Digest: amd64.Digest.String()}
	_, err = client.GetPlugin(pin, amd64Params())
	requireCacheCorrupted(t, err)
}

// An entry whose recorded signed digest is neither the digest it was
// resolved from nor its own platform manifest (another platform's, say) is
// not served, even though everything else about it matches.
func TestPlatformFallback_EntrySignedOnAnotherPlatformIsNotServed(t *testing.T) {
	client, verifier, reg, index, amd64, arm64 := fallbackFixture(t)
	verifier.signed[index.Digest.String()] = true
	pin := &OCIReference{Registry: reg.Host(), Repository: "plugins/demo", Digest: index.Digest.String()}

	_, err := client.FetchPlugin(context.Background(), pin, amd64Params())
	require.NoError(t, err)
	_, err = client.GetPlugin(pin, amd64Params())
	require.NoError(t, err, "the intact entry is served")

	meta, err := client.storage.LoadMetadata(index.Digest.String(), "linux/amd64")
	require.NoError(t, err)
	require.Equal(t, amd64.Digest.String(), meta.ManifestDigest)
	meta.SignedDigest = arm64.Digest.String()
	require.NoError(t, client.storage.StoreMetadata(index.Digest.String(), "linux/amd64", meta))

	_, err = client.GetPlugin(pin, amd64Params())
	requireCacheCorrupted(t, err)
}
