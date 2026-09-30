package api

import (
	"context"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/pkg/cluster"
)

// EdgeOwner is the Studio replica holding an edge's configuration stream:
// empty when no replica holds it (the edge is not connected). With several
// replicas (a Dashboard's Studio and headless control planes in MDCB), it
// tells operators which one serves the edge.
type EdgeOwner struct {
	OwnerNodeID string `json:"owner_node_id"`
	// OwnerLabel is the replica's label (cluster.NodeOptions.Label), empty
	// when it has none or is no longer registered.
	OwnerLabel string `json:"owner_label"`
	// OwnerLive: the replica's registration is fresh.
	OwnerLive bool `json:"owner_live"`
}

// withEdgeOwners fills in the owning replicas' labels and liveness, with one
// query for all of resps. It is best effort: without them the edges still
// list, with only the owner's node ID.
func (a *API) withEdgeOwners(ctx context.Context, resps []EdgeResponse) {
	ids := make([]string, 0, len(resps))
	for _, r := range resps {
		ids = append(ids, r.Attributes.OwnerNodeID)
	}
	owners, err := cluster.Owners(ctx, a.service.DB, ids)
	if err != nil {
		logger.Warnf("Listing edges without their replicas' labels: %v", err)
		return
	}
	for i := range resps {
		if o, ok := owners[resps[i].Attributes.OwnerNodeID]; ok {
			resps[i].Attributes.OwnerLabel = o.Label
			resps[i].Attributes.OwnerLive = o.Live
		}
	}
}
