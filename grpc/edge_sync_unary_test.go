package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A reload makes the microgateway pull through the unary GetFullConfiguration
// with its configured EDGE_NAMESPACE, which is "" by default while the edge
// registered under the normalised "default". The pull must resolve the same
// namespace registration did, and mark the pulling edge in sync right away
// instead of leaving it pending until the next heartbeat.

func edgeRow(t *testing.T, db *gorm.DB, edgeID string) models.EdgeInstance {
	t.Helper()
	var e models.EdgeInstance
	require.NoError(t, db.Where("edge_id = ?", edgeID).First(&e).Error)
	return e
}

func TestControlServer_GetFullConfiguration_MarksRequestingEdgeInSync(t *testing.T) {
	for _, registered := range []string{"", "default"} {
		t.Run("registered under "+registered, func(t *testing.T) {
			server, db := setupTestServer(t, nil)
			ctx := context.Background()
			edgeID := "edge-" + registered

			reg, err := server.RegisterEdge(ctx, &pb.EdgeRegistrationRequest{EdgeId: edgeID, EdgeNamespace: registered, Version: "1.0.0"})
			require.NoError(t, err)
			require.True(t, reg.Success)
			stored := edgeRow(t, db, edgeID)
			require.Equal(t, "default", stored.Namespace, "registration normalises the namespace")

			// A configuration change since registration.
			createTestLLMs(db, "")

			// The pull arrives with the edge's raw EDGE_NAMESPACE ("").
			snapshot, err := server.GetFullConfiguration(ctx, &pb.ConfigurationRequest{EdgeId: edgeID, EdgeNamespace: ""})
			require.NoError(t, err)
			require.NotEmpty(t, snapshot.Checksum)
			assert.NotEqual(t, reg.InitialConfig.Checksum, snapshot.Checksum, "the change is in the snapshot")

			var row models.NamespaceSyncStatus
			require.NoError(t, row.GetByNamespace(db, stored.Namespace))
			assert.Equal(t, snapshot.Checksum, row.ExpectedChecksum, "the row the heartbeat compares against carries the pulled checksum")

			after := edgeRow(t, db, edgeID)
			assert.Equal(t, models.EdgeSyncStatusInSync, after.SyncStatus)
			assert.Equal(t, snapshot.Checksum, after.LoadedChecksum)
			assert.Equal(t, snapshot.Version, after.LoadedVersion)
			require.NotNil(t, after.LastSyncAck)
			assert.WithinDuration(t, time.Now(), *after.LastSyncAck, 5*time.Second)
		})
	}
}

// Unknown edge id (or none, from an older microgateway): the snapshot is
// still served for the normalised namespace and nothing is marked.
func TestControlServer_GetFullConfiguration_WithoutEdgeID(t *testing.T) {
	server, db := setupTestServer(t, nil)
	createTestLLMs(db, "")

	snapshot, err := server.GetFullConfiguration(context.Background(), &pb.ConfigurationRequest{EdgeNamespace: ""})
	require.NoError(t, err)
	require.NotEmpty(t, snapshot.Checksum)

	var row models.NamespaceSyncStatus
	require.NoError(t, row.GetByNamespace(db, "default"))
	assert.Equal(t, snapshot.Checksum, row.ExpectedChecksum)
	var count int64
	require.NoError(t, db.Model(&models.NamespaceSyncStatus{}).Count(&count).Error)
	assert.Equal(t, int64(1), count, "no second row under the raw spelling")
}

// Two edges in one namespace: after a change, the edge that pulls is in sync
// with the new checksum and the other one is pending.
func TestControlServer_GetFullConfiguration_OnlyPullingEdgeFlipsInSync(t *testing.T) {
	server, db := setupTestServer(t, nil)
	ctx := context.Background()

	for _, id := range []string{"edge-a", "edge-b"} {
		_, err := server.RegisterEdge(ctx, &pb.EdgeRegistrationRequest{EdgeId: id, EdgeNamespace: "", Version: "1.0.0"})
		require.NoError(t, err)
		require.Equal(t, models.EdgeSyncStatusInSync, edgeRow(t, db, id).SyncStatus)
	}

	createTestLLMs(db, "")
	snapshot, err := server.GetFullConfiguration(ctx, &pb.ConfigurationRequest{EdgeId: "edge-a", EdgeNamespace: ""})
	require.NoError(t, err)

	a := edgeRow(t, db, "edge-a")
	b := edgeRow(t, db, "edge-b")
	assert.Equal(t, models.EdgeSyncStatusInSync, a.SyncStatus)
	assert.Equal(t, snapshot.Checksum, a.LoadedChecksum)
	assert.Equal(t, models.EdgeSyncStatusPending, b.SyncStatus)
	assert.NotEqual(t, snapshot.Checksum, b.LoadedChecksum)
}

// A push issued before any snapshot exists creates the namespace row with an
// empty checksum. The first snapshot after it fills the checksum in; that is
// not a configuration change, so edges are not flipped and nothing is
// audited. A later different checksum is a change as before.
func TestControlServer_updateNamespaceSyncStatus_FirstFillAfterPushIsNotAChange(t *testing.T) {
	server, db := setupTestServer(t, nil)
	pushedAt := time.Now().Add(-time.Minute)
	require.NoError(t, models.MarkNamespacePushed(db, "default", pushedAt))

	now := time.Now()
	edge := models.EdgeInstance{EdgeID: "edge-1", Namespace: "default", Status: models.EdgeStatusConnected,
		SyncStatus: models.EdgeSyncStatusInSync, LoadedChecksum: "abc", LastSyncAck: &now}
	require.NoError(t, db.Create(&edge).Error)

	require.NoError(t, server.updateNamespaceSyncStatus("default", "abc", "1"))

	assert.Equal(t, models.EdgeSyncStatusInSync, edgeRow(t, db, "edge-1").SyncStatus, "first fill leaves edges alone")
	var row models.NamespaceSyncStatus
	require.NoError(t, row.GetByNamespace(db, "default"))
	assert.Equal(t, "abc", row.ExpectedChecksum)
	require.NotNil(t, row.LastPushAt, "the push stamp survives the fill")
	var audits int64
	require.NoError(t, db.Model(&models.SyncAuditLog{}).Where("event_type = ?", models.SyncEventConfigChanged).Count(&audits).Error)
	assert.Equal(t, int64(0), audits, "no config_changed audit for the first fill")

	// A real change afterwards behaves as before.
	require.NoError(t, server.updateNamespaceSyncStatus("", "def", "2"))
	assert.Equal(t, models.EdgeSyncStatusPending, edgeRow(t, db, "edge-1").SyncStatus)
	require.NoError(t, db.Model(&models.SyncAuditLog{}).Where("event_type = ?", models.SyncEventConfigChanged).Count(&audits).Error)
	assert.Equal(t, int64(1), audits)
	var rows int64
	require.NoError(t, db.Model(&models.NamespaceSyncStatus{}).Count(&rows).Error)
	assert.Equal(t, int64(1), rows, `"" and "default" are one row`)
}

// "" and "default" name the same namespace: same object set, same checksum,
// one sync row.
func TestControlServer_getConfigurationSnapshot_NamespaceSpellingsAgree(t *testing.T) {
	server, db := setupTestServer(t, nil)
	createTestLLMs(db, "")
	createTestLLMs(db, "default")

	empty, err := server.getConfigurationSnapshot("")
	require.NoError(t, err)
	named, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	global, err := server.getConfigurationSnapshot("global")
	require.NoError(t, err)

	assert.Equal(t, named.Checksum, empty.Checksum)
	assert.Equal(t, named.Checksum, global.Checksum)
	assert.Len(t, empty.Llms, len(named.Llms))
	assert.Greater(t, len(named.Llms), len(createTestLLMs(db, "other")), "global and default-namespaced LLMs are both included")

	var rows []models.NamespaceSyncStatus
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, "default", rows[0].Namespace)
}

// Two snapshots over the same seeded database give the same checksum: the
// snapshot queries are ordered so row order cannot leak into the checksum.
func TestControlServer_getConfigurationSnapshot_IsDeterministic(t *testing.T) {
	server, db := setupTestServer(t, nil)
	llms := createTestLLMs(db, "")
	createTestApps(db, "", llms)
	createTestLLMs(db, "default")
	require.NoError(t, db.Create(&models.Filter{Name: "F1", Namespace: ""}).Error)
	require.NoError(t, db.Create(&models.Filter{Name: "F2", Namespace: "default"}).Error)
	require.NoError(t, db.Create(&models.ModelPrice{ModelName: "m1", Vendor: "openai"}).Error)
	require.NoError(t, db.Create(&models.ModelPrice{ModelName: "m2", Vendor: "openai"}).Error)
	require.NoError(t, db.Create(&models.Tool{Name: "T1", Namespace: ""}).Error)
	require.NoError(t, db.Create(&models.Datasource{Name: "D1", Namespace: ""}).Error)

	first, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	second, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	require.NotEmpty(t, first.Checksum)
	assert.Equal(t, first.Checksum, second.Checksum)
}
