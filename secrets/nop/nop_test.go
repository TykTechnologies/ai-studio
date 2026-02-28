package nop

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/secrets"
)

func TestNop_CRUD(t *testing.T) {
	store := New()
	ctx := context.Background()

	// Create
	s := &secrets.Secret{VarName: "KEY", Value: "val"}
	require.NoError(t, store.Create(ctx, s))
	assert.Equal(t, uint(1), s.ID)

	// GetByID
	got, err := store.GetByID(ctx, 1, false)
	require.NoError(t, err)
	assert.Equal(t, "val", got.Value)

	// GetByVarName
	got, err = store.GetByVarName(ctx, "KEY", false)
	require.NoError(t, err)
	assert.Equal(t, "val", got.Value)

	// Update
	s.Value = "new-val"
	require.NoError(t, store.Update(ctx, s))
	got, _ = store.GetByID(ctx, 1, false)
	assert.Equal(t, "new-val", got.Value)

	// List
	items, total, _, err := store.List(ctx, 10, 1, true)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, items, 1)

	// Delete
	require.NoError(t, store.Delete(ctx, 1))
	_, err = store.GetByID(ctx, 1, false)
	assert.Error(t, err)
}

func TestNop_Passthrough(t *testing.T) {
	store := New()
	ctx := context.Background()

	encrypted, err := store.EncryptValue(ctx, "hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", encrypted)

	decrypted, err := store.DecryptValue(ctx, "$ENC/v2/something")
	require.NoError(t, err)
	assert.Equal(t, "$ENC/v2/something", decrypted)

	ref := store.ResolveReference(ctx, "$SECRET/KEY", false)
	assert.Equal(t, "$SECRET/KEY", ref)
}

func TestNop_Registry(t *testing.T) {
	store, err := secrets.NewStore("nop", nil, "")
	require.NoError(t, err)
	assert.NotNil(t, store)
}

func TestNop_RotateKey(t *testing.T) {
	store := New()
	result, err := store.RotateKey(context.Background(), "old", "new")
	require.NoError(t, err)
	assert.Equal(t, 0, result.Total)
}
