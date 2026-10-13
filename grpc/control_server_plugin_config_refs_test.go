package grpc

import (
	"encoding/json"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Plugin config secret references are resolved only for the Studio-side
// plugin process (services.pluginInitConfig). The snapshot sent to edges
// must carry the reference, never the secret value.
func TestControlServer_SnapshotKeepsPluginSecretReferences(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "test-encryption-key-for-secrets")
	server, db := setupTestServer(t, nil)
	require.NoError(t, db.AutoMigrate(&secrets.Secret{}))
	secrets.SetDBRef(db)
	t.Cleanup(func() { secrets.SetDBRef(nil) })
	require.NoError(t, secrets.CreateSecret(db, &secrets.Secret{VarName: "VAULT_TOKEN", Value: "hvs.real-token"}))

	plugin := &models.Plugin{Name: "rag", Command: "/bin/rag", HookType: models.HookTypeStudioUI, IsActive: true,
		Config: map[string]interface{}{"vault_token": "$SECRET/VAULT_TOKEN"}}
	require.NoError(t, db.Create(plugin).Error)

	snapshot, err := server.getConfigurationSnapshot("")
	require.NoError(t, err)
	found := false
	for _, p := range snapshot.Plugins {
		if p.Id != uint32(plugin.ID) {
			continue
		}
		found = true
		var cfg map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(p.Config), &cfg))
		assert.Equal(t, "$SECRET/VAULT_TOKEN", cfg["vault_token"])
		assert.NotContains(t, p.Config, "hvs.real-token")
	}
	require.True(t, found, "plugin missing from the snapshot")
}
