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

// TestEdgeSyncService_GovernedMetadataPersisted checks the governed_metadata
// snapshot field lands on the LLM, Tool and Datasource rows (and stays NULL
// when the control plane sends nothing).
func TestEdgeSyncService_GovernedMetadataPersisted(t *testing.T) {
	db := setupToolsSyncTestDB(t)
	now := timestamppb.New(time.Now())
	snapshot := &pb.ConfigurationSnapshot{
		Version:      "1.0.0",
		SnapshotTime: timestamppb.Now(),
		Llms: []*pb.LLMConfig{
			{Id: 1, Name: "gpt", Slug: "gpt", Vendor: "openai", Endpoint: "https://x", IsActive: true, CreatedAt: now, UpdatedAt: now,
				GovernedMetadata: `{"data_classification":"confidential"}`},
			{Id: 2, Name: "plain", Slug: "plain", Vendor: "openai", Endpoint: "https://x", IsActive: true, CreatedAt: now, UpdatedAt: now},
		},
		Tools: []*pb.ToolConfig{
			{Id: 1, Name: "weather", Slug: "weather", ToolType: "REST", IsActive: true, CreatedAt: now, UpdatedAt: now,
				GovernedMetadata: `{"regulatory_applicability":["gdpr"]}`},
		},
		Datasources: []*pb.DatasourceConfig{
			{Id: 1, Name: "docs", IsActive: true, CreatedAt: now, UpdatedAt: now, GovernedMetadata: `{"data_classification":"internal"}`},
			{Id: 2, Name: "nothing", IsActive: true, CreatedAt: now, UpdatedAt: now},
		},
	}

	require.NoError(t, NewEdgeSyncService(db, "").SyncConfiguration(snapshot))

	var llm database.LLM
	require.NoError(t, db.First(&llm, 1).Error)
	assert.JSONEq(t, `{"data_classification":"confidential"}`, string(llm.GovernedMetadata))
	var plain database.LLM
	require.NoError(t, db.First(&plain, 2).Error)
	assert.Empty(t, plain.GovernedMetadata)

	var tool database.Tool
	require.NoError(t, db.First(&tool, 1).Error)
	assert.JSONEq(t, `{"regulatory_applicability":["gdpr"]}`, string(tool.GovernedMetadata))

	var ds database.Datasource
	require.NoError(t, db.First(&ds, 1).Error)
	assert.JSONEq(t, `{"data_classification":"internal"}`, string(ds.GovernedMetadata))
	var empty database.Datasource
	require.NoError(t, db.First(&empty, 2).Error)
	assert.Empty(t, empty.GovernedMetadata)

	// The plugin-context helper reads straight off the synced row.
	meta := map[string]interface{}{}
	database.AddGovernedMetadataToContext(meta, &llm)
	assert.Equal(t, "confidential", meta["governed_metadata.data_classification"])
}
