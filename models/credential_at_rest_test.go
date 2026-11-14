package models

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/rs/zerolog"
)

const credentialTestKey = "credentials-unit-test-key"

// storedCredential is a credential row as the database holds it.
type storedCredential struct {
	ID         uint
	Secret     string
	SecretHash *string
}

// openStored opens a stored secret as the credential keyID's.
func openStored(t *testing.T, db *gorm.DB, keyID, stored string) string {
	t.Helper()
	plain, ok := openCredentialSecret(db, keyID, stored)
	require.True(t, ok, "the stored secret does not open")
	return plain
}

func readStored(t *testing.T, db *gorm.DB, id uint) storedCredential {
	t.Helper()
	var row storedCredential
	require.NoError(t, db.Table("credentials").Select("id", "secret", "secret_hash").Where("id = ?", id).Scan(&row).Error)
	return row
}

// forEachCredentialDB runs body on SQLite and, with DATABASE_URL, on
// Postgres in a schema of its own (these tests count rows and remove the
// database's salt, which must not touch what other tests left behind).
func forEachCredentialDB(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "c.db")), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, migrateTestSchema(db))
		body(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		base := os.Getenv("DATABASE_URL")
		if !strings.HasPrefix(base, "postgres") {
			t.Skip("DATABASE_URL is not a Postgres URL")
		}
		admin, err := gorm.Open(postgres.Open(base), &gorm.Config{})
		require.NoError(t, err)
		schema := fmt.Sprintf("credential_at_rest_%d", time.Now().UnixNano())
		require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
		sep := "?"
		if strings.Contains(base, "?") {
			sep = "&"
		}
		db, err := gorm.Open(postgres.Open(base+sep+"search_path="+schema), &gorm.Config{})
		require.NoError(t, err)
		t.Cleanup(func() {
			if pool, err := db.DB(); err == nil {
				pool.Close()
			}
			admin.Exec("DROP SCHEMA " + schema + " CASCADE")
			if pool, err := admin.DB(); err == nil {
				pool.Close()
			}
		})
		require.NoError(t, InitModels(db))
		body(t, db)
	})
}

// keyID names a test credential (unique per test and database).
func keyID(t *testing.T, name string) string {
	return "at-rest-" + strings.NewReplacer("/", "-", " ", "-").Replace(t.Name()) + "-" + name
}

// With TYK_AI_SECRET_KEY the secret is stored encrypted with a keyed lookup
// hash, and every reader sees the plaintext: the struct after a save, a
// find, a preload through the App, and a lookup by the secret.
func TestCredentialAtRest_Postgres(t *testing.T) {
	forEachCredentialDB(t, func(t *testing.T, db *gorm.DB) {
		t.Setenv("TYK_AI_SECRET_KEY", credentialTestKey)

		cred := &Credential{KeyID: keyID(t, "a"), Secret: "the-app-key-" + keyID(t, "a"), Active: true}
		plain := cred.Secret
		require.NoError(t, cred.Create(db))
		assert.Equal(t, plain, cred.Secret, "the caller keeps the plaintext")

		row := readStored(t, db, cred.ID)
		assert.True(t, strings.HasPrefix(row.Secret, secrets.DataKeyPrefix), "stored encrypted: %q", row.Secret)
		assert.NotContains(t, row.Secret, plain)
		require.NotNil(t, row.SecretHash)
		assert.True(t, strings.HasPrefix(*row.SecretHash, secrets.LookupHashKeyedPrefix))
		assert.NotContains(t, *row.SecretHash, plain)
		assert.LessOrEqual(t, len(*row.SecretHash), secrets.LookupHashMaxLen)

		var byID Credential
		require.NoError(t, byID.Get(db, cred.ID))
		assert.Equal(t, plain, byID.Secret)

		var bySecret Credential
		require.NoError(t, bySecret.GetBySecret(db, plain))
		assert.Equal(t, cred.ID, bySecret.ID)
		assert.Equal(t, plain, bySecret.Secret)
		assert.ErrorIs(t, (&Credential{}).GetBySecret(db, plain+"x"), gorm.ErrRecordNotFound)
		assert.ErrorIs(t, (&Credential{}).GetBySecret(db, ""), gorm.ErrRecordNotFound)
		assert.ErrorIs(t, (&Credential{}).GetBySecret(db, row.Secret), gorm.ErrRecordNotFound, "the ciphertext is not the key")
		assert.ErrorIs(t, (&Credential{}).GetBySecret(db, *row.SecretHash), gorm.ErrRecordNotFound, "nor is the hash")

		// Through the App, as the API, the decision call and the
		// management server read it.
		owner := User{Email: keyID(t, "owner") + "@example.com", Name: "Owner"}
		require.NoError(t, db.Create(&owner).Error)
		t.Cleanup(func() { db.Unscoped().Delete(&owner) })
		app := App{Name: keyID(t, "app"), UserID: owner.ID, CredentialID: cred.ID}
		require.NoError(t, db.Create(&app).Error)
		var loaded App
		require.NoError(t, db.Preload("Credential").First(&loaded, app.ID).Error)
		assert.Equal(t, plain, loaded.Credential.Secret)

		// Saving the App with its credential loaded neither stores the
		// plaintext nor leaves ciphertext in the struct.
		loaded.Description = "changed"
		require.NoError(t, db.Save(&loaded).Error)
		assert.Equal(t, plain, loaded.Credential.Secret)
		assert.True(t, strings.HasPrefix(readStored(t, db, cred.ID).Secret, secrets.DataKeyPrefix))

		// Activation and deactivation keep the secret and its hash.
		require.NoError(t, byID.Deactivate(db))
		assert.Equal(t, plain, byID.Secret)
		after := readStored(t, db, cred.ID)
		assert.Equal(t, *row.SecretHash, *after.SecretHash)
		require.NoError(t, (&Credential{}).GetBySecret(db, plain))

		// A new secret (a regenerated key) replaces hash and ciphertext.
		byID.Secret = "a-new-key-" + keyID(t, "a")
		require.NoError(t, byID.Update(db))
		assert.ErrorIs(t, (&Credential{}).GetBySecret(db, plain), gorm.ErrRecordNotFound)
		require.NoError(t, (&Credential{}).GetBySecret(db, byID.Secret))

		// Two credentials cannot share a secret.
		dup := &Credential{KeyID: keyID(t, "dup"), Secret: byID.Secret}
		assert.Error(t, dup.Create(db))

		// A list is decrypted too.
		var all Credentials
		require.NoError(t, db.Where("key_id LIKE ?", keyID(t, "")+"%").Find(&all).Error)
		require.Len(t, all, 1)
		assert.Equal(t, byID.Secret, all[0].Secret)
	})
}

// Without TYK_AI_SECRET_KEY the secret is stored as written, as LLM
// credentials are, and still found by a hash, an unkeyed one.
func TestCredentialAtRest_NoKey(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "")
	db := setupTestDB(t)
	cred := &Credential{KeyID: "nokey", Secret: "plain-app-key"}
	require.NoError(t, cred.Create(db))
	row := readStored(t, db, cred.ID)
	assert.Equal(t, "plain-app-key", row.Secret)
	require.NotNil(t, row.SecretHash)
	assert.True(t, strings.HasPrefix(*row.SecretHash, secrets.LookupHashPlainPrefix))
	require.NoError(t, (&Credential{}).GetBySecret(db, "plain-app-key"))

	// A key shorter than the keyed hash needs: encrypted, unkeyed hash.
	t.Setenv("TYK_AI_SECRET_KEY", "short")
	short := &Credential{KeyID: "short", Secret: "another-app-key"}
	require.NoError(t, short.Create(db))
	row = readStored(t, db, short.ID)
	assert.True(t, strings.HasPrefix(row.Secret, secrets.DataKeyPrefix))
	assert.True(t, strings.HasPrefix(*row.SecretHash, secrets.LookupHashPlainPrefix))
	require.NoError(t, (&Credential{}).GetBySecret(db, "another-app-key"))
}

// A secret that does not decrypt (the key was changed or removed) reads as
// the refused marker, never as its ciphertext, is found by nothing, and
// survives a save of its row: with the right key it is readable again.
func TestCredentialAtRest_WrongKey(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", credentialTestKey)
	db := setupTestDB(t)
	cred := &Credential{KeyID: "k", Secret: "the-app-key", Active: true}
	require.NoError(t, cred.Create(db))
	before := readStored(t, db, cred.ID)

	t.Setenv("TYK_AI_SECRET_KEY", "another-unit-test-key")
	var read Credential
	require.NoError(t, read.Get(db, cred.ID))
	assert.Equal(t, secrets.RefusedCredential, read.Secret)
	assert.ErrorIs(t, (&Credential{}).GetBySecret(db, "the-app-key"), gorm.ErrRecordNotFound)
	assert.ErrorIs(t, (&Credential{}).GetBySecret(db, secrets.RefusedCredential), gorm.ErrRecordNotFound)

	require.NoError(t, read.Deactivate(db))
	assert.Equal(t, secrets.RefusedCredential, read.Secret)
	after := readStored(t, db, cred.ID)
	assert.Equal(t, before.Secret, after.Secret, "the stored secret is kept")
	assert.Equal(t, *before.SecretHash, *after.SecretHash)

	res, err := MigrateCredentialsAtRest(db)
	require.NoError(t, err)
	assert.Equal(t, CredentialMigration{}, res, "the migration leaves it alone too: it is in its final form, for another key")
	assert.Equal(t, before.Secret, readStored(t, db, cred.ID).Secret)

	t.Setenv("TYK_AI_SECRET_KEY", credentialTestKey)
	var again Credential
	require.NoError(t, again.GetBySecret(db, "the-app-key"))
	assert.Equal(t, "the-app-key", again.Secret)
	assert.False(t, again.Active)
}

// insertLegacy writes rows as code from before the change did: the secret
// in plaintext and no hash.
func insertLegacy(t *testing.T, db *gorm.DB, keyIDs ...string) map[string]uint {
	t.Helper()
	ids := map[string]uint{}
	for _, k := range keyIDs {
		require.NoError(t, db.Exec("INSERT INTO credentials (key_id, secret, active, created_at, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", k, "legacy-secret-of-"+k, true).Error)
		var id uint
		require.NoError(t, db.Table("credentials").Select("id").Where("key_id = ?", k).Scan(&id).Error)
		ids[k] = id
	}
	return ids
}

// plaintextSecrets counts rows whose stored secret is not encrypted.
func plaintextSecrets(t *testing.T, db *gorm.DB, like string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Table("credentials").Where("key_id LIKE ? AND secret NOT LIKE ?", like, secrets.DataKeyPrefix+"%").Count(&n).Error)
	return n
}

// The migration of rows written before the change: afterwards no row has a
// plaintext secret, every key is still found by its secret, a second run
// does nothing, and a run that was interrupted is finished by the next.
func TestMigrateCredentialsAtRest_Postgres(t *testing.T) {
	forEachCredentialDB(t, func(t *testing.T, db *gorm.DB) {
		t.Setenv("TYK_AI_SECRET_KEY", credentialTestKey)
		prevBatch := credentialMigrationBatch
		credentialMigrationBatch = 3 // several batches
		t.Cleanup(func() { credentialMigrationBatch = prevBatch })

		var keys []string
		for i := 0; i < 10; i++ {
			keys = append(keys, keyID(t, fmt.Sprintf("legacy-%02d", i)))
		}
		ids := insertLegacy(t, db, keys...)
		like := keyID(t, "") + "%"
		require.Equal(t, int64(10), plaintextSecrets(t, db, like))
		assert.ErrorIs(t, (&Credential{}).GetBySecret(db, "legacy-secret-of-"+keys[0]), gorm.ErrRecordNotFound, "not found before it is migrated")

		// A soft-deleted row is migrated like the others.
		require.NoError(t, db.Delete(&Credential{}, ids[keys[9]]).Error)

		// An interrupted run: four rows done, the rest not.
		for _, k := range keys[:4] {
			plain := "legacy-secret-of-" + k
			sealed, err := sealCredentialSecret(db, k, plain)
			require.NoError(t, err)
			done, err := setCredentialAtRest(db, ids[k], plain, sealed, secrets.LookupHash(plain))
			require.NoError(t, err)
			require.True(t, done)
		}
		doneBefore := readStored(t, db, ids[keys[0]])
		require.Equal(t, int64(6), plaintextSecrets(t, db, like))

		res, err := MigrateCredentialsAtRest(db)
		require.NoError(t, err)
		assert.Equal(t, 6, res.Migrated, "the run finishes what was left")
		assert.Zero(t, res.Skipped)
		assert.Zero(t, plaintextSecrets(t, db, like), "no row has a plaintext secret")
		assert.Equal(t, doneBefore, readStored(t, db, ids[keys[0]]), "a row already done is not rewritten")

		for _, k := range keys[:9] {
			var c Credential
			require.NoError(t, c.GetBySecret(db, "legacy-secret-of-"+k), k)
			assert.Equal(t, ids[k], c.ID)
			assert.Equal(t, "legacy-secret-of-"+k, c.Secret)
			assert.True(t, c.Active)
		}
		deleted := readStored(t, db, ids[keys[9]])
		assert.True(t, strings.HasPrefix(deleted.Secret, secrets.DataKeyPrefix))
		require.NotNil(t, deleted.SecretHash)

		// A second start is a no-op.
		snapshot := readStored(t, db, ids[keys[5]])
		res, err = MigrateCredentialsAtRest(db)
		require.NoError(t, err)
		assert.Equal(t, CredentialMigration{}, res)
		assert.Equal(t, snapshot, readStored(t, db, ids[keys[5]]))

		// A row changed since it was read is not overwritten with what
		// was read.
		done, err := setCredentialAtRest(db, ids[keys[5]], "what-was-read-earlier", "stale", "stale-hash-"+keys[5])
		require.NoError(t, err)
		assert.False(t, done)
		assert.Equal(t, snapshot, readStored(t, db, ids[keys[5]]))
	})
}

// Rows written while no key was set (plaintext, unkeyed hash) are encrypted
// and rehashed once a key is set; two rows with one secret cannot both be
// found, so the later one is left without a hash and reported.
func TestMigrateCredentialsAtRest_KeySetLaterAndDuplicates(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "")
	db := setupTestDB(t)
	cred := &Credential{KeyID: "written-without-a-key", Secret: "app-key-one", Active: true}
	require.NoError(t, cred.Create(db))
	res, err := MigrateCredentialsAtRest(db)
	require.NoError(t, err)
	assert.Equal(t, CredentialMigration{}, res, "nothing to do without a key: already in its final form")

	legacy := insertLegacy(t, db, "first", "second")
	require.NoError(t, db.Exec("UPDATE credentials SET secret = 'shared-secret' WHERE key_id IN ('first', 'second')").Error)
	res, err = MigrateCredentialsAtRest(db)
	require.NoError(t, err)
	assert.Equal(t, CredentialMigration{Migrated: 1, Skipped: 1}, res)
	var found Credential
	require.NoError(t, found.GetBySecret(db, "shared-secret"))
	assert.Equal(t, legacy["first"], found.ID, "the earlier row keeps the secret")
	assert.Nil(t, readStored(t, db, legacy["second"]).SecretHash)

	t.Setenv("TYK_AI_SECRET_KEY", credentialTestKey)
	assert.ErrorIs(t, (&Credential{}).GetBySecret(db, "app-key-one"), gorm.ErrRecordNotFound, "hashed without the key: not found until migrated")
	res, err = MigrateCredentialsAtRest(db)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Migrated)
	assert.Equal(t, 1, res.Skipped, "the duplicate is still a duplicate")
	row := readStored(t, db, cred.ID)
	assert.True(t, strings.HasPrefix(row.Secret, secrets.DataKeyPrefix))
	assert.True(t, strings.HasPrefix(*row.SecretHash, secrets.LookupHashKeyedPrefix))
	require.NoError(t, (&Credential{}).GetBySecret(db, "app-key-one"))
	require.NoError(t, (&Credential{}).GetBySecret(db, "shared-secret"))
}

// The schema this change needs cannot be read by older code: it would look
// credentials up by a plaintext secret that is no longer there and show the
// ciphertext as the key. So the minimum reader version is this change's
// schema version, and an older headless control plane refuses the database
// (CheckSchemaVersion; see TestCheckSchemaVersion).
func TestCredentialAtRestRaisesMinimumReader(t *testing.T) {
	assert.GreaterOrEqual(t, MinReaderSchemaVersion, 42)
	assert.LessOrEqual(t, MinReaderSchemaVersion, SchemaVersion)
}

// Nothing writes the secret column except a whole-struct save (which runs
// the hooks) and the migration: a column or map update would store a
// plaintext secret, or leave the lookup hash behind.
func TestCredentialSecretIsOnlySavedWhole(t *testing.T) {
	// The migration is the one place that writes the columns directly.
	allowed := map[string]bool{"models/credential_at_rest.go": true}
	columnUpdate := regexp.MustCompile(`\.(Update|UpdateColumn)\(\s*"(secret|secret_hash)"`)
	mapUpdate := regexp.MustCompile(`\.(Updates|UpdateColumns)\(.*"(secret|secret_hash)"\s*:`)
	rawSQL := regexp.MustCompile(`(?i)(UPDATE\s+credentials\s+SET|INSERT\s+INTO\s+credentials)`)

	root, err := filepath.Abs("..")
	require.NoError(t, err)
	var found []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if rel != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			switch rel {
			case "third_party", "microgateway", "community", "node_modules", "ui", "docs", "data", "oci-cache", "examples", "benchmarks":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(src), "\n") {
			if !columnUpdate.MatchString(line) && !mapUpdate.MatchString(line) && !rawSQL.MatchString(line) {
				continue
			}
			if allowed[filepath.ToSlash(rel)] {
				continue
			}
			found = append(found, fmt.Sprintf("%s:%d: %s", rel, i+1, strings.TrimSpace(line)))
		}
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, found, "these write a credential's secret without its hooks:\n%s", strings.Join(found, "\n"))
}

// The read-side twin of TestCredentialSecretIsOnlySavedWhole. A credential's
// secret is opened by its AfterFind hook, which needs the database (for the
// salt). Nothing loads a Credential in a way that does not run it: a Joins
// of the association (gorm fills joined structs without their hooks), a
// session with SkipHooks, or SQL that reads the credentials table itself
// (Raw(...).Scan, Table("credentials")). Such a read gets the refused
// marker, not the key (TestCredentialReadWithoutHooksIsRefusedNotCiphertext);
// this test fails when one is written, so that it is noticed. The migration
// and the salt check read the table directly on purpose, into structs of
// their own.
func TestCredentialIsOnlyReadThroughItsHooks(t *testing.T) {
	allowed := map[string]bool{
		"models/credential_at_rest.go": true, // the migration
		"models/data_key_salt.go":      true, // counts sealed rows
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`\.(Joins|InnerJoins)\(\s*"[A-Za-z.]*Credential"`),
		regexp.MustCompile(`SkipHooks\s*:\s*true|\.SkipHooks\s*=\s*true`),
		regexp.MustCompile(`(?i)\b(FROM|JOIN)\s+["` + "`" + `]?credentials\b`),
		regexp.MustCompile(`\.Table\(\s*"credentials"`),
	}
	comment := regexp.MustCompile(`^\s*//`)

	root, err := filepath.Abs("..")
	require.NoError(t, err)
	var found []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if rel != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			switch rel {
			case "third_party", "microgateway", "community", "node_modules", "ui", "docs", "data", "oci-cache", "examples", "benchmarks", "proto":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || allowed[filepath.ToSlash(rel)] {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(src), "\n") {
			if comment.MatchString(line) {
				continue
			}
			for _, p := range patterns {
				if p.MatchString(line) {
					found = append(found, fmt.Sprintf("%s:%d: %s", rel, i+1, strings.TrimSpace(line)))
					break
				}
			}
		}
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, found, "these may load a credential without its AfterFind hook, which opens the secret:\n%s", strings.Join(found, "\n"))

	// The patterns catch what they are meant to.
	for _, line := range []string{
		`db.Joins("Credential").First(&app, id)`,
		`db.InnerJoins("App.Credential").Find(&tokens)`,
		`db.Session(&gorm.Session{SkipHooks: true}).First(&c, id)`,
		`db.Raw("SELECT * FROM credentials WHERE id = ?", id).Scan(&c)`,
		"db.Raw(`select c.* from apps a join credentials c on c.id = a.credential_id`).Scan(&c)",
		`db.Table("credentials").Where("id = ?", id).Scan(&c)`,
	} {
		matched := false
		for _, p := range patterns {
			matched = matched || p.MatchString(line)
		}
		assert.True(t, matched, "not caught: %s", line)
	}
}

// A save that fails (a database error, as during a failover) must leave the
// caller's struct as it was, so that saving it again stores the same key:
// the secret is never put in its stored form in the struct, and a struct
// that does hold a stored form (a caller that read the raw row) is hashed
// by what it decrypts to, not encrypted a second time.
func TestCredentialFailedSaveThenRetry(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", credentialTestKey)
	db := setupTestDB(t)

	failing := db.Session(&gorm.Session{})
	const failName = "test:fail_credential_write"
	fail := func(tx *gorm.DB) { _ = tx.AddError(fmt.Errorf("the database is unavailable")) }
	require.NoError(t, failing.Callback().Create().Before("gorm:create").Register(failName, fail))
	require.NoError(t, failing.Callback().Update().Before("gorm:update").Register(failName, fail))
	working := func() {
		require.NoError(t, db.Callback().Create().Remove(failName))
		require.NoError(t, db.Callback().Update().Remove(failName))
	}

	// Create fails, then succeeds with the same struct.
	cred := &Credential{KeyID: "retry", Secret: "the-key-being-saved", Active: true}
	require.Error(t, cred.Create(failing))
	assert.Equal(t, "the-key-being-saved", cred.Secret, "a failed create leaves the plaintext in the struct")
	working()
	require.NoError(t, cred.Create(db))
	assert.Equal(t, "the-key-being-saved", cred.Secret)
	row := readStored(t, db, cred.ID)
	assert.Equal(t, "the-key-being-saved", openStored(t, db, "retry", row.Secret), "stored once encrypted, not twice")
	require.NoError(t, (&Credential{}).GetBySecret(db, "the-key-being-saved"))

	// Update fails, then succeeds.
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(failName, fail))
	cred.Secret = "a-regenerated-key"
	require.Error(t, cred.Update(db))
	assert.Equal(t, "a-regenerated-key", cred.Secret, "a failed update leaves the plaintext in the struct")
	assert.Equal(t, "the-key-being-saved", openStored(t, db, "retry", readStored(t, db, cred.ID).Secret), "nothing was written")
	require.NoError(t, db.Callback().Update().Remove(failName))
	require.NoError(t, cred.Update(db))
	assert.Equal(t, "a-regenerated-key", cred.Secret)
	require.NoError(t, (&Credential{}).GetBySecret(db, "a-regenerated-key"))
	assert.ErrorIs(t, (&Credential{}).GetBySecret(db, "the-key-being-saved"), gorm.ErrRecordNotFound)
	assert.Equal(t, "a-regenerated-key", openStored(t, db, "retry", readStored(t, db, cred.ID).Secret))

	// A struct holding the stored form is saved as that key, not as its
	// ciphertext.
	raw := readStored(t, db, cred.ID)
	held := &Credential{KeyID: "retry"}
	require.NoError(t, held.Get(db, cred.ID))
	held.Secret = raw.Secret
	require.NoError(t, held.Update(db))
	after := readStored(t, db, cred.ID)
	assert.Equal(t, "a-regenerated-key", openStored(t, db, "retry", after.Secret))
	assert.Equal(t, *raw.SecretHash, *after.SecretHash)
	require.NoError(t, (&Credential{}).GetBySecret(db, "a-regenerated-key"))
}

// Of two rows with one secret, the one that validated before the change
// keeps doing so: the first by id among the live, active ones, as the
// control server's lookup found it (it skipped inactive and soft-deleted
// rows). The other is left without a hash.
func TestMigrateCredentialsAtRest_DuplicateKeepsTheRowThatValidated(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", credentialTestKey)
	for _, tc := range []struct {
		name          string
		first, second string // "active", "inactive" or "deleted"
		winner        string
	}{
		{"both active: the lower id", "active", "active", "first"},
		{"the lower id is inactive", "inactive", "active", "second"},
		{"the lower id is soft-deleted", "deleted", "active", "second"},
		{"the higher id is inactive", "active", "inactive", "first"},
		{"neither validated: the lower id", "inactive", "deleted", "first"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTestDB(t)
			ids := insertLegacy(t, db, "first", "second")
			require.NoError(t, db.Exec("UPDATE credentials SET secret = 'one-shared-secret'").Error)
			for key, state := range map[string]string{"first": tc.first, "second": tc.second} {
				switch state {
				case "inactive":
					require.NoError(t, db.Exec("UPDATE credentials SET active = ? WHERE id = ?", false, ids[key]).Error)
				case "deleted":
					require.NoError(t, db.Exec("UPDATE credentials SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?", ids[key]).Error)
				}
			}

			res, err := MigrateCredentialsAtRest(db)
			require.NoError(t, err)
			assert.Equal(t, CredentialMigration{Migrated: 1, Skipped: 1}, res)

			loser := "second"
			if tc.winner == "second" {
				loser = "first"
			}
			won, lost := readStored(t, db, ids[tc.winner]), readStored(t, db, ids[loser])
			require.NotNil(t, won.SecretHash)
			assert.Nil(t, lost.SecretHash)
			for _, row := range []storedCredential{won, lost} {
				assert.True(t, strings.HasPrefix(row.Secret, secrets.DataKeyPrefix), "both secrets are encrypted")
			}
			var found Credential
			if err := found.GetBySecret(db.Unscoped(), "one-shared-secret"); assert.NoError(t, err) {
				assert.Equal(t, ids[tc.winner], found.ID)
			}

			// The next start comes to the same answer.
			res, err = MigrateCredentialsAtRest(db)
			require.NoError(t, err)
			assert.Equal(t, CredentialMigration{Skipped: 1}, res)
			assert.NotNil(t, readStored(t, db, ids[tc.winner]).SecretHash)
			assert.Nil(t, readStored(t, db, ids[loser]).SecretHash)
		})
	}
}

// The work on N credentials derives a key a constant number of times, not
// N: creating them, reading them one by one and as a list, looking each up
// by its secret, and migrating N more from before the change. With
// secrets.EncryptValue it was one derivation (about 100 ms at production
// cost) per credential written, and per credential read beyond a small
// cache.
func TestCredentialWorkDerivesOneKey_Postgres(t *testing.T) {
	forEachCredentialDB(t, func(t *testing.T, db *gorm.DB) {
		t.Setenv("TYK_AI_SECRET_KEY", credentialTestKey+"-"+t.Name()) // a key no other test has derived for
		const n = 300
		before := secrets.KeyDerivations()

		var created []*Credential
		for i := 0; i < n; i++ {
			c := &Credential{KeyID: keyID(t, fmt.Sprintf("n-%03d", i)), Secret: fmt.Sprintf("secret-%s-%03d", t.Name(), i), Active: true}
			require.NoError(t, c.Create(db))
			created = append(created, c)
		}
		for _, c := range created {
			var byID, bySecret Credential
			require.NoError(t, byID.Get(db, c.ID))
			require.Equal(t, c.Secret, byID.Secret)
			require.NoError(t, bySecret.GetBySecret(db, c.Secret))
			require.Equal(t, c.ID, bySecret.ID)
		}
		var list Credentials
		require.NoError(t, db.Where("key_id LIKE ?", keyID(t, "")+"%").Find(&list).Error)
		require.Len(t, list, n)

		var legacy []string
		for i := 0; i < n; i++ {
			legacy = append(legacy, keyID(t, fmt.Sprintf("legacy-%03d", i)))
		}
		insertLegacy(t, db, legacy...)
		res, err := MigrateCredentialsAtRest(db)
		require.NoError(t, err)
		require.Equal(t, n, res.Migrated)
		require.NoError(t, (&Credential{}).GetBySecret(db, "legacy-secret-of-"+legacy[n-1]))

		assert.LessOrEqual(t, secrets.KeyDerivations()-before, uint64(1),
			"creating, reading, looking up and migrating %d credentials derived more than the one data key", 2*n)
	})
}

// A lookup by the secret compares hashes and does not read the secret
// column: it answers for a row whose stored secret is unreadable, hands
// back the secret it was given, and derives nothing.
func TestGetBySecretDoesNotReadTheSecret(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", credentialTestKey)
	db := setupTestDB(t)
	cred := &Credential{KeyID: "lookup", Secret: "the-key-looked-up", Active: true}
	require.NoError(t, cred.Create(db))
	require.NoError(t, db.Exec("UPDATE credentials SET secret = ? WHERE id = ?", secrets.DataKeyCurrentPrefix()+"bm90LWEtY2lwaGVydGV4dA==", cred.ID).Error)

	var statements []string
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:statements", func(tx *gorm.DB) {
		statements = append(statements, tx.Statement.SQL.String())
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove("test:statements") })

	before := secrets.KeyDerivations()
	var found Credential
	require.NoError(t, found.GetBySecret(db, "the-key-looked-up"))
	assert.Equal(t, cred.ID, found.ID)
	assert.True(t, found.Active)
	assert.Equal(t, "the-key-looked-up", found.Secret, "the caller's secret is the row's")
	assert.Equal(t, before, secrets.KeyDerivations())
	require.NotEmpty(t, statements)
	for _, sql := range statements {
		assert.NotRegexp(t, "[`\"]secret[`\"]", sql, "the secret column is not selected")
	}

	// Read whole, the same row is refused: its stored secret does not open.
	var whole Credential
	require.NoError(t, whole.Get(db, cred.ID))
	assert.Equal(t, secrets.RefusedCredential, whole.Secret)
}

// A stored secret is bound to its row: copied into another credential's
// row it does not open there.
func TestCredentialCiphertextIsBoundToItsRow(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", credentialTestKey)
	db := setupTestDB(t)
	a := &Credential{KeyID: "row-a", Secret: "secret-of-a", Active: true}
	b := &Credential{KeyID: "row-b", Secret: "secret-of-b", Active: true}
	require.NoError(t, a.Create(db))
	require.NoError(t, b.Create(db))
	require.NoError(t, db.Exec("UPDATE credentials SET secret = (SELECT secret FROM credentials WHERE id = ?) WHERE id = ?", a.ID, b.ID).Error)

	var readA, readB Credential
	require.NoError(t, readA.Get(db, a.ID))
	require.NoError(t, readB.Get(db, b.ID))
	assert.Equal(t, "secret-of-a", readA.Secret)
	assert.Equal(t, secrets.RefusedCredential, readB.Secret, "a's ciphertext does not open as b's")
}

// The database's salt is created once and never replaced. Every handle on
// the database reads the same one. If it is missing while credentials
// sealed under it exist, nothing generates a new one: the credentials read
// as refused, saving fails, and EnsureDataKeySalt (start-up) reports what
// was lost.
func TestDataKeySalt_Postgres(t *testing.T) {
	forEachCredentialDB(t, func(t *testing.T, db *gorm.DB) {
		t.Setenv("TYK_AI_SECRET_KEY", credentialTestKey)
		salt, err := EnsureDataKeySalt(db)
		require.NoError(t, err)
		require.Len(t, salt, secrets.DataKeySaltLength)
		again, err := EnsureDataKeySalt(db)
		require.NoError(t, err)
		assert.Equal(t, salt, again, "created once")
		dataKeySalts.Delete(dataKeySaltCacheKey(db)) // a new process
		fromDB, err := EnsureDataKeySalt(db)
		require.NoError(t, err)
		assert.Equal(t, salt, fromDB, "every replica reads the stored one")
		var rows int64
		require.NoError(t, db.Model(&DataKeySalt{}).Count(&rows).Error)
		assert.Equal(t, int64(1), rows)

		cred := &Credential{KeyID: keyID(t, "salted"), Secret: "sealed-under-the-salt-" + t.Name(), Active: true}
		require.NoError(t, cred.Create(db))

		// The salt is lost (a table restored without it, say).
		require.NoError(t, db.Exec("DELETE FROM data_key_salts").Error)
		dataKeySalts.Delete(dataKeySaltCacheKey(db))

		_, err = EnsureDataKeySalt(db)
		assert.ErrorIs(t, err, ErrDataKeySaltLost)
		assert.Contains(t, err.Error(), "data_key_salts")
		var read Credential
		require.NoError(t, read.Get(db, cred.ID))
		assert.Equal(t, secrets.RefusedCredential, read.Secret)
		assert.ErrorIs(t, (&Credential{KeyID: keyID(t, "new"), Secret: "cannot-be-sealed-" + t.Name()}).Create(db), ErrDataKeySaltLost,
			"a new credential is not encrypted under a salt made up on the spot")
		require.NoError(t, db.Model(&DataKeySalt{}).Count(&rows).Error)
		assert.Zero(t, rows, "no new salt was generated")

		// The row comes back (from the backup): everything reads again.
		require.NoError(t, db.Create(&DataKeySalt{ID: dataKeySaltRowID, Salt: salt}).Error)
		require.NoError(t, read.Get(db, cred.ID))
		assert.Equal(t, cred.Secret, read.Secret)
	})
}

// Rows an earlier revision of this change wrote with secrets.EncryptValue
// ("$ENC/", on development stacks only) are converted to the data-key form
// like rows from before the change, and keep validating.
func TestMigrateCredentialsAtRest_ConvertsEncValueRows(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", credentialTestKey)
	db := setupTestDB(t)
	ids := insertLegacy(t, db, "enc-form")
	plain := "legacy-secret-of-enc-form"
	require.NoError(t, db.Exec("UPDATE credentials SET secret = ?, secret_hash = ? WHERE id = ?",
		secrets.EncryptValue(plain), secrets.LookupHash(plain), ids["enc-form"]).Error)

	var read Credential
	require.NoError(t, read.Get(db, ids["enc-form"]))
	assert.Equal(t, plain, read.Secret, "readable before it is converted")

	res, err := MigrateCredentialsAtRest(db)
	require.NoError(t, err)
	assert.Equal(t, CredentialMigration{Migrated: 1}, res)
	row := readStored(t, db, ids["enc-form"])
	assert.True(t, strings.HasPrefix(row.Secret, secrets.DataKeyCurrentPrefix()), "%q", row.Secret)
	require.NoError(t, read.GetBySecret(db, plain))
	require.NoError(t, read.Get(db, ids["enc-form"]))
	assert.Equal(t, plain, read.Secret)

	res, err = MigrateCredentialsAtRest(db)
	require.NoError(t, err)
	assert.Equal(t, CredentialMigration{}, res)
}

// A FIPS-mode Studio that starts on a database a standard build wrote reads
// the scrypt-derived form, and its migration seals every credential again
// under a PBKDF2-derived data key, with the same salt.
func TestMigrateCredentialsAtRest_FIPSProcessConvertsScryptForm(t *testing.T) {
	if strings.HasPrefix(secrets.DataKeyCurrentPrefix(), secrets.DataKeyPrefix+"p") {
		t.Skip("this process is in FIPS mode and cannot write the scrypt form to start from (secrets.TestDataKeyFormsAcrossFIPSAndStandardBuilds covers reading it)")
	}
	t.Setenv("TYK_AI_SECRET_KEY", credentialTestKey)
	db := setupTestDB(t)
	cred, err := NewCredential()
	require.NoError(t, err)
	require.NoError(t, cred.Create(db))
	standard := readStored(t, db, cred.ID)
	require.True(t, strings.HasPrefix(standard.Secret, secrets.DataKeyPrefix+"s:"), "%q", standard.Secret)

	secrets.ForceFIPSModeForTest(t)
	require.True(t, strings.HasPrefix(secrets.DataKeyCurrentPrefix(), secrets.DataKeyPrefix+"p"))

	var read Credential
	require.NoError(t, read.Get(db, cred.ID))
	assert.Equal(t, cred.Secret, read.Secret, "the scrypt form is readable in FIPS mode")

	res, err := MigrateCredentialsAtRest(db)
	require.NoError(t, err)
	assert.Equal(t, CredentialMigration{Migrated: 1}, res)
	row := readStored(t, db, cred.ID)
	assert.True(t, strings.HasPrefix(row.Secret, secrets.DataKeyCurrentPrefix()), "%q", row.Secret)
	assert.Equal(t, *standard.SecretHash, *row.SecretHash, "the lookup hash does not change")
	require.NoError(t, read.GetBySecret(db, cred.Secret))
	require.NoError(t, read.Get(db, cred.ID))
	assert.Equal(t, cred.Secret, read.Secret)

	res, err = MigrateCredentialsAtRest(db)
	require.NoError(t, err)
	assert.Equal(t, CredentialMigration{}, res)
}

// The salt is read from the database once per process, whatever session or
// transaction of the opened database a credential is read or saved through
// (each has its own copy of the gorm.Config, so the cache cannot be keyed
// by that), and the cache holds one entry per database, not one per use.
func TestDataKeySaltIsReadOncePerDatabase_Postgres(t *testing.T) {
	forEachCredentialDB(t, func(t *testing.T, db *gorm.DB) {
		t.Setenv("TYK_AI_SECRET_KEY", credentialTestKey)
		var saltReads atomic.Int64
		require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:count_salt_reads", func(tx *gorm.DB) {
			if tx.Statement.Table == "data_key_salts" {
				saltReads.Add(1)
			}
		}))
		entries := func() (n int) {
			dataKeySalts.Range(func(_, _ any) bool { n++; return true })
			return n
		}

		first := &Credential{KeyID: keyID(t, "first"), Secret: "secret-" + t.Name(), Active: true}
		require.NoError(t, first.Create(db))
		readsAfterFirst, entriesAfterFirst := saltReads.Load(), entries()
		require.LessOrEqual(t, readsAfterFirst, int64(2), "found missing, created and read back")

		for i := 0; i < 50; i++ {
			session := db.WithContext(context.Background())
			c := &Credential{KeyID: keyID(t, fmt.Sprintf("s-%02d", i)), Secret: fmt.Sprintf("secret-%s-%02d", t.Name(), i), Active: true}
			require.NoError(t, c.Create(session))
			var read Credential
			require.NoError(t, read.Get(db.Session(&gorm.Session{}), c.ID))
			require.Equal(t, c.Secret, read.Secret)
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				var inTx Credential
				if err := inTx.Get(tx, c.ID); err != nil {
					return err
				}
				require.Equal(t, c.Secret, inTx.Secret)
				return inTx.Deactivate(tx)
			}))
		}
		assert.Equal(t, readsAfterFirst, saltReads.Load(), "the salt was read again")
		assert.Equal(t, entriesAfterFirst, entries(), "the salt cache grew with use")
	})
}

// A read that does not run the credential's hooks (SkipHooks, a Joins of an
// App's credential) leaves the refused marker in Secret, never the stored
// form, and saving such a struct keeps the stored secret and its hash.
func TestCredentialReadWithoutHooksIsRefusedNotCiphertext(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", credentialTestKey)
	db := setupTestDB(t)
	cred, err := NewCredential()
	require.NoError(t, err)
	cred.Active = true
	require.NoError(t, cred.Create(db))
	user := User{Email: "nohooks@example.com", Name: "No Hooks"}
	require.NoError(t, db.Create(&user).Error)
	app := App{Name: "no-hooks", UserID: user.ID, CredentialID: cred.ID, IsActive: true}
	require.NoError(t, db.Omit("Credential").Create(&app).Error)
	before := readStored(t, db, cred.ID)
	require.True(t, strings.HasPrefix(before.Secret, secrets.DataKeyPrefix))

	var skipped Credential
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).First(&skipped, cred.ID).Error)
	assert.Equal(t, secrets.RefusedCredential, skipped.Secret)

	var joined App
	require.NoError(t, db.Joins("Credential").First(&joined, app.ID).Error)
	assert.NotContains(t, joined.Credential.Secret, secrets.DataKeyPrefix)
	assert.Contains(t, []string{secrets.RefusedCredential, cred.Secret}, joined.Credential.Secret)

	// Saved for another reason, the row keeps its secret and hash.
	require.NoError(t, skipped.Deactivate(db))
	after := readStored(t, db, cred.ID)
	assert.Equal(t, before.Secret, after.Secret)
	assert.Equal(t, *before.SecretHash, *after.SecretHash)
	var read Credential
	require.NoError(t, read.Get(db, cred.ID))
	assert.Equal(t, cred.Secret, read.Secret)
	assert.False(t, read.Active)

	// The same struct read again with hooks is opened.
	require.NoError(t, skipped.Get(db, cred.ID))
	assert.Equal(t, cred.Secret, skipped.Secret)
}

// An unreadable secret is logged once per credential while it lasts, not
// on every read, and once when it can be read again
// (midsommar-clean-4x5y.39). What callers see does not change.
func TestUnreadableCredentialIsLoggedOnce(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", credentialTestKey)
	db := setupTestDB(t)
	cred := &Credential{KeyID: "k", Secret: "the-app-key", Active: true}
	require.NoError(t, cred.Create(db))

	prev := logger.Log
	t.Cleanup(func() { logger.Use(prev) })
	var logged bytes.Buffer
	logger.Use(zerolog.New(&logged))

	t.Setenv("TYK_AI_SECRET_KEY", "another-unit-test-key")
	for i := 0; i < 20; i++ {
		var read Credential
		require.NoError(t, read.Get(db, cred.ID))
		assert.Equal(t, secrets.RefusedCredential, read.Secret)
	}
	assert.Equal(t, 1, strings.Count(logged.String(), "cannot be read"), logged.String())

	t.Setenv("TYK_AI_SECRET_KEY", credentialTestKey)
	for i := 0; i < 5; i++ {
		var read Credential
		require.NoError(t, read.Get(db, cred.ID))
		assert.Equal(t, "the-app-key", read.Secret)
	}
	assert.Equal(t, 1, strings.Count(logged.String(), "can be read again"), logged.String())

	t.Setenv("TYK_AI_SECRET_KEY", "another-unit-test-key")
	var read Credential
	require.NoError(t, read.Get(db, cred.ID))
	assert.Equal(t, 2, strings.Count(logged.String(), "cannot be read"), "logged again when it recurs")
}
