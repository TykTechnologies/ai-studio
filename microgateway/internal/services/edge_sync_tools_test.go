// internal/services/edge_sync_tools_test.go
package services

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupToolsSyncTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = database.Migrate(db)
	require.NoError(t, err)

	return db
}

// createFullSnapshot creates a ConfigurationSnapshot with tools, datasources, OAuth entities, and app associations
func createFullSnapshot(namespace string) *pb.ConfigurationSnapshot {
	now := time.Now()
	return &pb.ConfigurationSnapshot{
		Version:       "1.0.0",
		EdgeNamespace: namespace,
		SnapshotTime:  timestamppb.Now(),
		Llms: []*pb.LLMConfig{
			{
				Id:       1,
				Name:     "Test LLM",
				Slug:     "test-llm",
				Vendor:   "openai",
				Endpoint: "https://api.openai.com/v1",
				IsActive: true,
				CreatedAt: timestamppb.New(now),
				UpdatedAt: timestamppb.New(now),
			},
		},
		Apps: []*pb.AppConfig{
			{
				Id:            1,
				Name:          "Test App",
				IsActive:      true,
				Namespace:     namespace,
				LlmIds:        []uint32{1},
				ToolIds:       []uint32{1, 2},
				DatasourceIds: []uint32{1},
				CreatedAt:     timestamppb.New(now),
				UpdatedAt:     timestamppb.New(now),
			},
		},
		Filters: []*pb.FilterConfig{
			{
				Id:       1,
				Name:     "Test Filter",
				Script:   "return {block: false}",
				IsActive: true,
				CreatedAt: timestamppb.New(now),
				UpdatedAt: timestamppb.New(now),
			},
		},
		Tools: []*pb.ToolConfig{
			{
				Id:                  1,
				Name:                "Weather API",
				Slug:                "weather-api",
				Description:         "Get weather data",
				ToolType:            "REST",
				OasSpec:             base64.StdEncoding.EncodeToString([]byte(`{"openapi":"3.0.0","info":{"title":"Weather","version":"1.0"},"paths":{"/weather":{"get":{"operationId":"getWeather","parameters":[{"name":"city","in":"query","schema":{"type":"string"}}],"responses":{"200":{"description":"OK"}}}}}}`)),
				AvailableOperations: "getWeather",
				PrivacyScore:        5,
				AuthKeyEncrypted:    "encrypted-key-1",
				AuthSchemaName:      "apiKey",
				IsActive:            true,
				Namespace:           namespace,
				FilterIds:           []uint32{1},
				AppIds:              []uint32{1},
				CreatedAt:           timestamppb.New(now),
				UpdatedAt:           timestamppb.New(now),
			},
			{
				Id:                  2,
				Name:                "Search API",
				Slug:                "search-api",
				Description:         "Search the web",
				ToolType:            "REST",
				OasSpec:             base64.StdEncoding.EncodeToString([]byte(`{"openapi":"3.0.0","info":{"title":"Search","version":"1.0"},"paths":{"/search":{"get":{"operationId":"search","parameters":[{"name":"q","in":"query","schema":{"type":"string"}}],"responses":{"200":{"description":"OK"}}}}}}`)),
				AvailableOperations: "search",
				IsActive:            true,
				Namespace:           namespace,
				AppIds:              []uint32{1},
				CreatedAt:           timestamppb.New(now),
				UpdatedAt:           timestamppb.New(now),
			},
		},
		Datasources: []*pb.DatasourceConfig{
			{
				Id:                    1,
				Name:                  "Knowledge Base",
				ShortDescription:      "Company knowledge",
				DbSourceType:          "pgvector",
				DbConnStringEncrypted: "encrypted-conn-string",
				DbConnApiKeyEncrypted: "encrypted-conn-key",
				DbName:                "knowledge",
				EmbedVendor:           "openai",
				EmbedModel:            "text-embedding-3-small",
				EmbedApiKeyEncrypted:  "encrypted-embed-key",
				IsActive:              true,
				Namespace:             namespace,
				AppIds:                []uint32{1},
				CreatedAt:             timestamppb.New(now),
				UpdatedAt:             timestamppb.New(now),
			},
		},
		OauthClients: []*pb.OAuthClientConfig{
			{
				Id:               1,
				ClientId:         "mcp-client-abc",
				ClientSecretHash: "$2a$10$hashvalue",
				ClientName:       "MCP Client",
				RedirectUris:     "http://localhost:3000/callback",
				UserId:           1,
				Scope:            "mcp",
				CreatedAt:        timestamppb.New(now),
				UpdatedAt:        timestamppb.New(now),
			},
		},
		AccessTokens: []*pb.AccessTokenConfig{
			{
				Id:             1,
				TokenHash:      "6f56259054ada428a17e58137714a263b55ca44ed3e7933ef78986a5a3581c2b", // SHA-256 of "encrypted-token-xyz"
				TokenEncrypted: "encrypted-token-xyz",
				ClientId:       "mcp-client-abc",
				UserId:         1,
				Scope:          "mcp",
				ExpiresAt:      timestamppb.New(now.Add(24 * time.Hour)),
				CreatedAt:      timestamppb.New(now),
				UpdatedAt:      timestamppb.New(now),
			},
		},
	}
}

func TestEdgeSyncService_SyncTools(t *testing.T) {
	db := setupToolsSyncTestDB(t)
	namespace := "test-ns"
	syncService := NewEdgeSyncService(db, namespace)

	config := createFullSnapshot(namespace)
	err := syncService.SyncConfiguration(config)
	require.NoError(t, err)

	t.Run("ToolsCreated", func(t *testing.T) {
		var tools []database.Tool
		err := db.Find(&tools).Error
		require.NoError(t, err)
		assert.Len(t, tools, 2, "Expected 2 tools to be synced")

		// Verify first tool
		var tool database.Tool
		err = db.First(&tool, 1).Error
		require.NoError(t, err)
		assert.Equal(t, "Weather API", tool.Name)
		assert.Equal(t, "weather-api", tool.Slug)
		assert.Equal(t, "REST", tool.ToolType)
		assert.Equal(t, "getWeather", tool.AvailableOperations)
		assert.Equal(t, "encrypted-key-1", tool.AuthKeyEncrypted)
		assert.Equal(t, "apiKey", tool.AuthSchemaName)
		assert.True(t, tool.Active)
		assert.Equal(t, namespace, tool.Namespace)
		assert.NotEmpty(t, tool.OASSpec)
	})

	t.Run("ToolFilterAssociations", func(t *testing.T) {
		var toolFilters []database.ToolFilter
		err := db.Where("tool_id = ?", 1).Find(&toolFilters).Error
		require.NoError(t, err)
		assert.Len(t, toolFilters, 1, "Tool 1 should have 1 filter")
		assert.Equal(t, uint(1), toolFilters[0].FilterID)
	})

	t.Run("AppToolAssociations", func(t *testing.T) {
		var appTools []database.AppTool
		err := db.Where("app_id = ?", 1).Find(&appTools).Error
		require.NoError(t, err)
		assert.Len(t, appTools, 2, "App 1 should have 2 tool associations")
	})

	t.Run("ToolPreloadedViaApp", func(t *testing.T) {
		var app database.App
		err := db.Preload("Tools").First(&app, 1).Error
		require.NoError(t, err)
		assert.Len(t, app.Tools, 2, "App should have 2 tools preloaded")
	})
}

func TestEdgeSyncService_SyncDatasources(t *testing.T) {
	db := setupToolsSyncTestDB(t)
	namespace := "test-ns"
	syncService := NewEdgeSyncService(db, namespace)

	config := createFullSnapshot(namespace)
	err := syncService.SyncConfiguration(config)
	require.NoError(t, err)

	t.Run("DatasourcesCreated", func(t *testing.T) {
		var datasources []database.Datasource
		err := db.Find(&datasources).Error
		require.NoError(t, err)
		assert.Len(t, datasources, 1, "Expected 1 datasource to be synced")

		ds := datasources[0]
		assert.Equal(t, "Knowledge Base", ds.Name)
		assert.Equal(t, "pgvector", ds.DBSourceType)
		assert.Equal(t, "encrypted-conn-string", ds.DBConnStringEncrypted)
		assert.Equal(t, "encrypted-conn-key", ds.DBConnAPIKeyEncrypted)
		assert.Equal(t, "knowledge", ds.DBName)
		assert.Equal(t, "openai", ds.EmbedVendor)
		assert.Equal(t, "text-embedding-3-small", ds.EmbedModel)
		assert.Equal(t, "encrypted-embed-key", ds.EmbedAPIKeyEncrypted)
		assert.True(t, ds.Active)
	})

	t.Run("AppDatasourceAssociations", func(t *testing.T) {
		var appDS []database.AppDatasource
		err := db.Where("app_id = ?", 1).Find(&appDS).Error
		require.NoError(t, err)
		assert.Len(t, appDS, 1, "App 1 should have 1 datasource association")
		assert.Equal(t, uint(1), appDS[0].DatasourceID)
	})

	t.Run("DatasourcePreloadedViaApp", func(t *testing.T) {
		var app database.App
		err := db.Preload("Datasources").First(&app, 1).Error
		require.NoError(t, err)
		assert.Len(t, app.Datasources, 1, "App should have 1 datasource preloaded")
	})
}

func TestEdgeSyncService_SyncOAuthClients(t *testing.T) {
	db := setupToolsSyncTestDB(t)
	namespace := "test-ns"
	syncService := NewEdgeSyncService(db, namespace)

	config := createFullSnapshot(namespace)
	err := syncService.SyncConfiguration(config)
	require.NoError(t, err)

	t.Run("OAuthClientCreated", func(t *testing.T) {
		var clients []database.OAuthClientEdge
		err := db.Find(&clients).Error
		require.NoError(t, err)
		assert.Len(t, clients, 1, "Expected 1 OAuth client to be synced")

		client := clients[0]
		assert.Equal(t, "mcp-client-abc", client.ClientID)
		assert.Equal(t, "$2a$10$hashvalue", client.ClientSecret)
		assert.Equal(t, "MCP Client", client.ClientName)
		assert.Equal(t, "http://localhost:3000/callback", client.RedirectURIs)
		assert.Equal(t, uint(1), client.UserID)
		assert.Equal(t, "mcp", client.Scope)
	})
}

func TestEdgeSyncService_SyncAccessTokens(t *testing.T) {
	db := setupToolsSyncTestDB(t)
	namespace := "test-ns"
	syncService := NewEdgeSyncService(db, namespace)

	config := createFullSnapshot(namespace)
	err := syncService.SyncConfiguration(config)
	require.NoError(t, err)

	t.Run("AccessTokenCreated", func(t *testing.T) {
		var tokens []database.AccessTokenEdge
		err := db.Find(&tokens).Error
		require.NoError(t, err)
		assert.Len(t, tokens, 1, "Expected 1 access token to be synced")

		token := tokens[0]
		assert.Equal(t, "encrypted-token-xyz", token.TokenEncrypted)
		assert.Equal(t, "mcp-client-abc", token.ClientID)
		assert.Equal(t, uint(1), token.UserID)
		assert.Equal(t, "mcp", token.Scope)
		assert.True(t, token.ExpiresAt.After(time.Now()), "Token should not be expired")
	})
}

func TestEdgeSyncService_ResyncClearsOldData(t *testing.T) {
	db := setupToolsSyncTestDB(t)
	namespace := "test-ns"
	syncService := NewEdgeSyncService(db, namespace)

	// First sync
	config := createFullSnapshot(namespace)
	err := syncService.SyncConfiguration(config)
	require.NoError(t, err)

	// Verify initial state
	var toolCount int64
	db.Model(&database.Tool{}).Count(&toolCount)
	assert.Equal(t, int64(2), toolCount)

	// Second sync with different tools (simulating config change)
	now := time.Now()
	config2 := &pb.ConfigurationSnapshot{
		Version:       "2.0.0",
		EdgeNamespace: namespace,
		SnapshotTime:  timestamppb.Now(),
		Tools: []*pb.ToolConfig{
			{
				Id:       3,
				Name:     "New Tool",
				Slug:     "new-tool",
				ToolType: "REST",
				IsActive: true,
				CreatedAt: timestamppb.New(now),
				UpdatedAt: timestamppb.New(now),
			},
		},
	}

	err = syncService.SyncConfiguration(config2)
	require.NoError(t, err)

	// Verify old tools were cleared
	db.Model(&database.Tool{}).Count(&toolCount)
	assert.Equal(t, int64(1), toolCount, "Old tools should be cleared, only new tool remains")

	var tool database.Tool
	err = db.First(&tool, 3).Error
	require.NoError(t, err)
	assert.Equal(t, "New Tool", tool.Name)

	// Old tools should be gone
	err = db.First(&database.Tool{}, 1).Error
	assert.Error(t, err, "Old tool 1 should be deleted")
}

func TestEdgeSyncService_EmptyToolsAndDatasources(t *testing.T) {
	db := setupToolsSyncTestDB(t)
	namespace := "test-ns"
	syncService := NewEdgeSyncService(db, namespace)

	// Sync with no tools or datasources
	config := &pb.ConfigurationSnapshot{
		Version:       "1.0.0",
		EdgeNamespace: namespace,
		SnapshotTime:  timestamppb.Now(),
	}

	err := syncService.SyncConfiguration(config)
	require.NoError(t, err)

	var toolCount int64
	db.Model(&database.Tool{}).Count(&toolCount)
	assert.Equal(t, int64(0), toolCount)

	var dsCount int64
	db.Model(&database.Datasource{}).Count(&dsCount)
	assert.Equal(t, int64(0), dsCount)
}

func TestGatewayAdapterToolMethods(t *testing.T) {
	db := setupToolsSyncTestDB(t)
	namespace := "test-ns"

	// Sync test data
	syncService := NewEdgeSyncService(db, namespace)
	config := createFullSnapshot(namespace)
	err := syncService.SyncConfiguration(config)
	require.NoError(t, err)

	// Create a mock crypto service that returns the input (no-op encryption)
	crypto := &noopCryptoService{}

	// Create the adapter with DB access
	repo := database.NewRepository(db)
	gatewayService := NewDatabaseGatewayService(db, repo)
	mgmt := NewManagementService(db, repo, crypto)

	adapter := NewGatewayServiceAdapter(
		gatewayService,
		mgmt,
		nil,    // analytics
		crypto,
		NewFilterService(db, repo),
		NewPluginService(db, repo),
		nil, // plugin manager
		db,
	)

	t.Run("GetToolBySlug", func(t *testing.T) {
		tool, err := adapter.GetToolBySlug(context.Background(),"weather-api")
		require.NoError(t, err)
		assert.Equal(t, "Weather API", tool.Name)
		assert.Equal(t, "weather-api", tool.Slug)
		assert.Equal(t, "REST", tool.ToolType)
		assert.Equal(t, "getWeather", tool.AvailableOperations)
		// Auth key should be "decrypted" (noop crypto returns input)
		assert.Equal(t, "encrypted-key-1", tool.AuthKey)
		assert.Equal(t, "apiKey", tool.AuthSchemaName)
	})

	t.Run("GetToolBySlug_NotFound", func(t *testing.T) {
		_, err := adapter.GetToolBySlug(context.Background(),"nonexistent-tool")
		assert.Error(t, err)
	})

	t.Run("GetToolByID", func(t *testing.T) {
		tool, err := adapter.GetToolByID(context.Background(),1)
		require.NoError(t, err)
		assert.Equal(t, "Weather API", tool.Name)
		assert.Equal(t, uint(1), tool.ID)
	})

	t.Run("GetToolByID_NotFound", func(t *testing.T) {
		_, err := adapter.GetToolByID(context.Background(),999)
		assert.Error(t, err)
	})

	t.Run("GetActiveDatasources", func(t *testing.T) {
		datasources, err := adapter.GetActiveDatasources()
		require.NoError(t, err)
		assert.Len(t, datasources, 1)
		assert.Equal(t, "Knowledge Base", datasources[0].Name)
		assert.Equal(t, "pgvector", datasources[0].DBSourceType)
		// Secrets should be "decrypted" (noop crypto returns input)
		assert.Equal(t, "encrypted-conn-string", datasources[0].DBConnString)
		assert.Equal(t, "encrypted-conn-key", datasources[0].DBConnAPIKey)
		assert.Equal(t, "encrypted-embed-key", datasources[0].EmbedAPIKey)
	})

	t.Run("GetDatasourceByID", func(t *testing.T) {
		ds, err := adapter.GetDatasourceByID(context.Background(),1)
		require.NoError(t, err)
		assert.Equal(t, "Knowledge Base", ds.Name)
	})

	t.Run("GetDatasourceByID_NotFound", func(t *testing.T) {
		_, err := adapter.GetDatasourceByID(context.Background(),999)
		assert.Error(t, err)
	})

	t.Run("GetOAuthClient", func(t *testing.T) {
		client, err := adapter.GetOAuthClient("mcp-client-abc")
		require.NoError(t, err)
		assert.Equal(t, "mcp-client-abc", client.ClientID)
		assert.Equal(t, "$2a$10$hashvalue", client.ClientSecret)
		assert.Equal(t, "MCP Client", client.ClientName)
		assert.Equal(t, "mcp", client.Scope)
	})

	t.Run("GetOAuthClient_NotFound", func(t *testing.T) {
		_, err := adapter.GetOAuthClient("nonexistent-client")
		assert.Error(t, err)
	})

	t.Run("GetValidAccessTokenByToken", func(t *testing.T) {
		// The noop crypto "decrypts" by returning the encrypted value
		// So we search for the encrypted value since that's what decrypt returns
		token, err := adapter.GetValidAccessTokenByToken("encrypted-token-xyz")
		require.NoError(t, err)
		assert.Equal(t, "mcp-client-abc", token.ClientID)
		assert.Equal(t, uint(1), token.UserID)
		assert.Equal(t, "mcp", token.Scope)
		assert.True(t, token.ExpiresAt.After(time.Now()))
	})

	t.Run("GetValidAccessTokenByToken_Invalid", func(t *testing.T) {
		_, err := adapter.GetValidAccessTokenByToken("invalid-token")
		assert.Error(t, err)
	})

	t.Run("AppWithToolAndDatasourcePreloads", func(t *testing.T) {
		// Create an API token for the app
		apiToken := &database.APIToken{
			ID:       1,
			Token:    "test-token-123",
			Name:     "Test Token",
			AppID:    1,
			IsActive: true,
		}
		err := db.Create(apiToken).Error
		require.NoError(t, err)

		// Use the concrete gateway service to get app by token (GetAppByTokenID is not on the interface)
		dbGatewayService := gatewayService.(*DatabaseGatewayService)
		result, err := dbGatewayService.GetAppByTokenID(1)
		require.NoError(t, err)
		assert.Equal(t, "Test App", result.Name)
		assert.Len(t, result.LLMs, 1, "App should have 1 LLM")
		assert.Len(t, result.Tools, 2, "App should have 2 tools preloaded")
		assert.Len(t, result.Datasources, 1, "App should have 1 datasource preloaded")
	})
}

// noopCryptoService returns input as-is (for testing without real encryption)
type noopCryptoService struct{}

func (n *noopCryptoService) Encrypt(plaintext string) (string, error)          { return plaintext, nil }
func (n *noopCryptoService) Decrypt(ciphertext string) (string, error)         { return ciphertext, nil }
func (n *noopCryptoService) HashSecret(secret string) string                   { return "hash:" + secret }
func (n *noopCryptoService) VerifySecret(secret, hash string) bool             { return hash == "hash:"+secret }
func (n *noopCryptoService) GenerateSecureToken(length int) (string, error)    { return "secure-token", nil }
func (n *noopCryptoService) GenerateKeyPair() (string, string, error)          { return "key-id", "secret", nil }
func (n *noopCryptoService) EncryptAPIKey(key string) (string, error)          { return key, nil }
func (n *noopCryptoService) DecryptAPIKey(encrypted string) (string, error)    { return encrypted, nil }
