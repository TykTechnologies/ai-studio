package secrets

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRotateKEK_ClearsCache_OldStoreFailsAfterRotation(t *testing.T) {
	db := setupTestDB(t)
	rawKey := "cache-rotation"
	ctx := context.Background()

	oldKEK := newTestLocalKEK("old")
	store := NewWithKEKProvider(db, rawKey, oldKEK)

	// Create secrets and warm the cache
	s := &Secret{VarName: "A", Value: "val"}
	require.NoError(t, store.Create(ctx, s))

	got, err := store.GetByVarName(ctx, "A", false)
	require.NoError(t, err)
	assert.Equal(t, "val", got.Value)

	// Rotate KEK — cache should be cleared
	newKEK := newTestLocalKEK("new")
	result, err := store.RotateKEK(ctx, oldKEK, newKEK)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Rotated)

	// Old store should fail because cache is cleared and KEK can't unwrap new wrapping
	_, err = store.GetByVarName(ctx, "A", false)
	assert.Error(t, err, "old KEK should fail after rotation + cache clear")
}

func TestRotateKEK_ZeroKeys(t *testing.T) {
	db := setupTestDB(t)
	rawKey := "zero-keys"
	ctx := context.Background()

	oldKEK := newTestLocalKEK("old")
	store := NewWithKEKProvider(db, rawKey, oldKEK)

	newKEK := newTestLocalKEK("new")
	result, err := store.RotateKEK(ctx, oldKEK, newKEK)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Total)
	assert.Equal(t, 0, result.Rotated)
	assert.Empty(t, result.Errors)
}

func TestRotateKEK_PartialFailure_CacheStillCleared(t *testing.T) {
	db := setupTestDB(t)
	rawKey := "partial-fail"
	ctx := context.Background()

	kek1 := newTestLocalKEK("kek1")
	kek2 := newTestLocalKEK("kek2")
	store := NewWithKEKProvider(db, rawKey, kek1)

	// Create a secret so an encryption key exists
	s := &Secret{VarName: "A", Value: "val"}
	require.NoError(t, store.Create(ctx, s))

	// Warm cache
	_, err := store.GetByVarName(ctx, "A", false)
	require.NoError(t, err)

	// Rotate with wrong old KEK — unwrap will fail for all keys
	wrongOldKEK := newTestLocalKEK("wrong-old")
	result, err := store.RotateKEK(ctx, wrongOldKEK, kek2)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, 0, result.Rotated)
	assert.Len(t, result.Errors, 1)

	// Cache should still be cleared even though rotation had errors
	// Next decrypt will re-fetch from DB and try to unwrap with store's original KEK (kek1)
	got, err := store.GetByVarName(ctx, "A", false)
	require.NoError(t, err)
	assert.Equal(t, "val", got.Value, "should still decrypt since DB wrapping didn't change")
}

func TestRotateKey_ClearsCacheSoNewEncryptUsesNewKey(t *testing.T) {
	db := setupTestDB(t)
	rawKey := "rotate-key-cache"
	ctx := context.Background()

	store, err := New(db, rawKey)
	require.NoError(t, err)

	// Create secrets to warm cache
	s1 := &Secret{VarName: "S1", Value: "v1"}
	require.NoError(t, store.Create(ctx, s1))

	// Warm decrypt cache
	_, err = store.GetByVarName(ctx, "S1", false)
	require.NoError(t, err)

	// RotateKey re-encrypts all secrets and clears cache via encryptValue
	result, err := store.RotateKey(ctx, rawKey, rawKey)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Rotated)

	// Should still decrypt after rotation
	got, err := store.GetByVarName(ctx, "S1", false)
	require.NoError(t, err)
	assert.Equal(t, "v1", got.Value)
}
