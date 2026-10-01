package models

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

// lockTestSchema creates a schema of its own for one test (the lock is per
// schema, so tests do not wait for each other) and returns a function that
// opens a pool on it through base (DATABASE_URL, or a PgBouncer in front of
// the same database) with extra connection parameters.
func lockTestSchema(t *testing.T) (open func(base string, params map[string]string) *gorm.DB, schema string) {
	t.Helper()
	admin := os.Getenv("DATABASE_URL")
	if admin == "" {
		t.Skip("DATABASE_URL not set")
	}
	schema = fmt.Sprintf("lock_test_%d", time.Now().UnixNano())
	adb := openLockTestDB(t, admin)
	require.NoError(t, adb.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { adb.Exec(`DROP SCHEMA "` + schema + `" CASCADE`) })

	return func(base string, params map[string]string) *gorm.DB {
		u, err := url.Parse(base)
		require.NoError(t, err)
		q := u.Query()
		q.Set("search_path", schema)
		for k, v := range params {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
		return openLockTestDB(t, u.String())
	}, schema
}

func openLockTestDB(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return db
}

// tryLock reports whether AcquireMigrationLock gets the lock within d, and
// releases it if so.
func tryLock(t *testing.T, db *gorm.DB, d time.Duration) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	release, err := AcquireMigrationLock(ctx, db)
	if err != nil {
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		return false
	}
	release()
	return true
}

// One holder at a time; releasing hands the lock on at once.
func TestMigrationLockOneHolder_Postgres(t *testing.T) {
	open, _ := lockTestSchema(t)
	base := os.Getenv("DATABASE_URL")
	a, b := open(base, nil), open(base, nil)

	release, err := AcquireMigrationLock(context.Background(), a)
	require.NoError(t, err)
	assert.False(t, tryLock(t, b, 1500*time.Millisecond), "the lock is held")
	release()
	assert.True(t, tryLock(t, b, 3*time.Second), "released")
}

// A waiter gives up with an error that says what it waited for and how to
// get unstuck, rather than waiting for ever.
func TestMigrationLockWaitIsBounded_Postgres(t *testing.T) {
	open, _ := lockTestSchema(t)
	base := os.Getenv("DATABASE_URL")
	release, err := AcquireMigrationLock(context.Background(), open(base, nil))
	require.NoError(t, err)
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = AcquireMigrationLock(ctx, open(base, nil))
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), "gave up after waiting")
	assert.Contains(t, err.Error(), "MIGRATION_LOCK_TIMEOUT")
}

// The lock is held by a transaction: when the holder's session ends (a
// crash, an administrator, a pooler dropping it) the server releases it,
// and the holder's release does not panic.
func TestMigrationLockReleasedWithTheHoldersSession_Postgres(t *testing.T) {
	open, schema := lockTestSchema(t)
	base := os.Getenv("DATABASE_URL")
	app := schema + "_holder"
	holder := open(base, map[string]string{"application_name": app})
	release, err := AcquireMigrationLock(context.Background(), holder)
	require.NoError(t, err)

	other := open(base, nil)
	assert.False(t, tryLock(t, other, time.Second))
	require.NoError(t, other.Exec(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE application_name = ?`, app).Error)
	assert.True(t, tryLock(t, other, 5*time.Second), "the server releases the lock with the session")
	release()
}

// A server that ends idle transactions (idle_in_transaction_session_timeout)
// does not take the lock from a holder whose migrations run for longer: the
// holder keeps its transaction busy.
func TestMigrationLockSurvivesIdleInTransactionTimeout_Postgres(t *testing.T) {
	old := migrationLockKeepalive
	migrationLockKeepalive = 200 * time.Millisecond
	defer func() { migrationLockKeepalive = old }()

	open, _ := lockTestSchema(t)
	base := os.Getenv("DATABASE_URL")
	holder := open(base, map[string]string{"idle_in_transaction_session_timeout": "1000"})
	release, err := AcquireMigrationLock(context.Background(), holder)
	require.NoError(t, err)
	defer release()

	time.Sleep(2500 * time.Millisecond) // "migrating"
	assert.False(t, tryLock(t, open(base, nil), time.Second), "the holder must still hold the lock")
}

// Behind PgBouncer in transaction mode the lock is released on release: a
// session-level lock stayed behind on the pooled server connection and every
// later instance waited for ever. Set PGBOUNCER_URL to a transaction-mode
// PgBouncer in front of DATABASE_URL's database to run it. It uses the
// public schema's lock: PgBouncer does not keep a client's search_path on
// PostgreSQL before 18 (the server does not report it), so a per-test
// schema would not reach the server.
func TestMigrationLockThroughPgBouncer_Postgres(t *testing.T) {
	bouncer := os.Getenv("PGBOUNCER_URL")
	base := os.Getenv("DATABASE_URL")
	if bouncer == "" || base == "" {
		t.Skip("PGBOUNCER_URL or DATABASE_URL not set")
	}
	simple := func(dsn string) string {
		u, err := url.Parse(dsn)
		require.NoError(t, err)
		q := u.Query()
		q.Set("default_query_exec_mode", "simple_protocol")
		u.RawQuery = q.Encode()
		return u.String()
	}
	viaBouncer := openLockTestDB(t, simple(bouncer))
	migrating := openLockTestDB(t, simple(bouncer))
	direct := openLockTestDB(t, base)

	for i := 0; i < 3; i++ {
		release, err := AcquireMigrationLock(context.Background(), viaBouncer)
		require.NoError(t, err)
		// The migrations run through the pooler meanwhile. PgBouncer hands
		// out the most recently used server connection first: the one the
		// lock was taken on, unless the lock's transaction still holds it.
		busy := make(chan error, 1)
		go func() { busy <- migrating.Exec("SELECT pg_sleep(1)").Error }()
		time.Sleep(200 * time.Millisecond)
		assert.False(t, tryLock(t, direct, 300*time.Millisecond), "held while migrating")
		release()
		require.NoError(t, <-busy)
		assert.True(t, tryLock(t, direct, 3*time.Second), "released after release (round %d)", i)
	}
}
