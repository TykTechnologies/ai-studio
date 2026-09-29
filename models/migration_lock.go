package models

import (
	"context"
	"database/sql/driver"
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

// AcquireMigrationLock serialises schema migration and seeding between
// Studio instances that share a Postgres database, such as the replicas of
// a host that embeds Studio: AutoMigrate and the get-or-create seeds are
// not safe to run concurrently against one schema, and replicas booting
// together would race.
//
// It takes a session-level advisory lock on a connection of its own and
// returns the function that releases it; the migrations themselves run on
// the pool's other connections. It waits until the lock is free or ctx
// ends, logging while it waits. On SQLite, and on a pool limited to one
// connection (which the lock would starve), it does nothing.
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

	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("migration lock: %w", err)
	}
	const keyExpr = "hashtextextended($1 || current_schema(), 0)"

	started := time.Now()
	lastNotice := started
	for {
		var got bool
		if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock("+keyExpr+")", migrationLockKey).Scan(&got); err != nil {
			conn.Close()
			return nil, fmt.Errorf("migration lock: %w", err)
		}
		if got {
			if waited := time.Since(started); waited > time.Second {
				logger.Infof("Acquired the database migration lock after %s", waited.Round(time.Second))
			}
			break
		}
		if time.Since(lastNotice) >= migrationLockNotice || lastNotice == started {
			logger.Infof("Another Studio instance is migrating this database; waiting for it (%s so far)", time.Since(started).Round(time.Second))
			lastNotice = time.Now()
		}
		select {
		case <-ctx.Done():
			conn.Close()
			return nil, fmt.Errorf("migration lock: %w", ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}

	return func() {
		// A fresh context: the unlock must run even when ctx has ended.
		unlockCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(unlockCtx, "SELECT pg_advisory_unlock("+keyExpr+")", migrationLockKey); err != nil {
			logger.Warnf("Releasing the database migration lock: %v; closing its connection instead", err)
			// Close would hand the connection, lock and all, back to the
			// pool. Returning ErrBadConn from Raw discards it, and Postgres
			// drops the lock with the session.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		conn.Close()
	}, nil
}
