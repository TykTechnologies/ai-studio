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
	// Check for vN/ prefix pattern (e.g., "v2/", "v99/")
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

// decryptWith decrypts a stored value using three-way prefix detection:
//
//   - "$PLAIN/" → plaintext passthrough, strip prefix and return as-is
//   - "$ENC/"   → encrypted, detect version and decrypt
//   - no prefix → legacy v1 AES-CFB encrypted (base64-encoded, no tag)
//
// If rawKey is empty, the original value is returned as-is (no key to decrypt with).
// Empty strings pass through unchanged.
func decryptWith(ctx context.Context, ciphers map[string]Cipher, rawKey string, value string) (string, error) {
	if value == "" {
		return value, nil
	}

	// $PLAIN/ prefix: strip and return as-is
	if strings.HasPrefix(value, "$PLAIN/") {
		return strings.TrimPrefix(value, "$PLAIN/"), nil
	}

	if rawKey == "" {
		return value, nil
	}

	// $ENC/ prefix: versioned encrypted value
	if strings.HasPrefix(value, "$ENC/") {
		trimmed := strings.TrimPrefix(value, "$ENC/")
		version, payload := detectVersion(trimmed)

		c, ok := ciphers[version]
		if !ok {
			return "", fmt.Errorf("unsupported cipher version: %s", version)
		}

		// v2 envelope uses a structured payload (<key_id>/<ciphertext>)
		if version == "v2" {
			plaintext, err := c.Decrypt(ctx, nil, []byte(payload))
			if err != nil {
				return "", fmt.Errorf("decrypt (version %s): %w", version, err)
			}
			return string(plaintext), nil
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

	// No prefix: legacy v1 (raw base64-encoded AES-CFB ciphertext)
	c, ok := ciphers["v1"]
	if !ok {
		return "", fmt.Errorf("unsupported cipher version: v1")
	}

	raw, err := base64.URLEncoding.DecodeString(value)
	if err != nil {
		return "", fmt.Errorf("decode legacy base64: %w", err)
	}

	key := deriveKey(rawKey)
	plaintext, err := c.Decrypt(ctx, key, raw)
	if err != nil {
		return "", fmt.Errorf("decrypt legacy v1: %w", err)
	}
	return string(plaintext), nil
}

// legacyCipherInstances returns a map of legacy cipher versions (v1 only).
// Used when no envelope encryption is configured.
func legacyCipherInstances() map[string]Cipher {
	return map[string]Cipher{
		"v1": &cfbCipher{},
	}
}
