package local

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"github.com/TykTechnologies/midsommar/v2/secrets"
)

func init() {
	if err := secrets.DefaultRegistry.Register("local", func(rawKey string, _ map[string]string) (secrets.KEKProvider, error) {
		return New(rawKey), nil
	}); err != nil {
		panic(err)
	}
}

// Provider wraps DEKs using a local KEK derived from a passphrase.
// Suitable for single-node deployments without an external KMS.
type Provider struct {
	kek []byte
}

// New creates a KEKProvider that uses AES-256-GCM with a KEK
// derived from rawKey via SHA-256.
func New(rawKey string) *Provider {
	hash := sha256.Sum256([]byte(rawKey))
	return &Provider{kek: hash[:]}
}

func (p *Provider) GenerateDEK(ctx context.Context) ([]byte, error) {
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

func (p *Provider) WrapKey(_ context.Context, dek []byte) ([]byte, error) {
	block, err := aes.NewCipher(p.kek)
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

func (p *Provider) UnwrapKey(_ context.Context, wrappedDEK []byte) ([]byte, error) {
	block, err := aes.NewCipher(p.kek)
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
