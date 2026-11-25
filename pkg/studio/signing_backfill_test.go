//go:build !enterprise

package studio

import (
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/studiodb"
	"github.com/TykTechnologies/midsommar/v2/secrets"
)

// The first 2.3 start on a 2.2 database generates each LLM's resource signing
// secret and keeps the key of its vendor credential for the IDs handed out
// before. That step ran before the secret store had its database, so an LLM
// whose credential is $SECRET/NAME kept the key of the text "$SECRET/NAME",
// which any reader of the LLM can compute (midsommar-clean-6bg5.94).
func TestFirstStartKeepsNoSigningKeyDerivedFromAReferenceText(t *testing.T) {
	opts := newTestOptions(t)
	t.Setenv("TYK_AI_SECRET_KEY", "embedded-test-secret-key")
	// This process has not given the secret store a database yet, as on a
	// real start.
	secrets.SetDBRef(nil)

	db := opts.DB
	require.NoError(t, studiodb.Migrate(db))
	require.NoError(t, secrets.CreateSecret(db, &secrets.Secret{VarName: "OPENAI_KEY", Value: "sk-from-the-secret-store"}))

	derived := func(material string) string {
		key, err := models.DeriveCredentialSigningKey(material)
		require.NoError(t, err)
		return models.ResourceSigningDerivedKeyPrefix + base64.RawURLEncoding.EncodeToString(key)
	}
	seed := func(name, apiKey string, cols map[string]interface{}) uint {
		l := &models.LLM{Name: name, Vendor: models.OPENAI, APIKey: apiKey, APIEndpoint: "https://api.openai.com/v1"}
		require.NoError(t, l.Create(db))
		require.NoError(t, db.Model(&models.LLM{}).Where("id = ?", l.ID).UpdateColumns(cols).Error)
		return l.ID
	}
	asIn22 := map[string]interface{}{"resource_signing_secret": "", "resource_signing_secret_prev": "",
		"resource_signing_prev_expires_at": nil, "resource_signing_rotated_at": nil}
	bySecret := seed("from-2.2 secret", "$SECRET/OPENAI_KEY", asIn22)
	byRefusedEnv := seed("from-2.2 env", "$ENV/NOT_ALLOWLISTED_KEY", asIn22)
	byLiteral := seed("from-2.2 literal", "sk-literal", asIn22)

	// A database an earlier 2.3 build has already upgraded: the previous key
	// in it is the key of the reference's text.
	poisonedKey := poisonedPreviousKey(t, "$SECRET/OPENAI_KEY")
	l := &models.LLM{Name: "upgraded by an earlier 2.3 build", Vendor: models.OPENAI, APIKey: "$SECRET/OPENAI_KEY", APIEndpoint: "https://api.openai.com/v1"}
	require.NoError(t, l.Create(db))
	require.NoError(t, db.Model(&models.LLM{}).Where("id = ?", l.ID).UpdateColumns(map[string]interface{}{
		"resource_signing_secret_prev":     secrets.EncryptValue(poisonedKey),
		"resource_signing_prev_expires_at": l.CreatedAt.Add(models.DefaultResourceSigningGrace),
	}).Error)
	poisoned := l.ID

	s, err := New(opts)
	require.NoError(t, err)
	stopStudio(t, s)

	keys := func(id uint) models.ResourceSigningKeys {
		var got models.LLM
		require.NoError(t, db.First(&got, id).Error)
		return got.ResourceSigningKeys()
	}

	k := keys(bySecret)
	assert.NotEmpty(t, k.Secret)
	assert.NotEqual(t, poisonedKey, k.Previous, "the previous key is the key of the reference's text")
	assert.Equal(t, derived("sk-from-the-secret-store"), k.Previous, "IDs signed before the upgrade used the resolved credential")
	assert.NotNil(t, k.PreviousExpiresAt)

	k = keys(byRefusedEnv)
	assert.NotEmpty(t, k.Secret)
	assert.Empty(t, k.Previous, "a reference that does not resolve leaves no previous key")
	assert.Nil(t, k.PreviousExpiresAt)

	k = keys(byLiteral)
	assert.Equal(t, derived("sk-literal"), k.Previous)

	k = keys(poisoned)
	assert.NotEmpty(t, k.Secret, "the current secret is kept")
	assert.Empty(t, k.Previous, "a reference-derived key an earlier build stored is no longer trusted")
	assert.Nil(t, k.PreviousExpiresAt)
}

// poisonedPreviousKey is what a 2.3 build before the fix stored as the
// previous key of an LLM whose credential was the given reference.
func poisonedPreviousKey(t *testing.T, reference string) string {
	t.Helper()
	key, err := hkdf.Key(sha256.New, []byte(reference), nil, "tyk-native-resource/v1", sha256.Size)
	require.NoError(t, err)
	return models.ResourceSigningDerivedKeyPrefix + base64.RawURLEncoding.EncodeToString(key)
}
