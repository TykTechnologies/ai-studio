package pushes

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// The database's clock is compared with this replica's monotonic clock: a
// change of two seconds or more between two readings is a step, smaller
// ones (round trips, drift) are not, and a slow reading says nothing.
func TestDBClockObserve(t *testing.T) {
	var k dbClock
	t0 := time.Now()
	at := func(d time.Duration) time.Time { return t0.Add(d) }
	read := func(local, offset time.Duration) time.Duration {
		return k.observe(at(local), at(local+5*time.Millisecond), at(local).Add(offset))
	}

	assert.Zero(t, read(0, 300*time.Millisecond), "the first reading is the baseline")
	assert.Zero(t, read(time.Second, 350*time.Millisecond))
	assert.Zero(t, read(time.Hour, 1200*time.Millisecond), "drift")
	assert.False(t, k.pending(0))

	assert.InDelta(t, 30, read(time.Hour+time.Second, 31200*time.Millisecond).Seconds(), 0.01, "the new server's clock is 30 s ahead")
	assert.True(t, k.pending(0))
	assert.Zero(t, read(time.Hour+2*time.Second, 31200*time.Millisecond), "one step is reported once")
	assert.InDelta(t, -45, read(time.Hour+3*time.Second, -13800*time.Millisecond).Seconds(), 0.01, "and back, 45 s")
	assert.InDelta(t, -15, k.step.Seconds(), 0.01, "steps not yet acted on add up")

	// A reading that took a second to come back is not used.
	assert.Zero(t, k.observe(at(2*time.Hour), at(2*time.Hour+time.Second), at(2*time.Hour).Add(time.Minute)))

	var fresh dbClock
	assert.True(t, fresh.pending(1), "a failover the database tracker counted is looked at too")
}

// pushClockDB is a Postgres schema with one push sent by node-b and still
// open, and the two replicas' coordinators as started.
func pushClockDB(t *testing.T, body func(t *testing.T, db *gorm.DB, a, b *replica, op string)) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set")
	}
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		if db.Dialector.Name() != "postgres" {
			t.Skip("the database's clock is only another machine's on Postgres")
		}
		a := newReplica(t, db, "node-a", Options{})
		b := newReplica(t, db, "node-b", Options{})
		for _, r := range []*replica{a, b} {
			r.c.clock.identify = func(*gorm.DB) (string, error) { return "10.0.0.1:5432/1", nil }
			r.c.startClock()
			r.c.now() // the baseline reading
			// Long past their start: the grace a starting replica gives
			// registrations is over.
			r.c.clock.mu.Lock()
			r.c.clock.graceUntil = time.Time{}
			r.c.clock.mu.Unlock()
		}
		addEdge(t, db, "edge-1", "default", models.EdgeStatusConnected, "node-b")
		b.streams.open("edge-1", "sb")
		op := push(t, a.c, Request{Scope: ScopeEdge, EdgeIDs: []string{"edge-1"}}).Operation.OperationID
		b.c.dispatch()
		require.Equal(t, models.PushCommandSent, command(t, db, op, "edge-1").Status)
		body(t, db, a, b, op)
	})
}

func clockMarkers(t *testing.T, db *gorm.DB) []models.ClusterCounter {
	t.Helper()
	var rows []models.ClusterCounter
	require.NoError(t, db.Where("name LIKE ?", clockMarkerPrefix+"%").Order("name").Find(&rows).Error)
	return rows
}

// failOverToClockAhead makes the database look as it does after a failover
// to a server whose clock is ahead by d: every stored time is d older than
// it was, and each coordinator's next reading of the clock shows the step.
func failOverToClockAhead(t *testing.T, db *gorm.DB, d time.Duration, server string, rs ...*replica) {
	t.Helper()
	secs := d.Seconds()
	require.NoError(t, db.Exec(`UPDATE edge_push_commands SET deadline_at = deadline_at - make_interval(secs => ?), sent_at = sent_at - make_interval(secs => ?),
		claim_expires_at = claim_expires_at - make_interval(secs => ?), phase_at = phase_at - make_interval(secs => ?)`, secs, secs, secs, secs).Error)
	require.NoError(t, db.Exec(`UPDATE push_operations SET deadline_at = deadline_at - make_interval(secs => ?)`, secs).Error)
	require.NoError(t, db.Exec(`UPDATE cluster_nodes SET last_seen = last_seen - make_interval(secs => ?)`, secs).Error)
	for _, r := range rs {
		r.c.clock.identify = func(*gorm.DB) (string, error) { return server, nil }
		r.c.clock.mu.Lock()
		r.c.clock.offset -= d // the next reading is d ahead of the last
		r.c.clock.mu.Unlock()
	}
}

// A failover to a server whose clock is 30 s ahead, with a push in flight:
// the replica that sent it looks dead (its registration is 30 s old by the
// new clock). For one liveness window its command is not requeued as a dead
// replica's; the new server is recorded.
func TestClockAheadAfterFailoverDoesNotRequeueLiveReplicasCommands(t *testing.T) {
	pushClockDB(t, func(t *testing.T, db *gorm.DB, a, b *replica, op string) {
		before := command(t, db, op, "edge-1")
		failOverToClockAhead(t, db, 15*time.Second, "10.0.0.2:5432/2", a, b)

		a.c.janitor()
		b.c.janitor()

		after := command(t, db, op, "edge-1")
		assert.Equal(t, models.PushCommandSent, after.Status, "not requeued although node-b looked dead: %s", after.Message)
		assert.Equal(t, before.Attempts, after.Attempts)
		assert.Equal(t, []models.ClusterCounter{{Name: "push-clock:10.0.0.2:5432/2", Value: clockMarkerSeen}}, clockMarkers(t, db))
		assert.False(t, a.c.clock.pending(0))
	})
}

// The same step without a new server (the server's own clock was
// corrected) moves nothing: there is no telling which stored times were
// stamped before it. The liveness grace still applies.
func TestClockStepOnTheSameServerMovesNothing(t *testing.T) {
	pushClockDB(t, func(t *testing.T, db *gorm.DB, a, b *replica, op string) {
		before := command(t, db, op, "edge-1")
		failOverToClockAhead(t, db, 30*time.Second, "10.0.0.1:5432/1", a, b)

		a.c.janitor()

		after := command(t, db, op, "edge-1")
		assert.WithinDuration(t, before.DeadlineAt.Add(-30*time.Second), after.DeadlineAt, time.Millisecond)
		assert.Equal(t, models.PushCommandSent, after.Status, "node-b's command is not requeued while registrations may be stale: %s", after.Message)
		assert.False(t, a.c.clock.pending(0))
	})
}
