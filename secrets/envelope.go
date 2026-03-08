package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// KEKProvider abstracts the master encryption key (MEK) operations.
// The MEK stays with the provider — it generates DEKs, wraps them for storage,
// and unwraps them for use. Implementations live in subpackages (e.g., secrets/local)
// and register via init() with the DefaultRegistry.
type KEKProvider interface {
	// GenerateDEK creates a new random DEK and returns it wrapped for storage.
	GenerateDEK(ctx context.Context) (wrappedDEK []byte, err error)
	// WrapKey wraps a plaintext DEK for storage.
	WrapKey(ctx context.Context, dek []byte) ([]byte, error)
	// UnwrapKey unwraps a stored DEK for use.
	UnwrapKey(ctx context.Context, wrappedDEK []byte) ([]byte, error)
}

// KeyStore provides access to encryption keys. The database implementation
// stores keys in the encryption_keys table.
type KeyStore interface {
	// GetActiveKey returns the current active encryption key.
	// If none exists, one is created.
	GetActiveKey(ctx context.Context) (*EncryptionKey, error)
	// GetKeyByID returns an encryption key by ID for decryption.
	GetKeyByID(ctx context.Context, id uint) (*EncryptionKey, error)
	// CreateKey creates a new encryption key with the given wrapped key bytes.
	CreateKey(ctx context.Context, wrappedKey string, status string) (*EncryptionKey, error)
	// ListKeys returns all encryption keys.
	ListKeys(ctx context.Context) ([]EncryptionKey, error)
	// UpdateKey updates an encryption key record.
	UpdateKey(ctx context.Context, key *EncryptionKey) error
}

// EnvelopeCipher implements envelope encryption (v2).
// Uses a shared active DEK from the encryption_keys table.
// Format: $ENC/v2/<key_id>/<base64(nonce+ciphertext)>
type EnvelopeCipher struct {
	kek      KEKProvider
	keyStore KeyStore
}

// NewEnvelopeCipher creates a v2 envelope cipher.
func NewEnvelopeCipher(kek KEKProvider, keyStore KeyStore) *EnvelopeCipher {
	return &EnvelopeCipher{kek: kek, keyStore: keyStore}
}

func (c *EnvelopeCipher) Version() string { return "v2" }

func (c *EnvelopeCipher) Encrypt(ctx context.Context, _ []byte, plaintext []byte) ([]byte, error) {
	// Get or create the active encryption key
	encKey, err := c.keyStore.GetActiveKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("envelope encrypt: get active key: %w", err)
	}

	// Unwrap the DEK
	wrappedDEK, err := base64.URLEncoding.DecodeString(encKey.WrappedKey)
	if err != nil {
		return nil, fmt.Errorf("envelope encrypt: decode wrapped key: %w", err)
	}

	dek, err := c.kek.UnwrapKey(ctx, wrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("envelope encrypt: unwrap key: %w", err)
	}

	// Encrypt plaintext with DEK using AES-GCM
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("envelope encrypt: create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("envelope encrypt: create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("envelope encrypt: generate nonce: %w", err)
	}

	ct := gcm.Seal(nonce, nonce, plaintext, nil)

	// Format: <key_id>/<base64(nonce+ciphertext)>
	encoded := fmt.Sprintf("%d/%s", encKey.ID, base64.URLEncoding.EncodeToString(ct))

	return []byte(encoded), nil
}

func (c *EnvelopeCipher) Decrypt(ctx context.Context, _ []byte, payload []byte) ([]byte, error) {
	// Parse "<key_id>/<ciphertext_base64>"
	parts := strings.SplitN(string(payload), "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("envelope decrypt: invalid format, expected <key_id>/<ciphertext>")
	}

	keyID, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("envelope decrypt: invalid key id %q: %w", parts[0], err)
	}

	ct, err := base64.URLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("envelope decrypt: decode ciphertext: %w", err)
	}

	// Look up the encryption key
	encKey, err := c.keyStore.GetKeyByID(ctx, uint(keyID))
	if err != nil {
		return nil, fmt.Errorf("envelope decrypt: get key %d: %w", keyID, err)
	}

	// Unwrap the DEK
	wrappedDEK, err := base64.URLEncoding.DecodeString(encKey.WrappedKey)
	if err != nil {
		return nil, fmt.Errorf("envelope decrypt: decode wrapped key: %w", err)
	}

	dek, err := c.kek.UnwrapKey(ctx, wrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("envelope decrypt: unwrap key %d: %w", keyID, err)
	}

	// Decrypt with DEK
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("envelope decrypt: create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("envelope decrypt: create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ct) < nonceSize {
		return nil, fmt.Errorf("envelope decrypt: ciphertext too short")
	}

	nonce, raw := ct[:nonceSize], ct[nonceSize:]
	result, err := gcm.Open(nil, nonce, raw, nil)
	if err != nil {
		return nil, fmt.Errorf("envelope decrypt: %w", err)
	}

	return result, nil
}

// EncryptEnvelope encrypts plaintext using envelope encryption (v2).
// Returns the full "$ENC/v2/<key_id>/<ciphertext>" string.
// Empty or "[redacted]" values pass through unchanged.
func EncryptEnvelope(ctx context.Context, c *EnvelopeCipher, plaintext string) (string, error) {
	if plaintext == "" || plaintext == "[redacted]" {
		return plaintext, nil
	}

	ct, err := c.Encrypt(ctx, nil, []byte(plaintext))
	if err != nil {
		return "", err
	}

	return "$ENC/v2/" + string(ct), nil
}
