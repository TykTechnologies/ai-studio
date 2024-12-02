package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"gorm.io/gorm"
)

type Secret struct {
	gorm.Model
	ID      uint   `gorm:"primaryKey" json:"id" access:"secrets"`
	VarName string `json:"name"`
	Value   string `json:"value"`
}

var midsommarSecret = "TYK_AI_SECRET_KEY"

func encrypt(keyString string, stringToEncrypt string) (encryptedString string, err error) {
	// convert key to bytes
	key, err := hex.DecodeString(keyString)
	plaintext := []byte(stringToEncrypt)

	// Create a new Cipher Block from the key
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	// The IV needs to be unique, but not secure. Therefore it's common to
	// include it at the beginning of the ciphertext.
	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}

	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(ciphertext[aes.BlockSize:], plaintext)

	// convert to base64
	return base64.URLEncoding.EncodeToString(ciphertext), nil
}

// decrypt from base64 to decrypted string
func decrypt(keyString string, stringToDecrypt string) string {
	key, _ := hex.DecodeString(keyString)
	ciphertext, _ := base64.URLEncoding.DecodeString(stringToDecrypt)

	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}

	// The IV needs to be unique, but not secure. Therefore it's common to
	// include it at the beginning of the ciphertext.
	if len(ciphertext) < aes.BlockSize {
		panic("ciphertext too short")
	}
	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	stream := cipher.NewCFBDecrypter(block, iv)

	// XORKeyStream can work in-place if the two arguments are the same.
	stream.XORKeyStream(ciphertext, ciphertext)

	return fmt.Sprintf("%s", ciphertext)
}

// GetSecretByID retrieves a Secret record from the database by ID.
func GetSecretByID(db *gorm.DB, id uint) (*Secret, error) {
	var settings Secret
	err := db.First(&settings, id).Error
	if err != nil {
		return nil, err
	}

	key := os.Getenv(midsommarSecret)
	settings.Value = decrypt(key, settings.Value)
	return &settings, nil
}

// GetSecretByVarName retrieves a Secret record from the database by it's name.
func GetSecretByVarName(db *gorm.DB, name string) (*Secret, error) {
	var settings Secret
	err := db.Where("var_name = (?)", name).First(&settings).Error
	if err != nil {
		return nil, err
	}
	key := os.Getenv(midsommarSecret)
	settings.Value = decrypt(key, settings.Value)
	return &settings, nil
}

// DeleteSecretByID deletes a Secret record from the database by ID.
func DeleteSecretByID(db *gorm.DB, id uint) error {
	return db.Delete(&Secret{}, id).Error
}

// CreateSecret creates a new Secret record in the database.
func CreateSecret(db *gorm.DB, settings *Secret) error {
	key := os.Getenv(midsommarSecret)
	var err error
	settings.Value, err = encrypt(key, settings.Value)
	if err != nil {
		return err
	}

	return db.Create(settings).Error
}

// UpdateSecret updates an existing Secret record in the database.
func UpdateSecret(db *gorm.DB, settings *Secret) error {
	key := os.Getenv(midsommarSecret)
	var err error
	settings.Value, err = encrypt(key, settings.Value)
	if err != nil {
		return err
	}

	return db.Save(settings).Error
}

func ListSecrets(db *gorm.DB, pageSize int, pageNumber int, all bool) ([]Secret, int64, int, error) {
	var secrets []Secret
	var totalCount int64
	query := db.Model(&Secret{})

	// Get total count of secrets
	if err := query.Count(&totalCount).Error; err != nil {
		return nil, 0, 0, err
	}

	// Calculate total pages
	totalPages := int(totalCount) / pageSize
	if int(totalCount)%pageSize != 0 {
		totalPages++
	}

	// Apply pagination if not requesting all records
	if !all {
		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	// Execute the query
	err := query.Find(&secrets).Error
	if err != nil {
		return nil, 0, 0, err
	}

	return secrets, totalCount, totalPages, nil
}
