package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"testing"

	"golang.org/x/crypto/argon2"
)

// testLocalKEK is a test-only KEKProvider identical to local.Provider.
// We can't import secrets/local from package secrets tests, so we
// duplicate the minimal implementation here for testing.
type testLocalKEK struct {
	kek   []byte
	keyID string
}

func newTestLocalKEK(rawKey string) *testLocalKEK {
	salt := []byte("tyk-ai-studio-local-kek-v1")
	kek := argon2.IDKey([]byte(rawKey), salt, 3, 64*1024, 4, 32)
	return &testLocalKEK{
		kek:   kek,
		keyID: "test-key-v1",
	}
}

func (p *testLocalKEK) KeyID() string {
	return p.keyID
}

func (p *testLocalKEK) WrapKey(_ context.Context, dek []byte) ([]byte, error) {
	block, err := aes.NewCipher(p.kek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, dek, nil), nil
}

func (p *testLocalKEK) UnwrapKey(_ context.Context, wrappedDEK []byte) ([]byte, error) {
	block, err := aes.NewCipher(p.kek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(wrappedDEK) < nonceSize {
		return nil, fmt.Errorf("unwrap key: wrapped key too short")
	}
	nonce, ciphertext := wrappedDEK[:nonceSize], wrappedDEK[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// encryptWith encrypts plaintext using the given key and cipher, returning
// the versioned "$ENC/..." string. Test-only — v1 encryption is never used
// in production (v1 is read-only).
func encryptWith(ctx context.Context, c Cipher, rawKey string, plaintext string) (string, error) {
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

// newTestStore creates a test store with given KEK and cache.
func newTestStore(db *testing.T, kek KEKProvider) *Store {
	dbInst := setupTestDB(db)
	cache := map[string]KEKProvider{kek.KeyID(): kek}
	return NewWithKEKProvider(dbInst, "test-key", kek, cache)
}

func TestMain(m *testing.M) {
	// Register the local provider for tests since we can't import secrets/local
	// from same-package tests.
	DefaultRegistry.Register("local", func(config map[string]string) (KEKProvider, error) {
		kek := newTestLocalKEK(config["RAW_KEY"])
		if keyID := config["KEK_ID"]; keyID != "" {
			kek.keyID = keyID
		}
		return kek, nil
	})
	os.Exit(m.Run())
}
