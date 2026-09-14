package api

import (
	"encoding/json"
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GET /api/v1/sync/pending-changes through the router, and a global reload
// stamping last_push_at so the next preview is empty and /sync/status
// carries the stamp.

type pendingChangesResponse struct {
	Data services.PendingChanges `json:"data"`
}

func TestSyncPendingChanges_RouteAndReloadAll(t *testing.T) {
	f := setupLifecycleAPI(t)
	db := f.auth.Config.DB

	edge := &models.EdgeInstance{EdgeID: "edge-dev", Namespace: "default", Status: models.EdgeStatusConnected}
	require.NoError(t, db.Create(edge).Error)
	require.NoError(t, db.Create(&models.LLM{Name: "Mock GPT", Namespace: ""}).Error)
	require.NoError(t, db.Create(&models.App{Name: "Portal App", Namespace: "default"}).Error)

	get := func(query string) pendingChangesResponse {
		w := apitest.PerformAuthRequest(f.router, "GET", "/api/v1/sync/pending-changes"+query, nil, f.admin.APIKey)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp pendingChangesResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp
	}

	resp := get("?namespace=default")
	assert.Nil(t, resp.Data.Since)
	assert.Equal(t, 2, resp.Data.Total)
	names := map[string]string{}
	for _, c := range resp.Data.Changes {
		names[c.Type+":"+c.Name] = c.Change
	}
	assert.Equal(t, map[string]string{"llm:Mock GPT": "created", "app:Portal App": "created"}, names)

	// Non-admins may not read edges.
	w := apitest.PerformAuthRequest(f.router, "GET", "/api/v1/sync/pending-changes?namespace=default", nil, f.member.APIKey)
	assert.Equal(t, http.StatusForbidden, w.Code)
	// Bad namespace spelling is refused.
	w = apitest.PerformAuthRequest(f.router, "GET", "/api/v1/sync/pending-changes?namespace=bad%20ns", nil, f.admin.APIKey)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = apitest.PerformAuthRequest(f.router, "POST", "/api/v1/edges/reload-all", nil, f.admin.APIKey)
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())

	resp = get("?namespace=default")
	require.NotNil(t, resp.Data.Since, "the push is now the reference point")
	assert.Equal(t, 0, resp.Data.Total)

	w = apitest.PerformAuthRequest(f.router, "GET", "/api/v1/sync/status", nil, f.admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var status struct {
		Data []services.NamespaceSyncSummary `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
	require.Len(t, status.Data, 1)
	assert.Equal(t, "default", status.Data[0].Namespace)
	assert.NotNil(t, status.Data[0].LastPushAt)
}
