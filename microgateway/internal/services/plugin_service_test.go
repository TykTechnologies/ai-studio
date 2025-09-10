// internal/services/plugin_service_test.go
package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) *gorm.DB {
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

func TestPluginService_CreatePlugin(t *testing.T) {
	db := setupTestDB(t)
	repo := database.NewRepository(db)
	service := NewPluginService(db, repo)

	tests := []struct {
		name    string
		request *CreatePluginRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid plugin",
			request: &CreatePluginRequest{
				Name:        "Test Plugin",
				Slug:        "test-plugin",
				Description: "A test plugin",
				Command:     "./test-plugin",
				Checksum:    "abc123",
				Config:      map[string]interface{}{"key": "value"},
				HookType:    "pre_auth",
				IsActive:    true,
			},
			wantErr: false,
		},
		{
			name: "empty name",
			request: &CreatePluginRequest{
				Name:     "",
				Slug:     "test-plugin",
				Command:  "./test-plugin",
				HookType: "pre_auth",
			},
			wantErr: true,
			errMsg:  "plugin name cannot be empty",
		},
		{
			name: "empty slug",
			request: &CreatePluginRequest{
				Name:     "Test Plugin",
				Slug:     "",
				Command:  "./test-plugin",
				HookType: "pre_auth",
			},
			wantErr: true,
			errMsg:  "plugin slug cannot be empty",
		},
		{
			name: "empty command",
			request: &CreatePluginRequest{
				Name:     "Test Plugin",
				Slug:     "test-plugin",
				Command:  "",
				HookType: "pre_auth",
			},
			wantErr: true,
			errMsg:  "plugin command cannot be empty",
		},
		{
			name: "invalid hook type",
			request: &CreatePluginRequest{
				Name:     "Test Plugin",
				Slug:     "test-plugin",
				Command:  "./test-plugin",
				HookType: "invalid",
			},
			wantErr: true,
			errMsg:  "invalid hook type: invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin, err := service.CreatePlugin(tt.request)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, plugin)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, plugin)
				assert.Equal(t, tt.request.Name, plugin.Name)
				assert.Equal(t, tt.request.Slug, plugin.Slug)
				assert.Equal(t, tt.request.Command, plugin.Command)
				assert.Equal(t, tt.request.HookType, plugin.HookType)
				assert.Equal(t, tt.request.IsActive, plugin.IsActive)
				assert.NotZero(t, plugin.ID)
			}
		})
	}
}

func TestPluginService_GetPlugin(t *testing.T) {
	db := setupTestDB(t)
	repo := database.NewRepository(db)
	service := NewPluginService(db, repo)

	// Create a test plugin
	plugin, err := service.CreatePlugin(&CreatePluginRequest{
		Name:     "Test Plugin",
		Slug:     "test-plugin",
		Command:  "./test-plugin",
		HookType: "pre_auth",
		IsActive: true,
	})
	require.NoError(t, err)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "existing plugin",
			id:      plugin.ID,
			wantErr: false,
		},
		{
			name:    "non-existing plugin",
			id:      999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.GetPlugin(tt.id)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, plugin.Name, result.Name)
				assert.Equal(t, plugin.Slug, result.Slug)
			}
		})
	}
}

func TestPluginService_ListPlugins(t *testing.T) {
	db := setupTestDB(t)
	repo := database.NewRepository(db)
	service := NewPluginService(db, repo)

	// Create test plugins with alternating active/inactive
	plugins := []*database.Plugin{}
	hookTypes := []string{"pre_auth", "auth", "post_auth", "on_response"}
	activeStates := []bool{true, false, true, false} // Alternate active/inactive
	
	for i, hookType := range hookTypes {
		plugin, err := service.CreatePlugin(&CreatePluginRequest{
			Name:     fmt.Sprintf("Plugin %d", i+1),
			Slug:     fmt.Sprintf("plugin-%d", i+1),
			Command:  fmt.Sprintf("./plugin-%d", i+1),
			HookType: hookType,
			IsActive: activeStates[i],
		})
		require.NoError(t, err)
		plugins = append(plugins, plugin)
	}

	tests := []struct {
		name        string
		page        int
		limit       int
		hookType    string
		isActive    bool
		expectedLen int
	}{
		{
			name:        "list all active",
			page:        1,
			limit:       10,
			hookType:    "",
			isActive:    true,
			expectedLen: 2, // plugins 1 and 3 are active (i%2 == 0)
		},
		{
			name:        "list all inactive",
			page:        1,
			limit:       10,
			hookType:    "",
			isActive:    false,
			expectedLen: 2, // plugins 2 and 4 are inactive
		},
		{
			name:        "filter by hook type",
			page:        1,
			limit:       10,
			hookType:    "pre_auth",
			isActive:    true,
			expectedLen: 1, // only plugin 1 is pre_auth and active
		},
		{
			name:        "pagination",
			page:        1,
			limit:       1,
			hookType:    "",
			isActive:    true,
			expectedLen: 1, // limit to 1 result
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, total, err := service.ListPlugins(tt.page, tt.limit, tt.hookType, tt.isActive)

			assert.NoError(t, err)
			assert.Len(t, result, tt.expectedLen)
			
			if tt.hookType == "" {
				if tt.isActive {
					assert.Equal(t, int64(2), total) // 2 active plugins
				} else {
					assert.Equal(t, int64(2), total) // 2 inactive plugins
				}
			}
		})
	}
}

func TestPluginService_UpdatePlugin(t *testing.T) {
	db := setupTestDB(t)
	repo := database.NewRepository(db)
	service := NewPluginService(db, repo)

	// Create a test plugin
	plugin, err := service.CreatePlugin(&CreatePluginRequest{
		Name:     "Test Plugin",
		Slug:     "test-plugin",
		Command:  "./test-plugin",
		HookType: "pre_auth",
		IsActive: true,
	})
	require.NoError(t, err)

	newName := "Updated Plugin"
	newDescription := "Updated description"
	newActive := false

	updateReq := &UpdatePluginRequest{
		Name:        &newName,
		Description: &newDescription,
		IsActive:    &newActive,
	}

	updatedPlugin, err := service.UpdatePlugin(plugin.ID, updateReq)
	assert.NoError(t, err)
	assert.NotNil(t, updatedPlugin)
	assert.Equal(t, newName, updatedPlugin.Name)
	assert.Equal(t, newDescription, updatedPlugin.Description)
	assert.Equal(t, newActive, updatedPlugin.IsActive)
	assert.Equal(t, plugin.Slug, updatedPlugin.Slug) // Unchanged
}

func TestPluginService_DeletePlugin(t *testing.T) {
	db := setupTestDB(t)
	repo := database.NewRepository(db)
	service := NewPluginService(db, repo)

	// Create a test plugin
	plugin, err := service.CreatePlugin(&CreatePluginRequest{
		Name:     "Test Plugin",
		Slug:     "test-plugin",
		Command:  "./test-plugin",
		HookType: "pre_auth",
		IsActive: true,
	})
	require.NoError(t, err)

	// Delete the plugin
	err = service.DeletePlugin(plugin.ID)
	assert.NoError(t, err)

	// Try to get the deleted plugin
	_, err = service.GetPlugin(plugin.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "plugin not found")

	// Try to delete non-existing plugin
	err = service.DeletePlugin(999)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "plugin not found")
}

func TestPluginService_LLMPluginAssociations(t *testing.T) {
	db := setupTestDB(t)
	repo := database.NewRepository(db)
	service := NewPluginService(db, repo)

	// Create an LLM
	llm := &database.LLM{
		Name:         "Test LLM",
		Slug:         "test-llm",
		Vendor:       "test",
		DefaultModel: "test-model",
	}
	err := db.Create(llm).Error
	require.NoError(t, err)

	// Create test plugins
	plugins := []*database.Plugin{}
	for i := 0; i < 3; i++ {
		plugin, err := service.CreatePlugin(&CreatePluginRequest{
			Name:     fmt.Sprintf("Plugin %d", i+1),
			Slug:     fmt.Sprintf("plugin-%d", i+1),
			Command:  fmt.Sprintf("./plugin-%d", i+1),
			HookType: "pre_auth",
			IsActive: true,
		})
		require.NoError(t, err)
		plugins = append(plugins, plugin)
	}

	// Test UpdateLLMPlugins
	pluginIDs := []uint{plugins[0].ID, plugins[1].ID}
	err = service.UpdateLLMPlugins(llm.ID, pluginIDs)
	assert.NoError(t, err)

	// Test GetPluginsForLLM
	llmPlugins, err := service.GetPluginsForLLM(llm.ID)
	assert.NoError(t, err)
	assert.Len(t, llmPlugins, 2)
	
	// Check order
	assert.Equal(t, plugins[0].ID, llmPlugins[0].ID)
	assert.Equal(t, plugins[1].ID, llmPlugins[1].ID)

	// Test updating with different plugins
	newPluginIDs := []uint{plugins[2].ID}
	err = service.UpdateLLMPlugins(llm.ID, newPluginIDs)
	assert.NoError(t, err)

	llmPlugins, err = service.GetPluginsForLLM(llm.ID)
	assert.NoError(t, err)
	assert.Len(t, llmPlugins, 1)
	assert.Equal(t, plugins[2].ID, llmPlugins[0].ID)
}

func TestPluginService_PluginSlugExists(t *testing.T) {
	db := setupTestDB(t)
	repo := database.NewRepository(db)
	service := NewPluginService(db, repo)

	// Create a test plugin
	_, err := service.CreatePlugin(&CreatePluginRequest{
		Name:     "Test Plugin",
		Slug:     "test-plugin",
		Command:  "./test-plugin",
		HookType: "pre_auth",
		IsActive: true,
	})
	require.NoError(t, err)

	// Test existing slug
	exists, err := service.PluginSlugExists("test-plugin")
	assert.NoError(t, err)
	assert.True(t, exists)

	// Test non-existing slug
	exists, err = service.PluginSlugExists("non-existing")
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestPluginService_ValidatePluginChecksum(t *testing.T) {
	db := setupTestDB(t)
	repo := database.NewRepository(db)
	service := NewPluginService(db, repo)

	// Create a temporary file for testing
	tmpFile, err := os.CreateTemp("", "plugin-test-*")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	content := "test plugin content"
	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)
	tmpFile.Close()

	// Calculate the expected checksum
	hasher := sha256.New()
	hasher.Write([]byte(content))
	expectedChecksum := hex.EncodeToString(hasher.Sum(nil))

	// Create plugin with correct checksum
	plugin, err := service.CreatePlugin(&CreatePluginRequest{
		Name:     "Test Plugin",
		Slug:     "test-plugin",
		Command:  "./test-plugin",
		Checksum: expectedChecksum,
		HookType: "pre_auth",
		IsActive: true,
	})
	require.NoError(t, err)

	// Test valid checksum
	err = service.ValidatePluginChecksum(plugin.ID, tmpFile.Name())
	assert.NoError(t, err)

	// Create plugin with wrong checksum
	plugin2, err := service.CreatePlugin(&CreatePluginRequest{
		Name:     "Test Plugin 2",
		Slug:     "test-plugin-2",
		Command:  "./test-plugin-2",
		Checksum: "wrongchecksum",
		HookType: "pre_auth",
		IsActive: true,
	})
	require.NoError(t, err)

	// Test invalid checksum
	err = service.ValidatePluginChecksum(plugin2.ID, tmpFile.Name())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")

	// Create plugin with no checksum
	plugin3, err := service.CreatePlugin(&CreatePluginRequest{
		Name:     "Test Plugin 3",
		Slug:     "test-plugin-3",
		Command:  "./test-plugin-3",
		HookType: "pre_auth",
		IsActive: true,
	})
	require.NoError(t, err)

	// Test no checksum (should pass)
	err = service.ValidatePluginChecksum(plugin3.ID, tmpFile.Name())
	assert.NoError(t, err)
}