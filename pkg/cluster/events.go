package cluster

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/pglisten"
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
	// (default 1 s). Notifications only make delivery faster: a lost one
	// (listener reconnecting, NOTIFY dropped) delays an event by at most
	// this long.
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

func (o LogOptions) withDefaults() LogOptions {
	if o.PollInterval <= 0 {
		o.PollInterval = time.Second
	}
	if o.Window <= 0 {
		o.Window = 30 * time.Second
	}
	if o.Retention < 2*o.Window {
		o.Retention = 15 * time.Minute
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
	seen   map[int64]time.Time // id -> when it was handled (local clock)

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
	return &Log{
		db:       db,
		node:     node,
		opts:     opts.withDefaults(),
		enabled:  db.Dialector.Name() == "postgres",
		handlers: map[string][]*handlerEntry{},
		seen:     map[int64]time.Time{},
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
	var recent []int64
	if err := l.db.WithContext(ctx).Model(&models.ClusterEvent{}).Where("created_at > ?", sinceExpr(l.db, l.opts.Window)).Pluck("id", &recent).Error; err != nil {
		return fmt.Errorf("cluster event log: %w", err)
	}
	now := time.Now()
	for _, id := range recent {
		l.seen[id] = now
	}
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
	if !l.readStragglers() {
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
}

// readStragglers handles rows at or below the cursor that were not visible
// when the cursor passed them. It reads ids only, then the rows it has not
// seen, so a busy window costs little.
func (l *Log) readStragglers() bool {
	var ids []int64
	err := l.db.Model(&models.ClusterEvent{}).
		Where("id <= ? AND created_at > ? AND origin <> ?", l.cursor, sinceExpr(l.db, l.opts.Window), l.node).
		Order("id").Pluck("id", &ids).Error
	if err != nil {
		l.readFailed(err)
		return false
	}
	var missing []int64
	for _, id := range ids {
		if _, ok := l.seen[id]; !ok {
			missing = append(missing, id)
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
		if _, ok := l.seen[row.ID]; ok {
			continue
		}
		l.seen[row.ID] = time.Now()
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

// forgetOld drops ids handled long enough ago that the window can no longer
// return them. Local time is fine here: a row is handled after it commits,
// so once twice the window has passed locally it is outside the window by
// the database's clock too.
func (l *Log) forgetOld() {
	cutoff := time.Now().Add(-2 * l.opts.Window)
	for id, at := range l.seen {
		if at.Before(cutoff) && id <= l.cursor {
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
	// Do not hammer a database that is down: skip reads for a moment.
	l.backoffUntil = time.Now().Add(l.opts.PollInterval * 2)
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

func (l *Log) setStats(f func(*LogStats)) {
	l.statsMu.Lock()
	f(&l.stats)
	l.statsMu.Unlock()
}
