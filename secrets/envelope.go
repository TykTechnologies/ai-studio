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
	"sync"
)

// KEKProvider abstracts the master encryption key (MEK) operations.
// The MEK stays with the provider — it generates DEKs, wraps them for storage,
// and unwraps them for use. Implementations live in subpackages (e.g., secrets/local)
// and register via init() with the DefaultRegistry.
type KEKProvider interface {
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

// KeyGeneratedHook is called after a new DEK is created and persisted.
type KeyGeneratedHook interface {
	KeyGenerated(ctx context.Context, keyID uint) error
}

// KeyRotatedHook is called after all DEKs have been re-wrapped with a new KEK.
type KeyRotatedHook interface {
	KeyRotated(ctx context.Context, rotated int, failed int) error
}

// KeyRetiredHook is called after an encryption key is marked as retired.
type KeyRetiredHook interface {
	KeyRetired(ctx context.Context, keyID uint) error
}

// GenerateDEK creates a new random 32-byte DEK and returns it wrapped by the given provider.
func GenerateDEK(ctx context.Context, kek KEKProvider) ([]byte, error) {
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("generate dek: %w", err)
	}
	return kek.WrapKey(ctx, dek)
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

// cachedDEK holds a plaintext DEK and the pre-built AES-GCM cipher for it.
type cachedDEK struct {
	keyID uint
	gcm   cipher.AEAD
}

// EnvelopeCipher implements envelope encryption (v2).
// Uses a shared active DEK from the encryption_keys table.
// Caches unwrapped DEKs in memory to avoid repeated DB lookups and
// KMS unwrap calls on every encrypt/decrypt operation.
// Format: $ENC/v2/<key_id>/<base64(nonce+ciphertext)>
type EnvelopeCipher struct {
	kek      KEKProvider
	keyStore KeyStore

	mu        sync.RWMutex
	activeKey *cachedDEK            // cached active key for encryption
	dekCache  map[uint]*cachedDEK   // cached DEKs by key ID for decryption
}

// NewEnvelopeCipher creates a v2 envelope cipher.
func NewEnvelopeCipher(kek KEKProvider, keyStore KeyStore) *EnvelopeCipher {
	return &EnvelopeCipher{
		kek:      kek,
		keyStore: keyStore,
		dekCache: make(map[uint]*cachedDEK),
	}
}

func (c *EnvelopeCipher) Version() string { return "v2" }

func (c *EnvelopeCipher) Encrypt(ctx context.Context, _ []byte, plaintext []byte) ([]byte, error) {
	cached, err := c.getActiveGCM(ctx)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, cached.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("envelope encrypt: generate nonce: %w", err)
	}

	ct := cached.gcm.Seal(nonce, nonce, plaintext, nil)

	// Format: <key_id>/<base64(nonce+ciphertext)>
	encoded := fmt.Sprintf("%d/%s", cached.keyID, base64.URLEncoding.EncodeToString(ct))

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

	cached, err := c.getGCMByKeyID(ctx, uint(keyID))
	if err != nil {
		return nil, err
	}

	nonceSize := cached.gcm.NonceSize()
	if len(ct) < nonceSize {
		return nil, fmt.Errorf("envelope decrypt: ciphertext too short")
	}

	nonce, raw := ct[:nonceSize], ct[nonceSize:]
	result, err := cached.gcm.Open(nil, nonce, raw, nil)
	if err != nil {
		return nil, fmt.Errorf("envelope decrypt: %w", err)
	}

	return result, nil
}

// getActiveGCM returns the cached active-key GCM cipher, loading it on first use.
func (c *EnvelopeCipher) getActiveGCM(ctx context.Context) (*cachedDEK, error) {
	c.mu.RLock()
	if c.activeKey != nil {
		cached := c.activeKey
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	// Double-check after acquiring write lock.
	if c.activeKey != nil {
		return c.activeKey, nil
	}

	encKey, err := c.keyStore.GetActiveKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("envelope encrypt: get active key: %w", err)
	}

	cached, err := c.unwrapAndBuild(ctx, encKey)
	if err != nil {
		return nil, err
	}
	c.activeKey = cached
	c.dekCache[cached.keyID] = cached
	return cached, nil
}

// getGCMByKeyID returns the cached GCM cipher for a given key ID, loading it on cache miss.
func (c *EnvelopeCipher) getGCMByKeyID(ctx context.Context, keyID uint) (*cachedDEK, error) {
	c.mu.RLock()
	if cached, ok := c.dekCache[keyID]; ok {
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	// Double-check after acquiring write lock.
	if cached, ok := c.dekCache[keyID]; ok {
		return cached, nil
	}

	encKey, err := c.keyStore.GetKeyByID(ctx, keyID)
	if err != nil {
		return nil, fmt.Errorf("envelope decrypt: get key %d: %w", keyID, err)
	}

	cached, err := c.unwrapAndBuild(ctx, encKey)
	if err != nil {
		return nil, err
	}
	c.dekCache[keyID] = cached
	return cached, nil
}

// unwrapAndBuild decodes/unwraps a stored key and builds an AES-GCM cipher from it.
func (c *EnvelopeCipher) unwrapAndBuild(ctx context.Context, encKey *EncryptionKey) (*cachedDEK, error) {
	wrappedDEK, err := base64.URLEncoding.DecodeString(encKey.WrappedKey)
	if err != nil {
		return nil, fmt.Errorf("envelope: decode wrapped key %d: %w", encKey.ID, err)
	}

	dek, err := c.kek.UnwrapKey(ctx, wrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("envelope: unwrap key %d: %w", encKey.ID, err)
	}

	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("envelope: create cipher for key %d: %w", encKey.ID, err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("envelope: create gcm for key %d: %w", encKey.ID, err)
	}

	return &cachedDEK{keyID: encKey.ID, gcm: gcm}, nil
}

// ClearCache invalidates the in-memory DEK cache. Call after key rotation
// to ensure the next operation picks up the new active key.
func (c *EnvelopeCipher) ClearCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.activeKey = nil
	c.dekCache = make(map[uint]*cachedDEK)
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
