package local

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"time"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"golang.org/x/crypto/argon2"
)

// Argon2id parameters for KEK derivation.
const (
	argon2Time    = 3
	argon2Memory  = 64 * 1024 // 64 MB
	argon2Threads = 4
	argon2KeyLen  = 32
)

// kekSalt is a fixed, application-scoped salt for KEK derivation.
// A fixed salt is acceptable here because the passphrase is the sole
// entropy source and we are not defending against multi-target attacks
// across different applications. Per-user or per-row salts don't apply —
// there is exactly one KEK per deployment.
var kekSalt = []byte("tyk-ai-studio-local-kek-v1")

func init() {
	secrets.DefaultRegistry.Register("local", func(config map[string]string) (secrets.KEKProvider, error) {
		rawKey := config["RAW_KEY"]
		if rawKey == "" {
			return nil, fmt.Errorf("local KEK provider requires RAW_KEY in config")
		}

		// Read keyID from config or auto-generate from date
		keyID := config["KEK_ID"]
		if keyID == "" {
			now := time.Now()
			keyID = fmt.Sprintf("key-%d-%02d", now.Year(), now.Month())
		}

		return New(rawKey, keyID), nil
	})
}

// Provider wraps DEKs using a local KEK derived from a passphrase.
// Suitable for single-node deployments without an external KMS.
type Provider struct {
	kek   []byte
	keyID string
}

// New creates a KEKProvider with explicit version identifier.
// The keyID is embedded in encrypted values for KEK rotation tracking.
func New(rawKey string, keyID string) *Provider {
	kek := argon2.IDKey([]byte(rawKey), kekSalt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	return &Provider{
		kek:   kek,
		keyID: keyID,
	}
}

// KeyID returns the version identifier for this KEK.
func (p *Provider) KeyID() string {
	return p.keyID
}

func (p *Provider) WrapKey(_ context.Context, dek []byte) ([]byte, error) {
	return wrapWithKey(p.kek, dek)
}

func (p *Provider) UnwrapKey(_ context.Context, wrappedDEK []byte) ([]byte, error) {
	return unwrapWithKey(p.kek, wrappedDEK)
}

func wrapWithKey(kek, dek []byte) ([]byte, error) {
	block, err := aes.NewCipher(kek)
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

func unwrapWithKey(kek, wrappedDEK []byte) ([]byte, error) {
	block, err := aes.NewCipher(kek)
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
	return gcm.Open(nil, nonce, ciphertext, nil)
}
