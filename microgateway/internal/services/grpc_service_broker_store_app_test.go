package services

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/microgateway/proto/microgateway_management"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupStoreAppTestDB(t *testing.T) (*gorm.DB, *MicrogatewayManagementServer) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = database.Migrate(db)
	require.NoError(t, err)

	repo := database.NewRepository(db)
	crypto := NewCryptoService("12345678901234567890123456789012")
	mgmtService := NewManagementService(db, repo, crypto)

	server := NewMicrogatewayManagementServer(db, nil, nil, mgmtService, crypto)

	// Create a plugin record so scope validation passes
	plugin := &database.Plugin{
		ID:            1,
		Name:          "test-plugin",
		Command:       "/bin/test-plugin",
		HookType:      "auth",
		IsActive:      true,
		ServiceScopes: datatypes.JSON(`["apps.write","apps.read","kv.readwrite"]`),
	}
	require.NoError(t, db.Create(plugin).Error)

	return db, server
}

func createTestLLM(t *testing.T, db *gorm.DB, id uint, name string) {
	t.Helper()
	llm := &database.LLM{
		Model:    gorm.Model{ID: id},
		Name:     name,
		Slug:     name,
		Vendor:   "test",
		Endpoint: "https://example.com",
		IsActive: true,
	}
	require.NoError(t, db.Create(llm).Error)
}

func createTestTool(t *testing.T, db *gorm.DB, id uint, name string) {
	t.Helper()
	tool := &database.Tool{
		Model: gorm.Model{ID: id},
		Name:  name,
	}
	require.NoError(t, db.Create(tool).Error)
}

func createTestDatasource(t *testing.T, db *gorm.DB, id uint, name string) {
	t.Helper()
	ds := &database.Datasource{
		Model: gorm.Model{ID: id},
		Name:  name,
	}
	require.NoError(t, db.Create(ds).Error)
}

func TestStoreApp_CreateNew(t *testing.T) {
	db, server := setupStoreAppTestDB(t)
	defer database.Close(db)

	// Create LLMs and tools for associations
	createTestLLM(t, db, 1, "test-llm-1")
	createTestLLM(t, db, 2, "test-llm-2")
	createTestTool(t, db, 1, "test-tool-1")
	createTestDatasource(t, db, 1, "test-ds-1")

	now := timestamppb.Now()
	req := &pb.StoreAppRequest{
		Context:       &pb.PluginContext{PluginId: 1, MethodScope: "apps.write"},
		AppId:         100,
		Name:          "Test Auto-Provisioned App",
		Description:   "Created by plugin",
		IsActive:      true,
		UserId:        1,
		Metadata:      `{"oauth2_provider_id":"auth0","auto_provisioned":true}`,
		Namespace:     "",
		MonthlyBudget: 50.0,
		LlmIds:        []uint32{1, 2},
		ToolIds:       []uint32{1},
		DatasourceIds: []uint32{1},
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	resp, err := server.StoreApp(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, resp.Success)

	// Verify app was created
	var app database.App
	require.NoError(t, db.First(&app, 100).Error)
	assert.Equal(t, "Test Auto-Provisioned App", app.Name)
	assert.Equal(t, "Created by plugin", app.Description)
	assert.True(t, app.IsActive)
	assert.Equal(t, uint(1), app.UserID)
	assert.Equal(t, 50.0, app.MonthlyBudget)

	// Verify metadata
	assert.Contains(t, string(app.Metadata), "oauth2_provider_id")
	assert.Contains(t, string(app.Metadata), "auto_provisioned")

	// Verify LLM associations
	var llmAssocs []database.AppLLM
	require.NoError(t, db.Where("app_id = ?", 100).Find(&llmAssocs).Error)
	assert.Len(t, llmAssocs, 2)

	// Verify Tool associations
	var toolAssocs []database.AppTool
	require.NoError(t, db.Where("app_id = ?", 100).Find(&toolAssocs).Error)
	assert.Len(t, toolAssocs, 1)

	// Verify Datasource associations
	var dsAssocs []database.AppDatasource
	require.NoError(t, db.Where("app_id = ?", 100).Find(&dsAssocs).Error)
	assert.Len(t, dsAssocs, 1)
}

func TestStoreApp_UpdateExisting(t *testing.T) {
	db, server := setupStoreAppTestDB(t)
	defer database.Close(db)

	createTestLLM(t, db, 1, "llm-1")
	createTestLLM(t, db, 2, "llm-2")
	createTestLLM(t, db, 3, "llm-3")

	ctx := context.Background()
	pluginCtx := &pb.PluginContext{PluginId: 1, MethodScope: "apps.write"}

	// Create initial app with LLM 1 and 2
	_, err := server.StoreApp(ctx, &pb.StoreAppRequest{
		Context:  pluginCtx,
		AppId:    200,
		Name:     "Original Name",
		IsActive: true,
		UserId:   1,
		LlmIds:   []uint32{1, 2},
	})
	require.NoError(t, err)

	// Update: change name, replace LLMs with 2 and 3
	resp, err := server.StoreApp(ctx, &pb.StoreAppRequest{
		Context:  pluginCtx,
		AppId:    200,
		Name:     "Updated Name",
		IsActive: true,
		UserId:   1,
		Metadata: `{"updated":true}`,
		LlmIds:   []uint32{2, 3},
	})
	require.NoError(t, err)
	assert.True(t, resp.Success)

	// Verify app was updated
	var app database.App
	require.NoError(t, db.First(&app, 200).Error)
	assert.Equal(t, "Updated Name", app.Name)
	assert.Contains(t, string(app.Metadata), "updated")

	// Verify LLM associations were replaced (old cleared, new created)
	var llmAssocs []database.AppLLM
	require.NoError(t, db.Where("app_id = ?", 200).Find(&llmAssocs).Error)
	assert.Len(t, llmAssocs, 2)

	llmIDs := make(map[uint]bool)
	for _, a := range llmAssocs {
		llmIDs[a.LLMID] = true
	}
	assert.False(t, llmIDs[1], "LLM 1 should have been removed")
	assert.True(t, llmIDs[2], "LLM 2 should be present")
	assert.True(t, llmIDs[3], "LLM 3 should be present")
}

func TestStoreApp_EmptyAssociations(t *testing.T) {
	db, server := setupStoreAppTestDB(t)
	defer database.Close(db)

	// Create app with no associations
	resp, err := server.StoreApp(context.Background(), &pb.StoreAppRequest{
		Context:  &pb.PluginContext{PluginId: 1, MethodScope: "apps.write"},
		AppId:    300,
		Name:     "No Associations",
		IsActive: true,
		UserId:   1,
	})
	require.NoError(t, err)
	assert.True(t, resp.Success)

	var app database.App
	require.NoError(t, db.First(&app, 300).Error)
	assert.Equal(t, "No Associations", app.Name)

	// Verify no associations
	var count int64
	db.Model(&database.AppLLM{}).Where("app_id = ?", 300).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestStoreApp_MissingAppID(t *testing.T) {
	_, server := setupStoreAppTestDB(t)

	_, err := server.StoreApp(context.Background(), &pb.StoreAppRequest{
		Context: &pb.PluginContext{PluginId: 1, MethodScope: "apps.write"},
		AppId:   0, // Missing
		Name:    "Bad Request",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "app_id is required")
}

func TestStoreApp_BudgetStartDate(t *testing.T) {
	db, server := setupStoreAppTestDB(t)
	defer database.Close(db)

	resp, err := server.StoreApp(context.Background(), &pb.StoreAppRequest{
		Context:         &pb.PluginContext{PluginId: 1, MethodScope: "apps.write"},
		AppId:           400,
		Name:            "Budget App",
		IsActive:        true,
		UserId:          1,
		MonthlyBudget:   100.0,
		BudgetStartDate: time.Now().Format(time.RFC3339),
	})
	require.NoError(t, err)
	assert.True(t, resp.Success)

	var app database.App
	require.NoError(t, db.First(&app, 400).Error)
	assert.Equal(t, 100.0, app.MonthlyBudget)
	assert.NotNil(t, app.BudgetStartDate)
}

func TestStoreApp_MetadataJSON(t *testing.T) {
	db, server := setupStoreAppTestDB(t)
	defer database.Close(db)

	metadata := `{"oauth2_provider_id":"auth0","oauth2_tenant_id":"https://example.auth0.com/","oauth2_client_id":"abc123","auto_provisioned":true}`

	resp, err := server.StoreApp(context.Background(), &pb.StoreAppRequest{
		Context:  &pb.PluginContext{PluginId: 1, MethodScope: "apps.write"},
		AppId:    500,
		Name:     "Metadata App",
		IsActive: true,
		UserId:   1,
		Metadata: metadata,
	})
	require.NoError(t, err)
	assert.True(t, resp.Success)

	var app database.App
	require.NoError(t, db.First(&app, 500).Error)
	assert.JSONEq(t, metadata, string(app.Metadata))
}

func TestStoreApp_ClearsOldAssociationsOnUpdate(t *testing.T) {
	db, server := setupStoreAppTestDB(t)
	defer database.Close(db)

	createTestLLM(t, db, 1, "llm-a")
	createTestTool(t, db, 1, "tool-a")
	createTestDatasource(t, db, 1, "ds-a")

	ctx := context.Background()
	pluginCtx := &pb.PluginContext{PluginId: 1, MethodScope: "apps.write"}

	// Create with associations
	_, err := server.StoreApp(ctx, &pb.StoreAppRequest{
		Context:       pluginCtx,
		AppId:         600,
		Name:          "Full App",
		IsActive:      true,
		UserId:        1,
		LlmIds:        []uint32{1},
		ToolIds:       []uint32{1},
		DatasourceIds: []uint32{1},
	})
	require.NoError(t, err)

	// Update with empty associations - should clear all
	_, err = server.StoreApp(ctx, &pb.StoreAppRequest{
		Context:       pluginCtx,
		AppId:         600,
		Name:          "Full App (cleared)",
		IsActive:      true,
		UserId:        1,
		LlmIds:        []uint32{},
		ToolIds:       []uint32{},
		DatasourceIds: []uint32{},
	})
	require.NoError(t, err)

	// All associations should be cleared
	var llmCount, toolCount, dsCount int64
	db.Model(&database.AppLLM{}).Where("app_id = ?", 600).Count(&llmCount)
	db.Model(&database.AppTool{}).Where("app_id = ?", 600).Count(&toolCount)
	db.Model(&database.AppDatasource{}).Where("app_id = ?", 600).Count(&dsCount)

	assert.Equal(t, int64(0), llmCount, "LLM associations should be cleared")
	assert.Equal(t, int64(0), toolCount, "Tool associations should be cleared")
	assert.Equal(t, int64(0), dsCount, "Datasource associations should be cleared")
}
