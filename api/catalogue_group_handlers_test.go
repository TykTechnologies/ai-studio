package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GET /{catalogues|data-catalogues|tool-catalogues}/:id/groups lists the
// teams a catalogue is shared with, by name, with member counts; an empty
// array when none; 404 for an unknown catalogue.
func TestCatalogueGroupsEndpoints(t *testing.T) {
	api, db := setupTestAPI(t)

	cat := &models.Catalogue{Name: "Shared"}
	require.NoError(t, db.Create(cat).Error)
	lonely := &models.Catalogue{Name: "Lonely"}
	require.NoError(t, db.Create(lonely).Error)
	dcat := &models.DataCatalogue{Name: "Docs"}
	require.NoError(t, db.Create(dcat).Error)
	tcat := &models.ToolCatalogue{Name: "Tools"}
	require.NoError(t, db.Create(tcat).Error)

	platform := &models.Group{Name: "Platform"}
	require.NoError(t, db.Create(platform).Error)
	analytics := &models.Group{Name: "Analytics"}
	require.NoError(t, db.Create(analytics).Error)
	u := &models.User{Email: "one@example.com", Name: "One", Password: "x"}
	require.NoError(t, db.Create(u).Error)
	require.NoError(t, db.Model(platform).Association("Users").Append(u))

	require.NoError(t, db.Model(platform).Association("Catalogues").Append(cat))
	require.NoError(t, db.Model(analytics).Association("Catalogues").Append(cat))
	require.NoError(t, db.Model(platform).Association("DataCatalogues").Append(dcat))
	require.NoError(t, db.Model(platform).Association("ToolCatalogues").Append(tcat))

	get := func(t *testing.T, path string) CatalogueGroupsResponse {
		t.Helper()
		w := performRequest(api.router, "GET", path, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp CatalogueGroupsResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp
	}

	resp := get(t, "/api/v1/catalogues/"+idStr(cat.ID)+"/groups")
	require.Len(t, resp.Data, 2)
	assert.Equal(t, "Analytics", resp.Data[0].Name)
	assert.Equal(t, analytics.ID, resp.Data[0].ID)
	assert.Equal(t, int64(0), resp.Data[0].MemberCount)
	assert.Equal(t, "Platform", resp.Data[1].Name)
	assert.Equal(t, int64(1), resp.Data[1].MemberCount)

	// The exact wire shape the UI codes against.
	w := performRequest(api.router, "GET", "/api/v1/catalogues/"+idStr(lonely.ID)+"/groups", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"data":[]}`, w.Body.String(), "empty array, not null")

	resp = get(t, "/api/v1/data-catalogues/"+idStr(dcat.ID)+"/groups")
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "Platform", resp.Data[0].Name)

	resp = get(t, "/api/v1/tool-catalogues/"+idStr(tcat.ID)+"/groups")
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "Platform", resp.Data[0].Name)

	for _, path := range []string{
		"/api/v1/catalogues/9999/groups",
		"/api/v1/data-catalogues/9999/groups",
		"/api/v1/tool-catalogues/9999/groups",
	} {
		w := performRequest(api.router, "GET", path, nil)
		assert.Equal(t, http.StatusNotFound, w.Code, path)
	}
	w = performRequest(api.router, "GET", "/api/v1/catalogues/abc/groups", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// GET /apps/:id/dependents joins the dependents family: agents that run as
// the app, every other array present and empty.
func TestAppDependentsEndpoint(t *testing.T) {
	api, db := setupTestAPI(t)

	app := &models.App{Name: "Copilot", UserID: 1}
	require.NoError(t, db.Create(app).Error)
	plugin := &models.Plugin{Name: "agent-plugin", Command: "/bin/true", HookType: "agent"}
	require.NoError(t, db.Create(plugin).Error)
	agent := &models.AgentConfig{Name: "Helper", Slug: "helper", PluginID: plugin.ID, AppID: app.ID}
	require.NoError(t, db.Create(agent).Error)

	w := performRequest(api.router, "GET", "/api/v1/apps/"+idStr(app.ID)+"/dependents", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var raw map[string]map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	attrs := map[string]json.RawMessage{}
	require.NoError(t, json.Unmarshal(raw["data"]["attributes"], &attrs))
	for _, key := range []string{"apps", "catalogues", "llms", "tools", "datasources", "agents", "model_routers", "chats", "total"} {
		assert.Contains(t, attrs, key)
	}

	var response DependentsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "dependents", response.Data.Type)
	assert.Equal(t, idStr(app.ID), response.Data.ID)
	require.Len(t, response.Data.Attributes.Agents, 1)
	assert.Equal(t, "Helper", response.Data.Attributes.Agents[0].Name)
	assert.Equal(t, 1, response.Data.Attributes.Total)
	assert.Empty(t, response.Data.Attributes.Chats)

	w = performRequest(api.router, "GET", "/api/v1/apps/9999/dependents", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
