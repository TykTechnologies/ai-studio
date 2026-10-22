package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The hub's agent tracing settings survive the snapshot -> SQLite ->
// runtime model round trip, so an edge captures what the hub decided. A
// later snapshot without them clears them.
func TestEdgeSyncService_AgentTracingSettings(t *testing.T) {
	db := setupToolsSyncTestDB(t)
	now := timestamppb.New(time.Now())
	snapshot := &pb.ConfigurationSnapshot{
		Version:      "1.0.0",
		SnapshotTime: timestamppb.Now(),
		Llms: []*pb.LLMConfig{
			{Id: 1, Name: "gpt", Slug: "gpt", Vendor: "openai", Endpoint: "https://x", IsActive: true,
				ApiKeyEncrypted: "k", CreatedAt: now, UpdatedAt: now,
				TracingContentModeMax: "metadata", TracingPropagationOptOut: true},
			{Id: 2, Name: "plain", Slug: "plain", Vendor: "openai", Endpoint: "https://y", IsActive: true,
				ApiKeyEncrypted: "k", CreatedAt: now, UpdatedAt: now},
		},
		Apps: []*pb.AppConfig{
			{Id: 10, Name: "traced", IsActive: true, LlmIds: []uint32{1}, CreatedAt: now, UpdatedAt: now,
				Tracing: `{"content_mode":"redacted","payload_days":3}`},
		},
		Tools: []*pb.ToolConfig{
			{Id: 20, Name: "t", Slug: "t", ToolType: "REST", IsActive: true, CreatedAt: now, UpdatedAt: now,
				TracingContentModeMax: "hashed"},
		},
		Datasources: []*pb.DatasourceConfig{
			{Id: 30, Name: "d", IsActive: true, CreatedAt: now, UpdatedAt: now, TracingContentModeMax: "off"},
		},
	}
	require.NoError(t, NewEdgeSyncService(db, "").SyncConfiguration(snapshot))

	var llm database.LLM
	require.NoError(t, db.First(&llm, 1).Error)
	assert.Equal(t, "metadata", llm.TracingContentModeMax)
	assert.True(t, llm.TracingPropagationOptOut)
	var tool database.Tool
	require.NoError(t, db.First(&tool, 20).Error)
	assert.Equal(t, "hashed", tool.TracingContentModeMax)
	var ds database.Datasource
	require.NoError(t, db.First(&ds, 30).Error)
	assert.Equal(t, "off", ds.TracingContentModeMax)
	var app database.App
	require.NoError(t, db.First(&app, 10).Error)
	assert.JSONEq(t, `{"content_mode":"redacted","payload_days":3}`, string(app.Tracing))

	repo := database.NewRepository(db)
	adapter := NewGatewayServiceAdapter(
		NewDatabaseGatewayService(db, repo),
		NewManagementService(db, repo, &noopCryptoService{}),
		nil, &noopCryptoService{},
		NewFilterService(db, repo), NewPluginService(db, repo),
		nil, db,
	)
	active, err := adapter.GetActiveLLMs()
	require.NoError(t, err)
	byName := map[string]models.LLM{}
	for _, l := range active {
		byName[l.Name] = l
	}
	assert.Equal(t, "metadata", byName["gpt"].TracingContentModeMax)
	assert.True(t, byName["gpt"].TracingPropagationOptOut)
	assert.False(t, byName["plain"].TracingPropagationOptOut, "propagation stays on by default")

	days := 3
	conv := adapter.(*GatewayServiceAdapter)
	modelApp := conv.convertDatabaseAppToModel(&app)
	assert.Equal(t, models.AppTracingSettings{ContentMode: "redacted", PayloadDays: &days}, modelApp.Tracing)
	assert.Equal(t, "hashed", conv.convertDatabaseToolToModel(&tool).TracingContentModeMax)
	assert.Equal(t, "off", conv.convertDatabaseDatasourceToModel(&ds).TracingContentModeMax)

	// A hub that later drops the overrides clears them on the edge.
	snapshot.Apps[0].Tracing = ""
	snapshot.Llms[0].TracingContentModeMax, snapshot.Llms[0].TracingPropagationOptOut = "", false
	require.NoError(t, NewEdgeSyncService(db, "").SyncConfiguration(snapshot))
	require.NoError(t, db.First(&llm, 1).Error)
	assert.Empty(t, llm.TracingContentModeMax)
	assert.False(t, llm.TracingPropagationOptOut)
	var cleared database.App
	require.NoError(t, db.First(&cleared, 10).Error)
	assert.Equal(t, models.AppTracingSettings{}, conv.convertDatabaseAppToModel(&cleared).Tracing)
}
