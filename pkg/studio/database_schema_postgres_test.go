package studio

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

// schemaTestDSN returns the Postgres database the shared-database tests use,
// or skips. They create schemas of their own and drop them afterwards.
func schemaTestDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	return dsn
}

func dropSchemaAfter(t *testing.T, dsn, schema string) {
	t.Helper()
	t.Cleanup(func() {
		admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			return
		}
		admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`)
		if sqlDB, err := admin.DB(); err == nil {
			sqlDB.Close()
		}
	})
}

func openSchema(t *testing.T, dsn, schema string) *gorm.DB {
	t.Helper()
	db, err := OpenDatabase(&config.AppConf{DatabaseType: "postgres", DatabaseURL: dsn, DatabaseSchema: schema})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return db
}

// DATABASE_SCHEMA creates the schema and puts every Studio table in it, so
// Studio can share a database with an application that has tables of the
// same names (users, roles, audit_records...).
func TestOpenDatabaseWithSchema_Postgres(t *testing.T) {
	dsn := schemaTestDSN(t)
	schema := fmt.Sprintf("studio_schema_%d", time.Now().UnixNano())
	dropSchemaAfter(t, dsn, schema)

	// Stand in for the host: its own users table in public.
	host, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	hostTable := fmt.Sprintf("host_users_%d", time.Now().UnixNano())
	require.NoError(t, host.Exec(`CREATE TABLE public.`+hostTable+` (id int)`).Error)
	t.Cleanup(func() {
		host.Exec(`DROP TABLE IF EXISTS public.` + hostTable)
		if sqlDB, err := host.DB(); err == nil {
			sqlDB.Close()
		}
	})

	db := openSchema(t, dsn, schema)
	var current string
	require.NoError(t, db.Raw("SELECT current_schema()").Scan(&current).Error)
	assert.Equal(t, schema, current)

	require.NoError(t, models.InitModels(db))
	require.NoError(t, analytics.Migrate(db))

	var inSchema int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM pg_tables WHERE schemaname = ?`, schema).Scan(&inSchema).Error)
	assert.Greater(t, inSchema, int64(50), "Studio's tables should be in its schema")
	for _, table := range []string{"users", "audit_records", "tyk_policies", "llm_chat_records"} {
		var reg *string
		require.NoError(t, db.Raw(`SELECT to_regclass(?)::text`, schema+"."+table).Scan(&reg).Error)
		assert.NotNil(t, reg, "%s.%s should exist", schema, table)
	}

	// Opening again finds the schema rather than failing to create it.
	openSchema(t, dsn, schema)
}

// Replicas booting together each take the migration lock in turn: no two
// hold it at once, and every one migrates and seeds without error.
func TestConcurrentMigrationsSerialise_Postgres(t *testing.T) {
	dsn := schemaTestDSN(t)
	schema := fmt.Sprintf("studio_lock_%d", time.Now().UnixNano())
	dropSchemaAfter(t, dsn, schema)

	const replicas = 3
	dbs := make([]*gorm.DB, replicas)
	for i := range dbs {
		dbs[i] = openSchema(t, dsn, schema)
	}

	var mu sync.Mutex
	holding, maxHolding := 0, 0
	var wg sync.WaitGroup
	errs := make(chan error, replicas)
	for _, db := range dbs {
		wg.Add(1)
		go func(db *gorm.DB) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			release, err := models.AcquireMigrationLock(ctx, db)
			if err != nil {
				errs <- err
				return
			}
			defer release()
			mu.Lock()
			holding++
			if holding > maxHolding {
				maxHolding = holding
			}
			mu.Unlock()
			defer func() { mu.Lock(); holding--; mu.Unlock() }()

			if err := models.InitModels(db); err != nil {
				errs <- fmt.Errorf("migrate: %w", err)
				return
			}
			if err := analytics.Migrate(db); err != nil {
				errs <- fmt.Errorf("analytics: %w", err)
				return
			}
			if err := models.MigrateTIBStores(db); err != nil {
				errs <- fmt.Errorf("tib: %w", err)
				return
			}
			if err := ensureDefaults(db, true); err != nil {
				errs <- fmt.Errorf("seed: %w", err)
			}
		}(db)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	assert.Equal(t, 1, maxHolding, "the migration lock must be held by one replica at a time")

	// Seeding ran three times; the get-or-create seeds must not duplicate.
	var groups int64
	require.NoError(t, dbs[0].Raw(`SELECT count(*) FROM groups WHERE name = 'Default'`).Scan(&groups).Error)
	assert.Equal(t, int64(1), groups)
}

// The lock is per schema: a Studio in another schema of the same database
// does not wait for it.
func TestMigrationLockIsPerSchema_Postgres(t *testing.T) {
	dsn := schemaTestDSN(t)
	a := fmt.Sprintf("studio_lock_a_%d", time.Now().UnixNano())
	b := fmt.Sprintf("studio_lock_b_%d", time.Now().UnixNano())
	dropSchemaAfter(t, dsn, a)
	dropSchemaAfter(t, dsn, b)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	releaseA, err := models.AcquireMigrationLock(ctx, openSchema(t, dsn, a))
	require.NoError(t, err)
	defer releaseA()

	releaseB, err := models.AcquireMigrationLock(ctx, openSchema(t, dsn, b))
	require.NoError(t, err, "a different schema must not wait for schema a's lock")
	releaseB()

	// The same schema does wait, until its context ends.
	short, cancelShort := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancelShort()
	_, err = models.AcquireMigrationLock(short, openSchema(t, dsn, a))
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
