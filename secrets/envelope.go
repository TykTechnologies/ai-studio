package secrets

import "context"

// KeyWrapper wraps and unwraps Data Encryption Keys (DEKs).
// Implementations include a local AES-GCM wrapper (for development/single-node)
// and external KMS backends (Vault, AWS KMS) for production.
type KeyWrapper interface {
	WrapKey(ctx context.Context, dek []byte) ([]byte, error)
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
