//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pushTestAPI struct {
	api    *API
	router *gin.Engine
}

func setupPushTestAPI(t *testing.T) *pushTestAPI {
	t.Helper()
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	cfg := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	a := NewAPI(service, true, authService, cfg, nil, emptyFile, nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user", &models.User{Email: "admin@example.com"}); c.Next() })
	v1 := r.Group("/api/v1")
	v1.POST("/edges/:edge_id/reload", a.triggerEdgeReload)
	v1.POST("/edges/reload-all", a.reloadAllEdges)
	v1.GET("/edges/reload-operations", a.listReloadOperations)
	v1.GET("/reload-operations/:operation_id/status", a.getReloadOperationStatus)
	v1.POST("/namespaces/:namespace/reload", a.triggerNamespaceReload)
	return &pushTestAPI{api: a, router: r}
}

func (p *pushTestAPI) edge(t *testing.T, id, ns, status string) {
	t.Helper()
	now := time.Now()
	require.NoError(t, p.api.service.DB.Create(&models.EdgeInstance{EdgeID: id, Namespace: ns, Status: status, LastHeartbeat: &now}).Error)
}

type pushAttrs struct {
	OperationID string         `json:"operation_id"`
	Scope       string         `json:"scope"`
	Namespace   string         `json:"target_namespace"`
	TargetEdges []string       `json:"target_edges"`
	Status      string         `json:"status"`
	Progress    int            `json:"progress"`
	Message     string         `json:"message"`
	Counts      map[string]int `json:"counts"`
	Warnings    []string       `json:"warnings"`
	Skipped     []struct {
		EdgeID string `json:"edge_id"`
		Reason string `json:"reason"`
	} `json:"skipped"`
	Edges []struct {
		EdgeID      string `json:"edge_id"`
		Status      string `json:"status"`
		Phase       string `json:"phase"`
		Message     string `json:"message"`
		Warning     string `json:"warning"`
		Attempts    int    `json:"attempts"`
		MaxAttempts int    `json:"max_attempts"`
		History     []struct {
			Outcome string `json:"outcome"`
		} `json:"history"`
		Reachable     *bool  `json:"reachable"`
		WaitingReason string `json:"waiting_reason"`
	} `json:"edges"`
}

func (p *pushTestAPI) do(t *testing.T, method, path string, want int) pushAttrs {
	t.Helper()
	w := apitest.PerformRequest(p.router, method, path, nil)
	require.Equal(t, want, w.Code, w.Body.String())
	var resp struct {
		Data struct {
			Type       string    `json:"type"`
			ID         string    `json:"id"`
			Attributes pushAttrs `json:"attributes"`
		} `json:"data"`
	}
	if want < 300 {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "reload-operations", resp.Data.Type)
		assert.Equal(t, resp.Data.ID, resp.Data.Attributes.OperationID)
	}
	return resp.Data.Attributes
}

// Simulates an edge's replica having sent the push, then the edge's answer.
func (p *pushTestAPI) answer(t *testing.T, op, edge string, phase pb.ReloadPhase, msg string) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, p.api.service.DB.Model(&models.EdgePushCommand{}).Where("operation_id = ? AND edge_id = ?", op, edge).
		Updates(map[string]interface{}{"status": models.PushCommandSent, "attempts": 1, "claimed_by": "node-x", "sent_at": now}).Error)
	p.api.service.NamespaceService.Pushes().HandleReloadResponse(&pb.ConfigurationReloadResponse{
		OperationId: op, EdgeId: edge, Phase: phase, Success: phase != pb.ReloadPhase_FAILED, Message: msg})
}

func TestPushAPI_NamespacePushAndPerEdgeStatus(t *testing.T) {
	p := setupPushTestAPI(t)
	p.edge(t, "edge-a", "ns1", models.EdgeStatusConnected)
	p.edge(t, "edge-b", "ns1", models.EdgeStatusRegistered)
	p.edge(t, "edge-c", "ns2", models.EdgeStatusConnected)

	started := p.do(t, "POST", "/api/v1/namespaces/ns1/reload", http.StatusAccepted)
	assert.Equal(t, "namespace", started.Scope)
	assert.Equal(t, "ns1", started.Namespace)
	assert.ElementsMatch(t, []string{"edge-a", "edge-b"}, started.TargetEdges)
	op := started.OperationID

	st := p.do(t, "GET", "/api/v1/reload-operations/"+op+"/status", http.StatusOK)
	assert.Equal(t, models.PushOperationInProgress, st.Status)
	assert.Equal(t, 0, st.Progress)
	require.Len(t, st.Edges, 2)
	for _, e := range st.Edges {
		assert.Equal(t, models.PushCommandPending, e.Status)
		require.NotNil(t, e.Reachable, "a waiting edge says whether it can be reached")
		assert.False(t, *e.Reachable)
		assert.NotEmpty(t, e.WaitingReason)
		assert.Equal(t, 3, e.MaxAttempts)
	}

	p.answer(t, op, "edge-a", pb.ReloadPhase_READY, "Configuration reloaded")
	p.answer(t, op, "edge-b", pb.ReloadPhase_FAILED, "Failed to update SQLite: disk full")

	st = p.do(t, "GET", "/api/v1/reload-operations/"+op+"/status", http.StatusOK)
	assert.Equal(t, models.PushOperationPartiallyFailed, st.Status)
	assert.Equal(t, 100, st.Progress)
	assert.Equal(t, map[string]int{models.PushCommandSucceeded: 1, models.PushCommandFailed: 1}, st.Counts)
	assert.Contains(t, st.Message, "1 updated, 1 failed")
	byEdge := map[string]int{}
	for i, e := range st.Edges {
		byEdge[e.EdgeID] = i
	}
	failed := st.Edges[byEdge["edge-b"]]
	assert.Equal(t, models.PushCommandFailed, failed.Status)
	assert.Equal(t, "Failed to update SQLite: disk full", failed.Message)
	assert.Equal(t, "FAILED", failed.Phase)
	assert.Nil(t, failed.Reachable, "settled edges carry no reachability")
	assert.Equal(t, models.PushCommandSucceeded, st.Edges[byEdge["edge-a"]].Status)

	// The listing agrees.
	w := apitest.PerformRequest(p.router, "GET", "/api/v1/edges/reload-operations", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"partially_failed"`)
}

func TestPushAPI_ReadyWithStaleChecksumReportsAWarning(t *testing.T) {
	p := setupPushTestAPI(t)
	p.edge(t, "edge-a", "default", models.EdgeStatusConnected)
	require.NoError(t, (&models.NamespaceSyncStatus{Namespace: "default", ExpectedChecksum: "new"}).Upsert(p.api.service.DB))
	require.NoError(t, p.api.service.DB.Model(&models.EdgeInstance{}).Where("edge_id = ?", "edge-a").Update("loaded_checksum", "old").Error)

	op := p.do(t, "POST", "/api/v1/edges/edge-a/reload", http.StatusAccepted).OperationID
	p.answer(t, op, "edge-a", pb.ReloadPhase_READY, "")

	st := p.do(t, "GET", "/api/v1/reload-operations/"+op+"/status", http.StatusOK)
	assert.Equal(t, models.PushOperationSucceededWarning, st.Status)
	require.Len(t, st.Warnings, 1)
	assert.Contains(t, st.Warnings[0], "edge-a: ")
	assert.NotEmpty(t, st.Edges[0].Warning)
}

func TestPushAPI_ReloadAllIsOneOperation(t *testing.T) {
	p := setupPushTestAPI(t)
	p.edge(t, "edge-a", "ns1", models.EdgeStatusConnected)
	p.edge(t, "edge-b", "ns2", models.EdgeStatusConnected)
	p.edge(t, "edge-gone", "ns3", models.EdgeStatusDisconnected)
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, p.api.service.DB.Model(&models.EdgeInstance{}).Where("edge_id = ?", "edge-gone").Update("last_heartbeat", old).Error)

	all := p.do(t, "POST", "/api/v1/edges/reload-all", http.StatusAccepted)
	assert.Equal(t, "all", all.Scope)
	assert.ElementsMatch(t, []string{"edge-a", "edge-b"}, all.TargetEdges)
	require.Len(t, all.Skipped, 1, "long-offline edges are reported, not silently dropped")
	assert.Equal(t, "edge-gone", all.Skipped[0].EdgeID)
	assert.Contains(t, all.Skipped[0].Reason, "offline since")
	assert.Len(t, all.Warnings, 2)
}

func TestPushAPI_Errors(t *testing.T) {
	p := setupPushTestAPI(t)
	p.edge(t, "edge-a", "ns1", models.EdgeStatusConnected)

	p.do(t, "POST", "/api/v1/edges/nope/reload", http.StatusNotFound)
	p.do(t, "POST", "/api/v1/namespaces/empty/reload", http.StatusConflict)
	p.do(t, "GET", "/api/v1/reload-operations/push-unknown/status", http.StatusNotFound)
	p.do(t, "GET", "/api/v1/reload-operations/bad%20id/status", http.StatusBadRequest)

	// No coordinator (standalone Studio): refused, never faked.
	p.api.service.NamespaceService.SetPushes(nil)
	w := apitest.PerformRequest(p.router, "POST", "/api/v1/namespaces/ns1/reload", nil)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "not available")
	w = apitest.PerformRequest(p.router, "GET", "/api/v1/edges/reload-operations", nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

// Clients written against v2.2 read reload-all's {message, operations,
// operations_count} under "data"; they are still there, next to the one
// operation (M2).
func TestPushAPI_ReloadAllKeepsTheV22Fields(t *testing.T) {
	p := setupPushTestAPI(t)
	p.edge(t, "edge-a", "ns1", models.EdgeStatusConnected)
	p.edge(t, "edge-b", "ns2", models.EdgeStatusConnected)
	p.edge(t, "edge-c", "ns2", models.EdgeStatusConnected)

	w := apitest.PerformRequest(p.router, "POST", "/api/v1/edges/reload-all", nil)
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	var resp struct {
		Data struct {
			ID              string `json:"id"`
			Message         string `json:"message"`
			OperationsCount int    `json:"operations_count"`
			Operations      []struct {
				OperationID string `json:"operation_id"`
				Namespace   string `json:"namespace"`
				Status      string `json:"status"`
			} `json:"operations"`
			Attributes pushAttrs `json:"attributes"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Global reload triggered for all namespaces", resp.Data.Message)
	assert.Equal(t, 2, resp.Data.OperationsCount)
	require.Len(t, resp.Data.Operations, 2)
	for i, ns := range []string{"ns1", "ns2"} {
		assert.Equal(t, ns, resp.Data.Operations[i].Namespace)
		assert.Equal(t, resp.Data.ID, resp.Data.Operations[i].OperationID, "one operation for every namespace")
		assert.Equal(t, models.PushOperationInProgress, resp.Data.Operations[i].Status)
	}
	assert.Equal(t, "all", resp.Data.Attributes.Scope)
}
