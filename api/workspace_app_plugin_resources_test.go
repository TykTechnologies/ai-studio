package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
)

// The App form hides the plugin resource pickers from a user without
// plugins:read (a Workspace-scoped Editor) and sends the App's stored
// plugin_resources back unchanged. That save must succeed and leave the
// attachments as they were (midsommar-clean-6bg5.74).

type appPluginResourceFixture struct {
	*workspaceFixture
	t      *testing.T
	engine *gin.Engine
	grants authz.Grants
	caller *models.User
	app    *models.App
	plugin *models.Plugin
}

func newAppPluginResourceFixture(t *testing.T) *appPluginResourceFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	f := &appPluginResourceFixture{workspaceFixture: newWorkspaceFixture(t), t: t}
	a := f.api

	f.caller = models.NewUser()
	f.caller.Name, f.caller.Email, f.caller.Password, f.caller.EmailVerified = "editor", "editor@example.com", "hash", true
	require.NoError(t, f.caller.Create(f.db))

	f.plugin = &models.Plugin{Name: "asset-catalog", Command: "/usr/bin/catalog", HookType: models.HookTypeResourceProvider,
		HookTypes: []string{models.HookTypeResourceProvider}, IsActive: true}
	require.NoError(t, f.db.Create(f.plugin).Error)
	require.NoError(t, a.service.RegisterPluginResourceTypes(f.plugin.ID, []models.PluginResourceType{
		{Slug: "vector-db", Name: "Vector DB"},
		{Slug: "prompt", Name: "Prompt"},
	}))

	f.app = &models.App{Name: "With resources", Description: "d", UserID: f.caller.ID, WorkspaceID: f.w1}
	require.NoError(t, f.db.Create(f.app).Error)
	for slug, ids := range map[string][]string{"vector-db": {"vdb-1", "vdb-2"}, "prompt": {"p-1"}} {
		prt, err := a.service.GetPluginResourceTypeByPluginAndSlug(f.plugin.ID, slug)
		require.NoError(t, err)
		for _, id := range ids {
			require.NoError(t, f.db.Create(&models.AppPluginResource{
				AppID: f.app.ID, PluginResourceTypeID: prt.ID, InstanceID: id, InstanceName: "name of " + id,
			}).Error)
		}
	}

	f.engine = gin.New()
	g := f.engine.Group("/api/v1", func(c *gin.Context) {
		c.Set("user", f.caller)
		grants := f.grants
		authz.WithContext(c, authz.NewGrantsContext(f.caller.ID, false, func() (authz.Grants, error) { return grants, nil }))
	}, a.requireRoutePermission())
	g.GET("/apps/:id", a.onObject(&models.App{}, authz.Read("apps"), appNotFound), a.getApp)
	g.PATCH("/apps/:id", a.updateApp)
	g.GET("/plugin-resource-types", a.listPluginResourceTypes)
	return f
}

func (f *appPluginResourceFixture) do(method, path string, body interface{}) *httptest.ResponseRecorder {
	f.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(f.t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	return w
}

// stored returns the App's attachments as "slug/instance" strings, sorted.
func (f *appPluginResourceFixture) stored() []string {
	f.t.Helper()
	var rows []models.AppPluginResource
	require.NoError(f.t, f.db.Preload("PluginResourceType").Where("app_id = ?", f.app.ID).Find(&rows).Error)
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.PluginResourceType.Slug+"/"+r.InstanceID)
	}
	sort.Strings(out)
	return out
}

type appWithPluginResources struct {
	Data struct {
		Attributes struct {
			Name            string                `json:"name"`
			Description     string                `json:"description"`
			UserID          uint                  `json:"user_id"`
			PluginResources []PluginResourceInput `json:"plugin_resources"`
		} `json:"attributes"`
	} `json:"data"`
}

func TestWorkspaceEditor_AppSaveKeepsPluginResources(t *testing.T) {
	f := newAppPluginResourceFixture(t)
	// A Workspace-scoped Editor: apps read and write in W1, nothing global,
	// no plugins permission anywhere.
	f.grants = scoped(f.w1, authz.Read("apps"), authz.Write("apps"))
	path := fmt.Sprintf("/api/v1/apps/%d", f.app.ID)
	before := []string{"prompt/p-1", "vector-db/vdb-1", "vector-db/vdb-2"}
	require.Equal(t, before, f.stored())

	// The pickers' data source is closed to this caller: that is why the
	// form hides them.
	require.Equal(t, http.StatusForbidden, f.do(http.MethodGet, "/api/v1/plugin-resource-types", nil).Code)

	// The form reads the App, which carries the attachments.
	w := f.do(http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got appWithPluginResources
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got.Data.Attributes.PluginResources, 2, "the App response carries its plugin resources for a Workspace reader")

	// The payload the form sends on save: the fields it holds, with
	// plugin_resources as loaded.
	save := func(name string, resources []PluginResourceInput) *httptest.ResponseRecorder {
		attrs := map[string]interface{}{
			"name":                name,
			"description":         got.Data.Attributes.Description,
			"user_id":             got.Data.Attributes.UserID,
			"llm_ids":             []uint{},
			"datasource_ids":      []uint{},
			"tool_ids":            []uint{},
			"model_router_ids":    []uint{},
			"semantic_router_ids": []uint{},
			"metadata":            map[string]interface{}{},
		}
		if resources != nil {
			attrs["plugin_resources"] = resources
		}
		return f.do(http.MethodPatch, path, map[string]interface{}{"data": map[string]interface{}{"type": "apps", "attributes": attrs}})
	}

	t.Run("save with the loaded plugin_resources", func(t *testing.T) {
		w := save("Renamed once", got.Data.Attributes.PluginResources)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, before, f.stored(), "attachments unchanged after the save")

		var after appWithPluginResources
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &after))
		assert.Equal(t, "Renamed once", after.Data.Attributes.Name)
		assert.Len(t, after.Data.Attributes.PluginResources, 2, "the response still lists them")
	})

	t.Run("save without a plugin_resources key", func(t *testing.T) {
		w := save("Renamed twice", nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, before, f.stored(), "an omitted key keeps the attachments")
	})

	t.Run("a second save round-trips what the first returned", func(t *testing.T) {
		w := f.do(http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, w.Code)
		var again appWithPluginResources
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &again))
		w = save("Renamed thrice", again.Data.Attributes.PluginResources)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, before, f.stored())
	})

	t.Run("an Editor of another Workspace cannot save it", func(t *testing.T) {
		f.grants = scoped(f.w2, authz.Read("apps"), authz.Write("apps"))
		w := save("Not mine", got.Data.Attributes.PluginResources)
		assert.NotEqual(t, http.StatusOK, w.Code)
		assert.Equal(t, before, f.stored())
	})
}
