package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The hub's "Allow internal host" switches (w4e) reach the edge's database
// and the models the gateway builds from it: an LLM's, a tool's, a
// datasource's and its embedder's, and a plugin's. Off stays off.
func TestEdgeSync_AllowInternalHost(t *testing.T) {
	crypto := NewCryptoService(metadataSecretsTestKey)
	db := setupToolsSyncTestDB(t)
	repo := database.NewRepository(db)
	adapter := NewGatewayServiceAdapter(
		NewDatabaseGatewayService(db, repo),
		NewManagementService(db, repo, crypto),
		nil, crypto,
		NewFilterService(db, repo), NewPluginService(db, repo),
		nil, db,
	)
	now := timestamppb.New(time.Now())
	snapshot := &pb.ConfigurationSnapshot{
		Version: "1.0.0", SnapshotTime: now,
		Llms: []*pb.LLMConfig{
			{Id: 1, Name: "ollama", Slug: "ollama", Vendor: "ollama", Endpoint: "http://ollama:11434", IsActive: true, CreatedAt: now, UpdatedAt: now, AllowInternalHost: true},
			{Id: 2, Name: "openai", Slug: "openai", Vendor: "openai", IsActive: true, CreatedAt: now, UpdatedAt: now},
		},
		Tools: []*pb.ToolConfig{
			{Id: 1, Name: "local", Slug: "local", ToolType: "REST", IsActive: true, CreatedAt: now, UpdatedAt: now, AllowInternalHost: true},
		},
		Datasources: []*pb.DatasourceConfig{
			{Id: 1, Name: "pg", DbSourceType: "pgvector", EmbedVendor: "ollama", EmbedUrl: "http://ollama:11434", IsActive: true, CreatedAt: now, UpdatedAt: now,
				AllowInternalHost: true, EmbedAllowInternalHost: true},
			{Id: 2, Name: "pinecone", DbSourceType: "pinecone", EmbedVendor: "openai", IsActive: true, CreatedAt: now, UpdatedAt: now},
		},
		Plugins: []*pb.PluginConfig{
			{Id: 1, Name: "p", Command: "oci://registry.internal/p:1", IsActive: true, CreatedAt: now, UpdatedAt: now, AllowInternalHost: true},
		},
	}
	require.NoError(t, NewEdgeSyncService(db, "").SyncConfiguration(snapshot))

	llms, err := adapter.GetActiveLLMs()
	require.NoError(t, err)
	got := map[uint]bool{}
	for _, l := range llms {
		got[l.ID] = l.AllowInternalHost
	}
	assert.Equal(t, map[uint]bool{1: true, 2: false}, got)

	tool, err := adapter.GetToolByID(1)
	require.NoError(t, err)
	assert.True(t, tool.AllowInternalHost)

	ds, err := adapter.GetDatasourceByID(1)
	require.NoError(t, err)
	assert.True(t, ds.AllowInternalHost)
	require.NotNil(t, ds.Embedder)
	assert.True(t, ds.Embedder.AllowsInternalHost())
	off, err := adapter.GetDatasourceByID(2)
	require.NoError(t, err)
	assert.False(t, off.AllowInternalHost)
	require.NotNil(t, off.Embedder)
	assert.False(t, off.Embedder.AllowsInternalHost())

	var plugin database.Plugin
	require.NoError(t, db.First(&plugin, 1).Error)
	assert.True(t, plugin.AllowInternalHost)
}
