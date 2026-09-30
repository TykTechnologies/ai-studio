package models

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// migrationLockKey names the Postgres advisory lock Studio's migrations and
// seeding run under. It is hashed with the current schema, so Studio
// installations in different schemas of one database do not wait for each
// other.
const migrationLockKey = "tyk-ai-studio:migrate:"

// migrationLockNotice is how often a waiting instance logs that it is still
// waiting.
var migrationLockNotice = 30 * time.Second

// migrationLockKeepalive is how often the holder runs a statement in its
// transaction, so a server with idle_in_transaction_session_timeout does
// not end it while the migrations (on other connections) take their time.
var migrationLockKeepalive = 5 * time.Second

// DefaultMigrationLockWait bounds the wait for the migration lock when ctx
// has no deadline of its own.
const DefaultMigrationLockWait = 15 * time.Minute

// AcquireMigrationLock serialises schema migration and seeding between
// Studio instances that share a Postgres database, such as the replicas of
// a host that embeds Studio: AutoMigrate and the get-or-create seeds are
// not safe to run concurrently against one schema, and replicas booting
// together would race.
//
// It takes a transaction-level advisory lock in a transaction of its own,
// on a connection of its own, and returns the function that ends the
// transaction and so releases the lock; the migrations themselves run on
// the pool's other connections. A transaction-level lock is released by
// the server whatever happens to the connection, and it works behind
// PgBouncer in transaction mode, which keeps a transaction on one server
// connection (a session-level lock taken through it stayed behind on a
// pooled server connection and blocked every later start).
//
// It waits until the lock is free, logging while it waits, and gives up
// when ctx ends or, if ctx has no deadline, after DefaultMigrationLockWait.
// On SQLite, and on a pool limited to one connection (which the lock would
// starve), it does nothing.
func AcquireMigrationLock(ctx context.Context, db *gorm.DB) (release func(), err error) {
	noop := func() {}
	if db.Dialector.Name() != "postgres" {
		return noop, nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	if sqlDB.Stats().MaxOpenConnections == 1 {
		logger.Warn("Database pool is limited to one connection; migrating without the cross-instance migration lock")
		return noop, nil
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultMigrationLockWait)
		defer cancel()
	}

	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("migration lock: %w", err)
	}
	const lockSQL = "SELECT pg_try_advisory_xact_lock(hashtextextended($1 || current_schema(), 0))"

	started := time.Now()
	lastNotice := started
	var tx *sql.Tx
	for {
		// The transaction outlives ctx once the lock is held: it is ended
		// by release, not by the caller's deadline.
		tx, err = conn.BeginTx(context.Background(), nil)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("migration lock: %w", err)
		}
		var got bool
		if err := tx.QueryRowContext(ctx, lockSQL, migrationLockKey).Scan(&got); err != nil {
			_ = tx.Rollback()
			conn.Close()
			return nil, migrationLockWaitError(ctx, started, err)
		}
		if got {
			if waited := time.Since(started); waited > time.Second {
				logger.Infof("Acquired the database migration lock after %s", waited.Round(time.Second))
			}
			break
		}
		// Not ours: end the transaction rather than sit idle in it while
		// waiting.
		_ = tx.Rollback()
		if time.Since(lastNotice) >= migrationLockNotice || lastNotice == started {
			logger.Infof("Another Studio instance is migrating this database; waiting for it (%s so far)", time.Since(started).Round(time.Second))
			lastNotice = time.Now()
		}
		select {
		case <-ctx.Done():
			conn.Close()
			return nil, migrationLockWaitError(ctx, started, ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}

	// Keep the transaction busy while the lock is held.
	stop := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		tick := time.NewTicker(migrationLockKeepalive)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
			}
			kctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_, err := tx.ExecContext(kctx, "SELECT 1")
			cancel()
			if err != nil {
				logger.Warnf("The database migration lock was lost while this instance was migrating (%v); another instance starting now may migrate at the same time", err)
				return
			}
		}
	}()

	return func() {
		close(stop)
		<-stopped
		// Ending the transaction releases the lock. If that fails the
		// connection is broken; discard it (returning ErrBadConn from Raw
		// keeps it out of the pool) and the server drops the lock with the
		// session.
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			logger.Warnf("Releasing the database migration lock: %v; closing its connection instead", err)
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		conn.Close()
	}, nil
}

// migrationLockWaitError explains why the wait for the lock ended.
func migrationLockWaitError(ctx context.Context, started time.Time, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("migration lock: gave up after waiting %s for another Studio instance to finish migrating this database; "+
			"if no other instance is starting, look for a session holding the advisory lock (pg_locks, locktype 'advisory') "+
			"and end it, or raise MIGRATION_LOCK_TIMEOUT: %w", time.Since(started).Round(time.Second), err)
	}
	return fmt.Errorf("migration lock: %w", err)
}
