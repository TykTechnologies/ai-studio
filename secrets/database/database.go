// Package database provides a GORM-backed implementation of secrets.SecretStore.
// It registers itself under the name "database" via init().
package database

import (
	"context"
	"fmt"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/secrets"
)

func init() {
	secrets.RegisterStore("database", func(db interface{}, rawKey string) (secrets.SecretStore, error) {
		gormDB, ok := db.(*gorm.DB)
		if !ok {
			return nil, fmt.Errorf("database store requires *gorm.DB, got %T", db)
		}
		return New(gormDB, rawKey), nil
	})
}

// Database implements secrets.SecretStore backed by a GORM database.
type Database struct {
	db      *gorm.DB
	rawKey  string
	cipher  secrets.Cipher
	ciphers map[string]secrets.Cipher
}

// New creates a new DB-backed secret store.
func New(db *gorm.DB, rawKey string) *Database {
	return &Database{
		db:      db,
		rawKey:  rawKey,
		cipher:  secrets.DefaultCipherInstance(),
		ciphers: secrets.AllCipherInstances(),
	}
}

// DB returns the underlying gorm.DB.
func (s *Database) DB() *gorm.DB {
	return s.db
}

func (s *Database) Create(ctx context.Context, secret *secrets.Secret) error {
	log.Debugf("[DEBUG] CreateSecret: Got key, length: %d", len(s.rawKey))

	encrypted, err := secrets.EncryptWith(ctx, s.cipher, s.rawKey, secret.Value)
	if err != nil {
		log.Errorf("[DEBUG] CreateSecret: Failed to encrypt value: %v", err)
		return err
	}
	secret.Value = encrypted

	if err := s.db.Create(secret).Error; err != nil {
		log.Errorf("[DEBUG] CreateSecret: Failed to create in DB: %v", err)
		return err
	}
	return nil
}

func (s *Database) GetByID(ctx context.Context, id uint, preserveRef bool) (*secrets.Secret, error) {
	var secret secrets.Secret
	if err := s.db.First(&secret, id).Error; err != nil {
		return nil, err
	}

	if preserveRef {
		secret.PreserveReference()
		return &secret, nil
	}

	decrypted, err := secrets.DecryptWith(ctx, s.ciphers, s.rawKey, secret.Value)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret %d: %w", id, err)
	}
	secret.Value = decrypted
	return &secret, nil
}

func (s *Database) GetByVarName(ctx context.Context, name string, preserveRef bool) (*secrets.Secret, error) {
	var secret secrets.Secret
	if err := s.db.Where("var_name = (?)", name).First(&secret).Error; err != nil {
		return nil, err
	}

	if preserveRef {
		secret.PreserveReference()
		return &secret, nil
	}

	decrypted, err := secrets.DecryptWith(ctx, s.ciphers, s.rawKey, secret.Value)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret %q: %w", name, err)
	}
	secret.Value = decrypted
	return &secret, nil
}

func (s *Database) Update(ctx context.Context, secret *secrets.Secret) error {
	encrypted, err := secrets.EncryptWith(ctx, s.cipher, s.rawKey, secret.Value)
	if err != nil {
		return err
	}
	secret.Value = encrypted

	return s.db.Save(secret).Error
}

func (s *Database) Delete(_ context.Context, id uint) error {
	return s.db.Delete(&secrets.Secret{}, id).Error
}

func (s *Database) List(_ context.Context, pageSize, pageNumber int, all bool) ([]secrets.Secret, int64, int, error) {
	var items []secrets.Secret
	var totalCount int64
	query := s.db.Model(&secrets.Secret{})

	if err := query.Count(&totalCount).Error; err != nil {
		return nil, 0, 0, err
	}

	totalPages := int(totalCount) / pageSize
	if int(totalCount)%pageSize != 0 {
		totalPages++
	}

	if !all {
		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	if err := query.Find(&items).Error; err != nil {
		return nil, 0, 0, err
	}

	return items, totalCount, totalPages, nil
}

func (s *Database) EnsureDefaults(ctx context.Context, names []string) error {
	for _, name := range names {
		var count int64
		if err := s.db.Model(&secrets.Secret{}).Where("var_name = ?", name).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			secret := &secrets.Secret{
				VarName: name,
				Value:   "",
			}
			if err := s.Create(ctx, secret); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Database) EncryptValue(ctx context.Context, plaintext string) (string, error) {
	return secrets.EncryptWith(ctx, s.cipher, s.rawKey, plaintext)
}

func (s *Database) DecryptValue(ctx context.Context, ciphertext string) (string, error) {
	return secrets.DecryptWith(ctx, s.ciphers, s.rawKey, ciphertext)
}

func (s *Database) ResolveReference(ctx context.Context, reference string, preserveRef bool) string {
	if !strings.HasPrefix(reference, "$") {
		return reference
	}

	parts := strings.Split(reference, "/")
	if len(parts) != 2 {
		return reference
	}

	loc := parts[0]
	name := parts[1]

	switch loc {
	case "$ENV":
		return os.Getenv(name)
	case "$SECRET":
		if secrets.IsSecretReference(reference) && preserveRef {
			return reference
		}
		val, err := s.GetByVarName(ctx, name, preserveRef)
		if err != nil {
			log.Println(err)
			return reference
		}
		return val.Value
	default:
		return reference
	}
}
