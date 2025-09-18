// internal/services/simple_test.go
package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Simple working test to verify test infrastructure
func TestCryptoService_Basic(t *testing.T) {
	crypto := NewCryptoService("12345678901234567890123456789012")

	t.Run("HashSecret", func(t *testing.T) {
		secret := "test-secret"
		hash := crypto.HashSecret(secret)
		assert.NotEmpty(t, hash)
		assert.NotEqual(t, secret, hash)

		// Verify the secret
		isValid := crypto.VerifySecret(secret, hash)
		assert.True(t, isValid)

		// Verify wrong secret fails
		isValid = crypto.VerifySecret("wrong-secret", hash)
		assert.False(t, isValid)
	})

	t.Run("EncryptDecrypt", func(t *testing.T) {
		plaintext := "sensitive-data-123"

		ciphertext, err := crypto.Encrypt(plaintext)
		assert.NoError(t, err)
		assert.NotEmpty(t, ciphertext)
		assert.NotEqual(t, plaintext, ciphertext)

		decrypted, err := crypto.Decrypt(ciphertext)
		assert.NoError(t, err)
		assert.Equal(t, plaintext, decrypted)
	})
}

func TestValidateEncryptionKey_Simple(t *testing.T) {
	t.Run("ValidKey", func(t *testing.T) {
		err := ValidateEncryptionKey("12345678901234567890123456789012")
		assert.NoError(t, err)
	})

	t.Run("InvalidLength", func(t *testing.T) {
		err := ValidateEncryptionKey("short")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exactly 32 characters")
	})
}