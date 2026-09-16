//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"fmt"
	"io"
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

// writableFakeDashboard stores proxies and policies so registration can be
// exercised end to end over HTTP.
type writableFakeDashboard struct {
	srv      *httptest.Server
	mu       sync.Mutex
	mcps     []map[string]interface{}
	policies []map[string]interface{}
	bodies   []string
	seq      int
}

func newWritableFakeDashboard(t *testing.T, token string) *writableFakeDashboard {
	t.Helper()
	f := &writableFakeDashboard{}
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
		path := r.URL.Path
		body, _ := io.ReadAll(r.Body)
		if r.Method == "POST" || r.Method == "PUT" {
			f.bodies = append(f.bodies, string(body))
		}
		findMCP := func(id string) int {
			for i, m := range f.mcps {
				if m["x-tyk-api-gateway"].(map[string]interface{})["info"].(map[string]interface{})["id"] == id {
					return i
				}
			}
			return -1
		}
		switch {
		case r.Method == "GET" && path == "/api/schemas/apidefs/mcp":
			_, _ = w.Write([]byte(`{"definitions":{"x-tyk-mcp-server":{}}}`))
		case r.Method == "GET" && path == "/api/mcps":
			_ = enc.Encode(map[string]interface{}{"mcps": f.mcps, "pages": 1})
		case r.Method == "POST" && path == "/api/mcps":
			var doc map[string]interface{}
			if json.Unmarshal(body, &doc) != nil || doc["x-tyk-api-gateway"] == nil {
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"Status":"Error","Message":"invalid definition"}`))
				return
			}
			if r.URL.Query().Get("dryRun") == "true" {
				_ = enc.Encode(doc)
				return
			}
			f.seq++
			id := fmt.Sprintf("mcp-%d", f.seq)
			info := doc["x-tyk-api-gateway"].(map[string]interface{})["info"].(map[string]interface{})
			info["id"], info["orgId"] = id, "org-1"
			f.mcps = append(f.mcps, doc)
			_ = enc.Encode(map[string]interface{}{"Status": "OK", "Message": "created", "ID": id})
		case strings.HasPrefix(path, "/api/mcps/"):
			id := strings.TrimPrefix(path, "/api/mcps/")
			i := findMCP(id)
			if i < 0 {
				w.WriteHeader(404)
				_, _ = w.Write([]byte(`{"Status":"Error","Message":"not found"}`))
				return
			}
			switch r.Method {
			case "GET":
				_ = enc.Encode(f.mcps[i])
			case "PUT":
				var doc map[string]interface{}
				_ = json.Unmarshal(body, &doc)
				f.mcps[i] = doc
				_, _ = w.Write([]byte(`{"Status":"OK","Message":"updated"}`))
			case "DELETE":
				f.mcps = append(f.mcps[:i], f.mcps[i+1:]...)
				_, _ = w.Write([]byte(`{"Status":"OK","Message":"deleted"}`))
			}
		case r.Method == "GET" && path == "/api/portal/policies":
			_ = enc.Encode(map[string]interface{}{"Data": f.policies, "Pages": 1})
		case r.Method == "POST" && path == "/api/portal/policies":
			var doc map[string]interface{}
			_ = json.Unmarshal(body, &doc)
			f.seq++
			id := fmt.Sprintf("pol-%d", f.seq)
			doc["_id"] = id
			f.policies = append(f.policies, doc)
			_ = enc.Encode(map[string]interface{}{"Status": "OK", "Message": id, "Meta": nil})
		case strings.HasPrefix(path, "/api/portal/policies/"):
			id := strings.TrimPrefix(path, "/api/portal/policies/")
			for i, p := range f.policies {
				if p["_id"] == id {
					if r.Method == "GET" {
						_ = enc.Encode(p)
						return
					}
					var doc map[string]interface{}
					_ = json.Unmarshal(body, &doc)
					doc["_id"] = id
					f.policies[i] = doc
					_, _ = w.Write([]byte(`{"Status":"OK","Message":"updated"}`))
					return
				}
			}
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"Status":"Error","Message":"not found"}`))
		case r.Method == "GET" && path == "/api/apis":
			_, _ = w.Write([]byte(`{"apis":[{"api_definition":{"api_id":"api-orders","name":"Orders API","active":true,"is_oas":true,"proxy":{"listen_path":"/orders/"}}}],"pages":1}`))
		case r.Method == "GET" && path == "/api/apis/oas/api-orders":
			_, _ = w.Write([]byte(`{"openapi":"3.0.3","paths":{"/orders/{id}":{"get":{"operationId":"getOrder","summary":"Get order"}}}}`))
		case r.Method == "POST" && path == "/api/keys/preview":
			_, _ = w.Write([]byte(`{"key_id":"","data":{"org_id":"org-1"}}`))
		default:
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"Status":"Error","Message":"Not found"}`))
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func TestTykMCPEnterprise_RegistrationFlow(t *testing.T) {
	h := setupTykMCPEnterpriseAPI(t)
	dash := newWritableFakeDashboard(t, "user-key-abcd")
	r := h.api.router
	key := h.admin.APIKey

	input := map[string]interface{}{
		"name": "Prod", "dashboard_url": dash.srv.URL, "dashboard_access_token": "user-key-abcd",
		"declared_mode": "full", "allow_internal_host": true, "gateway_base_url": "https://gw.example.com",
		"known_gateway_tags": []map[string]string{{"tag": "edge-eu", "label": "EU edge"}},
	}
	w := apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections", input, key)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var conn models.TykConnectionResponse
	decodeWebhookJSON(t, w, &conn)
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/"+tykIDStr(conn.ID)+"/activate", nil, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	cid := tykIDStr(conn.ID)

	// Wizard helpers.
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/tyk-connections/"+cid+"/gateway-tags", nil, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var tags []tykmcp.GatewayTagOption
	decodeWebhookJSON(t, w, &tags)
	require.Len(t, tags, 1)
	assert.Equal(t, "EU edge", tags[0].Label)
	assert.False(t, tags[0].Verified)
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/tyk-connections/"+cid+"/apis", nil, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var apis []tykmcp.SourceAPI
	decodeWebhookJSON(t, w, &apis)
	require.Len(t, apis, 1)
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/tyk-connections/"+cid+"/apis/api-orders/operations", nil, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var ops []tykmcp.SourceOperation
	decodeWebhookJSON(t, w, &ops)
	require.Len(t, ops, 1)
	assert.Equal(t, "getOrder", ops[0].OperationID)

	// Dry run, then create. The upstream token reaches the Dashboard and
	// appears in no Studio response.
	reg := map[string]interface{}{
		"connection_id": conn.ID, "kind": "remote", "name": "Weather MCP", "listen_path": "/weather-mcp/",
		"upstream_url": "https://weather.example.com/mcp", "upstream_auth_token": "UPSTREAM-SECRET",
		"consumer_auth": "auth_token", "gateway_tags": []string{"edge-eu"}, "privacy_score": 20, "publish": true,
		"description": "Weather for agents",
	}
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/mcp-servers/register?dry_run=1", reg, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "UPSTREAM-SECRET")
	var preview tykmcp.RegisterPreview
	decodeWebhookJSON(t, w, &preview)
	assert.Equal(t, "https://gw.example.com/weather-mcp/mcp", preview.EndpointURL)
	assert.Contains(t, string(preview.Definition), `"***"`)

	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/mcp-servers/register", reg, key)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "UPSTREAM-SECRET")
	var created struct {
		Server models.MCPServerResponse `json:"server"`
	}
	decodeWebhookJSON(t, w, &created)
	srv := created.Server
	assert.Equal(t, "studio", srv.Origin)
	assert.Equal(t, "mcp-1", srv.TykAPIID)
	assert.True(t, srv.IsActive, "published in the same call")
	assert.Equal(t, []string{"edge-eu"}, srv.GatewayTags.Tags)
	require.GreaterOrEqual(t, len(dash.bodies), 2)
	assert.Contains(t, dash.bodies[len(dash.bodies)-1], "UPSTREAM-SECRET")

	// Validation errors are 400s with the reason.
	bad := map[string]interface{}{"connection_id": conn.ID, "name": "Bad", "upstream_url": "https://x.example.com", "listen_path": "/weather-mcp/", "gateway_tags": []string{"edge-eu"}}
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/mcp-servers/register", bad, key)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "already used")

	// Minimal policy creator: access + consumption, pinned in one go.
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/"+cid+"/policies", map[string]interface{}{"kind": "access", "name": "Weather access", "server_id": srv.ID, "pin": true}, key)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var access models.TykPolicyResponse
	decodeWebhookJSON(t, w, &access)
	assert.True(t, access.StudioManaged)
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/"+cid+"/policies", map[string]interface{}{"kind": "consumption", "name": "Gold", "server_id": srv.ID, "rate": 100, "per": 60, "pin": true}, key)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var gold models.TykPolicyResponse
	decodeWebhookJSON(t, w, &gold)
	w = apitest.PerformAuthRequest(r, "PATCH", "/api/v1/tyk-connections/"+cid+"/policies/"+gold.TykPolicyID, map[string]interface{}{"name": "Gold v2", "rate": 200, "per": 60}, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/mcp-servers/"+tykIDStr(srv.ID), nil, key)
	require.Equal(t, http.StatusOK, w.Code)
	var detail models.MCPServerResponse
	decodeWebhookJSON(t, w, &detail)
	require.Len(t, detail.Bundle, 2)
	assert.True(t, detail.Brokerable)
	assert.Equal(t, "Gold v2", detail.Bundle[1].Policy.Name)

	// Push an edit: masked secret restored, stale hash refused.
	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(detail.Definition), &doc))
	doc["x-tyk-api-gateway"].(map[string]interface{})["upstream"].(map[string]interface{})["url"] = "https://weather-v2.example.com/mcp"
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/mcp-servers/"+tykIDStr(srv.ID)+"/push?dry_run=1", map[string]interface{}{"definition": doc, "expected_hash": detail.DefinitionHash}, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/mcp-servers/"+tykIDStr(srv.ID)+"/push", map[string]interface{}{"definition": doc, "expected_hash": detail.DefinitionHash}, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "UPSTREAM-SECRET")
	assert.Contains(t, dash.bodies[len(dash.bodies)-1], "UPSTREAM-SECRET", "the live secret was spliced back")
	assert.NotContains(t, dash.bodies[len(dash.bodies)-1], "***")
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/mcp-servers/"+tykIDStr(srv.ID)+"/push", map[string]interface{}{"definition": doc, "expected_hash": detail.DefinitionHash}, key)
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())

	// Delete removes the proxy on the Dashboard.
	w = apitest.PerformAuthRequest(r, "DELETE", "/api/v1/mcp-servers/"+tykIDStr(srv.ID), nil, key)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	dash.mu.Lock()
	assert.Empty(t, dash.mcps)
	dash.mu.Unlock()
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/mcp-servers/"+tykIDStr(srv.ID), nil, key)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
