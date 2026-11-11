package models

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"reflect"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/schema"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/google/uuid"
)

// Credential is an App's gateway key.
//
// At rest (midsommar-clean-4x5y.15): the secret is stored as
// secrets.EncryptValue output ($ENC/) when TYK_AI_SECRET_KEY is set, and as
// written without it, as LLM credentials are; and it is found by
// secret_hash (secrets.LookupHash), never by its value.
//
// Secret always holds the plaintext in memory. A gorm serializer on the
// field (credentialSecretSerializer) encrypts it as it is written and
// decrypts it as it is read, for a credential on its own or as an App's
// association, so every reader sees the plaintext: the hub has to be able
// to show the key again and uses it as a bearer token itself. The struct is
// never changed to its stored form, so a save that fails (a database error
// during a failover) leaves nothing to undo and can be retried as it is.
// BeforeSave sets the lookup hash. A column or map update of the secret
// would bypass that; there is none, and TestCredentialSecretIsOnlySavedWhole
// keeps it that way.
type Credential struct {
	gorm.Model
	ID     uint   `json:"id" gorm:"primaryKey"`
	KeyID  string `json:"key_id" gorm:"uniqueIndex"`
	Secret string `json:"secret" gorm:"serializer:credential_secret"`
	// SecretHash is what the credential is looked up by (GetBySecret,
	// the control server's ValidateToken). Null on a row no current Studio
	// has migrated yet; such a row is found by nothing.
	SecretHash *string `json:"-" gorm:"column:secret_hash;size:80;uniqueIndex"`
	Active     bool    `json:"active" gorm:"default:false"`

	// stored is the secret as it is in the database, kept while Secret
	// holds secrets.RefusedCredential (it did not decrypt), so that saving
	// the row for another reason does not overwrite a secret that the
	// right key would still read.
	stored string
}

// isEncrypted reports whether v is secrets.EncryptValue output.
func isEncrypted(v string) bool { return strings.HasPrefix(v, encryptedPrefix) }

const credentialSecretSerializerName = "credential_secret"

func init() {
	schema.RegisterSerializer(credentialSecretSerializerName, credentialSecretSerializer{})
}

// credentialSecretSerializer stores Credential.Secret encrypted and reads it
// back as plaintext, without ever putting the stored form in the struct.
type credentialSecretSerializer struct{}

// Value is the secret as it is stored.
func (credentialSecretSerializer) Value(_ context.Context, _ *schema.Field, dst reflect.Value, fieldValue interface{}) (interface{}, error) {
	plain, _ := fieldValue.(string)
	switch {
	case plain == "":
		return "", nil
	case plain == secrets.RefusedCredential:
		// Not readable with this key: keep what is stored.
		if c := credentialOf(dst); c != nil && c.stored != "" {
			return c.stored, nil
		}
		return plain, nil
	case isEncrypted(plain):
		// Already in stored form (a caller holding a raw row): as it is.
		return plain, nil
	}
	return secrets.EncryptValue(plain), nil // plain again when no key is set
}

// Scan reads a stored secret into the struct as plaintext. One that does
// not decrypt (no key, another key) reads as secrets.RefusedCredential,
// never as its ciphertext, which would be shown and sent as the key.
func (credentialSecretSerializer) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, dbValue interface{}) error {
	var stored string
	switch v := dbValue.(type) {
	case nil:
	case string:
		stored = v
	case []byte:
		stored = string(v)
	default:
		return fmt.Errorf("credential secret: unsupported column value %T", dbValue)
	}
	plain, kept := stored, ""
	if isEncrypted(stored) {
		if plain = secrets.DecryptValue(stored); isEncrypted(plain) {
			logger.Errorf("A credential's stored secret does not decrypt with TYK_AI_SECRET_KEY (unset or changed); it is refused until the key is set again")
			plain, kept = secrets.RefusedCredential, stored
		}
	}
	field.ReflectValueOf(ctx, dst).SetString(plain)
	if c := credentialOf(dst); c != nil {
		c.stored = kept
	}
	return nil
}

// credentialOf is the Credential a serializer call is for, when dst is one
// (it is: the serializer is used on no other model).
func credentialOf(dst reflect.Value) *Credential {
	if dst.Kind() == reflect.Struct && dst.CanAddr() {
		if c, ok := dst.Addr().Interface().(*Credential); ok {
			return c
		}
	}
	return nil
}

// BeforeSave sets the lookup hash of the secret being saved. It leaves
// Secret alone.
func (c *Credential) BeforeSave(tx *gorm.DB) error {
	if reflect.Indirect(reflect.ValueOf(tx.Statement.Dest)).Kind() == reflect.Map {
		// A column or map update (Update("active", ...)) does not write
		// the secret.
		return nil
	}
	plain := c.Secret
	if isEncrypted(plain) {
		// A caller holding the stored form: hash what it decrypts to.
		plain = secrets.DecryptValue(plain)
	}
	switch {
	case plain == "":
		c.SecretHash = nil
	case plain == secrets.RefusedCredential || isEncrypted(plain):
		// Not readable with this key: the stored secret is kept, and so
		// is its hash.
	default:
		hash := secrets.LookupHash(plain)
		c.SecretHash = &hash
	}
	return nil
}

func NewCredential() (*Credential, error) {
	// Generate a random 32-byte secret key
	secretBytes := make([]byte, 32)
	_, err := rand.Read(secretBytes)
	if err != nil {
		return nil, err
	}
	secret := base64.URLEncoding.EncodeToString(secretBytes)

	// Generate a UUID for KeyID
	keyID := uuid.New().String()

	return &Credential{
		KeyID:  keyID,
		Secret: secret,
		Active: false,
	}, nil
}

func (c *Credential) Create(db *gorm.DB) error {
	return db.Create(c).Error
}

func (c *Credential) Get(db *gorm.DB, id uint) error {
	return db.First(c, id).Error
}

func (c *Credential) GetByKeyID(db *gorm.DB, keyID string) error {
	return db.Where("key_id = ?", keyID).First(c).Error
}

// GetBySecret finds the credential with this secret, by its lookup hash.
// An empty secret finds nothing.
func (c *Credential) GetBySecret(db *gorm.DB, secret string) error {
	if secret == "" {
		return gorm.ErrRecordNotFound
	}
	return db.Where("secret_hash = ?", secrets.LookupHash(secret)).First(c).Error
}

func (c *Credential) Update(db *gorm.DB) error {
	return db.Save(c).Error
}

func (c *Credential) Delete(db *gorm.DB) error {
	return db.Delete(c).Error
}

func (c *Credential) Activate(db *gorm.DB) error {
	c.Active = true
	return c.Update(db)
}

func (c *Credential) Deactivate(db *gorm.DB) error {
	c.Active = false
	return c.Update(db)
}

type Credentials []Credential

func (cl *Credentials) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool) (int64, int, error) {
	var totalCount int64
	query := db.Model(&Credential{})

	if err := query.Count(&totalCount).Error; err != nil {
		return 0, 0, err
	}

	totalPages := int(totalCount) / pageSize
	if int(totalCount)%pageSize != 0 {
		totalPages++
	}

	if !all {
		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	err := query.Find(cl).Error
	return totalCount, totalPages, err
}

func (cl *Credentials) GetActive(db *gorm.DB) error {
	return db.Where("active = ?", true).Find(cl).Error
}
