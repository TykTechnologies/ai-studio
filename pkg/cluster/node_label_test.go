package cluster

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// A replica registers its label and whether it may lead; both survive its
// refreshes and show in the cluster status.
func TestNodeLabelAndLeaderEligibility(t *testing.T) {
	db := sqliteDB(t)
	require.NoError(t, db.AutoMigrate(&models.ClusterLease{}, &models.EdgeInstance{}, &models.EdgePushCommand{}))

	full, err := StartNode(db, "dash-1", "test", NodeOptions{Label: "dashboard"})
	require.NoError(t, err)
	defer full.Stop(context.Background())
	headless, err := StartNode(db, "mdcb-1", "test", NodeOptions{Label: "mdcb-eu-1", NeverLeads: true})
	require.NoError(t, err)
	defer headless.Stop(context.Background())
	require.NoError(t, headless.beat(), "a refresh keeps label and eligibility")

	st, err := Snapshot(context.Background(), db, "dash-1", nil, nil)
	require.NoError(t, err)
	byID := map[string]NodeStatus{}
	for _, n := range st.Nodes {
		byID[n.NodeID] = n
	}
	assert.Equal(t, "dashboard", byID["dash-1"].Label)
	assert.True(t, byID["dash-1"].LeaderEligible)
	assert.Equal(t, "mdcb-eu-1", byID["mdcb-1"].Label)
	assert.False(t, byID["mdcb-1"].LeaderEligible)
}

// Without options a node is leader-eligible and unlabelled, as before; a row
// written by an older replica (no eligibility recorded) counts as eligible.
func TestNodeDefaultsAndOlderRows(t *testing.T) {
	db := sqliteDB(t)
	require.NoError(t, db.AutoMigrate(&models.ClusterLease{}, &models.EdgeInstance{}, &models.EdgePushCommand{}))
	n, err := StartNode(db, "plain", "test")
	require.NoError(t, err)
	defer n.Stop(context.Background())
	require.NoError(t, db.Exec(`INSERT INTO cluster_nodes (node_id, hostname, version, started_at, last_seen) VALUES ('older', 'h', 'old', ?, ?)`,
		nowExpr(db), nowExpr(db)).Error)

	st, err := Snapshot(context.Background(), db, "plain", nil, nil)
	require.NoError(t, err)
	for _, s := range st.Nodes {
		assert.True(t, s.LeaderEligible, s.NodeID)
		assert.Empty(t, s.Label, s.NodeID)
	}
	assert.Len(t, st.Nodes, 2)
}

func TestNodeLabelValidation(t *testing.T) {
	db := sqliteDB(t)
	for _, bad := range []string{
		strings.Repeat("a", 65), "tab\there", "new\nline",
		// Characters that mean something to HTML, a shell or a query string.
		"<script>", `quote"d`, "it's", "a&b", "back`tick", "semi;colon",
	} {
		_, err := StartNode(db, "n", "test", NodeOptions{Label: bad})
		assert.Error(t, err, "%q", bad)
	}
	for _, good := range []string{"", "mdcb-eu-west-1", "Dashboard (primary)", "mdcb.host_2", "eu:west/1", "Zürich"} {
		n, err := StartNode(db, "n-"+good, "test", NodeOptions{Label: good})
		require.NoError(t, err, "%q", good)
		n.Stop(context.Background())
	}
}
