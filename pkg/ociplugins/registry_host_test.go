package ociplugins

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
	"github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"
)

// cachedFetchFixture fetches a signed plugin into a fresh cache while its
// registry name resolves publicly, and returns the client and its fakes.
func cachedFetchFixture(t *testing.T) (*OCIPluginClient, *countingPuller, *OCIReference, *OCIPluginParams) {
	t.Helper()
	t.Setenv(netguard.EnvAllowInternal, "")
	client, puller, _, ref, params := fetchFixture(t)
	_, err := client.FetchPlugin(context.Background(), ref, params)
	require.NoError(t, err)
	require.Equal(t, 1, puller.pulls)
	return client, puller, ref, params
}

// While the registry name does not resolve (DNS down), a cache hit that
// passes the integrity, signature and key checks still loads, since no
// request is made; a fetch is refused (midsommar-clean-c1g.33).
func TestGetPlugin_VerifiedCacheHitLoadsWhileRegistryDoesNotResolve(t *testing.T) {
	client, puller, ref, params := cachedFetchFixture(t)
	netguardtest.StubLookupIP(t, map[string]string{ref.Registry: ""})

	local, err := client.GetPlugin(ref, params)
	require.NoError(t, err)
	assert.True(t, local.Verified)

	_, err = client.FetchPlugin(context.Background(), ref, params)
	require.ErrorIs(t, err, netguard.ErrInternalPluginHost)
	assert.True(t, netguard.IsUnresolvedPluginHost(err))
	assert.Equal(t, 1, puller.pulls, "no registry request")
}

// A registry name that resolves to an internal address is refused even on a
// verified cache hit.
func TestGetPlugin_CacheHitRefusedWhenRegistryResolvesInternally(t *testing.T) {
	client, puller, ref, params := cachedFetchFixture(t)
	netguardtest.StubLookupIP(t, map[string]string{ref.Registry: "10.0.0.5"})

	_, err := client.GetPlugin(ref, params)
	require.ErrorIs(t, err, netguard.ErrInternalPluginHost)
	assert.False(t, netguard.IsUnresolvedPluginHost(err))
	_, err = client.FetchPlugin(context.Background(), ref, params)
	require.ErrorIs(t, err, netguard.ErrInternalPluginHost)
	assert.Equal(t, 1, puller.pulls, "no registry request")
}

// A cache entry that fails its integrity check while the registry name does
// not resolve is refused, and the fetch it falls through to is refused
// without a registry request.
func TestGetPlugin_CorruptedCacheEntryRefusedWhileRegistryDoesNotResolve(t *testing.T) {
	client, puller, ref, params := cachedFetchFixture(t)
	path := client.storage.GetExecutablePath(ref.Digest, cacheTestArch)
	require.NoError(t, os.WriteFile(path, []byte("tampered binary"), 0o700))
	netguardtest.StubLookupIP(t, map[string]string{ref.Registry: ""})

	_, err := client.GetPlugin(ref, params)
	require.ErrorIs(t, err, netguard.ErrInternalPluginHost)
	_, err = client.FetchPlugin(context.Background(), ref, params)
	require.ErrorIs(t, err, netguard.ErrInternalPluginHost)
	assert.Equal(t, 1, puller.pulls, "no registry request")
}

// A plugin's own "Allow internal host" switch (w4e), carried on the
// reference, lets its internal registry through, and only that registry.
func TestCheckRegistryHost_PluginExemption(t *testing.T) {
	t.Setenv(netguard.EnvAllowInternal, "")
	t.Setenv(netguard.EnvAllowedInternalHosts, "")
	netguardtest.StubLookupIP(t, map[string]string{"registry.corp.internal": "10.0.0.9", "other.corp.internal": "10.0.0.10"})
	command := "oci://registry.corp.internal/plugins/p@sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	ref, _, err := ParseOCICommand(command)
	require.NoError(t, err)
	require.ErrorIs(t, CheckRegistryHost(ref), netguard.ErrInternalPluginHost)

	ref.Exemption = netguard.CommandExemption(true, command)
	require.NoError(t, CheckRegistryHost(ref))

	other := *ref
	other.Registry = "other.corp.internal"
	require.ErrorIs(t, CheckRegistryHost(&other), netguard.ErrInternalPluginHost, "the exemption is for the plugin's own registry")
}
