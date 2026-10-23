package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/marketplace"
	"github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins/ocitest"
)

func requireInternalHostError(t *testing.T, err error, host string) {
	t.Helper()
	require.Error(t, err)
	assert.Contains(t, err.Error(), host, "the error names the host")
	assert.Contains(t, err.Error(), "ALLOW_INTERNAL_NETWORK_ACCESS", "the error names the setting")
}

// Unless ALLOW_INTERNAL_NETWORK_ACCESS=true, every Studio OCI path refuses a
// plugin whose registry resolves to an internal address: create, refresh,
// update, manifest extraction, marketplace install and load of a stored
// plugin, the last even when a copy is cached (midsommar-clean-c1g.32).
// Upgrades go through validatePluginCommand
// (TestValidatePluginCommand_InternalTargets).
func TestOCIPaths_InternalRegistryRefused(t *testing.T) {
	db, cfg := setupSecurityWiringTest(t)
	cfg.RequireSignature = false // the test registry's plugin is unsigned
	svc := NewServiceWithOCI(db, cfg)

	reg := ocitest.NewRegistry(t) // loopback; allows internal hosts for now
	manifest := reg.PushPlugin("plugins/demo", "1.0.0", "linux/amd64", []byte("binary"))
	suffix := "/plugins/demo@" + manifest.Digest.String() + "?arch=linux/amd64"
	loopback := "oci://" + reg.Host() + suffix
	internalName := "oci://registry.corp.internal" + suffix
	netguardtest.StubLookupIP(t, map[string]string{"registry.corp.internal": "10.0.0.5"})

	// Installed (and cached) while internal hosts were allowed.
	cached, err := svc.PluginService.CreateOCIPluginFromReference(&CreateOCIPluginRequest{Name: "cached", OCIReference: loopback, IsActive: true})
	require.NoError(t, err)
	stored := &models.Plugin{Name: "stored", Command: internalName, OCIReference: internalName, HookType: models.HookTypeStudioUI, IsActive: true}
	require.NoError(t, db.Create(stored).Error)
	requestsBefore := reg.Requests("GET", "manifests")

	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "")

	_, err = svc.PluginService.CreatePlugin(&CreatePluginRequest{Name: "new", Command: internalName, HookType: "post_auth"})
	requireInternalHostError(t, err, "registry.corp.internal")
	_, err = svc.PluginService.UpdatePlugin(cached.ID, &UpdatePluginRequest{Command: &internalName})
	requireInternalHostError(t, err, "registry.corp.internal")
	_, err = svc.PluginService.UpdatePlugin(cached.ID, &UpdatePluginRequest{OCIReference: &internalName})
	requireInternalHostError(t, err, "registry.corp.internal")
	_, err = svc.PluginService.CreateOCIPluginFromReference(&CreateOCIPluginRequest{Name: "new", OCIReference: internalName})
	requireInternalHostError(t, err, "registry.corp.internal")
	_, err = svc.PluginService.RefreshOCIPlugin(stored.ID)
	requireInternalHostError(t, err, "registry.corp.internal")
	_, err = svc.PluginManifestService.ParsePluginManifest(stored)
	requireInternalHostError(t, err, "registry.corp.internal")
	_, err = svc.AIStudioPluginManager.LoadPlugin(stored.ID)
	requireInternalHostError(t, err, "registry.corp.internal")
	assert.False(t, svc.AIStudioPluginManager.IsPluginLoaded(stored.ID))

	// The cached plugin on the loopback registry is not loaded either, and
	// its manifest is not read from the cache.
	_, err = svc.AIStudioPluginManager.LoadPlugin(cached.ID)
	requireInternalHostError(t, err, "127.0.0.1")
	_, err = svc.PluginManifestService.ParsePluginManifest(cached)
	requireInternalHostError(t, err, "127.0.0.1")

	ms := NewMarketplaceService(db, nil, svc.PluginService, svc.AIStudioPluginManager, t.TempDir(), "https://marketplace.example.com/index.yaml", 0)
	require.NoError(t, db.Create(&models.MarketplacePlugin{
		PluginID: "com.example.corp", Version: "1.0.0", Name: "Corp",
		OCIRegistry: "registry.corp.internal", OCIRepository: "plugins/demo", OCIDigest: manifest.Digest.String(), PrimaryHook: "post_auth",
	}).Error)
	_, err = ms.InstallFromMarketplace(context.Background(), &marketplace.InstallRequest{PluginID: "com.example.corp", Version: "1.0.0"})
	requireInternalHostError(t, err, "registry.corp.internal")
	var installed int64
	require.NoError(t, db.Model(&models.Plugin{}).Where("name = ?", "Corp").Count(&installed).Error)
	assert.Zero(t, installed, "a refused install creates no plugin")

	assert.Equal(t, requestsBefore, reg.Requests("GET", "manifests"), "no request reached the registry")

	// The documented opt-out allows them again.
	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "true")
	_, err = svc.PluginService.CreatePlugin(&CreatePluginRequest{Name: "new", Command: internalName, HookType: "post_auth"})
	require.NoError(t, err)
	_, err = svc.PluginManifestService.ParsePluginManifest(cached)
	if err != nil {
		assert.NotContains(t, err.Error(), "ALLOW_INTERNAL_NETWORK_ACCESS")
	}
}
