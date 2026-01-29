package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSyncStatusTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = db.AutoMigrate(&NamespaceSyncStatus{}, &SyncAuditLog{}, &EdgeInstance{})
	require.NoError(t, err)
	return db
}

func TestNamespaceSyncStatus_Upsert(t *testing.T) {
	db := setupSyncStatusTestDB(t)

	t.Run("creates new record", func(t *testing.T) {
		status := &NamespaceSyncStatus{
			Namespace:        "test-namespace",
			ExpectedChecksum: "abc123",
			ConfigVersion:    "1234567890",
			LastConfigChange: time.Now(),
		}

		err := status.Upsert(db)
		require.NoError(t, err)
		assert.NotZero(t, status.ID)

		// Verify in database
		var found NamespaceSyncStatus
		err = db.Where("namespace = ?", "test-namespace").First(&found).Error
		require.NoError(t, err)
		assert.Equal(t, "abc123", found.ExpectedChecksum)
	})

	t.Run("updates existing record", func(t *testing.T) {
		// Create initial record
		status := &NamespaceSyncStatus{
			Namespace:        "update-test",
			ExpectedChecksum: "initial-checksum",
			ConfigVersion:    "v1",
			LastConfigChange: time.Now(),
		}
		err := status.Upsert(db)
		require.NoError(t, err)
		originalID := status.ID

		// Update with new checksum
		status2 := &NamespaceSyncStatus{
			Namespace:        "update-test",
			ExpectedChecksum: "updated-checksum",
			ConfigVersion:    "v2",
			LastConfigChange: time.Now(),
		}
		err = status2.Upsert(db)
		require.NoError(t, err)

		// Should have same ID (updated, not created new)
		assert.Equal(t, originalID, status2.ID)

		// Verify updated values
		var found NamespaceSyncStatus
		err = db.Where("namespace = ?", "update-test").First(&found).Error
		require.NoError(t, err)
		assert.Equal(t, "updated-checksum", found.ExpectedChecksum)
		assert.Equal(t, "v2", found.ConfigVersion)
	})
}

func TestNamespaceSyncStatus_GetByNamespace(t *testing.T) {
	db := setupSyncStatusTestDB(t)

	// Create test record
	status := &NamespaceSyncStatus{
		Namespace:        "get-test",
		ExpectedChecksum: "test-checksum",
		ConfigVersion:    "v1",
		LastConfigChange: time.Now(),
	}
	err := status.Upsert(db)
	require.NoError(t, err)

	t.Run("finds existing namespace", func(t *testing.T) {
		var found NamespaceSyncStatus
		err := found.GetByNamespace(db, "get-test")
		require.NoError(t, err)
		assert.Equal(t, "test-checksum", found.ExpectedChecksum)
	})

	t.Run("returns error for non-existent namespace", func(t *testing.T) {
		var notFound NamespaceSyncStatus
		err := notFound.GetByNamespace(db, "non-existent")
		assert.Error(t, err)
	})
}

func TestNamespaceSyncStatus_GetAll(t *testing.T) {
	db := setupSyncStatusTestDB(t)

	// Create test records
	namespaces := []string{"ns-a", "ns-b", "ns-c"}
	for _, ns := range namespaces {
		status := &NamespaceSyncStatus{
			Namespace:        ns,
			ExpectedChecksum: "checksum-" + ns,
			ConfigVersion:    "v1",
			LastConfigChange: time.Now(),
		}
		err := status.Upsert(db)
		require.NoError(t, err)
	}

	t.Run("returns all namespaces sorted", func(t *testing.T) {
		var status NamespaceSyncStatus
		all, err := status.GetAll(db)
		require.NoError(t, err)
		assert.Len(t, all, 3)
		// Should be sorted alphabetically
		assert.Equal(t, "ns-a", all[0].Namespace)
		assert.Equal(t, "ns-b", all[1].Namespace)
		assert.Equal(t, "ns-c", all[2].Namespace)
	})
}

func TestSyncAuditLog_Create(t *testing.T) {
	db := setupSyncStatusTestDB(t)

	t.Run("creates audit log entry", func(t *testing.T) {
		edgeID := "edge-123"
		log := &SyncAuditLog{
			EventType:     SyncEventConfigChanged,
			Namespace:     "test-ns",
			EdgeID:        &edgeID,
			Checksum:      "abc123",
			ConfigVersion: "v1",
			Details:       "Test details",
		}

		err := log.Create(db)
		require.NoError(t, err)
		assert.NotZero(t, log.ID)
	})
}

func TestSyncAuditLog_GetFiltered(t *testing.T) {
	db := setupSyncStatusTestDB(t)

	// Create test logs
	edge1 := "edge-1"
	edge2 := "edge-2"
	logs := []*SyncAuditLog{
		{EventType: SyncEventConfigChanged, Namespace: "ns-a", Checksum: "c1"},
		{EventType: SyncEventEdgeAck, Namespace: "ns-a", EdgeID: &edge1, Checksum: "c1"},
		{EventType: SyncEventEdgeOutOfSync, Namespace: "ns-b", EdgeID: &edge2, Checksum: "c2"},
		{EventType: SyncEventConfigChanged, Namespace: "ns-b", Checksum: "c2"},
	}
	for _, l := range logs {
		err := l.Create(db)
		require.NoError(t, err)
	}

	t.Run("filters by namespace", func(t *testing.T) {
		var log SyncAuditLog
		results, err := log.GetFiltered(db, "ns-a", "", "", 10)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("filters by edge ID", func(t *testing.T) {
		var log SyncAuditLog
		results, err := log.GetFiltered(db, "", "edge-1", "", 10)
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})

	t.Run("filters by event type", func(t *testing.T) {
		var log SyncAuditLog
		results, err := log.GetFiltered(db, "", "", SyncEventConfigChanged, 10)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("respects limit", func(t *testing.T) {
		var log SyncAuditLog
		results, err := log.GetFiltered(db, "", "", "", 2)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})
}

func TestEdgeInstance_SyncStatus(t *testing.T) {
	db := setupSyncStatusTestDB(t)

	// Create test edge
	edge := &EdgeInstance{
		EdgeID:    "test-edge",
		Namespace: "test-ns",
		Status:    EdgeStatusConnected,
	}
	err := edge.Create(db)
	require.NoError(t, err)

	t.Run("updates sync status", func(t *testing.T) {
		err := edge.UpdateSyncStatus(db, "checksum-123", "v1", EdgeSyncStatusInSync)
		require.NoError(t, err)

		// Verify
		var found EdgeInstance
		err = found.GetByEdgeID(db, "test-edge")
		require.NoError(t, err)
		assert.Equal(t, "checksum-123", found.LoadedChecksum)
		assert.Equal(t, "v1", found.LoadedVersion)
		assert.Equal(t, EdgeSyncStatusInSync, found.SyncStatus)
		assert.NotNil(t, found.LastSyncAck) // Should be set when in_sync
	})

	t.Run("marks edges as pending in namespace", func(t *testing.T) {
		// Create additional edges
		edges := []*EdgeInstance{
			{EdgeID: "edge-a", Namespace: "ns-pending", Status: EdgeStatusConnected, SyncStatus: EdgeSyncStatusInSync},
			{EdgeID: "edge-b", Namespace: "ns-pending", Status: EdgeStatusConnected, SyncStatus: EdgeSyncStatusInSync},
			{EdgeID: "edge-c", Namespace: "other-ns", Status: EdgeStatusConnected, SyncStatus: EdgeSyncStatusInSync},
		}
		for _, e := range edges {
			err := e.Create(db)
			require.NoError(t, err)
		}

		// Mark ns-pending as pending
		edge := &EdgeInstance{}
		err := edge.MarkEdgesAsPendingInNamespace(db, "ns-pending")
		require.NoError(t, err)

		// Verify ns-pending edges are pending
		var edgeA EdgeInstance
		err = edgeA.GetByEdgeID(db, "edge-a")
		require.NoError(t, err)
		assert.Equal(t, EdgeSyncStatusPending, edgeA.SyncStatus)

		// Verify other-ns edge is still in_sync
		var edgeC EdgeInstance
		err = edgeC.GetByEdgeID(db, "edge-c")
		require.NoError(t, err)
		assert.Equal(t, EdgeSyncStatusInSync, edgeC.SyncStatus)
	})

	t.Run("counts edges by sync status", func(t *testing.T) {
		edge := &EdgeInstance{}
		counts, err := edge.CountEdgesBySyncStatus(db, "ns-pending")
		require.NoError(t, err)
		assert.Equal(t, int64(2), counts[EdgeSyncStatusPending])
	})
}
