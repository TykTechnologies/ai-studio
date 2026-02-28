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
	require.NoError(t, db.AutoMigrate(&secrets.Secret{}))
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

	// Value should be encrypted in DB
	assert.True(t, strings.HasPrefix(secret.Value, "$ENC/"))

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
