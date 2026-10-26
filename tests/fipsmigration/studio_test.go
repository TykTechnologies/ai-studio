// Package fipsmigration checks that a database moves between standard and
// FIPS 140-3 builds of AI Studio in both directions (epic
// midsommar-clean-8dy7, bead .19).
//
// TestCrossBuildMigration compiles this package twice, as a standard build
// and as a GOFIPS140=v1.0.0 build (pkg/fipstest.BuildTestBinaries), and runs
// three phases against one database, on SQLite and, with DATABASE_URL, on
// PostgreSQL:
//
//  1. standard build: seeds what a standard build writes (a bcrypt user
//     password, a bcrypt OAuth client secret, a v2 secret) plus a pre-v2
//     AES-CFB secret;
//  2. FIPS build (fips140=on, and separately fips140=only): the start-up
//     re-encryption sweep moves both secrets to v3, logins and the client
//     secret verify and are rewritten as PBKDF2, new values are written in
//     the approved formats;
//  3. standard build again: everything the FIPS build wrote still works.
//
// It skips under -short (it builds two test binaries).
package fipsmigration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/fipstest"
	"github.com/TykTechnologies/midsommar/v2/pkg/pwhash"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	encryptionKey = "fixture-key"
	password      = "correct horse"
	legacyEmail   = "legacy@example.com"
	fipsEmail     = "fips@example.com"

	// legacyCFB is "fips-fixture-plaintext" under encryptionKey in the
	// pre-v2 AES-CFB format (pinned in secrets/v3_test.go; no build writes
	// it any more).
	legacyCFB       = "O2gIJ4ilRrj7MlNlst2-VomTEGDSu0B3GTp5b2cco2oJmMbRLD8="
	legacyPlaintext = "fips-fixture-plaintext"

	// Environment of the phase children.
	envDialect = "FIPS_MIGRATION_DIALECT"
	envDSN     = "FIPS_MIGRATION_DSN"
	envState   = "FIPS_MIGRATION_STATE"
)

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
			dir := t.TempDir()
			runPhases(t, bins, mode, "sqlite", filepath.Join(dir, "studio.db"), dir)
		})
		t.Run("postgres/fips140="+string(mode), func(t *testing.T) {
			dsn := postgresSchema(t)
			runPhases(t, bins, mode, "postgres", dsn, t.TempDir())
		})
	}
}

func runPhases(t *testing.T, bins fipstest.Binaries, mode fipstest.Mode, dialect, dsn, stateDir string) {
	env := []string{
		envDialect + "=" + dialect,
		envDSN + "=" + dsn,
		envState + "=" + stateDir,
		"TYK_AI_SECRET_KEY=" + encryptionKey,
	}
	fipstest.RunPhase(t, bins, fipstest.Standard, "TestPhaseSeedStandard", env...)
	fipstest.RunPhase(t, bins, mode, "TestPhaseOpenInFIPS", env...)
	fipstest.RunPhase(t, bins, fipstest.Standard, "TestPhaseBackToStandard", env...)
}

// postgresSchema returns a DATABASE_URL DSN for a fresh schema, dropped at
// the end of t, or skips t without DATABASE_URL.
func postgresSchema(t *testing.T) string {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set - skipping PostgreSQL")
	}
	admin, err := gorm.Open(postgres.Open(url), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	schema := fmt.Sprintf("fipsmig_%d", time.Now().UnixNano())
	require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() {
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		if db, err := admin.DB(); err == nil {
			db.Close()
		}
	})
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	return url + sep + "search_path=" + schema
}

// openDB opens the phase's database and migrates it, as Studio does at
// start-up.
func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	var dialector gorm.Dialector
	switch os.Getenv(envDialect) {
	case "sqlite":
		dialector = sqlite.Open(os.Getenv(envDSN))
	case "postgres":
		dialector = postgres.Open(os.Getenv(envDSN))
	default:
		t.Fatalf("%s=%q", envDialect, os.Getenv(envDialect))
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	require.NoError(t, models.InitModels(db))
	secrets.SetDBRef(db)
	return db
}

// The OAuth client's id and secret travel between phases in a file.
func saveClient(t *testing.T, id, secret string) {
	require.NoError(t, os.WriteFile(filepath.Join(os.Getenv(envState), "client"), []byte(id+"\n"+secret), 0o600))
}

func loadClient(t *testing.T) (id, secret string) {
	b, err := os.ReadFile(filepath.Join(os.Getenv(envState), "client"))
	require.NoError(t, err)
	parts := strings.SplitN(string(b), "\n", 2)
	require.Len(t, parts, 2)
	return parts[0], parts[1]
}

func storedPassword(t *testing.T, db *gorm.DB, email string) string {
	var u models.User
	require.NoError(t, db.Where("email = ?", email).First(&u).Error)
	return u.Password
}

func storedSecret(t *testing.T, db *gorm.DB, name string) string {
	s, err := secrets.GetSecretByVarName(db, name, true)
	require.NoError(t, err)
	return s.Value
}

func secretValue(t *testing.T, db *gorm.DB, name string) string {
	s, err := secrets.GetSecretByVarName(db, name, false)
	require.NoError(t, err, "reading secret %s", name)
	return s.Value
}

func createUser(t *testing.T, db *gorm.DB, email string) {
	user := models.NewUser()
	user.Email = email
	user.EmailVerified = true
	require.NoError(t, user.SetPassword(password))
	require.NoError(t, user.Create(db))
}

func login(t *testing.T, svc *services.Service, email string) {
	t.Helper()
	_, err := svc.AuthenticateUser(email, "wrong horse")
	assert.Error(t, err, "%s: a wrong password must fail", email)
	_, err = svc.AuthenticateUser(email, password)
	require.NoError(t, err, "%s: login", email)
}

func validateClient(t *testing.T, db *gorm.DB) {
	t.Helper()
	id, secret := loadClient(t)
	oauth := services.NewOAuthClientService(db)
	client, err := oauth.GetClient(id)
	require.NoError(t, err)
	ok, err := oauth.ValidateClientSecret(client, "not the secret")
	require.NoError(t, err)
	assert.False(t, ok, "a wrong client secret must fail")
	ok, err = oauth.ValidateClientSecret(client, secret)
	require.NoError(t, err)
	require.True(t, ok, "client secret")
}

func storedClientSecret(t *testing.T, db *gorm.DB) string {
	id, _ := loadClient(t)
	var c models.OAuthClient
	require.NoError(t, db.Where("client_id = ?", id).First(&c).Error)
	return c.ClientSecret
}

// Phase 1: what a standard build writes.
func TestPhaseSeedStandard(t *testing.T) {
	fipstest.Phase(t)
	db := openDB(t)

	createUser(t, db, legacyEmail)
	assert.True(t, strings.HasPrefix(storedPassword(t, db, legacyEmail), "$2"), "standard builds hash passwords with bcrypt")

	client, secret, err := services.NewOAuthClientService(db).CreateClient("legacy", []string{"http://localhost/cb"}, nil, "mcp")
	require.NoError(t, err)
	saveClient(t, client.ClientID, secret)
	assert.True(t, strings.HasPrefix(storedClientSecret(t, db), "$2"), "standard builds hash client secrets with bcrypt")

	require.NoError(t, secrets.CreateSecret(db, &secrets.Secret{VarName: "V2", Value: "v2-plaintext"}))
	assert.True(t, strings.HasPrefix(storedSecret(t, db, "V2"), "v2:"), "standard builds write v2 secrets")
	require.NoError(t, db.Create(&secrets.Secret{VarName: "LEGACY", Value: legacyCFB}).Error)

	login(t, services.NewService(db), legacyEmail)
	validateClient(t, db)
	assert.Equal(t, "v2-plaintext", secretValue(t, db, "V2"))
	assert.Equal(t, legacyPlaintext, secretValue(t, db, "LEGACY"))
}

// Phase 2: the same database opened by a FIPS build.
func TestPhaseOpenInFIPS(t *testing.T) {
	fipstest.Phase(t)
	db := openDB(t)

	// The start-up sweep (pkg/studio) moves legacy and v2 secrets to v3.
	migrated, err := secrets.ReencryptLegacySecrets(db)
	require.NoError(t, err)
	assert.Equal(t, 2, migrated)
	for _, name := range []string{"V2", "LEGACY"} {
		assert.True(t, strings.HasPrefix(storedSecret(t, db, name), "v3:"), "%s re-encrypted to v3", name)
	}
	assert.Equal(t, "v2-plaintext", secretValue(t, db, "V2"))
	assert.Equal(t, legacyPlaintext, secretValue(t, db, "LEGACY"))

	// A bcrypt password verifies and is rewritten as PBKDF2.
	svc := services.NewService(db)
	login(t, svc, legacyEmail)
	upgraded := storedPassword(t, db, legacyEmail)
	assert.True(t, pwhash.IsPBKDF2(upgraded), "password upgraded on login, got %q", upgraded)
	login(t, svc, legacyEmail)

	// So is the OAuth client secret, on its first successful use.
	validateClient(t, db)
	assert.True(t, pwhash.IsPBKDF2(storedClientSecret(t, db)), "client secret upgraded on use")
	validateClient(t, db)

	// New values are written in the approved formats.
	require.NoError(t, secrets.CreateSecret(db, &secrets.Secret{VarName: "V3_NEW", Value: "written-in-fips"}))
	assert.True(t, strings.HasPrefix(storedSecret(t, db, "V3_NEW"), "v3:"))
	createUser(t, db, fipsEmail)
	assert.True(t, pwhash.IsPBKDF2(storedPassword(t, db, fipsEmail)))
	login(t, svc, fipsEmail)
}

// Phase 3: back on a standard build, everything the FIPS build wrote works,
// and nothing is rewritten to the standard formats.
func TestPhaseBackToStandard(t *testing.T) {
	fipstest.Phase(t)
	db := openDB(t)

	migrated, err := secrets.ReencryptLegacySecrets(db)
	require.NoError(t, err)
	assert.Equal(t, 0, migrated, "a standard build leaves v3 secrets alone")
	for name, want := range map[string]string{"V2": "v2-plaintext", "LEGACY": legacyPlaintext, "V3_NEW": "written-in-fips"} {
		assert.True(t, strings.HasPrefix(storedSecret(t, db, name), "v3:"), name)
		assert.Equal(t, want, secretValue(t, db, name), name)
	}

	svc := services.NewService(db)
	for _, email := range []string{legacyEmail, fipsEmail} {
		before := storedPassword(t, db, email)
		login(t, svc, email)
		assert.Equal(t, before, storedPassword(t, db, email), "%s: a standard build keeps the PBKDF2 hash", email)
	}
	before := storedClientSecret(t, db)
	validateClient(t, db)
	assert.Equal(t, before, storedClientSecret(t, db))

	// New values are standard formats again, alongside the FIPS ones.
	require.NoError(t, secrets.CreateSecret(db, &secrets.Secret{VarName: "V2_AGAIN", Value: "standard-again"}))
	assert.True(t, strings.HasPrefix(storedSecret(t, db, "V2_AGAIN"), "v2:"))
	assert.Equal(t, "standard-again", secretValue(t, db, "V2_AGAIN"))
}
