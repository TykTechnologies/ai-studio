package grpc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto-migrate the models
	err = db.AutoMigrate(&models.Plugin{})
	require.NoError(t, err)

	return db
}

// createTestPlugin creates a test plugin with specified service access settings
func createTestPlugin(t *testing.T, db *gorm.DB, authorized bool, scopes []string) *models.Plugin {
	// Generate unique slug to avoid conflicts
	slug := fmt.Sprintf("test-plugin-%d", time.Now().UnixNano())

	plugin := &models.Plugin{
		Name:                    "Test Plugin",
		Slug:                    slug,
		Command:                 "test-command",
		HookType:                models.HookTypeStudioUI,
		PluginType:              models.PluginTypeAIStudio,
		IsActive:                true,
		ServiceAccessAuthorized: authorized,
		ServiceScopes:           scopes,
	}

	err := db.Create(plugin).Error
	require.NoError(t, err)

	return plugin
}

func TestPluginAuthInterceptor(t *testing.T) {
	t.Run("no plugin ID in context", func(t *testing.T) {
		// Test case: no plugin ID in context should fail
		ctx := context.Background() // No plugin ID
		pluginID := GetPluginIDFromContext(ctx)
		assert.Equal(t, uint(0), pluginID, "Should return 0 when no plugin ID in context")
	})

	t.Run("plugin ID context operations", func(t *testing.T) {
		// Test setting and getting plugin ID from context
		originalCtx := context.Background()
		ctx := SetPluginIDInContext(originalCtx, 123)

		retrievedID := GetPluginIDFromContext(ctx)
		assert.Equal(t, uint(123), retrievedID, "Should retrieve correct plugin ID from context")
	})
}

func TestExtractScopeFromMethod(t *testing.T) {
	tests := []struct {
		fullMethod    string
		expectedScope string
	}{
		{
			fullMethod:    "/ai_studio_management.AIStudioManagementService/ListPlugins",
			expectedScope: models.ServiceScopePluginsRead,
		},
		{
			fullMethod:    "/ai_studio_management.AIStudioManagementService/UpdatePluginConfig",
			expectedScope: models.ServiceScopePluginsConfig,
		},
		{
			fullMethod:    "/ai_studio_management.AIStudioManagementService/ListLLMs",
			expectedScope: models.ServiceScopeLLMsRead,
		},
		{
			fullMethod:    "/ai_studio_management.AIStudioManagementService/GetAnalyticsSummary",
			expectedScope: models.ServiceScopeAnalyticsRead,
		},
		{
			fullMethod:    "/unknown/service/method",
			expectedScope: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.fullMethod, func(t *testing.T) {
			scope := extractScopeFromMethod(tt.fullMethod)
			assert.Equal(t, tt.expectedScope, scope)
		})
	}
}

func TestPluginServiceAccess(t *testing.T) {
	db := setupTestDB(t)

	t.Run("HasServiceAccess", func(t *testing.T) {
		authorizedPlugin := createTestPlugin(t, db, true, []string{models.ServiceScopePluginsRead})
		unauthorizedPlugin := createTestPlugin(t, db, false, []string{})

		assert.True(t, authorizedPlugin.HasServiceAccess())
		assert.False(t, unauthorizedPlugin.HasServiceAccess())
	})

	t.Run("HasServiceScope", func(t *testing.T) {
		plugin := createTestPlugin(t, db, true, []string{
			models.ServiceScopePluginsRead,
			models.ServiceScopeAnalyticsRead,
		})

		assert.True(t, plugin.HasServiceScope(models.ServiceScopePluginsRead))
		assert.True(t, plugin.HasServiceScope(models.ServiceScopeAnalyticsRead))
		assert.False(t, plugin.HasServiceScope(models.ServiceScopePluginsWrite))
	})

	t.Run("AuthorizeServiceAccess", func(t *testing.T) {
		plugin := createTestPlugin(t, db, false, []string{})

		scopes := []string{models.ServiceScopePluginsRead, models.ServiceScopeAnalyticsRead}
		err := plugin.AuthorizeServiceAccess(db, scopes)
		require.NoError(t, err)

		// Reload from database
		var reloadedPlugin models.Plugin
		err = db.First(&reloadedPlugin, plugin.ID).Error
		require.NoError(t, err)

		assert.True(t, reloadedPlugin.ServiceAccessAuthorized)
		assert.Equal(t, scopes, reloadedPlugin.ServiceScopes)
	})

	t.Run("RevokeServiceAccess", func(t *testing.T) {
		plugin := createTestPlugin(t, db, true, []string{models.ServiceScopePluginsRead})

		err := plugin.RevokeServiceAccess(db)
		require.NoError(t, err)

		// Reload from database
		var reloadedPlugin models.Plugin
		err = db.First(&reloadedPlugin, plugin.ID).Error
		require.NoError(t, err)

		assert.False(t, reloadedPlugin.ServiceAccessAuthorized)
		assert.Empty(t, reloadedPlugin.ServiceScopes)
	})
}

// mockUnaryServerInfo implements grpc.UnaryServerInfo for testing
type mockUnaryServerInfo struct {
	fullMethod string
}

func (m *mockUnaryServerInfo) FullMethod() string {
	return m.fullMethod
}

func (m *mockUnaryServerInfo) Server() interface{} {
	return nil
}