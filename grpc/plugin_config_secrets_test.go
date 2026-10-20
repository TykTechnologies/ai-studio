package grpc

import (
	"encoding/json"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func setupPluginSecretServer(t *testing.T) (*ControlServer, func(name, value string)) {
	t.Helper()
	t.Setenv("TYK_AI_SECRET_KEY", "test-encryption-key-for-secrets")
	server, db := setupTestServer(t, nil)
	require.NoError(t, db.AutoMigrate(&secrets.Secret{}))
	secrets.SetDBRef(db)
	t.Cleanup(func() { secrets.SetDBRef(nil) })
	return server, func(name, value string) {
		require.NoError(t, db.Unscoped().Where("var_name = ?", name).Delete(&secrets.Secret{}).Error)
		require.NoError(t, secrets.CreateSecret(db, &secrets.Secret{VarName: name, Value: value}))
	}
}

func snapshotPlugin(t *testing.T, server *ControlServer, id uint) (*pb.ConfigurationSnapshot, *pb.PluginConfig) {
	t.Helper()
	snapshot, err := server.getConfigurationSnapshot("")
	require.NoError(t, err)
	for _, p := range snapshot.Plugins {
		if p.Id == uint32(id) {
			return snapshot, p
		}
	}
	t.Fatalf("plugin %d missing from the snapshot", id)
	return nil, nil
}

// An edge plugin's top-level $SECRET/ reference reaches the edge encrypted,
// in its own field, and its value appears nowhere else in the snapshot. The
// plain config keeps the reference for edges that predate the field.
func TestSnapshot_EdgePluginSecretReferencesAreEncrypted(t *testing.T) {
	server, putSecret := setupPluginSecretServer(t)
	putSecret("ENGINE_TOKEN", "hr-real-engine-token")
	t.Setenv("ENGINE_ENV_TOKEN", "env-value-must-not-leave")
	t.Setenv(secrets.EnvRefAllowlistVar, "ENGINE_ENV_TOKEN")

	plugin := &models.Plugin{Name: "prompt-compressor", Command: "/bin/pc", IsActive: true,
		HookType: models.HookTypePostAuth, HookTypes: []string{models.HookTypeStudioUI, models.HookTypePostAuth},
		Config: map[string]interface{}{
			"engine_main_token": "$SECRET/ENGINE_TOKEN",
			"engine_main_url":   "http://127.0.0.1:8787",
			"engine_gone_token": "$SECRET/NO_SUCH_SECRET",
			"engine_env_token":  "$ENV/ENGINE_ENV_TOKEN",
			"policies":          map[string]interface{}{"max": map[string]interface{}{"token": "$SECRET/ENGINE_TOKEN"}},
		}}
	require.NoError(t, server.db.Create(plugin).Error)

	snapshot, p := snapshotPlugin(t, server, plugin.ID)

	require.Len(t, p.ConfigSecretsEncrypted, 2, "only top-level $SECRET/ references: %v", p.ConfigSecretsEncrypted)
	assert.Equal(t, "hr-real-engine-token", decryptLikeMicrogateway(t, testEncryptionKey, p.ConfigSecretsEncrypted["engine_main_token"]))
	assert.Equal(t, secrets.RefusedCredential, decryptLikeMicrogateway(t, testEncryptionKey, p.ConfigSecretsEncrypted["engine_gone_token"]),
		"an unresolvable reference never goes out as the reference")

	var cfg map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(p.Config), &cfg))
	assert.Equal(t, "$SECRET/ENGINE_TOKEN", cfg["engine_main_token"])
	assert.Equal(t, "$ENV/ENGINE_ENV_TOKEN", cfg["engine_env_token"], "$ENV/ is never resolved for plugin config")
	assert.Equal(t, "$SECRET/ENGINE_TOKEN", cfg["policies"].(map[string]interface{})["max"].(map[string]interface{})["token"],
		"a nested reference is left as is")

	wire, err := proto.Marshal(snapshot)
	require.NoError(t, err)
	assert.NotContains(t, string(wire), "hr-real-engine-token")
	assert.NotContains(t, string(wire), "env-value-must-not-leave")
}

// A plugin an edge never runs gets no secret values on the edges.
func TestSnapshot_StudioOnlyPluginGetsNoSecrets(t *testing.T) {
	server, putSecret := setupPluginSecretServer(t)
	putSecret("VAULT_TOKEN", "hvs.real-token")

	studioOnly := &models.Plugin{Name: "rag", Command: "/bin/rag", HookType: models.HookTypeStudioUI, IsActive: true,
		Config: map[string]interface{}{"vault_token": "$SECRET/VAULT_TOKEN"}}
	require.NoError(t, server.db.Create(studioOnly).Error)
	inactive := &models.Plugin{Name: "off", Command: "/bin/off", HookType: models.HookTypePostAuth, IsActive: true,
		Config: map[string]interface{}{"vault_token": "$SECRET/VAULT_TOKEN"}}
	require.NoError(t, server.db.Create(inactive).Error)
	require.NoError(t, server.db.Model(inactive).Update("is_active", false).Error)

	_, p := snapshotPlugin(t, server, studioOnly.ID)
	assert.Empty(t, p.ConfigSecretsEncrypted)
	snapshot, err := server.getConfigurationSnapshot("")
	require.NoError(t, err)
	for _, sp := range snapshot.Plugins {
		if sp.Id == uint32(inactive.ID) {
			assert.Empty(t, sp.ConfigSecretsEncrypted)
		}
	}
}

// Rotating a referenced secret changes the snapshot checksum (so edges show
// as pending until it is pushed); an unchanged secret keeps it stable.
func TestSnapshot_PluginSecretRotationChangesChecksum(t *testing.T) {
	server, putSecret := setupPluginSecretServer(t)
	putSecret("ENGINE_TOKEN", "first")
	plugin := &models.Plugin{Name: "pc", Command: "/bin/pc", HookType: models.HookTypePostAuth, IsActive: true,
		Config: map[string]interface{}{"engine_main_token": "$SECRET/ENGINE_TOKEN"}}
	require.NoError(t, server.db.Create(plugin).Error)

	checksum := func() string {
		snapshot, p := snapshotPlugin(t, server, plugin.ID)
		require.NotEmpty(t, p.ConfigSecretsEncrypted["engine_main_token"])
		sum, err := ComputeSnapshotChecksum(snapshot)
		require.NoError(t, err)
		return sum
	}
	first := checksum()
	assert.Equal(t, first, checksum(), "the same secret gives the same checksum")

	putSecret("ENGINE_TOKEN", "second")
	_, p := snapshotPlugin(t, server, plugin.ID)
	assert.Equal(t, "second", decryptLikeMicrogateway(t, testEncryptionKey, p.ConfigSecretsEncrypted["engine_main_token"]))
	assert.NotEqual(t, first, checksum())
}

// A reference that a per-LLM override put in the merged config is not
// resolved: writing an override does not need the right to read secrets, so
// resolving it would hand any LLM editor every secret's value.
func TestPluginConfigSecretsForEdge_OverrideReferenceNotResolved(t *testing.T) {
	server, putSecret := setupPluginSecretServer(t)
	putSecret("ENGINE_TOKEN", "base-token")
	putSecret("OTHER_KEY", "someone-elses-key")

	plugin := &models.Plugin{ID: 9, HookType: models.HookTypePostAuth, IsActive: true,
		Config: map[string]interface{}{"engine_main_token": "$SECRET/ENGINE_TOKEN"}}
	merged := map[string]interface{}{
		"engine_main_token":  "$SECRET/ENGINE_TOKEN",
		"engine_other_token": "$SECRET/OTHER_KEY",
	}
	got := server.pluginConfigSecretsForEdge(plugin, merged)
	require.Len(t, got, 1)
	assert.Equal(t, "base-token", decryptLikeMicrogateway(t, testEncryptionKey, got["engine_main_token"]))

	merged["engine_main_token"] = "$SECRET/OTHER_KEY"
	assert.Empty(t, server.pluginConfigSecretsForEdge(plugin, merged), "an override that swaps the reference is not resolved")
}

// A secret change recomputes the snapshot: edges whose plugin references the
// secret show as pending until the new value is pushed.
func TestSecretChangeMarksEdgesPending(t *testing.T) {
	server, putSecret := setupPluginSecretServer(t)
	putSecret("ENGINE_TOKEN", "first")
	plugin := &models.Plugin{Name: "pc", Command: "/bin/pc", HookType: models.HookTypePostAuth, IsActive: true,
		Config: map[string]interface{}{"engine_main_token": "$SECRET/ENGINE_TOKEN"}}
	require.NoError(t, server.db.Create(plugin).Error)
	edge := models.EdgeInstance{EdgeID: "edge-1", Namespace: "default", Status: models.EdgeStatusRegistered, SyncStatus: models.EdgeSyncStatusInSync}
	require.NoError(t, server.db.Create(&edge).Error)
	syncOf := func() string {
		var e models.EdgeInstance
		require.NoError(t, server.db.Where("edge_id = ?", "edge-1").First(&e).Error)
		return e.SyncStatus
	}
	inSync := func() {
		require.NoError(t, server.db.Model(&models.EdgeInstance{}).Where("edge_id = ?", "edge-1").Update("sync_status", models.EdgeSyncStatusInSync).Error)
	}

	server.onConfigurationChanged(topicSecretUpdated, eventbridge.Event{ID: "1"})
	inSync()
	server.onConfigurationChanged(topicSecretUpdated, eventbridge.Event{ID: "2"})
	assert.Equal(t, models.EdgeSyncStatusInSync, syncOf(), "an unchanged secret leaves edges in sync")

	putSecret("ENGINE_TOKEN", "second")
	server.onConfigurationChanged(topicSecretUpdated, eventbridge.Event{ID: "3"})
	assert.Equal(t, models.EdgeSyncStatusPending, syncOf())
}
