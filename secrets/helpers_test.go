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
	kek []byte
}

func newTestLocalKEK(rawKey string) *testLocalKEK {
	salt := []byte("tyk-ai-studio-local-kek-v1")
	kek := argon2.IDKey([]byte(rawKey), salt, 3, 64*1024, 4, 32)
	return &testLocalKEK{kek: kek}
}

func (p *testLocalKEK) GenerateDEK(ctx context.Context) ([]byte, error) {
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("generate dek: %w", err)
	}
	wrapped, err := p.WrapKey(ctx, dek)
	if err != nil {
		return nil, fmt.Errorf("wrap new dek: %w", err)
	}
	return wrapped, nil
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

func TestMain(m *testing.M) {
	// Register the local provider for tests since we can't import secrets/local
	// from same-package tests.
	DefaultRegistry.Register("local", func(config map[string]string) (KEKProvider, error) {
		return newTestLocalKEK(config["RAW_KEY"]), nil
	})
	os.Exit(m.Run())
}
