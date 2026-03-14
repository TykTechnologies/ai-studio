package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

// KEKProvider abstracts the master encryption key (MEK) operations.
// The MEK stays with the provider — it generates DEKs, wraps them for storage,
// and unwraps them for use. Implementations live in subpackages (e.g., secrets/local)
// and register via init() with the DefaultRegistry.
type KEKProvider interface {
	// KeyID returns the version identifier for this KEK (e.g., "key-2024-01").
	// This value is embedded in encrypted data to enable safe KEK rotation.
	// Multiple KEK providers can coexist with different IDs, allowing decryption
	// of data encrypted with previous KEKs.
	KeyID() string

	// WrapKey wraps a plaintext DEK for storage.
	WrapKey(ctx context.Context, dek []byte) ([]byte, error)
	// UnwrapKey unwraps a stored DEK for use.
	UnwrapKey(ctx context.Context, wrappedDEK []byte) ([]byte, error)
}

// Optional lifecycle interfaces. Providers implement these to hook into
// specific points in the encryption lifecycle. The core code type-asserts
// and calls them when present; providers that don't need them simply
// don't implement them.

// StartupChecker is implemented by providers that need to verify
// connectivity or configuration before first use (e.g., Vault token validity).
type StartupChecker interface {
	Startup(ctx context.Context) error
}

// Shutdowner is implemented by providers that hold resources requiring
// cleanup (e.g., HTTP clients, token renewal goroutines).
type Shutdowner interface {
	Shutdown(ctx context.Context) error
}

// KeyRotatedHook is called after all DEKs have been re-wrapped with a new KEK.
type KeyRotatedHook interface {
	KeyRotated(ctx context.Context, rotated int, failed int) error
}

// GenerateDEK creates a new random 32-byte DEK and returns it wrapped by the given provider.
func GenerateDEK(ctx context.Context, kek KEKProvider) ([]byte, error) {
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("generate dek: %w", err)
	}
	return kek.WrapKey(ctx, dek)
}

// EnvelopeCipher implements envelope encryption (v2) with inline DEK storage.
// Each Encrypt call generates a fresh DEK, wraps it with the KEK, and embeds
// it directly in the encrypted value. No database storage required.
// Format: $ENC/v2/${keyID}/${wrappedDEK}/${ciphertext}
type EnvelopeCipher struct {
	kek      KEKProvider
	kekCache map[string]KEKProvider // Historical KEKs for rotation support
}

// NewEnvelopeCipher creates a v2 envelope cipher with inline DEK storage.
// The kekCache holds historical KEK providers for decrypting data encrypted
// with previous KEKs after rotation.
func NewEnvelopeCipher(kek KEKProvider, kekCache map[string]KEKProvider) *EnvelopeCipher {
	if kekCache == nil {
		kekCache = make(map[string]KEKProvider)
	}
	// Register current KEK in cache
	kekCache[kek.KeyID()] = kek

	return &EnvelopeCipher{
		kek:      kek,
		kekCache: kekCache,
	}
}

func (c *EnvelopeCipher) Version() string { return "v2" }

func (c *EnvelopeCipher) Encrypt(ctx context.Context, _ []byte, plaintext []byte) ([]byte, error) {
	// Generate a fresh per-object DEK
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("envelope encrypt: generate dek: %w", err)
	}

	// Wrap with KEK for inline storage
	wrappedDEK, err := c.kek.WrapKey(ctx, dek)
	if err != nil {
		return nil, fmt.Errorf("envelope encrypt: wrap dek: %w", err)
	}

	// Build AES-GCM from plaintext DEK
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("envelope encrypt: create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("envelope encrypt: create gcm: %w", err)
	}

	// Encrypt plaintext
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("envelope encrypt: generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	// Format: ${keyID}/${wrappedDEK_base64}/${ciphertext_base64}
	keyID := c.kek.KeyID()
	encoded := fmt.Sprintf("%s/%s/%s",
		keyID,
		base64.URLEncoding.EncodeToString(wrappedDEK),
		base64.URLEncoding.EncodeToString(ciphertext),
	)
	return []byte(encoded), nil
}

func (c *EnvelopeCipher) Decrypt(ctx context.Context, _ []byte, payload []byte) ([]byte, error) {
	// Parse "${keyID}/${wrappedDEK_base64}/${ciphertext_base64}"
	parts := strings.SplitN(string(payload), "/", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("envelope decrypt: invalid format, expected <keyID>/<wrappedDEK>/<ciphertext>")
	}

	keyID := parts[0]
	wrappedDEK, err := base64.URLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("envelope decrypt: decode wrapped dek: %w", err)
	}
	ciphertext, err := base64.URLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("envelope decrypt: decode ciphertext: %w", err)
	}

	// Lookup KEK by version (supports rotation)
	kek, ok := c.kekCache[keyID]
	if !ok {
		return nil, fmt.Errorf("envelope decrypt: unknown KEK version %q (available: %v)", keyID, c.availableKeyIDs())
	}

	// Unwrap DEK using historical KEK
	dek, err := kek.UnwrapKey(ctx, wrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("envelope decrypt: unwrap dek with KEK %q: %w", keyID, err)
	}

	// Build AES-GCM from unwrapped DEK
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("envelope decrypt: create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("envelope decrypt: create gcm: %w", err)
	}

	// Decrypt
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("envelope decrypt: ciphertext too short")
	}

	nonce, raw := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, raw, nil)
	if err != nil {
		return nil, fmt.Errorf("envelope decrypt: %w", err)
	}

	return plaintext, nil
}

// availableKeyIDs returns the list of KEK versions available for decryption.
func (c *EnvelopeCipher) availableKeyIDs() []string {
	var ids []string
	for id := range c.kekCache {
		ids = append(ids, id)
	}
	return ids
}

// EncryptEnvelope encrypts plaintext using envelope encryption (v2).
// Returns the full "$ENC/v2/${keyID}/${wrappedDEK}/${ciphertext}" string.
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
