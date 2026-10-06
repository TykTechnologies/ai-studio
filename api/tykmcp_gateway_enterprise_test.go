//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/tykmcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeOSSGateway is one Tyk OSS Gateway node serving the Gateway API.
type fakeOSSGateway struct {
	srv  *httptest.Server
	mu   sync.Mutex
	mcps []json.RawMessage
	// other records any call outside the MCP-only allow-list.
	other []string
}

func newFakeOSSGateway(t *testing.T, secret string) *fakeOSSGateway {
	t.Helper()
	g := &fakeOSSGateway{}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		g.mu.Lock()
		defer g.mu.Unlock()
		if r.URL.Path == "/hello" {
			_, _ = w.Write([]byte(`{"status":"pass","version":"5.15.1"}`))
			return
		}
		if r.Header.Get("X-Tyk-Authorization") != secret {
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`{"status":"error","message":"Attempted administrative access with invalid or missing key!"}`))
			return
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/tyk/mcps":
			_ = json.NewEncoder(w).Encode(g.mcps)
		case r.Method == "POST" && r.URL.Path == "/tyk/mcps":
			_, _ = w.Write([]byte(`{"openapi":"3.0.3"}`))
		case r.Method == "POST" && r.URL.Path == "/tyk/keys/preview":
			_, _ = w.Write([]byte(`{}`))
		default:
			g.other = append(g.other, r.Method+" "+r.URL.Path)
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"status":"error","message":"not found"}`))
		}
	}))
	t.Cleanup(g.srv.Close)
	return g
}

func TestTykMCPEnterprise_GatewayConnection(t *testing.T) {
	h := setupTykMCPEnterpriseAPI(t)
	gw := newFakeOSSGateway(t, "gw-secret-1234")
	gw.mcps = []json.RawMessage{json.RawMessage(strings.Replace(weatherMCP, `"gatewayTags":{"enabled":true,"tags":["edge-eu"]}`, `"gatewayTags":{"enabled":false,"tags":[]}`, 1))}
	r := h.api.router
	key := h.admin.APIKey

	input := map[string]interface{}{
		"name": "OSS", "kind": "gateway", "dashboard_url": gw.srv.URL, "dashboard_access_token": "gw-secret-1234",
		"declared_mode": "broker", "allow_internal_host": true, "gateway_base_url": "https://mcp.example.com",
	}
	w := apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections", input, key)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var conn models.TykConnectionResponse
	decodeWebhookJSON(t, w, &conn)
	assert.Equal(t, "gateway", conn.Kind)
	assert.NotContains(t, w.Body.String(), "gw-secret-1234")
	id := tykIDStr(conn.ID)

	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/"+id+"/activate", nil, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	decodeWebhookJSON(t, w, &conn)
	assert.Equal(t, "broker", conn.EffectiveMode)

	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/"+id+"/sync?wait=true", nil, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/tyk-connections/"+id+"/nodes", nil, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var nodes []models.TykGatewayNodeResponse
	decodeWebhookJSON(t, w, &nodes)
	require.Len(t, nodes, 1)
	assert.True(t, nodes[0].Reachable)
	assert.Equal(t, "5.15.1", nodes[0].Version)

	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/mcp-servers?connection_id="+id, nil, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var list tykmcp.ServerList
	decodeWebhookJSON(t, w, &list)
	require.Len(t, list.Servers, 1)
	srv := list.Servers[0]
	assert.Equal(t, "gateway", srv.Origin)
	assert.True(t, srv.Brokerable)
	assert.Equal(t, "1/1", srv.GatewayCoverage)

	ka := map[string]interface{}{"allowed_tools": []string{"get-weather"}, "rate": 10, "per": 60}
	w = apitest.PerformAuthRequest(r, "PUT", "/api/v1/mcp-servers/"+tykIDStr(srv.ID)+"/key-access", ka, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var detail models.MCPServerResponse
	decodeWebhookJSON(t, w, &detail)
	require.NotNil(t, detail.KeyAccess)
	assert.Equal(t, []string{"get-weather"}, detail.KeyAccess.AllowedTools)

	w = apitest.PerformAuthRequest(r, "PUT", "/api/v1/mcp-servers/"+tykIDStr(srv.ID)+"/key-access", map[string]interface{}{"rate": 10}, key)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// Dashboard-only reads are refused rather than sent to the Gateway.
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/tyk-connections/"+id+"/apis", nil, key)
	assert.GreaterOrEqual(t, w.Code, 400)
	assert.Empty(t, gw.other, "nothing outside the MCP-only allow-list reached the Gateway")

	// Nodes of a Dashboard connection do not exist.
	dash := newRichFakeDashboard(t, "user-key-abcd")
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections", map[string]interface{}{
		"name": "Dash", "dashboard_url": dash.srv.URL, "dashboard_access_token": "user-key-abcd", "allow_internal_host": true,
	}, key)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var dconn models.TykConnectionResponse
	decodeWebhookJSON(t, w, &dconn)
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/tyk-connections/"+tykIDStr(dconn.ID)+"/nodes", nil, key)
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}
