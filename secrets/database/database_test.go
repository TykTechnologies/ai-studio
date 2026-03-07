package database

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/secrets"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&secrets.Secret{}, &secrets.EncryptionKey{}))
	return db
}

func newTestStore(t *testing.T) *Database {
	t.Helper()
	db := setupTestDB(t)
	return New(db, "test-secret-key")
}

func TestDatabase_CreateAndGetByID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	secret := &secrets.Secret{VarName: "MY_KEY", Value: "my-secret-value"}
	require.NoError(t, store.Create(ctx, secret))

	// Value should be v2 envelope encrypted
	assert.True(t, strings.HasPrefix(secret.Value, "$ENC/v2/"))

	// GetByID with preserveRef=false decrypts
	got, err := store.GetByID(ctx, secret.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "my-secret-value", got.Value)

	// GetByID with preserveRef=true returns reference format
	got, err = store.GetByID(ctx, secret.ID, true)
	require.NoError(t, err)
	assert.Equal(t, "$SECRET/MY_KEY", got.GetValue())
}

func TestDatabase_GetByVarName(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	secret := &secrets.Secret{VarName: "API_TOKEN", Value: "token-123"}
	require.NoError(t, store.Create(ctx, secret))

	got, err := store.GetByVarName(ctx, "API_TOKEN", false)
	require.NoError(t, err)
	assert.Equal(t, "token-123", got.Value)
}

func TestDatabase_Update(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	secret := &secrets.Secret{VarName: "KEY", Value: "old-value"}
	require.NoError(t, store.Create(ctx, secret))

	secret.Value = "new-value"
	require.NoError(t, store.Update(ctx, secret))

	got, err := store.GetByID(ctx, secret.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "new-value", got.Value)
}

func TestDatabase_Delete(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	secret := &secrets.Secret{VarName: "DEL_ME", Value: "bye"}
	require.NoError(t, store.Create(ctx, secret))

	require.NoError(t, store.Delete(ctx, secret.ID))

	_, err := store.GetByID(ctx, secret.ID, false)
	assert.Error(t, err)
}

func TestDatabase_List(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		s := &secrets.Secret{VarName: "KEY_" + string(rune('A'+i)), Value: "val"}
		require.NoError(t, store.Create(ctx, s))
	}

	// All
	items, total, pages, err := store.List(ctx, 10, 1, true)
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Equal(t, 1, pages)
	assert.Len(t, items, 5)

	// Paginated
	items, total, pages, err = store.List(ctx, 2, 1, false)
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Equal(t, 3, pages)
	assert.Len(t, items, 2)
}

func TestDatabase_EnsureDefaults(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, store.EnsureDefaults(ctx, []string{"OPENAI_KEY", "ANTHROPIC_KEY"}))

	_, err := store.GetByVarName(ctx, "OPENAI_KEY", false)
	require.NoError(t, err)

	// Calling again should not duplicate
	require.NoError(t, store.EnsureDefaults(ctx, []string{"OPENAI_KEY", "ANTHROPIC_KEY"}))

	items, total, _, err := store.List(ctx, 100, 1, true)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, items, 2)
}

func TestDatabase_EncryptDecryptValue(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	encrypted, err := store.EncryptValue(ctx, "hello")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(encrypted, "$ENC/v2/"))

	decrypted, err := store.DecryptValue(ctx, encrypted)
	require.NoError(t, err)
	assert.Equal(t, "hello", decrypted)
}

func TestDatabase_ResolveReference(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	secret := &secrets.Secret{VarName: "MY_SECRET", Value: "resolved-value"}
	require.NoError(t, store.Create(ctx, secret))

	val := store.ResolveReference(ctx, "$SECRET/MY_SECRET", false)
	assert.Equal(t, "resolved-value", val)

	val = store.ResolveReference(ctx, "$SECRET/MY_SECRET", true)
	assert.Equal(t, "$SECRET/MY_SECRET", val)

	val = store.ResolveReference(ctx, "plain-value", false)
	assert.Equal(t, "plain-value", val)

	t.Setenv("TEST_ENV_VAR", "env-value")
	val = store.ResolveReference(ctx, "$ENV/TEST_ENV_VAR", false)
	assert.Equal(t, "env-value", val)
}

func TestRegistryRegistration(t *testing.T) {
	db := setupTestDB(t)

	store, err := secrets.NewStore("database", db, "test-key")
	require.NoError(t, err)
	assert.NotNil(t, store)

	ctx := context.Background()
	s := &secrets.Secret{VarName: "REG_TEST", Value: "value"}
	require.NoError(t, store.Create(ctx, s))

	got, err := store.GetByVarName(ctx, "REG_TEST", false)
	require.NoError(t, err)
	assert.Equal(t, "value", got.Value)
}

func TestRegistryUnknownStore(t *testing.T) {
	_, err := secrets.NewStore("nonexistent", nil, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown store type")
}

// --- Backward compatibility: v1 (legacy CFB) secrets ---

// insertV1Secret encrypts a value with legacy v1 CFB cipher and inserts it
// directly into the database, simulating data written by older versions.
func insertV1Secret(t *testing.T, db *gorm.DB, rawKey, varName, plaintext string) *secrets.Secret {
	t.Helper()
	ctx := context.Background()
	v1 := secrets.LegacyCipherInstances()["v1"]
	encrypted, err := secrets.EncryptWith(ctx, v1, rawKey, plaintext)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(encrypted, "$ENC/"))
	require.False(t, strings.HasPrefix(encrypted, "$ENC/v2/"))

	secret := &secrets.Secret{VarName: varName, Value: encrypted}
	require.NoError(t, db.Create(secret).Error)
	return secret
}

func TestBackwardCompat_ReadV1Secret(t *testing.T) {
	db := setupTestDB(t)
	rawKey := "compat-key"
	ctx := context.Background()

	// Insert a v1 encrypted secret directly
	secret := insertV1Secret(t, db, rawKey, "LEGACY_V1", "v1-data")

	// Current store should decrypt it transparently
	store := New(db, rawKey)
	got, err := store.GetByID(ctx, secret.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "v1-data", got.Value)
}

func TestBackwardCompat_ReadV1ByVarName(t *testing.T) {
	db := setupTestDB(t)
	rawKey := "compat-key"
	ctx := context.Background()

	insertV1Secret(t, db, rawKey, "LEGACY_TOKEN", "sk-legacy-123")

	store := New(db, rawKey)
	got, err := store.GetByVarName(ctx, "LEGACY_TOKEN", false)
	require.NoError(t, err)
	assert.Equal(t, "sk-legacy-123", got.Value)
}

func TestBackwardCompat_UpdateV1SecretRewritesAsV2(t *testing.T) {
	db := setupTestDB(t)
	rawKey := "compat-key"
	ctx := context.Background()

	secret := insertV1Secret(t, db, rawKey, "UPGRADE_ME", "old-v1-value")

	store := New(db, rawKey)

	// Read, modify, and update
	got, err := store.GetByID(ctx, secret.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "old-v1-value", got.Value)

	got.Value = "new-value"
	require.NoError(t, store.Update(ctx, got))

	// Verify DB now has v2 format
	var raw secrets.Secret
	require.NoError(t, db.First(&raw, secret.ID).Error)
	assert.True(t, strings.HasPrefix(raw.Value, "$ENC/v2/"), "updated secret should use v2 envelope format")

	// Value should decrypt correctly
	got2, err := store.GetByID(ctx, secret.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "new-value", got2.Value)
}

func TestBackwardCompat_DecryptValueV1(t *testing.T) {
	db := setupTestDB(t)
	rawKey := "compat-key"
	ctx := context.Background()

	// Encrypt with v1 manually
	v1 := secrets.LegacyCipherInstances()["v1"]
	encrypted, err := secrets.EncryptWith(ctx, v1, rawKey, "direct-v1")
	require.NoError(t, err)

	// DecryptValue should handle it
	store := New(db, rawKey)
	decrypted, err := store.DecryptValue(ctx, encrypted)
	require.NoError(t, err)
	assert.Equal(t, "direct-v1", decrypted)
}

// --- Envelope encryption (v2) tests ---

func TestEnvelope_CreateAndGetByID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	secret := &secrets.Secret{VarName: "ENV_KEY", Value: "envelope-secret"}
	require.NoError(t, store.Create(ctx, secret))

	assert.True(t, strings.HasPrefix(secret.Value, "$ENC/v2/"))

	got, err := store.GetByID(ctx, secret.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "envelope-secret", got.Value)
}

func TestEnvelope_EncryptDecryptValue(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	encrypted, err := store.EncryptValue(ctx, "hello envelope")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(encrypted, "$ENC/v2/"))

	decrypted, err := store.DecryptValue(ctx, encrypted)
	require.NoError(t, err)
	assert.Equal(t, "hello envelope", decrypted)
}

func TestEnvelope_RotateKEK(t *testing.T) {
	db := setupTestDB(t)
	rawKey := "kek-rotation-key"
	ctx := context.Background()

	oldWrapper := secrets.NewLocalKeyWrapper("old-kek")
	oldStore := NewWithEnvelope(db, rawKey, oldWrapper)

	for _, name := range []string{"A", "B", "C"} {
		s := &secrets.Secret{VarName: name, Value: "val-" + name}
		require.NoError(t, oldStore.Create(ctx, s))
	}

	// Rotate KEK (re-wraps encryption_keys rows, not secrets)
	newWrapper := secrets.NewLocalKeyWrapper("new-kek")
	result, err := oldStore.RotateKEK(ctx, oldWrapper, newWrapper)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)   // 1 encryption key
	assert.Equal(t, 1, result.Rotated)
	assert.Empty(t, result.Errors)

	// New store with new KEK should decrypt all secrets
	newStore := NewWithEnvelope(db, rawKey, newWrapper)
	for _, name := range []string{"A", "B", "C"} {
		got, err := newStore.GetByVarName(ctx, name, false)
		require.NoError(t, err)
		assert.Equal(t, "val-"+name, got.Value)
	}

	// Old store should NOT decrypt (wrong KEK)
	_, err = oldStore.GetByVarName(ctx, "A", false)
	assert.Error(t, err)
}

func TestEnvelope_EncryptionKeyAutoCreated(t *testing.T) {
	db := setupTestDB(t)
	store := New(db, "auto-key")
	ctx := context.Background()

	var count int64
	db.Model(&secrets.EncryptionKey{}).Count(&count)
	assert.Equal(t, int64(0), count)

	s := &secrets.Secret{VarName: "AUTO", Value: "auto-val"}
	require.NoError(t, store.Create(ctx, s))

	db.Model(&secrets.EncryptionKey{}).Count(&count)
	assert.Equal(t, int64(1), count)

	var key secrets.EncryptionKey
	db.First(&key)
	assert.Equal(t, secrets.EncryptionKeyActive, key.Status)
}

func TestEnvelope_NewAlwaysWritesV2(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// All new secrets should be v2
	for _, name := range []string{"A", "B", "C"} {
		s := &secrets.Secret{VarName: name, Value: "val-" + name}
		require.NoError(t, store.Create(ctx, s))
		assert.True(t, strings.HasPrefix(s.Value, "$ENC/v2/"),
			"New() should always write v2 envelope format, got: %s", s.Value)
	}
}
