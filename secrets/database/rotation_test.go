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

func TestRotateKey_V1ToV2Envelope(t *testing.T) {
	db := setupTestDB(t)
	key := "migration-key"
	ctx := context.Background()

	// Create a v1 encrypted secret
	v1Store := New(db, key)
	s := &secrets.Secret{VarName: "LEGACY", Value: "legacy-value"}
	require.NoError(t, v1Store.Create(ctx, s))
	assert.True(t, strings.HasPrefix(s.Value, "$ENC/"))
	assert.False(t, strings.HasPrefix(s.Value, "$ENC/v2/"))

	// Rotate with envelope store → migrates to v2
	wrapper := secrets.NewLocalKeyWrapper(key)
	v2Store := NewWithEnvelope(db, key, wrapper)

	result, err := v2Store.RotateKey(ctx, key, key)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, 1, result.Rotated)
	assert.Equal(t, "v2", result.NewCipher)

	// Verify DB has v2 format
	var raw secrets.Secret
	require.NoError(t, db.First(&raw, s.ID).Error)
	assert.True(t, strings.HasPrefix(raw.Value, "$ENC/v2/"))

	// Should decrypt
	got, err := v2Store.GetByVarName(ctx, "LEGACY", false)
	require.NoError(t, err)
	assert.Equal(t, "legacy-value", got.Value)
}

func TestRotateKEK_ReWrapsKeys(t *testing.T) {
	db := setupTestDB(t)
	rawKey := "kek-test"
	ctx := context.Background()

	oldWrapper := secrets.NewLocalKeyWrapper("old-kek")
	store := NewWithEnvelope(db, rawKey, oldWrapper)

	// Create some secrets
	for _, name := range []string{"S1", "S2"} {
		s := &secrets.Secret{VarName: name, Value: "val-" + name}
		require.NoError(t, store.Create(ctx, s))
	}

	// Verify we have 1 encryption key
	var keyCount int64
	db.Model(&secrets.EncryptionKey{}).Count(&keyCount)
	assert.Equal(t, int64(1), keyCount)

	// Rotate KEK
	newWrapper := secrets.NewLocalKeyWrapper("new-kek")
	result, err := store.RotateKEK(ctx, oldWrapper, newWrapper)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, 1, result.Rotated)
	assert.Empty(t, result.Errors)

	// New store can decrypt
	newStore := NewWithEnvelope(db, rawKey, newWrapper)
	for _, name := range []string{"S1", "S2"} {
		got, err := newStore.GetByVarName(ctx, name, false)
		require.NoError(t, err)
		assert.Equal(t, "val-"+name, got.Value)
	}
}
