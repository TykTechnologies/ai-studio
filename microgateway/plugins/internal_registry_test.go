package plugins

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
	"github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins/ocitest"
)

// internalRegistryFixture pushes a plugin to a test registry (on loopback)
// and returns a gateway plugin manager with an OCI client, the command for
// that registry, and the same plugin named on a registry hostname that
// resolves to 10.0.0.5.
func internalRegistryFixture(t *testing.T) (*PluginManager, *ocitest.Registry, string, string) {
	t.Helper()
	reg := ocitest.NewRegistry(t) // allows internal hosts for the test
	manifest := reg.PushPlugin("plugins/demo", "1.0.0", "linux/amd64", []byte("binary"))
	suffix := "/plugins/demo@" + manifest.Digest.String() + "?arch=linux/amd64"
	netguardtest.StubLookupIP(t, map[string]string{"registry.corp.internal": "10.0.0.5"})

	cfg := ociplugins.DefaultOCIConfig()
	cfg.CacheDir = t.TempDir()
	cfg.GCInterval = 0
	cfg.InsecureRegistries = []string{"registry.corp.internal"}
	pm, err := NewPluginManagerWithOCI(&mockPluginService{}, cfg, acceptingVerifier{})
	require.NoError(t, err)
	return pm, reg, "oci://" + reg.Host() + suffix, "oci://registry.corp.internal" + suffix
}

func requireInternalHostRefused(t *testing.T, err error, host string) {
	t.Helper()
	require.ErrorIs(t, err, netguard.ErrInternalPluginHost)
	assert.Contains(t, err.Error(), host, "the error names the host")
	assert.Contains(t, err.Error(), "ALLOW_INTERNAL_NETWORK_ACCESS", "the error names the setting")
}

// Pre-fetch (the pre-warm path) refuses an OCI registry that resolves to an
// internal address before any request is made, and allows it when the host is
// named in LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS or with
// ALLOW_INTERNAL_NETWORK_ACCESS=true (midsommar-clean-c1g.32).
func TestPreFetchOCIPlugin_InternalRegistryRefused(t *testing.T) {
	pm, reg, loopback, internalName := internalRegistryFixture(t)

	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "")
	requireInternalHostRefused(t, pm.PreFetchOCIPlugin(internalName), "registry.corp.internal")
	requireInternalHostRefused(t, pm.PreFetchOCIPlugin(loopback), "127.0.0.1")
	for _, method := range []string{"GET", "HEAD"} {
		assert.Zero(t, reg.Requests(method, "manifests"), "%s manifests reached the registry", method)
	}

	// Exempt by name: allowed, and the dial is not guarded.
	t.Setenv("LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS", "127.0.0.1")
	require.NoError(t, pm.PreFetchOCIPlugin(loopback))
	requireInternalHostRefused(t, pm.PreFetchOCIPlugin(internalName), "registry.corp.internal")
	t.Setenv("LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS", "")

	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "true")
	require.NoError(t, pm.PreFetchOCIPlugin(loopback))
	assert.NotErrorIs(t, pm.PreFetchOCIPlugin(internalName), netguard.ErrInternalPluginHost,
		"with internal hosts allowed the name passes the rule (and then fails to resolve for real)")
}

// Pre-warm marks a stored OCI plugin on an internal registry failed in
// plugin health, with the host and the setting in the message.
func TestPreWarmOCIPlugins_InternalRegistryFailed(t *testing.T) {
	pm, _, _, internalName := internalRegistryFixture(t)
	pm.service = &mockPluginService{plugins: []PluginData{{ID: 3, Name: "corp", HookType: "post_auth", Command: internalName, IsActive: true}}}

	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "")
	_ = pm.PreWarmOCIPlugins(context.Background())
	health := pm.GetPluginHealth()[3]
	require.NotNil(t, health)
	assert.Equal(t, PluginStatusFailed, health.Status)
	assert.Contains(t, health.ErrorMessage, "registry.corp.internal")
	assert.Contains(t, health.ErrorMessage, "ALLOW_INTERNAL_NETWORK_ACCESS")
}

// A stored or hub-pushed OCI plugin on an internal registry is not loaded,
// even when a copy is already cached: installed plugins follow the rule too.
func TestCreatePluginClient_OCIInternalRegistryRefusedEvenWhenCached(t *testing.T) {
	pm, _, loopback, internalName := internalRegistryFixture(t)

	// Cached while internal hosts were allowed.
	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "true")
	require.NoError(t, pm.PreFetchOCIPlugin(loopback))

	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "")
	for host, command := range map[string]string{"127.0.0.1": loopback, "registry.corp.internal": internalName} {
		client, err := pm.createPluginClient(command, netguard.Exemption{})
		assert.Nil(t, client, command)
		requireInternalHostRefused(t, err, host)
	}
}

// ReconcilePlugins (the hub-pushed load path) leaves the plugin unloaded and
// failed in plugin health.
func TestReconcile_HubPushedOCIPluginOnInternalRegistryRefused(t *testing.T) {
	pm, _, _, internalName := internalRegistryFixture(t)
	pushed := PluginData{ID: 9, Name: "corp", HookType: "post_auth", Command: internalName, IsActive: true}
	pm.service = &mockPluginService{
		allActiveGwPlugins: []PluginData{pushed},
		getPluginByID:      map[uint]PluginData{9: pushed},
	}

	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "")
	require.NoError(t, pm.ReconcilePlugins(context.Background()))
	assert.False(t, pm.IsPluginLoaded(9))
	health := pm.GetPluginHealth()[9]
	require.NotNil(t, health)
	assert.Equal(t, PluginStatusFailed, health.Status)
	assert.Contains(t, health.ErrorMessage, "registry.corp.internal")
	assert.Contains(t, health.ErrorMessage, "ALLOW_INTERNAL_NETWORK_ACCESS")
}
