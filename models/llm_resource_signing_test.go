package models

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// getLLM reads a fresh row (gorm keeps a reused struct's pointer fields when
// the column is NULL).
func getLLM(t *testing.T, db *gorm.DB, id uint) LLM {
	t.Helper()
	var l LLM
	require.NoError(t, db.First(&l, id).Error)
	return l
}

func signingKeysOf(t *testing.T, db *gorm.DB, id uint) ResourceSigningKeys {
	t.Helper()
	l := getLLM(t, db, id)
	return l.ResourceSigningKeys()
}

func TestLLMCreateGeneratesEncryptedSigningSecret(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "signing-unit-test-key")
	db := setupTestDB(t)
	a := &LLM{Name: "A", Vendor: ANTHROPIC}
	b := &LLM{Name: "B", Vendor: ANTHROPIC}
	require.NoError(t, a.Create(db))
	require.NoError(t, b.Create(db))

	stored := getLLM(t, db, a.ID)
	assert.True(t, strings.HasPrefix(stored.ResourceSigningSecret, "$ENC/"), "the secret is encrypted at rest")
	ka := stored.ResourceSigningKeys()
	assert.Len(t, ka.Secret, 43, "32 random bytes, base64url")
	assert.NotNil(t, ka.RotatedAt)
	assert.Empty(t, ka.Previous)

	stored = getLLM(t, db, b.ID)
	assert.NotEqual(t, ka.Secret, stored.ResourceSigningKeys().Secret, "every LLM gets its own secret")

	out, err := json.Marshal(stored)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "resource_signing")
	assert.NotContains(t, string(out), stored.ResourceSigningSecret)
}

// LLM.Update never writes the signing columns: an edit rebuilt from JSON
// (no secret) keeps the stored one, and an edit that loaded the LLM before a
// rotation does not undo it.
func TestLLMUpdateLeavesSigningKeysAlone(t *testing.T) {
	db := setupTestDB(t)
	l := &LLM{Name: "A", Vendor: ANTHROPIC}
	require.NoError(t, l.Create(db))
	loaded := getLLM(t, db, l.ID)

	require.NoError(t, RotateLLMResourceSigningSecret(db, l.ID, time.Hour, time.Now()))
	rotated := getLLM(t, db, l.ID)

	loaded.ShortDescription = "stale edit"
	require.NoError(t, loaded.Update(db))
	rebuilt := &LLM{Name: "A", Vendor: ANTHROPIC, ShortDescription: "from json"}
	rebuilt.ID, rebuilt.Model.ID, rebuilt.CreatedAt = l.ID, l.ID, l.CreatedAt
	require.NoError(t, rebuilt.Update(db))

	after := getLLM(t, db, l.ID)
	assert.Equal(t, "from json", after.ShortDescription)
	assert.Equal(t, rotated.ResourceSigningSecret, after.ResourceSigningSecret)
	assert.Equal(t, rotated.ResourceSigningSecretPrev, after.ResourceSigningSecretPrev)
	require.NotNil(t, after.ResourceSigningPrevExpiresAt)
}

func TestRotateLLMResourceSigningSecret(t *testing.T) {
	db := setupTestDB(t)
	l := &LLM{Name: "A", Vendor: ANTHROPIC}
	require.NoError(t, l.Create(db))
	before := getLLM(t, db, l.ID)
	k0 := before.ResourceSigningKeys()

	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	require.NoError(t, RotateLLMResourceSigningSecret(db, l.ID, 2*time.Hour, now))
	after := getLLM(t, db, l.ID)
	k1 := after.ResourceSigningKeys()
	assert.NotEqual(t, k0.Secret, k1.Secret)
	assert.Equal(t, k0.Secret, k1.Previous, "the old secret is kept for the grace period")
	require.NotNil(t, k1.PreviousExpiresAt)
	assert.True(t, k1.PreviousExpiresAt.Equal(now.Add(2*time.Hour)))
	assert.True(t, k1.RotatedAt.Equal(now))
	assert.True(t, k1.PreviousActive(now.Add(time.Hour)))
	assert.False(t, k1.PreviousActive(now.Add(3*time.Hour)))

	// No grace revokes the previous key outright.
	require.NoError(t, RotateLLMResourceSigningSecret(db, l.ID, 0, now))
	after = getLLM(t, db, l.ID)
	k2 := after.ResourceSigningKeys()
	assert.NotEqual(t, k1.Secret, k2.Secret)
	assert.Empty(t, k2.Previous)
	assert.Nil(t, k2.PreviousExpiresAt)
	assert.False(t, k2.PreviousActive(now))

	assert.Error(t, RotateLLMResourceSigningSecret(db, l.ID, -time.Second, now))
	assert.Error(t, RotateLLMResourceSigningSecret(db, l.ID, MaxResourceSigningGrace+time.Second, now))
	assert.Error(t, RotateLLMResourceSigningSecret(db, 9999, time.Hour, now), "unknown LLM")
}

// Concurrent rotations each compare-and-set on the secret they read: no
// two of them both win from the same starting key, so none silently drops
// the key another one kept as previous.
func TestRotateLLMResourceSigningSecretConcurrent(t *testing.T) {
	db := setupTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1) // one :memory: database
	l := &LLM{Name: "A", Vendor: ANTHROPIC}
	require.NoError(t, l.Create(db))

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- RotateLLMResourceSigningSecret(db, l.ID, time.Hour, time.Now())
		}()
	}
	wg.Wait()
	close(errs)
	won := 0
	for err := range errs {
		if err == nil {
			won++
			continue
		}
		assert.ErrorIs(t, err, ErrResourceSigningConflict)
	}
	assert.GreaterOrEqual(t, won, 1)
}

func TestBackfillLLMResourceSigningSecrets(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "signing-unit-test-key")
	db := setupTestDB(t)
	withSecret := &LLM{Name: "Has", Vendor: ANTHROPIC, APIKey: "k0"}
	require.NoError(t, withSecret.Create(db))
	legacy := &LLM{Name: "Legacy", Vendor: ANTHROPIC, APIKey: "vendor-key"}
	require.NoError(t, legacy.Create(db))
	keyless := &LLM{Name: "Keyless", Vendor: OLLAMA}
	require.NoError(t, keyless.Create(db))
	clear := map[string]interface{}{"resource_signing_secret": "", "resource_signing_rotated_at": nil}
	require.NoError(t, db.Model(&LLM{}).Where("id IN ?", []uint{legacy.ID, keyless.ID}).UpdateColumns(clear).Error)
	keep := getLLM(t, db, withSecret.ID)

	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	n, err := BackfillLLMResourceSigningSecrets(db, DefaultResourceSigningGrace, now)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	derived, err := DeriveCredentialSigningKey("vendor-key")
	require.NoError(t, err)
	wantPrev := ResourceSigningDerivedKeyPrefix + base64.RawURLEncoding.EncodeToString(derived)

	got := getLLM(t, db, legacy.ID)
	assert.True(t, strings.HasPrefix(got.ResourceSigningSecretPrev, "$ENC/"), "the stored key is encrypted at rest")
	k := got.ResourceSigningKeys()
	assert.NotEmpty(t, k.Secret)
	assert.Equal(t, wantPrev, k.Previous, "the vendor-credential key is stored, not re-derived later")
	assert.True(t, k.PreviousIsCredential())
	require.NotNil(t, k.PreviousExpiresAt)
	assert.True(t, k.PreviousExpiresAt.Equal(now.Add(DefaultResourceSigningGrace)))

	// Changing the credential afterwards leaves the stored key alone.
	require.NoError(t, db.Model(&LLM{}).Where("id = ?", legacy.ID).Update("api_key", "rotated-key").Error)
	assert.Equal(t, wantPrev, signingKeysOf(t, db, legacy.ID).Previous)

	// An LLM without a credential could not have handed out IDs: no previous key.
	kk := signingKeysOf(t, db, keyless.ID)
	assert.NotEmpty(t, kk.Secret)
	assert.Empty(t, kk.Previous)
	assert.Nil(t, kk.PreviousExpiresAt)

	got = getLLM(t, db, withSecret.ID)
	assert.Equal(t, keep.ResourceSigningSecret, got.ResourceSigningSecret, "an LLM with a secret is untouched")

	n, err = BackfillLLMResourceSigningSecrets(db, DefaultResourceSigningGrace, now)
	require.NoError(t, err)
	assert.Zero(t, n, "the backfill is idempotent")

	// Rotating an LLM that never had a secret stores its vendor-credential
	// key as the previous one, the same way.
	require.NoError(t, db.Model(&LLM{}).Where("id = ?", legacy.ID).
		UpdateColumns(map[string]interface{}{"resource_signing_secret": "", "resource_signing_secret_prev": ""}).Error)
	require.NoError(t, RotateLLMResourceSigningSecret(db, legacy.ID, time.Hour, now))
	rotatedDerived, err := DeriveCredentialSigningKey("rotated-key")
	require.NoError(t, err)
	assert.Equal(t, ResourceSigningDerivedKeyPrefix+base64.RawURLEncoding.EncodeToString(rotatedDerived),
		signingKeysOf(t, db, legacy.ID).Previous)

	// A keyless LLM's rotation without a secret keeps no previous key.
	require.NoError(t, db.Model(&LLM{}).Where("id = ?", keyless.ID).Update("resource_signing_secret", "").Error)
	require.NoError(t, RotateLLMResourceSigningSecret(db, keyless.ID, time.Hour, now))
	kk = signingKeysOf(t, db, keyless.ID)
	assert.Empty(t, kk.Previous)
	assert.Nil(t, kk.PreviousExpiresAt)
}

func TestResourceSigningKeysPreviousIsCredential(t *testing.T) {
	assert.True(t, ResourceSigningKeys{Previous: ResourceSigningLegacyKey}.PreviousIsCredential())
	assert.True(t, ResourceSigningKeys{Previous: ResourceSigningDerivedKeyPrefix + "abc"}.PreviousIsCredential())
	assert.False(t, ResourceSigningKeys{Previous: "a-secret"}.PreviousIsCredential())
	assert.False(t, ResourceSigningKeys{}.PreviousIsCredential())
	assert.ErrorIs(t, RotateLLMResourceSigningSecret(nil, 1, -time.Second, time.Now()), ErrResourceSigningGrace)
}
