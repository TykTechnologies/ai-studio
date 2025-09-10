// plugins/integration_test.go
package plugins

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupIntegrationTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Auto-migrate all models
	err = db.AutoMigrate(
		&database.Plugin{},
		&database.LLM{},
		&database.LLMPlugin{},
	)
	require.NoError(t, err)

	return db
}

// TestPluginSystemIntegration tests the complete plugin lifecycle
func TestPluginSystemIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	db := setupIntegrationTestDB(t)
	repo := database.NewRepository(db)
	pluginService := services.NewPluginService(db, repo)
	manager := NewPluginManager(pluginService)

	// Create an LLM for testing
	llm := &database.LLM{
		Name:         "Test LLM",
		Slug:         "test-llm",
		Vendor:       "test",
		DefaultModel: "test-model",
		IsActive:     true,
	}
	err := db.Create(llm).Error
	require.NoError(t, err)

	// Create a test plugin configuration
	pluginReq := &services.CreatePluginRequest{
		Name:        "Integration Test Plugin",
		Slug:        "integration-test-plugin",
		Description: "A plugin for integration testing",
		Command:     "./non-existent-plugin", // Will fail to load, but we're testing the service layer
		HookType:    "pre_auth",
		IsActive:    true,
		Config:      map[string]interface{}{"test": "value"},
	}

	// Test plugin creation
	plugin, err := pluginService.CreatePlugin(pluginReq)
	require.NoError(t, err)
	assert.NotNil(t, plugin)
	assert.Equal(t, pluginReq.Name, plugin.Name)
	assert.Equal(t, pluginReq.Slug, plugin.Slug)

	// Test plugin retrieval
	retrievedPlugin, err := pluginService.GetPlugin(plugin.ID)
	require.NoError(t, err)
	assert.Equal(t, plugin.ID, retrievedPlugin.ID)
	assert.Equal(t, plugin.Name, retrievedPlugin.Name)

	// Test plugin listing
	plugins, total, err := pluginService.ListPlugins(1, 10, "", true)
	require.NoError(t, err)
	assert.Len(t, plugins, 1)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, plugin.ID, plugins[0].ID)

	// Test LLM-plugin association
	err = pluginService.UpdateLLMPlugins(llm.ID, []uint{plugin.ID})
	require.NoError(t, err)

	// Test getting plugins for LLM
	llmPlugins, err := pluginService.GetPluginsForLLM(llm.ID)
	require.NoError(t, err)
	assert.Len(t, llmPlugins, 1)
	assert.Equal(t, plugin.ID, llmPlugins[0].ID)

	// Test plugin manager's refresh functionality
	err = manager.RefreshLLMPluginMapping(llm.ID)
	require.NoError(t, err)

	// Test getting plugins for LLM through manager (should not load since command doesn't exist)
	_, err = manager.GetPluginsForLLM(llm.ID, interfaces.HookTypePreAuth)
	assert.NoError(t, err) // Should not error, but won't have loaded plugins due to invalid command

	// Test plugin update
	updateReq := &services.UpdatePluginRequest{
		Description: strPtr("Updated description"),
		IsActive:    boolPtr(false),
	}
	updatedPlugin, err := pluginService.UpdatePlugin(plugin.ID, updateReq)
	require.NoError(t, err)
	assert.Equal(t, "Updated description", updatedPlugin.Description)
	assert.False(t, updatedPlugin.IsActive)

	// Test plugin deletion
	err = pluginService.DeletePlugin(plugin.ID)
	require.NoError(t, err)

	// Verify plugin is deleted
	_, err = pluginService.GetPlugin(plugin.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "plugin not found")

	// Test manager shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = manager.Shutdown(ctx)
	assert.NoError(t, err)
}

// Helper functions for pointer types
func strPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

// TestPluginConfigValidation tests plugin configuration validation
func TestPluginConfigValidation(t *testing.T) {
	db := setupIntegrationTestDB(t)
	repo := database.NewRepository(db)
	service := services.NewPluginService(db, repo)

	tests := []struct {
		name      string
		hookType  string
		wantError bool
	}{
		{"valid pre_auth", "pre_auth", false},
		{"valid auth", "auth", false},
		{"valid post_auth", "post_auth", false},
		{"valid on_response", "on_response", false},
		{"invalid hook", "invalid_hook", true},
		{"empty hook", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &services.CreatePluginRequest{
				Name:     "Test Plugin",
				Slug:     fmt.Sprintf("test-%s", tt.name),
				Command:  "./test-plugin",
				HookType: tt.hookType,
				IsActive: true,
			}

			_, err := service.CreatePlugin(req)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestPluginSlugUniqueness tests that plugin slugs must be unique
func TestPluginSlugUniqueness(t *testing.T) {
	db := setupIntegrationTestDB(t)
	repo := database.NewRepository(db)
	service := services.NewPluginService(db, repo)

	// Create first plugin
	req1 := &services.CreatePluginRequest{
		Name:     "Plugin 1",
		Slug:     "unique-slug",
		Command:  "./plugin1",
		HookType: "pre_auth",
		IsActive: true,
	}
	_, err := service.CreatePlugin(req1)
	require.NoError(t, err)

	// Try to create second plugin with same slug
	req2 := &services.CreatePluginRequest{
		Name:     "Plugin 2",
		Slug:     "unique-slug", // Same slug
		Command:  "./plugin2",
		HookType: "auth",
		IsActive: true,
	}
	_, err = service.CreatePlugin(req2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "slug 'unique-slug' already exists")
}

// TestPluginConfigParsing tests JSON configuration parsing
func TestPluginConfigParsing(t *testing.T) {
	db := setupIntegrationTestDB(t)
	repo := database.NewRepository(db)
	service := services.NewPluginService(db, repo)

	config := map[string]interface{}{
		"string_val":  "test",
		"int_val":     42,
		"bool_val":    true,
		"nested_obj":  map[string]interface{}{"inner": "value"},
		"array_val":   []interface{}{1, 2, 3},
	}

	req := &services.CreatePluginRequest{
		Name:     "Config Test Plugin",
		Slug:     "config-test",
		Command:  "./config-test",
		HookType: "pre_auth",
		IsActive: true,
		Config:   config,
	}

	plugin, err := service.CreatePlugin(req)
	require.NoError(t, err)

	// Retrieve plugin and verify config was stored correctly
	retrievedPlugin, err := service.GetPlugin(plugin.ID)
	require.NoError(t, err)

	// The config should be stored as JSON and retrievable
	assert.NotNil(t, retrievedPlugin.Config)
	
	// Note: Direct comparison of JSON config requires unmarshaling
	// This test ensures the config was stored without errors
}