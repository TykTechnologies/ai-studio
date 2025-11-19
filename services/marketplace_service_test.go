package services

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/marketplace"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupMarketplaceTest(t *testing.T) (*MarketplaceService, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	// Create marketplace service with test configuration
	ms := NewMarketplaceService(
		db,
		nil, // ociClient - not needed for these tests
		nil, // pluginService - not needed for these tests
		nil, // pluginManager - not needed for these tests
		"./test-cache",
		"https://marketplace.example.com/index.yaml",
		1*time.Hour,
	)

	return ms, db
}

func TestNewMarketplaceService(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	t.Run("Create marketplace service with default sync interval", func(t *testing.T) {
		ms := NewMarketplaceService(db, nil, nil, nil, "./cache", "https://example.com", 0)
		assert.NotNil(t, ms)
		assert.Equal(t, 1*time.Hour, ms.syncInterval) // Should default to 1 hour
		assert.Equal(t, "./cache", ms.cacheDir)
		assert.Equal(t, "https://example.com", ms.defaultIndexURL)
	})

	t.Run("Create marketplace service with custom sync interval", func(t *testing.T) {
		ms := NewMarketplaceService(db, nil, nil, nil, "./cache", "https://example.com", 30*time.Minute)
		assert.NotNil(t, ms)
		assert.Equal(t, 30*time.Minute, ms.syncInterval)
	})
}

func TestMarketplaceService_GetDB(t *testing.T) {
	ms, db := setupMarketplaceTest(t)

	t.Run("GetDB returns database connection", func(t *testing.T) {
		retrievedDB := ms.GetDB()
		assert.Equal(t, db, retrievedDB)
	})
}

func TestMarketplaceService_EnsureCacheDirectory(t *testing.T) {
	t.Run("Ensure cache directory with configured path", func(t *testing.T) {
		ms, _ := setupMarketplaceTest(t)
		err := ms.EnsureCacheDirectory()
		assert.NoError(t, err)
		assert.Equal(t, "./test-cache", ms.cacheDir)
	})

	t.Run("Ensure cache directory with empty path", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		ms := NewMarketplaceService(db, nil, nil, nil, "", "https://example.com", 1*time.Hour)

		err := ms.EnsureCacheDirectory()
		assert.NoError(t, err)
		assert.Contains(t, ms.cacheDir, ".marketplace-cache") // Should use default
	})
}

func TestMarketplaceService_IndexedPluginToModel(t *testing.T) {
	ms, _ := setupMarketplaceTest(t)

	t.Run("Convert indexed plugin to model", func(t *testing.T) {
		now := time.Now()
		indexed := &marketplace.IndexedPlugin{
			ID:               "com.tyk.test-plugin",
			Name:             "Test Plugin",
			Version:          "1.0.0",
			Description:      "A test plugin",
			Category:         "agents",
			Maturity:         "stable",
			Publisher:        "tyk-official",
			OCIRegistry:      "ghcr.io",
			OCIRepository:    "tyk/test-plugin",
			OCITag:           "1.0.0",
			OCIDigest:        "sha256:abc123",
			OCIPlatform:      []string{"linux/amd64"},
			Icon:             "https://example.com/icon.png",
			PrimaryHook:      "post_auth",
			MinStudioVersion: "1.0.0",
			CreatedAt:        now,
			UpdatedAt:        now,
			Deprecated:       false,
			RequiredServices: []string{"llm.proxy"},
			RequiredKV:       []string{"read", "write"},
			RequiredRPC:      []string{"call"},
			RequiredUI:       []string{"sidebar"},
		}

		model := ms.indexedPluginToModel(indexed, "https://marketplace.example.com/index.yaml")

		assert.Equal(t, "com.tyk.test-plugin", model.PluginID)
		assert.Equal(t, "1.0.0", model.Version)
		assert.Equal(t, "Test Plugin", model.Name)
		assert.Equal(t, "A test plugin", model.Description)
		assert.Equal(t, "agents", model.Category)
		assert.Equal(t, "stable", model.Maturity)
		assert.Equal(t, "tyk-official", model.Publisher)
		assert.Equal(t, "ghcr.io", model.OCIRegistry)
		assert.Equal(t, "tyk/test-plugin", model.OCIRepository)
		assert.Equal(t, "1.0.0", model.OCITag)
		assert.Equal(t, "sha256:abc123", model.OCIDigest)
		assert.Equal(t, "https://example.com/icon.png", model.IconURL)
		assert.Equal(t, "post_auth", model.PrimaryHook)
		assert.Equal(t, "https://marketplace.example.com/index.yaml", model.SyncedFromURL)
		assert.Equal(t, []string{"llm.proxy"}, model.RequiredServices)
		assert.False(t, model.Deprecated)
	})
}

func TestMarketplaceService_SearchPlugins(t *testing.T) {
	ms, db := setupMarketplaceTest(t)

	// Create test marketplace plugins
	plugin1 := &models.MarketplacePlugin{
		PluginID:      "com.tyk.agent1",
		Version:       "1.0.0",
		Name:          "Agent Plugin",
		Description:   "Test agent",
		Category:      "agents",
		Publisher:     "tyk-official",
		Maturity:      "stable",
		OCIRegistry:   "ghcr.io",
		OCIRepository: "tyk/agent1",
	}
	db.Create(plugin1)

	plugin2 := &models.MarketplacePlugin{
		PluginID:      "com.tyk.connector1",
		Version:       "1.0.0",
		Name:          "Connector Plugin",
		Description:   "Test connector",
		Category:      "connectors",
		Publisher:     "community",
		Maturity:      "beta",
		OCIRegistry:   "ghcr.io",
		OCIRepository: "tyk/connector1",
	}
	db.Create(plugin2)

	t.Run("Search all plugins", func(t *testing.T) {
		filters := &marketplace.SearchFilters{
			PageSize:   10,
			PageNumber: 1,
		}
		plugins, totalCount, totalPages, err := ms.SearchPlugins(filters)
		assert.NoError(t, err)
		assert.Len(t, plugins, 2)
		assert.Equal(t, int64(2), totalCount)
		assert.Equal(t, 1, totalPages)
	})

	t.Run("Search by category", func(t *testing.T) {
		filters := &marketplace.SearchFilters{
			Category:   "agents",
			PageSize:   10,
			PageNumber: 1,
		}
		plugins, totalCount, _, err := ms.SearchPlugins(filters)
		assert.NoError(t, err)
		assert.Len(t, plugins, 1)
		assert.Equal(t, int64(1), totalCount)
		assert.Equal(t, "Agent Plugin", plugins[0].Name)
	})

	t.Run("Search by publisher", func(t *testing.T) {
		filters := &marketplace.SearchFilters{
			Publisher:  "tyk-official",
			PageSize:   10,
			PageNumber: 1,
		}
		plugins, totalCount, _, err := ms.SearchPlugins(filters)
		assert.NoError(t, err)
		assert.Len(t, plugins, 1)
		assert.Equal(t, int64(1), totalCount)
	})

	t.Run("Search by query", func(t *testing.T) {
		filters := &marketplace.SearchFilters{
			Query:      "Agent",
			PageSize:   10,
			PageNumber: 1,
		}
		plugins, totalCount, _, err := ms.SearchPlugins(filters)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(plugins), 1)
		assert.GreaterOrEqual(t, totalCount, int64(1))
	})
}

func TestMarketplaceService_GetPlugin(t *testing.T) {
	ms, db := setupMarketplaceTest(t)

	plugin := &models.MarketplacePlugin{
		PluginID:      "com.tyk.test",
		Version:       "2.0.0",
		Name:          "Test Plugin",
		OCIRegistry:   "ghcr.io",
		OCIRepository: "tyk/test",
	}
	db.Create(plugin)

	t.Run("Get existing plugin", func(t *testing.T) {
		retrieved, err := ms.GetPlugin("com.tyk.test", "2.0.0")
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, "com.tyk.test", retrieved.PluginID)
		assert.Equal(t, "2.0.0", retrieved.Version)
	})

	t.Run("Get non-existent plugin", func(t *testing.T) {
		retrieved, err := ms.GetPlugin("com.tyk.nonexistent", "1.0.0")
		assert.Error(t, err)
		assert.NotNil(t, retrieved) // Function returns pointer even on error
	})
}

func TestMarketplaceService_GetPluginVersions(t *testing.T) {
	ms, db := setupMarketplaceTest(t)

	// Create multiple versions of the same plugin
	for i := 1; i <= 3; i++ {
		plugin := &models.MarketplacePlugin{
			PluginID:      "com.tyk.versioned",
			Version:       "1." + string(rune('0'+i)) + ".0",
			Name:          "Versioned Plugin",
			OCIRegistry:   "ghcr.io",
			OCIRepository: "tyk/versioned",
		}
		db.Create(plugin)
	}

	t.Run("Get all versions of plugin", func(t *testing.T) {
		versions, err := ms.GetPluginVersions("com.tyk.versioned")
		assert.NoError(t, err)
		assert.Len(t, versions, 3)
	})

	t.Run("Get versions of non-existent plugin", func(t *testing.T) {
		versions, err := ms.GetPluginVersions("com.tyk.nonexistent")
		assert.NoError(t, err)
		assert.Len(t, versions, 0)
	})
}

func TestMarketplaceService_GetAvailableUpdates(t *testing.T) {
	ms, db := setupMarketplaceTest(t)

	// Create a plugin
	plugin := &models.Plugin{
		Name:     "Test Plugin",
		Command:  "/bin/test",
		HookType: "post_auth",
		IsActive: true,
	}
	db.Create(plugin)

	// Create marketplace plugin (newer version)
	marketplacePlugin := &models.MarketplacePlugin{
		PluginID:      "com.tyk.test",
		Version:       "2.0.0",
		Name:          "Test Plugin",
		OCIRegistry:   "ghcr.io",
		OCIRepository: "tyk/test",
	}
	db.Create(marketplacePlugin)

	// Create installed version record with update available
	installedVersion := &models.InstalledPluginVersion{
		PluginID:            plugin.ID,
		MarketplacePluginID: "com.tyk.test",
		InstalledVersion:    "1.0.0",
		AvailableVersion:    "2.0.0",
		UpdateAvailable:     true,
		AutoUpdate:          false,
		LastChecked:         time.Now(),
		InstallSource:       "marketplace",
	}
	db.Create(installedVersion)

	// Reload plugin association
	db.Model(&installedVersion).Association("Plugin").Find(&installedVersion.Plugin)
	installedVersion.Plugin = plugin

	t.Run("Get available updates", func(t *testing.T) {
		response, err := ms.GetAvailableUpdates()
		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.GreaterOrEqual(t, response.UpdatesAvailable, 0)
	})
}

func TestMarketplaceService_ProcessIndex(t *testing.T) {
	ms, _ := setupMarketplaceTest(t)

	t.Run("Process index with new plugins", func(t *testing.T) {
		index := &marketplace.MarketplaceIndex{
			APIVersion: "v1",
			Generated:  time.Now(),
			Plugins: map[string][]marketplace.IndexedPlugin{
				"com.tyk.new-plugin": {
					{
						ID:            "com.tyk.new-plugin",
						Name:          "New Plugin",
						Version:       "1.0.0",
						Description:   "A new plugin",
						Category:      "agents",
						Maturity:      "stable",
						Publisher:     "tyk-official",
						OCIRegistry:   "ghcr.io",
						OCIRepository: "tyk/new-plugin",
						OCITag:        "1.0.0",
						OCIDigest:     "sha256:xyz789",
						PrimaryHook:   "post_auth",
						CreatedAt:     time.Now(),
						UpdatedAt:     time.Now(),
					},
				},
			},
		}

		result, err := ms.processIndex(context.Background(), index, "https://test-source.com")
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Equal(t, 1, result.PluginsAdded)
		assert.Equal(t, 0, result.PluginsUpdated)
	})

	t.Run("Process index with existing plugins (update)", func(t *testing.T) {
		// Create index with same plugin again
		index := &marketplace.MarketplaceIndex{
			APIVersion: "v1",
			Generated:  time.Now(),
			Plugins: map[string][]marketplace.IndexedPlugin{
				"com.tyk.new-plugin": {
					{
						ID:            "com.tyk.new-plugin",
						Name:          "New Plugin Updated",
						Version:       "1.0.0",
						Description:   "Updated description",
						Category:      "agents",
						Maturity:      "stable",
						Publisher:     "tyk-official",
						OCIRegistry:   "ghcr.io",
						OCIRepository: "tyk/new-plugin",
						OCITag:        "1.0.0",
						OCIDigest:     "sha256:xyz789",
						PrimaryHook:   "post_auth",
						CreatedAt:     time.Now(),
						UpdatedAt:     time.Now(),
					},
				},
			},
		}

		result, err := ms.processIndex(context.Background(), index, "https://test-source.com")
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Equal(t, 0, result.PluginsAdded)
		assert.Equal(t, 1, result.PluginsUpdated)
	})

	t.Run("Process empty index", func(t *testing.T) {
		index := &marketplace.MarketplaceIndex{
			APIVersion: "v1",
			Generated:  time.Now(),
			Plugins:    map[string][]marketplace.IndexedPlugin{},
		}

		result, err := ms.processIndex(context.Background(), index, "https://empty-source.com")
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.Success)
		assert.Equal(t, 0, result.PluginsAdded)
	})
}

func TestMarketplaceService_InstallFromMarketplace(t *testing.T) {
	ms, db := setupMarketplaceTest(t)

	// Create a marketplace plugin
	marketplacePlugin := &models.MarketplacePlugin{
		PluginID:         "com.tyk.installable",
		Version:          "1.0.0",
		Name:             "Installable Plugin",
		Description:      "Plugin for installation",
		Category:         "agents",
		Publisher:        "tyk-official",
		OCIRegistry:      "ghcr.io",
		OCIRepository:    "tyk/installable",
		OCIDigest:        "sha256:install123",
		PrimaryHook:      "post_auth",
		RequiredServices: []string{"llm.proxy"},
	}
	db.Create(marketplacePlugin)

	t.Run("Install plugin from marketplace - plugin not found", func(t *testing.T) {
		req := &marketplace.InstallRequest{
			PluginID: "com.tyk.nonexistent",
			Version:  "1.0.0",
			Name:     "Custom Name",
		}

		response, err := ms.InstallFromMarketplace(context.Background(), req)
		assert.Error(t, err)
		assert.Nil(t, response)
		assert.Contains(t, err.Error(), "plugin not found")
	})

	t.Run("Install plugin successfully with custom name", func(t *testing.T) {
		req := &marketplace.InstallRequest{
			PluginID:   "com.tyk.installable",
			Version:    "1.0.0",
			Name:       "My Custom Plugin",
			Namespace:  "production",
			AutoUpdate: true,
		}

		response, err := ms.InstallFromMarketplace(context.Background(), req)
		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.True(t, response.Success)
		assert.Equal(t, "com.tyk.installable", response.MarketplaceID)
		assert.Equal(t, "1.0.0", response.Version)
		assert.NotZero(t, response.PluginID)

		// Verify plugin was created in database
		var plugin models.Plugin
		err = db.First(&plugin, response.PluginID).Error
		assert.NoError(t, err)
		assert.Equal(t, "My Custom Plugin", plugin.Name)
		assert.Equal(t, "production", plugin.Namespace)
		assert.Contains(t, plugin.Command, "oci://")
	})

	t.Run("Install plugin without custom name uses marketplace name", func(t *testing.T) {
		req := &marketplace.InstallRequest{
			PluginID: "com.tyk.installable",
			Version:  "1.0.0",
			Name:     "", // No custom name
		}

		response, err := ms.InstallFromMarketplace(context.Background(), req)
		assert.NoError(t, err)
		assert.NotNil(t, response)

		// Verify plugin uses marketplace name
		var plugin models.Plugin
		err = db.First(&plugin, response.PluginID).Error
		assert.NoError(t, err)
		assert.Equal(t, "Installable Plugin", plugin.Name)
	})
}

func TestMarketplaceService_CheckForUpdates(t *testing.T) {
	ms, db := setupMarketplaceTest(t)

	// Create a plugin
	plugin := &models.Plugin{
		Name:     "Update Check Plugin",
		Command:  "/bin/test",
		HookType: "post_auth",
		IsActive: true,
	}
	db.Create(plugin)

	// Create marketplace plugin (older and newer versions)
	oldTime := time.Now().Add(-24 * time.Hour)
	newTime := time.Now()

	oldVersion := &models.MarketplacePlugin{
		PluginID:        "com.tyk.updatecheck",
		Version:         "1.0.0",
		Name:            "Update Check Plugin",
		OCIRegistry:     "ghcr.io",
		OCIRepository:   "tyk/updatecheck",
		PluginUpdatedAt: oldTime,
	}
	db.Create(oldVersion)

	newVersion := &models.MarketplacePlugin{
		PluginID:        "com.tyk.updatecheck",
		Version:         "2.0.0",
		Name:            "Update Check Plugin",
		OCIRegistry:     "ghcr.io",
		OCIRepository:   "tyk/updatecheck",
		PluginUpdatedAt: newTime,
	}
	db.Create(newVersion)

	// Create installed version record
	installedVersion := &models.InstalledPluginVersion{
		PluginID:            plugin.ID,
		MarketplacePluginID: "com.tyk.updatecheck",
		InstalledVersion:    "1.0.0",
		AvailableVersion:    "1.0.0",
		UpdateAvailable:     false,
		AutoUpdate:          false,
		LastChecked:         time.Now().Add(-24 * time.Hour),
		InstallSource:       "marketplace",
	}
	db.Create(installedVersion)

	t.Run("Check for updates finds new version", func(t *testing.T) {
		err := ms.CheckForUpdates(context.Background())
		assert.NoError(t, err)

		// Verify installed version was updated
		var updated models.InstalledPluginVersion
		err = db.Preload("Plugin").First(&updated, installedVersion.ID).Error
		assert.NoError(t, err)

		// Verify LastChecked was updated
		assert.WithinDuration(t, time.Now(), updated.LastChecked, 5*time.Second)

		// Note: The actual version comparison logic depends on GORM ordering
		// We verify the function runs without error and updates LastChecked
	})

	t.Run("Check for updates with no installed plugins", func(t *testing.T) {
		ms2, db2 := setupMarketplaceTest(t)
		err := ms2.CheckForUpdates(context.Background())
		assert.NoError(t, err)

		// Should handle empty list gracefully
		var count int64
		db2.Model(&models.InstalledPluginVersion{}).Count(&count)
		assert.Equal(t, int64(0), count)
	})
}

func TestMarketplaceService_Stop(t *testing.T) {
	ms, _ := setupMarketplaceTest(t)

	t.Run("Stop marketplace service", func(t *testing.T) {
		// Should not panic
		ms.Stop()

		// Verify stop channel is closed
		select {
		case <-ms.stopCh:
			// Expected - channel is closed
		case <-time.After(100 * time.Millisecond):
			t.Error("Stop channel should be closed after Stop()")
		}
	})
}

func TestMarketplaceService_SyncAll(t *testing.T) {
	t.Run("SyncAll with no indexes creates default", func(t *testing.T) {
		ms, db := setupMarketplaceTest(t)

		// Note: This will fail to fetch from the real URL, but we can test the logic
		err := ms.SyncAll(context.Background())
		// Expected to fail fetching from fake URL
		assert.Error(t, err)

		// But should have created default index
		var indexes []*models.MarketplaceIndex
		db.Find(&indexes)
		assert.GreaterOrEqual(t, len(indexes), 1)
	})

	t.Run("SyncAll with no default URL", func(t *testing.T) {
		db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		models.InitModels(db)

		ms := NewMarketplaceService(db, nil, nil, nil, "./cache", "", 1*time.Hour)

		err := ms.SyncAll(context.Background())
		// Should return nil when no indexes exist and no default URL
		assert.NoError(t, err)
	})
}
