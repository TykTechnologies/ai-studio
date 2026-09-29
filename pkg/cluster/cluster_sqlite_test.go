package cluster

import (
	"context"
	"path/filepath"
	"testing"

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
