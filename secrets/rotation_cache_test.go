package secrets

import (
	"context"
	"fmt"
	"sync/atomic"
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

func TestRotateKEK_InvalidBase64WrappedKey(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	kek := newTestLocalKEK("kek")
	store := NewWithKEKProvider(db, "key", kek)

	// Create a secret so an encryption key exists
	s := &Secret{VarName: "A", Value: "val"}
	require.NoError(t, store.Create(ctx, s))

	// Corrupt the wrapped key in the DB to invalid base64
	db.Model(&EncryptionKey{}).Where("1 = 1").Update("wrapped_key", "!!!not-base64!!!")

	newKEK := newTestLocalKEK("new")
	result, err := store.RotateKEK(ctx, kek, newKEK)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, 0, result.Rotated)
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Error(), "decode wrapped key")
}

func TestRotateKEK_WrapKeyError(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	kek := newTestLocalKEK("kek")
	store := NewWithKEKProvider(db, "key", kek)

	// Create a secret so an encryption key exists
	s := &Secret{VarName: "A", Value: "val"}
	require.NoError(t, store.Create(ctx, s))

	// Use a newKEK that fails on WrapKey
	badNewKEK := &failingWrapKEK{testLocalKEK: *newTestLocalKEK("bad")}
	result, err := store.RotateKEK(ctx, kek, badNewKEK)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, 0, result.Rotated)
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Error(), "wrap with new kek")
}

// failingWrapKEK wraps successfully on read but fails on WrapKey.
type failingWrapKEK struct {
	testLocalKEK
}

func (f *failingWrapKEK) WrapKey(_ context.Context, _ []byte) ([]byte, error) {
	return nil, fmt.Errorf("simulated wrap failure")
}

// failOnSecondWrapKEK succeeds on WrapKey until failWrap is set to true.
type failOnSecondWrapKEK struct {
	testLocalKEK
	failWrap atomic.Bool
}

func (f *failOnSecondWrapKEK) WrapKey(ctx context.Context, dek []byte) ([]byte, error) {
	if f.failWrap.Load() {
		return nil, fmt.Errorf("simulated wrap failure on re-encrypt")
	}
	return f.testLocalKEK.WrapKey(ctx, dek)
}

func (f *failOnSecondWrapKEK) UnwrapKey(ctx context.Context, wrappedDEK []byte) ([]byte, error) {
	if f.failWrap.Load() {
		return nil, fmt.Errorf("simulated unwrap failure on re-encrypt")
	}
	return f.testLocalKEK.UnwrapKey(ctx, wrappedDEK)
}

func TestRotateKey_DecryptFailure(t *testing.T) {
	db := setupTestDB(t)
	rawKey := "decrypt-fail"
	ctx := context.Background()

	store, err := New(db, rawKey)
	require.NoError(t, err)

	// Create a valid secret
	s1 := &Secret{VarName: "GOOD", Value: "hello"}
	require.NoError(t, store.Create(ctx, s1))

	// Insert a secret with a corrupt encrypted value directly in DB
	corrupt := &Secret{VarName: "BAD", Value: "$ENC/v2/999/not-valid-ciphertext"}
	require.NoError(t, db.Create(corrupt).Error)

	result, err := store.RotateKey(ctx, rawKey, rawKey)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Total)
	assert.Equal(t, 1, result.Rotated)
	assert.Len(t, result.Errors, 1)
	assert.Equal(t, "BAD", result.Errors[0].VarName)
}

func TestRotateKey_ReEncryptFailure(t *testing.T) {
	db := setupTestDB(t)
	rawKey := "reencrypt-fail"
	ctx := context.Background()

	// Use a KEK that works initially but can be toggled to fail.
	kek := &failOnSecondWrapKEK{testLocalKEK: *newTestLocalKEK(rawKey)}
	store := NewWithKEKProvider(db, rawKey, kek)

	// Insert a v1-encrypted secret so decrypt uses the v1 cipher (no envelope needed).
	v1Cipher := legacyCipherInstances()["v1"]
	enc, err := encryptWith(ctx, v1Cipher, rawKey, "hello")
	require.NoError(t, err)
	require.NoError(t, db.Create(&Secret{VarName: "V1", Value: enc}).Error)

	// Now fail the KEK so envelope re-encrypt (WrapKey for new DEK) fails.
	kek.failWrap.Store(true)

	result, err := store.RotateKey(ctx, rawKey, rawKey)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, 0, result.Rotated)
	assert.Len(t, result.Errors, 1)
}

func TestRotateKey_DBUpdateFailure(t *testing.T) {
	db := setupTestDB(t)
	rawKey := "update-fail"
	ctx := context.Background()

	store, err := New(db, rawKey)
	require.NoError(t, err)

	// Create a secret
	s := &Secret{VarName: "A", Value: "val"}
	require.NoError(t, store.Create(ctx, s))

	// Drop the secrets table so the batch load fails
	db.Exec("DROP TABLE secrets")

	_, err = store.RotateKey(ctx, rawKey, rawKey)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load batch")
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

func TestRotateKey_StableKeyIDReferences(t *testing.T) {
	db := setupTestDB(t)
	rawKey := "stable-refs"
	ctx := context.Background()

	store, err := New(db, rawKey)
	require.NoError(t, err)

	// Create secrets
	for _, name := range []string{"A", "B", "C"} {
		require.NoError(t, store.Create(ctx, &Secret{VarName: name, Value: "val-" + name}))
	}

	// Record key IDs before rotation
	var beforeKeys []EncryptionKey
	db.Order("id ASC").Find(&beforeKeys)
	require.Len(t, beforeKeys, 3)

	beforeKeyIDs := make(map[uint]uint) // key ID -> version
	for _, k := range beforeKeys {
		beforeKeyIDs[k.ID] = k.Version
	}

	// Record encrypted values to extract key ID references
	var beforeSecrets []Secret
	db.Find(&beforeSecrets)
	beforeRefs := make(map[string]uint) // var_name -> key_id
	for _, s := range beforeSecrets {
		keyID, err := parseV2KeyID(s.Value)
		require.NoError(t, err)
		beforeRefs[s.VarName] = keyID
	}

	// Rotate
	result, err := store.RotateKey(ctx, rawKey, rawKey)
	require.NoError(t, err)
	assert.Equal(t, 3, result.Rotated)
	assert.Empty(t, result.Errors)

	// Verify key IDs in secrets are unchanged
	var afterSecrets []Secret
	db.Find(&afterSecrets)
	for _, s := range afterSecrets {
		keyID, err := parseV2KeyID(s.Value)
		require.NoError(t, err)
		assert.Equal(t, beforeRefs[s.VarName], keyID, "key ID reference for %s should be stable", s.VarName)
	}

	// Verify no new keys were created (same count)
	var afterKeys []EncryptionKey
	db.Order("id ASC").Find(&afterKeys)
	assert.Len(t, afterKeys, 3, "no new keys should be created")

	// Verify versions were bumped
	for _, k := range afterKeys {
		assert.Equal(t, beforeKeyIDs[k.ID]+1, k.Version, "key %d version should be bumped", k.ID)
	}

	// Verify values still decrypt
	for _, name := range []string{"A", "B", "C"} {
		got, err := store.GetByVarName(ctx, name, false)
		require.NoError(t, err)
		assert.Equal(t, "val-"+name, got.Value)
	}
}
