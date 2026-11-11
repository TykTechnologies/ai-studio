package pushes

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
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
//   - if the database server changed too (a failover, not a correction of
//     one server's clock) and the step is forward, the times of the pushes
//     still open are moved forward by the step, so that none times out
//     sooner than it would have (moveOpenPushes). A step backwards moves
//     nothing: late is safe. That is done once per failover for the whole
//     cluster, by a replica that measured the step, under the marker row of
//     the new server (clockMarker).

// clockStepMin is the smallest change of the database's clock against this
// replica's that counts as a step: below it, a difference is the round trip
// of the reading or drift.
const clockStepMin = time.Second

// clockStepMax is the largest step open pushes are moved by. A larger one
// is not a difference between two servers' clocks, or not one to trust: the
// pushes are left as they are (a push that times out early is retried or
// reported; one moved wrongly is neither).
const clockStepMax = 5 * time.Minute

// Values of a clock marker row.
const (
	// clockMarkerSeen: the pushes' times are known to be on this server's
	// clock, or nobody who measured the step to it has looked yet.
	clockMarkerSeen = 1
	// clockMarkerDecided: a replica that measured the step to this server
	// has decided about the open pushes (moved them, or left them).
	clockMarkerDecided = 2
)

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
// server's clock: the server's name when it fits the column (64
// characters), a digest of it otherwise (an IPv6 address with port and
// timeline does not fit). Never a cut name: two servers must not share a
// marker.
func clockMarker(server string) string {
	name := clockMarkerPrefix + server
	if len(name) > 64 {
		sum := sha256.Sum256([]byte(server))
		name = clockMarkerPrefix + "sha256:" + hex.EncodeToString(sum[:20])
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
// If the server did not change, nothing is moved. If it did, the new
// server's marker is recorded (by whichever replica looks first, measured
// or not), and a replica that measured the step decides about the open
// pushes, once: the one whose update of the marker from seen to decided
// succeeds. A replica without a measurement (it started during or after
// the failover, or its readings around it were too slow to use) records
// the marker and decides nothing, so a replica that did measure still can.
// If no replica measured, nothing is moved and open pushes may time out
// early by the difference between the two servers' clocks.
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

	// measured: this replica read the old server's clock and the new one's.
	measured := known != "" && step != 0
	moved, decided := false, false
	err = c.db.Transaction(func(tx *gorm.DB) error {
		marker := clockMarker(server)
		res := tx.Exec("INSERT INTO cluster_counters (name, value) VALUES (?, ?) ON CONFLICT (name) DO NOTHING", marker, clockMarkerSeen)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 1 {
			// The markers of earlier servers have done their work.
			if err := tx.Exec("DELETE FROM cluster_counters WHERE name LIKE ? AND name <> ?", clockMarkerPrefix+"%", marker).Error; err != nil {
				return err
			}
		}
		if !measured {
			return nil
		}
		res = tx.Exec("UPDATE cluster_counters SET value = ? WHERE name = ? AND value = ?", clockMarkerDecided, marker, clockMarkerSeen)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return nil // another replica decided
		}
		decided = true
		if step < clockStepMin || step > clockStepMax {
			return nil
		}
		moved = true
		return moveOpenPushes(tx, step)
	})
	if err != nil {
		logger.Warnf("Edge pushes: could not move open pushes to the new database server's clock; retrying: %v", err)
		return false
	}
	switch {
	case moved:
		logger.Warnf("Edge pushes: the database failed over to %s, whose clock is %s ahead of the previous server's; the times of open pushes were moved forward by that much, so none times out early", server, step.Round(time.Millisecond))
	case decided && step > clockStepMax:
		logger.Errorf("Edge pushes: the database failed over to %s and its clock is %s ahead of the previous server's, more than the %s open pushes are moved by; they keep their times and may time out early. Check the database servers' clocks.", server, step.Round(time.Second), clockStepMax)
	case decided && step < 0:
		logger.Warnf("Edge pushes: the database failed over to %s, whose clock is %s behind the previous server's; open pushes keep their times and may take that much longer to time out", server, (-step).Round(time.Millisecond))
	case !measured:
		logger.Infof("Edge pushes: the database server is now %s; this replica did not measure a difference between the servers' clocks", server)
	}
	return done()
}

// moveOpenPushes moves forward by step every time the janitor and the
// dispatcher compare with the database's clock, on the pushes that are
// still open:
//
//   - edge_push_commands in pending, claimed or sent: deadline_at (the
//     deadline), claim_expires_at (a claim that was never sent), sent_at
//     and phase_at (the answer timeout);
//   - push_operations in progress: deadline_at.
//
// Finished commands and operations, and the times that are only records
// (created_at, updated_at, completed_at, the history), are left alone.
func moveOpenPushes(tx *gorm.DB, step time.Duration) error {
	by := gorm.Expr("make_interval(secs => ?)", step.Seconds())
	if err := tx.Exec(`UPDATE edge_push_commands SET deadline_at = deadline_at + ?, claim_expires_at = claim_expires_at + ?,
		sent_at = sent_at + ?, phase_at = phase_at + ? WHERE status IN ?`, by, by, by, by, pushCommandOpen).Error; err != nil {
		return err
	}
	return tx.Exec(`UPDATE push_operations SET deadline_at = deadline_at + ? WHERE status = ?`, by, models.PushOperationInProgress).Error
}
