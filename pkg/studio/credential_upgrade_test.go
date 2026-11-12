//go:build !enterprise

package studio

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// legacyCredentials is the credentials table as 2.2.2 created it (and as
// every 2.3 release candidate before this change did: the table had not
// changed): the secret in plaintext, found by its value.
const legacyCredentials = "CREATE TABLE `credentials` (`id` integer PRIMARY KEY AUTOINCREMENT,`created_at` datetime,`updated_at` datetime,`deleted_at` datetime,`key_id` text,`secret` text,`active` numeric DEFAULT false)"

func legacyKeys(t *testing.T, db *gorm.DB, n int) map[uint]string {
	t.Helper()
	keys := map[uint]string{}
	for i := 0; i < n; i++ {
		secret := "legacy-app-key-" + string(rune('a'+i))
		require.NoError(t, db.Exec("INSERT INTO credentials (key_id, secret, active, created_at, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
			"legacy-"+string(rune('a'+i)), secret, i%2 == 0).Error)
		var id uint
		require.NoError(t, db.Table("credentials").Select("id").Where("secret = ?", secret).Scan(&id).Error)
		keys[id] = secret
	}
	return keys
}

// assertCredentialsAtRest checks what an upgrade must leave: no plaintext
// secret in the table, and every key still found by its secret, with the
// state it had.
func assertCredentialsAtRest(t *testing.T, db *gorm.DB, keys map[uint]string) {
	t.Helper()
	var rows []struct {
		ID         uint
		Secret     string
		SecretHash *string
	}
	require.NoError(t, db.Table("credentials").Select("id", "secret", "secret_hash").Scan(&rows).Error)
	require.Len(t, rows, len(keys))
	for _, r := range rows {
		assert.True(t, strings.HasPrefix(r.Secret, secrets.DataKeyCurrentPrefix()), "credential %d is not encrypted under the data key: %q", r.ID, r.Secret)
		assert.NotEqual(t, keys[r.ID], r.Secret)
		require.NotNil(t, r.SecretHash, "credential %d has no lookup hash", r.ID)
		assert.True(t, strings.HasPrefix(*r.SecretHash, secrets.LookupHashKeyedPrefix))
	}
	for id, secret := range keys {
		var c models.Credential
		require.NoError(t, c.GetBySecret(db, secret), "credential %d no longer validates", id)
		assert.Equal(t, id, c.ID)
		assert.Equal(t, secret, c.Secret, "the hub can still show the key")
	}
}

// Upgrading a 2.2.2 database: the credentials table exists in its old form
// before Studio starts. Start-up adds the hash column, and hashes and
// encrypts every row under the migration lock; a second start changes
// nothing; the schema record then keeps older readers out.
func TestUpgradeFrom222HashesAndEncryptsAppCredentials(t *testing.T) {
	opts := newTestOptions(t)
	require.NoError(t, opts.DB.Exec(legacyCredentials).Error)
	require.NoError(t, opts.DB.Exec("CREATE UNIQUE INDEX `idx_credentials_key_id` ON `credentials`(`key_id`)").Error)
	require.NoError(t, opts.DB.Exec("CREATE INDEX `idx_credentials_deleted_at` ON `credentials`(`deleted_at`)").Error)
	keys := legacyKeys(t, opts.DB, 7)

	s, err := New(opts)
	require.NoError(t, err)
	assertCredentialsAtRest(t, opts.DB, keys)
	var schema models.StudioSchema
	require.NoError(t, opts.DB.First(&schema).Error)
	assert.GreaterOrEqual(t, schema.MinReaderVersion, 42, "older code must not read this database")
	stopStudio(t, s)

	var before []string
	require.NoError(t, opts.DB.Table("credentials").Order("id").Pluck("secret", &before).Error)
	s, err = New(opts)
	require.NoError(t, err)
	var after []string
	require.NoError(t, opts.DB.Table("credentials").Order("id").Pluck("secret", &after).Error)
	assert.Equal(t, before, after, "a second start is a no-op")
	assertCredentialsAtRest(t, opts.DB, keys)
	stopStudio(t, s)
}

// Upgrading a database a 2.3 release candidate migrated: everything is at
// the current schema except the credentials, whose secrets are in
// plaintext with no hash.
func TestUpgradeFrom23RCHashesAndEncryptsAppCredentials(t *testing.T) {
	opts := newTestOptions(t)
	s, err := New(opts) // the current schema
	require.NoError(t, err)
	stopStudio(t, s)
	// Back to what the release candidates had.
	require.NoError(t, opts.DB.Exec("DROP INDEX idx_credentials_secret_hash").Error)
	require.NoError(t, opts.DB.Exec("ALTER TABLE credentials DROP COLUMN secret_hash").Error)
	require.NoError(t, opts.DB.Exec("DELETE FROM credentials").Error)
	keys := legacyKeys(t, opts.DB, 5)

	s, err = New(opts)
	require.NoError(t, err)
	assertCredentialsAtRest(t, opts.DB, keys)
	stopStudio(t, s)
}
