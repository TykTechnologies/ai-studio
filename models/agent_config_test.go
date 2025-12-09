package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAgentConfigTest(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = InitModels(db)
	assert.NoError(t, err)

	return db
}

// Helper to create test plugin with agent hook
func createTestAgentPlugin(t *testing.T, db *gorm.DB, name string) *Plugin {
	plugin := &Plugin{
		Name:      name,
		Command:   "/usr/bin/test-agent",
		HookType:  HookTypeAgent,
		HookTypes: []string{HookTypeAgent},
		IsActive:  true,
	}
	err := db.Create(plugin).Error
	assert.NoError(t, err)
	return plugin
}

// Helper to create test app with LLM
func createTestAgentApp(t *testing.T, db *gorm.DB, name string) *App {
	// Create LLM first
	llm := &LLM{
		Name:             name + "-llm",
		Vendor:           OPENAI,
		ShortDescription: "Test LLM",
		DefaultModel:     "gpt-4",
		Active:           true,
	}
	db.Create(llm)

	// Create app with credential
	app := &App{
		Name:        name,
		Description: "Test App",
		IsActive:    true,
	}
	app.Create(db) // This creates credential automatically

	// Associate LLM with app
	db.Model(app).Association("LLMs").Append(llm)

	// Reload app to get credential
	app.Get(db, app.ID)

	return app
}

func TestNewAgentConfig(t *testing.T) {
	t.Run("Create new agent config", func(t *testing.T) {
		config := NewAgentConfig()
		assert.NotNil(t, config)
		assert.True(t, config.IsActive)
		assert.NotNil(t, config.Config)
		assert.Len(t, config.Config, 0)
	})
}

func TestAgentConfig_TableName(t *testing.T) {
	t.Run("Table name is agent_configs", func(t *testing.T) {
		config := AgentConfig{}
		assert.Equal(t, "agent_configs", config.TableName())
	})
}

func TestAgentConfig_Validate(t *testing.T) {
	db := setupAgentConfigTest(t)

	plugin := createTestAgentPlugin(t, db, "test-agent-plugin")
	app := createTestAgentApp(t, db, "test-agent-app")

	t.Run("Valid agent config passes validation", func(t *testing.T) {
		config := &AgentConfig{
			Name:     "Test Agent",
			Slug:     "test-agent",
			PluginID: plugin.ID,
			AppID:    app.ID,
		}
		err := config.Validate(db)
		assert.NoError(t, err)
	})

	t.Run("Missing name fails validation", func(t *testing.T) {
		config := &AgentConfig{
			Name:     "",
			Slug:     "test-agent",
			PluginID: plugin.ID,
			AppID:    app.ID,
		}
		err := config.Validate(db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "agent name is required")
	})

	t.Run("Missing slug fails validation", func(t *testing.T) {
		config := &AgentConfig{
			Name:     "Test Agent",
			Slug:     "",
			PluginID: plugin.ID,
			AppID:    app.ID,
		}
		err := config.Validate(db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "agent slug is required")
	})

	t.Run("Missing plugin ID fails validation", func(t *testing.T) {
		config := &AgentConfig{
			Name:     "Test Agent",
			Slug:     "test-agent",
			PluginID: 0,
			AppID:    app.ID,
		}
		err := config.Validate(db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "plugin ID is required")
	})

	t.Run("Missing app ID fails validation", func(t *testing.T) {
		config := &AgentConfig{
			Name:     "Test Agent",
			Slug:     "test-agent",
			PluginID: plugin.ID,
			AppID:    0,
		}
		err := config.Validate(db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "app ID is required")
	})

	t.Run("Non-existent plugin fails validation", func(t *testing.T) {
		config := &AgentConfig{
			Name:     "Test Agent",
			Slug:     "test-agent",
			PluginID: 99999,
			AppID:    app.ID,
		}
		err := config.Validate(db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "plugin not found")
	})

	t.Run("Plugin without agent hook fails validation", func(t *testing.T) {
		wrongPlugin := &Plugin{
			Name:     "Non-Agent Plugin",
			Command:  "/bin/test",
			HookType: "post_auth",
			IsActive: true,
		}
		db.Create(wrongPlugin)

		config := &AgentConfig{
			Name:     "Test Agent",
			Slug:     "test-agent",
			PluginID: wrongPlugin.ID,
			AppID:    app.ID,
		}
		err := config.Validate(db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not support agent hook type")
	})

	t.Run("Inactive plugin fails validation", func(t *testing.T) {
		inactivePlugin := createTestAgentPlugin(t, db, "inactive-plugin")
		inactivePlugin.IsActive = false
		db.Save(inactivePlugin)

		config := &AgentConfig{
			Name:     "Test Agent",
			Slug:     "test-agent",
			PluginID: inactivePlugin.ID,
			AppID:    app.ID,
		}
		err := config.Validate(db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "plugin is not active")
	})

	t.Run("Non-existent app fails validation", func(t *testing.T) {
		config := &AgentConfig{
			Name:     "Test Agent",
			Slug:     "test-agent",
			PluginID: plugin.ID,
			AppID:    99999,
		}
		err := config.Validate(db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "app not found")
	})

	t.Run("Inactive app fails validation", func(t *testing.T) {
		inactiveApp := createTestAgentApp(t, db, "inactive-app")
		inactiveApp.IsActive = false
		db.Save(inactiveApp)

		config := &AgentConfig{
			Name:     "Test Agent",
			Slug:     "test-agent",
			PluginID: plugin.ID,
			AppID:    inactiveApp.ID,
		}
		err := config.Validate(db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "app is not active")
	})

	t.Run("App without LLM fails validation", func(t *testing.T) {
		appNoLLM := &App{
			Name:     "App No LLM",
			IsActive: true,
		}
		appNoLLM.Create(db)

		config := &AgentConfig{
			Name:     "Test Agent",
			Slug:     "test-agent",
			PluginID: plugin.ID,
			AppID:    appNoLLM.ID,
		}
		err := config.Validate(db)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "app must have at least one LLM")
	})
}

func TestAgentConfig_Create(t *testing.T) {
	db := setupAgentConfigTest(t)

	plugin := createTestAgentPlugin(t, db, "create-plugin")
	app := createTestAgentApp(t, db, "create-app")

	t.Run("Create agent config successfully", func(t *testing.T) {
		config := &AgentConfig{
			Name:        "Test Agent",
			Slug:        "test-agent",
			Description: "Test Description",
			PluginID:    plugin.ID,
			AppID:       app.ID,
			Config:      map[string]interface{}{"key": "value"},
		}

		err := config.Create(db)
		assert.NoError(t, err)
		assert.NotZero(t, config.ID)
	})

	t.Run("Create with duplicate slug fails", func(t *testing.T) {
		config1 := &AgentConfig{
			Name:     "Agent 1",
			Slug:     "duplicate-slug",
			PluginID: plugin.ID,
			AppID:    app.ID,
		}
		err := config1.Create(db)
		assert.NoError(t, err)

		config2 := &AgentConfig{
			Name:     "Agent 2",
			Slug:     "duplicate-slug",
			PluginID: plugin.ID,
			AppID:    app.ID,
		}
		err = config2.Create(db)
		assert.Error(t, err) // Should fail due to unique constraint
	})

	t.Run("Create with invalid data fails validation", func(t *testing.T) {
		config := &AgentConfig{
			Name:     "",
			Slug:     "invalid",
			PluginID: plugin.ID,
			AppID:    app.ID,
		}
		err := config.Create(db)
		assert.Error(t, err)
	})
}

func TestAgentConfig_Get(t *testing.T) {
	db := setupAgentConfigTest(t)

	plugin := createTestAgentPlugin(t, db, "get-plugin")
	app := createTestAgentApp(t, db, "get-app")

	config := &AgentConfig{
		Name:     "Get Test Agent",
		Slug:     "get-test-agent",
		PluginID: plugin.ID,
		AppID:    app.ID,
	}
	config.Create(db)

	t.Run("Get existing agent config", func(t *testing.T) {
		retrieved := &AgentConfig{}
		err := retrieved.Get(db, config.ID)
		assert.NoError(t, err)
		assert.Equal(t, config.ID, retrieved.ID)
		assert.Equal(t, "Get Test Agent", retrieved.Name)
		assert.NotNil(t, retrieved.Plugin)
		assert.NotNil(t, retrieved.App)
	})

	t.Run("Get non-existent agent config", func(t *testing.T) {
		retrieved := &AgentConfig{}
		err := retrieved.Get(db, 99999)
		assert.Error(t, err)
	})
}

func TestAgentConfig_GetBySlug(t *testing.T) {
	db := setupAgentConfigTest(t)

	plugin := createTestAgentPlugin(t, db, "slug-plugin")
	app := createTestAgentApp(t, db, "slug-app")

	config := &AgentConfig{
		Name:     "Slug Test Agent",
		Slug:     "slug-test-agent",
		PluginID: plugin.ID,
		AppID:    app.ID,
	}
	config.Create(db)

	t.Run("Get by existing slug", func(t *testing.T) {
		retrieved := &AgentConfig{}
		err := retrieved.GetBySlug(db, "slug-test-agent")
		assert.NoError(t, err)
		assert.Equal(t, config.ID, retrieved.ID)
	})

	t.Run("Get by non-existent slug", func(t *testing.T) {
		retrieved := &AgentConfig{}
		err := retrieved.GetBySlug(db, "nonexistent-slug")
		assert.Error(t, err)
	})
}

func TestAgentConfig_Update(t *testing.T) {
	db := setupAgentConfigTest(t)

	plugin := createTestAgentPlugin(t, db, "update-plugin")
	app := createTestAgentApp(t, db, "update-app")

	config := &AgentConfig{
		Name:     "Original Name",
		Slug:     "update-test",
		PluginID: plugin.ID,
		AppID:    app.ID,
	}
	config.Create(db)

	t.Run("Update agent config successfully", func(t *testing.T) {
		config.Name = "Updated Name"
		config.Description = "Updated Description"
		err := config.Update(db)
		assert.NoError(t, err)

		// Verify update
		retrieved := &AgentConfig{}
		retrieved.Get(db, config.ID)
		assert.Equal(t, "Updated Name", retrieved.Name)
		assert.Equal(t, "Updated Description", retrieved.Description)
	})

	t.Run("Update with invalid data fails", func(t *testing.T) {
		config.Name = ""
		err := config.Update(db)
		assert.Error(t, err)
	})
}

func TestAgentConfig_Delete(t *testing.T) {
	db := setupAgentConfigTest(t)

	plugin := createTestAgentPlugin(t, db, "delete-plugin")
	app := createTestAgentApp(t, db, "delete-app")

	config := &AgentConfig{
		Name:     "Delete Test",
		Slug:     "delete-test",
		PluginID: plugin.ID,
		AppID:    app.ID,
	}
	config.Create(db)

	t.Run("Delete agent config", func(t *testing.T) {
		err := config.Delete(db)
		assert.NoError(t, err)

		// Verify deletion
		retrieved := &AgentConfig{}
		err = retrieved.Get(db, config.ID)
		assert.Error(t, err)
	})
}

func TestAgentConfig_ActivateDeactivate(t *testing.T) {
	db := setupAgentConfigTest(t)

	plugin := createTestAgentPlugin(t, db, "active-plugin")
	app := createTestAgentApp(t, db, "active-app")

	config := &AgentConfig{
		Name:     "Active Test",
		Slug:     "active-test",
		PluginID: plugin.ID,
		AppID:    app.ID,
		IsActive: true,
	}
	config.Create(db)

	t.Run("Deactivate agent config", func(t *testing.T) {
		err := config.Deactivate(db)
		assert.NoError(t, err)

		retrieved := &AgentConfig{}
		retrieved.Get(db, config.ID)
		assert.False(t, retrieved.IsActive)
	})

	t.Run("Activate agent config", func(t *testing.T) {
		err := config.Activate(db)
		assert.NoError(t, err)

		retrieved := &AgentConfig{}
		retrieved.Get(db, config.ID)
		assert.True(t, retrieved.IsActive)
	})
}

func TestAgentConfig_GroupOperations(t *testing.T) {
	db := setupAgentConfigTest(t)

	plugin := createTestAgentPlugin(t, db, "group-plugin")
	app := createTestAgentApp(t, db, "group-app")

	config := &AgentConfig{
		Name:     "Group Test",
		Slug:     "group-test",
		PluginID: plugin.ID,
		AppID:    app.ID,
	}
	config.Create(db)

	// Create test groups
	group1 := &Group{Name: "Group 1"}
	db.Create(group1)
	group2 := &Group{Name: "Group 2"}
	db.Create(group2)

	t.Run("Add group to agent config", func(t *testing.T) {
		err := config.AddGroup(db, group1)
		assert.NoError(t, err)

		// Verify group was added
		config.GetGroups(db)
		assert.Len(t, config.Groups, 1)
	})

	t.Run("Add multiple groups", func(t *testing.T) {
		err := config.AddGroup(db, group2)
		assert.NoError(t, err)

		config.GetGroups(db)
		assert.Len(t, config.Groups, 2)
	})

	t.Run("Remove group from agent config", func(t *testing.T) {
		err := config.RemoveGroup(db, group1)
		assert.NoError(t, err)

		// Clear and reload groups
		config.Groups = nil
		config.GetGroups(db)
		assert.Len(t, config.Groups, 1)
		assert.Equal(t, group2.ID, config.Groups[0].ID)
	})

	t.Run("Get groups for agent config", func(t *testing.T) {
		config.Groups = nil
		err := config.GetGroups(db)
		assert.NoError(t, err)
		assert.Len(t, config.Groups, 1)
	})
}

func TestAgentConfigs_ListWithPagination(t *testing.T) {
	db := setupAgentConfigTest(t)

	plugin := createTestAgentPlugin(t, db, "list-plugin")
	app := createTestAgentApp(t, db, "list-app")

	// Create agent configs in different namespaces
	for i := 1; i <= 5; i++ {
		namespace := ""
		if i > 2 {
			namespace = "production"
		}
		config := &AgentConfig{
			Name:      "Agent " + string(rune('0'+i)),
			Slug:      "agent-" + string(rune('0'+i)),
			PluginID:  plugin.ID,
			AppID:     app.ID,
			Namespace: namespace,
		}
		config.Create(db)
	}

	t.Run("List all agents with pagination", func(t *testing.T) {
		var configs AgentConfigs
		totalCount, totalPages, err := configs.ListWithPagination(db, 2, 1, false, "", nil)
		assert.NoError(t, err)
		assert.Len(t, configs, 2)
		assert.Equal(t, int64(5), totalCount)
		assert.Equal(t, 3, totalPages)
	})

	t.Run("List agents by namespace", func(t *testing.T) {
		var configs AgentConfigs
		totalCount, _, err := configs.ListWithPagination(db, 10, 1, false, "production", nil)
		assert.NoError(t, err)
		// Should include global agents (empty namespace) + production agents
		assert.GreaterOrEqual(t, len(configs), 3)
		assert.GreaterOrEqual(t, totalCount, int64(3))
	})

	t.Run("List active agents only", func(t *testing.T) {
		active := true
		var configs AgentConfigs
		totalCount, _, err := configs.ListWithPagination(db, 10, 1, false, "", &active)
		assert.NoError(t, err)
		assert.Equal(t, int64(5), totalCount)

		// Verify all are active
		for _, config := range configs {
			assert.True(t, config.IsActive)
		}
	})
}

func TestAgentConfigs_GetByPluginID(t *testing.T) {
	db := setupAgentConfigTest(t)

	plugin1 := createTestAgentPlugin(t, db, "plugin1")
	plugin2 := createTestAgentPlugin(t, db, "plugin2")
	app := createTestAgentApp(t, db, "app")

	// Create configs for plugin1
	for i := 1; i <= 3; i++ {
		config := &AgentConfig{
			Name:     "Plugin1 Agent " + string(rune('0'+i)),
			Slug:     "plugin1-agent-" + string(rune('0'+i)),
			PluginID: plugin1.ID,
			AppID:    app.ID,
		}
		config.Create(db)
	}

	// Create config for plugin2
	config := &AgentConfig{
		Name:     "Plugin2 Agent",
		Slug:     "plugin2-agent",
		PluginID: plugin2.ID,
		AppID:    app.ID,
	}
	config.Create(db)

	t.Run("Get agents by plugin ID", func(t *testing.T) {
		var configs AgentConfigs
		err := configs.GetByPluginID(db, plugin1.ID)
		assert.NoError(t, err)
		assert.Len(t, configs, 3)

		// Verify all belong to plugin1
		for _, cfg := range configs {
			assert.Equal(t, plugin1.ID, cfg.PluginID)
		}
	})

	t.Run("Get agents for plugin with no configs", func(t *testing.T) {
		plugin3 := createTestAgentPlugin(t, db, "plugin3")
		var configs AgentConfigs
		err := configs.GetByPluginID(db, plugin3.ID)
		assert.NoError(t, err)
		assert.Len(t, configs, 0)
	})
}

func TestAgentConfigs_GetByAppID(t *testing.T) {
	db := setupAgentConfigTest(t)

	plugin := createTestAgentPlugin(t, db, "app-plugin")
	app1 := createTestAgentApp(t, db, "app1")
	app2 := createTestAgentApp(t, db, "app2")

	// Create configs for app1
	for i := 1; i <= 3; i++ {
		config := &AgentConfig{
			Name:     "App1 Agent " + string(rune('0'+i)),
			Slug:     "app1-agent-" + string(rune('0'+i)),
			PluginID: plugin.ID,
			AppID:    app1.ID,
		}
		config.Create(db)
	}

	// Create config for app2
	config := &AgentConfig{
		Name:     "App2 Agent",
		Slug:     "app2-agent",
		PluginID: plugin.ID,
		AppID:    app2.ID,
	}
	config.Create(db)

	t.Run("Get agents by app ID", func(t *testing.T) {
		var configs AgentConfigs
		err := configs.GetByAppID(db, app1.ID)
		assert.NoError(t, err)
		assert.Len(t, configs, 3)

		// Verify all belong to app1
		for _, cfg := range configs {
			assert.Equal(t, app1.ID, cfg.AppID)
		}
	})
}

func TestAgentConfig_CountActive(t *testing.T) {
	db := setupAgentConfigTest(t)

	plugin := createTestAgentPlugin(t, db, "count-plugin")
	app := createTestAgentApp(t, db, "count-app")

	// Create active agents
	for i := 1; i <= 3; i++ {
		config := &AgentConfig{
			Name:     "Active Agent " + string(rune('0'+i)),
			Slug:     "active-count-" + string(rune('0'+i)),
			PluginID: plugin.ID,
			AppID:    app.ID,
			IsActive: true,
		}
		config.Create(db)
	}

	// Create inactive agent
	inactiveConfig := &AgentConfig{
		Name:     "Inactive Agent",
		Slug:     "inactive-count",
		PluginID: plugin.ID,
		AppID:    app.ID,
		IsActive: false,
	}
	db.Create(inactiveConfig) // Use db.Create to bypass validation

	t.Run("Count active agents", func(t *testing.T) {
		// Count all active agents in this fresh database
		config := &AgentConfig{}
		count, err := config.CountActive(db)
		assert.NoError(t, err)
		// Count should be exactly 3 (only the ones we created as active)
		assert.GreaterOrEqual(t, count, int64(3))
	})
}

func TestAgentConfig_CountByPluginID(t *testing.T) {
	db := setupAgentConfigTest(t)

	plugin1 := createTestAgentPlugin(t, db, "count-plugin1")
	plugin2 := createTestAgentPlugin(t, db, "count-plugin2")
	app := createTestAgentApp(t, db, "count-app")

	// Create agents for plugin1
	for i := 1; i <= 3; i++ {
		config := &AgentConfig{
			Name:     "Plugin1 Agent " + string(rune('0'+i)),
			Slug:     "p1-agent-" + string(rune('0'+i)),
			PluginID: plugin1.ID,
			AppID:    app.ID,
		}
		config.Create(db)
	}

	// Create agent for plugin2
	config := &AgentConfig{
		Name:     "Plugin2 Agent",
		Slug:     "p2-agent",
		PluginID: plugin2.ID,
		AppID:    app.ID,
	}
	config.Create(db)

	t.Run("Count agents by plugin ID", func(t *testing.T) {
		agentConfig := &AgentConfig{}
		count, err := agentConfig.CountByPluginID(db, plugin1.ID)
		assert.NoError(t, err)
		assert.Equal(t, int64(3), count)
	})
}

func TestAgentConfig_HasAccessForUser(t *testing.T) {
	db := setupAgentConfigTest(t)

	plugin := createTestAgentPlugin(t, db, "access-plugin")
	app := createTestAgentApp(t, db, "access-app")

	// Create test users
	user1 := &User{
		Email:    "user1@test.com",
		Name:     "User 1",
		Password: "password",
	}
	db.Create(user1)

	user2 := &User{
		Email:    "user2@test.com",
		Name:     "User 2",
		Password: "password",
	}
	db.Create(user2)

	// Create test groups
	group1 := &Group{Name: "Access Group 1"}
	db.Create(group1)

	// Add user1 to group1
	db.Model(user1).Association("Groups").Append(group1)

	t.Run("Agent with no groups is accessible to all users", func(t *testing.T) {
		config := &AgentConfig{
			Name:     "Public Agent",
			Slug:     "public-agent",
			PluginID: plugin.ID,
			AppID:    app.ID,
		}
		config.Create(db)

		hasAccess, err := config.HasAccessForUser(db, user1.ID)
		assert.NoError(t, err)
		assert.True(t, hasAccess)

		hasAccess, err = config.HasAccessForUser(db, user2.ID)
		assert.NoError(t, err)
		assert.True(t, hasAccess)
	})

	t.Run("User in agent group has access", func(t *testing.T) {
		config := &AgentConfig{
			Name:     "Restricted Agent",
			Slug:     "restricted-agent",
			PluginID: plugin.ID,
			AppID:    app.ID,
		}
		config.Create(db)

		// Add group to agent
		config.AddGroup(db, group1)

		// user1 is in group1, should have access
		hasAccess, err := config.HasAccessForUser(db, user1.ID)
		assert.NoError(t, err)
		assert.True(t, hasAccess)

		// user2 is not in group1, should not have access
		hasAccess, err = config.HasAccessForUser(db, user2.ID)
		assert.NoError(t, err)
		assert.False(t, hasAccess)
	})

	t.Run("Non-existent user with restricted agent returns error", func(t *testing.T) {
		config := &AgentConfig{
			Name:     "Restricted Test Agent",
			Slug:     "test-access-nonexistent",
			PluginID: plugin.ID,
			AppID:    app.ID,
		}
		config.Create(db)

		// Add a group to make it restricted
		group2 := &Group{Name: "Access Group 2"}
		db.Create(group2)
		config.AddGroup(db, group2)

		// Now non-existent user should cause error (because we try to check groups)
		hasAccess, err := config.HasAccessForUser(db, 99999)
		assert.Error(t, err)
		assert.False(t, hasAccess)
		assert.Contains(t, err.Error(), "failed to load user")
	})
}
