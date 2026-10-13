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
	for _, tc := range snap.Tools {
		if tc.Id == uint32(tool.ID) {
			assert.Equal(t, secrets.RefusedCredential, decryptLikeMicrogateway(t, key, tc.AuthKeyEncrypted))
		}
	}
}
