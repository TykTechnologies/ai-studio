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

	// Shaped like a proxying provider (custom endpoints + resources), so its
	// resource types grant access through an App and can be attached.
	f.plugin = &models.Plugin{Name: "mcp-registry", Command: "/usr/bin/registry", HookType: models.HookTypeCustomEndpoint,
		HookTypes: []string{models.HookTypeCustomEndpoint, models.HookTypeResourceProvider}, IsActive: true}
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
	g.POST("/apps", a.createApp)
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

// Plugin resource instances are global and can carry upstream credentials
// (an MCP server's auth headers). Attaching one to an App needed only
// apps:write, so a Workspace-scoped Editor could attach any instance by
// guessing plugin id, type and instance id. Adding an instance that is not
// already on the App now needs plugins:read, the permission that lists them
// (midsommar-clean-6bg5.82). Keeping and removing stay with the App writer.
func TestAppPluginResources_AddingNeedsPluginsRead(t *testing.T) {
	f := newAppPluginResourceFixture(t)
	path := fmt.Sprintf("/api/v1/apps/%d", f.app.ID)
	before := []string{"prompt/p-1", "vector-db/vdb-1", "vector-db/vdb-2"}
	require.Equal(t, before, f.stored())

	editor := scoped(f.w1, authz.Read("apps"), authz.Write("apps"))
	withPluginsRead := authz.Grants{Global: authz.NewSet(authz.Read("plugins")),
		Workspaces: map[uint]authz.Set{f.w1: authz.NewSet(authz.Read("apps"), authz.Write("apps"))}}

	patch := func(g authz.Grants, resources ...PluginResourceInput) *httptest.ResponseRecorder {
		f.grants = g
		return f.do(http.MethodPatch, path, map[string]interface{}{"data": map[string]interface{}{"type": "apps",
			"attributes": map[string]interface{}{"name": "With resources", "user_id": f.caller.ID, "plugin_resources": resources}}})
	}
	vdb := func(ids ...string) PluginResourceInput {
		return PluginResourceInput{PluginID: f.plugin.ID, ResourceTypeSlug: "vector-db", InstanceIDs: ids}
	}
	denied := func(t *testing.T, w *httptest.ResponseRecorder) {
		t.Helper()
		require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "plugins", "the refusal names the missing permission")
	}

	t.Run("an Editor without plugins:read cannot add an instance", func(t *testing.T) {
		denied(t, patch(editor, vdb("vdb-1", "vdb-2", "guessed-id")))
		assert.Equal(t, before, f.stored(), "the App is unchanged")
	})

	t.Run("nor swap one for another", func(t *testing.T) {
		denied(t, patch(editor, vdb("guessed-id")))
		assert.Equal(t, before, f.stored())
	})

	t.Run("nor add one of a type the App has none of, or of a type that does not exist", func(t *testing.T) {
		require.NoError(t, f.api.service.RegisterPluginResourceTypes(f.plugin.ID, []models.PluginResourceType{
			{Slug: "vector-db", Name: "Vector DB"}, {Slug: "prompt", Name: "Prompt"}, {Slug: "mcp_servers", Name: "MCP Servers"},
		}))
		denied(t, patch(editor, PluginResourceInput{PluginID: f.plugin.ID, ResourceTypeSlug: "mcp_servers", InstanceIDs: []string{"srv-1"}}))
		// The same answer for a type that does not exist: no oracle.
		denied(t, patch(editor, PluginResourceInput{PluginID: f.plugin.ID, ResourceTypeSlug: "no-such-type", InstanceIDs: []string{"x"}}))
		denied(t, patch(editor, PluginResourceInput{PluginID: 9999, ResourceTypeSlug: "vector-db", InstanceIDs: []string{"vdb-1"}}))
		assert.Equal(t, before, f.stored())
	})

	t.Run("nor create an App with one", func(t *testing.T) {
		f.grants = editor
		w := f.do(http.MethodPost, "/api/v1/apps", map[string]interface{}{"data": map[string]interface{}{"type": "apps",
			"attributes": map[string]interface{}{"name": "New with a guess", "user_id": f.caller.ID, "workspace_id": f.w1,
				"plugin_resources": []PluginResourceInput{vdb("vdb-1")}}}})
		denied(t, w)
		var n int64
		require.NoError(t, f.db.Model(&models.App{}).Where("name = ?", "New with a guess").Count(&n).Error)
		assert.Zero(t, n, "no App is created")
	})

	t.Run("the Editor can still remove one", func(t *testing.T) {
		w := patch(editor, vdb("vdb-1"))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, []string{"prompt/p-1", "vector-db/vdb-1"}, f.stored())
	})

	t.Run("and cannot put it back", func(t *testing.T) {
		denied(t, patch(editor, vdb("vdb-1", "vdb-2")))
		assert.Equal(t, []string{"prompt/p-1", "vector-db/vdb-1"}, f.stored())
	})

	t.Run("a caller with plugins:read can add", func(t *testing.T) {
		w := patch(withPluginsRead, vdb("vdb-1", "vdb-2", "vdb-3"))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, []string{"prompt/p-1", "vector-db/vdb-1", "vector-db/vdb-2", "vector-db/vdb-3"}, f.stored())
	})

	t.Run("a global administrator is unaffected", func(t *testing.T) {
		w := patch(authz.GlobalGrants(authz.NewSet(authz.Read("apps"), authz.Write("apps"), authz.Read("plugins"))), vdb("vdb-1", "vdb-4"))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, []string{"prompt/p-1", "vector-db/vdb-1", "vector-db/vdb-4"}, f.stored())
	})

	t.Run("plugins:read alone does not let someone write another Workspace's App", func(t *testing.T) {
		w := patch(authz.Grants{Global: authz.NewSet(authz.Read("plugins")),
			Workspaces: map[uint]authz.Set{f.w2: authz.NewSet(authz.Read("apps"), authz.Write("apps"))}}, vdb("vdb-1", "vdb-9"))
		assert.NotEqual(t, http.StatusOK, w.Code)
		assert.Equal(t, []string{"prompt/p-1", "vector-db/vdb-1", "vector-db/vdb-4"}, f.stored())
	})
}
