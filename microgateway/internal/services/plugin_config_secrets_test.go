package services

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const pluginSecretTestKey = "abcdefghijklmnopqrstuvwxyz012345"

func encryptedConfigSecret(t *testing.T, plaintext string) string {
	t.Helper()
	enc, err := NewCryptoService(pluginSecretTestKey).Encrypt(plaintext)
	require.NoError(t, err)
	return enc
}

func syncSecretPlugin(t *testing.T, svc *EdgeSyncService, config string, encrypted map[string]string) {
	t.Helper()
	now := time.Now()
	require.NoError(t, svc.SyncConfiguration(&pb.ConfigurationSnapshot{
		Version:      "1",
		SnapshotTime: timestamppb.Now(),
		Llms:         []*pb.LLMConfig{authPluginTestLLM(1, "a", now)},
		Plugins: []*pb.PluginConfig{{
			Id: 42, Name: "prompt-compressor", Command: "file:///pc", HookType: "post_auth",
			HookTypes: []string{"post_auth", "studio_ui"}, IsActive: true, LlmIds: []uint32{1},
			Config: config, ConfigSecretsEncrypted: encrypted,
			CreatedAt: timestamppb.New(now), UpdatedAt: timestamppb.New(now),
		}},
	}))
}

// The hub's encrypted value for a top-level $SECRET/ reference reaches the
// plugin's Initialize config. The edge stores it encrypted, the plugin row
// (what the admin API returns) keeps the reference, and no log line carries
// the value.
func TestPluginConfigSecrets_EdgeDecryptsForInitializeOnly(t *testing.T) {
	var logs bytes.Buffer
	prev := log.Logger
	log.Logger = zerolog.New(&logs).Level(zerolog.DebugLevel)
	t.Cleanup(func() { log.Logger = prev })

	db := setupEdgeSyncTestDB(t)
	svc := NewEdgeSyncService(db, "")
	ciphertext := encryptedConfigSecret(t, "hr-real-engine-token")
	syncSecretPlugin(t, svc, `{"engine_main_token":"$SECRET/ENGINE_TOKEN","engine_main_url":"http://127.0.0.1:8787"}`,
		map[string]string{"engine_main_token": ciphertext})

	adapter := NewPluginServiceAdapter(NewPluginService(db, database.NewRepository(db)))
	adapter.SetCrypto(NewCryptoService(pluginSecretTestKey))

	pd, err := adapter.GetPlugin(42)
	require.NoError(t, err)
	var cfg map[string]interface{}
	require.NoError(t, json.Unmarshal(pd.Config, &cfg))
	assert.Equal(t, "hr-real-engine-token", cfg["engine_main_token"])
	assert.Equal(t, "http://127.0.0.1:8787", cfg["engine_main_url"])

	forLLM, err := adapter.GetPluginsForLLM(1)
	require.NoError(t, err)
	require.Len(t, forLLM, 1)
	assert.Equal(t, pd.Config, forLLM[0].Config, "every lookup path gets the same config")

	var row database.Plugin
	require.NoError(t, db.First(&row, 42).Error)
	assert.NotContains(t, string(row.Config), "hr-real-engine-token")
	assert.Contains(t, string(row.Config), "$SECRET/ENGINE_TOKEN")
	assert.Contains(t, string(row.ConfigSecretsEncrypted), ciphertext, "stored encrypted at rest")
	assert.NotContains(t, string(row.ConfigSecretsEncrypted), "hr-real-engine-token")

	echoed, err := json.Marshal(row)
	require.NoError(t, err)
	assert.NotContains(t, string(echoed), "hr-real-engine-token")
	assert.NotContains(t, string(echoed), ciphertext, "the admin API never echoes the secret field")

	assert.NotContains(t, logs.String(), "hr-real-engine-token")
}

// Rotating the secret on the hub changes the config the edge initialises the
// plugin with, which is what ReconcilePlugins reloads on
// (TestReconcilePlugins_DetectsChangedConfig).
func TestPluginConfigSecrets_RotationChangesInitializeConfig(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	svc := NewEdgeSyncService(db, "")
	adapter := NewPluginServiceAdapter(NewPluginService(db, database.NewRepository(db)))
	adapter.SetCrypto(NewCryptoService(pluginSecretTestKey))
	config := `{"engine_main_token":"$SECRET/ENGINE_TOKEN"}`

	syncSecretPlugin(t, svc, config, map[string]string{"engine_main_token": encryptedConfigSecret(t, "first")})
	before, err := adapter.GetPlugin(42)
	require.NoError(t, err)

	syncSecretPlugin(t, svc, config, map[string]string{"engine_main_token": encryptedConfigSecret(t, "second")})
	after, err := adapter.GetPlugin(42)
	require.NoError(t, err)

	assert.NotEqual(t, before.Config, after.Config)
	assert.Contains(t, string(after.Config), `"second"`)
}

// Only a key whose stored value is a $SECRET/ reference is replaced; a value
// that does not decrypt becomes the refused-credential marker; every other
// value is passed on byte for byte.
func TestPluginConfigSecrets_MergeRules(t *testing.T) {
	adapter := &PluginServiceAdapter{crypto: NewCryptoService(pluginSecretTestKey)}
	p := database.Plugin{
		ID: 42,
		Config: database.JSON(`{"engine_main_token":"$SECRET/A","engine_bad_token":"$SECRET/B","plain":"keep",` +
			`"env":"$ENV/HOME","big":12345678901234567890,"nested":{"t":"$SECRET/A"}}`),
	}
	encrypted, err := json.Marshal(map[string]string{
		"engine_main_token": encryptedConfigSecret(t, "tok"),
		"engine_bad_token":  "not-a-ciphertext",
		"plain":             encryptedConfigSecret(t, "injected"),
		"env":               encryptedConfigSecret(t, "injected"),
		"nested":            encryptedConfigSecret(t, "injected"),
		"absent":            encryptedConfigSecret(t, "injected"),
	})
	require.NoError(t, err)
	p.ConfigSecretsEncrypted = database.JSON(encrypted)

	got := adapter.pluginConfigBytes(p)
	var cfg map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(got, &cfg))
	assert.JSONEq(t, `"tok"`, string(cfg["engine_main_token"]))
	assert.JSONEq(t, `"`+secrets.RefusedCredential+`"`, string(cfg["engine_bad_token"]))
	assert.JSONEq(t, `"keep"`, string(cfg["plain"]))
	assert.JSONEq(t, `"$ENV/HOME"`, string(cfg["env"]))
	assert.JSONEq(t, `{"t":"$SECRET/A"}`, string(cfg["nested"]))
	assert.Equal(t, "12345678901234567890", string(cfg["big"]))
	assert.NotContains(t, cfg, "absent")
	assert.NotContains(t, string(got), "injected")

	// Without the hub's field, the stored config goes out as before.
	p.ConfigSecretsEncrypted = nil
	plain, _ := json.Marshal(p.Config)
	assert.Equal(t, plain, adapter.pluginConfigBytes(p))
}
