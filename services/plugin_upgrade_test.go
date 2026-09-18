package services

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fakeUpgradeProber answers for the commands it knows, like a registry would.
type fakeUpgradeProber struct {
	manifests map[string]*models.PluginManifest
	schemas   map[string]string
	probed    []string
}

func (f *fakeUpgradeProber) LoadPluginMetadata(_ context.Context, command string) (*PluginMetadata, error) {
	f.probed = append(f.probed, command)
	manifest, ok := f.manifests[command]
	if !ok {
		return nil, fmt.Errorf("artifact not found: %s", command)
	}
	return &PluginMetadata{Command: command, Manifest: manifest, ConfigSchema: f.schemas[command]}, nil
}

// fakeUpgradeRuntime records process swaps and fails to load chosen commands.
type fakeUpgradeRuntime struct {
	db           *gorm.DB
	loaded       map[uint]string // plugin ID -> command it runs
	failCommands map[string]error
	events       []string
}

func (f *fakeUpgradeRuntime) IsPluginLoaded(pluginID uint) bool {
	_, ok := f.loaded[pluginID]
	return ok
}

func (f *fakeUpgradeRuntime) UnloadPlugin(pluginID uint) error {
	f.events = append(f.events, "unload:"+f.loaded[pluginID])
	delete(f.loaded, pluginID)
	return nil
}

func (f *fakeUpgradeRuntime) LoadPlugin(pluginID uint) error {
	// Like the real manager: run whatever command the row holds now.
	var plugin models.Plugin
	if err := f.db.First(&plugin, pluginID).Error; err != nil {
		return err
	}
	if err := f.failCommands[plugin.Command]; err != nil {
		f.events = append(f.events, "load-failed:"+plugin.Command)
		return err
	}
	f.events = append(f.events, "load:"+plugin.Command)
	f.loaded[pluginID] = plugin.Command
	return nil
}

func (f *fakeUpgradeRuntime) GetPluginManifest(pluginID uint) (string, error) {
	return "", errors.New("not a UI plugin")
}

type upgradeFixture struct {
	svc     *PluginUpgradeService
	db      *gorm.DB
	prober  *fakeUpgradeProber
	runtime *fakeUpgradeRuntime
	plugin  *models.Plugin
	synced  []uint
}

const (
	cacheV1Command = "oci://ghcr.io/tyk/cache@" + digestV1
	cacheV2Command = "oci://ghcr.io/tyk/cache@" + digestV2
	cacheV3Command = "oci://ghcr.io/tyk/cache@" + digestV3
)

func cacheManifest(version string, scopes ...string) *models.PluginManifest {
	m := &models.PluginManifest{ID: "com.tyk.cache", Version: version, Name: "Cache"}
	m.Capabilities = &models.PluginCapabilities{Hooks: []string{"post_auth", "on_response"}, PrimaryHook: "post_auth"}
	m.Permissions.Services = scopes
	return m
}

// newUpgradeFixture installs com.tyk.cache 1.0.0 (scope kv.read approved,
// loaded, configured) with 1.2.0 and 1.10.0 available.
func newUpgradeFixture(t *testing.T) *upgradeFixture {
	t.Helper()
	ms, db := setupMarketplaceTest(t)
	seedCachePluginVersions(t, db)

	plugin := &models.Plugin{
		Name:                    "cache",
		Command:                 cacheV1Command,
		OCIReference:            cacheV1Command,
		HookType:                "post_auth",
		HookTypes:               []string{"post_auth", "on_response"},
		IsActive:                true,
		Config:                  map[string]interface{}{"ttl": float64(60), "backend": "redis"},
		Manifest:                map[string]interface{}{"id": "com.tyk.cache", "version": "1.0.0"},
		ServiceScopes:           []string{"kv.read"},
		ServiceAccessAuthorized: true,
	}
	require.NoError(t, db.Create(plugin).Error)
	require.NoError(t, ms.ReconcileInstalledVersions(context.Background()))

	prober := &fakeUpgradeProber{
		manifests: map[string]*models.PluginManifest{
			cacheV2Command: cacheManifest("1.2.0", "kv.read"),
			cacheV3Command: cacheManifest("1.10.0", "kv.read", "llms.proxy"),
		},
		schemas: map[string]string{},
	}
	runtime := &fakeUpgradeRuntime{db: db, loaded: map[uint]string{plugin.ID: cacheV1Command}, failCommands: map[string]error{}}

	f := &upgradeFixture{db: db, prober: prober, runtime: runtime, plugin: plugin}
	plugins := NewPluginService(db)
	plugins.SetPermissionSync(func(p *models.Plugin, removed bool) { f.synced = append(f.synced, p.ID) })
	f.svc = &PluginUpgradeService{db: db, plugins: plugins, marketplace: ms, prober: prober, runtime: runtime}
	return f
}

func (f *upgradeFixture) reload(t *testing.T) *models.Plugin {
	t.Helper()
	var plugin models.Plugin
	require.NoError(t, f.db.First(&plugin, f.plugin.ID).Error)
	return &plugin
}

func TestPluginUpgrade_KeepsRowAndConfig(t *testing.T) {
	f := newUpgradeFixture(t)

	result, err := f.svc.Upgrade(context.Background(), f.plugin.ID, &PluginUpgradeRequest{Version: "1.2.0"})
	require.NoError(t, err)

	assert.Equal(t, "1.0.0", result.FromVersion)
	assert.Equal(t, "1.2.0", result.ToVersion)
	assert.True(t, result.Reloaded)
	assert.True(t, result.AffectsEdges, "post_auth runs on edge gateways")

	plugin := f.reload(t)
	assert.Equal(t, f.plugin.ID, plugin.ID, "same row: everything keyed on the plugin ID survives")
	assert.Equal(t, cacheV2Command, plugin.Command)
	assert.Equal(t, cacheV2Command, plugin.OCIReference)
	assert.Equal(t, map[string]interface{}{"ttl": float64(60), "backend": "redis"}, plugin.Config, "configuration is untouched")
	assert.Equal(t, digestV2, plugin.Checksum, "the checksum is what makes an edge gateway reload the plugin")
	assert.Equal(t, "1.2.0", plugin.Manifest["version"])
	assert.True(t, plugin.ServiceAccessAuthorized, "no unauthorized window for a running plugin")

	assert.Equal(t, []string{"unload:" + cacheV1Command, "load:" + cacheV2Command}, f.runtime.events)
	assert.Contains(t, f.synced, plugin.ID, "new RBAC resources of the new version are registered")

	row := trackingFor(t, f.db, plugin.ID)
	require.NotNil(t, row)
	assert.Equal(t, "1.2.0", row.InstalledVersion)
	assert.True(t, row.UpdateAvailable, "1.10.0 is still ahead")
}

func TestPluginUpgrade_DefaultsToLatest(t *testing.T) {
	f := newUpgradeFixture(t)

	result, err := f.svc.Upgrade(context.Background(), f.plugin.ID, &PluginUpgradeRequest{ApprovedScopes: []string{"llms.proxy"}})
	require.NoError(t, err)
	assert.Equal(t, "1.10.0", result.ToVersion)

	row := trackingFor(t, f.db, f.plugin.ID)
	require.NotNil(t, row)
	assert.False(t, row.UpdateAvailable)
}

func TestPluginUpgrade_NewScopesMustBeApproved(t *testing.T) {
	f := newUpgradeFixture(t)

	_, err := f.svc.Upgrade(context.Background(), f.plugin.ID, &PluginUpgradeRequest{Version: "1.10.0"})
	var scopesErr *UpgradeScopesNotApprovedError
	require.ErrorAs(t, err, &scopesErr)
	assert.Equal(t, []string{"llms.proxy"}, scopesErr.Missing)

	// Approving something else does not count.
	_, err = f.svc.Upgrade(context.Background(), f.plugin.ID, &PluginUpgradeRequest{Version: "1.10.0", ApprovedScopes: []string{"tools.call"}})
	require.ErrorAs(t, err, &scopesErr)

	plugin := f.reload(t)
	assert.Equal(t, cacheV1Command, plugin.Command, "nothing is written when the gate fails")
	assert.Empty(t, f.runtime.events)

	_, err = f.svc.Upgrade(context.Background(), f.plugin.ID, &PluginUpgradeRequest{Version: "1.10.0", ApprovedScopes: []string{"llms.proxy"}})
	require.NoError(t, err)
	plugin = f.reload(t)
	assert.ElementsMatch(t, []string{"kv.read", "llms.proxy"}, plugin.ServiceScopes)
	assert.True(t, plugin.ServiceAccessAuthorized)
}

func TestPluginUpgrade_UnapprovedInstallNeedsAllScopesApproved(t *testing.T) {
	f := newUpgradeFixture(t)
	require.NoError(t, f.db.Model(f.plugin).Select("ServiceAccessAuthorized").Updates(&models.Plugin{ServiceAccessAuthorized: false}).Error)

	_, err := f.svc.Upgrade(context.Background(), f.plugin.ID, &PluginUpgradeRequest{Version: "1.2.0"})
	var scopesErr *UpgradeScopesNotApprovedError
	require.ErrorAs(t, err, &scopesErr, "scopes that were never approved are not carried over")
	assert.Equal(t, []string{"kv.read"}, scopesErr.Missing)
}

func TestPluginUpgrade_RemovedScopesAreDropped(t *testing.T) {
	f := newUpgradeFixture(t)
	f.prober.manifests[cacheV2Command] = cacheManifest("1.2.0")

	preview, err := f.svc.Preview(context.Background(), f.plugin.ID, "1.2.0")
	require.NoError(t, err)
	assert.Equal(t, []string{"kv.read"}, preview.Scopes.Removed)
	assert.Empty(t, preview.Scopes.Added)

	_, err = f.svc.Upgrade(context.Background(), f.plugin.ID, &PluginUpgradeRequest{Version: "1.2.0"})
	require.NoError(t, err)
	assert.Empty(t, f.reload(t).ServiceScopes, "a grant the new version does not ask for does not linger")
}

func TestPluginUpgrade_RollsBackWhenNewVersionFailsToStart(t *testing.T) {
	f := newUpgradeFixture(t)
	f.runtime.failCommands[cacheV2Command] = errors.New("plugin initialization failed: unknown config key")

	_, err := f.svc.Upgrade(context.Background(), f.plugin.ID, &PluginUpgradeRequest{Version: "1.2.0"})
	var rolledBack *UpgradeRolledBackError
	require.ErrorAs(t, err, &rolledBack)
	assert.NoError(t, rolledBack.RollbackErr)
	assert.Contains(t, err.Error(), "unknown config key")

	plugin := f.reload(t)
	assert.Equal(t, cacheV1Command, plugin.Command)
	assert.Equal(t, "", plugin.Checksum)
	assert.Equal(t, "1.0.0", plugin.Manifest["version"])
	assert.Equal(t, []string{"kv.read"}, plugin.ServiceScopes)
	assert.Equal(t, cacheV1Command, f.runtime.loaded[plugin.ID], "the previous version is running again")
	assert.Equal(t, []string{"unload:" + cacheV1Command, "load-failed:" + cacheV2Command, "load:" + cacheV1Command}, f.runtime.events)

	row := trackingFor(t, f.db, plugin.ID)
	require.NotNil(t, row)
	assert.Equal(t, "1.0.0", row.InstalledVersion)
}

func TestPluginUpgrade_DowngradeNeedsExplicitFlag(t *testing.T) {
	f := newUpgradeFixture(t)
	_, err := f.svc.Upgrade(context.Background(), f.plugin.ID, &PluginUpgradeRequest{Version: "1.2.0"})
	require.NoError(t, err)

	f.prober.manifests[cacheV1Command] = cacheManifest("1.0.0", "kv.read")

	_, err = f.svc.Upgrade(context.Background(), f.plugin.ID, &PluginUpgradeRequest{Version: "1.0.0"})
	assert.ErrorIs(t, err, ErrUpgradeDowngradeNotAllowed)

	preview, err := f.svc.Preview(context.Background(), f.plugin.ID, "1.0.0")
	require.NoError(t, err)
	assert.True(t, preview.IsDowngrade)

	result, err := f.svc.Upgrade(context.Background(), f.plugin.ID, &PluginUpgradeRequest{Version: "1.0.0", AllowDowngrade: true})
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", result.ToVersion)
}

func TestPluginUpgrade_Refusals(t *testing.T) {
	t.Run("a different plugin behind the target reference", func(t *testing.T) {
		f := newUpgradeFixture(t)
		other := cacheManifest("1.2.0", "kv.read")
		other.ID = "com.evil.cache"
		f.prober.manifests[cacheV2Command] = other

		_, err := f.svc.Upgrade(context.Background(), f.plugin.ID, &PluginUpgradeRequest{Version: "1.2.0"})
		assert.ErrorIs(t, err, ErrUpgradeManifestMismatch)
		assert.Equal(t, cacheV1Command, f.reload(t).Command)
	})

	t.Run("an artifact that cannot be pulled changes nothing", func(t *testing.T) {
		f := newUpgradeFixture(t)
		delete(f.prober.manifests, cacheV2Command)

		_, err := f.svc.Upgrade(context.Background(), f.plugin.ID, &PluginUpgradeRequest{Version: "1.2.0"})
		assert.ErrorIs(t, err, ErrUpgradeTargetUnavailable)
		assert.Equal(t, cacheV1Command, f.reload(t).Command)
		assert.Empty(t, f.runtime.events)
	})

	t.Run("unknown plugin", func(t *testing.T) {
		f := newUpgradeFixture(t)
		_, err := f.svc.Upgrade(context.Background(), 999999, &PluginUpgradeRequest{})
		assert.ErrorIs(t, err, ErrUpgradePluginNotFound)
	})

	t.Run("unknown version", func(t *testing.T) {
		f := newUpgradeFixture(t)
		_, err := f.svc.Upgrade(context.Background(), f.plugin.ID, &PluginUpgradeRequest{Version: "9.9.9"})
		assert.ErrorIs(t, err, ErrUpgradeVersionNotFound)
	})

	t.Run("already on the version", func(t *testing.T) {
		f := newUpgradeFixture(t)
		_, err := f.svc.Upgrade(context.Background(), f.plugin.ID, &PluginUpgradeRequest{Version: "1.0.0"})
		assert.ErrorIs(t, err, ErrUpgradeSameVersion)
		assert.Empty(t, f.prober.probed, "nothing is pulled for a no-op")
	})

	t.Run("a plugin that is not from the marketplace", func(t *testing.T) {
		f := newUpgradeFixture(t)
		local := seedInstalledPlugin(t, f.db, "local", "/usr/local/bin/plugin", nil)
		_, err := f.svc.Upgrade(context.Background(), local.ID, &PluginUpgradeRequest{})
		assert.ErrorIs(t, err, ErrUpgradeNotFromMarketplace)
	})

	t.Run("a version from another repository under the same marketplace ID", func(t *testing.T) {
		f := newUpgradeFixture(t)
		seedMarketplaceVersion(t, f.db, models.MarketplacePlugin{
			PluginID: "com.tyk.cache", Version: "5.0.0", OCIRegistry: "ghcr.io", OCIRepository: "someone-else/cache",
			OCIDigest: digestXX, SyncedFromURL: "https://other-marketplace.example.com/index.yaml",
		})
		_, err := f.svc.Upgrade(context.Background(), f.plugin.ID, &PluginUpgradeRequest{Version: "5.0.0"})
		assert.ErrorIs(t, err, ErrUpgradeVersionNotFound, "a second source cannot offer its artifact as an upgrade")

		require.NoError(t, f.svc.marketplace.ReconcileInstalledVersions(context.Background()))
		assert.Equal(t, "1.10.0", trackingFor(t, f.db, f.plugin.ID).AvailableVersion)
	})
}

func TestPluginUpgrade_PreviewReportsWithoutWriting(t *testing.T) {
	f := newUpgradeFixture(t)
	f.prober.schemas[cacheV3Command] = `{"type":"object","required":["backend","region"],"properties":{"ttl":{"type":"integer"},"backend":{"type":"string"},"region":{"type":"string"}}}`

	preview, err := f.svc.Preview(context.Background(), f.plugin.ID, "")
	require.NoError(t, err)

	assert.Equal(t, "1.0.0", preview.InstalledVersion)
	assert.Equal(t, "1.10.0", preview.TargetVersion)
	assert.Equal(t, "1.10.0", preview.LatestVersion)
	assert.False(t, preview.IsDowngrade)
	assert.Equal(t, []string{"llms.proxy"}, preview.Scopes.Added)
	assert.Empty(t, preview.Scopes.Removed)
	assert.True(t, preview.AffectsEdges)
	require.Len(t, preview.ConfigIssues, 1, "the existing config lacks the newly required key")
	assert.Contains(t, preview.ConfigIssues[0], "region")

	versions := make([]string, 0, len(preview.Versions))
	for _, v := range preview.Versions {
		versions = append(versions, v.Version)
	}
	assert.Equal(t, []string{"1.10.0", "1.2.0", "1.0.0"}, versions, "newest first")

	assert.Equal(t, cacheV1Command, f.reload(t).Command)
	assert.Empty(t, f.runtime.events)
}

func TestPluginUpgrade_CustomisedHooksAreKeptWhereStillSupported(t *testing.T) {
	f := newUpgradeFixture(t)
	require.NoError(t, f.db.Model(f.plugin).Select("HookTypes", "HookTypesCustomized").
		Updates(&models.Plugin{HookTypes: []string{"on_response"}, HookTypesCustomized: true}).Error)
	require.NoError(t, f.db.Model(f.plugin).Update("hook_type", "on_response").Error)

	_, err := f.svc.Upgrade(context.Background(), f.plugin.ID, &PluginUpgradeRequest{Version: "1.2.0"})
	require.NoError(t, err)

	plugin := f.reload(t)
	assert.Equal(t, []string{"on_response"}, plugin.HookTypes)
	assert.Equal(t, "on_response", plugin.HookType)
	assert.True(t, plugin.HookTypesCustomized)
}

func TestPluginUpgrade_InactivePluginIsNotStarted(t *testing.T) {
	f := newUpgradeFixture(t)
	require.NoError(t, f.db.Model(f.plugin).Select("IsActive").Updates(&models.Plugin{IsActive: false}).Error)
	delete(f.runtime.loaded, f.plugin.ID)

	result, err := f.svc.Upgrade(context.Background(), f.plugin.ID, &PluginUpgradeRequest{Version: "1.2.0"})
	require.NoError(t, err)
	assert.False(t, result.Reloaded)
	assert.Empty(t, f.runtime.events)
	assert.Equal(t, cacheV2Command, f.reload(t).Command)
}

func TestPluginUpgrade_PreviewWhenAlreadyOnTarget(t *testing.T) {
	f := newUpgradeFixture(t)
	_, err := f.svc.Upgrade(context.Background(), f.plugin.ID, &PluginUpgradeRequest{ApprovedScopes: []string{"llms.proxy"}})
	require.NoError(t, err)
	f.prober.probed = nil

	// Up to date: the preview still answers so another version can be picked.
	preview, err := f.svc.Preview(context.Background(), f.plugin.ID, "")
	require.NoError(t, err)
	assert.True(t, preview.SameVersion)
	assert.Equal(t, "1.10.0", preview.TargetVersion)
	assert.Empty(t, f.prober.probed, "nothing is pulled when there is nothing to move to")
	require.Len(t, preview.Versions, 3)
	assert.True(t, preview.Versions[0].Installed)
	assert.False(t, preview.Versions[1].Installed)
}
