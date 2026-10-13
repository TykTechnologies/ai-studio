package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withVaultSecret(t *testing.T) {
	t.Helper()
	t.Setenv("TYK_AI_SECRET_KEY", "test-encryption-key-for-secrets")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&secrets.Secret{}))
	secrets.SetDBRef(db)
	t.Cleanup(func() { secrets.SetDBRef(nil) })
	require.NoError(t, secrets.CreateSecret(db, &secrets.Secret{VarName: "VAULT_TOKEN", Value: "hvs.real-token"}))
}

func TestPluginInitConfigResolvesSecretReferences(t *testing.T) {
	withVaultSecret(t)
	t.Setenv("PLUGIN_TEST_HOST_SECRET", "host-only-value")

	stored := map[string]interface{}{
		"vault_token":     "$SECRET/VAULT_TOKEN",
		"host_env":        "$ENV/PLUGIN_TEST_HOST_SECRET",
		"missing":         "$SECRET/DOES_NOT_EXIST",
		"mid_string":      "prefix $SECRET/VAULT_TOKEN",
		"plain":           "https://vault.example.com",
		"count":           3,
		"enabled":         true,
		"vendors":         []interface{}{"a", "b"},
		"nested_not_seen": map[string]interface{}{"token": "$SECRET/VAULT_TOKEN"},
	}
	got := pluginInitConfig(stored)

	assert.Equal(t, "hvs.real-token", got["vault_token"], "$SECRET/ reference resolved")
	assert.Equal(t, "$ENV/PLUGIN_TEST_HOST_SECRET", got["host_env"], "$ENV/ is never resolved from AI Studio's environment")
	assert.Equal(t, "$SECRET/DOES_NOT_EXIST", got["missing"], "unresolvable reference passed through")
	assert.Equal(t, "prefix $SECRET/VAULT_TOKEN", got["mid_string"], "only an exact-prefix reference resolves")
	assert.Equal(t, "https://vault.example.com", got["plain"])
	assert.Equal(t, "3", got["count"])
	assert.Equal(t, "true", got["enabled"])
	assert.JSONEq(t, `["a","b"]`, got["vendors"])
	assert.JSONEq(t, `{"token":"$SECRET/VAULT_TOKEN"}`, got["nested_not_seen"], "only top-level strings are resolved")
	assert.Equal(t, "$SECRET/VAULT_TOKEN", stored["vault_token"], "the stored config keeps the reference")
}

func TestNewSecretReferences(t *testing.T) {
	old := map[string]interface{}{"kept": "$SECRET/A", "changed": "$SECRET/B", "plain": "x"}
	updated := map[string]interface{}{
		"kept":    "$SECRET/A", // unchanged: no new right needed
		"changed": "$SECRET/C", // different secret
		"plain":   "$SECRET/D", // literal turned into a reference
		"added":   "$SECRET/E",
		"other":   "literal",
		"env":     "$ENV/X", // not resolved, so not a secret reference
	}
	assert.Equal(t, []string{"added", "changed", "plain"}, NewSecretReferences(old, updated))
	assert.Empty(t, NewSecretReferences(nil, map[string]interface{}{"a": "b"}))
}

// A plugin that saves the config it was given (resolved) keeps the reference.
func TestRestoreSecretReferences(t *testing.T) {
	withVaultSecret(t)
	old := map[string]interface{}{"vault_token": "$SECRET/VAULT_TOKEN", "missing": "$SECRET/NOPE"}

	got := RestoreSecretReferences(old, map[string]interface{}{"vault_token": "hvs.real-token", "missing": "$SECRET/NOPE"})
	assert.Equal(t, "$SECRET/VAULT_TOKEN", got["vault_token"])

	got = RestoreSecretReferences(old, map[string]interface{}{"vault_token": "hvs.other-token"})
	assert.Equal(t, "hvs.other-token", got["vault_token"], "a different value replaces the reference")
}
