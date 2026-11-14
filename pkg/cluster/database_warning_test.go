package cluster

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/dbstate"
)

// The cluster status warns when this replica's database does not take
// writes, and says when it stopped.
func TestDatabaseWarning(t *testing.T) {
	since := time.Date(2026, 11, 12, 9, 30, 0, 0, time.UTC)
	ro := databaseWarning(dbstate.Status{State: dbstate.ReadOnly, NotWritableSince: since}, `Replica "site-b" (node-b)`)
	assert.Contains(t, ro, `Replica "site-b" (node-b) has seen its database as read-only since 2026-11-12T09:30:00Z`)
	assert.Contains(t, ro, "writes fail")
	down := databaseWarning(dbstate.Status{State: dbstate.Unreachable, NotWritableSince: since}, "Replica node-b")
	assert.Contains(t, down, "Replica node-b cannot reach its database since 2026-11-12T09:30:00Z")
}

// The warning names the replica by its label and node ID, from its row
// even when it is no longer among the live nodes, and by its node ID alone
// when it has no label.
func TestSelfName(t *testing.T) {
	db := sqliteDB(t)
	require.NoError(t, db.Create(&models.ClusterNode{NodeID: "node-b", Hostname: "h", Version: "test", Label: "site-b", StartedAt: time.Now(), LastSeen: time.Now().Add(-time.Hour)}).Error)
	assert.Equal(t, `Replica "site-a" (node-a)`, selfName(db, "node-a", []models.ClusterNode{{NodeID: "node-a", Label: "site-a"}}))
	assert.Equal(t, `Replica "site-b" (node-b)`, selfName(db, "node-b", nil), "read from its row when not live")
	assert.Equal(t, "Replica node-c", selfName(db, "node-c", nil))
}

// SQLite has no tracker: no database warning.
func TestClusterStatusNoDatabaseWarningOnSQLite(t *testing.T) {
	db := sqliteDB(t)
	require.NoError(t, db.AutoMigrate(&models.ClusterLease{}, &models.EdgeInstance{}, &models.EdgePushCommand{}))
	st, err := Snapshot(context.Background(), db, "node-a", nil, nil)
	require.NoError(t, err)
	for _, w := range st.Warnings {
		assert.NotContains(t, w, "database")
	}
}
