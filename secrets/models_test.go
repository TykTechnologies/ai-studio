package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeriveKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLen  int
		wantDiff bool // should different inputs produce different keys
	}{
		{
			name:     "simple string",
			input:    "my-secret-key",
			wantLen:  32,
			wantDiff: false,
		},
		{
			name:     "empty string",
			input:    "",
			wantLen:  32,
			wantDiff: true,
		},
		{
			name:     "long string",
			input:    "this-is-a-very-long-string-that-is-longer-than-32-bytes-but-should-still-work",
			wantLen:  32,
			wantDiff: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := deriveKey(tt.input)
			assert.Equal(t, tt.wantLen, len(key), "key length should be 32 bytes")

			if tt.wantDiff {
				// Test that different inputs produce different keys
				otherKey := deriveKey(tt.input + "-different")
				assert.NotEqual(t, key, otherKey, "different inputs should produce different keys")
			}
		})
	}
}

func TestEncryptDecrypt(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantErr bool
	}{
		{
			name:    "simple string key",
			key:     "my-secret-key",
			value:   "test-value",
			wantErr: false,
		},
		{
			name:    "empty key",
			key:     "",
			value:   "test-value",
			wantErr: false,
		},
		{
			name:    "long key",
			key:     "this-is-a-very-long-key-that-should-still-work-fine",
			value:   "test-value",
			wantErr: false,
		},
		{
			name:    "empty value",
			key:     "my-secret-key",
			value:   "",
			wantErr: false,
			// Skip different key test for empty value since it will always decrypt to empty
		},
		{
			name:    "long value",
			key:     "my-secret-key",
			value:   "this is a very long value that should still encrypt and decrypt properly without any issues",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test encryption
			encrypted, err := encrypt(tt.key, tt.value)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.NotEmpty(t, encrypted)
			assert.NotEqual(t, tt.value, encrypted, "encrypted value should be different from original")

			// Test decryption
			decrypted := decrypt(tt.key, encrypted)
			assert.Equal(t, tt.value, decrypted, "decrypted value should match original")

			// Test that different keys produce different results, except for empty values
			if tt.value != "" {
				differentKey := tt.key + "-different"
				differentDecrypted := decrypt(differentKey, encrypted)
				assert.NotEqual(t, tt.value, differentDecrypted, "decryption with wrong key should not match original")
			}
		})
	}
}
