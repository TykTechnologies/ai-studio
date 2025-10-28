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
}

func TestPluginService_DeletePlugin(t *testing.T) {
	db := setupTestDB(t)
	repo := database.NewRepository(db)
	service := NewPluginService(db, repo)

	// Create a test plugin
	plugin, err := service.CreatePlugin(&CreatePluginRequest{
		Name:     "Test Plugin",
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

// TestPluginService_ListPluginsComprehensiveFiltering tests all filtering scenarios
func TestPluginService_ListPluginsComprehensiveFiltering(t *testing.T) {
	db := setupTestDB(t)
	repo := database.NewRepository(db)
	service := NewPluginService(db, repo)

	// Create comprehensive test data set
	testPlugins := []struct {
		name      string
		slug      string
		hookType  string
		isActive  bool
		namespace string
	}{
		// Different hook types with varying active states and namespaces
		{"PreAuth Active Global", "preauth-active-global", "pre_auth", true, ""},
		{"PreAuth Inactive Global", "preauth-inactive-global", "pre_auth", false, ""},
		{"PreAuth Active TenantA", "preauth-active-tenant-a", "pre_auth", true, "tenant-a"},
		{"Auth Active Global", "auth-active-global", "auth", true, ""},
		{"Auth Inactive TenantA", "auth-inactive-tenant-a", "auth", false, "tenant-a"},
		{"PostAuth Active TenantB", "postauth-active-tenant-b", "post_auth", true, "tenant-b"},
		{"PostAuth Inactive Global", "postauth-inactive-global", "post_auth", false, ""},
		{"OnResponse Active Global", "onresponse-active-global", "on_response", true, ""},
		{"OnResponse Active TenantA", "onresponse-active-tenant-a", "on_response", true, "tenant-a"},
		{"OnResponse Inactive TenantB", "onresponse-inactive-tenant-b", "on_response", false, "tenant-b"},
	}

	createdPlugins := make([]*database.Plugin, 0)

	// Create all test plugins
	for _, testPlugin := range testPlugins {
		plugin, err := service.CreatePlugin(&CreatePluginRequest{
			Name:     testPlugin.name,
			Command:  fmt.Sprintf("./bin/%s", testPlugin.slug),
			HookType: testPlugin.hookType,
			IsActive: testPlugin.isActive,
		})
		// Set namespace manually after creation since CreatePluginRequest doesn't have namespace in microgateway
		if err == nil && testPlugin.namespace != "" {
			plugin.Namespace = testPlugin.namespace
			db.Save(plugin)
		}
		require.NoError(t, err, "Failed to create plugin %s", testPlugin.name)
		createdPlugins = append(createdPlugins, plugin)
	}

	// Test 1: Filter by hook type only (active plugins)
	t.Run("Filter by hook type", func(t *testing.T) {
		hookTypeTests := []struct {
			hookType      string
			expectedCount int
		}{
			{"pre_auth", 2},    // PreAuth Active Global + PreAuth Active TenantA
			{"auth", 1},        // Auth Active Global
			{"post_auth", 1},   // PostAuth Active TenantB
			{"on_response", 2}, // OnResponse Active Global + OnResponse Active TenantA
			{"nonexistent", 0}, // No plugins with this hook type
		}

		for _, test := range hookTypeTests {
			result, total, err := service.ListPlugins(1, 10, test.hookType, true)
			assert.NoError(t, err)
			assert.Equal(t, test.expectedCount, len(result), "Hook type %s should return %d active plugins", test.hookType, test.expectedCount)
			assert.Equal(t, int64(test.expectedCount), total)

			// Verify all returned plugins have correct hook type and are active
			for _, plugin := range result {
				assert.Equal(t, test.hookType, plugin.HookType)
				assert.True(t, plugin.IsActive)
			}
		}
	})

	// Test 2: Filter by active status only
	t.Run("Filter by active status", func(t *testing.T) {
		// Test active plugins
		result, total, err := service.ListPlugins(1, 20, "", true)
		assert.NoError(t, err)
		assert.Equal(t, 6, len(result)) // 6 active plugins
		assert.Equal(t, int64(6), total)
		for _, plugin := range result {
			assert.True(t, plugin.IsActive)
		}

		// Test inactive plugins
		result, total, err = service.ListPlugins(1, 20, "", false)
		assert.NoError(t, err)
		assert.Equal(t, 4, len(result)) // 4 inactive plugins
		assert.Equal(t, int64(4), total)
		for _, plugin := range result {
			assert.False(t, plugin.IsActive)
		}
	})

	// Test 3: Combined filters (hook type + active status)
	t.Run("Combined filters: hook type and active status", func(t *testing.T) {
		tests := []struct {
			hookType      string
			isActive      bool
			expectedCount int
			description   string
		}{
			{"pre_auth", true, 2, "Active pre_auth plugins"},
			{"pre_auth", false, 1, "Inactive pre_auth plugins"},
			{"auth", true, 1, "Active auth plugins"},
			{"auth", false, 1, "Inactive auth plugins"},
			{"post_auth", true, 1, "Active post_auth plugins"},
			{"post_auth", false, 1, "Inactive post_auth plugins"},
			{"on_response", true, 2, "Active on_response plugins"},
			{"on_response", false, 1, "Inactive on_response plugins"},
		}

		for _, test := range tests {
			result, total, err := service.ListPlugins(1, 10, test.hookType, test.isActive)
			assert.NoError(t, err)
			assert.Equal(t, test.expectedCount, len(result), test.description)
			assert.Equal(t, int64(test.expectedCount), total)

			for _, plugin := range result {
				assert.Equal(t, test.hookType, plugin.HookType)
				assert.Equal(t, test.isActive, plugin.IsActive)
			}
		}
	})

	// Test 4: Pagination scenarios
	t.Run("Pagination with filters", func(t *testing.T) {
		// Test first page with limit 3
		result, total, err := service.ListPlugins(1, 3, "", true)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(result))
		assert.Equal(t, int64(6), total) // Total active plugins

		// Test second page
		result, total, err = service.ListPlugins(2, 3, "", true)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(result))
		assert.Equal(t, int64(6), total)

		// Test third page (should have no results beyond total)
		result, total, err = service.ListPlugins(3, 3, "", true)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(result)) // No more results
		assert.Equal(t, int64(6), total)

		// Test pagination with hook type filter
		result, total, err = service.ListPlugins(1, 1, "pre_auth", true)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(result))
		assert.Equal(t, int64(2), total) // Total pre_auth active plugins
		assert.Equal(t, "pre_auth", result[0].HookType)
		assert.True(t, result[0].IsActive)

		// Second page of pre_auth plugins
		result, total, err = service.ListPlugins(2, 1, "pre_auth", true)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(result))
		assert.Equal(t, int64(2), total)
		assert.Equal(t, "pre_auth", result[0].HookType)
		assert.True(t, result[0].IsActive)
	})

	// Test 5: Edge cases
	t.Run("Edge cases", func(t *testing.T) {
		// Test invalid page (0 or negative)
		result, total, err := service.ListPlugins(0, 10, "", true)
		assert.NoError(t, err)
		// Should handle gracefully - implementation dependent

		// Test very large limit
		result, total, err = service.ListPlugins(1, 1000, "", true)
		assert.NoError(t, err)
		assert.Equal(t, 6, len(result)) // Should return all active plugins
		assert.Equal(t, int64(6), total)

		// Test page beyond available data
		result, total, err = service.ListPlugins(100, 10, "", true)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(result))
		assert.Equal(t, int64(6), total) // Total count should still be correct
	})

	// Test 6: Verify namespace handling (note: this tests the database layer)
	t.Run("Namespace verification in created plugins", func(t *testing.T) {
		// Verify that our test plugins were created with correct namespaces
		for i, testPlugin := range testPlugins {
			assert.Equal(t, testPlugin.namespace, createdPlugins[i].Namespace, "Plugin %s should have namespace %s", testPlugin.name, testPlugin.namespace)
		}

		// Count plugins by namespace (direct database query)
		var globalCount, tenantACount, tenantBCount int64
		db.Model(&database.Plugin{}).Where("namespace = ?", "").Count(&globalCount)
		db.Model(&database.Plugin{}).Where("namespace = ?", "tenant-a").Count(&tenantACount)
		db.Model(&database.Plugin{}).Where("namespace = ?", "tenant-b").Count(&tenantBCount)

		assert.Equal(t, int64(5), globalCount, "Should have 5 global plugins")
		assert.Equal(t, int64(3), tenantACount, "Should have 3 tenant-a plugins")
		assert.Equal(t, int64(2), tenantBCount, "Should have 2 tenant-b plugins")
	})
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
		Command:  "./test-plugin-3",
		HookType: "pre_auth",
		IsActive: true,
	})
	require.NoError(t, err)

	// Test no checksum (should pass)
	err = service.ValidatePluginChecksum(plugin3.ID, tmpFile.Name())
	assert.NoError(t, err)
}
