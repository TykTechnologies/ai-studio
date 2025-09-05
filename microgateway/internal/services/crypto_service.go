// internal/services/crypto_service.go
package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/pbkdf2"
)

// CryptoService provides cryptographic operations
type CryptoService struct {
	key []byte
	gcm cipher.AEAD
}

// NewCryptoService creates a new crypto service with the given encryption key
func NewCryptoService(encryptionKey string) CryptoServiceInterface {
	if len(encryptionKey) != 32 {
		panic(fmt.Sprintf("encryption key must be exactly 32 bytes, got %d", len(encryptionKey)))
	}

	key := []byte(encryptionKey)
	
	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(fmt.Sprintf("failed to create AES cipher: %v", err))
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(fmt.Sprintf("failed to create GCM: %v", err))
	}

	return &CryptoService{
		key: key,
		gcm: gcm,
	}
}

// Encrypt encrypts a plaintext string using AES-GCM
func (c *CryptoService) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	// Create a random nonce
	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the plaintext
	ciphertext := c.gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// Encode to base64 for storage
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a ciphertext string using AES-GCM
func (c *CryptoService) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	// Decode from base64
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}

	// Check minimum length
	nonceSize := c.gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	// Split nonce and ciphertext
	nonce, encryptedData := data[:nonceSize], data[nonceSize:]

	// Decrypt
	plaintext, err := c.gcm.Open(nil, nonce, encryptedData, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

// HashSecret creates a bcrypt hash of a secret
func (c *CryptoService) HashSecret(secret string) string {
	// Use bcrypt with default cost for password-like secrets
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		// Fallback to SHA256 if bcrypt fails
		h := sha256.Sum256([]byte(secret))
		return fmt.Sprintf("sha256:%x", h)
	}
	return fmt.Sprintf("bcrypt:%s", string(hash))
}

// VerifySecret verifies a secret against its hash
func (c *CryptoService) VerifySecret(secret, hash string) bool {
	if hash == "" || secret == "" {
		return false
	}

	// Check hash type
	if len(hash) > 7 {
		switch hash[:7] {
		case "bcrypt:":
			err := bcrypt.CompareHashAndPassword([]byte(hash[7:]), []byte(secret))
			return err == nil
		case "sha256:":
			h := sha256.Sum256([]byte(secret))
			expectedHash := fmt.Sprintf("sha256:%x", h)
			return expectedHash == hash
		}
	}

	// Fallback: direct comparison (not recommended for production)
	return hash == secret
}

// GenerateSecureToken generates a cryptographically secure random token
func (c *CryptoService) GenerateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// GenerateKeyPair generates a new encryption key pair for API keys
func (c *CryptoService) GenerateKeyPair() (keyID, secret string, err error) {
	// Generate key ID (shorter, used for identification)
	keyIDBytes := make([]byte, 16)
	if _, err := rand.Read(keyIDBytes); err != nil {
		return "", "", fmt.Errorf("failed to generate key ID: %w", err)
	}
	keyID = base64.URLEncoding.EncodeToString(keyIDBytes)

	// Generate secret (longer, used for authentication)
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", "", fmt.Errorf("failed to generate secret: %w", err)
	}
	secret = base64.URLEncoding.EncodeToString(secretBytes)

	return keyID, secret, nil
}

// DeriveKey derives a key from a password using PBKDF2
func (c *CryptoService) DeriveKey(password, salt string, iterations int) []byte {
	if salt == "" {
		salt = "microgateway-default-salt" // Should be random in production
	}
	
	return pbkdf2.Key([]byte(password), []byte(salt), iterations, 32, sha256.New)
}

// ValidateEncryptionKey validates that an encryption key is suitable
func ValidateEncryptionKey(key string) error {
	if len(key) != 32 {
		return fmt.Errorf("encryption key must be exactly 32 characters, got %d", len(key))
	}

	// Check for common weak keys
	weakKeys := []string{
		"change-me-in-production00000000",
		"00000000000000000000000000000000",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}

	for _, weak := range weakKeys {
		if key == weak {
			return fmt.Errorf("encryption key appears to be a weak/default key")
		}
	}

	return nil
}