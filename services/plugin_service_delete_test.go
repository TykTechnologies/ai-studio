package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Deleting a plugin must take its KV namespace with it. A reinstall gets a
// new id, so rows left under the old id are unreachable through the KV
// service (ClearAllPluginData refuses an id with no plugin row) and would
// otherwise leak forever.
func TestDeletePlugin_RemovesKVData(t *testing.T) {
	kvService, db, pluginID := setupPluginKVTest(t)

	created, err := kvService.WriteKV(pluginID, "settings", []byte(`{"enabled":true}`), nil)
	require.NoError(t, err)
	require.True(t, created)
	created, err = kvService.WriteKV(pluginID, "counter", []byte("42"), nil)
	require.NoError(t, err)
	require.True(t, created)

	before, err := models.CountPluginDataByPluginID(db, pluginID)
	require.NoError(t, err)
	require.Equal(t, int64(2), before)

	require.NoError(t, NewPluginService(db).DeletePlugin(pluginID))

	after, err := models.CountPluginDataByPluginID(db, pluginID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), after, "KV rows must not outlive their plugin")

	// Not just soft-deleted: tombstones under the old id serve nothing and
	// still accumulate across delete + reinstall cycles.
	var remaining int64
	require.NoError(t, db.Unscoped().Model(&models.PluginData{}).Where("plugin_id = ?", pluginID).Count(&remaining).Error)
	assert.Equal(t, int64(0), remaining, "plugin_data rows are removed outright, not tombstoned")
}
