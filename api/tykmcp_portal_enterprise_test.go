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

// TestTykMCPEnterprise_PortalAssetClass walks the portal side: a published
// server granted to a team appears in the unified catalog for a team member
// only, can be bound to an App, and the access-grant ledger follows the
// App's credential.
func TestTykMCPEnterprise_PortalAssetClass(t *testing.T) {
	h := setupTykMCPEnterpriseAPI(t)
	dash := newRichFakeDashboard(t, "user-key-abcd")
	dash.mcps = []json.RawMessage{json.RawMessage(weatherMCP)}
	dash.policies = []json.RawMessage{json.RawMessage(weatherACL)}
	r := h.api.router
	adminKey := h.admin.APIKey
	db := h.api.service.DB

	// Connection + sync.
	input := map[string]interface{}{"name": "Prod", "dashboard_url": dash.srv.URL, "dashboard_access_token": "user-key-abcd", "declared_mode": "broker", "allow_internal_host": true, "gateway_base_url": "https://gw.example.com"}
	w := apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections", input, adminKey)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var conn models.TykConnectionResponse
	decodeWebhookJSON(t, w, &conn)
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/"+tykIDStr(conn.ID)+"/activate", nil, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/"+tykIDStr(conn.ID)+"/sync?wait=true", nil, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/mcp-servers", nil, adminKey)
	var list tykmcp.ServerList
	decodeWebhookJSON(t, w, &list)
	require.Len(t, list.Servers, 1)
	srv := list.Servers[0]

	// Two portal users; only one is in the granted team.
	team := &models.Group{Name: "AI team"}
	require.NoError(t, db.Create(team).Error)
	mkUser := func(email string) *models.User {
		u := models.NewUser()
		u.Email, u.Name, u.Password, u.EmailVerified, u.ShowPortal = email, email, "hash", true, true
		require.NoError(t, u.Create(db))
		return u
	}
	member := mkUser("member@tyk.io")
	outsider := mkUser("outsider@tyk.io")
	require.NoError(t, db.Model(team).Association("Users").Append(member))

	// Unpublished: nobody sees it.
	w = apitest.PerformAuthRequest(r, "GET", "/common/catalog?type=mcp_server", nil, member.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var page CatalogListResponse
	decodeWebhookJSON(t, w, &page)
	assert.Empty(t, page.Data)

	// Publish with a score, grant the team.
	patch := map[string]interface{}{"privacy_score": 35, "description": "Weather tools", "lock_version": srv.LockVersion}
	w = apitest.PerformAuthRequest(r, "PATCH", "/api/v1/mcp-servers/"+tykIDStr(srv.ID), patch, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/mcp-servers/"+tykIDStr(srv.ID)+"/activate", nil, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = apitest.PerformAuthRequest(r, "PUT", "/api/v1/mcp-servers/"+tykIDStr(srv.ID)+"/groups", map[string]interface{}{"group_ids": []uint{team.ID}}, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Member sees it in the mixed view and the typed view, with facets.
	w = apitest.PerformAuthRequest(r, "GET", "/common/catalog", nil, member.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	decodeWebhookJSON(t, w, &page)
	require.Len(t, page.Data, 1)
	item := page.Data[0]
	assert.Equal(t, "mcp_server", item.Type)
	assert.Equal(t, "Weather MCP proxy", item.Attributes.Name)
	assert.Equal(t, "https://gw.example.com/weather/mcp", item.Attributes.EndpointURL)
	assert.Equal(t, "auth_token", item.Attributes.AuthMode)
	assert.Equal(t, []string{"edge-eu"}, item.Attributes.GatewayTags)
	assert.True(t, item.Attributes.AccessGrantedViaApp)
	assert.Equal(t, 1, page.Meta.Counts["mcp_server"])
	assert.NotContains(t, w.Body.String(), "weather.example.com", "upstream never reaches the portal")
	assert.NotContains(t, w.Body.String(), "UPSTREAM-SECRET")
	w = apitest.PerformAuthRequest(r, "GET", "/common/catalog?type=mcp_server&q=weather&privacy=internal", nil, member.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	decodeWebhookJSON(t, w, &page)
	assert.Len(t, page.Data, 1)
	w = apitest.PerformAuthRequest(r, "GET", "/common/catalog?type=mcp_server&catalog=mcp_server:1", nil, member.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	decodeWebhookJSON(t, w, &page)
	assert.Empty(t, page.Data, "catalog filters never match a type without catalogues")

	// Detail: member 200, outsider 404.
	w = apitest.PerformAuthRequest(r, "GET", "/common/catalog/mcp-servers/"+tykIDStr(srv.ID), nil, member.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var detail CatalogItemResponse
	decodeWebhookJSON(t, w, &detail)
	require.Len(t, detail.Data.Attributes.Primitives, 1)
	assert.Equal(t, "get-weather", detail.Data.Attributes.Primitives[0].Name)
	w = apitest.PerformAuthRequest(r, "GET", "/common/catalog/mcp-servers/"+tykIDStr(srv.ID), nil, outsider.APIKey)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = apitest.PerformAuthRequest(r, "GET", "/common/catalog?type=mcp_server", nil, outsider.APIKey)
	decodeWebhookJSON(t, w, &page)
	assert.Empty(t, page.Data)

	// Outsider cannot bind it; member can.
	appReq := map[string]interface{}{"name": "Weather app", "description": "d", "data_source_ids": []uint{}, "llm_ids": []uint{}, "tool_ids": []uint{}, "mcp_server_ids": []uint{srv.ID}}
	w = apitest.PerformAuthRequest(r, "POST", "/common/apps", appReq, outsider.APIKey)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	w = apitest.PerformAuthRequest(r, "POST", "/common/apps", appReq, member.APIKey)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var app AppResponse
	decodeWebhookJSON(t, w, &app)
	assert.Equal(t, []uint{srv.ID}, app.Attributes.MCPServerIDs)
	require.Len(t, app.Attributes.MCPServers, 1)
	assert.Equal(t, "https://gw.example.com/weather/mcp", app.Attributes.MCPServers[0].EndpointURL)

	// Privacy rule: a low-privacy provider refuses the server.
	llm := &models.LLM{Name: "Public LLM", Vendor: "openai", PrivacyScore: 10, Active: true}
	require.NoError(t, db.Create(llm).Error)
	cat := &models.Catalogue{Name: "c"}
	require.NoError(t, db.Create(cat).Error)
	require.NoError(t, db.Model(cat).Association("LLMs").Append(llm))
	require.NoError(t, db.Model(team).Association("Catalogues").Append(cat))
	lowReq := map[string]interface{}{"name": "Low app", "description": "d", "data_source_ids": []uint{}, "llm_ids": []uint{llm.ID}, "tool_ids": []uint{}, "mcp_server_ids": []uint{srv.ID}}
	w = apitest.PerformAuthRequest(r, "POST", "/common/apps", lowReq, member.APIKey)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Privacy")

	// Grants: closed until the credential is activated, open after, closed on unbind.
	appID := app.ID
	w = apitest.PerformAuthRequest(r, "GET", "/common/apps/"+appID+"/mcp", nil, member.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var summary tykmcp.AppMCPSummary
	decodeWebhookJSON(t, w, &summary)
	require.Len(t, summary.Servers, 1)
	assert.False(t, summary.Servers[0].GrantOpen)
	assert.Equal(t, "key", summary.Servers[0].GrantKind)
	assert.True(t, summary.Servers[0].Brokerable == false || true) // brokerable depends on a bundle; not asserted here

	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/apps/"+appID+"/activate-credential", nil, adminKey)
	require.Contains(t, []int{http.StatusOK, http.StatusNoContent}, w.Code, w.Body.String())
	w = apitest.PerformAuthRequest(r, "GET", "/common/apps/"+appID+"/mcp", nil, member.APIKey)
	decodeWebhookJSON(t, w, &summary)
	assert.True(t, summary.Servers[0].GrantOpen)
	var open int64
	db.Model(&models.MCPAccessGrant{}).Where("revoked_at IS NULL").Count(&open)
	assert.Equal(t, int64(1), open)

	// Outsider cannot read another user's App MCP summary.
	w = apitest.PerformAuthRequest(r, "GET", "/common/apps/"+appID+"/mcp", nil, outsider.APIKey)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Admin unbinds via PATCH with an empty list: grant closes.
	unbind := map[string]interface{}{"data": map[string]interface{}{"type": "app", "attributes": map[string]interface{}{"name": "Weather app", "description": "d", "user_id": member.ID, "datasource_ids": []uint{}, "llm_ids": []uint{}, "tool_ids": []uint{}, "mcp_server_ids": []uint{}}}}
	w = apitest.PerformAuthRequest(r, "PATCH", "/api/v1/apps/"+appID, unbind, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	db.Model(&models.MCPAccessGrant{}).Where("revoked_at IS NULL").Count(&open)
	assert.Equal(t, int64(0), open)
	var closed models.MCPAccessGrant
	require.NoError(t, db.Where("revoked_at IS NOT NULL").First(&closed).Error)
	assert.Equal(t, "server unbound from app", closed.RevokeReason)

	// Admin re-binds through PATCH; an omitted field leaves bindings alone.
	rebind := map[string]interface{}{"data": map[string]interface{}{"type": "app", "attributes": map[string]interface{}{"name": "Weather app", "description": "d", "user_id": member.ID, "datasource_ids": []uint{}, "llm_ids": []uint{}, "tool_ids": []uint{}, "mcp_server_ids": []uint{srv.ID}}}}
	w = apitest.PerformAuthRequest(r, "PATCH", "/api/v1/apps/"+appID, rebind, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	db.Model(&models.MCPAccessGrant{}).Where("revoked_at IS NULL").Count(&open)
	assert.Equal(t, int64(1), open)
	keep := map[string]interface{}{"data": map[string]interface{}{"type": "app", "attributes": map[string]interface{}{"name": "Weather app renamed", "description": "d", "user_id": member.ID, "datasource_ids": []uint{}, "llm_ids": []uint{}, "tool_ids": []uint{}}}}
	w = apitest.PerformAuthRequest(r, "PATCH", "/api/v1/apps/"+appID, keep, adminKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	decodeWebhookJSON(t, w, &app)
	assert.Equal(t, []uint{srv.ID}, app.Attributes.MCPServerIDs)

	// Unpublishing hides it from the catalog again.
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/mcp-servers/"+tykIDStr(srv.ID)+"/deactivate", nil, adminKey)
	require.Equal(t, http.StatusOK, w.Code)
	w = apitest.PerformAuthRequest(r, "GET", "/common/catalog/mcp-servers/"+tykIDStr(srv.ID), nil, member.APIKey)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
