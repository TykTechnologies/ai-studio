package grpc

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
			Command:                 "authorized-command",
			HookType:                models.HookTypeStudioUI,
			PluginType:              models.PluginTypeAIStudio,
			IsActive:                true,
			ServiceAccessAuthorized: true,
			ServiceScopes:           []string{models.ServiceScopePluginsRead},
		}

		plugin2 := &models.Plugin{
			Name:       "Another Plugin",
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
		assert.True(t, foundPlugin.ServiceAccessAuthorized)
		assert.Contains(t, foundPlugin.ServiceScopes, models.ServiceScopePluginsRead)
	})
}

func TestEnhancedFilteringIntegration(t *testing.T) {
	// Setup test database with all required models
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.Plugin{}, &models.LLM{}, &models.App{}, &models.Filter{}, &models.Datasource{})
	require.NoError(t, err)

	// Create services
	service := services.NewService(db)
	server := NewAIStudioManagementServer(service)

	t.Run("app filtering with namespace and is_active", func(t *testing.T) {
		// Create test apps with different namespaces and active states
		app1 := &models.App{
			Name:      "Test App 1",
			Namespace: "production",
			IsActive:  true,
		}
		app2 := &models.App{
			Name:      "Test App 2",
			Namespace: "",
			IsActive:  true,
		}
		app3 := &models.App{
			Name:      "Test App 3",
			Namespace: "production",
			IsActive:  false,
		}

		err := app1.Create(db)
		require.NoError(t, err)
		err = app2.Create(db)
		require.NoError(t, err)
		err = app3.Create(db)
		require.NoError(t, err)

		// Ensure app3 is definitely inactive (GORM default might override)
		err = db.Model(app3).Update("is_active", false).Error
		require.NoError(t, err)

		// Create authorized plugin for testing
		plugin := &models.Plugin{
			Name:                    "Test Plugin Apps",
			Command:                 "test-command",
			HookType:                models.HookTypeStudioUI,
			PluginType:              models.PluginTypeAIStudio,
			IsActive:                true,
			ServiceAccessAuthorized: true,
			ServiceScopes:           []string{models.ServiceScopeAppsRead},
		}
		err = db.Create(plugin).Error
		require.NoError(t, err)

		ctx := SetPluginInContext(context.Background(), plugin)

		// Test 1: Filter by namespace (production)
		req := &pb.ListAppsRequest{
			Context: &pb.PluginContext{
				PluginId:    uint32(plugin.ID),
				MethodScope: models.ServiceScopeAppsRead,
			},
			Namespace: "production",
			Page:      1,
			Limit:     10,
		}

		resp, err := server.ListApps(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, int64(2), resp.TotalCount) // app1 and app3 (both in production namespace)

		// Test 2: Filter by namespace and active status
		isActive := true
		req.IsActive = &isActive

		resp, err = server.ListApps(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, int64(1), resp.TotalCount) // Only app1 (active in production)

		// Test 3: All namespaces (should return all apps)
		req.Namespace = ""
		req.IsActive = nil

		resp, err = server.ListApps(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, int64(3), resp.TotalCount) // All apps
	})

	t.Run("datasource filtering with is_active and user_id", func(t *testing.T) {
		// Create test datasources
		ds1 := &models.Datasource{
			Name:   "Active Datasource",
			UserID: 1,
			Active: true,
		}
		ds2 := &models.Datasource{
			Name:   "Inactive Datasource",
			UserID: 1,
			Active: false,
		}
		ds3 := &models.Datasource{
			Name:   "Other User Datasource",
			UserID: 2,
			Active: true,
		}

		err := ds1.Create(db)
		require.NoError(t, err)
		err = ds2.Create(db)
		require.NoError(t, err)
		err = ds3.Create(db)
		require.NoError(t, err)

		// Create authorized plugin for testing
		plugin := &models.Plugin{
			Name:                    "Test Plugin Datasources",
			Command:                 "test-command",
			HookType:                models.HookTypeStudioUI,
			PluginType:              models.PluginTypeAIStudio,
			IsActive:                true,
			ServiceAccessAuthorized: true,
			ServiceScopes:           []string{models.ServiceScopeDatasourcesRead},
		}
		err = db.Create(plugin).Error
		require.NoError(t, err)

		ctx := SetPluginInContext(context.Background(), plugin)

		// Test 1: Filter by active status only
		isActive := true
		req := &pb.ListDatasourcesRequest{
			Context: &pb.PluginContext{
				PluginId:    uint32(plugin.ID),
				MethodScope: models.ServiceScopeDatasourcesRead,
			},
			IsActive: &isActive,
			Page:     1,
			Limit:    10,
		}

		resp, err := server.ListDatasources(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, int64(2), resp.TotalCount) // ds1 and ds3 (both active)

		// Test 2: Filter by user ID only
		req.IsActive = nil
		req.UserId = "1"

		resp, err = server.ListDatasources(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, int64(2), resp.TotalCount) // ds1 and ds2 (both belong to user 1)

		// Test 3: Filter by both active status and user ID
		req.IsActive = &isActive
		req.UserId = "1"

		resp, err = server.ListDatasources(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, int64(1), resp.TotalCount) // Only ds1 (active and belongs to user 1)
	})

	t.Run("error handling with proper error types", func(t *testing.T) {
		// Create authorized plugin for testing
		plugin := &models.Plugin{
			Name:                    "Test Plugin Errors",
			Command:                 "test-command",
			HookType:                models.HookTypeStudioUI,
			PluginType:              models.PluginTypeAIStudio,
			IsActive:                true,
			ServiceAccessAuthorized: true,
			ServiceScopes:           []string{models.ServiceScopeAppsRead, models.ServiceScopeDatasourcesRead, models.ServiceScopeFiltersRead},
		}
		err = db.Create(plugin).Error
		require.NoError(t, err)

		ctx := SetPluginInContext(context.Background(), plugin)

		// Test 1: App not found should return NotFound error
		req := &pb.GetAppRequest{
			Context: &pb.PluginContext{
				PluginId:    uint32(plugin.ID),
				MethodScope: models.ServiceScopeAppsRead,
			},
			AppId: 99999, // Non-existent ID
		}

		_, err = server.GetApp(ctx, req)
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())

		// Test 2: Datasource not found should return NotFound error
		dsReq := &pb.GetDatasourceRequest{
			Context: &pb.PluginContext{
				PluginId:    uint32(plugin.ID),
				MethodScope: models.ServiceScopeDatasourcesRead,
			},
			DatasourceId: 99999, // Non-existent ID
		}

		_, err = server.GetDatasource(ctx, dsReq)
		require.Error(t, err)
		st, ok = status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())

		// Test 3: Filter not found should return NotFound error
		filterReq := &pb.GetFilterRequest{
			Context: &pb.PluginContext{
				PluginId:    uint32(plugin.ID),
				MethodScope: models.ServiceScopeFiltersRead,
			},
			FilterId: 99999, // Non-existent ID
		}

		_, err = server.GetFilter(ctx, filterReq)
		require.Error(t, err)
		st, ok = status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.NotFound, st.Code())
	})

	t.Run("filter LLM relationships are properly queried", func(t *testing.T) {
		// Create test LLM and Filter with relationship
		llm := &models.LLM{
			Name:         "Test LLM",
			Vendor:       models.OPENAI,
			APIKey:       "test-key",
			APIEndpoint:  "https://api.openai.com/v1",
			DefaultModel: "gpt-4",
			Active:       true,
		}
		err := llm.Create(db)
		require.NoError(t, err)

		filter := &models.Filter{
			Name:        "Test Filter",
			Description: "Test filter description",
			Script:      []byte("result = true"),
		}
		err = filter.Create(db)
		require.NoError(t, err)

		// Create LLM-Filter relationship
		err = db.Table("llm_filters").Create(map[string]interface{}{
			"llm_id":    llm.ID,
			"filter_id": filter.ID,
		}).Error
		require.NoError(t, err)

		// Create authorized plugin
		plugin := &models.Plugin{
			Name:                    "Test Plugin Filters",
			Command:                 "test-command",
			HookType:                models.HookTypeStudioUI,
			PluginType:              models.PluginTypeAIStudio,
			IsActive:                true,
			ServiceAccessAuthorized: true,
			ServiceScopes:           []string{models.ServiceScopeFiltersRead},
		}
		err = db.Create(plugin).Error
		require.NoError(t, err)

		ctx := SetPluginInContext(context.Background(), plugin)

		// Test GetFilter with LLM relationships
		req := &pb.GetFilterRequest{
			Context: &pb.PluginContext{
				PluginId:    uint32(plugin.ID),
				MethodScope: models.ServiceScopeFiltersRead,
			},
			FilterId: uint32(filter.ID),
		}

		resp, err := server.GetFilter(ctx, req)
		require.NoError(t, err)
		assert.NotNil(t, resp.Filter)
		assert.Equal(t, filter.Name, resp.Filter.Name)
		assert.Equal(t, filter.Namespace, resp.Filter.Namespace)

		// Verify LLM relationship is properly queried
		assert.Len(t, resp.Filter.LlmIds, 1)
		assert.Equal(t, uint32(llm.ID), resp.Filter.LlmIds[0])
	})
}

