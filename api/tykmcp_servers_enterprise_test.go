//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/tykmcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// richFakeDashboard serves MCP proxies and policies for the discovery tests.
type richFakeDashboard struct {
	srv      *httptest.Server
	mu       sync.Mutex
	mcps     []json.RawMessage
	policies []json.RawMessage
}

func newRichFakeDashboard(t *testing.T, token string) *richFakeDashboard {
	t.Helper()
	f := &richFakeDashboard{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != token {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"Status":"Error","Message":"Not authorised"}`))
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		enc := json.NewEncoder(w)
		switch r.Method + " " + r.URL.Path {
		case "GET /api/schemas/apidefs/mcp":
			_, _ = w.Write([]byte(`{"definitions":{"x-tyk-mcp-server":{}}}`))
		case "GET /api/mcps":
			_ = enc.Encode(map[string]interface{}{"mcps": f.mcps, "pages": 1})
		case "GET /api/portal/policies":
			_ = enc.Encode(map[string]interface{}{"Data": f.policies, "Pages": 1})
		case "GET /api/apis":
			_, _ = w.Write([]byte(`{"apis":[],"pages":1}`))
		case "POST /api/keys/preview":
			_, _ = w.Write([]byte(`{"key_id":"","data":{"org_id":"org-1"}}`))
		case "POST /api/mcps":
			_, _ = w.Write([]byte(`{"openapi":"3.0.3"}`))
		default:
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"Status":"Error","Message":"Not found"}`))
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

const weatherMCP = `{"openapi":"3.0.3","info":{"title":"Weather MCP proxy","version":"2025-11-25"},
"paths":{"/mcp":{"post":{"operationId":"mcpTransportPost","responses":{"200":{"description":"ok"}}}}},
"x-tyk-api-gateway":{"info":{"id":"api-weather","orgId":"org-1","name":"Weather MCP proxy","state":{"active":true}},
"server":{"listenPath":{"value":"/weather/","strip":true},"authentication":{"enabled":true,"securitySchemes":{"authToken":{"enabled":true}}},
"gatewayTags":{"enabled":true,"tags":["edge-eu"]}},
"upstream":{"url":"https://weather.example.com","authentication":{"enabled":true,"headers":[{"name":"X-Key","value":"UPSTREAM-SECRET"}]}},
"middleware":{"mcpTools":{"get-weather":{"allow":{"enabled":true}}}}}}`

const weatherACL = `{"_id":"pol-acl","name":"Weather access","org_id":"org-1","active":true,"partitions":{"acl":true},"access_rights":{"api-weather":{"api_id":"api-weather","versions":["Default"]}}}`
const goldPlan = `{"_id":"pol-gold","name":"Gold plan","org_id":"org-1","active":true,"partitions":{"rate_limit":true,"quota":true},"rate":100,"per":60}`

func TestTykMCPEnterprise_DiscoveryFlow(t *testing.T) {
	h := setupTykMCPEnterpriseAPI(t)
	dash := newRichFakeDashboard(t, "user-key-abcd")
	dash.mcps = []json.RawMessage{json.RawMessage(weatherMCP)}
	dash.policies = []json.RawMessage{json.RawMessage(weatherACL), json.RawMessage(goldPlan)}
	r := h.api.router
	key := h.admin.APIKey

	// Connection in broker mode with a per-tag gateway URL.
	input := map[string]interface{}{
		"name": "Prod", "dashboard_url": dash.srv.URL, "dashboard_access_token": "user-key-abcd",
		"declared_mode": "broker", "allow_internal_host": true, "gateway_base_url": "https://gw.example.com",
		"known_gateway_tags": []map[string]string{{"tag": "edge-eu", "label": "EU edge"}},
		"gateway_base_urls":  map[string]string{"edge-eu": "https://eu.gw.example.com"},
	}
	w := apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections", input, key)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var conn models.TykConnectionResponse
	decodeWebhookJSON(t, w, &conn)
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/"+tykIDStr(conn.ID)+"/activate", nil, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Inline sync.
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/"+tykIDStr(conn.ID)+"/sync?wait=true", nil, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var run models.MCPSyncRun
	decodeWebhookJSON(t, w, &run)
	assert.Equal(t, "ok", run.Status)
	assert.Equal(t, 1, run.ProxiesAdded)
	assert.Equal(t, 2, run.PoliciesSeen)

	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/tyk-connections/"+tykIDStr(conn.ID)+"/sync-runs", nil, key)
	require.Equal(t, http.StatusOK, w.Code)
	var runs []models.MCPSyncRun
	decodeWebhookJSON(t, w, &runs)
	assert.Len(t, runs, 1)

	// List and detail.
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/mcp-servers?connection_id="+tykIDStr(conn.ID)+"&q=weather", nil, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var list tykmcp.ServerList
	decodeWebhookJSON(t, w, &list)
	require.Len(t, list.Servers, 1)
	srv := list.Servers[0]
	assert.Equal(t, "api-weather", srv.TykAPIID)
	assert.Equal(t, "https://gw.example.com/weather/mcp", srv.EndpointURL)
	assert.Equal(t, "https://eu.gw.example.com/weather/mcp", srv.EndpointURLs["edge-eu"])
	assert.Equal(t, []string{"edge-eu"}, srv.GatewayTags.Tags)
	assert.False(t, srv.IsActive)

	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/mcp-servers/"+tykIDStr(srv.ID), nil, key)
	require.Equal(t, http.StatusOK, w.Code)
	var detail models.MCPServerResponse
	decodeWebhookJSON(t, w, &detail)
	assert.NotContains(t, w.Body.String(), "UPSTREAM-SECRET", "upstream credentials are masked in the stored definition")
	assert.Contains(t, detail.Definition, `"value":"***"`)
	assert.Empty(t, detail.Bundle)

	// Policies of the connection.
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/tyk-connections/"+tykIDStr(conn.ID)+"/policies?mcp_only=true", nil, key)
	require.Equal(t, http.StatusOK, w.Code)
	var pols []models.TykPolicyResponse
	decodeWebhookJSON(t, w, &pols)
	require.Len(t, pols, 1)
	assert.Equal(t, "pol-acl", pols[0].TykPolicyID)

	// Publish is refused without a privacy score.
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/mcp-servers/"+tykIDStr(srv.ID)+"/activate", nil, key)
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())

	// Patch presentation + score, then publish.
	patch := map[string]interface{}{"description": "Weather tools for agents", "privacy_score": 35, "tags": []string{"weather"}, "lock_version": detail.LockVersion}
	w = apitest.PerformAuthRequest(r, "PATCH", "/api/v1/mcp-servers/"+tykIDStr(srv.ID), patch, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/mcp-servers/"+tykIDStr(srv.ID)+"/activate", nil, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	decodeWebhookJSON(t, w, &detail)
	assert.True(t, detail.IsActive)

	// Bundle.
	bundle := map[string]interface{}{"pins": []map[string]string{{"tyk_policy_id": "pol-acl", "role": "access"}, {"tyk_policy_id": "pol-gold", "role": "consumption"}}}
	w = apitest.PerformAuthRequest(r, "PUT", "/api/v1/mcp-servers/"+tykIDStr(srv.ID)+"/bundle", bundle, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	decodeWebhookJSON(t, w, &detail)
	require.Len(t, detail.Bundle, 2)
	assert.True(t, detail.Brokerable)
	bad := map[string]interface{}{"pins": []map[string]string{{"tyk_policy_id": "pol-gold", "role": "access"}}}
	w = apitest.PerformAuthRequest(r, "PUT", "/api/v1/mcp-servers/"+tykIDStr(srv.ID)+"/bundle", bad, key)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())

	// Teams.
	grp := &models.Group{Name: "AI team"}
	require.NoError(t, h.api.service.DB.Create(grp).Error)
	w = apitest.PerformAuthRequest(r, "PUT", "/api/v1/mcp-servers/"+tykIDStr(srv.ID)+"/groups", map[string]interface{}{"group_ids": []uint{grp.ID}}, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	decodeWebhookJSON(t, w, &detail)
	assert.Equal(t, []uint{grp.ID}, detail.GroupIDs)

	// Deleting a live server is refused; unpublish works.
	w = apitest.PerformAuthRequest(r, "DELETE", "/api/v1/mcp-servers/"+tykIDStr(srv.ID), nil, key)
	assert.Equal(t, http.StatusConflict, w.Code)
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/mcp-servers/"+tykIDStr(srv.ID)+"/deactivate", nil, key)
	require.Equal(t, http.StatusOK, w.Code)

	// Bad filters.
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/mcp-servers?page_size=9999", nil, key)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
