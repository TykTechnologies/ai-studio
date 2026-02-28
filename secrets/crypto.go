package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

// Cipher provides encrypt/decrypt operations for a specific algorithm version.
type Cipher interface {
	Encrypt(ctx context.Context, key []byte, plaintext []byte) ([]byte, error)
	Decrypt(ctx context.Context, key []byte, ciphertext []byte) ([]byte, error)
	Version() string
}

// deriveKey takes any string and returns a 32-byte key suitable for AES-256.
func deriveKey(input string) []byte {
	hash := sha256.Sum256([]byte(input))
	return hash[:]
}

// detectVersion inspects a trimmed ciphertext payload (after removing "$ENC/" prefix)
// and returns the version string and the raw payload. Legacy data without a version
// prefix is treated as "v1".
func detectVersion(trimmed string) (version string, payload string) {
	// Check for vN/ prefix pattern (e.g., "v2/", "v3/", "v99/")
	if idx := strings.Index(trimmed, "/"); idx > 0 {
		candidate := trimmed[:idx]
		if len(candidate) >= 2 && candidate[0] == 'v' {
			allDigits := true
			for _, c := range candidate[1:] {
				if c < '0' || c > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				return candidate, trimmed[idx+1:]
			}
		}
	}
	// Legacy format: no version prefix
	return "v1", trimmed
}

// cfbCipher implements the legacy AES-CFB encryption (v1).
// This cipher does NOT provide authentication — it exists only
// for backwards compatibility with existing encrypted data.
type cfbCipher struct{}

func (c *cfbCipher) Version() string { return "v1" }

func (c *cfbCipher) Encrypt(_ context.Context, key []byte, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cfb encrypt: create cipher: %w", err)
	}

	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("cfb encrypt: generate iv: %w", err)
	}

	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], plaintext)
	return ciphertext, nil
}

func (c *cfbCipher) Decrypt(_ context.Context, key []byte, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cfb decrypt: create cipher: %w", err)
	}

	if len(ciphertext) < aes.BlockSize {
		return nil, fmt.Errorf("cfb decrypt: ciphertext too short")
	}

	iv := ciphertext[:aes.BlockSize]
	raw := make([]byte, len(ciphertext[aes.BlockSize:]))
	copy(raw, ciphertext[aes.BlockSize:])

	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(raw, raw)
	return raw, nil
}

// gcmCipher implements AES-256-GCM encryption (v2).
// GCM provides both confidentiality and authenticity.
type gcmCipher struct{}

func (c *gcmCipher) Version() string { return "v2" }

func (c *gcmCipher) Encrypt(_ context.Context, key []byte, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("gcm encrypt: create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm encrypt: create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("gcm encrypt: generate nonce: %w", err)
	}

	// nonce is prepended to the ciphertext
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func (c *gcmCipher) Decrypt(_ context.Context, key []byte, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("gcm decrypt: create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm decrypt: create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("gcm decrypt: ciphertext too short")
	}

	nonce, raw := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, raw, nil)
	if err != nil {
		return nil, fmt.Errorf("gcm decrypt: %w", err)
	}

	return plaintext, nil
}

// EncryptWith encrypts plaintext using the given key and cipher, returning
// the versioned "$ENC/..." string. Empty or "[redacted]" values pass through unchanged.
// If rawKey is empty, plaintext is returned as-is (no encryption key configured).
func EncryptWith(ctx context.Context, c Cipher, rawKey string, plaintext string) (string, error) {
	if plaintext == "" || plaintext == "[redacted]" {
		return plaintext, nil
	}
	if rawKey == "" {
		return plaintext, nil
	}

	key := deriveKey(rawKey)
	ciphertext, err := c.Encrypt(ctx, key, []byte(plaintext))
	if err != nil {
		return "", err
	}

	encoded := base64.URLEncoding.EncodeToString(ciphertext)

	if c.Version() == "v1" {
		return "$ENC/" + encoded, nil
	}
	return "$ENC/" + c.Version() + "/" + encoded, nil
}

// DecryptWith decrypts a "$ENC/..." string, auto-detecting the cipher version.
// Non-encrypted values pass through unchanged.
// If rawKey is empty, the original value is returned as-is (no key to decrypt with).
func DecryptWith(ctx context.Context, ciphers map[string]Cipher, rawKey string, value string) (string, error) {
	if !strings.HasPrefix(value, "$ENC/") {
		return value, nil
	}
	if rawKey == "" {
		return value, nil
	}

	trimmed := strings.TrimPrefix(value, "$ENC/")
	version, payload := detectVersion(trimmed)

	c, ok := ciphers[version]
	if !ok {
		return "", fmt.Errorf("unsupported cipher version: %s", version)
	}

	raw, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("decode base64: %w", err)
	}

	key := deriveKey(rawKey)
	plaintext, err := c.Decrypt(ctx, key, raw)
	if err != nil {
		return "", fmt.Errorf("decrypt (version %s): %w", version, err)
	}

	return string(plaintext), nil
}

// AllCipherInstances returns a map of all known cipher versions.
func AllCipherInstances() map[string]Cipher {
	return map[string]Cipher{
		"v1": &cfbCipher{},
		"v2": &gcmCipher{},
	}
}

// DefaultCipherInstance returns the current default cipher for new encryptions.
func DefaultCipherInstance() Cipher {
	return &gcmCipher{}
}
