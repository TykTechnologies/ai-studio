package cluster

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loseEventsAfter makes the table what a new primary that had not received
// the newest events would hold: the rows after id are gone and the sequence
// hands their ids out again.
func (c *testCluster) loseEventsAfter(id int64) {
	c.t.Helper()
	db := c.replicaDB("failover")
	require.NoError(c.t, db.Exec("DELETE FROM cluster_events WHERE id > ?", id).Error)
	require.NoError(c.t, db.Exec("SELECT setval(pg_get_serial_sequence('cluster_events', 'id'), ?, true)", id).Error)
}

func lastEventID(t *testing.T, r *replica) int64 {
	t.Helper()
	evs := r.received()
	require.NotEmpty(t, evs)
	return evs[len(evs)-1].ID
}

// After a failover that lost the newest events, the ids of the lost rows
// are handed out again. A reader that had handled the lost rows must still
// deliver the new events that took their ids, at once and without being
// told of the failover: rows are told apart by what they are.
func TestEventThatTakesALostEventsIDIsDelivered_Postgres(t *testing.T) {
	c := newTestCluster(t)
	a := c.startReplica("a", LogOptions{PollInterval: 100 * time.Millisecond})
	b := c.startReplica("b", LogOptions{PollInterval: 100 * time.Millisecond})

	publish(t, a, "before", "1")
	b.waitFor(t, 1, 5*time.Second)
	kept := lastEventID(t, b)
	publish(t, a, "lost", "2")
	publish(t, a, "lost", "3")
	b.waitFor(t, 3, 5*time.Second)
	require.Equal(t, kept+2, b.log.Stats().Cursor)

	// created_at is what tells the new rows from the lost ones.
	time.Sleep(5 * time.Millisecond)
	c.loseEventsAfter(kept)
	publish(t, a, "after", "4")
	publish(t, a, "after", "5")

	got := b.waitFor(t, 5, 5*time.Second)
	assert.Equal(t, []string{"before", "lost", "lost", "after", "after"}, b.topics())
	assert.Equal(t, kept+1, got[3].ID, "the new event has the id of a lost one")
	assert.Equal(t, "4", string(got[3].Payload))
	assert.Equal(t, "5", string(got[4].Payload))
}

// The same failover seen by a replica that could not read for a while (its
// connections were down through the failover): when it reads again the
// table's newest id is below its cursor. It moves the cursor back, counts
// it, and delivers what was published since, including events older than
// the window that took ids it had passed.
func TestCursorAheadOfTheTableIsReset_Postgres(t *testing.T) {
	c := newTestCluster(t)
	a := c.startReplica("a", LogOptions{PollInterval: 100 * time.Millisecond})

	// b's clock can be moved, to make it look away for longer than the window.
	var away atomic.Int64
	b := &replica{name: "b", db: c.replicaDB("b"), counts: map[int64]int{}}
	b.log = NewLog(b.db, "b", LogOptions{PollInterval: time.Hour, Window: time.Second, Retention: time.Hour, ListenerDSN: "postgres://127.0.0.1:1/none?connect_timeout=1"})
	b.log.now = func() time.Time { return time.Now().Add(time.Duration(away.Load())) }
	b.log.Subscribe("*", func(ev Event) {
		b.mu.Lock()
		b.got = append(b.got, ev)
		b.counts[ev.ID]++
		b.mu.Unlock()
	})
	require.NoError(t, b.log.Start(context.Background()))
	t.Cleanup(b.log.Stop)

	publish(t, a, "before", "1")
	publish(t, a, "lost", "2")
	publish(t, a, "lost", "3")
	publish(t, a, "lost", "4")
	b.log.poke()
	b.waitFor(t, 4, 5*time.Second)
	kept := b.received()[0].ID

	// The failover loses three events; one is published on the new primary
	// and then b stays away for longer than its window (1 s).
	c.loseEventsAfter(kept)
	publish(t, a, "after", "5")
	time.Sleep(1500 * time.Millisecond)
	// Unnoticed, the event would never be read: its id is below b's cursor
	// and it is older than the window.
	b.log.poke()
	time.Sleep(300 * time.Millisecond)
	require.Len(t, b.received(), 4, "without a sign of the failover the reader does not look that far back")

	// The sign: the listener reconnected (or reads failed and work again).
	away.Store(int64(1500 * time.Millisecond))
	b.log.checkHead.Store(true)
	b.log.poke()
	got := b.waitFor(t, 5, 5*time.Second)
	assert.Equal(t, "5", string(got[4].Payload))
	assert.Equal(t, kept+1, got[4].ID)

	stats := b.log.Stats()
	assert.EqualValues(t, 1, stats.CursorResets)
	assert.False(t, stats.LastCursorReset.IsZero())
	assert.Equal(t, kept+1, stats.Cursor)

	// Reading goes on from there, and nothing was delivered twice.
	publish(t, a, "later", "6")
	b.log.poke()
	b.waitFor(t, 6, 5*time.Second)
	b.log.checkHead.Store(true)
	b.log.poke()
	time.Sleep(200 * time.Millisecond)
	assert.EqualValues(t, 1, b.log.Stats().CursorResets, "a table that is not behind the cursor is not a reset")
	// Every event once, in order (ids repeat here: two events took lost ones').
	var payloads []string
	for _, ev := range b.received() {
		payloads = append(payloads, string(ev.Payload))
	}
	assert.Equal(t, []string{"1", "2", "3", "4", "5", "6"}, payloads)
}

// A replica that could not read the log for longer than its retention has
// missed events for good: it says so, once per gap, to the handlers that
// rebuild from the database.
func TestGapLongerThanRetentionIsReported_Postgres(t *testing.T) {
	c := newTestCluster(t)
	var away atomic.Int64
	var gaps atomic.Int32
	db := c.replicaDB("b")
	log := NewLog(db, "b", LogOptions{PollInterval: 50 * time.Millisecond, Window: time.Second, Retention: time.Minute})
	log.opts.ListenerDSN = c.dsn("b")
	log.now = func() time.Time { return time.Now().Add(time.Duration(away.Load())) }
	var reported atomic.Int64
	log.OnGap(func(away time.Duration) { reported.Store(int64(away)); gaps.Add(1) })
	log.OnGap(func(time.Duration) { panic("a handler that panics does not stop the log") })
	require.NoError(t, log.Start(context.Background()))
	t.Cleanup(log.Stop)

	// Reading every 50 ms, and within the retention: no gap.
	away.Store(int64(30 * time.Second))
	time.Sleep(300 * time.Millisecond)
	require.Zero(t, gaps.Load())
	require.Zero(t, log.Stats().Gaps)

	// The next read comes more than a minute after the last one.
	away.Store(int64(2 * time.Minute))
	require.Eventually(t, func() bool { return gaps.Load() == 1 }, 5*time.Second, 20*time.Millisecond)
	time.Sleep(300 * time.Millisecond)
	assert.EqualValues(t, 1, gaps.Load(), "one gap is reported once")
	assert.InDelta(t, 90, time.Duration(reported.Load()).Seconds(), 5, "with how long the replica was away")
	stats := log.Stats()
	assert.EqualValues(t, 1, stats.Gaps)
	assert.False(t, stats.LastGap.IsZero())

	// It still delivers.
	a := c.startReplica("a", LogOptions{})
	var got atomic.Int32
	log.Subscribe("x", func(Event) { got.Add(1) })
	publish(t, a, "x", "1")
	require.Eventually(t, func() bool { return got.Load() == 1 }, 5*time.Second, 20*time.Millisecond)
}

// The read after a failover looks further back than an ordinary one: over
// the time since the last read plus two windows, on a clock that may be
// behind the one that stamped the rows. An event this replica handled
// before the failover, old enough to be outside an ordinary read's window,
// is inside that look-back; it must not be handled again. What is
// remembered about handled rows is kept for three windows for that.
func TestEventHandledBeforeAFailoverIsNotHandledAgain_Postgres(t *testing.T) {
	c := newTestCluster(t)
	a := c.startReplica("a", LogOptions{PollInterval: 50 * time.Millisecond})

	var ahead atomic.Int64
	b := &replica{name: "b", db: c.replicaDB("b"), counts: map[int64]int{}}
	b.log = NewLog(b.db, "b", LogOptions{PollInterval: 50 * time.Millisecond, Window: time.Second, Retention: time.Hour})
	b.log.opts.ListenerDSN = c.dsn("b")
	b.log.now = func() time.Time { return time.Now().Add(time.Duration(ahead.Load())) }
	b.log.Subscribe("*", func(ev Event) {
		b.mu.Lock()
		b.got = append(b.got, ev)
		b.counts[ev.ID]++
		b.mu.Unlock()
	})
	require.NoError(t, b.log.Start(context.Background()))
	t.Cleanup(b.log.Stop)

	publish(t, a, "before", "1")
	b.waitFor(t, 1, 5*time.Second)

	// 2.3 s later, with b reading all the while: the event is outside the
	// window (1 s) and past two windows since it was handled.
	time.Sleep(2300 * time.Millisecond)
	// The failover: b was away for 0.9 s by its reckoning (equally: the new
	// primary's clock is that much behind), so it looks back 0.9 s plus two
	// windows, 2.9 s, which reaches the event.
	ahead.Store(int64(900 * time.Millisecond))
	b.log.checkHead.Store(true)
	b.log.poke()
	time.Sleep(500 * time.Millisecond)

	b.assertNoDuplicates(t)
	assert.Len(t, b.received(), 1, "handled once")

	// And it still delivers what is new.
	publish(t, a, "after", "2")
	b.waitFor(t, 2, 5*time.Second)
	b.assertNoDuplicates(t)
}
