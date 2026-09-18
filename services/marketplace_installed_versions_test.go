package services

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	digestV1 = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	digestV2 = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	digestV3 = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
	digestXX = "sha256:9999999999999999999999999999999999999999999999999999999999999999"
)

func seedMarketplaceVersion(t *testing.T, db *gorm.DB, mp models.MarketplacePlugin) *models.MarketplacePlugin {
	t.Helper()
	if mp.Name == "" {
		mp.Name = mp.PluginID
	}
	if mp.SyncedFromURL == "" {
		mp.SyncedFromURL = "https://marketplace.example.com/index.yaml"
	}
	require.NoError(t, db.Create(&mp).Error)
	return &mp
}

func seedCachePluginVersions(t *testing.T, db *gorm.DB) {
	t.Helper()
	seedMarketplaceVersion(t, db, models.MarketplacePlugin{PluginID: "com.tyk.cache", Version: "1.0.0", OCIRegistry: "ghcr.io", OCIRepository: "tyk/cache", OCITag: "1.0", OCIDigest: digestV1})
	seedMarketplaceVersion(t, db, models.MarketplacePlugin{PluginID: "com.tyk.cache", Version: "1.2.0", OCIRegistry: "ghcr.io", OCIRepository: "tyk/cache", OCITag: "1.0", OCIDigest: digestV2})
	seedMarketplaceVersion(t, db, models.MarketplacePlugin{PluginID: "com.tyk.cache", Version: "1.10.0", OCIRegistry: "ghcr.io", OCIRepository: "tyk/cache", OCITag: "1.10", OCIDigest: digestV3})
}

func seedInstalledPlugin(t *testing.T, db *gorm.DB, name, command string, manifest map[string]interface{}) *models.Plugin {
	t.Helper()
	plugin := &models.Plugin{Name: name, Command: command, HookType: "post_auth", IsActive: true, Manifest: manifest}
	require.NoError(t, db.Create(plugin).Error)
	return plugin
}

func trackingFor(t *testing.T, db *gorm.DB, pluginID uint) *models.InstalledPluginVersion {
	t.Helper()
	rows, err := models.GetInstalledPluginVersions(db, []uint{pluginID})
	require.NoError(t, err)
	return rows[pluginID]
}

func TestReconcileInstalledVersions(t *testing.T) {
	t.Run("links an existing install by digest and compares by semver", func(t *testing.T) {
		ms, db := setupMarketplaceTest(t)
		seedCachePluginVersions(t, db)
		plugin := seedInstalledPlugin(t, db, "cache", "oci://ghcr.io/tyk/cache@"+digestV2, nil)

		require.NoError(t, ms.ReconcileInstalledVersions(context.Background()))

		row := trackingFor(t, db, plugin.ID)
		require.NotNil(t, row, "an install made without any bookkeeping is picked up")
		assert.Equal(t, "com.tyk.cache", row.MarketplacePluginID)
		assert.Equal(t, "1.2.0", row.InstalledVersion)
		assert.Equal(t, "1.10.0", row.AvailableVersion, "1.10.0 is newer than 1.2.0 by semver, not by string order")
		assert.True(t, row.UpdateAvailable)
	})

	t.Run("no update when already on the latest version", func(t *testing.T) {
		ms, db := setupMarketplaceTest(t)
		seedCachePluginVersions(t, db)
		plugin := seedInstalledPlugin(t, db, "cache", "oci://ghcr.io/tyk/cache@"+digestV3, nil)

		require.NoError(t, ms.ReconcileInstalledVersions(context.Background()))

		row := trackingFor(t, db, plugin.ID)
		require.NotNil(t, row)
		assert.Equal(t, "1.10.0", row.InstalledVersion)
		assert.False(t, row.UpdateAvailable)
	})

	t.Run("a lower marketplace version is not an update", func(t *testing.T) {
		ms, db := setupMarketplaceTest(t)
		seedMarketplaceVersion(t, db, models.MarketplacePlugin{PluginID: "com.tyk.cache", Version: "1.0.0", OCIRegistry: "ghcr.io", OCIRepository: "tyk/cache", OCIDigest: digestV1})
		// Installed version was pulled from the index; the plugin still reports it.
		plugin := seedInstalledPlugin(t, db, "cache", "oci://ghcr.io/tyk/cache@"+digestXX, map[string]interface{}{"id": "com.tyk.cache", "version": "2.0.0"})

		require.NoError(t, ms.ReconcileInstalledVersions(context.Background()))

		row := trackingFor(t, db, plugin.ID)
		require.NotNil(t, row)
		assert.Equal(t, "2.0.0", row.InstalledVersion, "falls back to the manifest version when the digest is not in the index")
		assert.Equal(t, "1.0.0", row.AvailableVersion)
		assert.False(t, row.UpdateAvailable)
	})

	t.Run("a shared tag does not identify a version", func(t *testing.T) {
		ms, db := setupMarketplaceTest(t)
		seedCachePluginVersions(t, db)
		shared := seedInstalledPlugin(t, db, "cache-shared-tag", "oci://ghcr.io/tyk/cache:1.0", nil)
		unique := seedInstalledPlugin(t, db, "cache-unique-tag", "oci://ghcr.io/tyk/cache:1.10", nil)

		require.NoError(t, ms.ReconcileInstalledVersions(context.Background()))

		sharedRow := trackingFor(t, db, shared.ID)
		require.NotNil(t, sharedRow)
		assert.Equal(t, "", sharedRow.InstalledVersion, "two versions carry tag 1.0")
		assert.False(t, sharedRow.UpdateAvailable, "an unknown version is not reported as outdated")

		uniqueRow := trackingFor(t, db, unique.ID)
		require.NotNil(t, uniqueRow)
		assert.Equal(t, "1.10.0", uniqueRow.InstalledVersion)
	})

	t.Run("falls back to the registered manifest version", func(t *testing.T) {
		ms, db := setupMarketplaceTest(t)
		seedCachePluginVersions(t, db)
		plugin := seedInstalledPlugin(t, db, "cache", "oci://ghcr.io/tyk/cache@"+digestXX, nil)
		require.NoError(t, db.Create(&models.RegisteredPlugin{PluginID: plugin.ID, ManifestVersion: "1.1.0"}).Error)

		require.NoError(t, ms.ReconcileInstalledVersions(context.Background()))

		row := trackingFor(t, db, plugin.ID)
		require.NotNil(t, row)
		assert.Equal(t, "1.1.0", row.InstalledVersion)
		assert.True(t, row.UpdateAvailable)
	})

	t.Run("plugins outside the marketplace are not tracked", func(t *testing.T) {
		ms, db := setupMarketplaceTest(t)
		seedCachePluginVersions(t, db)
		local := seedInstalledPlugin(t, db, "local", "/usr/local/bin/plugin", nil)
		otherRepo := seedInstalledPlugin(t, db, "other", "oci://ghcr.io/someone-else/cache@"+digestV1, nil)
		otherRegistry := seedInstalledPlugin(t, db, "mirror", "oci://registry.internal/tyk/cache@"+digestV1, nil)

		require.NoError(t, ms.ReconcileInstalledVersions(context.Background()))

		assert.Nil(t, trackingFor(t, db, local.ID))
		assert.Nil(t, trackingFor(t, db, otherRepo.ID), "same digest, different repository")
		assert.Nil(t, trackingFor(t, db, otherRegistry.ID), "same repository path, different registry")
	})

	t.Run("deprecated versions are not offered", func(t *testing.T) {
		ms, db := setupMarketplaceTest(t)
		seedMarketplaceVersion(t, db, models.MarketplacePlugin{PluginID: "com.tyk.cache", Version: "1.0.0", OCIRegistry: "ghcr.io", OCIRepository: "tyk/cache", OCIDigest: digestV1})
		seedMarketplaceVersion(t, db, models.MarketplacePlugin{PluginID: "com.tyk.cache", Version: "2.0.0", OCIRegistry: "ghcr.io", OCIRepository: "tyk/cache", OCIDigest: digestV2, Deprecated: true})
		plugin := seedInstalledPlugin(t, db, "cache", "oci://ghcr.io/tyk/cache@"+digestV1, nil)

		require.NoError(t, ms.ReconcileInstalledVersions(context.Background()))

		row := trackingFor(t, db, plugin.ID)
		require.NotNil(t, row)
		assert.Equal(t, "1.0.0", row.AvailableVersion)
		assert.False(t, row.UpdateAvailable)
	})

	t.Run("a cleared update is written back and stale rows are removed", func(t *testing.T) {
		ms, db := setupMarketplaceTest(t)
		seedCachePluginVersions(t, db)
		plugin := seedInstalledPlugin(t, db, "cache", "oci://ghcr.io/tyk/cache@"+digestV1, nil)
		gone := seedInstalledPlugin(t, db, "gone", "oci://ghcr.io/tyk/cache@"+digestV1, nil)

		require.NoError(t, ms.ReconcileInstalledVersions(context.Background()))
		require.True(t, trackingFor(t, db, plugin.ID).UpdateAvailable)

		// The plugin moves to the latest version; the other one is deleted.
		require.NoError(t, db.Model(plugin).Update("command", "oci://ghcr.io/tyk/cache@"+digestV3).Error)
		require.NoError(t, db.Delete(gone).Error)

		require.NoError(t, ms.ReconcileInstalledVersions(context.Background()))

		row := trackingFor(t, db, plugin.ID)
		require.NotNil(t, row)
		assert.Equal(t, "1.10.0", row.InstalledVersion)
		assert.False(t, row.UpdateAvailable)
		assert.Nil(t, trackingFor(t, db, gone.ID))

		// The unique index on plugin_id must not be blocked by a soft-deleted row.
		var leftovers int64
		require.NoError(t, db.Unscoped().Model(&models.InstalledPluginVersion{}).Where("plugin_id = ?", gone.ID).Count(&leftovers).Error)
		assert.Equal(t, int64(0), leftovers)
	})

	t.Run("a scoped reconcile leaves other plugins alone", func(t *testing.T) {
		ms, db := setupMarketplaceTest(t)
		seedCachePluginVersions(t, db)
		first := seedInstalledPlugin(t, db, "first", "oci://ghcr.io/tyk/cache@"+digestV1, nil)
		second := seedInstalledPlugin(t, db, "second", "oci://ghcr.io/tyk/cache@"+digestV2, nil)

		require.NoError(t, ms.ReconcileInstalledVersions(context.Background(), first.ID))

		assert.NotNil(t, trackingFor(t, db, first.ID))
		assert.Nil(t, trackingFor(t, db, second.ID))

		count, err := models.CountPluginUpdatesAvailable(db)
		require.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})
}

func TestIsNewerVersion(t *testing.T) {
	assert.True(t, models.IsNewerVersion("1.10.0", "1.2.0"))
	assert.True(t, models.IsNewerVersion("v2.0.0", "1.9.9"))
	assert.False(t, models.IsNewerVersion("1.2.0", "1.2.0"))
	assert.False(t, models.IsNewerVersion("1.0.0", "2.0.0"))
	assert.False(t, models.IsNewerVersion("2.0.0", ""), "an unknown installed version cannot be compared")
	assert.False(t, models.IsNewerVersion("not-a-version", "1.0.0"))
}

func TestMarketplacePluginOCIReference(t *testing.T) {
	assert.Equal(t, "oci://ghcr.io/tyk/cache@"+digestV1,
		(&models.MarketplacePlugin{OCIRegistry: "ghcr.io", OCIRepository: "tyk/cache", OCITag: "1.0", OCIDigest: digestV1}).OCIReference(),
		"pinned to the digest when the index has one")
	assert.Equal(t, "oci://ghcr.io/tyk/cache:1.0",
		(&models.MarketplacePlugin{OCIRegistry: "ghcr.io", OCIRepository: "tyk/cache", OCITag: "1.0"}).OCIReference())
	assert.Equal(t, "", (&models.MarketplacePlugin{OCIRegistry: "ghcr.io", OCIRepository: "tyk/cache"}).OCIReference())
}
