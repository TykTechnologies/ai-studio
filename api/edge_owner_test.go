//go:build !enterprise

package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// The edges list and detail name the replica holding each edge's stream:
// its node ID, label and liveness (operators with Studio replicas in the
// Dashboard and in MDCB see which one serves an edge).
func TestEdgesShowTheirOwningReplica(t *testing.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	api := NewAPI(service, true, apitest.SetupTestAuthService(db, service), apitest.SetupTestAuthConfig(db, service), nil, emptyFile, nil)

	// UTC, as replicas write it (the database's clock): SQLite compares the
	// stored text.
	now := time.Now().UTC()
	require.NoError(t, db.Create(&models.ClusterNode{NodeID: "mdcb-node", Label: "mdcb-eu-1", StartedAt: now, LastSeen: now}).Error)
	require.NoError(t, db.Create(&models.ClusterNode{NodeID: "old-node", Label: "dashboard", StartedAt: now.Add(-2 * time.Hour), LastSeen: now.Add(-time.Hour)}).Error)
	held := createTestEdge(t, service, "held-edge", "")
	require.NoError(t, db.Model(held).Update("owner_node_id", "mdcb-node").Error)
	stale := createTestEdge(t, service, "stale-edge", "")
	require.NoError(t, db.Model(stale).Update("owner_node_id", "old-node").Error)
	createTestEdge(t, service, "free-edge", "")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/edges", api.listEdges)
	r.GET("/api/v1/edges/:edge_id", api.getEdge)

	w := apitest.PerformRequest(r, "GET", "/api/v1/edges", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var list EdgeListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	owners := map[string]EdgeOwner{}
	for _, e := range list.Data {
		owners[e.Attributes.EdgeID] = e.Attributes.EdgeOwner
	}
	assert.Equal(t, EdgeOwner{OwnerNodeID: "mdcb-node", OwnerLabel: "mdcb-eu-1", OwnerLive: true}, owners["held-edge"])
	assert.Equal(t, EdgeOwner{OwnerNodeID: "old-node", OwnerLabel: "dashboard", OwnerLive: false}, owners["stale-edge"])
	assert.Equal(t, EdgeOwner{}, owners["free-edge"])

	w = apitest.PerformRequest(r, "GET", "/api/v1/edges/held-edge", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var detail struct {
		Attributes map[string]interface{} `json:"attributes"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &detail))
	assert.Equal(t, "mdcb-eu-1", detail.Attributes["owner_label"], "the owner's fields sit with the edge's other attributes")
	assert.Equal(t, "mdcb-node", detail.Attributes["owner_node_id"])
	assert.Equal(t, true, detail.Attributes["owner_live"])
}
