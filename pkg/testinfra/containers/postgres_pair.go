package containers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/network"
)

// A PostgresPair is two PostgreSQL nodes, a primary and a streaming hot
// standby, that a test can fail over: the rig behind the active-active tests
// (features/ActiveActive.md, Phase 0). See the README for an example.
//
// Each node is a container whose PostgreSQL server is started and stopped
// inside it, so a node keeps its address for the life of the pair: stopping a
// node leaves its port answering "connection refused" (or a reset), and
// starting it again brings it back on the same host and port. That is what
// lets one multi-host DSN (DSN) stay valid across any number of failovers.
//
// The pair does not fail over by itself. The test plays the part of the
// failover manager (Patroni, CloudNativePG, a managed service): it stops,
// promotes, demotes and rebuilds nodes, and the code under test has to follow.
//
// A pair is safe for use by several goroutines; its operations run one at a
// time.
type PostgresPair struct {
	cfg     PostgresPairConfig
	network *testcontainers.DockerNetwork
	nodes   [2]*PostgresNode

	mu      sync.Mutex
	primary int // index into nodes of the node the pair treats as primary
	sync    bool
	clock   bool
}

// PostgresPairConfig configures a PostgresPair. The zero value is usable.
type PostgresPairConfig struct {
	// Version is the postgres image tag. Defaults to "16-alpine", the image
	// the Go Postgres Tests job already runs.
	Version string
	// User is the superuser the nodes are created with. Defaults to
	// "postgres". Authentication is "trust": any password is accepted.
	User string
	// Password is only put in the DSNs. Defaults to "postgres".
	Password string
	// Database is created on the primary. Defaults to "studio".
	Database string
	// Synchronous makes the standby a synchronous one from the start: a
	// commit on the primary returns once the standby has it, so a failover
	// loses nothing that was acknowledged. Without it replication is
	// asynchronous. SetSynchronous changes it later.
	Synchronous bool
	// ConnectTimeout is the connect_timeout put in the DSNs, rounded to
	// whole seconds. Defaults to 2s.
	ConnectTimeout time.Duration
}

// PostgresNode is one node of a PostgresPair.
type PostgresNode struct {
	pair      *PostgresPair
	name      string // also its alias on the pair's network
	container testcontainers.Container
	host      string
	port      string

	// Guarded by the pair's mutex.
	running  bool
	settings map[string]string // rig.conf, rewritten whole on every change
}

const (
	// pgPairRequiredEnv makes StartPostgresPair fail instead of skip when
	// Docker is unavailable. scripts/ci-postgres-tests.sh sets it, so the
	// failover tests cannot pass in CI by skipping.
	pgPairRequiredEnv = "PGPAIR_REQUIRED"

	// pgPairClockSchema holds the functions that shadow pg_catalog.now()
	// and friends once EnableClockOffset has run.
	pgPairClockSchema = "rigclock"
	pgPairClockGUC    = "rig.clock_offset_seconds"
)

func (c PostgresPairConfig) withDefaults() PostgresPairConfig {
	if c.Version == "" {
		c.Version = "16-alpine"
	}
	if c.User == "" {
		c.User = "postgres"
	}
	if c.Password == "" {
		c.Password = "postgres"
	}
	if c.Database == "" {
		c.Database = "studio"
	}
	if c.ConnectTimeout < time.Second {
		c.ConnectTimeout = 2 * time.Second
	}
	return c
}

// StartPostgresPair starts a pair for a test and closes it when the test
// ends. The test is skipped under -short and when Docker is unavailable;
// with PGPAIR_REQUIRED set (as in the Go Postgres Tests job) a missing Docker
// fails the test instead.
func StartPostgresPair(t testing.TB, cfg *PostgresPairConfig) *PostgresPair {
	t.Helper()
	if testing.Short() {
		t.Skip("postgres pair: skipped under -short (it starts two containers)")
	}
	if err := dockerHealthy(); err != nil {
		if os.Getenv(pgPairRequiredEnv) != "" {
			t.Fatalf("postgres pair: Docker is unavailable and %s is set: %v", pgPairRequiredEnv, err)
		}
		t.Skipf("postgres pair: Docker is unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	pair, err := NewPostgresPair(ctx, cfg)
	if err != nil {
		t.Fatalf("postgres pair: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := pair.Close(ctx); err != nil {
			t.Logf("postgres pair: close: %v", err)
		}
	})
	return pair
}

// dockerHealthy reports why Docker cannot be used, or nil. Creating the
// provider panics rather than fails with some broken setups.
func dockerHealthy() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	provider, err := testcontainers.ProviderDocker.GetProvider()
	if err != nil {
		return err
	}
	defer provider.Close()
	return provider.Health(ctx)
}

// NewPostgresPair starts the two nodes ("pg-a" as primary, "pg-b" as its
// standby) and returns once the standby is streaming. Call Close when done.
func NewPostgresPair(ctx context.Context, cfg *PostgresPairConfig) (*PostgresPair, error) {
	p := &PostgresPair{}
	if cfg != nil {
		p.cfg = *cfg
	}
	p.cfg = p.cfg.withDefaults()

	nw, err := network.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create network: %w", err)
	}
	p.network = nw

	// The two containers start side by side; the image is the slow part.
	names := [2]string{"pg-a", "pg-b"}
	var wg sync.WaitGroup
	var errs [2]error
	for i := range names {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p.nodes[i], errs[i] = p.startContainer(ctx, names[i])
		}(i)
	}
	wg.Wait()
	if err := errors.Join(errs[:]...); err != nil {
		_ = p.Close(context.WithoutCancel(ctx))
		return nil, err
	}

	if err := p.bootstrap(ctx); err != nil {
		_ = p.Close(context.WithoutCancel(ctx))
		return nil, err
	}
	return p, nil
}

func (p *PostgresPair) startContainer(ctx context.Context, name string) (*PostgresNode, error) {
	req := testcontainers.ContainerRequest{
		Image:        "postgres:" + p.cfg.Version,
		ExposedPorts: []string{"5432/tcp"},
		// The container only hosts the server; the rig starts and stops
		// PostgreSQL inside it with pg_ctl. An init process reaps the
		// daemons that outlive the exec sessions that started them.
		Entrypoint:     []string{"tail", "-f", "/dev/null"},
		Networks:       []string{p.network.Name},
		NetworkAliases: map[string][]string{p.network.Name: {name}},
		ConfigModifier: func(c *container.Config) { c.User = "postgres" },
		HostConfigModifier: func(hc *container.HostConfig) {
			init := true
			hc.Init = &init
		},
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	n := &PostgresNode{pair: p, name: name, container: c, settings: map[string]string{}}
	if n.host, err = c.Host(ctx); err != nil {
		return n, fmt.Errorf("%s: host: %w", name, err)
	}
	port, err := c.MappedPort(ctx, "5432")
	if err != nil {
		return n, fmt.Errorf("%s: mapped port: %w", name, err)
	}
	n.port = port.Port()
	return n, nil
}

func (p *PostgresPair) bootstrap(ctx context.Context) error {
	a, b := p.nodes[0], p.nodes[1]
	user := shellQuote(p.cfg.User)
	if _, err := a.sh(ctx, `
initdb -D "$PGDATA" -U `+user+` --auth=trust -E UTF8 >/dev/null
cat >> "$PGDATA/postgresql.conf" <<'EOF'
listen_addresses = '*'
max_connections = 200
wal_level = replica
hot_standby = on
max_wal_senders = 10
wal_keep_size = '256MB'
fsync = off
include_if_exists = 'rig.conf'
EOF
cat >> "$PGDATA/pg_hba.conf" <<'EOF'
host all all all trust
host replication all all trust
EOF
: > "$PGDATA/rig.conf"`); err != nil {
		return fmt.Errorf("initdb: %w", err)
	}
	if err := a.start(ctx); err != nil {
		return err
	}
	if p.cfg.Database != "postgres" {
		if _, err := a.sh(ctx, "createdb -U "+user+" "+shellQuote(p.cfg.Database)); err != nil {
			return fmt.Errorf("createdb: %w", err)
		}
	}
	p.primary = 0
	if err := p.rebuildStandby(ctx, b, a); err != nil {
		return err
	}
	if p.cfg.Synchronous {
		return p.setSynchronous(ctx, true)
	}
	return nil
}

// Close removes both nodes and their network.
func (p *PostgresPair) Close(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	var errs []error
	for _, n := range p.nodes {
		if n != nil && n.container != nil {
			errs = append(errs, n.container.Terminate(ctx))
		}
	}
	if p.network != nil {
		errs = append(errs, p.network.Remove(ctx))
	}
	return errors.Join(errs...)
}

// A returns the node that started as the primary ("pg-a").
func (p *PostgresPair) A() *PostgresNode { return p.nodes[0] }

// B returns the node that started as the standby ("pg-b").
func (p *PostgresPair) B() *PostgresNode { return p.nodes[1] }

// Primary returns the node the pair currently treats as the primary: the one
// last promoted, whether or not it is running. After StopPrimary and before
// Promote it is the stopped node.
func (p *PostgresPair) Primary() *PostgresNode {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.nodes[p.primary]
}

// Standby returns the other node: the standby, or after a failover the old
// primary that RejoinOldPrimary will turn into one.
func (p *PostgresPair) Standby() *PostgresNode {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.nodes[1-p.primary]
}

// DSN returns the multi-host connection string a Studio replica in an
// active-active deployment uses: both nodes, in the order A then B, with
// target_session_attrs=read-write so the driver settles on whichever node
// accepts writes. It stays valid across failovers.
func (p *PostgresPair) DSN() string {
	return p.dsn(p.nodes[0].Addr()+","+p.nodes[1].Addr(), "read-write")
}

func (p *PostgresPair) dsn(hosts, attrs string) string {
	q := url.Values{}
	q.Set("sslmode", "disable")
	q.Set("connect_timeout", strconv.Itoa(int(p.cfg.ConnectTimeout/time.Second)))
	if attrs != "" {
		q.Set("target_session_attrs", attrs)
	}
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(p.cfg.User, p.cfg.Password),
		Host:     hosts,
		Path:     "/" + p.cfg.Database,
		RawQuery: q.Encode(),
	}
	return u.String()
}

// Name returns "pg-a" or "pg-b".
func (n *PostgresNode) Name() string { return n.name }

// Addr returns the node's "host:port" as reachable from the test process.
func (n *PostgresNode) Addr() string { return net.JoinHostPort(n.host, n.port) }

// DSN returns a connection string for this node alone, whatever its role, so
// a test can hold a connection to the standby or to a demoted primary.
func (n *PostgresNode) DSN() string { return n.pair.dsn(n.Addr(), "") }

// Running reports whether the rig has the node's server started.
func (n *PostgresNode) Running() bool {
	n.pair.mu.Lock()
	defer n.pair.mu.Unlock()
	return n.running
}

// SQL runs one statement on the node with psql, over the local socket, and
// returns the unaligned, tuples-only output. It works on any running node,
// including a read-only one, and does not go through the code under test.
func (n *PostgresNode) SQL(ctx context.Context, statement string) (string, error) {
	return n.sql(ctx, n.pair.cfg.Database, statement)
}

func (n *PostgresNode) sql(ctx context.Context, database string, statements ...string) (string, error) {
	cmd := "psql -X -qAt -v ON_ERROR_STOP=1 -U " + shellQuote(n.pair.cfg.User) + " -d " + shellQuote(database)
	for _, s := range statements {
		cmd += " -c " + shellQuote(s)
	}
	return n.sh(ctx, cmd)
}

// InRecovery reports pg_is_in_recovery(): true on a standby.
func (n *PostgresNode) InRecovery(ctx context.Context) (bool, error) {
	out, err := n.sql(ctx, "postgres", "SELECT pg_is_in_recovery()")
	return out == "t", err
}

// Logs returns the node's PostgreSQL server log, for a failing test.
func (n *PostgresNode) Logs(ctx context.Context) string {
	out, _ := n.sh(ctx, "cat /tmp/postgres.log 2>/dev/null || true")
	return out
}

// StopPrimary stops the primary's server at once, as a crash would
// (pg_ctl stop -m immediate). Nothing is promoted: until Promote, the pair
// has no writable node. With asynchronous replication, commits the standby
// had not received are lost.
func (p *PostgresPair) StopPrimary(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.nodes[p.primary].stop(ctx, "immediate")
}

// Promote promotes the standby and returns once it accepts writes. The old
// primary must have been stopped (StopPrimary) or demoted first; it is left
// as it is, and RejoinOldPrimary rebuilds it as the new standby.
func (p *PostgresPair) Promote(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.promote(ctx)
}

func (p *PostgresPair) promote(ctx context.Context) error {
	old, next := p.nodes[p.primary], p.nodes[1-p.primary]
	if old.running && old.settings["default_transaction_read_only"] != "on" {
		return fmt.Errorf("promote %s: %s is still a running, writable primary (stop or demote it first)", next.name, old.name)
	}
	if !next.running {
		return fmt.Errorf("promote %s: it is not running", next.name)
	}
	if _, err := next.sh(ctx, `pg_ctl -D "$PGDATA" -w -t 30 promote >/dev/null`); err != nil {
		return fmt.Errorf("promote %s: %w", next.name, err)
	}
	if err := next.waitFor(ctx, "postgres", "SELECT NOT pg_is_in_recovery()", "leave recovery"); err != nil {
		return err
	}
	p.primary = 1 - p.primary
	p.sync = false // the new primary has no standby yet
	return nil
}

// Failover is StopPrimary followed by Promote: the unplanned loss of the
// primary's site with an automatic failover behind it.
func (p *PostgresPair) Failover(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.nodes[p.primary].stop(ctx, "immediate"); err != nil {
		return err
	}
	return p.promote(ctx)
}

// DemotePrimary moves the primary role to the standby while the old primary
// keeps running and keeps its connections: the case of a pooled connection
// left on a node that no longer takes writes. The old primary is made
// read-only (default_transaction_read_only, applied to open sessions on
// their next statement), the standby catches up and is promoted.
//
// On the old primary every write then fails with SQLSTATE 25006 and
// "SHOW transaction_read_only" answers "on", which is what
// target_session_attrs=read-write checks, so new connections through DSN go
// to the new primary. PostgreSQL cannot turn a running primary into a real
// standby, so pg_is_in_recovery() stays false there; for a node that is in
// recovery, connect to the standby (Standby().DSN()) or call
// RejoinOldPrimary, which restarts the node as one.
func (p *PostgresPair) DemotePrimary(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	old, next := p.nodes[p.primary], p.nodes[1-p.primary]
	if !old.running || !next.running {
		return errors.New("demote: both nodes must be running")
	}
	// Synchronous commits would wait for a standby that is about to leave.
	if err := old.set(ctx, map[string]string{"default_transaction_read_only": "on", "synchronous_standby_names": ""}); err != nil {
		return err
	}
	// Nothing new is written on the old primary now; let the standby
	// replay what was, so the promotion loses nothing.
	if err := old.waitFor(ctx, "postgres",
		"SELECT count(*) = 1 FROM pg_stat_replication WHERE replay_lsn >= pg_current_wal_lsn()",
		"see its standby caught up"); err != nil {
		return err
	}
	return p.promote(ctx)
}

// RejoinOldPrimary brings the former primary back as a hot standby of the
// current one, whether it was stopped or demoted: its data directory is
// rebuilt from the current primary (pg_basebackup), as a failover manager
// does for a node that diverged. It returns once the node is streaming, and
// restores synchronous replication if the pair was configured with it.
func (p *PostgresPair) RejoinOldPrimary(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	cur, old := p.nodes[p.primary], p.nodes[1-p.primary]
	if !cur.running {
		return fmt.Errorf("rejoin %s: the primary %s is not running", old.name, cur.name)
	}
	if err := p.rebuildStandby(ctx, old, cur); err != nil {
		return err
	}
	if p.cfg.Synchronous {
		return p.setSynchronous(ctx, true)
	}
	return nil
}

// Switchover is a planned move of the primary role: the primary shuts down
// cleanly (so the standby has everything, even with asynchronous
// replication), the standby is promoted, and the old primary rejoins as the
// standby. Calling it twice is a fail-over and fail-back.
func (p *PostgresPair) Switchover(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	cur, next := p.nodes[p.primary], p.nodes[1-p.primary]
	if !cur.running || !next.running {
		return errors.New("switchover: both nodes must be running")
	}
	if err := cur.stop(ctx, "fast"); err != nil {
		return err
	}
	if err := p.promote(ctx); err != nil {
		return err
	}
	if err := p.rebuildStandby(ctx, cur, next); err != nil {
		return err
	}
	if p.cfg.Synchronous {
		return p.setSynchronous(ctx, true)
	}
	return nil
}

// DetachStandby stops the standby's replication stream while both nodes
// keep running. Whatever is committed on the primary from then on is not on
// the standby, so a Failover loses it: the way to test what follows lost
// transactions (ids and sequences that go back). Replication must be
// asynchronous. RejoinOldPrimary after the failover clears it.
//
// A sequence is logged 32 values ahead, so the promoted standby continues
// above ids it never saw unless more than 32 were lost: lose at least 33
// rows to see an id repeat.
func (p *PostgresPair) DetachStandby(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	cur, sb := p.nodes[p.primary], p.nodes[1-p.primary]
	if p.sync {
		return errors.New("detach standby: replication is synchronous; commits would hang (SetSynchronous(false) first)")
	}
	if err := sb.set(ctx, map[string]string{"primary_conninfo": ""}); err != nil {
		return err
	}
	return cur.waitFor(ctx, "postgres", "SELECT count(*) = 0 FROM pg_stat_replication", "lose its standby")
}

// SetSynchronous turns synchronous replication on or off on the current
// primary. Turning it on returns once the standby is the synchronous one.
func (p *PostgresPair) SetSynchronous(ctx context.Context, on bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.setSynchronous(ctx, on)
}

func (p *PostgresPair) setSynchronous(ctx context.Context, on bool) error {
	cur := p.nodes[p.primary]
	names := ""
	if on {
		names = "*"
	}
	if err := cur.set(ctx, map[string]string{"synchronous_standby_names": names}); err != nil {
		return err
	}
	p.sync = on
	if !on {
		return nil
	}
	return cur.waitFor(ctx, "postgres",
		"SELECT count(*) = 1 FROM pg_stat_replication WHERE sync_state = 'sync'", "get a synchronous standby")
}

// EnableClockOffset lets SetClockOffset give each node a different clock.
// Two containers share the host's clock, so the rig fakes it where Studio
// reads it: now() (and transaction_timestamp, statement_timestamp,
// clock_timestamp) are shadowed in the pair's database by functions that add
// the node's offset, and the database's default search_path is set to
// ClockSearchPath() so unqualified calls reach them. Connections opened
// afterwards see it. Not shadowed: the CURRENT_TIMESTAMP and LOCALTIMESTAMP
// keywords, and calls qualified with pg_catalog.
//
// A test that sets its own search_path (a schema per test) must use
// ClockSearchPath(schema) for it, or the offset does not apply.
func (p *PostgresPair) EnableClockOffset(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	cur := p.nodes[p.primary]
	stmts := []string{"CREATE SCHEMA IF NOT EXISTS " + pgPairClockSchema}
	for _, fn := range []string{"now", "transaction_timestamp", "statement_timestamp", "clock_timestamp"} {
		volatility := "STABLE"
		if fn == "clock_timestamp" {
			volatility = "VOLATILE"
		}
		stmts = append(stmts, fmt.Sprintf(
			`CREATE OR REPLACE FUNCTION %s.%s() RETURNS timestamptz LANGUAGE sql %s AS $rig$ SELECT pg_catalog.%s() + pg_catalog.make_interval(secs => COALESCE(NULLIF(pg_catalog.current_setting('%s', true), ''), '0')::float8) $rig$`,
			pgPairClockSchema, fn, volatility, fn, pgPairClockGUC))
	}
	stmts = append(stmts, fmt.Sprintf(`ALTER DATABASE %s SET search_path = %s`,
		quoteIdent(p.cfg.Database), ClockSearchPath()))
	if _, err := cur.sql(ctx, p.cfg.Database, stmts...); err != nil {
		return fmt.Errorf("enable clock offset: %w", err)
	}
	p.clock = true
	return nil
}

// ClockSearchPath returns the search_path under which the clock offset
// applies, with the given schemas (default: "$user", public) searched first
// for tables. The clock schema comes after them, so tables are never created
// in it, and before pg_catalog, so its now() wins.
func ClockSearchPath(schemas ...string) string {
	parts := []string{`"$user"`, "public"}
	if len(schemas) > 0 {
		parts = parts[:0]
		for _, s := range schemas {
			parts = append(parts, quoteIdent(s))
		}
	}
	return strings.Join(append(parts, pgPairClockSchema, "pg_catalog"), ", ")
}

// SetClockOffset makes this node's database clock run ahead of (or, with a
// negative offset, behind) the real one. It belongs to the node, not the
// role: it survives promotion and RejoinOldPrimary. EnableClockOffset must
// have been called on the pair.
func (n *PostgresNode) SetClockOffset(ctx context.Context, offset time.Duration) error {
	n.pair.mu.Lock()
	defer n.pair.mu.Unlock()
	if !n.pair.clock {
		return errors.New("set clock offset: call EnableClockOffset on the pair first")
	}
	return n.set(ctx, map[string]string{pgPairClockGUC: strconv.FormatFloat(offset.Seconds(), 'f', -1, 64)})
}

// Stop stops this node's server whatever its role: "immediate" (a crash),
// "fast" (a clean shutdown) or "smart". Stopping the standby of a
// synchronous pair makes commits on the primary wait.
func (n *PostgresNode) Stop(ctx context.Context, mode string) error {
	n.pair.mu.Lock()
	defer n.pair.mu.Unlock()
	return n.stop(ctx, mode)
}

// Start starts a stopped node's server with the data it has. A stopped
// standby resumes streaming. A stopped old primary comes back as a second
// primary on the old timeline (a split brain, which a test may want); use
// RejoinOldPrimary to bring it back as a standby instead.
func (n *PostgresNode) Start(ctx context.Context) error {
	n.pair.mu.Lock()
	defer n.pair.mu.Unlock()
	return n.start(ctx)
}

// --- internals; the pair's mutex is held ---------------------------------

func (n *PostgresNode) start(ctx context.Context) error {
	if n.running {
		return nil
	}
	if _, err := n.sh(ctx, `pg_ctl -D "$PGDATA" -w -t 60 -l /tmp/postgres.log start >/dev/null`); err != nil {
		return fmt.Errorf("start %s: %w\n%s", n.name, err, n.Logs(ctx))
	}
	n.running = true
	return nil
}

func (n *PostgresNode) stop(ctx context.Context, mode string) error {
	if !n.running {
		return nil
	}
	if _, err := n.sh(ctx, `pg_ctl -D "$PGDATA" -w -t 60 -m `+shellQuote(mode)+` stop >/dev/null`); err != nil {
		return fmt.Errorf("stop %s: %w", n.name, err)
	}
	n.running = false
	return nil
}

// set merges changes into the node's rig.conf (an empty value removes the
// key) and, if the server is running, reloads it and waits for the new
// values to be in effect.
func (n *PostgresNode) set(ctx context.Context, changes map[string]string) error {
	for k, v := range changes {
		if v == "" {
			delete(n.settings, k)
		} else {
			n.settings[k] = v
		}
	}
	keys := make([]string, 0, len(n.settings))
	for k := range n.settings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var conf strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&conf, "%s = '%s'\n", k, strings.ReplaceAll(n.settings[k], "'", "''"))
	}
	if _, err := n.sh(ctx, `printf %s `+shellQuote(conf.String())+` > "$PGDATA/rig.conf"`); err != nil {
		return fmt.Errorf("%s: write rig.conf: %w", n.name, err)
	}
	if !n.running {
		return nil
	}
	if _, err := n.sql(ctx, "postgres", "SELECT pg_reload_conf()"); err != nil {
		return fmt.Errorf("%s: reload: %w", n.name, err)
	}
	// The reload is a signal; a new session picks the file up once the
	// postmaster has processed it.
	for k := range changes {
		want := n.settings[k]
		check := fmt.Sprintf("SELECT COALESCE(current_setting(%s, true), '') = %s", quoteLiteral(k), quoteLiteral(want))
		if err := n.waitFor(ctx, "postgres", check, "apply "+k); err != nil {
			return err
		}
	}
	return nil
}

// rebuildStandby wipes node and brings it up as a streaming hot standby of
// primary.
func (p *PostgresPair) rebuildStandby(ctx context.Context, node, primary *PostgresNode) error {
	if err := node.stop(ctx, "immediate"); err != nil {
		return err
	}
	user := shellQuote(p.cfg.User)
	if _, err := node.sh(ctx, `
find "$PGDATA" -mindepth 1 -delete
pg_basebackup -h `+primary.name+` -p 5432 -U `+user+` -D "$PGDATA" -X stream -c fast
chmod 700 "$PGDATA"
touch "$PGDATA/standby.signal"`); err != nil {
		return fmt.Errorf("rebuild %s from %s: %w", node.name, primary.name, err)
	}
	// The backup copied the primary's rig.conf. A node keeps only what
	// belongs to it (its clock), not what belonged to its last role.
	clock := node.settings[pgPairClockGUC]
	node.settings = map[string]string{
		"primary_conninfo": fmt.Sprintf("host=%s port=5432 user=%s application_name=%s", primary.name, p.cfg.User, node.name),
	}
	if clock != "" {
		node.settings[pgPairClockGUC] = clock
	}
	if err := node.set(ctx, nil); err != nil {
		return err
	}
	if err := node.start(ctx); err != nil {
		return err
	}
	return primary.waitFor(ctx, "postgres",
		"SELECT count(*) = 1 FROM pg_stat_replication WHERE state = 'streaming'",
		"see "+node.name+" streaming")
}

// waitFor polls a boolean query on the node until it is true.
func (n *PostgresNode) waitFor(ctx context.Context, database, query, what string) error {
	deadline := time.Now().Add(60 * time.Second)
	var last string
	var lastErr error
	for {
		last, lastErr = n.sql(ctx, database, query)
		if lastErr == nil && last == "t" {
			return nil
		}
		if ctx.Err() != nil || time.Now().After(deadline) {
			return fmt.Errorf("%s did not %s (last answer %q, error %v)\n%s", n.name, what, last, lastErr, n.Logs(ctx))
		}
		// A cancelled context ends the wait at once; the check above
		// then reports it with the last answer.
		pause := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			pause.Stop()
		case <-pause.C:
		}
	}
}

// sh runs a script in the node's container as the postgres user and returns
// its trimmed output; a non-zero exit is an error carrying the output.
func (n *PostgresNode) sh(ctx context.Context, script string) (string, error) {
	code, r, err := n.container.Exec(ctx, []string{"sh", "-ec", script}, tcexec.Multiplexed())
	if err != nil {
		return "", err
	}
	b, err := io.ReadAll(r)
	out := strings.TrimSpace(string(b))
	if err != nil {
		return out, err
	}
	if code != 0 {
		return out, fmt.Errorf("exit %d: %s", code, out)
	}
	return out, nil
}

func shellQuote(s string) string   { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
func quoteLiteral(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
func quoteIdent(s string) string   { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
