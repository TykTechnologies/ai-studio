package grpc

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCompleteServiceScopeWorkflow(t *testing.T) {
	// Setup test database with all required models
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.Plugin{}, &models.LLM{}, &models.App{})
	require.NoError(t, err)

	t.Run("plugin service access authorization lifecycle", func(t *testing.T) {
		// Step 1: Create plugin with declared service scopes (simulating manifest loading)
		plugin := &models.Plugin{
			Name:                    "Workflow Test Plugin",
			Slug:                    "workflow-test-plugin",
			Command:                 "test-command",
			HookType:                models.HookTypeStudioUI,
			PluginType:              models.PluginTypeAIStudio,
			IsActive:                true,
			ServiceAccessAuthorized: false, // Not yet authorized
			ServiceScopes:           []string{"analytics.read", "plugins.read"}, // Declared in manifest
		}

		err := db.Create(plugin).Error
		require.NoError(t, err)

		// Step 2: Verify initial state - scopes declared but not authorized
		assert.False(t, plugin.HasServiceAccess())
		assert.True(t, plugin.HasServiceScope("analytics.read"))
		assert.True(t, plugin.HasServiceScope("plugins.read"))
		assert.False(t, plugin.HasServiceScope("plugins.write"))

		// Step 3: Admin authorizes service access
		err = plugin.AuthorizeServiceAccess(db, plugin.ServiceScopes)
		require.NoError(t, err)

		// Reload from database to verify persistence
		var reloadedPlugin models.Plugin
		err = db.First(&reloadedPlugin, plugin.ID).Error
		require.NoError(t, err)

		// Step 4: Verify authorization is now active
		assert.True(t, reloadedPlugin.HasServiceAccess())
		assert.Equal(t, []string{"analytics.read", "plugins.read"}, reloadedPlugin.ServiceScopes)

		// Step 5: Admin revokes service access
		err = reloadedPlugin.RevokeServiceAccess(db)
		require.NoError(t, err)

		// Step 6: Verify access is revoked
		err = db.First(&reloadedPlugin, plugin.ID).Error
		require.NoError(t, err)
		assert.False(t, reloadedPlugin.HasServiceAccess())
		assert.Empty(t, reloadedPlugin.ServiceScopes)
	})
}

func TestGRPCServiceIntegration(t *testing.T) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.Plugin{}, &models.LLM{}, &models.App{})
	require.NoError(t, err)

	// Create test service and server
	service := services.NewService(db)
	pluginServer := NewPluginManagementServer(service.PluginService)

	t.Run("plugin management server works with authorized plugin", func(t *testing.T) {
		// Create test plugins
		plugin1 := &models.Plugin{
			Name:                    "Authorized Plugin",
			Slug:                    "authorized-plugin",
			Command:                 "authorized-command",
			HookType:                models.HookTypeStudioUI,
			PluginType:              models.PluginTypeAIStudio,
			IsActive:                true,
			ServiceAccessAuthorized: true,
			ServiceScopes:           []string{models.ServiceScopePluginsRead},
		}

		plugin2 := &models.Plugin{
			Name:       "Another Plugin",
			Slug:       "another-plugin",
			Command:    "another-command",
			HookType:   models.HookTypeStudioUI,
			PluginType: models.PluginTypeAIStudio,
			IsActive:   true,
		}

		err := db.Create(plugin1).Error
		require.NoError(t, err)
		err = db.Create(plugin2).Error
		require.NoError(t, err)

		// Create context (auth interceptor would normally handle this)
		ctx := SetPluginInContext(context.Background(), plugin1)

		// Test ListPlugins call
		req := &pb.ListPluginsRequest{
			Context: &pb.PluginContext{
				PluginId:    uint32(plugin1.ID),
				MethodScope: models.ServiceScopePluginsRead,
			},
			Page:  1,
			Limit: 10,
		}

		resp, err := pluginServer.ListPlugins(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.GreaterOrEqual(t, len(resp.Plugins), 1) // Should find at least the plugins we created
		assert.Equal(t, int64(2), resp.TotalCount)     // Should count both plugins

		// Verify plugin information in response
		var foundPlugin *pb.PluginInfo
		for _, p := range resp.Plugins {
			if p.Id == uint32(plugin1.ID) {
				foundPlugin = p
				break
			}
		}

		require.NotNil(t, foundPlugin, "Should find the test plugin in response")
		assert.Equal(t, plugin1.Name, foundPlugin.Name)
		assert.Equal(t, plugin1.Slug, foundPlugin.Slug)
		assert.True(t, foundPlugin.ServiceAccessAuthorized)
		assert.Contains(t, foundPlugin.ServiceScopes, models.ServiceScopePluginsRead)
	})
}

