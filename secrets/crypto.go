package secrets

import "context"

// Cipher provides encrypt/decrypt operations for a specific algorithm version.
type Cipher interface {
	Encrypt(ctx context.Context, key []byte, plaintext []byte) ([]byte, error)
	Decrypt(ctx context.Context, key []byte, ciphertext []byte) ([]byte, error)
	Version() string
}
