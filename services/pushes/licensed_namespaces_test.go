package pushes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/entitlements"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// licensedSet licenses the default namespace and the listed ones, with the
// scope expressed in SQL as the enterprise engine does.
type licensedSet struct {
	entitlements.Service
	namespaces []string
}

func (l licensedSet) LicensedNamespaceScope(q *gorm.DB) *gorm.DB {
	return q.Where("namespace IN ?", append(models.NamespaceAliases(models.DefaultNamespace), l.namespaces...))
}

func licensedOnly(ns ...string) Options {
	return Options{Entitlements: licensedSet{Service: entitlements.NewCommunityService(), namespaces: ns}}
}

func targetIDs(res *Result) []string {
	var ids []string
	for _, t := range res.Targets {
		ids = append(ids, t.EdgeID)
	}
	return ids
}

// "Push all" reaches the default namespace and the licensed namespace only;
// edges in unlicensed namespaces are listed as skipped, with a warning.
func TestPushSkipsUnlicensedNamespaces(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		r := newReplica(t, db, "node-a", licensedOnly("team-a"))
		addEdge(t, db, "e-default", "default", models.EdgeStatusConnected, "node-a")
		addEdge(t, db, "e-legacy", "", models.EdgeStatusConnected, "node-a")
		addEdge(t, db, "e-a", "team-a", models.EdgeStatusConnected, "node-a")
		addEdge(t, db, "e-b", "team-b", models.EdgeStatusConnected, "node-a")
		addEdge(t, db, "e-c", "team-c", models.EdgeStatusRegistered, "")

		for i := 0; i < 3; i++ {
			res := push(t, r.c, Request{Scope: ScopeAll})
			assert.Equal(t, []string{"e-a", "e-default", "e-legacy"}, targetIDs(res))
			require.Len(t, res.Skipped, 2)
			assert.Equal(t, "e-b", res.Skipped[0].EdgeID)
			assert.Equal(t, unlicensedReason, res.Skipped[0].Reason)
			assert.Equal(t, "e-c", res.Skipped[1].EdgeID)
			assert.Equal(t, 2, res.SkippedTotal)
			assert.Contains(t, res.Warnings, "2 edge(s) were left out because their namespace is not licensed.")
			var cmds int64
			require.NoError(t, db.Model(&models.EdgePushCommand{}).Where("operation_id = ? AND edge_id IN ?", res.Operation.OperationID, []string{"e-b", "e-c"}).Count(&cmds).Error)
			assert.Zero(t, cmds, "no command for an unlicensed edge")
		}

		_, err := r.c.Push(context.Background(), Request{Scope: ScopeNamespace, Namespace: "team-b", InitiatedBy: "admin"})
		require.ErrorIs(t, err, ErrNoTargets)
		assert.Contains(t, err.Error(), "not licensed")

		_, err = r.c.Push(context.Background(), Request{Scope: ScopeEdge, EdgeIDs: []string{"e-b"}, InitiatedBy: "admin"})
		require.ErrorIs(t, err, ErrNoTargets)

		res := push(t, r.c, Request{Scope: ScopeEdge, EdgeIDs: []string{"e-a", "e-b"}})
		assert.Equal(t, []string{"e-a"}, targetIDs(res))
		require.Len(t, res.Skipped, 1)
		assert.Equal(t, unlicensedReason, res.Skipped[0].Reason)
	})
}

// Community Edition: every namespace is a target, as before.
func TestPushCommunityTargetsEveryNamespace(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		r := newReplica(t, db, "node-a", Options{Entitlements: entitlements.NewCommunityService()})
		addEdge(t, db, "e-a", "team-a", models.EdgeStatusConnected, "node-a")
		addEdge(t, db, "e-b", "team-b", models.EdgeStatusConnected, "node-a")
		res := push(t, r.c, Request{Scope: ScopeAll})
		assert.Equal(t, []string{"e-a", "e-b"}, targetIDs(res))
		assert.Empty(t, res.Skipped)
		assert.Empty(t, res.Warnings)
	})
}
