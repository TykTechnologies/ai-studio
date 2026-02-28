package secrets

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyEncryptDecryptRoundTrip(t *testing.T) {
	cfb := AllCipherInstances()["v1"]
	ciphers := AllCipherInstances()
	ctx := context.Background()

	tests := []struct {
		name        string
		key         string
		value       string
		expectNoOp  bool // true when empty key means no encryption
	}{
		{"simple string key", "my-secret-key", "test-value", false},
		{"empty key", "", "test-value", true},
		{"long key", "this-is-a-very-long-key-that-should-still-work-fine", "test-value", false},
		{"empty value", "my-secret-key", "", false},
		{"long value", "my-secret-key", "this is a very long value that should still encrypt and decrypt properly without any issues", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encrypted, err := EncryptWith(ctx, cfb, tt.key, tt.value)
			require.NoError(t, err)

			if tt.expectNoOp {
				// Empty key: passthrough, no encryption
				assert.Equal(t, tt.value, encrypted)
				return
			}

			if tt.value != "" {
				assert.NotEqual(t, tt.value, encrypted)
			}

			decrypted, err := DecryptWith(ctx, ciphers, tt.key, encrypted)
			require.NoError(t, err)
			assert.Equal(t, tt.value, decrypted)

			if tt.value != "" {
				differentKey := tt.key + "-different"
				wrongDecrypted, err := DecryptWith(ctx, ciphers, differentKey, encrypted)
				require.NoError(t, err)
				assert.NotEqual(t, tt.value, wrongDecrypted)
			}
		})
	}
}

func TestGCMRoundTrip(t *testing.T) {
	gcm := AllCipherInstances()["v2"]
	ciphers := AllCipherInstances()
	ctx := context.Background()

	encrypted, err := EncryptWith(ctx, gcm, "my-key", "secret-data")
	require.NoError(t, err)
	assert.Contains(t, encrypted, "$ENC/v2/")

	decrypted, err := DecryptWith(ctx, ciphers, "my-key", encrypted)
	require.NoError(t, err)
	assert.Equal(t, "secret-data", decrypted)

	_, err = DecryptWith(ctx, ciphers, "wrong-key", encrypted)
	assert.Error(t, err)
}

func TestSecretPreserveReference(t *testing.T) {
	s := &Secret{VarName: "MY_KEY", Value: "decrypted-value"}

	assert.Equal(t, "decrypted-value", s.GetValue())

	s.PreserveReference()
	assert.Equal(t, "$SECRET/MY_KEY", s.GetValue())
}
