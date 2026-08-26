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
	"sync"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/scrypt"
	"gorm.io/gorm"
)

const (
	// encVersionPrefix marks values encrypted with the current scheme
	// (scrypt KDF + AES-GCM). Values without it are legacy AES-CFB with a
	// bare-SHA256-derived key. The ':' character cannot appear in legacy
	// base64url output, so the prefix is an unambiguous discriminator.
	encVersionPrefix = "v2:"

	// saltLength is the per-value random scrypt salt size in bytes.
	saltLength = 16

	// scrypt parameters (interactive-grade, per the x/crypto/scrypt docs)
	scryptN = 32768
	scryptR = 8
	scryptP = 1
)

// deriveKey takes any string and returns a 32-byte key suitable for AES-256.
// Retained solely to decrypt legacy AES-CFB values; new values use
// deriveKeyScrypt.
func deriveKey(input string) []byte {
	hash := sha256.Sum256([]byte(input))
	return hash[:]
}

// derivedKeyCache memoizes scrypt outputs per (secret, salt) pair — scrypt is
// deliberately expensive, and decryption of the same stored value is frequent.
var (
	derivedKeyCacheMu sync.Mutex
	derivedKeyCache   = map[string][]byte{}
)

const derivedKeyCacheMax = 4096

// deriveKeyScrypt derives a 32-byte AES-256 key from the configured secret
// and a per-value salt using scrypt.
func deriveKeyScrypt(input string, salt []byte) ([]byte, error) {
	cacheKey := input + "\x00" + string(salt)

	derivedKeyCacheMu.Lock()
	if k, ok := derivedKeyCache[cacheKey]; ok {
		derivedKeyCacheMu.Unlock()
		return k, nil
	}
	derivedKeyCacheMu.Unlock()

	key, err := scrypt.Key([]byte(input), salt, scryptN, scryptR, scryptP, 32)
	if err != nil {
		return nil, fmt.Errorf("scrypt key derivation failed: %w", err)
	}

	derivedKeyCacheMu.Lock()
	if len(derivedKeyCache) >= derivedKeyCacheMax {
		derivedKeyCache = map[string][]byte{}
	}
	derivedKeyCache[cacheKey] = key
	derivedKeyCacheMu.Unlock()

	return key, nil
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

// encrypt encrypts a value with the current scheme: a 32-byte key derived
// from the configured secret via scrypt with a random per-value salt, then
// AES-256-GCM (authenticated encryption). Output layout:
//
//	"v2:" + base64url(salt[16] || nonce[12] || gcmCiphertext)
func encrypt(keyString string, stringToEncrypt string) (encryptedString string, err error) {
	salt := make([]byte, saltLength)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	key, err := deriveKeyScrypt(keyString, salt)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	sealed := gcm.Seal(nil, nonce, []byte(stringToEncrypt), nil)

	payload := make([]byte, 0, len(salt)+len(nonce)+len(sealed))
	payload = append(payload, salt...)
	payload = append(payload, nonce...)
	payload = append(payload, sealed...)

	return encVersionPrefix + base64.URLEncoding.EncodeToString(payload), nil
}

// decrypt decrypts a stored value. Values with the "v2:" prefix use
// scrypt+AES-GCM; anything else is treated as a legacy AES-CFB value.
// Malformed input is reported as an error rather than a panic, since stored
// values can be tampered with or truncated outside our control.
func decrypt(keyString string, stringToDecrypt string) (string, error) {
	if strings.HasPrefix(stringToDecrypt, encVersionPrefix) {
		return decryptGCM(keyString, strings.TrimPrefix(stringToDecrypt, encVersionPrefix))
	}
	return decryptCFBLegacy(keyString, stringToDecrypt)
}

// decryptGCM decrypts a value produced by encrypt (current scheme).
func decryptGCM(keyString string, encoded string) (string, error) {
	payload, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to base64-decode ciphertext: %w", err)
	}

	if len(payload) < saltLength {
		return "", fmt.Errorf("ciphertext too short: %d bytes", len(payload))
	}
	salt := payload[:saltLength]

	key, err := deriveKeyScrypt(keyString, salt)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	rest := payload[saltLength:]
	if len(rest) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short: %d bytes", len(payload))
	}
	nonce := rest[:gcm.NonceSize()]
	sealed := rest[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt value (wrong key or tampered ciphertext): %w", err)
	}

	return string(plaintext), nil
}

// decryptCFBLegacy decrypts values written before the GCM scheme
// (SHA256-derived key + AES-CFB). Retained for migration of stored values.
func decryptCFBLegacy(keyString string, stringToDecrypt string) (string, error) {
	key := deriveKey(keyString)
	ciphertext, err := base64.URLEncoding.DecodeString(stringToDecrypt)
	if err != nil {
		return "", fmt.Errorf("failed to base64-decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	// The IV needs to be unique, but not secure. Therefore it's common to
	// include it at the beginning of the ciphertext.
	if len(ciphertext) < aes.BlockSize {
		return "", fmt.Errorf("ciphertext too short: %d bytes", len(ciphertext))
	}
	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	stream := cipher.NewCFBDecrypter(block, iv) //nolint:staticcheck // legacy format support only; new values use AES-GCM

	// XORKeyStream can work in-place if the two arguments are the same.
	stream.XORKeyStream(ciphertext, ciphertext)

	return string(ciphertext), nil
}

// ReencryptLegacySecrets rewrites stored Secret rows that still use the
// legacy AES-CFB format to the current scrypt+AES-GCM scheme. Rows that are
// empty, already migrated, or fail to decrypt (e.g. written under a
// different key) are left untouched. Returns the number of migrated rows.
func ReencryptLegacySecrets(db *gorm.DB) (int, error) {
	key := os.Getenv(midsommarSecret)
	if key == "" {
		return 0, nil // nothing to do without a key
	}

	var all []Secret
	if err := db.Find(&all).Error; err != nil {
		return 0, fmt.Errorf("failed to list secrets for migration: %w", err)
	}

	migrated := 0
	for i := range all {
		s := &all[i]
		if s.Value == "" || strings.HasPrefix(s.Value, encVersionPrefix) {
			continue
		}

		plaintext, err := decryptCFBLegacy(key, s.Value)
		if err != nil {
			log.Warnf("skipping secret %q during re-encryption: %v", s.VarName, err)
			continue
		}

		reencrypted, err := encrypt(key, plaintext)
		if err != nil {
			log.Warnf("failed to re-encrypt secret %q: %v", s.VarName, err)
			continue
		}

		if err := db.Model(&Secret{}).Where("id = ?", s.ID).Update("value", reencrypted).Error; err != nil {
			return migrated, fmt.Errorf("failed to save re-encrypted secret %q: %w", s.VarName, err)
		}
		migrated++
	}

	if migrated > 0 {
		log.Infof("re-encrypted %d legacy secret(s) to authenticated encryption format", migrated)
	}
	return migrated, nil
}

// plaintextFallbackWarnOnce ensures the loud missing-key warning is emitted
// once per process rather than on every call.
var plaintextFallbackWarnOnce sync.Once

// EncryptionKeyConfigured reports whether the secrets encryption key
// environment variable is set.
func EncryptionKeyConfigured() bool {
	return os.Getenv(midsommarSecret) != ""
}

// WarnIfEncryptionUnconfigured emits a prominent startup warning when the
// encryption key is not configured, so operators know stored secrets will be
// persisted as plaintext. Returns true when the key is configured.
func WarnIfEncryptionUnconfigured() bool {
	if EncryptionKeyConfigured() {
		return true
	}
	log.Errorf("SECURITY WARNING: %s is not set — secrets and credentials will be stored as PLAINTEXT. Set %s to enable encryption at rest.", midsommarSecret, midsommarSecret)
	return false
}

// EncryptValue encrypts a plaintext string using the application's AES key.
// Returns the encrypted value or the original if encryption fails or is not configured.
func EncryptValue(plaintext string) string {
	if plaintext == "" || plaintext == "[redacted]" {
		return plaintext
	}
	key := os.Getenv(midsommarSecret)
	if key == "" {
		// No encryption key configured: the value is stored as plaintext.
		// Warn loudly once, and at debug level on subsequent calls.
		plaintextFallbackWarnOnce.Do(func() {
			log.Errorf("SECURITY WARNING: %s is not set — value stored as plaintext without encryption. This warning is logged once per process.", midsommarSecret)
		})
		log.Debugf("%s not set; storing value as plaintext", midsommarSecret)
		return plaintext
	}
	encrypted, err := encrypt(key, plaintext)
	if err != nil {
		log.Errorf("SECURITY WARNING: encryption failed (%v) — value stored as plaintext", err)
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
	decrypted, err := decrypt(key, encrypted)
	if err != nil {
		log.Errorf("failed to decrypt stored value: %v", err)
		return value // Return stored value untouched rather than crashing
	}
	return decrypted
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
	decrypted, err := decrypt(key, settings.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secret %d: %w", settings.ID, err)
	}
	settings.Value = decrypted
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
	decrypted, err := decrypt(key, settings.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secret %q: %w", settings.VarName, err)
	}
	settings.Value = decrypted
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
