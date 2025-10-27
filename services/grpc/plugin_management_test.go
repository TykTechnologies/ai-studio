package grpc

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPluginManagementServer(t *testing.T) {
	// Simple test focusing on core functionality
	t.Run("server creation", func(t *testing.T) {
		// Setup test database
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)

		// Create services
		service := services.NewService(db)
		server := NewPluginManagementServer(service.PluginService)

		assert.NotNil(t, server)
		assert.NotNil(t, server.pluginService)
	})
}

func TestConvertPluginToPB(t *testing.T) {
	plugin := &models.Plugin{
		ID:        123,
		CreatedAt: testTime,
		UpdatedAt: testTime,
		Name:                    "Test Plugin",
		Description:             "Test Description",
		Command:                 "test-command",
		HookType:                models.HookTypeStudioUI,
		PluginType:              models.PluginTypeAIStudio,
		IsActive:                true,
		Namespace:               "test-namespace",
		ServiceAccessAuthorized: true,
		ServiceScopes:           []string{models.ServiceScopePluginsRead, models.ServiceScopeAnalyticsRead},
		Config: map[string]interface{}{
			"key1": "value1",
			"key2": 42,
		},
	}

	pbPlugin := convertPluginToPB(plugin)

	assert.Equal(t, uint32(123), pbPlugin.Id)
	assert.Equal(t, "Test Plugin", pbPlugin.Name)
	assert.Equal(t, "Test Description", pbPlugin.Description)
	assert.Equal(t, "test-command", pbPlugin.Command)
	assert.Equal(t, models.HookTypeStudioUI, pbPlugin.HookType)
	assert.Equal(t, models.PluginTypeAIStudio, pbPlugin.PluginType)
	assert.True(t, pbPlugin.IsActive)
	assert.Equal(t, "test-namespace", pbPlugin.Namespace)
	assert.True(t, pbPlugin.ServiceAccessAuthorized)
	assert.Equal(t, []string{models.ServiceScopePluginsRead, models.ServiceScopeAnalyticsRead}, pbPlugin.ServiceScopes)

	// Verify config JSON conversion
	assert.Contains(t, pbPlugin.ConfigJson, "key1")
	assert.Contains(t, pbPlugin.ConfigJson, "value1")
	assert.Contains(t, pbPlugin.ConfigJson, "key2")

	// Verify timestamps
	assert.NotNil(t, pbPlugin.CreatedAt)
	assert.NotNil(t, pbPlugin.UpdatedAt)
}

// Test time constant
var testTime = time.Now()