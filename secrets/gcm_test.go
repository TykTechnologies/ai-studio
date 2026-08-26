package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Issue #393: secrets at rest must use authenticated encryption (AES-GCM)
// with a proper KDF (scrypt + per-value salt), while legacy AES-CFB values
// remain readable for migration.

func TestEncryptProducesVersionedGCMFormat(t *testing.T) {
	enc, err := encrypt("my-key", "hello world")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(enc, encVersionPrefix), "new ciphertext should carry the %q version prefix, got %q", encVersionPrefix, enc)

	// Payload must be valid base64 and long enough for salt+nonce+tag
	raw, err := base64.URLEncoding.DecodeString(strings.TrimPrefix(enc, encVersionPrefix))
	require.NoError(t, err)
	assert.Greater(t, len(raw), saltLength+12, "payload must include salt, nonce, and ciphertext")
}

func TestGCMRoundTrip(t *testing.T) {
	for _, val := range []string{"", "short", "a much longer value with unicode ✓ and symbols $/%\""} {
		enc, err := encrypt("round-trip-key", val)
		require.NoError(t, err)
		dec, err := decrypt("round-trip-key", enc)
		require.NoError(t, err)
		assert.Equal(t, val, dec)
	}
}

func TestGCMWrongKeyFailsAuthentication(t *testing.T) {
	enc, err := encrypt("key-one", "sensitive")
	require.NoError(t, err)

	_, err = decrypt("key-two", enc)
	assert.Error(t, err, "decryption with the wrong key must fail authentication, not return garbage")
}

func TestGCMTamperDetection(t *testing.T) {
	enc, err := encrypt("tamper-key", "sensitive-value")
	require.NoError(t, err)

	raw, err := base64.URLEncoding.DecodeString(strings.TrimPrefix(enc, encVersionPrefix))
	require.NoError(t, err)
	raw[len(raw)-1] ^= 0xFF // flip a bit in the ciphertext/tag
	tampered := encVersionPrefix + base64.URLEncoding.EncodeToString(raw)

	_, err = decrypt("tamper-key", tampered)
	assert.Error(t, err, "tampered ciphertext must fail authentication")
}

func TestUniqueSaltsAndNonces(t *testing.T) {
	a, err := encrypt("same-key", "same-value")
	require.NoError(t, err)
	b, err := encrypt("same-key", "same-value")
	require.NoError(t, err)
	assert.NotEqual(t, a, b, "identical inputs must produce distinct ciphertexts (random salt+nonce)")
}

// legacyCFBEncrypt replicates the pre-#393 encryption scheme
// (sha256 key derivation + AES-CFB) to prove backwards compatibility.
func legacyCFBEncrypt(t *testing.T, keyString, plaintext string) string {
	t.Helper()
	hash := sha256.Sum256([]byte(keyString))
	block, err := aes.NewCipher(hash[:])
	require.NoError(t, err)

	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]
	_, err = io.ReadFull(rand.Reader, iv)
	require.NoError(t, err)

	stream := cipher.NewCFBEncrypter(block, iv) //nolint:staticcheck // intentionally reproducing legacy format
	stream.XORKeyStream(ciphertext[aes.BlockSize:], []byte(plaintext))
	return base64.URLEncoding.EncodeToString(ciphertext)
}

func TestLegacyCFBValuesStillDecrypt(t *testing.T) {
	legacy := legacyCFBEncrypt(t, "legacy-key", "old-secret-value")
	dec, err := decrypt("legacy-key", legacy)
	require.NoError(t, err)
	assert.Equal(t, "old-secret-value", dec)
}

func TestDecryptValueHandlesBothFormats(t *testing.T) {
	t.Setenv(midsommarSecret, "env-key")

	// Legacy "$ENC/" value written by the old code
	legacy := "$ENC/" + legacyCFBEncrypt(t, "env-key", "legacy-payload")
	assert.Equal(t, "legacy-payload", DecryptValue(legacy))

	// New value written by the current code
	fresh := EncryptValue("fresh-payload")
	assert.True(t, strings.HasPrefix(fresh, "$ENC/"+encVersionPrefix))
	assert.Equal(t, "fresh-payload", DecryptValue(fresh))
}

func TestScryptKeyDerivation(t *testing.T) {
	salt := []byte("0123456789abcdef")
	k1, err := deriveKeyScrypt("password", salt)
	require.NoError(t, err)
	assert.Len(t, k1, 32)

	// Deterministic for same input+salt
	k2, err := deriveKeyScrypt("password", salt)
	require.NoError(t, err)
	assert.Equal(t, k1, k2)

	// Different salt -> different key
	k3, err := deriveKeyScrypt("password", []byte("fedcba9876543210"))
	require.NoError(t, err)
	assert.NotEqual(t, k1, k3)

	// Different password -> different key
	k4, err := deriveKeyScrypt("other-password", salt)
	require.NoError(t, err)
	assert.NotEqual(t, k1, k4)
}

func TestReencryptLegacySecrets(t *testing.T) {
	t.Setenv(midsommarSecret, "migrate-key")

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Secret{}))

	// Seed: one legacy-format secret, one new-format secret, one empty
	legacyVal := legacyCFBEncrypt(t, "migrate-key", "legacy-secret")
	require.NoError(t, db.Create(&Secret{VarName: "LEGACY", Value: legacyVal}).Error)

	newVal, err := encrypt("migrate-key", "new-secret")
	require.NoError(t, err)
	require.NoError(t, db.Create(&Secret{VarName: "FRESH", Value: newVal}).Error)

	require.NoError(t, db.Create(&Secret{VarName: "EMPTY", Value: ""}).Error)

	migrated, err := ReencryptLegacySecrets(db)
	require.NoError(t, err)
	assert.Equal(t, 1, migrated, "only the legacy-format secret should be rewritten")

	// The legacy secret is now stored in GCM format and still decrypts
	var s Secret
	require.NoError(t, db.Where("var_name = ?", "LEGACY").First(&s).Error)
	assert.True(t, strings.HasPrefix(s.Value, encVersionPrefix), "migrated value should be in v2 format, got %q", s.Value)

	got, err := GetSecretByVarName(db, "LEGACY", false)
	require.NoError(t, err)
	assert.Equal(t, "legacy-secret", got.Value)

	// The fresh secret is untouched
	var f Secret
	require.NoError(t, db.Where("var_name = ?", "FRESH").First(&f).Error)
	assert.Equal(t, newVal, f.Value)
}
