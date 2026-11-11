package models

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/schema"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/google/uuid"
)

// Credential is an App's gateway key.
//
// At rest (midsommar-clean-4x5y.15): the secret is stored encrypted when
// TYK_AI_SECRET_KEY is set (as written without it), and it is found by
// secret_hash (secrets.LookupHash), never by its value.
//
// It is encrypted under the database's data key (secrets.SealWithDataKey:
// one key derived once per process from TYK_AI_SECRET_KEY and the
// database's salt, then AES-256-GCM per value, bound to the credential's
// key_id), not with secrets.EncryptValue: that derives a key per value,
// about 100 ms each, which is fine for the handful of LLM credentials and
// would stall a hub with thousands of App keys. Sealing or opening one here
// costs microseconds.
//
// Secret holds the plaintext in memory: the hub has to be able to show the
// key again and uses it as a bearer token itself. BeforeSave computes the
// lookup hash and the sealed form, which the field's serializer writes, and
// AfterFind opens what was read, for a credential on its own or as an App's
// association. A save never puts the stored form in Secret, so one that
// fails (a database error during a failover) can be retried as it is.
// Lookups by the secret (GetBySecret, the control server's ValidateToken)
// compare hashes and do not read the secret column at all. A column or map
// update of the secret would bypass the hooks; there is none, and
// TestCredentialSecretIsOnlySavedWhole keeps it that way.
type Credential struct {
	gorm.Model
	ID     uint   `json:"id" gorm:"primaryKey"`
	KeyID  string `json:"key_id" gorm:"uniqueIndex"`
	Secret string `json:"secret" gorm:"serializer:credential_secret"`
	// SecretHash is what the credential is looked up by. Null on a row no
	// current Studio has migrated yet; such a row is found by nothing.
	SecretHash *string `json:"-" gorm:"column:secret_hash;size:80;uniqueIndex"`
	Active     bool    `json:"active" gorm:"default:false"`

	// stored is the secret as it is in the database, kept while Secret
	// holds secrets.RefusedCredential (it could not be opened), so that
	// saving the row for another reason does not overwrite a secret that
	// the right key would still read.
	stored string
	// sealed is what BeforeSave prepared for the serializer to write, for
	// the secret and key_id in sealedFor.
	sealed, sealedFor string
}

// isEncrypted reports whether v is a secret in a stored, encrypted form:
// sealed under the data key, or secrets.EncryptValue output (which this
// change wrote on development stacks before it used the data key).
func isEncrypted(v string) bool {
	return secrets.IsDataKeyValue(v) || strings.HasPrefix(v, encryptedPrefix)
}

// credentialAAD binds a sealed secret to its row: a ciphertext copied into
// another credential's row does not open there.
func credentialAAD(keyID string) string { return "credential\x00" + keyID }

const credentialSecretSerializerName = "credential_secret"

func init() {
	schema.RegisterSerializer(credentialSecretSerializerName, credentialSecretSerializer{})
}

// credentialSecretSerializer writes the stored form BeforeSave prepared, so
// that the struct's Secret is never replaced by it. Reading is left to
// AfterFind, which has the database (for its salt) and the whole row.
type credentialSecretSerializer struct{}

// Value is the secret as it is stored.
func (credentialSecretSerializer) Value(_ context.Context, _ *schema.Field, dst reflect.Value, fieldValue interface{}) (interface{}, error) {
	plain, _ := fieldValue.(string)
	if plain == "" {
		return "", nil
	}
	c := credentialOf(dst)
	if c == nil || c.sealedFor != plain+"\x00"+c.KeyID {
		// Written by something that did not run BeforeSave: refuse rather
		// than store a secret in plaintext or without its hash.
		return nil, errors.New("a credential's secret can only be saved with the whole credential (its hooks hash and encrypt it)")
	}
	return c.sealed, nil
}

// Scan puts the stored value in the struct for AfterFind to open.
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
	field.ReflectValueOf(ctx, dst).SetString(stored)
	return nil
}

// credentialOf is the Credential a serializer call is for.
func credentialOf(dst reflect.Value) *Credential {
	if dst.Kind() == reflect.Struct && dst.CanAddr() {
		if c, ok := dst.Addr().Interface().(*Credential); ok {
			return c
		}
	}
	return nil
}

var plaintextCredentialWarning sync.Once

// sealCredentialSecret is plain as it is stored for the credential keyID:
// sealed under the database's data key, or plain itself when no
// TYK_AI_SECRET_KEY is set (with secrets' own warning, as for every value
// stored unencrypted).
func sealCredentialSecret(tx *gorm.DB, keyID, plain string) (string, error) {
	if !secrets.EncryptionKeyConfigured() {
		plaintextCredentialWarning.Do(func() {
			logger.Errorf("SECURITY WARNING: TYK_AI_SECRET_KEY is not set: App credentials are stored as plaintext. This warning is logged once per process.")
		})
		return plain, nil
	}
	salt, err := EnsureDataKeySalt(tx)
	if err != nil {
		return "", err
	}
	return secrets.SealWithDataKey(salt, plain, credentialAAD(keyID))
}

// openCredentialSecret is the plaintext of a stored secret, and whether it
// could be opened.
func openCredentialSecret(tx *gorm.DB, keyID, stored string) (string, bool) {
	switch {
	case secrets.IsDataKeyValue(stored):
		salt, err := loadDataKeySalt(tx)
		if err != nil || salt == nil {
			return "", false
		}
		plain, err := secrets.OpenWithDataKey(salt, stored, credentialAAD(keyID))
		return plain, err == nil
	case strings.HasPrefix(stored, encryptedPrefix):
		plain := secrets.DecryptValue(stored)
		return plain, !strings.HasPrefix(plain, encryptedPrefix)
	}
	return stored, true
}

// BeforeSave computes the lookup hash and the stored form of the secret
// being saved. It leaves Secret alone.
func (c *Credential) BeforeSave(tx *gorm.DB) error {
	if reflect.Indirect(reflect.ValueOf(tx.Statement.Dest)).Kind() == reflect.Map {
		// A column or map update (Update("active", ...)) does not write
		// the secret.
		return nil
	}
	plain := c.Secret
	if isEncrypted(plain) {
		// A caller holding the stored form: save what it opens to.
		opened, ok := openCredentialSecret(tx, c.KeyID, plain)
		if !ok {
			// Not readable here: written back as it is, hash untouched.
			c.sealed, c.sealedFor = plain, plain+"\x00"+c.KeyID
			return nil
		}
		plain = opened
	}
	switch {
	case plain == "":
		c.SecretHash = nil
		c.sealed, c.sealedFor = "", ""
	case plain == secrets.RefusedCredential && c.stored != "":
		// Not readable with this key: the stored secret is kept, and so
		// is its hash.
		c.sealed, c.sealedFor = c.stored, c.Secret+"\x00"+c.KeyID
	default:
		sealed, err := sealCredentialSecret(tx, c.KeyID, plain)
		if err != nil {
			return fmt.Errorf("encrypt the credential's secret: %w", err)
		}
		hash := secrets.LookupHash(plain)
		c.SecretHash = &hash
		c.sealed, c.sealedFor = sealed, c.Secret+"\x00"+c.KeyID
	}
	return nil
}

// AfterFind opens the secret that was read. One that cannot be opened (no
// key, another key, a lost salt, another row's ciphertext) reads as
// secrets.RefusedCredential, never as its stored form, which would be shown
// and sent as the key. A read that left the secret column out has nothing
// to open.
func (c *Credential) AfterFind(tx *gorm.DB) error {
	c.sealed, c.sealedFor = "", ""
	if !isEncrypted(c.Secret) {
		c.stored = ""
		return nil
	}
	stored := c.Secret
	plain, ok := openCredentialSecret(tx, c.KeyID, stored)
	if !ok {
		logger.Errorf("Credential %d: the stored secret cannot be read (TYK_AI_SECRET_KEY unset or changed, or the database's data key salt is missing); it is refused until that is put right", c.ID)
		c.Secret, c.stored = secrets.RefusedCredential, stored
		return nil
	}
	c.Secret, c.stored = plain, ""
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
//
// The row is found by comparing hashes, and its secret column is not read:
// the caller's secret is the row's (that is what the hash matching means),
// so nothing is decrypted to answer "whose key is this".
func (c *Credential) GetBySecret(db *gorm.DB, secret string) error {
	if secret == "" {
		return gorm.ErrRecordNotFound
	}
	if err := db.Omit("secret").Where("secret_hash = ?", secrets.LookupHash(secret)).First(c).Error; err != nil {
		return err
	}
	c.Secret = secret
	return nil
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
