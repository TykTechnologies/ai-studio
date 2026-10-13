package grpc

import (
	"os"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// References in snapshot fields are resolved on the hub under the policy:
// an $ENV/ name the operator has not listed never reaches an edge, as its
// value or as the name (released edges would resolve the name against
// their own environment). A credential goes out as a dud key, never empty
// (a vendor client would replace an empty key with its own environment's);
// an endpoint goes out as a bare "$ENV/". A listed name goes out resolved.
func TestSnapshotNeverCarriesUnlistedEnvValues(t *testing.T) {
	server, db := setupTestServer(t, nil)
	key := os.Getenv("MICROGATEWAY_ENCRYPTION_KEY")
	const canary = "canary-master-key-value"
	t.Setenv("TYK_AI_SECRET_KEY", canary)
	t.Setenv("VAULT_ADDR", "https://vault.example")
	t.Setenv(secrets.EnvRefAllowlistVar, "VAULT_ADDR")
	raw := db.Session(&gorm.Session{SkipHooks: true})

	llm := &models.LLM{Name: "Probe LLM", Vendor: models.OPENAI, Active: true,
		APIKey: "$ENV/TYK_AI_SECRET_KEY", APIEndpoint: "$ENV/TYK_AI_SECRET_KEY",
		Metadata: models.JSONMap{"region": "$ENV/TYK_AI_SECRET_KEY", "vault": "$ENV/VAULT_ADDR"}}
	require.NoError(t, raw.Create(llm).Error)
	tool := &models.Tool{Name: "Probe tool", Slug: "probe-tool", ToolType: models.ToolTypeREST, Active: true, AuthKey: "$ENV/TYK_AI_SECRET_KEY"}
	require.NoError(t, raw.Create(tool).Error)
	embedder := &models.Embedder{Name: "Probe embedder", Vendor: models.OPENAI, ModelName: "m",
		Endpoint: "$ENV/TYK_AI_SECRET_KEY", APIKey: "$ENV/TYK_AI_SECRET_KEY"}
	require.NoError(t, raw.Create(embedder).Error)
	ds := &models.Datasource{Name: "Probe datasource", Active: true, DBSourceType: "pinecone",
		DBConnString: "$ENV/TYK_AI_SECRET_KEY", DBConnAPIKey: "$ENV/TYK_AI_SECRET_KEY", EmbedderID: &embedder.ID}
	require.NoError(t, raw.Create(ds).Error)
	guardrail := &models.Filter{Name: "Probe guardrail", Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider":   "azure_content_safety",
		"connection": map[string]any{"endpoint": "$ENV/TYK_AI_SECRET_KEY", "api_key": "$ENV/TYK_AI_SECRET_KEY"},
		"detectors":  []any{map[string]any{"name": "hate"}},
		"on_detect":  "block",
	}}
	require.NoError(t, raw.Create(guardrail).Error)

	snap, err := server.getConfigurationSnapshot("")
	require.NoError(t, err)
	b, err := proto.Marshal(snap)
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(b), canary), "the snapshot carries the unlisted variable's value")
	assert.False(t, strings.Contains(string(b), "TYK_AI_SECRET_KEY"), "the snapshot carries the unlisted variable's name")

	var gotLLM bool
	for _, l := range snap.Llms {
		if l.Id != uint32(llm.ID) {
			continue
		}
		gotLLM = true
		assert.Equal(t, "$ENV/", l.Endpoint)
		assert.Equal(t, secrets.RefusedCredential, decryptLikeMicrogateway(t, key, l.ApiKeyEncrypted))
		assert.Contains(t, l.Metadata, `"region":"`+secrets.RefusedCredential+`"`)
		assert.Contains(t, l.Metadata, `"vault":"https://vault.example"`, "an allowlisted name is resolved")
	}
	require.True(t, gotLLM)
	var gotTool, gotDS, gotGuardrail bool
	for _, tc := range snap.Tools {
		if tc.Id == uint32(tool.ID) {
			gotTool = true
			assert.Equal(t, secrets.RefusedCredential, decryptLikeMicrogateway(t, key, tc.AuthKeyEncrypted))
		}
	}
	for _, d := range snap.Datasources {
		if d.Id != uint32(ds.ID) {
			continue
		}
		gotDS = true
		assert.Equal(t, "$ENV/", decryptLikeMicrogateway(t, key, d.DbConnStringEncrypted))
		assert.Equal(t, secrets.RefusedCredential, decryptLikeMicrogateway(t, key, d.DbConnApiKeyEncrypted))
		assert.Equal(t, "$ENV/", d.EmbedUrl)
		assert.Equal(t, secrets.RefusedCredential, decryptLikeMicrogateway(t, key, d.EmbedApiKeyEncrypted))
	}
	for _, f := range snap.Filters {
		if f.Id != uint32(guardrail.ID) {
			continue
		}
		gotGuardrail = true
		// The rendering per field is the guardrail package's (a dud key for
		// credentials, "$ENV/" or the dud key for endpoints); no form may
		// carry the name or the value.
		assert.Contains(t, f.Config, `"api_key":"`+secrets.RefusedCredential+`"`)
		assert.NotContains(t, f.Config, "TYK_AI_SECRET_KEY")
		assert.NotContains(t, f.Config, canary)
	}
	assert.True(t, gotTool && gotDS && gotGuardrail, "tool %v, datasource %v, guardrail %v", gotTool, gotDS, gotGuardrail)
}
