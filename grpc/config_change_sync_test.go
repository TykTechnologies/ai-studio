package grpc

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Config-change events recompute the namespace checksum; edges are only
// marked pending when the snapshot actually changed, so governed metadata
// edits that are not gateway-visible (or any no-op change) don't churn edges.
func TestControlServer_onConfigurationChanged_MarksPendingOnlyWhenSnapshotChanges(t *testing.T) {
	server, db := setupTestServer(t, nil)

	edge := models.EdgeInstance{EdgeID: "edge-1", Namespace: "default", Status: models.EdgeStatusRegistered, SyncStatus: models.EdgeSyncStatusInSync}
	require.NoError(t, db.Create(&edge).Error)
	syncOf := func() string {
		var e models.EdgeInstance
		require.NoError(t, db.Where("edge_id = ?", "edge-1").First(&e).Error)
		return e.SyncStatus
	}

	// First event: no expected checksum recorded yet → edges are marked pending.
	server.onConfigurationChanged(topicGovernedMetadataUpdated, eventbridge.Event{ID: "1"})
	assert.Equal(t, models.EdgeSyncStatusPending, syncOf())
	var status models.NamespaceSyncStatus
	require.NoError(t, status.GetByNamespace(db, "default"))
	first := status.ExpectedChecksum
	assert.NotEmpty(t, first)

	// Same configuration again → checksum identical → edge stays in sync.
	require.NoError(t, db.Model(&models.EdgeInstance{}).Where("edge_id = ?", "edge-1").Update("sync_status", models.EdgeSyncStatusInSync).Error)
	server.onConfigurationChanged(topicGovernedMetadataUpdated, eventbridge.Event{ID: "2"})
	assert.Equal(t, models.EdgeSyncStatusInSync, syncOf())

	// A real change (new active LLM) alters the snapshot → pending again.
	createTestLLMs(db, "")
	server.onConfigurationChanged(topicLLMCreated, eventbridge.Event{ID: "3"})
	assert.Equal(t, models.EdgeSyncStatusPending, syncOf())
	require.NoError(t, status.GetByNamespace(db, "default"))
	assert.NotEqual(t, first, status.ExpectedChecksum)
}
