package database

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

	"github.com/TykTechnologies/midsommar/v2/secrets"
)

// LocalKeyWrapper wraps DEKs using a local KEK derived from a passphrase.
// Suitable for single-node deployments without an external KMS.
type LocalKeyWrapper struct {
	kek []byte
}

// NewLocalKeyWrapper creates a KeyWrapper that uses AES-256-GCM with a KEK
// derived from rawKey via SHA-256.
func NewLocalKeyWrapper(rawKey string) *LocalKeyWrapper {
	return &LocalKeyWrapper{kek: DeriveKey(rawKey)}
}

func (w *LocalKeyWrapper) WrapKey(_ context.Context, dek []byte) ([]byte, error) {
	block, err := aes.NewCipher(w.kek)
	if err != nil {
		return nil, fmt.Errorf("wrap key: create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("wrap key: create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("wrap key: generate nonce: %w", err)
	}

	return gcm.Seal(nonce, nonce, dek, nil), nil
}

func (w *LocalKeyWrapper) UnwrapKey(_ context.Context, wrappedDEK []byte) ([]byte, error) {
	block, err := aes.NewCipher(w.kek)
	if err != nil {
		return nil, fmt.Errorf("unwrap key: create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("unwrap key: create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(wrappedDEK) < nonceSize {
		return nil, fmt.Errorf("unwrap key: wrapped key too short")
	}

	nonce, ciphertext := wrappedDEK[:nonceSize], wrappedDEK[nonceSize:]
	dek, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("unwrap key: %w", err)
	}

	return dek, nil
}

// EnvelopeCipher implements envelope encryption (v2).
// Uses a shared active DEK from the encryption_keys table.
// Format: $ENC/v2/<key_id>/<base64(nonce+ciphertext)>
type EnvelopeCipher struct {
	wrapper  secrets.KeyWrapper
	keyStore secrets.KeyStore
}

// NewEnvelopeCipher creates a v2 envelope cipher.
func NewEnvelopeCipher(wrapper secrets.KeyWrapper, keyStore secrets.KeyStore) *EnvelopeCipher {
	return &EnvelopeCipher{wrapper: wrapper, keyStore: keyStore}
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

	dek, err := c.wrapper.UnwrapKey(ctx, wrappedDEK)
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

	dek, err := c.wrapper.UnwrapKey(ctx, wrappedDEK)
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
