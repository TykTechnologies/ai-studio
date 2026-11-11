package pushes

import (
	"fmt"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/pkg/cluster"
	"github.com/TykTechnologies/midsommar/v2/pkg/dbstate"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// A push is timed against the database's clock: its deadline, a claim's
// expiry and the answer timeout are timestamps every replica compares with
// the database's now(), so replicas whose own clocks differ agree.
//
// That clock is not one clock. After a failover it is the new primary's,
// which may be seconds ahead of or behind the old one's. Ahead, a claim
// made a moment ago looks lapsed, an edge that was sent a command a moment
// ago looks silent, every open push is past its deadline that much sooner,
// and every replica looks dead (its registration is that much older) until
// it refreshes it. Behind, everything takes that much longer, which loses
// nothing.
//
// So each coordinator watches the database's clock against its own
// monotonic clock. When the database's clock steps (clockStepMin or more
// between two readings):
//
//   - for one liveness window it does not requeue the commands of replicas
//     that look dead: their registrations were stamped by the old clock;
//   - it notes which database server the pushes' times are now on, in a
//     marker row (clockMarker), and says in its log what it saw.

// clockStepMin is the smallest change of the database's clock against this
// replica's that counts as a step: below it, a difference is the round trip
// of the reading or drift.
const clockStepMin = time.Second

// clockMarkerSeen is the value of a clock marker row: the pushes' times are
// known to be on this server's clock.
const clockMarkerSeen = 1

// dbClock is a coordinator's view of the database's clock.
type dbClock struct {
	mu sync.Mutex
	// base is a local time whose monotonic reading the offsets are measured
	// from (time.Since is monotonic: a step of this machine's wall clock is
	// not mistaken for one of the database's).
	base time.Time
	// offset is the database's clock minus this replica's at the last
	// reading; sampled says whether there has been one.
	offset  time.Duration
	sampled bool
	// step is the sum of the steps seen and not yet acted on.
	step time.Duration
	// graceUntil: until then replicas' registrations may carry the old
	// clock's stamps.
	graceUntil time.Time
	// server is the database server the pushes' times were last known to
	// be on ("" until it could be read); failovers the tracker's count
	// then.
	server    string
	failovers int64
	// identify is databaseServer, replaced in tests.
	identify func(*gorm.DB) (string, error)
}

func (k *dbClock) serverOf(db *gorm.DB) (string, error) {
	if k.identify != nil {
		return k.identify(db)
	}
	return databaseServer(db)
}

// observe notes a reading of the database's clock, taken between the local
// times before and after, and reports the step it reveals (0: none).
func (k *dbClock) observe(before, after, db time.Time) time.Duration {
	// A slow round trip says little about when the server read its clock.
	if after.Sub(before) > clockStepMin/4 { // 250 ms
		return 0
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.base.IsZero() {
		k.base = before
	}
	local := k.base.Add(before.Sub(k.base)) // base's wall time plus monotonic time since
	offset := db.Sub(local)
	if !k.sampled {
		k.offset, k.sampled = offset, true
		return 0
	}
	step := offset - k.offset
	k.offset = offset
	if step.Abs() < clockStepMin {
		return 0
	}
	k.step += step
	k.graceUntil = after.Add(cluster.LivenessWindow)
	return step
}

// pending reports whether a step waits to be acted on, or the database
// tracker counted a failover since the last look.
func (k *dbClock) pending(failovers int64) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.step != 0 || failovers != k.failovers
}

// inGrace reports whether replicas' registrations may still carry the old
// clock's stamps.
func (k *dbClock) inGrace() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return time.Now().Before(k.graceUntil)
}

// databaseServer identifies the primary answering: its address, and the
// timeline it writes on when that can be read (0 otherwise). Every
// promotion starts a new timeline, so each failover gives a new name, also
// back to a server that was primary before. The timeline is read from the
// name of the WAL file being written, which is right from the moment of the
// promotion (the control file's checkpoint is not, until the next one).
// Without it the name is the address alone, and only a promotion behind an
// unchanged address goes unseen: open pushes are then not moved.
func databaseServer(db *gorm.DB) (string, error) {
	var addr string
	if err := db.Raw("SELECT COALESCE(host(inet_server_addr()), 'local') || ':' || COALESCE(inet_server_port()::text, '')").Scan(&addr).Error; err != nil {
		return "", err
	}
	var timeline int64
	_ = db.Raw("SELECT ('x' || substr(pg_walfile_name(pg_current_wal_lsn()), 1, 8))::bit(32)::int").Scan(&timeline).Error
	return fmt.Sprintf("%s/%d", addr, timeline), nil
}

// clockMarkerPrefix starts the names of the marker rows. They live in
// cluster_counters, whose other rows are counters drawn by fixed names
// (models.NextClusterCounter); no counter's name starts like this, and a
// marker is never drawn as a counter. There is one row for the current
// server: recording a new one removes the others.
const clockMarkerPrefix = "push-clock:"

// clockMarker is the name of the row that says the pushes' times are on
// server's clock.
func clockMarker(server string) string {
	name := clockMarkerPrefix + server
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}

// startClock notes which database server the pushes' times are on, and
// makes sure its marker exists.
func (c *Coordinator) startClock() {
	if c.db.Dialector.Name() != "postgres" {
		return
	}
	// A replica that starts has no reading of the clock from before: if it
	// starts during or just after a failover, the registrations it reads
	// may carry the old clock's stamps. For its first liveness window it
	// leaves the commands of replicas that look dead alone; a replica that
	// really stopped is still seen as stopped after it.
	c.clock.mu.Lock()
	c.clock.graceUntil = time.Now().Add(cluster.LivenessWindow)
	c.clock.mu.Unlock()
	server, err := c.clock.serverOf(c.db)
	if err != nil {
		logger.Warnf("Edge pushes: could not identify the database server; after a failover open pushes keep their times even if the new server's clock differs: %v", err)
		return
	}
	if err := c.db.Exec("INSERT INTO cluster_counters (name, value) VALUES (?, ?) ON CONFLICT (name) DO NOTHING", clockMarker(server), clockMarkerSeen).Error; err != nil {
		logger.Warnf("Edge pushes: could not record the database server the pushes' times are on: %v", err)
		return
	}
	c.clock.mu.Lock()
	c.clock.server = server
	c.clock.failovers = dbstate.For(c.db).Status().Failovers
	c.clock.mu.Unlock()
}

// adoptClock acts on a step of the database's clock, or a failover the
// database tracker counted. It reports whether it could find out what
// happened; when not, the janitor leaves time-based decisions for its next
// round.
//
// The new server's marker is recorded by whichever replica looks first.
func (c *Coordinator) adoptClock() bool {
	failovers := dbstate.For(c.db).Status().Failovers
	server, err := c.clock.serverOf(c.db)
	if err != nil {
		logger.Warnf("Edge pushes: could not identify the database server after its clock changed; retrying: %v", err)
		return false
	}
	c.clock.mu.Lock()
	step, known := c.clock.step, c.clock.server
	c.clock.mu.Unlock()

	done := func() bool {
		c.clock.mu.Lock()
		c.clock.step -= step
		c.clock.server, c.clock.failovers = server, failovers
		c.clock.mu.Unlock()
		return true
	}
	if server == known {
		if step != 0 {
			logger.Warnf("Edge pushes: the database's clock moved by %s on the same server (%s); open pushes keep their times", step.Round(time.Millisecond), server)
		}
		return done()
	}

	err = c.db.Transaction(func(tx *gorm.DB) error {
		marker := clockMarker(server)
		res := tx.Exec("INSERT INTO cluster_counters (name, value) VALUES (?, ?) ON CONFLICT (name) DO NOTHING", marker, clockMarkerSeen)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 1 {
			// The markers of earlier servers have done their work.
			return tx.Exec("DELETE FROM cluster_counters WHERE name LIKE ? AND name <> ?", clockMarkerPrefix+"%", marker).Error
		}
		return nil
	})
	if err != nil {
		logger.Warnf("Edge pushes: could not record the new database server; retrying: %v", err)
		return false
	}
	if step != 0 {
		logger.Warnf("Edge pushes: the database failed over to %s, whose clock differs by %s from the previous server's; open pushes keep their times and may time out that much early or late", server, step.Round(time.Millisecond))
	}
	return done()
}
