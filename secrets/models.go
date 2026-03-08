package secrets

import "time"

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
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

const (
	// EncryptionKeyActive is the status for the current key used for new encryptions.
	EncryptionKeyActive = "active"
	// EncryptionKeyRetired means the key is still usable for decryption but not for new encryptions.
	EncryptionKeyRetired = "retired"
)
