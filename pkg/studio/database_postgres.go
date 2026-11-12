package studio

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/pkg/dbstate"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Studio opens its Postgres pool itself, rather than through the gorm
// driver's DSN path, to follow a database failover without a restart:
//
//   - DATABASE_URL may name several hosts
//     (postgres://u:p@pg-a,pg-b/db?target_session_attrs=read-write); pgx
//     then connects to the one that takes writes.
//   - A connection to a node that was demoted stays open and fails every
//     write with SQLSTATE 25006. A dbstate.Tracker notices (from a refused
//     write, or its periodic probe) and raises its generation; connections
//     are stamped with the generation they were dialled under, and the pool
//     discards one with an older stamp when it would next be reused, so it
//     is dialled again, to the writable node.
//   - The pool's limits and connect timeout come from the configuration.

// generationKey is where a connection's generation stamp is kept
// (pgconn.PgConn.CustomData).
const generationKey = "studio.dbstate.generation"

// timeZoneInDSN finds a time zone parameter, as the gorm driver does when
// it opens a pool from a DSN: the zone is sent as a run-time parameter and
// timestamps without a zone are read in it.
var timeZoneInDSN = regexp.MustCompile("(time_zone|TimeZone|timezone)=(.*?)($|&| )")

// stateProbe is what the tracker asks the server: whether it is read-only
// (in recovery, or with read-only sessions, which is what a demoted primary
// that keeps running looks like and what target_session_attrs=read-write
// checks), and which node answers.
const stateProbe = `SELECT pg_is_in_recovery() OR current_setting('transaction_read_only') = 'on',
	COALESCE(host(inet_server_addr()), 'local') || ':' || COALESCE(inet_server_port()::text, '')`

// openPostgres opens the pool for dsn, with its tracker running, and
// returns the dialector over it. The tracker stops when the pool is closed.
func openPostgres(conf *config.AppConf, dsn string) (gorm.Dialector, *sql.DB, error) {
	connConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("studio: parse DATABASE_URL: %w", err)
	}
	if connConfig.ConnectTimeout == 0 && conf.DatabaseConnectTimeout > 0 {
		connConfig.ConnectTimeout = conf.DatabaseConnectTimeout
	}
	// Every host's connections notice a host that has gone (database_tcp.go).
	connConfig.DialFunc = deadPeerDialer(connConfig.ConnectTimeout).DialContext

	// The tracker is created after the pool it probes, and its first probe
	// dials the first connection: until it is stored the generation is 0
	// (a nil Tracker's).
	var tracker atomic.Pointer[dbstate.Tracker]
	options := []stdlib.OptionOpenDB{
		stdlib.OptionResetSession(func(_ context.Context, conn *pgx.Conn) error {
			if stamp, _ := conn.PgConn().CustomData()[generationKey].(uint64); stamp < tracker.Load().Generation() {
				// Dialled before the server was found read-only: database/sql
				// closes it and takes (or dials) another.
				return driver.ErrBadConn
			}
			return nil
		}),
	}
	var location *time.Location
	if zone := timeZoneInDSN.FindStringSubmatch(dsn); len(zone) > 2 {
		connConfig.RuntimeParams["timezone"] = zone[2]
		if location, err = time.LoadLocation(zone[2]); err != nil {
			return nil, nil, fmt.Errorf("studio: DATABASE_URL time zone: %w", err)
		}
	}
	options = append(options, stdlib.OptionAfterConnect(func(_ context.Context, conn *pgx.Conn) error {
		conn.PgConn().CustomData()[generationKey] = tracker.Load().Generation()
		if location != nil {
			conn.TypeMap().RegisterType(&pgtype.Type{
				Name:  "timestamp",
				OID:   pgtype.TimestampOID,
				Codec: &pgtype.TimestampCodec{ScanLocation: location},
			})
		}
		return nil
	}))

	pool := stdlib.OpenDB(*connConfig, options...)
	if conf.DatabaseMaxOpenConns > 0 {
		pool.SetMaxOpenConns(conf.DatabaseMaxOpenConns)
	}
	if conf.DatabaseMaxIdleConns > 0 {
		pool.SetMaxIdleConns(conf.DatabaseMaxIdleConns)
	}
	if conf.DatabaseConnMaxLifetime > 0 {
		pool.SetConnMaxLifetime(conf.DatabaseConnMaxLifetime)
	}
	logger.Debugf("Database pool: %d host(s), target_session_attrs=%s, max_open_conns=%s, max_idle_conns=%s, conn_max_lifetime=%s, connect_timeout=%s",
		hostCount(connConfig), targetSessionAttrs(dsn),
		orDefault(conf.DatabaseMaxOpenConns, "unlimited"), orDefault(conf.DatabaseMaxIdleConns, "2 (default)"),
		durationOr(conf.DatabaseConnMaxLifetime, "unlimited"), durationOr(connConfig.ConnectTimeout, "none"))

	started := dbstate.Start(func(ctx context.Context) (dbstate.Probe, error) {
		var p dbstate.Probe
		if err := pool.QueryRowContext(ctx, stateProbe).Scan(&p.ReadOnly, &p.Server); err != nil {
			return p, err
		}
		// The timeline tells a promotion behind an unchanged address (a
		// virtual IP). Not every service lets it be read: without it only
		// a changed address counts as a failover.
		_ = pool.QueryRowContext(ctx, "SELECT timeline_id FROM pg_control_checkpoint()").Scan(&p.Timeline)
		return p, nil
	}, dbstate.Options{ProbeTimeout: probeTimeout(connConfig)})
	tracker.Store(started)
	dbstate.Register(pool, started)

	// The DSN stays on the dialector: the LISTEN connection is opened from
	// it (pglisten.DSN).
	return postgres.New(postgres.Config{DSN: dsn, Conn: pool}), pool, nil
}

// probeTimeout is how long one probe of the tracker may take. A probe may
// have to dial, and pgx tries the connection string's hosts in order,
// waiting up to the connect timeout at each attempt (two per host when
// sslmode allows both a TLS and a plain connection). So the bound covers
// every attempt, and the query after them: with a shorter one, a first host
// that does not answer at all (a site that is down hard) would have the
// tracker report a database that is writable on its second host as
// unreachable.
func probeTimeout(c *pgx.ConnConfig) time.Duration {
	perAttempt := c.ConnectTimeout
	if perAttempt <= 0 {
		return dbstate.DefaultProbeTimeout
	}
	return time.Duration(1+len(c.Fallbacks))*perAttempt + 3*time.Second
}

// hostCount is how many servers the connection string names. pgx lists a
// host twice among its fallbacks when sslmode allows both a TLS and a plain
// attempt; that is one server.
func hostCount(c *pgx.ConnConfig) int {
	hosts := map[string]bool{fmt.Sprintf("%s:%d", c.Host, c.Port): true}
	for _, f := range c.Fallbacks {
		hosts[fmt.Sprintf("%s:%d", f.Host, f.Port)] = true
	}
	return len(hosts)
}

// watchReadOnly reports every statement the server refuses as read-only to
// the pool's tracker, so its connections are dialled again at once rather
// than at the next probe.
func watchReadOnly(db *gorm.DB) error {
	tracker := dbstate.For(db)
	if tracker == nil {
		return nil
	}
	report := func(tx *gorm.DB) {
		if tx.Error != nil && dbstate.IsReadOnlyError(tx.Error) {
			tracker.ReportReadOnly()
		}
	}
	const name = "studio:database_read_only"
	cb := db.Callback()
	for _, register := range []func() error{
		func() error { return cb.Create().After("gorm:create").Register(name, report) },
		func() error { return cb.Query().After("gorm:query").Register(name, report) },
		func() error { return cb.Update().After("gorm:update").Register(name, report) },
		func() error { return cb.Delete().After("gorm:delete").Register(name, report) },
		func() error { return cb.Row().After("gorm:row").Register(name, report) },
		func() error { return cb.Raw().After("gorm:raw").Register(name, report) },
	} {
		if err := register(); err != nil {
			return fmt.Errorf("studio: watch for a read-only database: %w", err)
		}
	}
	return nil
}

func targetSessionAttrs(dsn string) string {
	if m := regexp.MustCompile(`target_session_attrs=([a-z-]+)`).FindStringSubmatch(dsn); len(m) > 1 {
		return m[1]
	}
	if strings.Contains(dsn, ",") {
		return "any (unset: add target_session_attrs=read-write to follow the primary)"
	}
	return "any"
}

func orDefault(n int, unset string) string {
	if n > 0 {
		return fmt.Sprint(n)
	}
	return unset
}

func durationOr(d time.Duration, unset string) string {
	if d > 0 {
		return d.String()
	}
	return unset
}
