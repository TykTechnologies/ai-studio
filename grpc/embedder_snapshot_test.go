package grpc

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snapshotDatasource(t *testing.T, server *ControlServer, name string) *pb.DatasourceConfig {
	t.Helper()
	snap, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	for _, ds := range snap.Datasources {
		if ds.Name == name {
			return ds
		}
	}
	t.Fatalf("datasource %q not in snapshot", name)
	return nil
}

// Edges get no Embedder objects: each datasource carries its embedder
// flattened into the embed_* fields the proto always had, with the key
// encrypted for transit.
func TestSnapshot_FlattensDatasourceEmbedders(t *testing.T) {
	server, db := setupTestServer(t, nil)
	key := os.Getenv("MICROGATEWAY_ENCRYPTION_KEY")

	standalone := &models.Embedder{Name: "ollama", Vendor: models.OLLAMA, Endpoint: "http://ollama:11434", APIKey: "ollama-key", ModelName: "nomic-embed-text"}
	require.NoError(t, db.Create(standalone).Error)
	llm := &models.LLM{Name: "OpenAI", Vendor: models.OPENAI, APIEndpoint: "https://api.openai.com/v1", APIKey: "sk-llm", Active: true}
	require.NoError(t, db.Create(llm).Error)
	linked := &models.Embedder{Name: "linked", LLMID: &llm.ID, ModelName: "text-embedding-3-small"}
	require.NoError(t, db.Create(linked).Error)

	require.NoError(t, db.Create(&models.Datasource{Name: "Local", Active: true, EmbedderID: &standalone.ID}).Error)
	require.NoError(t, db.Create(&models.Datasource{Name: "Cloud", Active: true, EmbedderID: &linked.ID}).Error)

	local := snapshotDatasource(t, server, "Local")
	assert.Equal(t, "ollama", local.EmbedVendor)
	assert.Equal(t, "http://ollama:11434", local.EmbedUrl)
	assert.Equal(t, "nomic-embed-text", local.EmbedModel)
	assert.Equal(t, "ollama-key", decryptLikeMicrogateway(t, key, local.EmbedApiKeyEncrypted))

	cloud := snapshotDatasource(t, server, "Cloud")
	assert.Equal(t, "openai", cloud.EmbedVendor, "a linked embedder takes the LLM's vendor")
	assert.Equal(t, "https://api.openai.com/v1", cloud.EmbedUrl)
	assert.Equal(t, "text-embedding-3-small", cloud.EmbedModel)
	assert.Equal(t, "sk-llm", decryptLikeMicrogateway(t, key, cloud.EmbedApiKeyEncrypted))
}

// Rotating an embedder's key (or its LLM's) changes what edges receive, so
// the checksum moves and edges reload.
func TestSnapshot_EmbedderEditMovesChecksum(t *testing.T) {
	server, db := setupTestServer(t, nil)
	e := &models.Embedder{Name: "e", Vendor: models.OPENAI, APIKey: "sk-1", ModelName: "m"}
	require.NoError(t, db.Create(e).Error)
	require.NoError(t, db.Create(&models.Datasource{Name: "Docs", Active: true, EmbedderID: &e.ID}).Error)

	before, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	again, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	assert.Equal(t, before.Checksum, again.Checksum)

	require.NoError(t, db.Model(e).Update("api_key", "sk-2").Error)
	after, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	assert.NotEqual(t, before.Checksum, after.Checksum)
}

// An embedder edit marks edges pending: its event is a config topic and the
// flattened snapshot has changed.
func TestConfigChange_EmbedderEditMarksEdgesPending(t *testing.T) {
	server, db := setupTestServer(t, nil)
	e := &models.Embedder{Name: "e", Vendor: models.OPENAI, APIKey: "sk-1", ModelName: "m"}
	require.NoError(t, db.Create(e).Error)
	require.NoError(t, db.Create(&models.Datasource{Name: "Docs", Active: true, EmbedderID: &e.ID}).Error)
	edge := models.EdgeInstance{EdgeID: "edge-1", Namespace: "default", Status: models.EdgeStatusRegistered, SyncStatus: models.EdgeSyncStatusInSync}
	require.NoError(t, db.Create(&edge).Error)
	syncOf := func() string {
		var got models.EdgeInstance
		require.NoError(t, db.Where("edge_id = ?", "edge-1").First(&got).Error)
		return got.SyncStatus
	}

	server.onConfigurationChanged(topicEmbedderCreated, eventbridge.Event{ID: "1"})
	require.NoError(t, db.Model(&models.EdgeInstance{}).Where("edge_id = ?", "edge-1").Update("sync_status", models.EdgeSyncStatusInSync).Error)

	require.NoError(t, db.Model(e).Update("api_key", "sk-2").Error)
	server.onConfigurationChanged(topicEmbedderUpdated, eventbridge.Event{ID: "2"})
	assert.Equal(t, models.EdgeSyncStatusPending, syncOf())
}

func snapshotRouterConfig(t *testing.T, server *ControlServer, slug string) sr.Config {
	t.Helper()
	snap, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	for _, r := range snap.SemanticRouters {
		if r.Slug == slug {
			var cfg sr.Config
			require.NoError(t, json.Unmarshal([]byte(r.ConfigJson), &cfg))
			return cfg
		}
	}
	t.Fatalf("semantic router %q not in snapshot", slug)
	return sr.Config{}
}

// Routers reach edges with their embedder flattened in: a linked embedder as
// the {llm_id, model} reference older edges understand, a standalone one
// inline with its key encrypted.
func TestSnapshot_FlattensRouterEmbedders(t *testing.T) {
	server, db := setupTestServer(t, nil)
	key := os.Getenv("MICROGATEWAY_ENCRYPTION_KEY")
	llm := &models.LLM{Name: "OpenAI", Vendor: models.OPENAI, APIKey: "sk-llm", Active: true}
	require.NoError(t, db.Create(llm).Error)

	mk := func(slug string, e *models.Embedder) {
		require.NoError(t, db.Create(e).Error)
		r := &models.SemanticRouter{Name: slug, Slug: slug, Active: true, EmbedderID: &e.ID,
			Settings: sr.Settings{DefaultRoute: "a", Embedding: &sr.ModelRef{TimeoutMs: 700}},
			Routes:   []sr.Route{{Name: "a", Utterances: []string{"hi"}, Target: sr.Target{Type: sr.TargetLLM, LLMID: llm.ID, Model: "m"}}}}
		require.NoError(t, r.Create(db))
	}
	mk("linked", &models.Embedder{Name: "linked", LLMID: &llm.ID, ModelName: "text-embedding-3-small"})
	mk("inline", &models.Embedder{Name: "inline", Vendor: models.OLLAMA, Endpoint: "http://ollama:11434", APIKey: "ollama-key", ModelName: "nomic"})

	linked := snapshotRouterConfig(t, server, "linked")
	assert.Equal(t, &sr.ModelRef{LLMID: llm.ID, Model: "text-embedding-3-small", TimeoutMs: 700}, linked.Settings.Embedding)

	inline := snapshotRouterConfig(t, server, "inline").Settings.Embedding
	require.NotNil(t, inline)
	assert.True(t, inline.Inline())
	assert.Equal(t, "ollama", inline.Vendor)
	assert.Equal(t, "http://ollama:11434", inline.Endpoint)
	assert.Equal(t, 700, inline.TimeoutMs)
	assert.Equal(t, "ollama-key", decryptLikeMicrogateway(t, key, inline.APIKeyEncrypted))
	require.NoError(t, sr.Validate(snapshotRouterConfig(t, server, "inline")))
}
