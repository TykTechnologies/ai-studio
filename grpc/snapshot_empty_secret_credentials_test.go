package grpc

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// A credential that references a secret stored empty reaches edges as
// secrets.RefusedCredential, never empty (0eu): an empty credential is what a
// client fills from its own environment (an AWS client's default chain, the
// edge's instance role). A field with no value at all (an Ollama LLM without
// a key) still goes out empty. One case per credential field of the snapshot.
func TestSnapshot_CredentialReferencingEmptySecretIsRefused(t *testing.T) {
	server, db := setupTestServer(t, nil)
	key := os.Getenv("MICROGATEWAY_ENCRYPTION_KEY")
	require.NoError(t, db.AutoMigrate(&secrets.Secret{}))
	secrets.SetDBRef(db)
	t.Cleanup(func() { secrets.SetDBRef(nil) })
	require.NoError(t, secrets.CreateSecret(db, &secrets.Secret{VarName: "EMPTY", Value: ""}))
	const ref = "$SECRET/EMPTY"
	raw := db.Session(&gorm.Session{SkipHooks: true})

	// LLMs: api_key, and credential metadata (Bedrock keys).
	bedrock := &models.LLM{Name: "Bedrock", Vendor: models.BEDROCK, Active: true, APIKey: ref,
		Metadata: models.JSONMap{"aws_secret_access_key": ref, "aws_region": ref}}
	require.NoError(t, raw.Create(bedrock).Error)
	ollama := &models.LLM{Name: "Ollama", Vendor: models.OLLAMA, Active: true, APIEndpoint: "http://ollama:11434"}
	require.NoError(t, raw.Create(ollama).Error)

	// Tools: auth_key.
	tool := &models.Tool{Name: "Ref tool", Slug: "ref-tool", ToolType: models.ToolTypeREST, Active: true, AuthKey: ref}
	require.NoError(t, raw.Create(tool).Error)
	openTool := &models.Tool{Name: "Open tool", Slug: "open-tool", ToolType: models.ToolTypeREST, Active: true}
	require.NoError(t, raw.Create(openTool).Error)

	// Datasources: db_conn_api_key and the embedder's key.
	refEmbedder := &models.Embedder{Name: "ref", Vendor: models.OPENAI, ModelName: "m", APIKey: ref}
	require.NoError(t, raw.Create(refEmbedder).Error)
	require.NoError(t, raw.Create(&models.Datasource{Name: "Ref DS", Active: true, DBSourceType: "pinecone",
		DBConnAPIKey: ref, EmbedderID: &refEmbedder.ID}).Error)
	openEmbedder := &models.Embedder{Name: "open", Vendor: models.OLLAMA, ModelName: "nomic", Endpoint: "http://ollama:11434"}
	require.NoError(t, raw.Create(openEmbedder).Error)
	require.NoError(t, raw.Create(&models.Datasource{Name: "Open DS", Active: true, DBSourceType: "chroma",
		EmbedderID: &openEmbedder.ID}).Error)

	// Semantic routers: a standalone embedder's api_key.
	llm := &models.LLM{Name: "Target", Vendor: models.OPENAI, APIKey: "sk-fake-target", Active: true}
	require.NoError(t, db.Create(llm).Error)
	mkRouter := func(slug string, e *models.Embedder) {
		r := &models.SemanticRouter{Name: slug, Slug: slug, Active: true, EmbedderID: &e.ID,
			Settings: sr.Settings{DefaultRoute: "a"},
			Routes:   []sr.Route{{Name: "a", Utterances: []string{"hi"}, Target: sr.Target{Type: sr.TargetLLM, LLMID: llm.ID, Model: "m"}}}}
		require.NoError(t, r.Create(db))
	}
	mkRouter("ref-router", refEmbedder)
	mkRouter("open-router", openEmbedder)

	snap, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)

	var seenLLMs int
	for _, l := range snap.Llms {
		switch l.Id {
		case uint32(bedrock.ID):
			seenLLMs++
			assert.Equal(t, secrets.RefusedCredential, decryptLikeMicrogateway(t, key, l.ApiKeyEncrypted), "llm api_key")
			assert.Equal(t, secrets.RefusedCredential, decryptLikeMicrogateway(t, key, l.MetadataSecretsEncrypted["aws_secret_access_key"]), "llm credential metadata")
			assert.Contains(t, l.Metadata, `"aws_region":""`, "a non-credential metadata value is not a credential")
		case uint32(ollama.ID):
			seenLLMs++
			assert.Empty(t, l.ApiKeyEncrypted, "no key stays no key")
		}
	}
	assert.Equal(t, 2, seenLLMs)

	var seenTools int
	for _, tc := range snap.Tools {
		switch tc.Id {
		case uint32(tool.ID):
			seenTools++
			assert.Equal(t, secrets.RefusedCredential, decryptLikeMicrogateway(t, key, tc.AuthKeyEncrypted), "tool auth_key")
		case uint32(openTool.ID):
			seenTools++
			assert.Empty(t, tc.AuthKeyEncrypted)
		}
	}
	assert.Equal(t, 2, seenTools)

	refDS := snapshotDatasource(t, server, "Ref DS")
	assert.Equal(t, secrets.RefusedCredential, decryptLikeMicrogateway(t, key, refDS.DbConnApiKeyEncrypted), "datasource db_conn_api_key")
	assert.Equal(t, secrets.RefusedCredential, decryptLikeMicrogateway(t, key, refDS.EmbedApiKeyEncrypted), "datasource embed key")
	openDS := snapshotDatasource(t, server, "Open DS")
	assert.Empty(t, openDS.DbConnApiKeyEncrypted)
	assert.Empty(t, openDS.EmbedApiKeyEncrypted)

	refRouter := snapshotRouterConfig(t, server, "ref-router").Settings.Embedding
	require.NotNil(t, refRouter)
	assert.Equal(t, secrets.RefusedCredential, decryptLikeMicrogateway(t, key, refRouter.APIKeyEncrypted), "embedder api_key")
	openRouter := snapshotRouterConfig(t, server, "open-router").Settings.Embedding
	require.NotNil(t, openRouter)
	assert.Empty(t, openRouter.APIKeyEncrypted)
}
