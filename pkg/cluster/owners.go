package cluster

import (
	"context"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Owner is the replica holding an edge's stream, as operators see it.
type Owner struct {
	Label string
	// Live: the replica's registration is fresh. An edge whose owner is not
	// live reconnects to another replica shortly.
	Live bool
}

// Owners returns, for the node IDs that own edge streams (a page of
// edges), each node's label and liveness, in one query. IDs with no
// registration, and empty IDs, are left out.
func Owners(ctx context.Context, db *gorm.DB, nodeIDs []string) (map[string]Owner, error) {
	ids := make([]string, 0, len(nodeIDs))
	seen := map[string]bool{}
	for _, id := range nodeIDs {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	owners := map[string]Owner{}
	if len(ids) == 0 {
		return owners, nil
	}
	var rows []struct {
		NodeID string
		Label  string
		Live   bool
	}
	db = db.WithContext(ctx)
	err := db.Model(&models.ClusterNode{}).
		Select("node_id, COALESCE(label, '') AS label, last_seen > ? AS live", sinceExpr(db, LivenessWindow)).
		Where("node_id IN ?", ids).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("cluster: edge owners: %w", err)
	}
	for _, r := range rows {
		owners[r.NodeID] = Owner{Label: r.Label, Live: r.Live}
	}
	return owners, nil
}
