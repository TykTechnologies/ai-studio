package services

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/auth"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/pkg/credhash"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const apiTokenTestKey = "0123456789abcdef0123456789abcdef"

func apiTokenDBs(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Cleanup(func() { require.NoError(t, database.SetAPITokenHasherFromKey(apiTokenTestKey)) })
	t.Run("sqlite", func(t *testing.T) { body(t, apiTokenSQLite(t)) })
	t.Run("postgres", func(t *testing.T) { body(t, openPostgresSchema(t)) })
}

// apiTokenSQLite is a SQLite database in a file: the auth provider updates
// last_used_at on another connection, and an in-memory database belongs to
// one.
func apiTokenSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "gateway.db")+"?_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))
	t.Cleanup(func() { database.Close(db) })
	return db
}

// storedBeforeHashing writes an api_tokens row the way a version before
// token hashing did: the token itself in the token column, no hash.
func storedBeforeHashing(t *testing.T, db *gorm.DB, appID uint, name, token string, active bool) {
	t.Helper()
	now := time.Now()
	require.NoError(t, db.Exec(
		"INSERT INTO api_tokens (token, name, app_id, is_active, namespace, created_at, updated_at) VALUES (?, ?, ?, ?, '', ?, ?)",
		token, name, appID, active, now, now).Error)
}

// scanAPITokens returns every value of every column of api_tokens, as text.
func scanAPITokens(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var rows []map[string]interface{}
	require.NoError(t, db.Table("api_tokens").Find(&rows).Error)
	var b strings.Builder
	for _, row := range rows {
		for col, v := range row {
			if bs, ok := v.([]byte); ok {
				v = string(bs)
			}
			fmt.Fprintf(&b, "%s=%v\n", col, v)
		}
	}
	return b.String()
}

// After an upgrade every token a previous version stored still
// authenticates, on every path that checks one, and the table no longer
// holds any token. A second start changes nothing.
func TestAPITokenHashing_UpgradeKeepsTokensWorkingAndRemovesThem(t *testing.T) {
	apiTokenDBs(t, func(t *testing.T, db *gorm.DB) {
		app := &database.App{Name: "upgrade-app", IsActive: true}
		require.NoError(t, db.Create(app).Error)
		tokens := map[string]string{
			"admin":   "5f2b1c9e7a4d4e0f9b3a6c8d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f",
			"service": "0e9d8c7b6a5f4e3d2c1b0a9f8e7d6c5b4a3f2e1d0c9b8a7f6e5d4c3b2a1f0e9d",
			"short":   "legacy-short-token",
		}
		for name, token := range tokens {
			storedBeforeHashing(t, db, app.ID, name, token, true)
		}
		const revoked = "aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaa7777bbbb8888"
		storedBeforeHashing(t, db, app.ID, "revoked", revoked, false)
		const deleted = "9999888877776666555544443333222211110000ffffeeeeddddccccbbbbaaaa"
		storedBeforeHashing(t, db, app.ID, "deleted", deleted, true)
		require.NoError(t, db.Exec("UPDATE api_tokens SET deleted_at = ? WHERE name = 'deleted'", time.Now()).Error)
		// A table early versions created, keyed by the token.
		require.NoError(t, db.Exec("CREATE TABLE token_cache (token varchar(255) PRIMARY KEY, cache_data text)").Error)
		require.NoError(t, db.Exec("INSERT INTO token_cache (token, cache_data) VALUES (?, '{}')", tokens["admin"]).Error)

		// The upgraded gateway starts.
		require.NoError(t, prepareAPITokenHashing(db, apiTokenTestKey))

		// No token is left anywhere in the table, soft-deleted rows included.
		contents := scanAPITokens(t, db)
		for name, token := range map[string]string{"admin": tokens["admin"], "service": tokens["service"], "short": tokens["short"], "revoked": revoked, "deleted": deleted} {
			assert.NotContains(t, contents, token, "the %s token is still in api_tokens", name)
		}
		var withoutHash int64
		require.NoError(t, db.Unscoped().Model(&database.APIToken{}).Where("token_hash IS NULL OR token_hash = ''").Count(&withoutHash).Error)
		assert.Zero(t, withoutHash, "every row has its hash")
		assert.False(t, db.Migrator().HasTable("token_cache"), "the unused token_cache table is removed")

		// What replaced a token identifies it without being it.
		var admin database.APIToken
		require.NoError(t, db.Where("name = ?", "admin").First(&admin).Error)
		assert.True(t, credhash.IsHash(admin.TokenHash))
		assert.True(t, strings.HasPrefix(admin.Token, "5f2b1c9e...redacted:"), admin.Token)
		assert.Len(t, strings.TrimPrefix(admin.Token, "5f2b1c9e...redacted:"), 16)
		var short database.APIToken
		require.NoError(t, db.Where("name = ?", "short").First(&short).Error)
		assert.True(t, strings.HasPrefix(short.Token, "lega...redacted:"), "a short token shows a quarter of itself at most: %s", short.Token)

		// Every existing token still authenticates, on each path.
		gateway := NewDatabaseGatewayService(db, database.NewRepository(db))
		provider := auth.NewTokenAuthProvider(db)
		repo := database.NewRepository(db)
		for name, token := range tokens {
			res, err := gateway.ValidateAPIToken(token)
			require.NoError(t, err, "gateway service: %s", name)
			assert.Equal(t, app.ID, res.AppID)
			ar, err := provider.ValidateToken(token)
			require.NoError(t, err)
			assert.True(t, ar.Valid, "auth provider: %s: %s", name, ar.Error)
			row, err := repo.GetAPIToken(token)
			require.NoError(t, err, "repository: %s", name)
			assert.Equal(t, name, row.Name)
		}
		// A revoked token is still revoked, and what is stored in place of a
		// token is not a token.
		_, err := gateway.ValidateAPIToken(revoked)
		assert.Error(t, err)
		_, err = gateway.ValidateAPIToken(admin.Token)
		assert.Error(t, err, "the redacted reference authenticates nothing")
		_, err = gateway.ValidateAPIToken(admin.TokenHash)
		assert.Error(t, err, "the hash authenticates nothing")

		// A second start is a no-op: nothing to convert, and every column of
		// every row is as it was. Two columns are left out of the
		// comparison, and only those: last_used_at, which the auth provider
		// used above sets in the background after a validation, and
		// updated_at, which that same update moves. (Compared whole, this
		// check failed 3 times in 200 runs, each time on exactly those two
		// columns of a token validated a moment before.)
		stored := func() map[string]string {
			var rows []map[string]interface{}
			require.NoError(t, db.Table("api_tokens").Order("id").Find(&rows).Error)
			out := map[string]string{}
			for _, row := range rows {
				for col, v := range row {
					if col == "last_used_at" || col == "updated_at" {
						continue
					}
					if bs, ok := v.([]byte); ok {
						v = string(bs)
					}
					out[fmt.Sprintf("%v.%s", row["id"], col)] = fmt.Sprint(v)
				}
			}
			return out
		}
		before := stored()
		require.Greater(t, len(before), 40, "every column of the five rows is compared")
		n, err := database.HashStoredAPITokens(db)
		require.NoError(t, err)
		assert.Zero(t, n)
		require.NoError(t, prepareAPITokenHashing(db, apiTokenTestKey))
		assert.Equal(t, before, stored(), "a second start changed the table")
	})
}

// New tokens are never stored: created through the provider, the
// repository or a row built with the token, the database gets the hash.
func TestAPITokenHashing_NewTokensAreNotStored(t *testing.T) {
	apiTokenDBs(t, func(t *testing.T, db *gorm.DB) {
		require.NoError(t, prepareAPITokenHashing(db, apiTokenTestKey))
		app := &database.App{Name: "new-app", IsActive: true}
		require.NoError(t, db.Create(app).Error)
		provider := auth.NewTokenAuthProvider(db)
		gateway := NewDatabaseGatewayService(db, database.NewRepository(db))

		generated, err := provider.GenerateToken(app.ID, "generated", []string{"admin"}, time.Hour)
		require.NoError(t, err)
		require.Len(t, generated, 64, "the caller gets the token, once")

		const viaRepo = "1111222233334444555566667777888899990000aaaabbbbccccddddeeeeffff"
		row := &database.APIToken{Token: viaRepo, Name: "via-repository", AppID: app.ID, IsActive: true}
		require.NoError(t, database.NewRepository(db).CreateAPIToken(row))
		assert.NotEqual(t, viaRepo, row.Token, "after the insert the row holds the redacted reference")

		contents := scanAPITokens(t, db)
		assert.NotContains(t, contents, generated)
		assert.NotContains(t, contents, viaRepo)
		for _, token := range []string{generated, viaRepo} {
			_, err := gateway.ValidateAPIToken(token)
			require.NoError(t, err)
		}

		// Revoking by token finds it by its hash.
		require.NoError(t, provider.RevokeToken(generated))
		_, err = gateway.ValidateAPIToken(generated)
		assert.Error(t, err)
		info, err := provider.GetTokenInfo(viaRepo)
		require.NoError(t, err)
		assert.Equal(t, "via-repository", info.Name)

		// The same token cannot be stored twice.
		dup := &database.APIToken{Token: viaRepo, Name: "duplicate", AppID: app.ID, IsActive: true}
		assert.Error(t, db.Create(dup).Error)
	})
}

// Tokens are hashed with ENCRYPTION_KEY: a gateway started with another key
// accepts none of them (it does not fall back to comparing what is stored),
// and one that cannot hash at all checks nothing.
func TestAPITokenHashing_FailsClosed(t *testing.T) {
	db := apiTokenSQLite(t)
	t.Cleanup(func() { require.NoError(t, database.SetAPITokenHasherFromKey(apiTokenTestKey)) })
	require.NoError(t, prepareAPITokenHashing(db, apiTokenTestKey))
	app := &database.App{Name: "app", IsActive: true}
	require.NoError(t, db.Create(app).Error)
	const token = "5f2b1c9e7a4d4e0f9b3a6c8d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f"
	require.NoError(t, db.Create(&database.APIToken{Token: token, Name: "t", AppID: app.ID, IsActive: true}).Error)
	gateway := NewDatabaseGatewayService(db, database.NewRepository(db))
	_, err := gateway.ValidateAPIToken(token)
	require.NoError(t, err)

	// Another ENCRYPTION_KEY.
	require.NoError(t, prepareAPITokenHashing(db, "fedcba9876543210fedcba9876543210"))
	_, err = gateway.ValidateAPIToken(token)
	assert.Error(t, err)

	// No hasher at all.
	database.SetAPITokenHasher(nil)
	_, err = gateway.ValidateAPIToken(token)
	require.Error(t, err)
	assert.ErrorIs(t, err, database.ErrNoAPITokenHasher)
	assert.ErrorIs(t, db.Create(&database.APIToken{Token: "another-token-value", Name: "x", AppID: app.ID}).Error, database.ErrNoAPITokenHasher,
		"a token cannot be stored unhashed")
	_, err = database.HashStoredAPITokens(db)
	assert.ErrorIs(t, err, database.ErrNoAPITokenHasher)

	// A key too short to hash with stops the start.
	assert.Error(t, prepareAPITokenHashing(db, "short"))
}

// Two gateways on one database convert the stored tokens at the same time:
// each row is converted once and every token still works.
func TestAPITokenHashing_ConcurrentStartsPostgres(t *testing.T) {
	db := openPostgresSchema(t)
	t.Cleanup(func() { require.NoError(t, database.SetAPITokenHasherFromKey(apiTokenTestKey)) })
	require.NoError(t, database.SetAPITokenHasherFromKey(apiTokenTestKey))
	app := &database.App{Name: "shared", IsActive: true}
	require.NoError(t, db.Create(app).Error)
	var tokens []string
	for i := 0; i < 40; i++ {
		token := fmt.Sprintf("%064x", i+1)
		tokens = append(tokens, token)
		storedBeforeHashing(t, db, app.ID, fmt.Sprintf("t%d", i), token, true)
	}

	errs := make(chan error, 2)
	counts := make(chan int, 2)
	for g := 0; g < 2; g++ {
		go func() {
			n, err := database.HashStoredAPITokens(db)
			counts <- n
			errs <- err
		}()
	}
	total := 0
	for g := 0; g < 2; g++ {
		require.NoError(t, <-errs)
		total += <-counts
	}
	assert.Equal(t, len(tokens), total, "each row is converted exactly once")
	gateway := NewDatabaseGatewayService(db, database.NewRepository(db))
	for _, token := range tokens {
		_, err := gateway.ValidateAPIToken(token)
		require.NoError(t, err)
	}
	assert.NotContains(t, scanAPITokens(t, db), tokens[0])
}
