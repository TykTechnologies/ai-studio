package cluster

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/dbhold"
	"github.com/TykTechnologies/midsommar/v2/pkg/dbstate"
	"github.com/TykTechnologies/midsommar/v2/pkg/pglisten"
	"github.com/TykTechnologies/midsommar/v2/pkg/safe"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Event is one entry of the cluster event log.
type Event struct {
	ID        int64
	Topic     string
	Origin    string // node ID of the publisher
	Payload   []byte
	CreatedAt time.Time
}

// Handler handles an event published by another replica. It runs on the
// log's reader goroutine, one event at a time in id order, so it must be
// quick (hand long work to a goroutine) and idempotent: delivery is at least
// once.
type Handler func(Event)

// LogOptions tune the event log. The zero value gives the defaults.
type LogOptions struct {
	// PollInterval is how often the log is read without a notification
	// (default 5 s). Notifications only make delivery faster: a lost one
	// (listener reconnecting, NOTIFY dropped) delays an event by at most
	// this long. Every notification wakes a read, and so does every
	// listener reconnect, so the poll only covers a listener whose
	// connection died unnoticed. Each poll is two queries on every
	// replica, idle or not.
	PollInterval time.Duration
	// Window is how far back every read looks again for rows it has not
	// seen (default 30 s). An id is allocated before its row commits, so a
	// row can become visible after a higher id was already read; the window
	// is how long such a straggler is still picked up.
	Window time.Duration
	// Retention is how long rows are kept (default 15 min); PruneInterval
	// how often they are pruned (default 1 min). Retention is at least
	// twice Window.
	Retention     time.Duration
	PruneInterval time.Duration
	// BatchSize bounds one read (default 500); a full batch is followed by
	// another read at once.
	BatchSize int
	// ListenerDSN is the connection string for LISTEN; empty means the one
	// the database was opened with. Tests give each replica its own.
	ListenerDSN string
}

// DefaultLogRetention is how long the event log keeps rows unless
// LogOptions.Retention says otherwise: the longest a reader can fall behind
// and still read every event.
const DefaultLogRetention = 15 * time.Minute

func (o LogOptions) withDefaults() LogOptions {
	if o.PollInterval <= 0 {
		o.PollInterval = 5 * time.Second
	}
	if o.Window <= 0 {
		o.Window = 30 * time.Second
	}
	if o.Retention < 2*o.Window {
		o.Retention = DefaultLogRetention
		if o.Retention < 2*o.Window {
			o.Retention = 2 * o.Window
		}
	}
	if o.PruneInterval <= 0 {
		o.PruneInterval = time.Minute
	}
	if o.BatchSize <= 0 {
		o.BatchSize = 500
	}
	return o
}

// LogStats describe a log's progress, for status pages and tests.
type LogStats struct {
	Enabled bool
	// Listening reports whether notifications wake the reader; without
	// them (the listener cannot connect) it reads every PollInterval.
	Listening          bool
	Cursor             int64     // highest id handled
	Delivered          uint64    // events handed to handlers
	LastRead           time.Time // last successful read
	ListenerReconnects uint64
	ReadErrors         uint64
	LastError          string
	// CursorResets counts the times the log found the table behind its
	// cursor (the newest events were lost when the database failed over to
	// a server that had not received them) and moved the cursor back;
	// LastCursorReset is when it last did.
	CursorResets    uint64
	LastCursorReset time.Time
	// Gaps counts the times this replica could not read the log for longer
	// than its retention, so events were pruned before it saw them, and
	// told its gap handlers (OnGap) to rebuild from the database; LastGap
	// is when it last did.
	Gaps    uint64
	LastGap time.Time
}

// seenEvent is a row the reader has handled: when (local clock), and enough
// of the row to tell it from another row that later gets the same id. After
// a failover that lost the newest rows, the new primary hands their ids out
// again.
type seenEvent struct {
	at      time.Time
	created int64 // created_at, Unix microseconds
	origin  string
}

func (e seenEvent) is(created time.Time, origin string) bool {
	return e.created == created.UnixMicro() && e.origin == origin
}

// Log is the cluster event log: events every other replica must see, kept
// in a table (cluster_events) that every replica reads. Delivery to each
// live replica is at least once and in id order; a replica never receives
// its own events. A replica that starts later does not receive what was
// published before it started: replicas build their state from the database
// at start, so they need no history.
type Log struct {
	db      *gorm.DB
	node    string
	opts    LogOptions
	enabled bool
	channel string

	mu       sync.Mutex
	handlers map[string][]*handlerEntry
	nextID   int

	// reader state, touched only by the reader goroutine (and Start)
	cursor int64
	seen   map[int64]seenEvent // id -> the row handled under it
	// lastGoodRead is when the last read succeeded (wall clock, so that
	// time a suspended machine spent asleep counts).
	lastGoodRead time.Time
	// failovers is the database tracker's failover count at the last read.
	failovers int64
	// checkHead asks the next read to compare the table with the cursor:
	// set when the listener reconnected, which is what a failover looks
	// like from here (so is a read that works again after failing, which
	// read checks for itself).
	checkHead atomic.Bool
	// now is time.Now, replaced in tests.
	now func() time.Time

	gapMu       sync.Mutex
	gapHandlers []func(time.Duration)

	statsMu sync.Mutex
	stats   LogStats

	follower     *pglisten.Follower
	wake         chan struct{}
	stop         chan struct{}
	done         chan struct{}
	stopOnce     sync.Once
	startOnce    sync.Once
	startErr     error
	backoffUntil time.Time
}

type handlerEntry struct {
	id int
	fn Handler
}

// NewLog returns the event log for node. It does nothing until Start.
func NewLog(db *gorm.DB, node string, opts LogOptions) *Log {
	db = dbhold.Unheld(db) // a background loop: fail fast, retry on the next tick
	return &Log{
		db:       db,
		node:     node,
		opts:     opts.withDefaults(),
		enabled:  db.Dialector.Name() == "postgres",
		handlers: map[string][]*handlerEntry{},
		seen:     map[int64]seenEvent{},
		now:      time.Now,
		wake:     make(chan struct{}, 1),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Subscribe registers h for events on topic ("*" for every topic). It may be
// called before or after Start. The returned function removes it.
func (l *Log) Subscribe(topic string, h Handler) (unsubscribe func()) {
	l.mu.Lock()
	id := l.nextID
	l.nextID++
	l.handlers[topic] = append(l.handlers[topic], &handlerEntry{id: id, fn: h})
	l.mu.Unlock()
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		hs := l.handlers[topic]
		for i, e := range hs {
			if e.id == id {
				l.handlers[topic] = append(hs[:i:i], hs[i+1:]...)
				break
			}
		}
	}
}

// Start begins reading. On Postgres it positions the log after the events
// already published (so none is replayed), listens for notifications, and
// reads on every notification, every PollInterval and after every listener
// reconnect. The notifications only make delivery faster: when the listener
// cannot connect, the log reads every PollInterval and keeps trying to
// listen in the background. On other databases it does nothing.
func (l *Log) Start(ctx context.Context) error {
	l.startOnce.Do(func() { l.startErr = l.start(ctx) })
	return l.startErr
}

// errStoppedBeforeStart is what Start returns after Stop.
var errStoppedBeforeStart = errors.New("cluster event log: stopped before it started")

func (l *Log) start(ctx context.Context) (err error) {
	// Stop waits for the reader; without one it must not wait at all.
	defer func() {
		if err != nil || !l.enabled {
			close(l.done)
		}
	}()
	l.setStats(func(s *LogStats) { s.Enabled = l.enabled })
	if !l.enabled {
		return nil
	}

	var schema string
	if err := l.db.WithContext(ctx).Raw("SELECT current_schema()").Scan(&schema).Error; err != nil {
		return fmt.Errorf("cluster event log: %w", err)
	}
	l.channel = "studio_cluster_events_" + schema

	// Position after everything already published, and mark the rows the
	// first window read would return as seen, so nothing from before this
	// start is delivered.
	var head struct{ Max int64 }
	if err := l.db.WithContext(ctx).Model(&models.ClusterEvent{}).Select("COALESCE(MAX(id), 0) AS max").Scan(&head).Error; err != nil {
		return fmt.Errorf("cluster event log: %w", err)
	}
	l.cursor = head.Max
	var recent []eventKey
	if err := l.db.WithContext(ctx).Model(&models.ClusterEvent{}).Select("id, created_at, origin").
		Where("created_at > ?", sinceExpr(l.db, l.opts.Window)).Find(&recent).Error; err != nil {
		return fmt.Errorf("cluster event log: %w", err)
	}
	now := l.now()
	for _, k := range recent {
		l.seen[k.ID] = seenEvent{at: now, created: k.CreatedAt.UnixMicro(), origin: k.Origin}
	}
	l.lastGoodRead = now.Round(0)
	l.failovers = dbstate.For(l.db).Status().Failovers
	l.setStats(func(s *LogStats) { s.Cursor = l.cursor })

	dsn := l.opts.ListenerDSN
	if dsn == "" {
		dsn, err = pglisten.DSN(l.db)
	}
	if err != nil {
		logger.Warnf("Cluster event log: %v; reading the log every %s without notifications", err, l.opts.PollInterval)
	} else {
		follower := pglisten.Follow(dsn, l.channel, pglisten.Options{Name: "Cluster event listener", ConnectTimeout: 30 * time.Second},
			func(string) { l.poke() },
			func() {
				l.setStats(func(s *LogStats) { s.ListenerReconnects++ })
				logger.Info("Cluster event listener reconnected; reading the event log to catch up")
				l.checkHead.Store(true)
				l.poke()
			})
		l.statsMu.Lock()
		l.follower = follower
		l.statsMu.Unlock()
	}

	go l.run()
	return nil
}

// Publish appends an event for every other replica and wakes them. It
// returns once the event is durable. On a database without the log it does
// nothing.
func (l *Log) Publish(ctx context.Context, topic string, payload []byte) error {
	if !l.enabled {
		return nil
	}
	channel := l.channel
	if channel == "" {
		var schema string
		if err := l.db.WithContext(ctx).Raw("SELECT current_schema()").Scan(&schema).Error; err != nil {
			return fmt.Errorf("cluster: publish %s: %w", topic, err)
		}
		channel = "studio_cluster_events_" + schema
	}
	if payload == nil {
		payload = []byte{}
	}
	// One statement: the row and its notification commit together.
	var id int64
	err := l.db.WithContext(ctx).Raw(
		`WITH ins AS (INSERT INTO cluster_events (topic, origin, payload, created_at) VALUES (?, ?, ?, now()) RETURNING id)
		 SELECT id FROM ins, LATERAL (SELECT pg_notify(?, ins.id::text)) n`,
		topic, l.node, payload, channel).Scan(&id).Error
	if err != nil {
		return fmt.Errorf("cluster: publish %s: %w", topic, err)
	}
	if id == 0 {
		return fmt.Errorf("cluster: publish %s: no id returned", topic)
	}
	return nil
}

// PublishBatch appends one event per payload, all on topic, in one
// statement with one notification: for a burst of events (an edge's batch of
// plugin payloads) that would otherwise cost a round trip each. Either every
// event is appended or none is.
func (l *Log) PublishBatch(ctx context.Context, topic string, payloads [][]byte) error {
	if !l.enabled || len(payloads) == 0 {
		return nil
	}
	channel := l.channel
	if channel == "" {
		var schema string
		if err := l.db.WithContext(ctx).Raw("SELECT current_schema()").Scan(&schema).Error; err != nil {
			return fmt.Errorf("cluster: publish %s: %w", topic, err)
		}
		channel = "studio_cluster_events_" + schema
	}
	// One parameter per payload, all in one statement. The space in "( ?"
	// matters: gorm expands a slice placed right after "(" into a list of
	// its elements, and a []byte is a slice.
	args := make([]interface{}, 0, len(payloads)+3)
	args = append(args, topic, l.node)
	values := make([]string, len(payloads))
	for i, p := range payloads {
		if p == nil {
			p = []byte{}
		}
		values[i] = "( ?::bytea)"
		args = append(args, p)
	}
	args = append(args, channel)
	// The rows and the notification commit together.
	var n int64
	err := l.db.WithContext(ctx).Raw(
		`WITH ins AS (INSERT INTO cluster_events (topic, origin, payload, created_at)
		   SELECT ?, ?, v.p, now() FROM (VALUES `+strings.Join(values, ", ")+`) AS v(p) RETURNING id)
		 SELECT c.n FROM (SELECT count(*) AS n, max(id) AS last FROM ins) c,
		   LATERAL (SELECT pg_notify(?, c.last::text)) notified`,
		args...).Scan(&n).Error
	if err != nil {
		return fmt.Errorf("cluster: publish %s: %w", topic, err)
	}
	if n != int64(len(payloads)) {
		return fmt.Errorf("cluster: publish %s: %d of %d events appended", topic, n, len(payloads))
	}
	return nil
}

// OnGap registers fn to be called (on the log's reader goroutine, so it must
// be quick: hand long work to a goroutine) when this replica could not read
// the log for longer than its retention. Events published meanwhile were
// pruned before it saw them, so what it holds in memory may be behind the
// database in ways no event will repair: fn must rebuild from the database,
// as a restart would. away is how long it could not read. It may be called
// before or after Start.
func (l *Log) OnGap(fn func(away time.Duration)) {
	l.gapMu.Lock()
	l.gapHandlers = append(l.gapHandlers, fn)
	l.gapMu.Unlock()
}

// Enabled reports whether this database carries the log (Postgres). On
// other databases Publish does nothing and no event is delivered.
func (l *Log) Enabled() bool { return l.enabled }

// Stats returns the log's progress.
func (l *Log) Stats() LogStats {
	l.statsMu.Lock()
	defer l.statsMu.Unlock()
	s := l.stats
	s.Listening = l.follower != nil && l.follower.Listening()
	return s
}

// Stop ends reading. No handler runs after it returns. It returns promptly
// whether or not Start ran or succeeded; a Start after Stop does nothing.
func (l *Log) Stop() {
	l.stopOnce.Do(func() {
		l.startOnce.Do(func() {
			l.startErr = errStoppedBeforeStart
			close(l.done)
		})
		close(l.stop)
		<-l.done
		l.statsMu.Lock()
		follower := l.follower
		l.statsMu.Unlock()
		if follower != nil {
			follower.Close()
		}
	})
}

func (l *Log) poke() {
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

func (l *Log) run() {
	defer close(l.done)
	safe.Loop("cluster event log", l.stop, l.loop)
}

func (l *Log) loop() {
	poll := time.NewTicker(l.opts.PollInterval)
	defer poll.Stop()
	prune := time.NewTicker(l.opts.PruneInterval)
	defer prune.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-prune.C:
			l.prune()
			continue
		case <-l.wake:
		case <-poll.C:
		}
		l.read()
	}
}

// read handles every row not yet handled: stragglers (ids at or below the
// cursor that committed late, still inside the window) first, then the rows
// after the cursor, in batches until none are left.
func (l *Log) read() {
	if time.Now().Before(l.backoffUntil) {
		return
	}
	// How long this replica has not read the log. Usually a poll interval;
	// after an outage, the outage.
	now := l.now().Round(0)
	away := now.Sub(l.lastGoodRead)
	if away < 0 {
		away = 0
	}
	window := l.opts.Window
	// After a failover the table may not be the one the cursor was counted
	// on: look at it before reading on (see afterFailover).
	failovers := dbstate.For(l.db).Status().Failovers
	if l.checkHead.Swap(false) || failovers != l.failovers || l.failing() {
		wider, ok := l.afterFailover(away)
		if !ok {
			// Look again on the next read.
			l.checkHead.Store(true)
			return
		}
		window = wider
	}
	l.failovers = failovers
	if !l.readStragglers(window) {
		return
	}
	for {
		select {
		case <-l.stop:
			return
		default:
		}
		var rows []models.ClusterEvent
		err := l.db.Model(&models.ClusterEvent{}).
			Where("id > ? AND origin <> ?", l.cursor, l.node).
			Order("id").Limit(l.opts.BatchSize).Find(&rows).Error
		if err != nil {
			l.readFailed(err)
			return
		}
		l.handle(rows)
		l.readSucceeded()
		if len(rows) < l.opts.BatchSize {
			break
		}
	}
	l.forgetOld()
	l.setStats(func(s *LogStats) { s.Cursor = l.cursor; s.LastRead = time.Now() })
	// Measured to the end of this read: one that itself took longer than
	// the retention (a database that hung, then answered) is a gap too.
	done := l.now().Round(0)
	since := done.Sub(l.lastGoodRead)
	l.lastGoodRead = done
	if since > l.opts.Retention {
		l.gap(since)
	}
}

// afterFailover runs when the database may have failed over since the last
// read: the listener reconnected, reads work again after failing, or the
// database tracker counted a failover. With asynchronous replication the new
// primary may not have received the newest events, and it then hands their
// ids out again: to new events, which a reader that kept its cursor would
// never see (ids at or below the cursor are only looked at within the
// window, and not at all when the reader has already handled a row with
// that id).
//
// If the newest id in the table is below the cursor, the cursor moves back
// to it. In every case the next straggler read looks back over the whole
// time this replica was away plus the window, twice (the new primary's clock
// may differ from the old one's, by up to a window), instead of the window
// alone; rows are told apart by what they are, not by their id (seenEvent),
// so an event that took a lost one's id is delivered, and nothing already
// handled is delivered again as long as the two servers' clocks are within
// a window of each other (forgetOld). It returns how far back to look.
func (l *Log) afterFailover(away time.Duration) (window time.Duration, ok bool) {
	var head struct{ Max int64 }
	if err := l.db.Model(&models.ClusterEvent{}).Select("COALESCE(MAX(id), 0) AS max").Scan(&head).Error; err != nil {
		l.readFailed(err)
		return 0, false
	}
	if head.Max < l.cursor {
		logger.Warnf("Cluster event log: the newest event in the database (%d) is older than the last one this replica read (%d): the database failed over to a server that had not received the newest events. Reading on from %d; events lost with them were lost for every replica.",
			head.Max, l.cursor, head.Max)
		l.cursor = head.Max
		// Rows above the new cursor are gone: what is remembered about
		// their ids describes rows that no longer exist.
		for id := range l.seen {
			if id > head.Max {
				delete(l.seen, id)
			}
		}
		l.setStats(func(s *LogStats) {
			s.CursorResets++
			s.LastCursorReset = time.Now()
			s.Cursor = l.cursor
		})
	}
	window = away + 2*l.opts.Window
	if window > l.opts.Retention {
		window = l.opts.Retention
	}
	return window, true
}

// gap tells the gap handlers that this replica was away from the log for
// longer than its retention.
func (l *Log) gap(away time.Duration) {
	logger.Warnf("Cluster event log: this replica could not read the event log for %s, longer than events are kept (%s): changes other replicas made meanwhile are reloaded from the database",
		away.Round(time.Second), l.opts.Retention)
	l.setStats(func(s *LogStats) {
		s.Gaps++
		s.LastGap = time.Now()
	})
	l.gapMu.Lock()
	handlers := append([]func(time.Duration){}, l.gapHandlers...)
	l.gapMu.Unlock()
	for _, fn := range handlers {
		safe.Call("cluster event log gap handler", func() { fn(away) })
	}
}

// eventKey is what tells one row of the log from another.
type eventKey struct {
	ID        int64
	CreatedAt time.Time
	Origin    string
}

// readStragglers handles rows at or below the cursor that were not visible
// when the cursor passed them, looking back window. It reads what
// identifies each row only, then the rows it has not handled, so a busy
// window costs little.
func (l *Log) readStragglers(window time.Duration) bool {
	var keys []eventKey
	err := l.db.Model(&models.ClusterEvent{}).Select("id, created_at, origin").
		Where("id <= ? AND created_at > ? AND origin <> ?", l.cursor, sinceExpr(l.db, window), l.node).
		Order("id").Find(&keys).Error
	if err != nil {
		l.readFailed(err)
		return false
	}
	var missing []int64
	for _, k := range keys {
		if seen, ok := l.seen[k.ID]; !ok || !seen.is(k.CreatedAt, k.Origin) {
			missing = append(missing, k.ID)
		}
	}
	if len(missing) == 0 {
		return true
	}
	var rows []models.ClusterEvent
	if err := l.db.Where("id IN ?", missing).Order("id").Find(&rows).Error; err != nil {
		l.readFailed(err)
		return false
	}
	logger.Debugf("Cluster event log: %d event(s) committed after later ones were read; handling them now", len(rows))
	l.handle(rows)
	return true
}

func (l *Log) handle(rows []models.ClusterEvent) {
	for _, row := range rows {
		if seen, ok := l.seen[row.ID]; ok && seen.is(row.CreatedAt, row.Origin) {
			continue
		}
		l.seen[row.ID] = seenEvent{at: time.Now(), created: row.CreatedAt.UnixMicro(), origin: row.Origin}
		if row.ID > l.cursor {
			l.cursor = row.ID
		}
		l.deliver(Event{ID: row.ID, Topic: row.Topic, Origin: row.Origin, Payload: row.Payload, CreatedAt: row.CreatedAt})
	}
}

func (l *Log) deliver(ev Event) {
	l.mu.Lock()
	hs := append(append([]*handlerEntry(nil), l.handlers[ev.Topic]...), l.handlers["*"]...)
	l.mu.Unlock()
	for _, h := range hs {
		l.call(h.fn, ev)
	}
	l.setStats(func(s *LogStats) { s.Delivered++ })
}

func (l *Log) call(h Handler, ev Event) {
	start := time.Now()
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("Cluster event handler for %s (event %d from %s) panicked: %v\n%s", ev.Topic, ev.ID, ev.Origin, r, debug.Stack())
		}
		if d := time.Since(start); d > time.Second {
			logger.Warnf("Cluster event handler for %s took %s; handlers run one at a time and delay every later event", ev.Topic, d.Round(time.Millisecond))
		}
	}()
	h(ev)
}

// forgetOld drops what is remembered about rows handled long enough ago
// that no read can return them again: three windows.
//
// An ordinary read looks back one window by the database's clock. A row is
// handled after it was created, and time passes equally on both clocks, so
// a row handled more than a window ago by this replica's clock was created
// more than a window ago by the database's: local time is fine here, and no
// agreement between the two clocks is assumed.
//
// The read after a failover (afterFailover) looks back over the time since
// the last successful read plus two windows, on the new primary's clock.
// What was forgotten at that last read was handled more than three windows
// before it, so it is outside that look-back unless the new primary's clock
// is more than a window behind the old one's. Up to that difference nothing
// handled is delivered again; beyond it a handler can see again an event it
// handled that much earlier, which delivery at least once allows.
func (l *Log) forgetOld() {
	cutoff := time.Now().Add(-3 * l.opts.Window)
	for id, e := range l.seen {
		if e.at.Before(cutoff) && id <= l.cursor {
			delete(l.seen, id)
		}
	}
}

func (l *Log) prune() {
	res := l.db.Where("created_at < ?", sinceExpr(l.db, l.opts.Retention)).Delete(&models.ClusterEvent{})
	if res.Error != nil && !errors.Is(res.Error, gorm.ErrRecordNotFound) {
		logger.Warnf("Cluster event log: pruning old events failed: %v", res.Error)
	}
}

func (l *Log) readFailed(err error) {
	first := false
	l.setStats(func(s *LogStats) {
		first = s.LastError == ""
		s.ReadErrors++
		s.LastError = err.Error()
	})
	if first {
		logger.Warnf("Cluster event log: reading events failed; retrying (events from other replicas are delayed until it recovers): %v", err)
	}
	// Do not hammer a database that is down: skip reads for a moment
	// (notifications included, so not for longer than a couple of seconds).
	l.backoffUntil = time.Now().Add(min(l.opts.PollInterval*2, 2*time.Second))
}

func (l *Log) readSucceeded() {
	recovered := false
	l.setStats(func(s *LogStats) {
		recovered = s.LastError != ""
		s.LastError = ""
	})
	if recovered {
		logger.Info("Cluster event log: reading events works again")
	}
}

// failing reports whether the last read failed.
func (l *Log) failing() bool {
	l.statsMu.Lock()
	defer l.statsMu.Unlock()
	return l.stats.LastError != ""
}

func (l *Log) setStats(f func(*LogStats)) {
	l.statsMu.Lock()
	f(&l.stats)
	l.statsMu.Unlock()
}
