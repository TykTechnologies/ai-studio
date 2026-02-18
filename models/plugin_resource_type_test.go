package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPluginResourceType_CRUD(t *testing.T) {
	db := setupTestDB(t)

	// Create a Plugin first (FK dependency)
	plugin := &Plugin{
		Name:     "test-plugin",
		Command:  "/usr/bin/test",
		HookType: HookTypeResourceProvider,
		IsActive: true,
	}
	err := db.Create(plugin).Error
	assert.NoError(t, err)

	t.Run("create resource type", func(t *testing.T) {
		prt := &PluginResourceType{
			PluginID:            plugin.ID,
			Slug:                "mcp_servers",
			Name:                "MCP Servers",
			Description:         "Model Context Protocol servers",
			HasPrivacyScore:     true,
			SupportsSubmissions: false,
			FormComponentTag:    "mcp-selector",
			FormComponentEntry:  "ui/webc/selector.js",
			IsActive:            true,
		}
		err := prt.Create(db)
		assert.NoError(t, err)
		assert.NotZero(t, prt.ID)
	})

	t.Run("get by plugin and slug", func(t *testing.T) {
		prt := &PluginResourceType{}
		err := prt.GetByPluginAndSlug(db, plugin.ID, "mcp_servers")
		assert.NoError(t, err)
		assert.Equal(t, "MCP Servers", prt.Name)
		assert.True(t, prt.HasPrivacyScore)
		assert.Equal(t, "mcp-selector", prt.FormComponentTag)
	})

	t.Run("get by ID with plugin preload", func(t *testing.T) {
		existing := &PluginResourceType{}
		existing.GetByPluginAndSlug(db, plugin.ID, "mcp_servers")

		prt := &PluginResourceType{}
		err := prt.Get(db, existing.ID)
		assert.NoError(t, err)
		assert.NotNil(t, prt.Plugin)
		assert.Equal(t, "test-plugin", prt.Plugin.Name)
	})

	t.Run("update resource type", func(t *testing.T) {
		prt := &PluginResourceType{}
		prt.GetByPluginAndSlug(db, plugin.ID, "mcp_servers")

		prt.Name = "MCP Servers (Updated)"
		prt.SupportsSubmissions = true
		err := prt.Update(db)
		assert.NoError(t, err)

		refreshed := &PluginResourceType{}
		refreshed.GetByPluginAndSlug(db, plugin.ID, "mcp_servers")
		assert.Equal(t, "MCP Servers (Updated)", refreshed.Name)
		assert.True(t, refreshed.SupportsSubmissions)
	})

	t.Run("get all active", func(t *testing.T) {
		// Add a second type
		prt2 := &PluginResourceType{
			PluginID: plugin.ID,
			Slug:     "vector_stores",
			Name:     "Vector Stores",
			IsActive: true,
		}
		prt2.Create(db)

		// Add an inactive type (must update after create since GORM default is true)
		prt3 := &PluginResourceType{
			PluginID: plugin.ID,
			Slug:     "inactive_type",
			Name:     "Inactive",
			IsActive: true,
		}
		prt3.Create(db)
		db.Model(prt3).Update("is_active", false)

		var types PluginResourceTypes
		err := types.GetAllActive(db)
		assert.NoError(t, err)
		assert.Len(t, types, 2) // mcp_servers + vector_stores, not inactive
	})

	t.Run("get by plugin", func(t *testing.T) {
		var types PluginResourceTypes
		err := types.GetByPlugin(db, plugin.ID)
		assert.NoError(t, err)
		assert.Len(t, types, 3) // all three including inactive
	})

	t.Run("unique constraint on plugin_id + slug", func(t *testing.T) {
		duplicate := &PluginResourceType{
			PluginID: plugin.ID,
			Slug:     "mcp_servers", // already exists
			Name:     "Duplicate",
			IsActive: true,
		}
		err := duplicate.Create(db)
		assert.Error(t, err) // should fail on unique index
	})

	t.Run("delete resource type", func(t *testing.T) {
		prt := &PluginResourceType{}
		prt.GetByPluginAndSlug(db, plugin.ID, "vector_stores")

		err := prt.Delete(db)
		assert.NoError(t, err)
	})
}

func TestAppPluginResource_Queries(t *testing.T) {
	db := setupTestDB(t)

	// Setup: plugin, resource type, app
	plugin := &Plugin{Name: "p1", Command: "/test", HookType: HookTypeResourceProvider, IsActive: true}
	db.Create(plugin)

	prt := &PluginResourceType{PluginID: plugin.ID, Slug: "servers", Name: "Servers", IsActive: true}
	prt.Create(db)

	app := &App{Name: "test-app", UserID: 1}
	app.Create(db)

	t.Run("create and query associations", func(t *testing.T) {
		apr1 := &AppPluginResource{AppID: app.ID, PluginResourceTypeID: prt.ID, InstanceID: "srv-1"}
		db.Create(apr1)
		apr2 := &AppPluginResource{AppID: app.ID, PluginResourceTypeID: prt.ID, InstanceID: "srv-2"}
		db.Create(apr2)

		var aprs AppPluginResources
		err := aprs.GetByApp(db, app.ID)
		assert.NoError(t, err)
		assert.Len(t, aprs, 2)
		// Verify preload works
		assert.NotNil(t, aprs[0].PluginResourceType)
		assert.Equal(t, "Servers", aprs[0].PluginResourceType.Name)
	})

	t.Run("delete by type", func(t *testing.T) {
		err := DeleteAppPluginResourcesByType(db, app.ID, prt.ID)
		assert.NoError(t, err)

		var aprs AppPluginResources
		aprs.GetByApp(db, app.ID)
		assert.Len(t, aprs, 0)
	})

	t.Run("delete all by app", func(t *testing.T) {
		// Re-add
		db.Create(&AppPluginResource{AppID: app.ID, PluginResourceTypeID: prt.ID, InstanceID: "srv-a"})

		err := DeleteAppPluginResources(db, app.ID)
		assert.NoError(t, err)

		var aprs AppPluginResources
		aprs.GetByApp(db, app.ID)
		assert.Len(t, aprs, 0)
	})
}

func TestGroupPluginResource_AccessControl(t *testing.T) {
	db := setupTestDB(t)

	plugin := &Plugin{Name: "p1", Command: "/test", HookType: HookTypeResourceProvider, IsActive: true}
	db.Create(plugin)
	prt := &PluginResourceType{PluginID: plugin.ID, Slug: "servers", Name: "Servers", IsActive: true}
	prt.Create(db)

	group := &Group{Name: "Engineering"}
	db.Create(group)

	user := &User{Email: "eng@test.com", Name: "Engineer", Password: "pass"}
	db.Create(user)
	db.Exec("INSERT INTO user_groups (user_id, group_id) VALUES (?, ?)", user.ID, group.ID)

	t.Run("assign instances to group", func(t *testing.T) {
		db.Create(&GroupPluginResource{GroupID: group.ID, PluginResourceTypeID: prt.ID, InstanceID: "srv-1"})
		db.Create(&GroupPluginResource{GroupID: group.ID, PluginResourceTypeID: prt.ID, InstanceID: "srv-2"})

		var gprs GroupPluginResources
		err := gprs.GetByGroup(db, group.ID)
		assert.NoError(t, err)
		assert.Len(t, gprs, 2)
	})

	t.Run("get accessible instances for user", func(t *testing.T) {
		ids, err := GetAccessiblePluginResourceInstanceIDs(db, user.ID, prt.ID)
		assert.NoError(t, err)
		assert.Len(t, ids, 2)
		assert.Contains(t, ids, "srv-1")
		assert.Contains(t, ids, "srv-2")
	})

	t.Run("user not in group gets nothing", func(t *testing.T) {
		outsider := &User{Email: "out@test.com", Name: "Outsider", Password: "pass"}
		db.Create(outsider)

		ids, err := GetAccessiblePluginResourceInstanceIDs(db, outsider.ID, prt.ID)
		assert.NoError(t, err)
		assert.Len(t, ids, 0)
	})

	t.Run("multiple groups deduplicate instances", func(t *testing.T) {
		group2 := &Group{Name: "Platform"}
		db.Create(group2)
		db.Exec("INSERT INTO user_groups (user_id, group_id) VALUES (?, ?)", user.ID, group2.ID)

		// group2 has srv-2 (overlap) and srv-3 (new)
		db.Create(&GroupPluginResource{GroupID: group2.ID, PluginResourceTypeID: prt.ID, InstanceID: "srv-2"})
		db.Create(&GroupPluginResource{GroupID: group2.ID, PluginResourceTypeID: prt.ID, InstanceID: "srv-3"})

		ids, err := GetAccessiblePluginResourceInstanceIDs(db, user.ID, prt.ID)
		assert.NoError(t, err)
		assert.Len(t, ids, 3) // srv-1, srv-2, srv-3 (deduplicated)
	})

	t.Run("get all accessible plugin resources for user", func(t *testing.T) {
		results, err := GetAllAccessiblePluginResources(db, user.ID)
		assert.NoError(t, err)
		assert.True(t, len(results) >= 3) // at least the 3 unique instances
	})

	t.Run("delete by group and type", func(t *testing.T) {
		err := DeleteGroupPluginResourcesByType(db, group.ID, prt.ID)
		assert.NoError(t, err)

		var gprs GroupPluginResources
		gprs.GetByGroup(db, group.ID)
		assert.Len(t, gprs, 0)
	})
}
