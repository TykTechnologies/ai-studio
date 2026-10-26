package secrets

import (
	"crypto/fips140"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// v3 (PBKDF2-HMAC-SHA256 + AES-GCM) is written in FIPS 140-3 mode; every
// build reads v3, v2 and legacy.

// v3Fixtures were produced by encrypt with key "fixture-key" in FIPS mode
// (1000 iterations, as test binaries write).
var v3Fixtures = map[string]string{
	"fips-fixture-plaintext": "v3:1000:X1r5gz80lrcFPcw0rHa7D54BD2EHpkHVkqKln32MIxsPLPaEgycNql_XwjXxSv7JHMod26PJ-r-7pK2oFV4WHhVx",
	"":                       "v3:1000:TbBZoTQX-CXioOK7sRD6y-n5csXpRA2ej0r19SwvqTQ60nxNWke8MP0xr24=",
}

// legacyFixture is "fips-fixture-plaintext" under key "fixture-key" in the
// pre-v2 AES-CFB format.
const legacyFixture = "O2gIJ4ilRrj7MlNlst2-VomTEGDSu0B3GTp5b2cco2oJmMbRLD8="

// forceFIPS makes this package write v3 until the test ends.
func forceFIPS(t *testing.T) {
	t.Helper()
	prev := fipsEnabled
	fipsEnabled = func() bool { return true }
	t.Cleanup(func() { fipsEnabled = prev })
}

// forceStandard makes this package write v2 until the test ends, even in a
// FIPS-mode process.
func forceStandard(t *testing.T) {
	t.Helper()
	prev := fipsEnabled
	fipsEnabled = func() bool { return false }
	t.Cleanup(func() { fipsEnabled = prev })
}

func TestEveryFormatDecryptsInEveryMode(t *testing.T) {
	check := func(t *testing.T) {
		for plaintext, enc := range v3Fixtures {
			got, err := decrypt("fixture-key", enc)
			require.NoError(t, err)
			assert.Equal(t, plaintext, got)
		}
		for plaintext, enc := range v2Fixtures {
			got, err := decrypt("fixture-key", enc)
			require.NoError(t, err)
			assert.Equal(t, plaintext, got)
		}
		got, err := decrypt("fixture-key", legacyFixture)
		require.NoError(t, err)
		assert.Equal(t, "fips-fixture-plaintext", got)
	}
	t.Run("standard", func(t *testing.T) { forceStandard(t); check(t) })
	t.Run("fips", func(t *testing.T) { forceFIPS(t); check(t) })
}

func TestEncrypt_WriterFollowsMode(t *testing.T) {
	t.Run("standard writes v2", func(t *testing.T) {
		forceStandard(t)
		enc, err := encrypt("k", "value")
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(enc, encVersionPrefix), enc)
	})
	t.Run("fips writes v3", func(t *testing.T) {
		forceFIPS(t)
		enc, err := encrypt("k", "value")
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(enc, "v3:1000:"), "test binaries write 1000 iterations, got %q", enc)
		got, err := decrypt("k", enc)
		require.NoError(t, err)
		assert.Equal(t, "value", got)

		_, err = decrypt("wrong", enc)
		assert.Error(t, err)
	})
}

func TestV3_MalformedHeaderIsAnError(t *testing.T) {
	for _, v := range []string{
		"v3:",
		"v3:abc:AAAA",
		"v3:0:AAAA",
		"v3:99999999999:AAAA",
		"v3:1000",
		"v3:1000:!!!",
		"v3:1000:AAAA", // too short
	} {
		_, err := decrypt("fixture-key", v)
		assert.Error(t, err, v)
	}
}

func TestHasValue_V3(t *testing.T) {
	assert.True(t, (&Secret{Value: v3Fixtures["fips-fixture-plaintext"]}).HasValue())
	assert.False(t, (&Secret{Value: v3Fixtures[""]}).HasValue(), "an encrypted empty string is empty")
	assert.True(t, (&Secret{Value: "v3:junk"}).HasValue(), "not something we wrote: the text is the value")
}

func openSweepDB(t *testing.T) *gorm.DB {
	t.Helper()
	if dsn := os.Getenv("SECRETS_TEST_POSTGRES_DSN"); dsn != "" {
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.Migrator().DropTable(&Secret{}))
		require.NoError(t, db.AutoMigrate(&Secret{}))
		t.Cleanup(func() { _ = db.Migrator().DropTable(&Secret{}) })
		return db
	}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Secret{}))
	return db
}

func storedValue(t *testing.T, db *gorm.DB, name string) string {
	t.Helper()
	var s Secret
	require.NoError(t, db.Where("var_name = ?", name).First(&s).Error)
	return s.Value
}

// In FIPS mode the sweep moves legacy and v2 rows to v3, in batches, and a
// second run changes nothing. Runs on Postgres too with
// SECRETS_TEST_POSTGRES_DSN set.
func TestReencrypt_FIPSModeMovesLegacyAndV2ToV3(t *testing.T) {
	t.Setenv(midsommarSecret, "fixture-key")
	db := openSweepDB(t)

	orig := reencryptBatchSize
	reencryptBatchSize = 3
	t.Cleanup(func() { reencryptBatchSize = orig })

	want := map[string]string{}
	for i := 0; i < 4; i++ {
		forceStandard(t)
		v2, err := encrypt("fixture-key", fmt.Sprintf("v2-value-%d", i))
		require.NoError(t, err)
		require.NoError(t, db.Create(&Secret{VarName: fmt.Sprintf("V2_%d", i), Value: v2}).Error)
		want[fmt.Sprintf("V2_%d", i)] = fmt.Sprintf("v2-value-%d", i)

		legacy := legacyCFBEncrypt(t, "fixture-key", fmt.Sprintf("legacy-value-%d", i))
		require.NoError(t, db.Create(&Secret{VarName: fmt.Sprintf("LEGACY_%d", i), Value: legacy}).Error)
		want[fmt.Sprintf("LEGACY_%d", i)] = fmt.Sprintf("legacy-value-%d", i)
	}
	require.NoError(t, db.Create(&Secret{VarName: "V3", Value: v3Fixtures["fips-fixture-plaintext"]}).Error)
	require.NoError(t, db.Create(&Secret{VarName: "EMPTY", Value: ""}).Error)
	require.NoError(t, db.Create(&Secret{VarName: "OTHER_KEY", Value: legacyCFBEncrypt(t, "another-key", "x")}).Error)

	forceFIPS(t)
	migrated, err := ReencryptLegacySecrets(db)
	require.NoError(t, err)
	// The row under another key decrypts to garbage under CFB (no
	// authentication), so it is migrated too, as before; it stays garbage.
	assert.Equal(t, 9, migrated)

	for name, plaintext := range want {
		v := storedValue(t, db, name)
		assert.True(t, strings.HasPrefix(v, encV3Prefix), "%s: %q", name, v)
		got, err := GetSecretByVarName(db, name, false)
		require.NoError(t, err)
		assert.Equal(t, plaintext, got.Value, name)
	}
	assert.Equal(t, v3Fixtures["fips-fixture-plaintext"], storedValue(t, db, "V3"), "a v3 row is left alone")
	assert.Equal(t, "", storedValue(t, db, "EMPTY"))

	again, err := ReencryptLegacySecrets(db)
	require.NoError(t, err)
	assert.Zero(t, again, "idempotent")
}

// A standard build moves only legacy rows (to v2) and leaves v3 rows alone,
// and reads them: a database can go back from FIPS to standard.
func TestReencrypt_StandardModeLeavesV3(t *testing.T) {
	t.Setenv(midsommarSecret, "fixture-key")
	db := openSweepDB(t)
	forceStandard(t)

	require.NoError(t, db.Create(&Secret{VarName: "V3", Value: v3Fixtures["fips-fixture-plaintext"]}).Error)
	require.NoError(t, db.Create(&Secret{VarName: "V2", Value: v2Fixtures["fips-fixture-plaintext"]}).Error)
	require.NoError(t, db.Create(&Secret{VarName: "LEGACY", Value: legacyFixture}).Error)

	migrated, err := ReencryptLegacySecrets(db)
	require.NoError(t, err)
	assert.Equal(t, 1, migrated)
	assert.True(t, strings.HasPrefix(storedValue(t, db, "LEGACY"), encVersionPrefix))
	assert.Equal(t, v3Fixtures["fips-fixture-plaintext"], storedValue(t, db, "V3"))
	assert.Equal(t, v2Fixtures["fips-fixture-plaintext"], storedValue(t, db, "V2"))

	got, err := GetSecretByVarName(db, "V3", false)
	require.NoError(t, err)
	assert.Equal(t, "fips-fixture-plaintext", got.Value)
}

func TestEncryptValue_FIPSModeWritesV3(t *testing.T) {
	t.Setenv(midsommarSecret, "env-key")
	forceFIPS(t)
	enc := EncryptValue("payload")
	assert.True(t, strings.HasPrefix(enc, "$ENC/"+encV3Prefix), enc)
	assert.Equal(t, "payload", DecryptValue(enc))
	if !fips140.Enabled() {
		forceStandard(t)
		assert.Equal(t, "payload", DecryptValue(enc), "a standard build reads v3")
	}
}
