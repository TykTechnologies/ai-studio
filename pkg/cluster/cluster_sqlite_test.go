package cluster

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

func sqliteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "c.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.ClusterNode{}, &models.ClusterEvent{}))
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return db
}

// SQLite serves one process: the node registry still runs (the status page
// and ownership checks use it) and the event log is inert.
func TestSQLiteRunsOneNodeAndNoLog(t *testing.T) {
	db := sqliteDB(t)
	n, err := StartNode(db, "solo", "test")
	require.NoError(t, err)
	live, err := LiveNodes(db)
	require.NoError(t, err)
	assert.Equal(t, []string{"solo"}, nodeIDs(live))
	ok, err := IsLive(db, "solo")
	require.NoError(t, err)
	assert.True(t, ok)
	require.NoError(t, n.beat(), "refreshing an existing row upserts")
	n.Stop(context.Background())
	ok, err = IsLive(db, "solo")
	require.NoError(t, err)
	assert.False(t, ok)

	l := NewLog(db, "solo", LogOptions{})
	l.Subscribe("*", func(Event) { t.Fatal("no delivery on SQLite") })
	require.NoError(t, l.Start(context.Background()))
	require.NoError(t, l.Publish(context.Background(), "t", []byte("x")))
	assert.False(t, l.Stats().Enabled)
	var count int64
	require.NoError(t, db.Model(&models.ClusterEvent{}).Count(&count).Error)
	assert.Zero(t, count)
	l.Stop()
}

func TestNewNodeIDIsUniquePerCall(t *testing.T) {
	a, b := NewNodeID(), NewNodeID()
	assert.NotEqual(t, a, b, "a restarted process must not reuse its predecessor's ID")
}

// pruneCase leaves the rows of two replicas that crashed (never removed
// their rows), one long ago and one recently, then starts a node: the long
// dead row is deleted, the recent one kept (it still shows what ran, and a
// restarted replica may need it to recognise its predecessor).
func pruneCase(t *testing.T, db *gorm.DB) {
	t.Helper()
	for node, silent := range map[string]time.Duration{"crashed-long-ago": NodeRetention + time.Minute, "crashed-recently": 5 * time.Minute} {
		require.NoError(t, db.Exec(`INSERT INTO cluster_nodes (node_id, hostname, version, started_at, last_seen) VALUES (?, 'h', 'test', ?, ?)`,
			node, sinceExpr(db, silent+time.Hour), sinceExpr(db, silent)).Error)
	}
	n, err := StartNode(db, "fresh", "test")
	require.NoError(t, err)
	defer n.Stop(context.Background())
	var ids []string
	require.NoError(t, db.Model(&models.ClusterNode{}).Order("node_id").Pluck("node_id", &ids).Error)
	assert.Equal(t, []string{"crashed-recently", "fresh"}, ids)
}

func TestNodePrunesLongStoppedNodes(t *testing.T) { pruneCase(t, sqliteDB(t)) }
