package local

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArgon2id_Deterministic(t *testing.T) {
	t.Parallel()
	p1 := New("same-key")
	p2 := New("same-key")
	// Both should derive the same KEK
	assert.Equal(t, p1.kek, p2.kek, "Argon2id derivation must be deterministic")
}

func TestArgon2id_DifferentKeys(t *testing.T) {
	t.Parallel()
	p1 := New("key-alpha")
	p2 := New("key-beta")
	assert.NotEqual(t, p1.kek, p2.kek, "different raw keys should produce different KEKs")
}

func TestArgon2id_EmptyRawKey(t *testing.T) {
	t.Parallel()
	require.NotPanics(t, func() {
		p := New("")
		assert.Len(t, p.kek, 32, "should produce a valid 32-byte key even with empty input")
	})
}

func TestArgon2id_VeryLongRawKey(t *testing.T) {
	t.Parallel()
	longKey := strings.Repeat("a", 2000)
	require.NotPanics(t, func() {
		p := New(longKey)
		assert.Len(t, p.kek, 32)
	})
}

func TestArgon2id_UnicodeRawKey(t *testing.T) {
	t.Parallel()
	require.NotPanics(t, func() {
		p := New("unicode-key-日本語テスト")
		assert.Len(t, p.kek, 32)
	})

	// And it should be deterministic
	p1 := New("unicode-key-日本語テスト")
	p2 := New("unicode-key-日本語テスト")
	assert.Equal(t, p1.kek, p2.kek)
}

func TestArgon2id_KeyLength32(t *testing.T) {
	t.Parallel()
	// Verify the constant matches actual output
	p := New("any-key")
	assert.Equal(t, argon2KeyLen, len(p.kek))
}
