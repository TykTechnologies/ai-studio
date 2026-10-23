package grpc

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// The hub resolves guardrail connection references into the edge snapshot.
// $ENV/ only resolves names on AI_STUDIO_ENV_REF_ALLOWLIST: anything else
// ships as a bare "$ENV/", so neither the hub's value nor the name (which an
// older edge would resolve against its own environment) leaves.
func TestControlServer_SnapshotGuardrailEnvReferences(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "hub-encryption-key-value")
	t.Setenv("GUARDRAIL_SNAPSHOT_KEY", "unlisted-prefixed-value")
	t.Setenv("CLASSIFIER_SNAPSHOT_KEY", "listed-key-value")
	t.Setenv(secrets.EnvRefAllowlistVar, "CLASSIFIER_SNAPSHOT_KEY")
	server, db := setupTestServer(t, nil)

	filter := &models.Filter{
		Name: "classifier",
		Kind: models.FilterKindGuardrail,
		Config: models.JSONMap{
			"provider":  "http",
			"detectors": []any{map[string]any{"name": "prompt_attack"}},
			"on_detect": "block",
			"connection": map[string]any{
				"endpoint": "$ENV/CLASSIFIER_ENDPOINT_UNLISTED",
				"api_key":  "$ENV/TYK_AI_SECRET_KEY",
				"token":    "$ENV/GUARDRAIL_SNAPSHOT_KEY",
				"key2":     "$ENV/CLASSIFIER_SNAPSHOT_KEY",
			},
		},
	}
	require.NoError(t, db.Create(filter).Error)

	snapshot, err := server.getConfigurationSnapshot("")
	require.NoError(t, err)
	found := false
	for _, f := range snapshot.Filters {
		if f.Id != uint32(filter.ID) {
			continue
		}
		found = true
		assert.NotContains(t, f.Config, "hub-encryption-key-value", "the hub's own setting reached the snapshot")
		// A refused name never ships: edges before 2.3 resolve $ENV/ against
		// their own environment. A bare "$ENV/" resolves to empty there.
		assert.NotContains(t, f.Config, "TYK_AI_SECRET_KEY", "a refused reference name reached the snapshot")
		// A secret field ships a dud key: a provider client could otherwise
		// fill an empty key from its own environment. A non-secret field
		// (the endpoint) ships a bare "$ENV/", empty on released edges.
		assert.Contains(t, f.Config, `"api_key":"`+secrets.RefusedCredential+`"`, "a refused credential ships as the dud key")
		assert.Contains(t, f.Config, `"endpoint":"$ENV/"`, "a refused endpoint ships as a bare $ENV/")
		assert.NotContains(t, f.Config, "unlisted-prefixed-value", "a GUARDRAIL_ prefix is not an exemption")
		assert.NotContains(t, f.Config, "GUARDRAIL_SNAPSHOT_KEY")
		// An allowlisted variable resolves, and as a secret field travels
		// encrypted with the edge key (vgi), never in clear.
		assert.NotContains(t, f.Config, "listed-key-value", "a resolved secret travels in clear")
		var cfg struct {
			Connection          map[string]string `json:"connection"`
			ConnectionEncrypted map[string]string `json:"connection_encrypted"`
		}
		require.NoError(t, json.Unmarshal([]byte(f.Config), &cfg))
		assert.Equal(t, secrets.RefusedCredential, cfg.Connection["key2"], "edges before 2.3 get the dud key")
		assert.Equal(t, "listed-key-value", decryptLikeMicrogateway(t, os.Getenv("MICROGATEWAY_ENCRYPTION_KEY"), cfg.ConnectionEncrypted["key2"]))
	}
	require.True(t, found, "filter missing from the snapshot")
	b, err := proto.Marshal(snapshot)
	require.NoError(t, err)
	assert.NotContains(t, string(b), "listed-key-value", "the serialized snapshot carries a resolved guardrail secret")
}
