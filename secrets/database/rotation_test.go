package database

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/secrets"
)

func TestRotateKey_SameKey(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for _, name := range []string{"KEY_A", "KEY_B", "KEY_C"} {
		s := &secrets.Secret{VarName: name, Value: "value-" + name}
		require.NoError(t, store.Create(ctx, s))
	}

	result, err := store.RotateKey(ctx, "test-secret-key", "test-secret-key")
	require.NoError(t, err)
	assert.Equal(t, 3, result.Total)
	assert.Equal(t, 3, result.Rotated)
	assert.Empty(t, result.Errors)

	for _, name := range []string{"KEY_A", "KEY_B", "KEY_C"} {
		got, err := store.GetByVarName(ctx, name, false)
		require.NoError(t, err)
		assert.Equal(t, "value-"+name, got.Value)
	}
}

func TestRotateKey_NewKey(t *testing.T) {
	db := setupTestDB(t)
	oldKey := "old-key"
	newKey := "new-key"
	ctx := context.Background()

	oldStore := New(db, oldKey)

	s := &secrets.Secret{VarName: "ROTATE_ME", Value: "secret-data"}
	require.NoError(t, oldStore.Create(ctx, s))

	result, err := oldStore.RotateKey(ctx, oldKey, newKey)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, 1, result.Rotated)
	assert.Empty(t, result.Errors)

	newStore := New(db, newKey)
	got, err := newStore.GetByVarName(ctx, "ROTATE_ME", false)
	require.NoError(t, err)
	assert.Equal(t, "secret-data", got.Value)

	// Old store should NOT be able to decrypt anymore (GCM will reject)
	_, err = oldStore.GetByVarName(ctx, "ROTATE_ME", false)
	assert.Error(t, err)
}

func TestRotateKey_V1ToV2(t *testing.T) {
	db := setupTestDB(t)
	key := "migration-key"
	ctx := context.Background()

	// Manually insert a v1 (CFB) encrypted secret
	cfb := &secrets.Secret{VarName: "LEGACY", Value: "legacy-value"}
	v1Cipher := secrets.AllCipherInstances()["v1"]
	encrypted, err := secrets.EncryptWith(ctx, v1Cipher, key, cfb.Value)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(encrypted, "$ENC/"))
	assert.False(t, strings.HasPrefix(encrypted, "$ENC/v2/"))
	cfb.Value = encrypted
	require.NoError(t, db.Create(cfb).Error)

	store := New(db, key)

	result, err := store.RotateKey(ctx, key, key)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, 1, result.Rotated)
	assert.Equal(t, "v2", result.NewCipher)

	// Verify the DB now has v2 format
	var raw secrets.Secret
	require.NoError(t, db.First(&raw, cfb.ID).Error)
	assert.True(t, strings.HasPrefix(raw.Value, "$ENC/v2/"))

	got, err := store.GetByVarName(ctx, "LEGACY", false)
	require.NoError(t, err)
	assert.Equal(t, "legacy-value", got.Value)
}

func TestRotateKey_EmptyDB(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	result, err := store.RotateKey(ctx, "old", "new")
	require.NoError(t, err)
	assert.Equal(t, 0, result.Total)
	assert.Equal(t, 0, result.Rotated)
	assert.Empty(t, result.Errors)
}

func TestRotateKey_EmptyValues(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	s := &secrets.Secret{VarName: "EMPTY", Value: ""}
	require.NoError(t, store.Create(ctx, s))

	result, err := store.RotateKey(ctx, "test-secret-key", "test-secret-key")
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, 1, result.Rotated)
}
