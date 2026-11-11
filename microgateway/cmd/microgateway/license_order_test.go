package main

import (
	"crypto/sha256"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/licensing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const licenseOrderHelperEnv = "MGW_TEST_LICENSE_ORDER_HELPER"

// refusingLicense is a licensing service whose license is not valid, as an
// Enterprise microgateway's is when the key is missing, expired or not for
// it. (The Enterprise service exits the process itself on that; returning
// the error is the other way Start can refuse.)
type refusingLicense struct{ licensing.Service }

func (refusingLicense) Start() error { return errors.New("license expired") }

type acceptingLicense struct{ licensing.Service }

func (acceptingLicense) Start() error { return nil }
func (acceptingLicense) Stop()        {}

// TestLicenseThenDatabaseHelper is not a test: it is the body of the child
// process TestLicenseIsValidatedBeforeTheDatabaseIsOpened runs. It starts a
// microgateway the way main does, up to the open database.
func TestLicenseThenDatabaseHelper(t *testing.T) {
	if os.Getenv(licenseOrderHelperEnv) != "1" {
		t.Skip("runs only as the child process of TestLicenseIsValidatedBeforeTheDatabaseIsOpened")
	}
	cfg := &config.Config{Database: config.DatabaseConfig{
		Type: "sqlite", DSN: "file:" + os.Getenv("MGW_TEST_LICENSE_DB") + "?mode=rwc", AutoMigrate: true,
		MaxOpenConns: 1, MaxIdleConns: 1,
	}}
	newLicensing := func(licensing.Config) licensing.Service { return acceptingLicense{} }
	if os.Getenv("MGW_TEST_LICENSE") == "refused" {
		newLicensing = func(licensing.Config) licensing.Service { return refusingLicense{} }
	}
	_, db := licenseThenDatabase(cfg, os.Getenv("MGW_TEST_MIGRATE_ONLY") == "1", newLicensing) // exits when the license is refused
	require.NoError(t, database.Close(db))
	os.Stdout.WriteString("STARTED\n")
}

// A microgateway whose license is refused exits before it opens its
// database: no file is created for a new one, and an existing one (here a
// database of an earlier version, with an API token stored as it was) is
// byte for byte what it was, so that version can be started on it again.
// With an accepted license the same start creates and migrates the
// database. The start-up runs in a child process, through the function
// main calls, so the exit is the real one.
func TestLicenseIsValidatedBeforeTheDatabaseIsOpened(t *testing.T) {
	if testing.Short() {
		t.Skip("runs child processes")
	}
	dir := t.TempDir()
	run := func(dbPath, license string, migrateOnly bool) (exitCode int, output string) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestLicenseThenDatabaseHelper$", "-test.v")
		cmd.Env = append(os.Environ(), licenseOrderHelperEnv+"=1", "MGW_TEST_LICENSE_DB="+dbPath, "MGW_TEST_LICENSE="+license)
		if migrateOnly {
			cmd.Env = append(cmd.Env, "MGW_TEST_MIGRATE_ONLY=1")
		}
		out, err := cmd.CombinedOutput()
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode(), string(out)
		}
		require.NoError(t, err, "%s", out)
		return 0, string(out)
	}
	digest := func(path string) [32]byte {
		b, err := os.ReadFile(path)
		require.NoError(t, err)
		return sha256.Sum256(b)
	}

	// An earlier version's database: its api_tokens table has no token_hash
	// column and holds the token itself.
	old := filepath.Join(dir, "old.db")
	seed, err := database.Connect(database.DatabaseConfig{Type: "sqlite", DSN: "file:" + old + "?mode=rwc", MaxOpenConns: 1, MaxIdleConns: 1})
	require.NoError(t, err)
	require.NoError(t, seed.Exec(`CREATE TABLE api_tokens (id integer PRIMARY KEY, token text NOT NULL UNIQUE, name text, app_id integer, is_active numeric)`).Error)
	require.NoError(t, seed.Exec(`INSERT INTO api_tokens (id, token, name, app_id, is_active) VALUES (1, 'the-token-as-2.2-stored-it', 'ci', 1, 1)`).Error)
	require.NoError(t, database.Close(seed))
	before := digest(old)

	t.Run("refused license, existing database", func(t *testing.T) {
		code, out := run(old, "refused", false)
		assert.Equal(t, 1, code, out)
		assert.Contains(t, out, "License validation failed")
		assert.NotContains(t, out, "Running database migrations")
		assert.NotContains(t, out, "STARTED")
		assert.Equal(t, before, digest(old), "the database is what the earlier version left")
	})

	t.Run("refused license, no database yet", func(t *testing.T) {
		fresh := filepath.Join(dir, "fresh.db")
		code, out := run(fresh, "refused", false)
		assert.Equal(t, 1, code, out)
		assert.NoFileExists(t, fresh, "not even created")
	})

	t.Run("refused license, -migrate", func(t *testing.T) {
		code, out := run(old, "refused", true)
		assert.Equal(t, 1, code, out)
		assert.Equal(t, before, digest(old), "the migration alone needs the license too")
	})

	t.Run("accepted license", func(t *testing.T) {
		code, out := run(old, "accepted", false)
		assert.Equal(t, 0, code, out)
		assert.Contains(t, out, "Running database migrations")
		assert.Contains(t, out, "STARTED")
		assert.NotEqual(t, before, digest(old), "migrated")
	})
}
