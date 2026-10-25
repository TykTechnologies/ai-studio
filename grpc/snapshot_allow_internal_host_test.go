package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// The "Allow internal host" switches (w4e) travel in the edge snapshot: an
// LLM's, a tool's, a plugin's, and a datasource's with its embedder's, which
// is the LLM's for a linked embedder.
func TestSnapshot_AllowInternalHost(t *testing.T) {
	server, db := setupTestServer(t, nil)

	ollama := &models.LLM{Name: "Ollama", Vendor: models.OLLAMA, Active: true, APIEndpoint: "http://ollama:11434", AllowInternalHost: true}
	require.NoError(t, db.Create(ollama).Error)
	openai := &models.LLM{Name: "OpenAI", Vendor: models.OPENAI, Active: true, APIKey: "sk-fake"}
	require.NoError(t, db.Create(openai).Error)
	tool := &models.Tool{Name: "Local", Slug: "local", ToolType: models.ToolTypeREST, Active: true, AllowInternalHost: true}
	require.NoError(t, db.Create(tool).Error)

	linked := &models.Embedder{Name: "linked", LLMID: &ollama.ID, ModelName: "nomic"}
	require.NoError(t, db.Create(linked).Error)
	standalone := &models.Embedder{Name: "standalone", Vendor: models.OLLAMA, Endpoint: "http://ollama:11434", ModelName: "nomic"}
	require.NoError(t, db.Create(standalone).Error)
	require.NoError(t, db.Create(&models.Datasource{Name: "Linked DS", Active: true, DBSourceType: "pgvector", AllowInternalHost: true, EmbedderID: &linked.ID}).Error)
	require.NoError(t, db.Create(&models.Datasource{Name: "Standalone DS", Active: true, DBSourceType: "pinecone", EmbedderID: &standalone.ID}).Error)

	snap, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)

	llms := map[uint32]bool{}
	for _, l := range snap.Llms {
		llms[l.Id] = l.AllowInternalHost
	}
	assert.True(t, llms[uint32(ollama.ID)])
	assert.False(t, llms[uint32(openai.ID)])
	for _, tc := range snap.Tools {
		if tc.Id == uint32(tool.ID) {
			assert.True(t, tc.AllowInternalHost)
		}
	}
	linkedDS := snapshotDatasource(t, server, "Linked DS")
	assert.True(t, linkedDS.AllowInternalHost)
	assert.True(t, linkedDS.EmbedAllowInternalHost, "a linked embedder takes its LLM's switch")
	standaloneDS := snapshotDatasource(t, server, "Standalone DS")
	assert.False(t, standaloneDS.AllowInternalHost)
	assert.False(t, standaloneDS.EmbedAllowInternalHost)

	require.NoError(t, db.Model(standalone).Update("allow_internal_host", true).Error)
	assert.True(t, snapshotDatasource(t, server, "Standalone DS").EmbedAllowInternalHost)
}
