//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/tykmcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTykMCPEnterprise_SubmissionFlow(t *testing.T) {
	h := setupTykMCPEnterpriseAPI(t)
	dash := newWritableFakeDashboard(t, "user-key-abcd")
	r := h.api.router
	adminKey := h.admin.APIKey
	db := h.api.service.DB

	connect := func(name, mode string) models.TykConnectionResponse {
		input := map[string]interface{}{
			"name": name, "dashboard_url": dash.srv.URL, "dashboard_access_token": "user-key-abcd",
			"declared_mode": mode, "allow_internal_host": true, "gateway_base_url": "https://gw.example.com",
			"known_gateway_tags": []map[string]string{{"tag": "edge-eu", "label": "EU edge"}},
		}
		w := apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections", input, adminKey)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var conn models.TykConnectionResponse
		decodeWebhookJSON(t, w, &conn)
		w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/"+tykIDStr(conn.ID)+"/activate", nil, adminKey)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		return conn
	}
	full := connect("Full", "full")
	cat := connect("Catalogue", "catalogue")

	member := models.NewUser()
	member.Email, member.Name, member.Password, member.EmailVerified, member.ShowPortal = "member@tyk.io", "Member", "hash", true, true
	require.NoError(t, member.Create(db))

	// The portal form learns which connections accept submissions.
	w := apitest.PerformAuthRequest(r, "GET", "/common/mcp/connections", nil, member.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var conns []tykmcp.SubmissionConnection
	decodeWebhookJSON(t, w, &conns)
	require.Len(t, conns, 2)

	submit := func(connID uint, name, path string) uint {
		body := map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{
			"resource_type": "mcp_server", "status": "submitted",
			"resource_payload": map[string]interface{}{
				"name": name, "description": "Weather for agents", "kind": "remote", "connection_id": connID,
				"upstream_url": "https://weather.example.com/mcp", "upstream_auth_header_name": "X-Token", "upstream_auth_token": "UPSTREAM-SECRET",
				"consumer_auth": "auth_token", "suggested_listen_path": path, "gateway_tags": []string{"edge-eu"},
			},
			"suggested_privacy": 30, "privacy_justification": "public weather data only", "primary_contact": "member@tyk.io",
		}}}
		w := apitest.PerformAuthRequest(r, "POST", "/common/submissions", body, member.APIKey)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		assert.NotContains(t, w.Body.String(), "UPSTREAM-SECRET", "the upstream credential is redacted for the submitter")
		assert.Contains(t, w.Body.String(), "[redacted]")
		var out struct {
			Data struct {
				ID uint `json:"id"`
			} `json:"data"`
		}
		decodeWebhookJSON(t, w, &out)
		return out.Data.ID
	}

	// A payload the connection cannot take is refused at the door.
	bad := map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{
		"resource_type": "mcp_server", "status": "draft",
		"resource_payload":  map[string]interface{}{"name": "Bad", "description": "d", "kind": "remote", "connection_id": full.ID, "upstream_url": "https://x.example.com", "gateway_tags": []string{"edge-us"}},
		"suggested_privacy": 10, "privacy_justification": "j", "primary_contact": "member@tyk.io",
	}}}
	w = apitest.PerformAuthRequest(r, "POST", "/common/submissions", bad, member.APIKey)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "not known")

	// --- Full-mode connection: approval creates the proxy. ---
	subID := submit(full.ID, "Weather MCP", "/weather-mcp/")
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/submissions/"+tykIDStr(subID)+"/test", nil, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "validated by the Tyk Dashboard")
	assert.NotContains(t, w.Body.String(), "UPSTREAM-SECRET")
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/submissions/"+tykIDStr(subID), nil, adminKey)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "UPSTREAM-SECRET", "redacted for reviewers too")

	approve := map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{"final_privacy_score": 35, "review_notes": "ok", "gateway_tags": []string{}, "publish": true}}}
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/submissions/"+tykIDStr(subID)+"/approve", approve, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var approved struct {
		Data struct {
			Status             string `json:"status"`
			ResourceID         *uint  `json:"resource_id"`
			ExternalResourceID string `json:"external_resource_id"`
		} `json:"data"`
	}
	decodeWebhookJSON(t, w, &approved)
	assert.Equal(t, "approved", approved.Data.Status)
	require.NotNil(t, approved.Data.ResourceID)
	assert.Equal(t, "mcp-1", approved.Data.ExternalResourceID)
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/mcp-servers/"+tykIDStr(*approved.Data.ResourceID), nil, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var srv models.MCPServerResponse
	decodeWebhookJSON(t, w, &srv)
	assert.Equal(t, "submission", srv.Origin)
	assert.True(t, srv.CommunitySubmitted)
	assert.Equal(t, member.ID, srv.OwnerUserID)
	assert.Equal(t, 35, *srv.PrivacyScore)
	assert.True(t, srv.IsActive, "published by the reviewer")
	assert.Empty(t, srv.GatewayTags.Tags, "the reviewer cleared the deployment target")
	assert.Contains(t, dash.bodies[len(dash.bodies)-1], "UPSTREAM-SECRET", "the real credential reached the Dashboard")

	// --- Catalogue connection: approval records a handoff. ---
	sub2 := submit(cat.ID, "Tickets MCP", "/tickets-mcp/")
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/submissions/"+tykIDStr(sub2)+"/test", nil, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"skipped":true`)
	approve = map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{"final_privacy_score": 20, "review_notes": "handoff"}}}
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/submissions/"+tykIDStr(sub2)+"/approve", approve, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	decodeWebhookJSON(t, w, &approved)
	require.NotNil(t, approved.Data.ResourceID)
	assert.Empty(t, approved.Data.ExternalResourceID)
	pendingID := *approved.Data.ResourceID
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/mcp-servers/"+tykIDStr(pendingID), nil, adminKey)
	require.Equal(t, http.StatusOK, w.Code)
	decodeWebhookJSON(t, w, &srv)
	assert.Equal(t, "pending_platform", srv.DashboardState)

	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/mcp-servers/"+tykIDStr(pendingID)+"/handoff", nil, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "UPSTREAM-SECRET")
	var pkg tykmcp.HandoffPackage
	decodeWebhookJSON(t, w, &pkg)
	assert.Equal(t, "member@tyk.io", pkg.Submitter.Email)
	assert.Contains(t, string(pkg.Definition), "/tickets-mcp/")
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/mcp-servers/"+tykIDStr(pendingID)+"/handoff?include_secrets=true", nil, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "UPSTREAM-SECRET")
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))

	// The platform team creates it by hand; a sync imports it; the admin links it.
	dash.mu.Lock()
	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(pkg.Definition, &doc))
	info := doc["x-tyk-api-gateway"].(map[string]interface{})["info"].(map[string]interface{})
	info["id"], info["orgId"] = "api-tickets-platform", "org-1"
	dash.mcps = append(dash.mcps, doc)
	dash.mu.Unlock()
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/"+tykIDStr(cat.ID)+"/sync?wait=true", nil, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/mcp-servers/"+tykIDStr(pendingID)+"/handoff", nil, adminKey)
	require.Equal(t, http.StatusOK, w.Code)
	decodeWebhookJSON(t, w, &pkg)
	require.Len(t, pkg.Candidates, 1)
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/mcp-servers/"+tykIDStr(pendingID)+"/link", map[string]string{"tyk_api_id": "api-tickets-platform"}, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	decodeWebhookJSON(t, w, &srv)
	assert.Equal(t, "api-tickets-platform", srv.TykAPIID)
	assert.Equal(t, "submission", srv.Origin)
	assert.Equal(t, member.ID, srv.OwnerUserID)
	assert.Equal(t, 20, *srv.PrivacyScore)
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/submissions/"+tykIDStr(sub2), nil, adminKey)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"external_resource_id":"api-tickets-platform"`)
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/mcp-servers/"+tykIDStr(pendingID), nil, adminKey)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
