package cluster

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Owners answers for a page of edges in one query: each owning node's label
// and whether it is live. Unknown and empty IDs are left out.
func TestOwners(t *testing.T) {
	db := sqliteDB(t)
	// Registered before the node's goroutine uses the handle: changing the
	// callback chain while it queries is a data race.
	var queries atomic.Int32
	count := func(*gorm.DB) { queries.Add(1) }
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:count", count))
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("test:count_rows", count))

	live, err := StartNode(db, "mdcb-1", "test", NodeOptions{Label: "mdcb-eu-1", NeverLeads: true})
	require.NoError(t, err)
	defer live.Stop(context.Background())
	require.NoError(t, db.Exec(`INSERT INTO cluster_nodes (node_id, hostname, version, label, started_at, last_seen) VALUES ('gone', 'h', 'test', 'dashboard', ?, ?)`,
		sinceExpr(db, time.Hour), sinceExpr(db, 10*time.Minute)).Error)

	queries.Store(0)
	owners, err := Owners(context.Background(), db, []string{"mdcb-1", "gone", "unknown", "", "mdcb-1"})
	require.NoError(t, err)
	assert.Equal(t, int32(1), queries.Load(), "one query for the whole page")
	assert.Equal(t, map[string]Owner{
		"mdcb-1": {Label: "mdcb-eu-1", Live: true},
		"gone":   {Label: "dashboard", Live: false},
	}, owners)

	none, err := Owners(context.Background(), db, []string{"", ""})
	require.NoError(t, err)
	assert.Empty(t, none)
	assert.Equal(t, int32(1), queries.Load(), "no IDs, no query")
}
