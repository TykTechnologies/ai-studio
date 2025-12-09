package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupEdgeInstanceTest(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = InitModels(db)
	assert.NoError(t, err)

	return db
}

func TestNewEdgeInstance(t *testing.T) {
	t.Run("Create new edge instance", func(t *testing.T) {
		edge := NewEdgeInstance()
		assert.NotNil(t, edge)
		assert.Equal(t, EdgeStatusRegistered, edge.Status)
	})
}

func TestEdgeInstance_Create(t *testing.T) {
	db := setupEdgeInstanceTest(t)

	t.Run("Create edge instance successfully", func(t *testing.T) {
		edge := &EdgeInstance{
			EdgeID:    "edge-001",
			Namespace: "production",
			Version:   "1.0.0",
			BuildHash: "abc123",
			Metadata:  map[string]interface{}{"region": "us-east-1"},
		}

		err := edge.Create(db)
		assert.NoError(t, err)
		assert.NotZero(t, edge.ID)
	})

	t.Run("Create duplicate edge ID fails", func(t *testing.T) {
		edge1 := &EdgeInstance{EdgeID: "edge-duplicate", Namespace: "prod"}
		edge1.Create(db)

		edge2 := &EdgeInstance{EdgeID: "edge-duplicate", Namespace: "dev"}
		err := edge2.Create(db)
		assert.Error(t, err) // Unique constraint violation
	})
}

func TestEdgeInstance_Get(t *testing.T) {
	db := setupEdgeInstanceTest(t)

	edge := &EdgeInstance{EdgeID: "edge-get-test", Namespace: "test"}
	db.Create(edge)

	t.Run("Get existing edge instance", func(t *testing.T) {
		retrieved := &EdgeInstance{}
		err := retrieved.Get(db, edge.ID)
		assert.NoError(t, err)
		assert.Equal(t, "edge-get-test", retrieved.EdgeID)
	})

	t.Run("Get non-existent edge instance", func(t *testing.T) {
		retrieved := &EdgeInstance{}
		err := retrieved.Get(db, 99999)
		assert.Error(t, err)
	})
}

func TestEdgeInstance_GetByEdgeID(t *testing.T) {
	db := setupEdgeInstanceTest(t)

	edge := &EdgeInstance{EdgeID: "edge-by-id", Namespace: "test"}
	db.Create(edge)

	t.Run("Get by existing edge ID", func(t *testing.T) {
		retrieved := &EdgeInstance{}
		err := retrieved.GetByEdgeID(db, "edge-by-id")
		assert.NoError(t, err)
		assert.Equal(t, edge.ID, retrieved.ID)
	})

	t.Run("Get by non-existent edge ID", func(t *testing.T) {
		retrieved := &EdgeInstance{}
		err := retrieved.GetByEdgeID(db, "nonexistent")
		assert.Error(t, err)
	})
}

func TestEdgeInstance_Update(t *testing.T) {
	db := setupEdgeInstanceTest(t)

	edge := &EdgeInstance{EdgeID: "edge-update", Namespace: "test", Version: "1.0.0"}
	db.Create(edge)

	t.Run("Update edge instance", func(t *testing.T) {
		edge.Version = "2.0.0"
		edge.BuildHash = "xyz789"

		err := edge.Update(db)
		assert.NoError(t, err)

		retrieved := &EdgeInstance{}
		retrieved.Get(db, edge.ID)
		assert.Equal(t, "2.0.0", retrieved.Version)
		assert.Equal(t, "xyz789", retrieved.BuildHash)
	})
}

func TestEdgeInstance_Delete(t *testing.T) {
	db := setupEdgeInstanceTest(t)

	edge := &EdgeInstance{EdgeID: "edge-delete", Namespace: "test"}
	db.Create(edge)

	t.Run("Delete edge instance (hard delete)", func(t *testing.T) {
		err := edge.Delete(db)
		assert.NoError(t, err)

		// Verify hard deletion (not soft delete)
		retrieved := &EdgeInstance{}
		err = db.Unscoped().First(retrieved, edge.ID).Error
		assert.Error(t, err) // Should not exist even in unscoped query
	})
}

func TestEdgeInstance_UpdateHeartbeat(t *testing.T) {
	db := setupEdgeInstanceTest(t)

	edge := &EdgeInstance{EdgeID: "edge-heartbeat", Namespace: "test"}
	db.Create(edge)

	t.Run("Update heartbeat timestamp", func(t *testing.T) {
		assert.Nil(t, edge.LastHeartbeat)

		err := edge.UpdateHeartbeat(db)
		assert.NoError(t, err)
		assert.NotNil(t, edge.LastHeartbeat)
		assert.WithinDuration(t, time.Now(), *edge.LastHeartbeat, 1*time.Second)
	})
}

func TestEdgeInstance_UpdateStatus(t *testing.T) {
	db := setupEdgeInstanceTest(t)

	edge := &EdgeInstance{EdgeID: "edge-status", Namespace: "test", Status: EdgeStatusRegistered}
	db.Create(edge)

	t.Run("Update status to connected", func(t *testing.T) {
		err := edge.UpdateStatus(db, EdgeStatusConnected)
		assert.NoError(t, err)
		assert.Equal(t, EdgeStatusConnected, edge.Status)

		retrieved := &EdgeInstance{}
		retrieved.Get(db, edge.ID)
		assert.Equal(t, EdgeStatusConnected, retrieved.Status)
	})

	t.Run("Update status to disconnected", func(t *testing.T) {
		err := edge.UpdateStatus(db, EdgeStatusDisconnected)
		assert.NoError(t, err)
		assert.Equal(t, EdgeStatusDisconnected, edge.Status)
	})
}

func TestEdgeInstance_IsHealthy(t *testing.T) {
	t.Run("Edge with no heartbeat is unhealthy", func(t *testing.T) {
		edge := &EdgeInstance{EdgeID: "edge-no-heartbeat"}
		assert.False(t, edge.IsHealthy(5*time.Minute))
	})

	t.Run("Edge with recent heartbeat is healthy", func(t *testing.T) {
		now := time.Now()
		edge := &EdgeInstance{EdgeID: "edge-recent", LastHeartbeat: &now}
		assert.True(t, edge.IsHealthy(5*time.Minute))
	})

	t.Run("Edge with old heartbeat is unhealthy", func(t *testing.T) {
		old := time.Now().Add(-10 * time.Minute)
		edge := &EdgeInstance{EdgeID: "edge-old", LastHeartbeat: &old}
		assert.False(t, edge.IsHealthy(5*time.Minute))
	})

	t.Run("Edge just within threshold is healthy", func(t *testing.T) {
		threshold := time.Now().Add(-4*time.Minute - 59*time.Second)
		edge := &EdgeInstance{EdgeID: "edge-threshold", LastHeartbeat: &threshold}
		assert.True(t, edge.IsHealthy(5*time.Minute))
	})
}

func TestEdgeInstances_ListEdgesInNamespace(t *testing.T) {
	db := setupEdgeInstanceTest(t)

	// Create edges in different namespaces
	db.Create(&EdgeInstance{EdgeID: "prod-1", Namespace: "production"})
	db.Create(&EdgeInstance{EdgeID: "prod-2", Namespace: "production"})
	db.Create(&EdgeInstance{EdgeID: "dev-1", Namespace: "development"})

	t.Run("List edges in production namespace", func(t *testing.T) {
		var edges EdgeInstances
		err := edges.ListEdgesInNamespace(db, "production")
		assert.NoError(t, err)
		assert.Len(t, edges, 2)

		for _, edge := range edges {
			assert.Equal(t, "production", edge.Namespace)
		}
	})

	t.Run("List edges in empty namespace", func(t *testing.T) {
		var edges EdgeInstances
		err := edges.ListEdgesInNamespace(db, "nonexistent")
		assert.NoError(t, err)
		assert.Len(t, edges, 0)
	})
}

func TestEdgeInstances_ListActiveEdges(t *testing.T) {
	db := setupEdgeInstanceTest(t)

	// Create edges with different statuses
	db.Create(&EdgeInstance{EdgeID: "active-1", Status: EdgeStatusConnected})
	db.Create(&EdgeInstance{EdgeID: "active-2", Status: EdgeStatusRegistered})
	db.Create(&EdgeInstance{EdgeID: "inactive-1", Status: EdgeStatusDisconnected})
	db.Create(&EdgeInstance{EdgeID: "inactive-2", Status: EdgeStatusUnhealthy})

	t.Run("List only active edges", func(t *testing.T) {
		var edges EdgeInstances
		err := edges.ListActiveEdges(db)
		assert.NoError(t, err)
		assert.Len(t, edges, 2)

		for _, edge := range edges {
			assert.Contains(t, []string{EdgeStatusConnected, EdgeStatusRegistered}, edge.Status)
		}
	})
}

func TestEdgeInstances_ListEdgesByStatus(t *testing.T) {
	db := setupEdgeInstanceTest(t)

	db.Create(&EdgeInstance{EdgeID: "connected-1", Status: EdgeStatusConnected})
	db.Create(&EdgeInstance{EdgeID: "connected-2", Status: EdgeStatusConnected})
	db.Create(&EdgeInstance{EdgeID: "disconnected-1", Status: EdgeStatusDisconnected})

	t.Run("List edges by status", func(t *testing.T) {
		var edges EdgeInstances
		err := edges.ListEdgesByStatus(db, EdgeStatusConnected)
		assert.NoError(t, err)
		assert.Len(t, edges, 2)

		for _, edge := range edges {
			assert.Equal(t, EdgeStatusConnected, edge.Status)
		}
	})
}

func TestEdgeInstance_CountEdgesInNamespace(t *testing.T) {
	db := setupEdgeInstanceTest(t)

	db.Create(&EdgeInstance{EdgeID: "ns-prod-1", Namespace: "production"})
	db.Create(&EdgeInstance{EdgeID: "ns-prod-2", Namespace: "production"})
	db.Create(&EdgeInstance{EdgeID: "ns-dev-1", Namespace: "development"})

	t.Run("Count edges in production namespace", func(t *testing.T) {
		edge := &EdgeInstance{}
		count, err := edge.CountEdgesInNamespace(db, "production")
		assert.NoError(t, err)
		assert.Equal(t, int64(2), count)
	})

	t.Run("Count edges in empty namespace", func(t *testing.T) {
		edge := &EdgeInstance{}
		count, err := edge.CountEdgesInNamespace(db, "nonexistent")
		assert.NoError(t, err)
		assert.Equal(t, int64(0), count)
	})
}

func TestEdgeInstance_CountActiveEdges(t *testing.T) {
	db := setupEdgeInstanceTest(t)

	db.Create(&EdgeInstance{EdgeID: "count-active-1", Status: EdgeStatusConnected})
	db.Create(&EdgeInstance{EdgeID: "count-active-2", Status: EdgeStatusRegistered})
	db.Create(&EdgeInstance{EdgeID: "count-inactive-1", Status: EdgeStatusDisconnected})

	t.Run("Count active edges", func(t *testing.T) {
		edge := &EdgeInstance{}
		count, err := edge.CountActiveEdges(db)
		assert.NoError(t, err)
		assert.Equal(t, int64(2), count)
	})
}

func TestEdgeInstance_CleanupStaleEdges(t *testing.T) {
	db := setupEdgeInstanceTest(t)

	// Create edge with old heartbeat
	oldHeartbeat := time.Now().Add(-10 * time.Minute)
	db.Create(&EdgeInstance{EdgeID: "stale-1", Status: EdgeStatusConnected, LastHeartbeat: &oldHeartbeat})

	// Create edge with recent heartbeat
	recentHeartbeat := time.Now().Add(-2 * time.Minute)
	db.Create(&EdgeInstance{EdgeID: "fresh-1", Status: EdgeStatusConnected, LastHeartbeat: &recentHeartbeat})

	// Create edge with no heartbeat
	db.Create(&EdgeInstance{EdgeID: "no-heartbeat", Status: EdgeStatusConnected, LastHeartbeat: nil})

	t.Run("Cleanup stale edges", func(t *testing.T) {
		edge := &EdgeInstance{}
		err := edge.CleanupStaleEdges(db, 5*time.Minute)
		assert.NoError(t, err)

		// Verify stale edge is marked disconnected
		var stale EdgeInstance
		db.Where("edge_id = ?", "stale-1").First(&stale)
		assert.Equal(t, EdgeStatusDisconnected, stale.Status)

		// Verify fresh edge is still connected
		var fresh EdgeInstance
		db.Where("edge_id = ?", "fresh-1").First(&fresh)
		assert.Equal(t, EdgeStatusConnected, fresh.Status)

		// Verify edge with no heartbeat is marked disconnected
		var noHB EdgeInstance
		db.Where("edge_id = ?", "no-heartbeat").First(&noHB)
		assert.Equal(t, EdgeStatusDisconnected, noHB.Status)
	})

	t.Run("Cleanup doesn't affect non-connected edges", func(t *testing.T) {
		db2 := setupEdgeInstanceTest(t)

		oldHB := time.Now().Add(-10 * time.Minute)
		db2.Create(&EdgeInstance{EdgeID: "disconnected", Status: EdgeStatusDisconnected, LastHeartbeat: &oldHB})

		edge := &EdgeInstance{}
		err := edge.CleanupStaleEdges(db2, 5*time.Minute)
		assert.NoError(t, err)

		// Verify it stays disconnected (not changed)
		var retrieved EdgeInstance
		db2.Where("edge_id = ?", "disconnected").First(&retrieved)
		assert.Equal(t, EdgeStatusDisconnected, retrieved.Status)
	})
}
