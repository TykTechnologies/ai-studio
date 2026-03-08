package local

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvider_RoundTrip(t *testing.T) {
	p := New("test-kek")
	ctx := context.Background()

	dek := make([]byte, 32)
	for i := range dek {
		dek[i] = byte(i)
	}

	wrapped, err := p.WrapKey(ctx, dek)
	require.NoError(t, err)
	assert.NotEqual(t, dek, wrapped)

	unwrapped, err := p.UnwrapKey(ctx, wrapped)
	require.NoError(t, err)
	assert.Equal(t, dek, unwrapped)
}

func TestGenerateDEK(t *testing.T) {
	p := New("test-kek")
	ctx := context.Background()

	wrapped, err := secrets.GenerateDEK(ctx, p)
	require.NoError(t, err)
	assert.NotEmpty(t, wrapped)

	dek, err := p.UnwrapKey(ctx, wrapped)
	require.NoError(t, err)
	assert.Len(t, dek, 32)
}

func TestProvider_WrongKEK(t *testing.T) {
	p1 := New("kek-1")
	p2 := New("kek-2")
	ctx := context.Background()

	dek := []byte("this-is-a-32-byte-data-enc-key!")
	wrapped, err := p1.WrapKey(ctx, dek)
	require.NoError(t, err)

	_, err = p2.UnwrapKey(ctx, wrapped)
	assert.Error(t, err)
}

func TestProvider_TooShort(t *testing.T) {
	p := New("kek")
	ctx := context.Background()

	_, err := p.UnwrapKey(ctx, []byte("short"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too short")
}
