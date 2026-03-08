package secrets

import (
	"context"
	"time"
)

// Secret represents an encrypted secret stored in the database.
type Secret struct {
	ID        uint       `gorm:"primaryKey" json:"id" access:"secrets"`
	CreatedAt time.Time  `json:"-"`
	UpdatedAt time.Time  `json:"-"`
	DeletedAt *time.Time `gorm:"index" json:"-"`
	VarName   string     `gorm:"uniqueIndex" json:"name"`
	Value     string     `json:"value"`

	// Transient field to control if we should return the reference format
	preserveReference bool `gorm:"-" json:"-"`
}

// PreserveReference sets the secret to return in reference format
func (s *Secret) PreserveReference() {
	s.preserveReference = true
}

// GetValue returns either the decrypted value or the reference format
func (s *Secret) GetValue() string {
	if s.preserveReference {
		return GetSecretReference(s.VarName)
	}
	return s.Value
}

// EncryptionKey represents a wrapped data encryption key (DEK) in the database.
// The actual DEK is wrapped (encrypted) by a Key Encryption Key (KEK) and stored
// in WrappedKey. The plaintext DEK is never persisted.
type EncryptionKey struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	WrappedKey string    `json:"-"`
	Status     string    `gorm:"default:active;index" json:"status"`
	ObjectType string    `gorm:"index;default:''" json:"object_type"` // e.g. "secret", "submission", "datasource"
	ObjectID   uint      `gorm:"index;default:0" json:"object_id"`   // ID of the encrypted object
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Context key type for encryption metadata.
type encMetaKeyType struct{}

var encMetaKey = encMetaKeyType{}

// EncryptionMeta holds metadata about what is being encrypted.
type EncryptionMeta struct {
	ObjectType string
	ObjectID   uint
}

// WithEncryptionMeta returns a context carrying encryption metadata.
func WithEncryptionMeta(ctx context.Context, objectType string, objectID uint) context.Context {
	return context.WithValue(ctx, encMetaKey, EncryptionMeta{ObjectType: objectType, ObjectID: objectID})
}

// encryptionMetaFromCtx extracts encryption metadata from context, if present.
func encryptionMetaFromCtx(ctx context.Context) (EncryptionMeta, bool) {
	m, ok := ctx.Value(encMetaKey).(EncryptionMeta)
	return m, ok
}

const (
	// EncryptionKeyActive is the status for the current key used for new encryptions.
	EncryptionKeyActive = "active"
	// EncryptionKeyRetired means the key is still usable for decryption but not for new encryptions.
	EncryptionKeyRetired = "retired"
)
