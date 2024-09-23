package models

import (
	"crypto/rand"
	"encoding/base64"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Credential struct {
	gorm.Model
	ID     uint   `json:"id" gorm:"primaryKey"`
	KeyID  string `json:"key_id" gorm:"uniqueIndex"`
	Secret string `json:"secret"`
	Active bool   `json:"active" gorm:"default:false"`
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

func (c *Credential) GetBySecret(db *gorm.DB, secret string) error {
	return db.Where("secret = ?", secret).First(c).Error
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
