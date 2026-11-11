package models

import (
	"crypto/rand"
	"encoding/base64"
	"reflect"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/secrets"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/google/uuid"
)

// Credential is an App's gateway key.
//
// At rest (midsommar-clean-4x5y.15): the secret is stored as
// secrets.EncryptValue output ($ENC/) when TYK_AI_SECRET_KEY is set, and as
// written without it, as LLM credentials are; and it is found by
// secret_hash (secrets.LookupHash), never by its value. The hooks below do
// this for every gorm write and read of a Credential, on its own or as an
// App's association, so every reader sees the plaintext in Secret: the hub
// has to be able to show the key again and uses it as a bearer token
// itself. A column or map update of the secret would run no hook; there is
// none, and TestCredentialSecretIsOnlySavedWhole keeps it that way.
type Credential struct {
	gorm.Model
	ID     uint   `json:"id" gorm:"primaryKey"`
	KeyID  string `json:"key_id" gorm:"uniqueIndex"`
	Secret string `json:"secret"`
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

// BeforeSave puts the secret in its stored form and sets its lookup hash.
func (c *Credential) BeforeSave(tx *gorm.DB) error {
	if reflect.Indirect(reflect.ValueOf(tx.Statement.Dest)).Kind() == reflect.Map {
		// A column or map update (Update("active", ...)) does not write
		// the secret and leaves the struct alone.
		return nil
	}
	switch {
	case c.Secret == secrets.RefusedCredential && c.stored != "":
		// Not readable with this key: keep what is stored, hash included.
		c.Secret = c.stored
	case c.Secret == "":
		c.SecretHash = nil
	default:
		hash := secrets.LookupHash(c.Secret)
		c.SecretHash = &hash
		c.Secret = secrets.EncryptValue(c.Secret)
	}
	return nil
}

// AfterSave gives the caller back the plaintext BeforeSave replaced.
func (c *Credential) AfterSave(tx *gorm.DB) error {
	if reflect.Indirect(reflect.ValueOf(tx.Statement.Dest)).Kind() == reflect.Map {
		return nil
	}
	c.decryptSecret()
	return nil
}

// AfterFind decrypts the secret.
func (c *Credential) AfterFind(tx *gorm.DB) error {
	c.decryptSecret()
	return nil
}

// decryptSecret puts the stored secret back in plaintext. One that does not
// decrypt (no key, another key) reads as secrets.RefusedCredential, never as
// its ciphertext, which would be shown and sent as the key.
func (c *Credential) decryptSecret() {
	if !isEncrypted(c.Secret) {
		return
	}
	stored := c.Secret
	plain := secrets.DecryptValue(stored)
	if isEncrypted(plain) {
		logger.Errorf("Credential %d: the stored secret does not decrypt with TYK_AI_SECRET_KEY (unset or changed); it is refused until the key is set again", c.ID)
		c.Secret, c.stored = secrets.RefusedCredential, stored
		return
	}
	c.Secret, c.stored = plain, ""
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
