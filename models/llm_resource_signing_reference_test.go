package models

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// The first 2.3 start on a 2.2 database ran the signing-key backfill before
// the secret store had its database. A $SECRET/NAME credential (and an
// $ENV/NAME the allowlist refuses) then came back as its own text, and the
// key kept for IDs handed out before the upgrade was derived from that text:
// anyone who could read the LLM could mint IDs over another App's resources
// (midsommar-clean-6bg5.94).

// referenceDerivedKey is the previous-key value such a build stored for a
// credential reference: HKDF of the reference's text.
func referenceDerivedKey(t *testing.T, reference string) string {
	t.Helper()
	key, err := deriveCredentialSigningKey([]byte(reference))
	require.NoError(t, err)
	return ResourceSigningDerivedKeyPrefix + base64.RawURLEncoding.EncodeToString(key)
}

func credentialDerivedKey(t *testing.T, credential string) string {
	t.Helper()
	key, err := DeriveCredentialSigningKey(credential)
	require.NoError(t, err)
	return ResourceSigningDerivedKeyPrefix + base64.RawURLEncoding.EncodeToString(key)
}

// legacyLLM stores an LLM as 2.2 left it: no signing secret.
func legacyLLM(t *testing.T, db *gorm.DB, name, apiKey string) uint {
	t.Helper()
	l := &LLM{Name: name, Vendor: OPENAI, APIKey: apiKey}
	require.NoError(t, l.Create(db))
	require.NoError(t, db.Model(&LLM{}).Where("id = ?", l.ID).UpdateColumns(map[string]interface{}{
		"resource_signing_secret": "", "resource_signing_secret_prev": "",
		"resource_signing_prev_expires_at": nil, "resource_signing_rotated_at": nil,
	}).Error)
	return l.ID
}

func TestDeriveCredentialSigningKeyRefusesAReference(t *testing.T) {
	for _, ref := range []string{"$SECRET/OPENAI_KEY", "$ENV/OPENAI_API_KEY", "$ENV/", "$SECRET/"} {
		_, err := DeriveCredentialSigningKey(ref)
		assert.ErrorIs(t, err, ErrUnresolvedCredential, ref)
	}
	_, err := DeriveCredentialSigningKey("sk-a-real-key")
	assert.NoError(t, err)
}

func TestBackfillDerivesNoKeyFromAReferenceText(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "signing-unit-test-key")
	t.Setenv(secrets.EnvRefAllowlistVar, "ALLOWED_VENDOR_KEY")
	t.Setenv("ALLOWED_VENDOR_KEY", "sk-from-the-environment")
	t.Setenv("REFUSED_VENDOR_KEY", "sk-never-read")
	db := setupTestDB(t)
	require.NoError(t, secrets.CreateSecret(db, &secrets.Secret{VarName: "OPENAI_KEY", Value: "sk-from-the-secret-store"}))

	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		apiKey string
		// want is the previous key once the secret store can resolve; ""
		// means none.
		want string
	}{
		"secret":            {"$SECRET/OPENAI_KEY", credentialDerivedKey(t, "sk-from-the-secret-store")},
		"unknown secret":    {"$SECRET/NO_SUCH_SECRET", ""},
		"env, allowlisted":  {"$ENV/ALLOWED_VENDOR_KEY", credentialDerivedKey(t, "sk-from-the-environment")},
		"env, refused":      {"$ENV/REFUSED_VENDOR_KEY", ""},
		"literal":           {"sk-literal", credentialDerivedKey(t, "sk-literal")},
		"no credential set": {"", ""},
	}

	t.Run("the secret store has no database yet", func(t *testing.T) {
		// The state of the process when the backfill used to run.
		secrets.SetDBRef(nil)
		t.Cleanup(func() { secrets.SetDBRef(nil) })
		ids := map[string]uint{}
		for name, tc := range cases {
			ids[name] = legacyLLM(t, db, "nodb "+name, tc.apiKey)
		}
		_, err := BackfillLLMResourceSigningSecrets(db, DefaultResourceSigningGrace, now)
		require.NoError(t, err)
		for name, tc := range cases {
			k := signingKeysOf(t, db, ids[name])
			assert.NotEmpty(t, k.Secret, name)
			if secrets.IsReference(tc.apiKey) {
				assert.NotEqual(t, referenceDerivedKey(t, tc.apiKey), k.Previous, "%s: the previous key is derived from the reference's text", name)
			}
			if secrets.IsSecretReference(tc.apiKey) {
				// Unresolvable here: no key at all, rather than a forgeable one.
				assert.Empty(t, k.Previous, name)
				assert.Nil(t, k.PreviousExpiresAt, name)
			}
		}
	})

	t.Run("the secret store resolves", func(t *testing.T) {
		secrets.SetDBRef(db)
		t.Cleanup(func() { secrets.SetDBRef(nil) })
		ids := map[string]uint{}
		for name, tc := range cases {
			ids[name] = legacyLLM(t, db, "db "+name, tc.apiKey)
		}
		_, err := BackfillLLMResourceSigningSecrets(db, DefaultResourceSigningGrace, now)
		require.NoError(t, err)
		for name, tc := range cases {
			k := signingKeysOf(t, db, ids[name])
			assert.NotEmpty(t, k.Secret, name)
			assert.Equal(t, tc.want, k.Previous, "%s: previous key", name)
			if tc.want == "" {
				assert.Nil(t, k.PreviousExpiresAt, name)
			} else {
				require.NotNil(t, k.PreviousExpiresAt, name)
				assert.True(t, k.PreviousExpiresAt.Equal(now.Add(DefaultResourceSigningGrace)), name)
			}
		}
	})

	t.Run("a rotation of an LLM without a secret keeps no reference-derived key either", func(t *testing.T) {
		secrets.SetDBRef(nil)
		id := legacyLLM(t, db, "rotate", "$SECRET/OPENAI_KEY")
		require.NoError(t, RotateLLMResourceSigningSecret(db, id, time.Hour, now))
		k := signingKeysOf(t, db, id)
		assert.Empty(t, k.Previous)
		assert.Nil(t, k.PreviousExpiresAt)
	})
}

// A database upgraded with a 2.3 build before the fix already holds such
// keys, trusted for 30 days. The fixed build stops trusting them on start.
func TestClearReferenceDerivedSigningKeys(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "signing-unit-test-key")
	t.Setenv(secrets.EnvRefAllowlistVar, "ALLOWED_VENDOR_KEY")
	db := setupTestDB(t)
	secrets.SetDBRef(db)
	t.Cleanup(func() { secrets.SetDBRef(nil) })
	require.NoError(t, secrets.CreateSecret(db, &secrets.Secret{VarName: "OPENAI_KEY", Value: "sk-from-the-secret-store"}))
	require.NoError(t, secrets.CreateSecret(db, &secrets.Secret{VarName: "OLD_KEY", Value: "sk-old"}))

	expires := time.Now().UTC().Add(DefaultResourceSigningGrace)
	withPrev := func(name, apiKey, prev string) uint {
		l := &LLM{Name: name, Vendor: OPENAI, APIKey: apiKey}
		require.NoError(t, l.Create(db))
		require.NoError(t, db.Model(&LLM{}).Where("id = ?", l.ID).UpdateColumns(map[string]interface{}{
			"resource_signing_secret_prev":     secrets.EncryptValue(prev),
			"resource_signing_prev_expires_at": expires,
		}).Error)
		return l.ID
	}
	rotatedSecret, err := NewResourceSigningSecret()
	require.NoError(t, err)

	forged := map[string]uint{
		"own $SECRET reference":             withPrev("f1", "$SECRET/OPENAI_KEY", referenceDerivedKey(t, "$SECRET/OPENAI_KEY")),
		"own refused $ENV reference":        withPrev("f2", "$ENV/REFUSED_VENDOR_KEY", referenceDerivedKey(t, "$ENV/REFUSED_VENDOR_KEY")),
		"credential since made a literal":   withPrev("f3", "sk-now-literal", referenceDerivedKey(t, "$SECRET/OLD_KEY")),
		"reference since made allowlisted":  withPrev("f4", "sk-other", referenceDerivedKey(t, "$ENV/ALLOWED_VENDOR_KEY")),
		"reference another LLM still names": withPrev("f5", "sk-third", referenceDerivedKey(t, "$ENV/REFUSED_VENDOR_KEY")),
	}
	kept := map[string]struct {
		id   uint
		prev string
	}{
		"key of the resolved credential": {prev: credentialDerivedKey(t, "sk-from-the-secret-store")},
		"key of a literal credential":    {prev: credentialDerivedKey(t, "sk-literal")},
		"a rotated signing secret":       {prev: rotatedSecret},
		"the legacy sentinel":            {prev: ResourceSigningLegacyKey},
	}
	for name, k := range kept {
		k.id = withPrev("k "+name, "$SECRET/OPENAI_KEY", k.prev)
		kept[name] = k
	}
	// A deleted LLM is cleaned too: it can be restored.
	deleted := withPrev("deleted", "$SECRET/OPENAI_KEY", referenceDerivedKey(t, "$SECRET/OPENAI_KEY"))
	require.NoError(t, db.Delete(&LLM{}, deleted).Error)

	n, err := ClearReferenceDerivedSigningKeys(db)
	require.NoError(t, err)
	assert.Equal(t, len(forged)+1, n)

	for name, id := range forged {
		k := signingKeysOf(t, db, id)
		assert.Empty(t, k.Previous, "%s: still trusted", name)
		assert.Nil(t, k.PreviousExpiresAt, name)
		assert.NotEmpty(t, k.Secret, "%s: the current secret is untouched", name)
	}
	for name, want := range kept {
		k := signingKeysOf(t, db, want.id)
		assert.Equal(t, want.prev, k.Previous, "%s: a legitimate previous key was removed", name)
		assert.NotNil(t, k.PreviousExpiresAt, name)
	}
	var gone LLM
	require.NoError(t, db.Unscoped().First(&gone, deleted).Error)
	assert.Empty(t, gone.ResourceSigningSecretPrev)

	n, err = ClearReferenceDerivedSigningKeys(db)
	require.NoError(t, err)
	assert.Zero(t, n, "nothing left to clear")
}
