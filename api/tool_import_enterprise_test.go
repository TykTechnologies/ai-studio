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

// The Tools import wizard reads a Dashboard through a saved Tyk connection:
// a slim connection list without disabled ones, the OAS APIs on it, and one
// masked definition. A pending connection (never activated) is enough.
func TestToolImportEnterprise_ConnectionsAPIsAndDocument(t *testing.T) {
	h := setupTykMCPEnterpriseAPI(t)
	dash := newWritableFakeDashboard(t, "user-key-abcd")
	r := h.api.router
	key := h.admin.APIKey

	create := func(name string) models.TykConnectionResponse {
		input := map[string]interface{}{
			"name": name, "dashboard_url": dash.srv.URL, "dashboard_access_token": "user-key-abcd",
			"declared_mode": "catalogue", "allow_internal_host": true,
		}
		w := apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections", input, key)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var conn models.TykConnectionResponse
		decodeWebhookJSON(t, w, &conn)
		return conn
	}
	pending := create("Pending")
	off := create("Switched off")
	w := apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/"+tykIDStr(off.ID)+"/activate", nil, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/"+tykIDStr(off.ID)+"/disable", map[string]interface{}{"reason": "retired"}, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Slim list: no token hint, no capabilities map, no disabled connection.
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/tools/import/tyk/connections", nil, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var list []ToolImportConnection
	decodeWebhookJSON(t, w, &list)
	require.Len(t, list, 1)
	assert.Equal(t, pending.ID, list[0].ID)
	assert.Equal(t, "Pending", list[0].Name)
	assert.Equal(t, models.TykConnectionPending, list[0].Status)
	assert.Equal(t, dash.srv.URL, list[0].DashboardURL)
	assert.NotContains(t, w.Body.String(), "token_hint")
	assert.NotContains(t, w.Body.String(), "abcd")

	cid := tykIDStr(pending.ID)
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/tools/import/tyk/connections/"+cid+"/apis", nil, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var apis []tykmcp.SourceAPI
	decodeWebhookJSON(t, w, &apis)
	require.Len(t, apis, 1)
	assert.Equal(t, "api-orders", apis[0].APIID)
	assert.Equal(t, "/orders/", apis[0].ListenPath)

	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/tools/import/tyk/connections/"+cid+"/apis/api-orders", nil, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "ORDERS-UPSTREAM-SECRET")
	var doc tykmcp.SourceAPIDocument
	decodeWebhookJSON(t, w, &doc)
	assert.Equal(t, "Orders API", doc.Name)
	assert.Equal(t, "/orders/", doc.ListenPath)
	assert.True(t, doc.Active)
	var def map[string]interface{}
	require.NoError(t, json.Unmarshal(doc.Definition, &def))
	assert.Equal(t, "3.0.3", def["openapi"])
	assert.Equal(t, "Order lookups", def["info"].(map[string]interface{})["description"])
	assert.Contains(t, string(doc.Definition), `"***"`)

	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/tools/import/tyk/connections/"+cid+"/apis/nope", nil, key)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/tools/import/tyk/connections/"+tykIDStr(off.ID+100)+"/apis", nil, key)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}
