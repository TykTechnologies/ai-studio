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
	assert.Empty(t, databaseWarning(dbstate.Status{State: dbstate.Writable}))

	since := time.Date(2026, 11, 12, 9, 30, 0, 0, time.UTC)
	ro := databaseWarning(dbstate.Status{State: dbstate.ReadOnly, NotWritableSince: since})
	assert.Contains(t, ro, "read-only since 2026-11-12T09:30:00Z")
	assert.Contains(t, ro, "writes fail")
	down := databaseWarning(dbstate.Status{State: dbstate.Unreachable, NotWritableSince: since})
	assert.Contains(t, down, "cannot reach its database since 2026-11-12T09:30:00Z")
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
