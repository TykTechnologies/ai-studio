package fipsmigration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/pkg/fipstest"
	"github.com/TykTechnologies/midsommar/v2/pkg/pwhash"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCrossBuildMigration runs three phases against one edge database, on
// SQLite and, with MGW_TEST_POSTGRES_DSN, on PostgreSQL: a standard build
// stores an encrypted LLM API key and a bcrypt credential secret hash; a
// FIPS build (fips140=on, and separately fips140=only) reads both and
// writes new ones (PBKDF2 hash); a standard build reads everything again.
// It skips under -short (it builds two test binaries).
func TestCrossBuildMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a standard and a FIPS test binary; not in -short runs")
	}
	if os.Getenv(fipstest.PhaseEnv) != "" {
		t.Skip("inside a phase child")
	}
	bins := fipstest.BuildTestBinaries(t, ".")
	for _, mode := range []fipstest.Mode{fipstest.FIPSOn, fipstest.FIPSStrict} {
		t.Run("sqlite/fips140="+string(mode), func(t *testing.T) {
			runPhases(t, bins, mode, "sqlite", filepath.Join(t.TempDir(), "edge.db"))
		})
		t.Run("postgres/fips140="+string(mode), func(t *testing.T) {
			runPhases(t, bins, mode, "postgres", postgresSchema(t))
		})
	}
}

const (
	encryptionKey = "0123456789abcdef0123456789abcdef" // MICROGATEWAY_ENCRYPTION_KEY: 32 bytes
	apiKey        = "sk-cross-build-api-key"
	credSecret    = "cross-build-credential-secret"

	envType = "FIPS_MIGRATION_DB_TYPE"
	envDSN  = "FIPS_MIGRATION_DSN"
)

func runPhases(t *testing.T, bins fipstest.Binaries, mode fipstest.Mode, dbType, dsn string) {
	env := []string{envType + "=" + dbType, envDSN + "=" + dsn}
	fipstest.RunPhase(t, bins, fipstest.Standard, "TestPhaseSeedStandard", env...)
	fipstest.RunPhase(t, bins, mode, "TestPhaseOpenInFIPS", env...)
	fipstest.RunPhase(t, bins, fipstest.Standard, "TestPhaseBackToStandard", env...)
}

// postgresSchema returns an MGW_TEST_POSTGRES_DSN DSN for a fresh schema,
// dropped at the end of t, or skips t without MGW_TEST_POSTGRES_DSN.
func postgresSchema(t *testing.T) string {
	url := os.Getenv("MGW_TEST_POSTGRES_DSN")
	if url == "" {
		t.Skip("MGW_TEST_POSTGRES_DSN not set - skipping PostgreSQL")
	}
	admin, err := database.Connect(database.DatabaseConfig{Type: "postgres", DSN: url, LogLevel: "silent"})
	require.NoError(t, err)
	schema := fmt.Sprintf("fipsmig_%d", time.Now().UnixNano())
	require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() {
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		_ = database.Close(admin)
	})
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	return url + sep + "search_path=" + schema
}

func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.Connect(database.DatabaseConfig{Type: os.Getenv(envType), DSN: os.Getenv(envDSN), LogLevel: "silent"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close(db) })
	require.NoError(t, database.Migrate(db))
	return db
}

func seed(t *testing.T, db *gorm.DB, crypto services.CryptoServiceInterface, name string) {
	t.Helper()
	enc, err := crypto.Encrypt(apiKey)
	require.NoError(t, err)
	llm := &database.LLM{Name: name, Slug: name, Vendor: "openai", APIKeyEncrypted: enc}
	require.NoError(t, db.Create(llm).Error)
	app := &database.App{Name: name, IsActive: true}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, db.Create(&database.Credential{AppID: app.ID, KeyID: name, SecretHash: crypto.HashSecret(credSecret), IsActive: true}).Error)
}

func check(t *testing.T, db *gorm.DB, crypto services.CryptoServiceInterface, name string) string {
	t.Helper()
	var llm database.LLM
	require.NoError(t, db.Where("slug = ?", name).First(&llm).Error)
	got, err := crypto.Decrypt(llm.APIKeyEncrypted)
	require.NoError(t, err, "%s: decrypt API key", name)
	assert.Equal(t, apiKey, got, name)
	var cred database.Credential
	require.NoError(t, db.Where("key_id = ?", name).First(&cred).Error)
	assert.True(t, crypto.VerifySecret(credSecret, cred.SecretHash), "%s: credential secret verifies", name)
	assert.False(t, crypto.VerifySecret("wrong", cred.SecretHash), "%s: a wrong secret fails", name)
	return cred.SecretHash
}

func TestPhaseSeedStandard(t *testing.T) {
	fipstest.Phase(t)
	db := openDB(t)
	crypto := services.NewCryptoService(encryptionKey)
	seed(t, db, crypto, "std")
	assert.True(t, strings.HasPrefix(check(t, db, crypto, "std"), "bcrypt:"), "standard builds hash with bcrypt")
}

func TestPhaseOpenInFIPS(t *testing.T) {
	fipstest.Phase(t)
	db := openDB(t)
	crypto := services.NewCryptoService(encryptionKey)
	check(t, db, crypto, "std")
	seed(t, db, crypto, "fips")
	assert.True(t, pwhash.IsPBKDF2(check(t, db, crypto, "fips")), "FIPS builds hash with PBKDF2")
}

func TestPhaseBackToStandard(t *testing.T) {
	fipstest.Phase(t)
	db := openDB(t)
	crypto := services.NewCryptoService(encryptionKey)
	check(t, db, crypto, "std")
	assert.True(t, pwhash.IsPBKDF2(check(t, db, crypto, "fips")))
}
