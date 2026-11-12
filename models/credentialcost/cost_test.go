// Package credentialcost_test measures App credentials at rest with the
// PRODUCTION key derivation cost. Every other test binary that touches
// encryption lowers that cost (secrets.UseMinScryptCostForTests), which is
// how a design that derived a key per credential, about 100 ms each, once
// passed every test: this one would have failed it.
package credentialcost_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	gormlogger "github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

func forEachDB(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	cfg := &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)}
	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "cost.db")+"?_journal_mode=WAL&_synchronous=NORMAL"), cfg)
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&models.Credential{}, &models.DataKeySalt{}))
		body(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		base := os.Getenv("DATABASE_URL")
		if !strings.HasPrefix(base, "postgres") {
			t.Skip("DATABASE_URL is not a Postgres URL")
		}
		admin, err := gorm.Open(postgres.Open(base), cfg)
		require.NoError(t, err)
		schema := fmt.Sprintf("credential_cost_%d", time.Now().UnixNano())
		require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
		sep := "?"
		if strings.Contains(base, "?") {
			sep = "&"
		}
		db, err := gorm.Open(postgres.Open(base+sep+"search_path="+schema), cfg)
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
		require.NoError(t, db.AutoMigrate(&models.Credential{}, &models.DataKeySalt{}))
		body(t, db)
	})
}

// insertLegacy writes n rows as code from before the change did: the secret
// in plaintext and no hash.
func insertLegacy(t testing.TB, db *gorm.DB, prefix string, n int) {
	t.Helper()
	const batch = 500
	type legacy struct {
		KeyID     string
		Secret    string
		Active    bool
		CreatedAt time.Time
		UpdatedAt time.Time
	}
	now := time.Now()
	for done := 0; done < n; done += batch {
		rows := make([]legacy, 0, batch)
		for i := done; i < done+batch && i < n; i++ {
			rows = append(rows, legacy{KeyID: fmt.Sprintf("%s-%06d", prefix, i), Secret: fmt.Sprintf("legacy-secret-%s-%06d", prefix, i), Active: true, CreatedAt: now, UpdatedAt: now})
		}
		require.NoError(t, db.Table("credentials").Create(&rows).Error)
	}
}

// At production cost: creating, reading, looking up and migrating
// credentials derives ONE key, and none of it takes anywhere near the 100 ms
// per credential a per-value key derivation costs.
func TestOneKeyDerivationAtProductionCost(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		t.Setenv("TYK_AI_SECRET_KEY", "production-cost-key-"+t.Name())
		const n = 200
		before := secrets.KeyDerivations()
		start := time.Now()

		var created []*models.Credential
		for i := 0; i < n; i++ {
			c, err := models.NewCredential()
			require.NoError(t, err)
			require.NoError(t, c.Create(db))
			created = append(created, c)
		}
		for _, c := range created {
			var byID, bySecret models.Credential
			require.NoError(t, byID.Get(db, c.ID))
			require.Equal(t, c.Secret, byID.Secret)
			require.NoError(t, bySecret.GetBySecret(db, c.Secret))
		}
		var all []models.Credential
		require.NoError(t, db.Find(&all).Error)
		require.Len(t, all, n)

		insertLegacy(t, db, "legacy", n)
		res, err := models.MigrateCredentialsAtRest(db)
		require.NoError(t, err)
		require.Equal(t, n, res.Migrated)

		elapsed := time.Since(start)
		assert.Equal(t, uint64(1), secrets.KeyDerivations()-before, "one data key, however many credentials")
		// 400 credentials written and 600 reads: at 100 ms per derivation
		// that would be over a minute.
		assert.Less(t, elapsed, 20*time.Second, "took %s", elapsed)
		t.Logf("%d created, %d read twice and listed, %d migrated: %s, 1 key derivation", n, n, n, elapsed)
	})
}

// The first-start migration of a large installation, at production cost.
// Skipped under -short; the times it logs are quoted in the pull request.
func TestMigrationTimeAtProductionCost(t *testing.T) {
	if testing.Short() {
		t.Skip("migrates 10,000 and 50,000 credentials")
	}
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		t.Setenv("TYK_AI_SECRET_KEY", "production-cost-key-"+t.Name())
		for _, n := range []int{10_000, 50_000} {
			require.NoError(t, db.Exec("DELETE FROM credentials").Error)
			insertLegacy(t, db, fmt.Sprintf("m%d", n), n)
			before := secrets.KeyDerivations()
			start := time.Now()
			res, err := models.MigrateCredentialsAtRest(db)
			elapsed := time.Since(start)
			require.NoError(t, err)
			require.Equal(t, n, res.Migrated)
			assert.LessOrEqual(t, secrets.KeyDerivations()-before, uint64(1))

			var plaintext int64
			require.NoError(t, db.Table("credentials").Where("secret NOT LIKE ?", secrets.DataKeyCurrentPrefix()+"%").Count(&plaintext).Error)
			assert.Zero(t, plaintext)
			var c models.Credential
			require.NoError(t, c.GetBySecret(db, fmt.Sprintf("legacy-secret-m%d-%06d", n, n-1)))

			again := time.Now()
			res, err = models.MigrateCredentialsAtRest(db)
			require.NoError(t, err)
			require.Zero(t, res.Migrated)
			t.Logf("%s: migrated %d credentials in %s (%.0f per second); a second run took %s",
				db.Dialector.Name(), n, elapsed.Round(time.Millisecond), float64(n)/elapsed.Seconds(), time.Since(again).Round(time.Millisecond))
		}
	})
}
