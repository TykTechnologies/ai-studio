package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginInitConfigResolvesReferences(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "test-encryption-key-for-secrets")
	t.Setenv("PLUGIN_TEST_TOKEN", "token-from-env")

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&secrets.Secret{}))
	secrets.SetDBRef(db)
	t.Cleanup(func() { secrets.SetDBRef(nil) })
	require.NoError(t, secrets.CreateSecret(db, &secrets.Secret{VarName: "VAULT_TOKEN", Value: "hvs.real-token"}))

	stored := map[string]interface{}{
		"vault_token":     "$SECRET/VAULT_TOKEN",
		"other_token":     "$ENV/PLUGIN_TEST_TOKEN",
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
	assert.Equal(t, "token-from-env", got["other_token"], "$ENV/ reference resolved")
	assert.Equal(t, "$SECRET/DOES_NOT_EXIST", got["missing"], "unresolvable reference passed through")
	assert.Equal(t, "prefix $SECRET/VAULT_TOKEN", got["mid_string"], "only an exact-prefix reference resolves")
	assert.Equal(t, "https://vault.example.com", got["plain"])
	assert.Equal(t, "3", got["count"])
	assert.Equal(t, "true", got["enabled"])
	assert.JSONEq(t, `["a","b"]`, got["vendors"])
	assert.JSONEq(t, `{"token":"$SECRET/VAULT_TOKEN"}`, got["nested_not_seen"], "only top-level strings are resolved")

	// The stored config keeps the reference
	assert.Equal(t, "$SECRET/VAULT_TOKEN", stored["vault_token"])
}
