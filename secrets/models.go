package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/fips140"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/scrypt"
	"golang.org/x/sync/singleflight"
)

const (
	// encVersionPrefix marks values encrypted with the current scheme
	// (scrypt KDF + AES-GCM). Values without it are legacy AES-CFB with a
	// bare-SHA256-derived key. The ':' character cannot appear in legacy
	// base64url output, so the prefix is an unambiguous discriminator.
	encVersionPrefix = "v2:"

	// encV3Prefix marks values written in FIPS 140-3 mode: PBKDF2-HMAC-SHA256
	// KDF + AES-GCM, "v3:<iterations>:" + base64url(salt || nonce || ct).
	// scrypt (v2) is not a FIPS-approved KDF. Every build reads v3, v2 and
	// legacy; only FIPS-mode processes write v3.
	encV3Prefix = "v3:"

	// pbkdf2Iterations is the v3 work factor. The input is the operator's
	// TYK_AI_SECRET_KEY and each value has its own salt, so every uncached
	// decryption pays one derivation: 210,000 iterations of HMAC-SHA256 cost
	// about the same as v2's scrypt (~100 ms), where OWASP's 600,000 for
	// passwords would triple start-up decryption time. The count is stored
	// in each value, so it can be raised later without breaking reads.
	pbkdf2Iterations = 210_000

	// pbkdf2TestIterations is used inside test binaries, which encrypt many
	// values; the count is stored, so such values decrypt anywhere.
	pbkdf2TestIterations = 1_000

	// pbkdf2MaxIterations bounds the count read from a stored value.
	pbkdf2MaxIterations = 10_000_000

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
// deliberately expensive, and decryption of the same stored value is
// frequent. LRU eviction keeps performance stable when the working set
// exceeds the capacity.
var derivedKeyCache = newKeyCache(4096)

// deriveGroup coalesces concurrent derivations of the same key so a burst of
// cache misses runs scrypt once instead of once per caller.
var deriveGroup singleflight.Group

// scryptComputations counts actual scrypt executions (test observability).
var scryptComputations atomic.Int64

// scryptCostN is the scrypt N every key is derived with: scryptN in every
// binary. N is not stored in the ciphertext, so a value encrypted with one N
// only decrypts with the same N. Only a test binary can lower it
// (UseMinScryptCostForTests), so production always reads and writes at
// scryptN.
var scryptCostN atomic.Int64

// scryptTestN is the N UseMinScryptCostForTests sets.
const scryptTestN = 16

func init() { scryptCostN.Store(scryptN) }

// UseMinScryptCostForTests lowers the scrypt N to scryptTestN for the rest of
// the test binary. One derivation at scryptN takes ~0.5-1 s under -race, and
// every encrypt derives a fresh key (a new random salt). It panics outside a
// test binary. Call it from an init(), before anything is encrypted: values
// encrypted earlier, and fixtures encrypted at scryptN, no longer decrypt.
func UseMinScryptCostForTests() {
	if !testing.Testing() {
		panic("secrets.UseMinScryptCostForTests called outside a test binary")
	}
	scryptCostN.Store(scryptTestN)
}

// deriveKeyScrypt derives a 32-byte AES-256 key from the configured secret
// and a per-value salt using scrypt. Concurrent calls for the same
// (secret, salt) pair share a single computation.
func deriveKeyScrypt(input string, salt []byte) ([]byte, error) {
	n := int(scryptCostN.Load())
	// N is part of the key: a key derived at another N is a different key.
	cacheKey := fmt.Sprintf("%d\x00", n) + input + "\x00" + string(salt)

	if k, ok := derivedKeyCache.get(cacheKey); ok {
		return k, nil
	}

	v, err, _ := deriveGroup.Do(cacheKey, func() (interface{}, error) {
		// Re-check under the flight: a caller that queued behind the leader
		// may arrive after the result has already been cached.
		if k, ok := derivedKeyCache.get(cacheKey); ok {
			return k, nil
		}

		scryptComputations.Add(1)
		key, err := scrypt.Key([]byte(input), salt, n, scryptR, scryptP, 32)
		if err != nil {
			return nil, fmt.Errorf("scrypt key derivation failed: %w", err)
		}

		derivedKeyCache.put(cacheKey, key)
		return key, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]byte), nil
}

// fipsEnabled is crypto/fips140.Enabled; tests replace it to cover the FIPS
// writer in standard runs.
var fipsEnabled = fips140.Enabled

// fipsEnforced is crypto/fips140.Enforced (GODEBUG=fips140=only); tests
// replace it.
var fipsEnforced = fips140.Enforced

// errLegacyUnderEnforcement is returned for a legacy AES-CFB value read
// outside the re-encryption sweep in a fips140=only process.
var errLegacyUnderEnforcement = errors.New("legacy AES-CFB value is not readable under fips140=only; run the re-encryption sweep (it runs at start-up) before enabling fips140=only")

// v3Iterations is the PBKDF2 count new v3 values get.
func v3Iterations() int {
	if testing.Testing() {
		return pbkdf2TestIterations
	}
	return pbkdf2Iterations
}

// deriveKeyPBKDF2 derives the v3 AES-256 key from the configured secret, a
// per-value salt and the stored iteration count, sharing the scrypt path's
// cache and single-flight group.
func deriveKeyPBKDF2(input string, salt []byte, iter int) ([]byte, error) {
	cacheKey := "pbkdf2\x00" + strconv.Itoa(iter) + "\x00" + input + "\x00" + string(salt)
	if k, ok := derivedKeyCache.get(cacheKey); ok {
		return k, nil
	}
	v, err, _ := deriveGroup.Do(cacheKey, func() (interface{}, error) {
		if k, ok := derivedKeyCache.get(cacheKey); ok {
			return k, nil
		}
		key, err := pbkdf2.Key(sha256.New, input, salt, iter, 32)
		if err != nil {
			return nil, fmt.Errorf("pbkdf2 key derivation failed: %w", err)
		}
		derivedKeyCache.put(cacheKey, key)
		return key, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]byte), nil
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

// HasValue reports whether the secret holds a non-empty value, without
// exposing it. Works on a row as stored (encrypted) and on one that has
// already been decrypted: an encrypted empty string is a real ciphertext, so
// the stored form has to be decrypted to know it is empty, while a decrypted
// value is simply checked for emptiness.
func (s *Secret) HasValue() bool {
	if s.Value == "" {
		return false
	}
	// A current-scheme ciphertext reveals whether its plaintext is empty from
	// its length alone: the payload is salt || nonce || GCM(plaintext), and
	// GCM adds exactly its tag to the plaintext length. So an encrypted empty
	// string is always saltLength+nonceSize+tagSize bytes, and anything longer
	// carries a value. No key derivation or decryption is needed, which keeps
	// the secrets list cheap however many secrets it shows.
	if strings.HasPrefix(s.Value, encVersionPrefix) {
		payload, err := base64.URLEncoding.DecodeString(strings.TrimPrefix(s.Value, encVersionPrefix))
		if err != nil {
			return true // not something we wrote; treat the stored text as the value
		}
		return len(payload) > saltLength+gcmNonceSize+gcmTagSize
	}
	// v3 has the same payload layout after its "v3:<iterations>:" header.
	if strings.HasPrefix(s.Value, encV3Prefix) {
		_, encoded, ok := splitV3(s.Value)
		if !ok {
			return true // not something we wrote; treat the stored text as the value
		}
		payload, err := base64.URLEncoding.DecodeString(encoded)
		if err != nil {
			return true
		}
		return len(payload) > saltLength+gcmNonceSize+gcmTagSize
	}
	key := encryptionKey()
	if key == "" {
		return true
	}
	// Legacy CFB values carry no length signal, so fall back to decrypting.
	plaintext, err := decrypt(key, s.Value)
	if err != nil {
		// Not a ciphertext we wrote (already decrypted by GetSecretByID/
		// GetSecretByVarName), so the value itself is the answer.
		return true
	}
	return plaintext != ""
}

// gcmNonceSize and gcmTagSize are the standard AES-GCM nonce and tag sizes
// that encrypt uses: cipher.NewGCMWithRandomNonce prepends a 12-byte nonce
// and appends a 16-byte tag.
const (
	gcmNonceSize = 12
	gcmTagSize   = 16
)

var midsommarSecret = "TYK_AI_SECRET_KEY"

// configuredKey holds a key installed with SetEncryptionKey.
var configuredKey atomic.Pointer[string]

// SetEncryptionKey sets the key that encrypts secrets at rest, taking the
// place of the TYK_AI_SECRET_KEY environment variable. An embedding host calls
// it before any secret is read or written.
func SetEncryptionKey(key string) {
	configuredKey.Store(&key)
}

// encryptionKey returns the key installed with SetEncryptionKey, or the value
// of TYK_AI_SECRET_KEY when none was installed.
func encryptionKey() string {
	if key := configuredKey.Load(); key != nil {
		return *key
	}
	return os.Getenv(midsommarSecret)
}

// encrypt encrypts a value with the current scheme: a 32-byte key derived
// from the configured secret with a random per-value salt, then AES-256-GCM
// (authenticated encryption). Standard builds derive with scrypt and write
// v2; FIPS-mode processes derive with PBKDF2-HMAC-SHA256 and write v3:
//
//	"v2:" + base64url(salt[16] || nonce[12] || gcmCiphertext)
//	"v3:<iterations>:" + base64url(salt[16] || nonce[12] || gcmCiphertext)
func encrypt(keyString string, stringToEncrypt string) (encryptedString string, err error) {
	salt := make([]byte, saltLength)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	prefix := encVersionPrefix
	var key []byte
	if fipsEnabled() {
		iter := v3Iterations()
		prefix = encV3Prefix + strconv.Itoa(iter) + ":"
		key, err = deriveKeyPBKDF2(keyString, salt, iter)
	} else {
		key, err = deriveKeyScrypt(keyString, salt)
	}
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	// NewGCMWithRandomNonce draws a random 12-byte nonce on each Seal and
	// prepends it to the ciphertext, which is exactly the nonce || ciphertext
	// layout this format has always used, and is approved in FIPS mode.
	gcm, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	sealed := gcm.Seal(nil, nil, []byte(stringToEncrypt), nil)

	payload := make([]byte, 0, len(salt)+len(sealed))
	payload = append(payload, salt...)
	payload = append(payload, sealed...)

	return prefix + base64.URLEncoding.EncodeToString(payload), nil
}

// decrypt decrypts a stored value. Values with the "v3:" prefix use
// PBKDF2+AES-GCM, "v2:" scrypt+AES-GCM; anything else is treated as a
// legacy AES-CFB value. Every build reads all three. Malformed input is
// reported as an error rather than a panic, since stored values can be
// tampered with or truncated outside our control.
func decrypt(keyString string, stringToDecrypt string) (string, error) {
	return decryptValue(keyString, stringToDecrypt, false)
}

// decryptForMigration is decrypt for the re-encryption sweep: the one place
// that reads legacy AES-CFB values outside FIPS 140-only enforcement, so it
// can move them to an approved format.
func decryptForMigration(keyString string, stringToDecrypt string) (string, error) {
	return decryptValue(keyString, stringToDecrypt, true)
}

func decryptValue(keyString string, stringToDecrypt string, migration bool) (string, error) {
	if strings.HasPrefix(stringToDecrypt, encV3Prefix) {
		iter, encoded, ok := splitV3(stringToDecrypt)
		if !ok {
			return "", fmt.Errorf("malformed v3 value header")
		}
		return decryptGCM(keyString, encoded, func(salt []byte) ([]byte, error) {
			return deriveKeyPBKDF2(keyString, salt, iter)
		})
	}
	if strings.HasPrefix(stringToDecrypt, encVersionPrefix) {
		return decryptGCM(keyString, strings.TrimPrefix(stringToDecrypt, encVersionPrefix), func(salt []byte) ([]byte, error) {
			return deriveKeyScrypt(keyString, salt)
		})
	}
	return decryptCFBLegacy(keyString, stringToDecrypt, migration)
}

// splitV3 splits "v3:<iterations>:<payload>" into the iteration count and
// the base64 payload, refusing a count out of range.
func splitV3(value string) (iter int, encoded string, ok bool) {
	countStr, encoded, found := strings.Cut(strings.TrimPrefix(value, encV3Prefix), ":")
	if !found {
		return 0, "", false
	}
	iter, err := strconv.Atoi(countStr)
	if err != nil || iter < 1 || iter > pbkdf2MaxIterations {
		return 0, "", false
	}
	return iter, encoded, true
}

// decryptGCM decrypts the base64 salt || nonce || ciphertext payload of a v2
// or v3 value, deriving the key from the salt with derive.
func decryptGCM(keyString string, encoded string, derive func(salt []byte) ([]byte, error)) (string, error) {
	payload, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to base64-decode ciphertext: %w", err)
	}

	if len(payload) < saltLength {
		return "", fmt.Errorf("ciphertext too short: %d bytes", len(payload))
	}
	salt := payload[:saltLength]

	key, err := derive(salt)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	// rest is nonce || ciphertext, the layout NewGCMWithRandomNonce reads.
	gcm, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	rest := payload[saltLength:]
	if len(rest) < gcmNonceSize {
		return "", fmt.Errorf("ciphertext too short: %d bytes", len(payload))
	}

	plaintext, err := gcm.Open(nil, nil, rest, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt value (wrong key or tampered ciphertext): %w", err)
	}

	return string(plaintext), nil
}

// decryptCFBLegacy decrypts values written before the GCM scheme
// (SHA256-derived key + AES-CFB). Retained for migration of stored values.
// AES-CFB is not approved and panics under GODEBUG=fips140=only, so in that
// mode only the sweep (migration) reads it, outside enforcement; any other
// read returns errLegacyUnderEnforcement.
func decryptCFBLegacy(keyString string, stringToDecrypt string, migration bool) (string, error) {
	if !migration && fipsEnforced() {
		return "", errLegacyUnderEnforcement
	}
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

	read := func() {
		stream := cipher.NewCFBDecrypter(block, iv) //nolint:staticcheck // legacy format support only; new values use AES-GCM

		// XORKeyStream can work in-place if the two arguments are the same.
		stream.XORKeyStream(ciphertext, ciphertext)
	}
	if migration {
		// The sweep's read, which moves the value to an approved format.
		fips140.WithoutEnforcement(read)
	} else {
		read()
	}

	return string(ciphertext), nil
}

// reencryptBatchSize bounds how many Secret rows are held in memory at once
// during migration. Variable so tests can exercise multi-batch runs cheaply.
var reencryptBatchSize = 100

// ReencryptLegacySecrets rewrites stored Secret rows to the format this
// process writes. A standard build moves legacy AES-CFB rows to v2
// (scrypt+AES-GCM) and leaves v2 and v3 rows alone; a FIPS-mode process moves
// legacy and v2 rows to v3 (PBKDF2+AES-GCM). Rows that are empty, already
// current, or fail to decrypt (e.g. written under a different key) are left
// untouched. Rows are processed in batches to bound memory usage, with each
// batch's updates applied in a single transaction. Returns the number of
// migrated rows.
func ReencryptLegacySecrets(db *gorm.DB) (int, error) {
	key := encryptionKey()
	if key == "" {
		return 0, nil // nothing to do without a key
	}

	query := db.Where("value <> '' AND value NOT LIKE ? AND value NOT LIKE ?", encVersionPrefix+"%", encV3Prefix+"%")
	if fipsEnabled() {
		query = db.Where("value <> '' AND value NOT LIKE ?", encV3Prefix+"%")
	}

	migrated := 0
	var batch []Secret
	err := query.
		FindInBatches(&batch, reencryptBatchSize, func(_ *gorm.DB, _ int) error {
			return db.Transaction(func(tx *gorm.DB) error {
				for i := range batch {
					ok, err := reencryptRow(tx, key, &batch[i])
					if err != nil {
						return err
					}
					if ok {
						migrated++
					}
				}
				return nil
			})
		}).Error
	if err != nil {
		return migrated, fmt.Errorf("failed to re-encrypt legacy secrets: %w", err)
	}

	if migrated > 0 {
		log.Infof("re-encrypted %d secret(s) to the current encryption format", migrated)
	}
	return migrated, nil
}

// reencryptRow re-encrypts one row read by the sweep. It writes only over
// the value that was read, so a secret updated meanwhile (through the API, or
// by another replica during a rolling start) keeps its new value; ok is false
// then, and for a row that cannot be decrypted.
func reencryptRow(tx *gorm.DB, key string, s *Secret) (ok bool, err error) {
	plaintext, err := decryptForMigration(key, s.Value)
	if err != nil {
		log.Warnf("skipping secret %q during re-encryption: %v", s.VarName, err)
		return false, nil
	}

	reencrypted, err := encrypt(key, plaintext)
	if err != nil {
		log.Warnf("failed to re-encrypt secret %q: %v", s.VarName, err)
		return false, nil
	}

	res := tx.Model(&Secret{}).Where("id = ? AND value = ?", s.ID, s.Value).Update("value", reencrypted)
	if res.Error != nil {
		return false, fmt.Errorf("failed to save re-encrypted secret %q: %w", s.VarName, res.Error)
	}
	return res.RowsAffected == 1, nil
}

// plaintextFallbackWarnOnce ensures the loud missing-key warning is emitted
// once per process rather than on every call.
var plaintextFallbackWarnOnce sync.Once

// EncryptionKeyConfigured reports whether a secrets encryption key is set,
// through SetEncryptionKey or the environment.
func EncryptionKeyConfigured() bool {
	return encryptionKey() != ""
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
	key := encryptionKey()
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
	key := encryptionKey()
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

	key := encryptionKey()
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

	key := encryptionKey()
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
	key := encryptionKey()
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
		key := encryptionKey()
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

func ListSecrets(db *gorm.DB, pageSize int, pageNumber int, all bool, scopes ...func(*gorm.DB) *gorm.DB) ([]Secret, int64, int, error) {
	var secrets []Secret
	var totalCount int64
	query := db.Model(&Secret{})

	// Optional search/sort scopes (services.ListOptions); applied before the
	// count so X-Total-Count reflects the filtered set.
	for _, scope := range scopes {
		query = scope(query)
	}

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
