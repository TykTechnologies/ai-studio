package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPluginResourceTest(t *testing.T) (*Service, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	service := NewService(db)
	return service, db
}

// createTestPlugin creates a Plugin record for testing
func createTestPlugin(t *testing.T, db *gorm.DB, name string) *models.Plugin {
	plugin := &models.Plugin{
		Name:     name,
		Command:  "/usr/bin/test-plugin",
		HookType: models.HookTypeResourceProvider,
		IsActive: true,
	}
	err := db.Create(plugin).Error
	assert.NoError(t, err)
	return plugin
}

// --- Resource Type Registration Tests ---

func TestRegisterPluginResourceTypes(t *testing.T) {
	service, db := setupPluginResourceTest(t)
	plugin := createTestPlugin(t, db, "test-plugin")

	t.Run("register new resource types", func(t *testing.T) {
		types := []models.PluginResourceType{
			{
				Slug:            "mcp_servers",
				Name:            "MCP Servers",
				Description:     "Model Context Protocol servers",
				HasPrivacyScore: true,
			},
			{
				Slug:                "vector_stores",
				Name:                "Vector Stores",
				SupportsSubmissions: true,
			},
		}

		err := service.RegisterPluginResourceTypes(plugin.ID, types)
		assert.NoError(t, err)

		// Verify both were created
		result, err := service.GetPluginResourceTypes()
		assert.NoError(t, err)
		assert.Len(t, result, 2)
	})

	t.Run("upsert existing resource types", func(t *testing.T) {
		// Re-register with updated name
		types := []models.PluginResourceType{
			{
				Slug:            "mcp_servers",
				Name:            "MCP Servers (Updated)",
				HasPrivacyScore: true,
			},
		}

		err := service.RegisterPluginResourceTypes(plugin.ID, types)
		assert.NoError(t, err)

		prt, err := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "mcp_servers")
		assert.NoError(t, err)
		assert.Equal(t, "MCP Servers (Updated)", prt.Name)
	})

	t.Run("deactivate resource types", func(t *testing.T) {
		err := service.DeactivatePluginResourceTypes(plugin.ID)
		assert.NoError(t, err)

		// GetPluginResourceTypes returns active only
		result, err := service.GetPluginResourceTypes()
		assert.NoError(t, err)
		assert.Len(t, result, 0)
	})
}

func TestGetPluginResourceTypeByPluginAndSlug(t *testing.T) {
	service, db := setupPluginResourceTest(t)
	plugin := createTestPlugin(t, db, "test-plugin")

	types := []models.PluginResourceType{{
		Slug: "my_resource",
		Name: "My Resource",
	}}
	err := service.RegisterPluginResourceTypes(plugin.ID, types)
	assert.NoError(t, err)

	t.Run("found", func(t *testing.T) {
		prt, err := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "my_resource")
		assert.NoError(t, err)
		assert.Equal(t, "My Resource", prt.Name)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "nonexistent")
		assert.Error(t, err)
	})
}

// --- App Plugin Resource Association Tests ---

func TestSetAppPluginResources(t *testing.T) {
	service, db := setupPluginResourceTest(t)
	plugin := createTestPlugin(t, db, "test-plugin")

	// Register a resource type
	err := service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{{
		Slug: "mcp_servers",
		Name: "MCP Servers",
	}})
	assert.NoError(t, err)
	prt, err := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "mcp_servers")
	assert.NoError(t, err)

	// Create a test user and app
	user := createTestAppUser(t, service, "plugin-test@test.com", "Plugin Test User")
	app, err := service.CreateApp("Test App", "Desc", user.ID, nil, nil, nil, nil, nil, nil)
	assert.NoError(t, err)

	t.Run("set resource instances for app", func(t *testing.T) {
		err := service.SetAppPluginResources(app.ID, prt.ID, []string{"server-1", "server-2"})
		assert.NoError(t, err)

		// Verify
		aprs, err := service.GetAppPluginResources(app.ID)
		assert.NoError(t, err)
		assert.Len(t, aprs, 2)

		instanceIDs := []string{aprs[0].InstanceID, aprs[1].InstanceID}
		assert.Contains(t, instanceIDs, "server-1")
		assert.Contains(t, instanceIDs, "server-2")
	})

	t.Run("replace resource instances", func(t *testing.T) {
		err := service.SetAppPluginResources(app.ID, prt.ID, []string{"server-3"})
		assert.NoError(t, err)

		aprs, err := service.GetAppPluginResources(app.ID)
		assert.NoError(t, err)
		assert.Len(t, aprs, 1)
		assert.Equal(t, "server-3", aprs[0].InstanceID)
	})

	t.Run("replace with same resource instances (soft-delete conflict)", func(t *testing.T) {
		// Set the resource instance initially
		err := service.SetAppPluginResources(app.ID, prt.ID, []string{"server-same"})
		assert.NoError(t, err)

		// Set it again with the same instance ID to trigger the conflict
		err = service.SetAppPluginResources(app.ID, prt.ID, []string{"server-same"})
		assert.NoError(t, err, "Should not fail with duplicate key violation on soft-deleted row")

		aprs, err := service.GetAppPluginResources(app.ID)
		assert.NoError(t, err)
		assert.Len(t, aprs, 1)
		assert.Equal(t, "server-same", aprs[0].InstanceID)
	})

	t.Run("clear resource instances", func(t *testing.T) {
		err := service.SetAppPluginResources(app.ID, prt.ID, []string{})
		assert.NoError(t, err)

		aprs, err := service.GetAppPluginResources(app.ID)
		assert.NoError(t, err)
		assert.Len(t, aprs, 0)
	})
}

func TestClearAppPluginResources(t *testing.T) {
	service, db := setupPluginResourceTest(t)
	plugin := createTestPlugin(t, db, "test-plugin")

	err := service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{
		{Slug: "type_a", Name: "Type A"},
		{Slug: "type_b", Name: "Type B"},
	})
	assert.NoError(t, err)

	prtA, _ := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "type_a")
	prtB, _ := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "type_b")

	user := createTestAppUser(t, service, "clear-test@test.com", "Clear Test")
	app, err := service.CreateApp("Clear Test App", "", user.ID, nil, nil, nil, nil, nil, nil)
	assert.NoError(t, err)

	// Set resources for both types
	service.SetAppPluginResources(app.ID, prtA.ID, []string{"a1", "a2"})
	service.SetAppPluginResources(app.ID, prtB.ID, []string{"b1"})

	aprs, _ := service.GetAppPluginResources(app.ID)
	assert.Len(t, aprs, 3)

	// Clear all
	err = service.ClearAppPluginResources(app.ID)
	assert.NoError(t, err)

	aprs, _ = service.GetAppPluginResources(app.ID)
	assert.Len(t, aprs, 0)
}

// --- Group Plugin Resource Access Control Tests ---

func TestSetGroupPluginResources(t *testing.T) {
	service, db := setupPluginResourceTest(t)
	plugin := createTestPlugin(t, db, "test-plugin")

	err := service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{{
		Slug: "mcp_servers",
		Name: "MCP Servers",
	}})
	assert.NoError(t, err)
	prt, _ := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "mcp_servers")

	// Create a group
	group := &models.Group{Name: "Test Group"}
	err = db.Create(group).Error
	assert.NoError(t, err)

	t.Run("assign instances to group", func(t *testing.T) {
		err := service.SetGroupPluginResources(group.ID, prt.ID, []string{"srv-1", "srv-2"})
		assert.NoError(t, err)

		gprs, err := service.GetGroupPluginResources(group.ID)
		assert.NoError(t, err)
		assert.Len(t, gprs, 2)
	})

	t.Run("replace instances", func(t *testing.T) {
		err := service.SetGroupPluginResources(group.ID, prt.ID, []string{"srv-3"})
		assert.NoError(t, err)

		gprs, err := service.GetGroupPluginResources(group.ID)
		assert.NoError(t, err)
		assert.Len(t, gprs, 1)
		assert.Equal(t, "srv-3", gprs[0].InstanceID)
	})
}

func TestGetAccessiblePluginResourceInstances(t *testing.T) {
	service, db := setupPluginResourceTest(t)
	plugin := createTestPlugin(t, db, "test-plugin")

	err := service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{{
		Slug: "mcp_servers",
		Name: "MCP Servers",
	}})
	assert.NoError(t, err)
	prt, _ := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "mcp_servers")

	// Create groups
	groupA := &models.Group{Name: "Group A"}
	db.Create(groupA)
	groupB := &models.Group{Name: "Group B"}
	db.Create(groupB)

	// Assign instances to groups
	service.SetGroupPluginResources(groupA.ID, prt.ID, []string{"srv-1", "srv-2"})
	service.SetGroupPluginResources(groupB.ID, prt.ID, []string{"srv-2", "srv-3"})

	// Create user in Group A only
	user := &models.User{Email: "access@test.com", Name: "Access User", Password: "pass123"}
	db.Create(user)
	db.Exec("INSERT INTO user_groups (user_id, group_id) VALUES (?, ?)", user.ID, groupA.ID)

	t.Run("user sees only their groups instances", func(t *testing.T) {
		ids, err := service.GetAccessiblePluginResourceInstances(user.ID, prt.ID)
		assert.NoError(t, err)
		assert.Len(t, ids, 2)
		assert.Contains(t, ids, "srv-1")
		assert.Contains(t, ids, "srv-2")
		assert.NotContains(t, ids, "srv-3") // only in Group B
	})

	t.Run("user in both groups sees union", func(t *testing.T) {
		// Add user to Group B
		db.Exec("INSERT INTO user_groups (user_id, group_id) VALUES (?, ?)", user.ID, groupB.ID)

		ids, err := service.GetAccessiblePluginResourceInstances(user.ID, prt.ID)
		assert.NoError(t, err)
		assert.Len(t, ids, 3) // srv-1, srv-2, srv-3 (deduplicated)
		assert.Contains(t, ids, "srv-1")
		assert.Contains(t, ids, "srv-2")
		assert.Contains(t, ids, "srv-3")
	})

	t.Run("user with no groups sees nothing", func(t *testing.T) {
		loneUser := &models.User{Email: "lone@test.com", Name: "Lone User", Password: "pass123"}
		db.Create(loneUser)

		ids, err := service.GetAccessiblePluginResourceInstances(loneUser.ID, prt.ID)
		assert.NoError(t, err)
		assert.Len(t, ids, 0)
	})
}

// --- Privacy Validation with Plugin Resources ---

func TestPrivacyValidationWithPluginResources(t *testing.T) {
	service, _ := setupPluginResourceTest(t)

	t.Run("plugin resource score within LLM limit passes", func(t *testing.T) {
		llm := createTestAppLLM(t, service, "high-llm", 80)

		err := service.validatePrivacyScoresWithPluginResources(
			nil,
			[]uint{llm.ID},
			[]int{50, 70},
		)
		assert.NoError(t, err)
	})

	t.Run("plugin resource score exceeding LLM limit fails", func(t *testing.T) {
		llm := createTestAppLLM(t, service, "low-llm", 30)

		err := service.validatePrivacyScoresWithPluginResources(
			nil,
			[]uint{llm.ID},
			[]int{50},
		)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "plugin resource has higher privacy requirements")
	})

	t.Run("mixed datasource and plugin resource scores", func(t *testing.T) {
		llm := createTestAppLLM(t, service, "mid-llm", 60)
		ds := createTestAppDatasource(t, service, "mid-ds", 40)

		// Datasource 40 + plugin resource 50 — both under LLM 60
		err := service.validatePrivacyScoresWithPluginResources(
			[]uint{ds.ID},
			[]uint{llm.ID},
			[]int{50},
		)
		assert.NoError(t, err)

		// Plugin resource 70 exceeds LLM 60
		err = service.validatePrivacyScoresWithPluginResources(
			[]uint{ds.ID},
			[]uint{llm.ID},
			[]int{70},
		)
		assert.Error(t, err)
	})

	t.Run("nil plugin scores same as old behavior", func(t *testing.T) {
		llm := createTestAppLLM(t, service, "basic-llm", 50)
		ds := createTestAppDatasource(t, service, "basic-ds", 30)

		err := service.validatePrivacyScoresWithPluginResources(
			[]uint{ds.ID},
			[]uint{llm.ID},
			nil,
		)
		assert.NoError(t, err)
	})
}

// --- CreateAppWithResources / UpdateAppWithResources ---

func TestCreateAppWithResources(t *testing.T) {
	service, db := setupPluginResourceTest(t)
	plugin := createTestPlugin(t, db, "test-plugin")

	err := service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{{
		Slug: "mcp_servers",
		Name: "MCP Servers",
	}})
	assert.NoError(t, err)

	user := createTestAppUser(t, service, "resources@test.com", "Resource User")

	t.Run("create app with plugin resources", func(t *testing.T) {
		budget := 100.0
		now := time.Now()

		app, err := service.CreateAppWithResources(
			"App With Resources",
			"Has plugin resources",
			user.ID,
			nil, nil, nil,
			&budget, &now,
			nil,
			[]PluginResourceSelection{{
				PluginID:         plugin.ID,
				ResourceTypeSlug: "mcp_servers",
				InstanceIDs:      []string{"srv-a", "srv-b"},
			}},
		)
		assert.NoError(t, err)
		assert.NotNil(t, app)

		// Verify plugin resources were bound
		aprs, err := service.GetAppPluginResources(app.ID)
		assert.NoError(t, err)
		assert.Len(t, aprs, 2)
	})

	t.Run("create app with unknown resource type fails", func(t *testing.T) {
		app, err := service.CreateAppWithResources(
			"Bad App",
			"Unknown type",
			user.ID,
			nil, nil, nil, nil, nil, nil,
			[]PluginResourceSelection{{
				PluginID:         plugin.ID,
				ResourceTypeSlug: "nonexistent",
				InstanceIDs:      []string{"x"},
			}},
		)
		assert.Error(t, err)
		assert.Nil(t, app)
		assert.Contains(t, err.Error(), "unknown resource type")
	})
}

func TestUpdateAppWithResources(t *testing.T) {
	service, db := setupPluginResourceTest(t)
	plugin := createTestPlugin(t, db, "test-plugin")

	err := service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{{
		Slug: "mcp_servers",
		Name: "MCP Servers",
	}})
	assert.NoError(t, err)

	user := createTestAppUser(t, service, "update@test.com", "Update User")
	app, err := service.CreateApp("Original App", "", user.ID, nil, nil, nil, nil, nil, nil)
	assert.NoError(t, err)

	t.Run("update app adding plugin resources", func(t *testing.T) {
		updated, err := service.UpdateAppWithResources(
			app.ID,
			"Updated App",
			"Now with resources",
			user.ID,
			nil, nil, nil, nil, nil, nil,
			[]PluginResourceSelection{{
				PluginID:         plugin.ID,
				ResourceTypeSlug: "mcp_servers",
				InstanceIDs:      []string{"srv-1"},
			}},
		)
		assert.NoError(t, err)
		assert.Equal(t, "Updated App", updated.Name)

		aprs, err := service.GetAppPluginResources(app.ID)
		assert.NoError(t, err)
		assert.Len(t, aprs, 1)
		assert.Equal(t, "srv-1", aprs[0].InstanceID)
	})

	t.Run("update app replacing plugin resources", func(t *testing.T) {
		_, err := service.UpdateAppWithResources(
			app.ID,
			"Updated App",
			"",
			user.ID,
			nil, nil, nil, nil, nil, nil,
			[]PluginResourceSelection{{
				PluginID:         plugin.ID,
				ResourceTypeSlug: "mcp_servers",
				InstanceIDs:      []string{"srv-2", "srv-3"},
			}},
		)
		assert.NoError(t, err)

		aprs, err := service.GetAppPluginResources(app.ID)
		assert.NoError(t, err)
		assert.Len(t, aprs, 2)
	})
}

// --- DeleteApp clears plugin resources ---

func TestDeleteAppClearsPluginResources(t *testing.T) {
	service, db := setupPluginResourceTest(t)
	plugin := createTestPlugin(t, db, "test-plugin")

	err := service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{{
		Slug: "servers",
		Name: "Servers",
	}})
	assert.NoError(t, err)
	prt, _ := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "servers")

	user := createTestAppUser(t, service, "delete@test.com", "Delete User")
	app, err := service.CreateApp("Delete Me", "", user.ID, nil, nil, nil, nil, nil, nil)
	assert.NoError(t, err)

	service.SetAppPluginResources(app.ID, prt.ID, []string{"s1", "s2"})

	// Verify resources exist
	aprs, _ := service.GetAppPluginResources(app.ID)
	assert.Len(t, aprs, 2)

	// Delete app
	err = service.DeleteApp(app.ID)
	assert.NoError(t, err)

	// Verify plugin resources were cleared
	aprs, _ = service.GetAppPluginResources(app.ID)
	assert.Len(t, aprs, 0)
}

// --- Fix 3: Denormalized instance details on AppPluginResource ---

func TestSetAppPluginResources_CachesInstanceDetails(t *testing.T) {
	// When AIStudioPluginManager is nil (no plugin RPC), instance details
	// should be empty but the association should still work.
	service, db := setupPluginResourceTest(t)
	plugin := createTestPlugin(t, db, "test-plugin")

	err := service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{{
		Slug: "mcp_servers",
		Name: "MCP Servers",
	}})
	assert.NoError(t, err)
	prt, _ := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "mcp_servers")

	user := createTestAppUser(t, service, "cache@test.com", "Cache User")
	app, err := service.CreateApp("Cache App", "", user.ID, nil, nil, nil, nil, nil, nil)
	assert.NoError(t, err)

	t.Run("without loaded plugin, details are empty but association works", func(t *testing.T) {
		// Plugin manager exists but no plugins are loaded — fetchInstanceDetails
		// should gracefully return empty details
		err := service.SetAppPluginResources(app.ID, prt.ID, []string{"srv-1"})
		assert.NoError(t, err)

		aprs, err := service.GetAppPluginResources(app.ID)
		assert.NoError(t, err)
		assert.Len(t, aprs, 1)
		assert.Equal(t, "srv-1", aprs[0].InstanceID)
		// Details are empty since no plugin is loaded to answer the RPC
		assert.Equal(t, "", aprs[0].InstanceName)
		assert.Equal(t, 0, aprs[0].InstancePrivacyScore)
	})
}

// --- Fix 3: fetchInstanceDetails graceful degradation ---

func TestFetchInstanceDetails_NoLoadedPlugin(t *testing.T) {
	service, _ := setupPluginResourceTest(t)

	// Plugin manager exists but plugin is not loaded — should return empty details
	details := service.fetchInstanceDetails(999, "any_slug", []string{"id1", "id2"})
	assert.NotNil(t, details)
	assert.Len(t, details, 0) // Empty map, no panic
}

// --- Fix 5: collectPluginResourcePrivacyScores graceful degradation ---

func TestCollectPluginResourcePrivacyScores_NoLoadedPlugin(t *testing.T) {
	service, _ := setupPluginResourceTest(t)

	// Plugin manager exists but plugin is not loaded — should return nil (skip validation)
	scores, err := service.collectPluginResourcePrivacyScores(999, "any_slug", []string{"id1"})
	assert.NoError(t, err)
	assert.Nil(t, scores) // nil, not empty slice — means "skip validation"
}

// --- Fix 5: Privacy validation with RPC-collected scores ---

func TestCreateAppWithResources_PrivacyValidationWithScores(t *testing.T) {
	service, db := setupPluginResourceTest(t)
	plugin := createTestPlugin(t, db, "test-plugin")

	// Register a resource type with privacy scoring
	err := service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{{
		Slug:            "private_servers",
		Name:            "Private Servers",
		HasPrivacyScore: true,
	}})
	assert.NoError(t, err)

	user := createTestAppUser(t, service, "privacy-rpc@test.com", "Privacy User")
	llm := createTestAppLLM(t, service, "privacy-llm", 50)

	t.Run("privacy validation passes when plugin manager nil (skips score collection)", func(t *testing.T) {
		// Without plugin manager, collectPluginResourcePrivacyScores returns nil
		// which means scores are skipped → validation passes
		app, err := service.CreateAppWithResources(
			"Privacy App",
			"",
			user.ID,
			nil,
			[]uint{llm.ID},
			nil, nil, nil, nil,
			[]PluginResourceSelection{{
				PluginID:         plugin.ID,
				ResourceTypeSlug: "private_servers",
				InstanceIDs:      []string{"srv-1"},
			}},
		)
		assert.NoError(t, err)
		assert.NotNil(t, app)
	})
}

// --- Fix 2: Event emission topic constant exists ---

func TestAppPluginResourcesChangedEventTopic(t *testing.T) {
	// Verify the event topic constant is defined correctly
	assert.Equal(t, "system.app.plugin_resources.changed", TopicAppPluginResourcesChanged)
}

// --- Fix 3: Denormalized fields accessible in model query ---

func TestAppPluginResource_DenormalizedFields(t *testing.T) {
	service, db := setupPluginResourceTest(t)
	plugin := createTestPlugin(t, db, "test-plugin")

	err := service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{{
		Slug: "servers",
		Name: "Servers",
	}})
	assert.NoError(t, err)
	prt, _ := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "servers")

	user := createTestAppUser(t, service, "denorm@test.com", "Denorm User")
	app, err := service.CreateApp("Denorm App", "", user.ID, nil, nil, nil, nil, nil, nil)
	assert.NoError(t, err)

	t.Run("manually set denormalized fields are persisted and queryable", func(t *testing.T) {
		// Simulate what happens when plugin manager provides instance details
		apr := &models.AppPluginResource{
			AppID:                app.ID,
			PluginResourceTypeID: prt.ID,
			InstanceID:           "srv-rich",
			InstanceName:         "Production MCP Server",
			InstancePrivacyScore: 75,
			InstanceMetadata:     []byte(`{"url":"https://mcp.example.com"}`),
		}
		err := db.Create(apr).Error
		assert.NoError(t, err)

		// Query back
		aprs, err := service.GetAppPluginResources(app.ID)
		assert.NoError(t, err)
		assert.Len(t, aprs, 1)
		assert.Equal(t, "srv-rich", aprs[0].InstanceID)
		assert.Equal(t, "Production MCP Server", aprs[0].InstanceName)
		assert.Equal(t, 75, aprs[0].InstancePrivacyScore)
		assert.JSONEq(t, `{"url":"https://mcp.example.com"}`, string(aprs[0].InstanceMetadata))
	})
}

// --- EnsureDefaultGroupAccess ---

func TestEnsureDefaultGroupAccess(t *testing.T) {
	service, db := setupPluginResourceTest(t)
	plugin := createTestPlugin(t, db, "test-plugin")

	err := service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{{
		Slug: "servers",
		Name: "Servers",
	}})
	assert.NoError(t, err)
	prt, _ := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "servers")

	// Create default group
	defaultGroup := &models.Group{Name: models.DefaultGroupName}
	db.Create(defaultGroup)

	t.Run("new instances are auto-assigned to default group", func(t *testing.T) {
		err := service.EnsureDefaultGroupAccess(prt.ID, []string{"srv-1", "srv-2"})
		assert.NoError(t, err)

		gprs, _ := service.GetGroupPluginResources(defaultGroup.ID)
		assert.Len(t, gprs, 2)
	})

	t.Run("existing instances are not duplicated", func(t *testing.T) {
		// Call again with one new and one existing
		err := service.EnsureDefaultGroupAccess(prt.ID, []string{"srv-1", "srv-2", "srv-3"})
		assert.NoError(t, err)

		gprs, _ := service.GetGroupPluginResources(defaultGroup.ID)
		assert.Len(t, gprs, 3) // srv-1, srv-2 unchanged + srv-3 added
	})

	t.Run("no default group means no-op", func(t *testing.T) {
		// Delete the default group
		db.Where("name = ?", models.DefaultGroupName).Delete(&models.Group{})

		err := service.EnsureDefaultGroupAccess(prt.ID, []string{"srv-4"})
		assert.NoError(t, err) // No error, just a no-op
	})
}
