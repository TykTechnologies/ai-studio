package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupMarketplaceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&MarketplacePlugin{}))
	return db
}

// seedVersions inserts marketplace plugin rows with the given versions in order.
// All rows share the same pluginID and sourceURL; timestamps are identical to
// exercise the semver-based ordering (timestamps alone cannot determine order).
func seedVersions(t *testing.T, db *gorm.DB, pluginID string, versions []string) {
	t.Helper()
	ts := time.Date(2025, 12, 5, 0, 0, 0, 0, time.UTC)
	for _, v := range versions {
		require.NoError(t, db.Create(&MarketplacePlugin{
			PluginID:        pluginID,
			Version:         v,
			Name:            pluginID,
			Description:     "test",
			PluginCreatedAt: ts,
			PluginUpdatedAt: ts,
			SyncedFromURL:   "https://example.com/index.yaml",
		}).Error)
	}
}

func TestGetLatestVersion_Semver(t *testing.T) {
	db := setupMarketplaceTestDB(t)

	// Insert versions in non-sorted order; all have the same timestamp.
	seedVersions(t, db, "com.tyk.test-plugin", []string{"1.0.2", "1.0.4", "1.0.0", "1.0.3"})

	var latest MarketplacePlugin
	err := latest.GetLatestVersion(db, "com.tyk.test-plugin")
	require.NoError(t, err)
	assert.Equal(t, "1.0.4", latest.Version)
}

func TestGetLatestVersion_NotFound(t *testing.T) {
	db := setupMarketplaceTestDB(t)

	var latest MarketplacePlugin
	err := latest.GetLatestVersion(db, "com.tyk.nonexistent")
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestGetAllPluginVersions_SortOrder(t *testing.T) {
	db := setupMarketplaceTestDB(t)

	seedVersions(t, db, "com.tyk.cache", []string{"1.0.2", "1.0.4", "1.0.0", "1.0.3"})

	versions, err := GetAllPluginVersions(db, "com.tyk.cache")
	require.NoError(t, err)
	require.Len(t, versions, 4)

	expected := []string{"1.0.4", "1.0.3", "1.0.2", "1.0.0"}
	for i, v := range versions {
		assert.Equal(t, expected[i], v.Version, "index %d", i)
	}
}

func TestListMarketplacePlugins_PicksHighestVersion(t *testing.T) {
	db := setupMarketplaceTestDB(t)

	// Two plugins, each with multiple versions inserted in random order.
	seedVersions(t, db, "com.tyk.alpha", []string{"2.0.0", "1.0.0", "3.0.0"})
	seedVersions(t, db, "com.tyk.beta", []string{"0.9.0", "1.1.0", "1.0.0"})

	plugins, totalCount, _, err := ListMarketplacePlugins(db, 10, 1, "", "", "", "", true)
	require.NoError(t, err)
	assert.Equal(t, int64(2), totalCount)
	require.Len(t, plugins, 2)

	// Build a map of pluginID -> version returned
	got := make(map[string]string)
	for _, p := range plugins {
		got[p.PluginID] = p.Version
	}

	assert.Equal(t, "3.0.0", got["com.tyk.alpha"])
	assert.Equal(t, "1.1.0", got["com.tyk.beta"])
}

func TestListMarketplacePlugins_Pagination(t *testing.T) {
	db := setupMarketplaceTestDB(t)

	// Create 5 distinct plugins
	for i := 0; i < 5; i++ {
		seedVersions(t, db, "com.tyk.plugin-"+string(rune('a'+i)), []string{"1.0.0"})
	}

	// Page 1: 2 items
	plugins, totalCount, totalPages, err := ListMarketplacePlugins(db, 2, 1, "", "", "", "", true)
	require.NoError(t, err)
	assert.Equal(t, int64(5), totalCount)
	assert.Equal(t, 3, totalPages)
	assert.Len(t, plugins, 2)

	// Page 3: 1 item
	plugins, _, _, err = ListMarketplacePlugins(db, 2, 3, "", "", "", "", true)
	require.NoError(t, err)
	assert.Len(t, plugins, 1)

	// Page beyond range: 0 items
	plugins, _, _, err = ListMarketplacePlugins(db, 2, 10, "", "", "", "", true)
	require.NoError(t, err)
	assert.Len(t, plugins, 0)
}

func TestSortPluginsBySemverDesc(t *testing.T) {
	plugins := []*MarketplacePlugin{
		{Version: "1.0.0"},
		{Version: "2.0.0"},
		{Version: "1.5.0"},
		{Version: "invalid"},
		{Version: "0.1.0"},
	}

	sortPluginsBySemverDesc(plugins)

	expected := []string{"2.0.0", "1.5.0", "1.0.0", "0.1.0", "invalid"}
	for i, p := range plugins {
		assert.Equal(t, expected[i], p.Version, "index %d", i)
	}
}
