package api

import (
	"encoding/json"
	"net/http"
	"strconv"
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
	assert.Equal(t, services.PendingBaselinePush, resp.Data.Baseline)

	// The UI asks with "default"; the API doc says "global" or nothing. All
	// three are the namespace the edge registered under.
	for _, query := range []string{"?namespace=global", ""} {
		alias := get(query)
		require.NotNil(t, alias.Data.Since, query)
		assert.Equal(t, resp.Data.LastPushAt.Unix(), alias.Data.LastPushAt.Unix(), query)
		assert.Equal(t, "default", alias.Data.Namespace, query)
		assert.Equal(t, 0, alias.Data.Total, query)
	}

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

// ?workspace_id= adds the stain summary; an unknown or malformed id is 422
// invalid_workspace; without it the response carries no stain fields.
func TestSyncPendingChanges_WorkspaceStain(t *testing.T) {
	f := setupLifecycleAPI(t)
	db := f.auth.Config.DB
	def, err := models.EnsureDefaultWorkspace(db)
	require.NoError(t, err)
	w1 := &models.Workspace{Name: "W1"}
	require.NoError(t, db.Create(w1).Error)
	llm := &models.LLM{Name: "Default LLM", Namespace: "default"}
	require.NoError(t, db.Create(llm).Error)
	require.NoError(t, db.Model(llm).UpdateColumn("workspace_id", def.ID).Error)

	get := func(query string) *httptestResponse {
		w := apitest.PerformAuthRequest(f.router, "GET", "/api/v1/sync/pending-changes?namespace=default"+query, nil, f.admin.APIKey)
		return &httptestResponse{code: w.Code, body: w.Body.Bytes()}
	}
	decode := func(r *httptestResponse) map[string]json.RawMessage {
		t.Helper()
		require.Equal(t, http.StatusOK, r.code, string(r.body))
		var resp struct{ Data map[string]json.RawMessage }
		require.NoError(t, json.Unmarshal(r.body, &resp))
		return resp.Data
	}

	plain := decode(get(""))
	assert.NotContains(t, plain, "stained")
	assert.NotContains(t, plain, "outside_total")
	assert.Contains(t, plain, "by_workspace")

	own := decode(get("&workspace_id=" + uintString(def.ID)))
	assert.JSONEq(t, "false", string(own["stained"]))
	assert.JSONEq(t, "0", string(own["outside_total"]))

	other := decode(get("&workspace_id=" + uintString(w1.ID)))
	assert.JSONEq(t, "true", string(other["stained"]))
	assert.JSONEq(t, "1", string(other["outside_total"]))

	for _, q := range []string{"&workspace_id=999999", "&workspace_id=abc", "&workspace_id=0"} {
		r := get(q)
		assert.Equal(t, http.StatusUnprocessableEntity, r.code, q)
		assert.Equal(t, "invalid_workspace", decodeErrorCode(t, r.body), q)
	}
}

type httptestResponse struct {
	code int
	body []byte
}

func uintString(v uint) string { return strconv.FormatUint(uint64(v), 10) }
