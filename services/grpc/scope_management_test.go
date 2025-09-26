package grpc

import (
	"encoding/json"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestManifestScopeExtraction(t *testing.T) {
	t.Run("extract service scopes from manifest", func(t *testing.T) {
		// Create test manifest with service scopes
		manifest := &models.PluginManifest{
			ID:          "test.plugin",
			Version:     "1.0.0",
			Name:        "Test Plugin",
			Description: "Test plugin for scope extraction",
			Permissions: struct {
				KV       []string `json:"kv"`
				RPC      []string `json:"rpc"`
				Routes   []string `json:"routes"`
				UI       []string `json:"ui"`
				Services []string `json:"services"`
			}{
				KV:       []string{"read", "write"},
				RPC:      []string{"call"},
				UI:       []string{"sidebar.register"},
				Services: []string{"analytics.read", "plugins.config", "llms.read"},
			},
		}

		// Test GetServiceScopes method
		scopes := manifest.GetServiceScopes()
		expected := []string{"analytics.read", "plugins.config", "llms.read"}
		assert.Equal(t, expected, scopes)

		// Test HasServiceScope method
		assert.True(t, manifest.HasServiceScope("analytics.read"))
		assert.True(t, manifest.HasServiceScope("plugins.config"))
		assert.True(t, manifest.HasServiceScope("llms.read"))
		assert.False(t, manifest.HasServiceScope("plugins.write"))
		assert.False(t, manifest.HasServiceScope("nonexistent.scope"))
	})

	t.Run("manifest with no service scopes", func(t *testing.T) {
		manifest := &models.PluginManifest{
			ID:      "test.plugin.no.scopes",
			Version: "1.0.0",
			Name:    "Plugin Without Scopes",
			Permissions: struct {
				KV       []string `json:"kv"`
				RPC      []string `json:"rpc"`
				Routes   []string `json:"routes"`
				UI       []string `json:"ui"`
				Services []string `json:"services"`
			}{
				UI:       []string{"sidebar.register"},
				Services: []string{}, // Empty services
			},
		}

		scopes := manifest.GetServiceScopes()
		assert.Empty(t, scopes)
		assert.False(t, manifest.HasServiceScope("analytics.read"))
	})

	t.Run("manifest validation with service scopes", func(t *testing.T) {
		// Test valid manifest
		validManifest := &models.PluginManifest{
			ID:      "valid.plugin",
			Version: "1.0.0",
			Name:    "Valid Plugin",
			Permissions: struct {
				KV       []string `json:"kv"`
				RPC      []string `json:"rpc"`
				Routes   []string `json:"routes"`
				UI       []string `json:"ui"`
				Services []string `json:"services"`
			}{
				Services: []string{"analytics.read"},
			},
		}

		err := validManifest.ValidateManifest()
		assert.NoError(t, err)

		// Test invalid manifest (missing required fields)
		invalidManifest := &models.PluginManifest{
			// Missing ID, Version, Name
			Permissions: struct {
				KV       []string `json:"kv"`
				RPC      []string `json:"rpc"`
				Routes   []string `json:"routes"`
				UI       []string `json:"ui"`
				Services []string `json:"services"`
			}{
				Services: []string{"analytics.read"},
			},
		}

		err = invalidManifest.ValidateManifest()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "ID is required")
	})
}

func TestServiceScopeConstants(t *testing.T) {
	t.Run("service scope constants are defined", func(t *testing.T) {
		// Verify all service scope constants exist
		expectedScopes := []string{
			models.ServiceScopePluginsRead,
			models.ServiceScopePluginsWrite,
			models.ServiceScopePluginsConfig,
			models.ServiceScopeLLMsRead,
			models.ServiceScopeLLMsWrite,
			models.ServiceScopeLLMsConfig,
			models.ServiceScopeAnalyticsRead,
			models.ServiceScopeAppsRead,
			models.ServiceScopeAppsWrite,
			models.ServiceScopeToolsRead,
			models.ServiceScopeToolsWrite,
			models.ServiceScopeSystemRead,
		}

		// Ensure all constants are not empty
		for _, scope := range expectedScopes {
			assert.NotEmpty(t, scope, "Service scope constant should not be empty")
			assert.Contains(t, scope, ".", "Service scope should contain domain.action format")
		}

		// Test specific values
		assert.Equal(t, "plugins.read", models.ServiceScopePluginsRead)
		assert.Equal(t, "analytics.read", models.ServiceScopeAnalyticsRead)
		assert.Equal(t, "llms.config", models.ServiceScopeLLMsConfig)
	})
}

func TestPluginServiceAccessLifecycle(t *testing.T) {
	// Setup test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.Plugin{})
	require.NoError(t, err)

	t.Run("complete plugin service access lifecycle", func(t *testing.T) {
		// 1. Create plugin without service access
		plugin := &models.Plugin{
			Name:                    "Lifecycle Test Plugin",
			Slug:                    "lifecycle-test-plugin",
			Command:                 "test-command",
			HookType:                models.HookTypeStudioUI,
			PluginType:              models.PluginTypeAIStudio,
			IsActive:                true,
			ServiceAccessAuthorized: false,
			ServiceScopes:           []string{}, // Initially empty
		}

		err := db.Create(plugin).Error
		require.NoError(t, err)

		// 2. Simulate manifest declaring service scopes
		manifestScopes := []string{"analytics.read", "plugins.config"}
		plugin.ServiceScopes = manifestScopes
		err = db.Save(plugin).Error
		require.NoError(t, err)

		// 3. Verify initial state - scopes declared but not authorized
		assert.False(t, plugin.HasServiceAccess())
		assert.True(t, plugin.HasServiceScope("analytics.read"))
		assert.True(t, plugin.HasServiceScope("plugins.config"))
		assert.False(t, plugin.HasServiceScope("plugins.write"))

		// 4. Admin authorizes service access
		err = plugin.AuthorizeServiceAccess(db, manifestScopes)
		require.NoError(t, err)

		// Reload from database
		var reloadedPlugin models.Plugin
		err = db.First(&reloadedPlugin, plugin.ID).Error
		require.NoError(t, err)

		// 5. Verify authorization is now active
		assert.True(t, reloadedPlugin.HasServiceAccess())
		assert.Equal(t, manifestScopes, reloadedPlugin.ServiceScopes)

		// 6. Admin revokes service access
		err = reloadedPlugin.RevokeServiceAccess(db)
		require.NoError(t, err)

		// Reload again
		err = db.First(&reloadedPlugin, plugin.ID).Error
		require.NoError(t, err)

		// 7. Verify access is revoked
		assert.False(t, reloadedPlugin.HasServiceAccess())
		assert.Empty(t, reloadedPlugin.ServiceScopes)
	})

	t.Run("scope validation", func(t *testing.T) {
		plugin := &models.Plugin{
			Name:                    "Scope Validation Plugin",
			Slug:                    "scope-validation-plugin",
			Command:                 "test-command",
			HookType:                models.HookTypeStudioUI,
			PluginType:              models.PluginTypeAIStudio,
			IsActive:                true,
			ServiceAccessAuthorized: true,
			ServiceScopes:           []string{"analytics.read", "plugins.read"},
		}

		err := db.Create(plugin).Error
		require.NoError(t, err)

		// Test scope checking
		assert.True(t, plugin.HasServiceScope("analytics.read"))
		assert.True(t, plugin.HasServiceScope("plugins.read"))
		assert.False(t, plugin.HasServiceScope("plugins.write"))
		assert.False(t, plugin.HasServiceScope("llms.write"))
		assert.False(t, plugin.HasServiceScope(""))
	})
}

func TestPluginManifestJSONSerialization(t *testing.T) {
	t.Run("manifest with service scopes serializes correctly", func(t *testing.T) {
		// Create manifest with service scopes
		manifest := &models.PluginManifest{
			ID:          "serialization.test",
			Version:     "1.0.0",
			Name:        "Serialization Test Plugin",
			Description: "Test plugin for JSON serialization",
			Permissions: struct {
				KV       []string `json:"kv"`
				RPC      []string `json:"rpc"`
				Routes   []string `json:"routes"`
				UI       []string `json:"ui"`
				Services []string `json:"services"`
			}{
				Services: []string{"analytics.read", "plugins.config"},
				UI:       []string{"sidebar.register"},
			},
		}

		// Serialize to JSON
		manifestJSON, err := json.Marshal(manifest)
		require.NoError(t, err)

		// Verify JSON contains service scopes
		assert.Contains(t, string(manifestJSON), "analytics.read")
		assert.Contains(t, string(manifestJSON), "plugins.config")
		assert.Contains(t, string(manifestJSON), "services")

		// Deserialize back
		var parsedManifest models.PluginManifest
		err = json.Unmarshal(manifestJSON, &parsedManifest)
		require.NoError(t, err)

		// Verify deserialized manifest has correct scopes
		assert.Equal(t, manifest.GetServiceScopes(), parsedManifest.GetServiceScopes())
		assert.True(t, parsedManifest.HasServiceScope("analytics.read"))
		assert.True(t, parsedManifest.HasServiceScope("plugins.config"))
	})
}

func TestScopeAuthorizationWorkflow(t *testing.T) {
	t.Run("unauthorized plugin cannot be used", func(t *testing.T) {
		plugin := &models.Plugin{
			ServiceAccessAuthorized: false,
			ServiceScopes:           []string{"analytics.read"},
		}

		// Plugin should not have service access
		assert.False(t, plugin.HasServiceAccess())
		assert.True(t, plugin.HasServiceScope("analytics.read"))
	})

	t.Run("authorized plugin with correct scope can be used", func(t *testing.T) {
		plugin := &models.Plugin{
			ServiceAccessAuthorized: true,
			ServiceScopes:           []string{"analytics.read", "plugins.read"},
		}

		// Plugin should have service access and correct scopes
		assert.True(t, plugin.HasServiceAccess())
		assert.True(t, plugin.HasServiceScope("analytics.read"))
		assert.True(t, plugin.HasServiceScope("plugins.read"))
		assert.False(t, plugin.HasServiceScope("plugins.write"))
	})

	t.Run("scope enforcement in auth interceptor", func(t *testing.T) {
		// Test the scope mapping function directly
		tests := []struct {
			method        string
			expectedScope string
		}{
			{
				method:        "/ai_studio_management.AIStudioManagementService/GetAnalyticsSummary",
				expectedScope: models.ServiceScopeAnalyticsRead,
			},
			{
				method:        "/ai_studio_management.AIStudioManagementService/UpdatePluginConfig",
				expectedScope: models.ServiceScopePluginsConfig,
			},
			{
				method:        "/ai_studio_management.AIStudioManagementService/ListLLMs",
				expectedScope: models.ServiceScopeLLMsRead,
			},
			{
				method:        "/unknown/service/method",
				expectedScope: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.method, func(t *testing.T) {
				scope := extractScopeFromMethod(tt.method)
				assert.Equal(t, tt.expectedScope, scope)
			})
		}
	})
}