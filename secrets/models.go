package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// deriveKey takes any string and returns a 32-byte key suitable for AES-256
func deriveKey(input string) []byte {
	hash := sha256.Sum256([]byte(input))
	return hash[:]
}

type Secret struct {
	gorm.Model
	ID      uint   `gorm:"primaryKey" json:"id" access:"secrets"`
	VarName string `json:"name"`
	Value   string `json:"value"`

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

var midsommarSecret = "TYK_AI_SECRET_KEY"

func encrypt(keyString string, stringToEncrypt string) (encryptedString string, err error) {
	// Derive a proper 32-byte key from the input string
	log.Printf("[DEBUG] Deriving key from input of length: %d", len(keyString))
	key := deriveKey(keyString)
	log.Printf("[DEBUG] Successfully derived key, length: %d", len(key))

	plaintext := []byte(stringToEncrypt)

	// Create a new Cipher Block from the derived key
	block, err := aes.NewCipher(key)
	if err != nil {
		log.Errorf("[DEBUG] Failed to create cipher block: %v", err)
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
	key := deriveKey(keyString)
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

// EncryptValue encrypts a plaintext string using the application's AES key.
// Returns the encrypted value or the original if encryption fails or is not configured.
func EncryptValue(plaintext string) string {
	if plaintext == "" || plaintext == "[redacted]" {
		return plaintext
	}
	key := os.Getenv(midsommarSecret)
	if key == "" {
		return plaintext // No encryption key configured
	}
	encrypted, err := encrypt(key, plaintext)
	if err != nil {
		return plaintext // Graceful fallback
	}
	return "$ENC/" + encrypted
}

// DecryptValue decrypts a value that was encrypted with EncryptValue.
// Returns the decrypted plaintext, or the original value if not encrypted.
func DecryptValue(value string) string {
	if !strings.HasPrefix(value, "$ENC/") {
		return value // Not encrypted
	}
	key := os.Getenv(midsommarSecret)
	if key == "" {
		return value // No key to decrypt with
	}
	encrypted := strings.TrimPrefix(value, "$ENC/")
	return decrypt(key, encrypted)
}

// GetSecretByID retrieves a Secret record from the database by ID.
func GetSecretByID(db *gorm.DB, id uint, preserveRef bool) (*Secret, error) {
	var settings Secret
	err := db.First(&settings, id).Error
	if err != nil {
		return nil, err
	}

	if preserveRef {
		settings.PreserveReference()
		return &settings, nil
	}

	key := os.Getenv(midsommarSecret)
	settings.Value = decrypt(key, settings.Value)
	return &settings, nil
}

// GetSecretByVarName retrieves a Secret record from the database by it's name.
func GetSecretByVarName(db *gorm.DB, name string, preserveRef bool) (*Secret, error) {
	var settings Secret
	err := db.Where("var_name = (?)", name).First(&settings).Error
	if err != nil {
		return nil, err
	}

	if preserveRef {
		settings.PreserveReference()
		return &settings, nil
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
	log.Debugf("[DEBUG] CreateSecret: Got key from env, length: %d", len(key))

	var err error
	settings.Value, err = encrypt(key, settings.Value)
	if err != nil {
		log.Errorf("[DEBUG] CreateSecret: Failed to encrypt value: %v", err)
		return err
	}

	if err := db.Create(settings).Error; err != nil {
		log.Errorf("[DEBUG] CreateSecret: Failed to create in DB: %v", err)
		return err
	}
	return nil
}

// UpdateSecret updates an existing Secret record in the database.
// When encryptValue is true, the Value field is encrypted before saving.
// Pass false when the Value already contains the stored (encrypted) value and should not be re-encrypted.
func UpdateSecret(db *gorm.DB, settings *Secret, encryptValue bool) error {
	if encryptValue {
		key := os.Getenv(midsommarSecret)
		var err error
		settings.Value, err = encrypt(key, settings.Value)
		if err != nil {
			return err
		}
	}

	return db.Save(settings).Error
}

// GetOrCreateDefaultSecrets ensures default secrets exist in the database.
// This function creates OPENAI_KEY and ANTHROPIC_KEY secrets with empty values
// if they don't already exist, allowing users to fill in their API keys later.
func GetOrCreateDefaultSecrets(db *gorm.DB) error {
	defaultSecrets := []string{"OPENAI_KEY", "ANTHROPIC_KEY"}

	for _, name := range defaultSecrets {
		// Check if secret already exists by name
		var count int64
		if err := db.Model(&Secret{}).Where("var_name = ?", name).Count(&count).Error; err != nil {
			return err
		}

		// Only create if it doesn't exist
		if count == 0 {
			secret := &Secret{
				VarName: name,
				Value:   "", // Empty value - user will fill in later
			}
			if err := CreateSecret(db, secret); err != nil {
				return err
			}
		}
	}
	return nil
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
