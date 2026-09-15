package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// accessViaApp returns a pointer for AccessGrantedViaAppDeclared fixtures.
func accessViaApp(v bool) *bool { return &v }

func TestValidatePortalDetailPath(t *testing.T) {
	for _, ok := range []string{"", "/portal/plugins/x", "/portal/plugins/asset-catalog#/assets/{id}", "  /portal/x  "} {
		assert.NoError(t, ValidatePortalDetailPath(ok), ok)
	}
	// Strict on purpose: a scheme anywhere (even in a query) is refused, since
	// the template is only ever a portal path.
	for _, bad := range []string{"https://evil.example/{id}", "//evil.example", "portal/relative", "javascript:alert(1)", "/portal/x y", "/portal/x?next=http://a"} {
		assert.ErrorIs(t, ValidatePortalDetailPath(bad), ErrInvalidPortalDetailPath, bad)
	}
}

// TestRegisterPluginResourceTypes_ResolvesAccessGrantedViaApp: the stored
// value is resolved from the declaration and the plugin's hook types, on
// create and on update, so neither existing plugin needs a change.
func TestRegisterPluginResourceTypes_ResolvesAccessGrantedViaApp(t *testing.T) {
	service, db := setupPluginResourceTest(t)

	proxying := &models.Plugin{Name: "mcp-registry", Command: "/usr/bin/mcp", HookType: models.HookTypeCustomEndpoint,
		HookTypes: []string{models.HookTypeCustomEndpoint, models.HookTypeStudioUI, models.HookTypeResourceProvider}, IsActive: true}
	require.NoError(t, db.Create(proxying).Error)
	catalog := createTestPlugin(t, db, "asset-catalog") // resource_provider only

	t.Run("undeclared follows the hook-type heuristic", func(t *testing.T) {
		require.NoError(t, service.RegisterPluginResourceTypes(proxying.ID, []models.PluginResourceType{{Slug: "mcp_servers", Name: "MCP Servers"}}))
		require.NoError(t, service.RegisterPluginResourceTypes(catalog.ID, []models.PluginResourceType{{Slug: "agent", Name: "Agent"}}))

		mcp, err := service.GetPluginResourceTypeByPluginAndSlug(proxying.ID, "mcp_servers")
		require.NoError(t, err)
		assert.True(t, mcp.AccessGrantedViaApp)
		assert.Nil(t, mcp.AccessGrantedViaAppDeclared)

		agent, err := service.GetPluginResourceTypeByPluginAndSlug(catalog.ID, "agent")
		require.NoError(t, err)
		assert.False(t, agent.AccessGrantedViaApp)
	})

	t.Run("explicit declaration wins on create and update", func(t *testing.T) {
		require.NoError(t, service.RegisterPluginResourceTypes(catalog.ID, []models.PluginResourceType{
			{Slug: "skill", Name: "Skill", AccessGrantedViaAppDeclared: accessViaApp(true), PortalDetailPath: " /portal/plugins/asset-catalog#/assets/{id} "},
		}))
		skill, err := service.GetPluginResourceTypeByPluginAndSlug(catalog.ID, "skill")
		require.NoError(t, err)
		assert.True(t, skill.AccessGrantedViaApp)
		assert.Equal(t, "/portal/plugins/asset-catalog#/assets/{id}", skill.PortalDetailPath, "path is trimmed")

		// Update branch: the proxying plugin opts one type out.
		require.NoError(t, service.RegisterPluginResourceTypes(proxying.ID, []models.PluginResourceType{
			{Slug: "mcp_servers", Name: "MCP Servers", AccessGrantedViaAppDeclared: accessViaApp(false)},
		}))
		mcp, err := service.GetPluginResourceTypeByPluginAndSlug(proxying.ID, "mcp_servers")
		require.NoError(t, err)
		assert.False(t, mcp.AccessGrantedViaApp)
		require.NotNil(t, mcp.AccessGrantedViaAppDeclared)
		assert.False(t, *mcp.AccessGrantedViaAppDeclared)

		// And back to undeclared: heuristic again.
		require.NoError(t, service.RegisterPluginResourceTypes(proxying.ID, []models.PluginResourceType{{Slug: "mcp_servers", Name: "MCP Servers"}}))
		mcp, err = service.GetPluginResourceTypeByPluginAndSlug(proxying.ID, "mcp_servers")
		require.NoError(t, err)
		assert.True(t, mcp.AccessGrantedViaApp)
		assert.Nil(t, mcp.AccessGrantedViaAppDeclared)
	})

	t.Run("rejects an off-site portal path", func(t *testing.T) {
		err := service.RegisterPluginResourceTypes(catalog.ID, []models.PluginResourceType{
			{Slug: "bad", Name: "Bad", PortalDetailPath: "https://evil.example/{id}"},
		})
		assert.ErrorIs(t, err, ErrInvalidPortalDetailPath)
		_, err = service.GetPluginResourceTypeByPluginAndSlug(catalog.ID, "bad")
		assert.Error(t, err, "nothing stored")
	})

	t.Run("unknown plugin is an error", func(t *testing.T) {
		err := service.RegisterPluginResourceTypes(999, []models.PluginResourceType{{Slug: "x", Name: "X"}})
		assert.Error(t, err)
	})
}

// TestAppPluginResources_NotAppGranted: binding a resource an App credential
// does not unlock is refused on create; on update, bindings the App already
// has survive and only new ones are refused.
func TestAppPluginResources_NotAppGranted(t *testing.T) {
	service, db := setupPluginResourceTest(t)
	plugin := createTestPlugin(t, db, "asset-catalog")
	require.NoError(t, service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{
		{Slug: "agent", Name: "Agent"}, // resolves false
		{Slug: "mcp_servers", Name: "MCP Servers", AccessGrantedViaAppDeclared: accessViaApp(true)},
	}))
	agentType, err := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "agent")
	require.NoError(t, err)
	user := createTestAppUser(t, service, "class@test.com", "Class User")

	t.Run("create refuses a non-app-granted instance and creates no app", func(t *testing.T) {
		var before int64
		require.NoError(t, db.Model(&models.App{}).Count(&before).Error)

		app, err := service.CreateAppWithResources("Agent App", "", user.ID, nil, nil, nil, nil, nil, nil,
			[]PluginResourceSelection{
				{PluginID: plugin.ID, ResourceTypeSlug: "mcp_servers", InstanceIDs: []string{"srv-1"}},
				{PluginID: plugin.ID, ResourceTypeSlug: "agent", InstanceIDs: []string{"ast_1"}},
			})
		assert.ErrorIs(t, err, ErrResourceNotAppGranted)
		assert.Contains(t, err.Error(), "ast_1")
		assert.Nil(t, app)

		var after int64
		require.NoError(t, db.Model(&models.App{}).Count(&after).Error)
		assert.Equal(t, before, after)
	})

	t.Run("create accepts app-granted instances", func(t *testing.T) {
		app, err := service.CreateAppWithResources("MCP App", "", user.ID, nil, nil, nil, nil, nil, nil,
			[]PluginResourceSelection{{PluginID: plugin.ID, ResourceTypeSlug: "mcp_servers", InstanceIDs: []string{"srv-1"}}})
		require.NoError(t, err)
		aprs, err := service.GetAppPluginResources(app.ID)
		require.NoError(t, err)
		assert.Len(t, aprs, 1)
	})

	t.Run("update keeps existing non-app-granted bindings, refuses new ones", func(t *testing.T) {
		app, err := service.CreateApp("Legacy App", "", user.ID, nil, nil, nil, nil, nil, nil)
		require.NoError(t, err)
		// A binding made before the type was classified.
		require.NoError(t, service.SetAppPluginResources(app.ID, agentType.ID, []string{"ast_old"}))

		// Resending the existing binding is fine.
		_, err = service.UpdateAppWithResources(app.ID, "Legacy App", "", user.ID, nil, nil, nil, nil, nil, nil,
			[]PluginResourceSelection{{PluginID: plugin.ID, ResourceTypeSlug: "agent", InstanceIDs: []string{"ast_old"}}})
		require.NoError(t, err)

		// Adding a new one is not.
		_, err = service.UpdateAppWithResources(app.ID, "Legacy App", "", user.ID, nil, nil, nil, nil, nil, nil,
			[]PluginResourceSelection{{PluginID: plugin.ID, ResourceTypeSlug: "agent", InstanceIDs: []string{"ast_old", "ast_new"}}})
		assert.ErrorIs(t, err, ErrResourceNotAppGranted)
		assert.Contains(t, err.Error(), "ast_new")

		// Omitting the type leaves the binding untouched.
		_, err = service.UpdateAppWithResources(app.ID, "Legacy App", "renamed", user.ID, nil, nil, nil, nil, nil, nil,
			[]PluginResourceSelection{{PluginID: plugin.ID, ResourceTypeSlug: "mcp_servers", InstanceIDs: []string{"srv-9"}}})
		require.NoError(t, err)
		aprs, err := service.GetAppPluginResources(app.ID)
		require.NoError(t, err)
		ids := map[string]bool{}
		for _, a := range aprs {
			ids[a.InstanceID] = true
		}
		assert.True(t, ids["ast_old"], "existing binding survives")
		assert.True(t, ids["srv-9"])
		assert.Len(t, aprs, 2)
	})
}
