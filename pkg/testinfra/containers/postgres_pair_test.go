package containers_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

// These tests are the rig's own smoke tests: they go through the vendored
// gorm driver with the multi-host DSN, as Studio does, and check that each
// helper produces the situation its comment promises. They skip under -short
// and without Docker (see containers.StartPostgresPair).

type smokeRow struct {
	ID   uint `gorm:"primaryKey"`
	Note string
}

func (smokeRow) TableName() string { return "rig_smoke" }

func openPair(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open %s: %v", dsn, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// writeEventually inserts a row, retrying while the pool re-dials: after a
// failover the pool's connections are dead and the first statements fail.
// It returns how long the write took to get through.
func writeEventually(t *testing.T, db *gorm.DB, note string) time.Duration {
	t.Helper()
	start := time.Now()
	var err error
	for time.Since(start) < 20*time.Second {
		if err = db.Create(&smokeRow{Note: note}).Error; err == nil {
			return time.Since(start)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("write %q did not get through: %v", note, err)
	return 0
}

func notes(t *testing.T, n *containers.PostgresNode) string {
	t.Helper()
	out, err := n.SQL(context.Background(), "SELECT COALESCE(string_agg(note, ',' ORDER BY id), '') FROM rig_smoke")
	if err != nil {
		t.Fatalf("%s: read rows: %v", n.Name(), err)
	}
	return out
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func inRecovery(t *testing.T, n *containers.PostgresNode) bool {
	t.Helper()
	r, err := n.InRecovery(context.Background())
	if err != nil {
		t.Fatalf("%s: %v", n.Name(), err)
	}
	return r
}

func TestPostgresPair_FailoverAndFailback(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	started := time.Now()
	pair := containers.StartPostgresPair(t, &containers.PostgresPairConfig{Synchronous: true})
	t.Logf("pair up in %s; DSN %s", time.Since(started).Round(time.Millisecond), pair.DSN())

	if !strings.Contains(pair.DSN(), pair.A().Addr()+","+pair.B().Addr()) ||
		!strings.Contains(pair.DSN(), "target_session_attrs=read-write") {
		t.Fatalf("not a multi-host read-write DSN: %s", pair.DSN())
	}
	if pair.Primary() != pair.A() || inRecovery(t, pair.A()) || !inRecovery(t, pair.B()) {
		t.Fatal("expected pg-a as primary and pg-b in recovery")
	}

	db := openPair(t, pair.DSN())
	if err := db.AutoMigrate(&smokeRow{}); err != nil {
		t.Fatal(err)
	}
	writeEventually(t, db, "before")
	// Synchronous replication: the standby has the row once the write returned.
	if got := notes(t, pair.B()); got != "before" {
		t.Fatalf("standby rows = %q, want the acknowledged write", got)
	}

	// Fail over to B: the same *gorm.DB writes again after re-dialling.
	cycle := time.Now()
	if err := pair.Failover(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("failover took %s", time.Since(cycle).Round(time.Millisecond))
	if pair.Primary() != pair.B() || pair.A().Running() {
		t.Fatal("expected pg-b as primary and pg-a stopped")
	}
	t.Logf("first write after failover got through in %s", writeEventually(t, db, "on-b").Round(time.Millisecond))
	if got := notes(t, pair.B()); got != "before,on-b" {
		t.Fatalf("new primary rows = %q", got)
	}

	// The old primary comes back as a standby and receives what it missed.
	if err := pair.RejoinOldPrimary(ctx); err != nil {
		t.Fatal(err)
	}
	if !inRecovery(t, pair.A()) {
		t.Fatal("pg-a should be in recovery after rejoining")
	}
	if got := notes(t, pair.A()); got != "before,on-b" {
		t.Fatalf("rejoined standby rows = %q", got)
	}

	// Fail back to A, and bring B back.
	if err := pair.Failover(ctx); err != nil {
		t.Fatal(err)
	}
	writeEventually(t, db, "on-a-again")
	if err := pair.RejoinOldPrimary(ctx); err != nil {
		t.Fatal(err)
	}
	took := time.Since(cycle)
	t.Logf("fail over, rejoin, fail back, rejoin: %s", took.Round(time.Millisecond))
	if pair.Primary() != pair.A() || inRecovery(t, pair.A()) || !inRecovery(t, pair.B()) {
		t.Fatal("expected pg-a as primary again and pg-b in recovery")
	}
	writeEventually(t, db, "after")
	if got := notes(t, pair.B()); got != "before,on-b,on-a-again,after" {
		t.Fatalf("standby rows after failback = %q", got)
	}
	// The target is about 30 s; the bound leaves room for a loaded CI runner.
	if took > 90*time.Second {
		t.Fatalf("fail over and fail back took %s", took)
	}
}

func TestPostgresPair_Switchover(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Asynchronous on purpose: a clean shutdown still loses nothing.
	pair := containers.StartPostgresPair(t, nil)
	db := openPair(t, pair.DSN())
	if err := db.AutoMigrate(&smokeRow{}); err != nil {
		t.Fatal(err)
	}
	for i := range 20 {
		writeEventually(t, db, fmt.Sprint(i))
	}
	want := notes(t, pair.A())
	if err := pair.Switchover(ctx); err != nil {
		t.Fatal(err)
	}
	if pair.Primary() != pair.B() || !inRecovery(t, pair.A()) {
		t.Fatal("expected pg-b as primary and pg-a as its standby")
	}
	if got := notes(t, pair.B()); got != want {
		t.Fatalf("rows after switchover = %q, want %q", got, want)
	}
	writeEventually(t, db, "after")
	eventually(t, "the new standby to receive the write", func() bool { return notes(t, pair.A()) == want+",after" })
}

func TestPostgresPair_DemoteKeepsConnectionsOpen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pair := containers.StartPostgresPair(t, &containers.PostgresPairConfig{Synchronous: true})
	db := openPair(t, pair.DSN())
	if err := db.AutoMigrate(&smokeRow{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()

	// A connection the pool holds on the primary, as a replica has.
	held, err := sqlDB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if _, err := held.ExecContext(ctx, "INSERT INTO rig_smoke (note) VALUES ('before')"); err != nil {
		t.Fatal(err)
	}

	if err := pair.DemotePrimary(ctx); err != nil {
		t.Fatal(err)
	}
	if pair.Primary() != pair.B() || !pair.A().Running() {
		t.Fatal("expected pg-b as primary and pg-a still running")
	}

	// The held connection is still open, on a node that refuses writes.
	var one int
	if err := held.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("the held connection should still answer reads: %v", err)
	}
	_, err = held.ExecContext(ctx, "INSERT INTO rig_smoke (note) VALUES ('on-demoted')")
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "25006" {
		t.Fatalf("write on the demoted primary: got %v, want SQLSTATE 25006", err)
	}
	var readOnly string
	if err := held.QueryRowContext(ctx, "SHOW transaction_read_only").Scan(&readOnly); err != nil || readOnly != "on" {
		t.Fatalf("transaction_read_only = %q, %v; want on", readOnly, err)
	}

	// A new connection through the multi-host DSN lands on the new primary.
	fresh := openPair(t, pair.DSN())
	if err := fresh.Create(&smokeRow{Note: "on-b"}).Error; err != nil {
		t.Fatalf("write through a fresh connection: %v", err)
	}
	if got := notes(t, pair.B()); got != "before,on-b" {
		t.Fatalf("new primary rows = %q", got)
	}

	// And the demoted node can be turned into a real standby.
	if err := pair.RejoinOldPrimary(ctx); err != nil {
		t.Fatal(err)
	}
	if !inRecovery(t, pair.A()) || notes(t, pair.A()) != "before,on-b" {
		t.Fatalf("pg-a after rejoin: in recovery %v, rows %q", inRecovery(t, pair.A()), notes(t, pair.A()))
	}
}

func TestPostgresPair_DetachStandbyLosesLaterCommits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pair := containers.StartPostgresPair(t, nil)
	db := openPair(t, pair.DSN())
	if err := db.AutoMigrate(&smokeRow{}); err != nil {
		t.Fatal(err)
	}
	writeEventually(t, db, "kept")
	eventually(t, "the standby to receive the first row", func() bool { return notes(t, pair.B()) == "kept" })

	if err := pair.DetachStandby(ctx); err != nil {
		t.Fatal(err)
	}
	// More rows than a sequence pre-logs (32 values): with fewer, the
	// promoted standby's sequence is already past the lost ids and nothing
	// repeats.
	var lastLost uint
	for range 40 {
		lost := smokeRow{Note: "lost"}
		if err := db.Create(&lost).Error; err != nil {
			t.Fatal(err)
		}
		lastLost = lost.ID
	}
	if err := pair.Failover(ctx); err != nil {
		t.Fatal(err)
	}
	if got := notes(t, pair.B()); got != "kept" {
		t.Fatalf("new primary rows = %q, want only the row from before the detach", got)
	}
	// The sequence went back with the data: the next id repeats a lost one.
	writeEventually(t, db, "reused")
	var reused smokeRow
	if err := db.Where("note = ?", "reused").First(&reused).Error; err != nil {
		t.Fatal(err)
	}
	if reused.ID > lastLost {
		t.Fatalf("id after the failover = %d, want one not above the last lost row's %d", reused.ID, lastLost)
	}
	if err := pair.RejoinOldPrimary(ctx); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the rebuilt standby to match", func() bool { return notes(t, pair.A()) == "kept,reused" })
}

func TestPostgresPair_ClockOffsetPerNode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pair := containers.StartPostgresPair(t, nil)
	if err := pair.B().SetClockOffset(ctx, time.Second); err == nil {
		t.Fatal("SetClockOffset before EnableClockOffset should fail")
	}
	if err := pair.EnableClockOffset(ctx); err != nil {
		t.Fatal(err)
	}
	if err := pair.B().SetClockOffset(ctx, 7*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := pair.A().SetClockOffset(ctx, -3*time.Second); err != nil {
		t.Fatal(err)
	}

	// What Studio's "SELECT now()" and "now() + interval" see, per node.
	skew := func(db *gorm.DB) float64 {
		t.Helper()
		var s float64
		if err := db.Raw("SELECT EXTRACT(EPOCH FROM now() - pg_catalog.now())::float8").Scan(&s).Error; err != nil {
			t.Fatal(err)
		}
		return s
	}
	db := openPair(t, pair.DSN())
	if got := skew(db); got != -3 {
		t.Fatalf("clock offset on pg-a = %v s, want -3", got)
	}
	// Tables still go to public, not to the schema holding the clock.
	if err := db.AutoMigrate(&smokeRow{}); err != nil {
		t.Fatal(err)
	}
	if out, err := pair.A().SQL(ctx, "SELECT schemaname FROM pg_tables WHERE tablename = 'rig_smoke'"); err != nil || out != "public" {
		t.Fatalf("rig_smoke created in %q, %v; want public", out, err)
	}

	if err := pair.Failover(ctx); err != nil {
		t.Fatal(err)
	}
	writeEventually(t, db, "on-b")
	if got := skew(db); got != 7 {
		t.Fatalf("clock offset on pg-b after failover = %v s, want 7", got)
	}

	// The offset belongs to the node: pg-a keeps its own after a rebuild.
	if err := pair.RejoinOldPrimary(ctx); err != nil {
		t.Fatal(err)
	}
	a := openPair(t, pair.A().DSN())
	if got := skew(a); got != -3 {
		t.Fatalf("clock offset on rebuilt pg-a = %v s, want -3", got)
	}
	// A test with a schema of its own keeps the offset with ClockSearchPath.
	if err := a.Exec("SET search_path = " + containers.ClockSearchPath("public")).Error; err != nil {
		t.Fatal(err)
	}
}
