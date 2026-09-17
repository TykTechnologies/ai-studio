package grpc

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A tool's access-method switches are enforced by edge gateways too, so they
// are snapshot content: flipping one has to change the checksum and mark the
// edges pending, or the switch would only take effect on the embedded gateway.
func TestControlServer_ToolAccessSwitchMarksEdgesPending(t *testing.T) {
	server, db := setupTestServer(t, nil)

	tool := models.Tool{Name: "Switch Tool", ToolType: models.ToolTypeREST, Active: true, AvailableOperations: "getThing"}
	require.NoError(t, db.Create(&tool).Error)

	edge := models.EdgeInstance{EdgeID: "edge-1", Namespace: "default", Status: models.EdgeStatusRegistered, SyncStatus: models.EdgeSyncStatusInSync}
	require.NoError(t, db.Create(&edge).Error)
	syncOf := func() string {
		var e models.EdgeInstance
		require.NoError(t, db.Where("edge_id = ?", "edge-1").First(&e).Error)
		return e.SyncStatus
	}

	server.onConfigurationChanged(topicToolCreated, eventbridge.Event{ID: "1"})
	var status models.NamespaceSyncStatus
	require.NoError(t, status.GetByNamespace(db, "default"))
	before := status.ExpectedChecksum
	require.NotEmpty(t, before)

	require.NoError(t, db.Model(&models.EdgeInstance{}).Where("edge_id = ?", "edge-1").Update("sync_status", models.EdgeSyncStatusInSync).Error)
	require.NoError(t, db.Model(&models.Tool{}).Where("id = ?", tool.ID).Update("mcp_access_disabled", true).Error)

	server.onConfigurationChanged(topicToolUpdated, eventbridge.Event{ID: "2"})
	assert.Equal(t, models.EdgeSyncStatusPending, syncOf())
	require.NoError(t, status.GetByNamespace(db, "default"))
	assert.NotEqual(t, before, status.ExpectedChecksum)
}

func TestSnapshot_CarriesToolAccessSwitches(t *testing.T) {
	server, db := setupTestServer(t, nil)

	require.NoError(t, db.Create(&models.Tool{
		Name: "Rest Only", ToolType: models.ToolTypeREST, Active: true, MCPAccessDisabled: true,
	}).Error)
	require.NoError(t, db.Create(&models.Tool{
		Name: "Legacy Row", ToolType: models.ToolTypeREST, Active: true,
	}).Error)

	snapshot, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)

	bySlug := map[string]struct{ rest, mcp bool }{}
	for _, tool := range snapshot.Tools {
		bySlug[tool.Slug] = struct{ rest, mcp bool }{tool.RestAccessDisabled, tool.McpAccessDisabled}
	}
	require.Contains(t, bySlug, "rest-only")
	assert.False(t, bySlug["rest-only"].rest)
	assert.True(t, bySlug["rest-only"].mcp)
	require.Contains(t, bySlug, "legacy-row")
	assert.False(t, bySlug["legacy-row"].rest, "a row without the switches stays enabled")
	assert.False(t, bySlug["legacy-row"].mcp, "a row without the switches stays enabled")
}
