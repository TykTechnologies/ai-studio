package secrets

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRotateKey_SameKey(t *testing.T) {
	store := newTestStoreDefault(t)
	ctx := context.Background()

	for _, name := range []string{"KEY_A", "KEY_B", "KEY_C"} {
		s := &Secret{VarName: name, Value: "value-" + name}
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

func TestRotateKey_EmptyDB(t *testing.T) {
	store := newTestStoreDefault(t)
	ctx := context.Background()

	result, err := store.RotateKey(ctx, "old", "new")
	require.NoError(t, err)
	assert.Equal(t, 0, result.Total)
	assert.Equal(t, 0, result.Rotated)
	assert.Empty(t, result.Errors)
}

func TestRotateKey_EmptyValues(t *testing.T) {
	store := newTestStoreDefault(t)
	ctx := context.Background()

	s := &Secret{VarName: "EMPTY", Value: ""}
	require.NoError(t, store.Create(ctx, s))

	result, err := store.RotateKey(ctx, "test-secret-key", "test-secret-key")
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, 1, result.Rotated)
}

func TestRotateKey_V1ToV2(t *testing.T) {
	db := setupTestDB(t)
	key := "migration-key"
	ctx := context.Background()

	// Insert a v1 encrypted secret directly
	insertV1Secret(t, db, key, "LEGACY", "legacy-value")

	// Rotate with current store -> migrates to v2
	store, err := New(db, key)
	require.NoError(t, err)
	result, err := store.RotateKey(ctx, key, key)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, 1, result.Rotated)
	assert.Equal(t, "v2", result.NewCipher)

	// Verify DB now has v2 format
	var raw Secret
	require.NoError(t, db.Where("var_name = ?", "LEGACY").First(&raw).Error)
	assert.True(t, strings.HasPrefix(raw.Value, "$ENC/v2/"))

	// Should decrypt
	got, err := store.GetByVarName(ctx, "LEGACY", false)
	require.NoError(t, err)
	assert.Equal(t, "legacy-value", got.Value)
}

func TestRotateKey_MixedV1AndV2(t *testing.T) {
	db := setupTestDB(t)
	key := "mixed-key"
	ctx := context.Background()

	// Insert a v1 secret directly
	insertV1Secret(t, db, key, "V1_SECRET", "v1-data")

	// Create a v2 secret via the store
	store, err := New(db, key)
	require.NoError(t, err)
	s := &Secret{VarName: "V2_SECRET", Value: "v2-data"}
	require.NoError(t, store.Create(ctx, s))

	// Rotate -- both should end up as v2
	result, err := store.RotateKey(ctx, key, key)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Total)
	assert.Equal(t, 2, result.Rotated)

	for _, name := range []string{"V1_SECRET", "V2_SECRET"} {
		var raw Secret
		require.NoError(t, db.Where("var_name = ?", name).First(&raw).Error)
		assert.True(t, strings.HasPrefix(raw.Value, "$ENC/v2/"), "%s should be v2", name)
	}
}
