package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// --- extractUserGroupNames tests ---

func TestExtractUserGroupNames(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("extracts groups from user context", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		user := &models.User{
			Email:   "test@example.com",
			Name:    "Test User",
			IsAdmin: false,
			Groups: []models.Group{
				{Name: "engineering"},
				{Name: "support"},
			},
		}
		c.Set("user", user)

		groups := extractUserGroupNames(c)
		assert.Equal(t, []string{"engineering", "support"}, groups)
	})

	t.Run("returns nil when no user in context", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		groups := extractUserGroupNames(c)
		assert.Nil(t, groups)
	})

	t.Run("returns empty slice for user with no groups", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		user := &models.User{
			Email:  "nogroups@example.com",
			Groups: []models.Group{},
		}
		c.Set("user", user)

		groups := extractUserGroupNames(c)
		assert.Empty(t, groups)
	})

	t.Run("returns nil for wrong type in context", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Set("user", "not a user object")

		groups := extractUserGroupNames(c)
		assert.Nil(t, groups)
	})

	t.Run("handles user with many groups", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		user := &models.User{
			Email: "multi@example.com",
			Groups: []models.Group{
				{Name: "engineering"},
				{Name: "support"},
				{Name: "premium"},
				{Name: "beta-testers"},
			},
		}
		c.Set("user", user)

		groups := extractUserGroupNames(c)
		assert.Len(t, groups, 4)
		assert.Contains(t, groups, "engineering")
		assert.Contains(t, groups, "premium")
		assert.Contains(t, groups, "beta-testers")
	})
}

// --- callPortalPluginRPC handler tests ---

func setupPortalRPCTest(t *testing.T) (*API, *gorm.DB) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))

	pluginService := services.NewPluginService(db)
	pluginManager := services.NewAIStudioPluginManager(db, nil)

	api := &API{
		service: &services.Service{
			DB:                    db,
			PluginService:         pluginService,
			AIStudioPluginManager: pluginManager,
		},
	}

	return api, db
}

func makePortalRPCRouter(api *API, user *models.User) *gin.Engine {
	router := gin.New()
	group := router.Group("/common")
	if user != nil {
		group.Use(func(c *gin.Context) {
			c.Set("user", user)
			c.Next()
		})
	}
	group.POST("/plugins/:id/portal-rpc/:method", api.callPortalPluginRPC)
	return router
}

func doPortalRPC(router *gin.Engine, pluginID string, method string, payload interface{}) *httptest.ResponseRecorder {
	var body []byte
	if payload != nil {
		body, _ = json.Marshal(payload)
	}
	path := fmt.Sprintf("/common/plugins/%s/portal-rpc/%s", pluginID, method)
	req, _ := http.NewRequest("POST", path, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestCallPortalPluginRPC_Unauthorized(t *testing.T) {
	api, _ := setupPortalRPCTest(t)

	t.Run("returns 401 when no user in context", func(t *testing.T) {
		router := makePortalRPCRouter(api, nil)
		w := doPortalRPC(router, "1", "test_method", map[string]interface{}{})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestCallPortalPluginRPC_InvalidInput(t *testing.T) {
	api, _ := setupPortalRPCTest(t)
	user := &models.User{Email: "test@example.com", Name: "Test User"}
	router := makePortalRPCRouter(api, user)

	t.Run("returns 400 for non-numeric plugin ID", func(t *testing.T) {
		w := doPortalRPC(router, "abc", "test_method", map[string]interface{}{})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 404 for non-existent plugin", func(t *testing.T) {
		w := doPortalRPC(router, "99999", "test_method", map[string]interface{}{})
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestCallPortalPluginRPC_PluginState(t *testing.T) {
	api, db := setupPortalRPCTest(t)
	user := &models.User{Email: "test@example.com", Name: "Test User"}

	t.Run("returns 400 for inactive plugin", func(t *testing.T) {
		plugin := &models.Plugin{
			Name:      "Inactive Plugin",
			Command:   "file:///test/inactive",
			HookType:  "studio_ui",
			HookTypes: []string{"studio_ui", "portal_ui"},
			IsActive:  true,
		}
		require.NoError(t, db.Create(plugin).Error)
		require.NoError(t, db.Model(plugin).Update("is_active", false).Error)

		router := makePortalRPCRouter(api, user)
		w := doPortalRPC(router, fmt.Sprintf("%d", plugin.ID), "test_method", map[string]interface{}{})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 404 for active but unloaded plugin", func(t *testing.T) {
		plugin := &models.Plugin{
			Name:      "Unloaded Plugin",
			Command:   "file:///test/unloaded",
			HookType:  "studio_ui",
			HookTypes: []string{"studio_ui", "portal_ui"},
			IsActive:  true,
		}
		require.NoError(t, db.Create(plugin).Error)

		router := makePortalRPCRouter(api, user)
		w := doPortalRPC(router, fmt.Sprintf("%d", plugin.ID), "test_method", map[string]interface{}{})
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("returns 403 for loaded plugin without portal_ui hook type", func(t *testing.T) {
		plugin := &models.Plugin{
			Name:      "No Portal Plugin",
			Command:   "file:///test/no-portal",
			HookType:  "studio_ui",
			HookTypes: []string{"studio_ui"}, // No portal_ui
			IsActive:  true,
		}
		require.NoError(t, db.Create(plugin).Error)

		// Inject as loaded so we get past the IsPluginLoaded check
		api.service.AIStudioPluginManager.InjectTestPlugin(plugin.ID, plugin.Name, &mockPortalPluginClient{})
		defer api.service.AIStudioPluginManager.RemoveTestPlugin(plugin.ID)

		router := makePortalRPCRouter(api, user)
		w := doPortalRPC(router, fmt.Sprintf("%d", plugin.ID), "test_method", map[string]interface{}{})
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestCallPortalPluginRPC_HappyPath(t *testing.T) {
	api, db := setupPortalRPCTest(t)

	// Create a plugin that supports portal_ui
	plugin := &models.Plugin{
		Name:      "Portal Happy Plugin",
		Command:   "file:///test/portal-happy",
		HookType:  "studio_ui",
		HookTypes: []string{"studio_ui", "portal_ui"},
		IsActive:  true,
	}
	require.NoError(t, db.Create(plugin).Error)

	user := &models.User{
		Email:   "portal@example.com",
		Name:    "Portal User",
		IsAdmin: false,
		Groups: []models.Group{
			{Name: "engineering"},
			{Name: "premium"},
		},
	}

	t.Run("successful portal RPC call returns 200 with data", func(t *testing.T) {
		mockClient := &mockPortalPluginClient{
			portalCallFn: func(ctx context.Context, req *pb.PortalCallRequest, opts ...grpc.CallOption) (*pb.PortalCallResponse, error) {
				// Verify the request was constructed correctly
				assert.Equal(t, "get_user_data", req.Method)
				assert.NotEmpty(t, req.Payload)
				assert.NotNil(t, req.UserContext)
				assert.Equal(t, "portal@example.com", req.UserContext.Email)
				assert.Equal(t, "Portal User", req.UserContext.Name)
				assert.False(t, req.UserContext.IsAdmin)
				assert.ElementsMatch(t, []string{"engineering", "premium"}, req.UserContext.Groups)

				return &pb.PortalCallResponse{
					Success: true,
					Data:    `{"message":"hello from plugin","user_id":0}`,
				}, nil
			},
		}

		api.service.AIStudioPluginManager.InjectTestPlugin(plugin.ID, plugin.Name, mockClient)
		defer api.service.AIStudioPluginManager.RemoveTestPlugin(plugin.ID)

		router := makePortalRPCRouter(api, user)
		w := doPortalRPC(router, fmt.Sprintf("%d", plugin.ID), "get_user_data", map[string]interface{}{"key": "value"})

		assert.Equal(t, http.StatusOK, w.Code)

		var response struct {
			Data map[string]interface{} `json:"data"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "hello from plugin", response.Data["message"])
	})

	t.Run("portal RPC returns 500 when plugin returns error", func(t *testing.T) {
		mockClient := &mockPortalPluginClient{
			portalCallFn: func(ctx context.Context, req *pb.PortalCallRequest, opts ...grpc.CallOption) (*pb.PortalCallResponse, error) {
				return &pb.PortalCallResponse{
					Success:      false,
					ErrorMessage: "something went wrong in plugin",
				}, nil
			},
		}

		api.service.AIStudioPluginManager.InjectTestPlugin(plugin.ID, plugin.Name, mockClient)
		defer api.service.AIStudioPluginManager.RemoveTestPlugin(plugin.ID)

		router := makePortalRPCRouter(api, user)
		w := doPortalRPC(router, fmt.Sprintf("%d", plugin.ID), "failing_method", map[string]interface{}{})

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("portal RPC works with empty payload", func(t *testing.T) {
		mockClient := &mockPortalPluginClient{
			portalCallFn: func(ctx context.Context, req *pb.PortalCallRequest, opts ...grpc.CallOption) (*pb.PortalCallResponse, error) {
				return &pb.PortalCallResponse{
					Success: true,
					Data:    `{"items":[]}`,
				}, nil
			},
		}

		api.service.AIStudioPluginManager.InjectTestPlugin(plugin.ID, plugin.Name, mockClient)
		defer api.service.AIStudioPluginManager.RemoveTestPlugin(plugin.ID)

		router := makePortalRPCRouter(api, user)
		w := doPortalRPC(router, fmt.Sprintf("%d", plugin.ID), "list_items", nil)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("admin user can also call portal RPC", func(t *testing.T) {
		adminUser := &models.User{
			Email:   "admin@example.com",
			Name:    "Admin User",
			IsAdmin: true,
			Groups:  []models.Group{{Name: "admins"}},
		}

		mockClient := &mockPortalPluginClient{
			portalCallFn: func(ctx context.Context, req *pb.PortalCallRequest, opts ...grpc.CallOption) (*pb.PortalCallResponse, error) {
				assert.True(t, req.UserContext.IsAdmin)
				return &pb.PortalCallResponse{
					Success: true,
					Data:    `{"admin":true}`,
				}, nil
			},
		}

		api.service.AIStudioPluginManager.InjectTestPlugin(plugin.ID, plugin.Name, mockClient)
		defer api.service.AIStudioPluginManager.RemoveTestPlugin(plugin.ID)

		router := makePortalRPCRouter(api, adminUser)
		w := doPortalRPC(router, fmt.Sprintf("%d", plugin.ID), "admin_test", map[string]interface{}{})

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// --- Portal UI registry endpoint tests via full router ---

func TestPortalUIRegistryEndpoints_ViaFullRouter(t *testing.T) {
	api, _ := setupTestAPI(t)

	t.Run("portal RPC returns error for invalid plugin ID", func(t *testing.T) {
		w := performRequest(api.router, "POST", "/common/plugins/invalid/portal-rpc/test_method", map[string]interface{}{})
		assert.NotEqual(t, http.StatusOK, w.Code)
	})

	t.Run("portal RPC returns error for non-existent plugin", func(t *testing.T) {
		w := performRequest(api.router, "POST", "/common/plugins/99999/portal-rpc/test_method", map[string]interface{}{})
		assert.NotEqual(t, http.StatusOK, w.Code)
	})
}

// --- Mock plugin client for testing ---

// mockPortalPluginClient implements pb.PluginServiceClient with a configurable
// PortalCall handler for testing the happy path.
type mockPortalPluginClient struct {
	pb.UnimplementedPluginServiceServer // Embed for all unimplemented methods
	portalCallFn                        func(ctx context.Context, req *pb.PortalCallRequest, opts ...grpc.CallOption) (*pb.PortalCallResponse, error)
}

// PortalCall delegates to the test-provided function
func (m *mockPortalPluginClient) PortalCall(ctx context.Context, req *pb.PortalCallRequest, opts ...grpc.CallOption) (*pb.PortalCallResponse, error) {
	if m.portalCallFn != nil {
		return m.portalCallFn(ctx, req, opts...)
	}
	return &pb.PortalCallResponse{Success: true, Data: `{}`}, nil
}

// Stub methods required by pb.PluginServiceClient interface
func (m *mockPortalPluginClient) Initialize(ctx context.Context, in *pb.InitRequest, opts ...grpc.CallOption) (*pb.InitResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) Ping(ctx context.Context, in *pb.PingRequest, opts ...grpc.CallOption) (*pb.PingResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) Shutdown(ctx context.Context, in *pb.ShutdownRequest, opts ...grpc.CallOption) (*pb.ShutdownResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) ProcessPreAuth(ctx context.Context, in *pb.PluginRequest, opts ...grpc.CallOption) (*pb.PluginResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) Authenticate(ctx context.Context, in *pb.AuthRequest, opts ...grpc.CallOption) (*pb.AuthResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) GetAppByCredential(ctx context.Context, in *pb.GetAppRequest, opts ...grpc.CallOption) (*pb.GetAppResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) GetUserByCredential(ctx context.Context, in *pb.GetUserRequest, opts ...grpc.CallOption) (*pb.GetUserResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) ProcessPostAuth(ctx context.Context, in *pb.EnrichedRequest, opts ...grpc.CallOption) (*pb.PluginResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) OnBeforeWriteHeaders(ctx context.Context, in *pb.HeadersRequest, opts ...grpc.CallOption) (*pb.HeadersResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) OnBeforeWrite(ctx context.Context, in *pb.ResponseWriteRequest, opts ...grpc.CallOption) (*pb.ResponseWriteResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) OnStreamComplete(ctx context.Context, in *pb.StreamCompleteRequest, opts ...grpc.CallOption) (*pb.StreamCompleteResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) HandleProxyLog(ctx context.Context, in *pb.ProxyLogRequest, opts ...grpc.CallOption) (*pb.DataCollectionResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) HandleAnalytics(ctx context.Context, in *pb.AnalyticsRequest, opts ...grpc.CallOption) (*pb.DataCollectionResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) HandleBudgetUsage(ctx context.Context, in *pb.BudgetUsageRequest, opts ...grpc.CallOption) (*pb.DataCollectionResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) GetAsset(ctx context.Context, in *pb.GetAssetRequest, opts ...grpc.CallOption) (*pb.GetAssetResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) ListAssets(ctx context.Context, in *pb.ListAssetsRequest, opts ...grpc.CallOption) (*pb.ListAssetsResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) GetManifest(ctx context.Context, in *pb.GetManifestRequest, opts ...grpc.CallOption) (*pb.GetManifestResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) Call(ctx context.Context, in *pb.CallRequest, opts ...grpc.CallOption) (*pb.CallResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) GetConfigSchema(ctx context.Context, in *pb.GetConfigSchemaRequest, opts ...grpc.CallOption) (*pb.GetConfigSchemaResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) HandleAgentMessage(ctx context.Context, in *pb.AgentMessageRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pb.AgentMessageChunk], error) {
	return nil, nil
}
func (m *mockPortalPluginClient) GetObjectHookRegistrations(ctx context.Context, in *pb.GetObjectHookRegistrationsRequest, opts ...grpc.CallOption) (*pb.GetObjectHookRegistrationsResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) HandleObjectHook(ctx context.Context, in *pb.ObjectHookRequest, opts ...grpc.CallOption) (*pb.ObjectHookResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) ExecuteScheduledTask(ctx context.Context, in *pb.ExecuteScheduledTaskRequest, opts ...grpc.CallOption) (*pb.ExecuteScheduledTaskResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) AcceptEdgePayload(ctx context.Context, in *pb.EdgePayloadRequest, opts ...grpc.CallOption) (*pb.EdgePayloadResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) OpenSession(ctx context.Context, in *pb.OpenSessionRequest, opts ...grpc.CallOption) (*pb.OpenSessionResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) CloseSession(ctx context.Context, in *pb.CloseSessionRequest, opts ...grpc.CallOption) (*pb.CloseSessionResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) GetEndpointRegistrations(ctx context.Context, in *pb.GetEndpointRegistrationsRequest, opts ...grpc.CallOption) (*pb.GetEndpointRegistrationsResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) HandleEndpointRequest(ctx context.Context, in *pb.EndpointRequest, opts ...grpc.CallOption) (*pb.EndpointResponse, error) {
	return nil, nil
}
func (m *mockPortalPluginClient) HandleEndpointRequestStream(ctx context.Context, in *pb.EndpointRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pb.EndpointResponseChunk], error) {
	return nil, nil
}
func (m *mockPortalPluginClient) GetResourceTypeRegistrations(ctx context.Context, in *pb.GetResourceTypeRegistrationsRequest, opts ...grpc.CallOption) (*pb.GetResourceTypeRegistrationsResponse, error) {
	return &pb.GetResourceTypeRegistrationsResponse{}, nil
}
func (m *mockPortalPluginClient) ListResourceInstances(ctx context.Context, in *pb.ListResourceInstancesRequest, opts ...grpc.CallOption) (*pb.ListResourceInstancesResponse, error) {
	return &pb.ListResourceInstancesResponse{}, nil
}
func (m *mockPortalPluginClient) GetResourceInstance(ctx context.Context, in *pb.GetResourceInstanceRequest, opts ...grpc.CallOption) (*pb.GetResourceInstanceResponse, error) {
	return &pb.GetResourceInstanceResponse{}, nil
}
func (m *mockPortalPluginClient) ValidateResourceSelection(ctx context.Context, in *pb.ValidateResourceSelectionRequest, opts ...grpc.CallOption) (*pb.ValidateResourceSelectionResponse, error) {
	return &pb.ValidateResourceSelectionResponse{Valid: true}, nil
}
func (m *mockPortalPluginClient) CreateResourceInstance(ctx context.Context, in *pb.CreateResourceInstanceRequest, opts ...grpc.CallOption) (*pb.CreateResourceInstanceResponse, error) {
	return &pb.CreateResourceInstanceResponse{}, nil
}
