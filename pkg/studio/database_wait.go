package studio

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// A Studio that starts while its Postgres database is failing over, or
// before the database is up at all, waits for it rather than exiting:
// waitForDatabase tries until the server answers and takes writes (New
// migrates, which a standby refuses), for up to DatabaseStartupTimeout.
// SQLite has no server to wait for and is tried once, as is Postgres with a
// timeout of 0.

// Retry delays: the first retry comes quickly (a database that is just
// starting), later ones no more often than every few seconds.
var (
	startupRetryFirst = 500 * time.Millisecond
	startupRetryMax   = 5 * time.Second
	// startupProgressEvery is how often a wait that has not ended says so.
	startupProgressEvery = 10 * time.Second
)

// writableProbe asks the server whether it refuses writes: a standby (in
// recovery), or a demoted primary with read-only sessions.
const writableProbe = `SELECT pg_is_in_recovery() OR current_setting('transaction_read_only') = 'on'`

// errReadOnlyServer is a database that answers but does not take writes.
var errReadOnlyServer = errors.New("the database server answers but is read-only (a hot standby, or a demoted primary); migrations need a writable one")

// waitForDatabase returns once db answers, takes writes (Postgres) and then
// also passes ready (creating the schema, which needs a writable server),
// or with an error once conf.DatabaseStartupTimeout has passed. Each
// distinct reason for waiting is logged once.
func waitForDatabase(conf *config.AppConf, db *gorm.DB, ready func() error) error {
	timeout := conf.DatabaseStartupTimeout
	if conf.DatabaseType != "postgres" {
		timeout = 0
	}
	started := time.Now()
	deadline := started.Add(timeout)
	delay := startupRetryFirst
	lastReason, lastProgress := "", started
	var lastErr error
	for attempts := 1; ; attempts++ {
		err := databaseReady(db, deadline, timeout, ready)
		if errors.Is(err, context.DeadlineExceeded) && lastErr != nil {
			// The attempt was cut short by the deadline: the reason to
			// report is the one before.
			err = lastErr
		}
		lastErr = err
		if err == nil {
			if attempts > 1 {
				logger.Infof("The database is reachable and writable, after %s (%d attempts)", time.Since(started).Round(time.Millisecond), attempts)
			}
			return nil
		}
		if timeout <= 0 {
			if errors.Is(err, errReadOnlyServer) {
				return fmt.Errorf("studio: %w", err)
			}
			return err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("studio: no writable database within DATABASE_STARTUP_TIMEOUT (%s, %d attempts): %w", timeout, attempts, err)
		}
		if reason := waitReason(err); reason != lastReason {
			logger.Warnf("Waiting for the database (up to DATABASE_STARTUP_TIMEOUT, %s): %v", timeout, err)
			lastReason = reason
		} else if time.Since(lastProgress) >= startupProgressEvery {
			logger.Infof("Still waiting for the database: %s of %s", time.Since(started).Round(time.Second), timeout)
			lastProgress = time.Now()
		}
		time.Sleep(min(delay, remaining))
		delay = min(2*delay, startupRetryMax)
	}
}

// databaseReady makes one attempt: a ping, the read-only check on Postgres,
// then ready.
func databaseReady(db *gorm.DB, deadline time.Time, timeout time.Duration, ready func() error) error {
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline)
		defer cancel()
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return err
	}
	if db.Dialector.Name() == "postgres" {
		var readOnly bool
		if err := sqlDB.QueryRowContext(ctx, writableProbe).Scan(&readOnly); err != nil {
			return err
		}
		if readOnly {
			return errReadOnlyServer
		}
	}
	return ready()
}

// waitReason is the kind of failure an attempt met, so that the wait logs
// each kind once: the error's text differs from attempt to attempt (pgx
// names the local port of every connection it tried).
func waitReason(err error) string {
	var pgErr *pgconn.PgError
	switch {
	case errors.Is(err, errReadOnlyServer):
		return "read-only server"
	case strings.Contains(err.Error(), "read only connection"):
		// target_session_attrs=read-write refused every server that answered.
		return "no read-write server"
	case errors.As(err, &pgErr):
		return "server error " + pgErr.Code
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "timeout"):
		return "timeout"
	case strings.Contains(err.Error(), "failed to connect"):
		return "cannot connect"
	default:
		return err.Error()
	}
}
