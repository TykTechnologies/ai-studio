// Package pushes delivers configuration pushes ("reloads") to edge gateways
// when several Studio replicas share the database. A push is durable: one
// row per target edge (models.EdgePushCommand), delivered by whichever
// replica holds the edge's stream, retried on transport failures, and ended
// in exactly one terminal status with a reason. See
// features/ClusterControlPlane.md for the guarantees; this file implements
// them.
//
// Every replica runs a Coordinator:
//
//   - its dispatcher claims pending commands for edges whose stream this
//     replica holds (a conditional update, so exactly one replica wins) and
//     sends the reload request on that stream;
//   - the control server hands it the edge's reload responses and tells it
//     when a stream opens or closes;
//   - its janitor, identical on every replica and idempotent, returns
//     commands to pending when their replica died, their claim lapsed or
//     the edge stopped answering, expires them at the deadline, and settles
//     operations whose commands are all done.
package pushes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/cluster"
	"github.com/TykTechnologies/midsommar/v2/pkg/pglisten"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Streams is what the coordinator needs from the control server: the edge
// streams this replica holds, and a way to send on one.
type Streams interface {
	// LocalStreams returns, for every edge with a live stream on this
	// replica, the stream's session ID.
	LocalStreams() map[string]string
	// SendReload sends req on the edge's stream, failing if that stream is
	// no longer the one identified by session. When nothing was sent
	// because there is no such stream (it closed, or the edge reconnected)
	// the error wraps ErrNoStream.
	SendReload(edgeID, session string, req *pb.ConfigurationReloadRequest) error
}

// ErrNoStream: the edge has no stream with the given session on this
// replica, so nothing was sent. That does not use one of the push's
// attempts; the push waits for the edge's current stream.
var ErrNoStream = errors.New("no such edge stream on this replica")

// Options tune the coordinator. The zero value gives the defaults.
type Options struct {
	// Deadline is how long a push may take, including waiting for an edge
	// that is not connected (default 5 min).
	Deadline time.Duration
	// AnswerTimeout is how long a sent command may go without word from the
	// edge (any phase resets it) before it is retried (default 60 s).
	AnswerTimeout time.Duration
	// ClaimTimeout is how long a replica may hold a claim without sending
	// (default 30 s).
	ClaimTimeout time.Duration
	// MaxAttempts bounds deliveries per command (default 3). Only transport
	// failures use an attempt: an edge that answers FAILED is not retried.
	MaxAttempts int
	// PollInterval is how often the dispatcher looks for work without a
	// notification (default 1 s); JanitorInterval how often the janitor
	// runs (default 2 s).
	PollInterval    time.Duration
	JanitorInterval time.Duration
	// ReachableHeartbeat is how recent an edge's heartbeat must be for it to
	// count as reachable (default 90 s: three missed 30 s heartbeats).
	ReachableHeartbeat time.Duration
	// RecentlyOffline is how long a disconnected edge stays a target of
	// namespace pushes (default 5 min): long enough to cover a rolling
	// restart, short enough that long-gone edges do not expire every push.
	RecentlyOffline time.Duration
	// Retention is how long finished pushes are kept (default 7 days).
	Retention time.Duration
	// ListenerDSN is the connection string for LISTEN; empty means the
	// database's own. Tests give each replica its own.
	ListenerDSN string
}

func (o Options) withDefaults() Options {
	def := func(d *time.Duration, v time.Duration) {
		if *d <= 0 {
			*d = v
		}
	}
	def(&o.Deadline, 5*time.Minute)
	def(&o.AnswerTimeout, 60*time.Second)
	def(&o.ClaimTimeout, 30*time.Second)
	def(&o.PollInterval, time.Second)
	def(&o.JanitorInterval, 2*time.Second)
	def(&o.ReachableHeartbeat, 90*time.Second)
	def(&o.RecentlyOffline, 5*time.Minute)
	def(&o.Retention, 7*24*time.Hour)
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = 3
	}
	return o
}

// Push scopes.
const (
	ScopeEdge      = "edge"
	ScopeNamespace = "namespace"
	ScopeAll       = "all"
)

// Errors a push can fail with before anything is written.
var (
	ErrEdgeNotFound = errors.New("edge not found")
	ErrNoTargets    = errors.New("no edges to push to")
)

// Request describes a push.
type Request struct {
	Scope       string
	Namespace   string   // ScopeNamespace
	EdgeIDs     []string // ScopeEdge
	InitiatedBy string
}

// Target is an edge a push was (or was not) sent to.
type Target struct {
	EdgeID    string `json:"edge_id"`
	Namespace string `json:"namespace"`
	// Reachable: its stream's replica is live and it sent a heartbeat
	// recently. An unreachable target is waited for until the deadline.
	Reachable bool   `json:"reachable"`
	Reason    string `json:"reason,omitempty"`
}

// Result is what Push reports at once, before any edge has answered.
type Result struct {
	Operation models.PushOperation `json:"operation"`
	Targets   []Target             `json:"targets"`
	// Skipped are edges of the namespace(s) that were left out because they
	// have been offline too long to wait for: the first maxSkippedListed of
	// SkippedTotal.
	Skipped      []Target `json:"skipped,omitempty"`
	SkippedTotal int      `json:"skipped_total,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
}

// Coordinator delivers pushes; one per replica.
type Coordinator struct {
	db      *gorm.DB
	node    string
	streams Streams
	opts    Options

	channel  string
	listener *pglisten.Listener
	sub      *pglisten.Subscription
	unhook   func()

	wake      chan struct{}
	stop      chan struct{}
	done      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	startErr  error

	lastPrune time.Time // janitor goroutine only
}

// New returns the coordinator for node. It does nothing until Start.
func New(db *gorm.DB, node string, streams Streams, opts Options) *Coordinator {
	return &Coordinator{
		db:      db,
		node:    node,
		streams: streams,
		opts:    opts.withDefaults(),
		wake:    make(chan struct{}, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// Start runs the dispatcher and the janitor until Stop. On Postgres it also
// listens for new pushes so the replica holding a target's stream delivers
// it at once rather than on its next poll.
func (c *Coordinator) Start(ctx context.Context) error {
	c.startOnce.Do(func() { c.startErr = c.start(ctx) })
	return c.startErr
}

func (c *Coordinator) start(ctx context.Context) error {
	if c.db.Dialector.Name() == "postgres" {
		var schema string
		if err := c.db.WithContext(ctx).Raw("SELECT current_schema()").Scan(&schema).Error; err != nil {
			return fmt.Errorf("pushes: %w", err)
		}
		c.channel = "studio_edge_push_" + schema
		dsn := c.opts.ListenerDSN
		if dsn == "" {
			var err error
			if dsn, err = pglisten.DSN(c.db); err != nil {
				return fmt.Errorf("pushes: %w", err)
			}
		}
		l, err := pglisten.Acquire(dsn, pglisten.Options{Name: "Edge push listener", ConnectTimeout: 30 * time.Second})
		if err != nil {
			return fmt.Errorf("pushes: %w", err)
		}
		sub, err := l.Subscribe(c.channel, func(string) { c.poke() }, 30*time.Second)
		if err != nil {
			l.Release()
			return fmt.Errorf("pushes: %w", err)
		}
		c.listener, c.sub = l, sub
		// Notifications lost while the listener reconnected are made up for
		// by the poll anyway; waking now just shortens the wait.
		c.unhook = l.OnReconnect(c.poke)
	}
	go c.run()
	return nil
}

// Stop ends the dispatcher and janitor. Commands this replica had claimed
// stay claimed; the janitor of another replica (or this one, after a
// restart) returns them to pending once this node's registration lapses.
func (c *Coordinator) Stop() {
	c.stopOnce.Do(func() {
		close(c.stop)
		<-c.done
		if c.listener != nil {
			if c.unhook != nil {
				c.unhook()
			}
			c.listener.Unsubscribe(c.channel, c.sub, 10*time.Second)
			c.listener.Release()
		}
	})
}

func (c *Coordinator) poke() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *Coordinator) run() {
	defer close(c.done)
	poll := time.NewTicker(c.opts.PollInterval)
	defer poll.Stop()
	janitor := time.NewTicker(c.opts.JanitorInterval)
	defer janitor.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-janitor.C:
			c.janitor()
		case <-c.wake:
			c.dispatch()
		case <-poll.C:
			c.dispatch()
		}
	}
}

// now is the database's clock on Postgres, so every replica compares
// timestamps against the same clock; SQLite serves one process.
func (c *Coordinator) now() time.Time {
	if c.db.Dialector.Name() == "postgres" {
		var t time.Time
		if err := c.db.Raw("SELECT now()").Scan(&t).Error; err == nil {
			return t.UTC()
		}
	}
	return time.Now().UTC()
}

// notify wakes every replica's dispatcher.
func (c *Coordinator) notify() {
	c.poke()
	if c.channel != "" {
		if err := c.db.Exec("SELECT pg_notify(?, '')", c.channel).Error; err != nil {
			logger.Warnf("Edge pushes: notifying other replicas failed; they pick the push up on their next poll: %v", err)
		}
	}
}

// Push records a push and its targets and returns at once; delivery
// happens in the background on whichever replicas hold the targets'
// streams.
func (c *Coordinator) Push(ctx context.Context, req Request) (*Result, error) {
	db := c.db.WithContext(ctx)
	now := c.now()

	var edges []models.EdgeInstance
	var skipped []models.EdgeInstance
	var skippedTotal int64
	switch req.Scope {
	case ScopeEdge:
		if len(req.EdgeIDs) == 0 {
			return nil, fmt.Errorf("%w: no edge given", ErrNoTargets)
		}
		if err := db.Where("edge_id IN ?", req.EdgeIDs).Find(&edges).Error; err != nil {
			return nil, err
		}
		if len(edges) != len(req.EdgeIDs) {
			found := map[string]bool{}
			for _, e := range edges {
				found[e.EdgeID] = true
			}
			var missing []string
			for _, id := range req.EdgeIDs {
				if !found[id] {
					missing = append(missing, id)
				}
			}
			return nil, fmt.Errorf("%w: %s", ErrEdgeNotFound, strings.Join(missing, ", "))
		}
	case ScopeNamespace, ScopeAll:
		// Targets: connected or registered edges, and edges that dropped
		// off within RecentlyOffline (mid-reconnect or restarting: worth
		// waiting for). The rest are left out; the database does the
		// filtering, and only the first few left-out edges are listed.
		scoped := func() *gorm.DB {
			q := db.Model(&models.EdgeInstance{})
			if req.Scope == ScopeNamespace {
				q = q.Where("namespace IN ?", models.NamespaceAliases(req.Namespace))
			}
			return q
		}
		cond, args := c.targetCondition(now.Add(-c.opts.RecentlyOffline))
		if err := scoped().Where(cond, args...).Order("edge_id").Find(&edges).Error; err != nil {
			return nil, err
		}
		if err := scoped().Where("NOT "+cond, args...).Count(&skippedTotal).Error; err != nil {
			return nil, err
		}
		if skippedTotal > 0 {
			if err := scoped().Where("NOT "+cond, args...).Select("edge_id", "namespace", "last_heartbeat").
				Order("edge_id").Limit(maxSkippedListed).Find(&skipped).Error; err != nil {
				return nil, err
			}
		}
		if len(edges) == 0 {
			if skippedTotal > 0 {
				return nil, fmt.Errorf("%w: the %d edge(s) %s have been offline for more than %s", ErrNoTargets, skippedTotal, scopeLabel(req), c.opts.RecentlyOffline)
			}
			return nil, fmt.Errorf("%w: no edges are registered %s", ErrNoTargets, scopeLabel(req))
		}
	default:
		return nil, fmt.Errorf("pushes: unknown scope %q", req.Scope)
	}

	op := models.PushOperation{
		OperationID: "push-" + uuid.New().String(),
		Scope:       req.Scope,
		Namespace:   req.Namespace,
		InitiatedBy: req.InitiatedBy,
		Status:      models.PushOperationInProgress,
		Total:       len(edges),
		CreatedAt:   now,
		DeadlineAt:  now.Add(c.opts.Deadline),
	}
	cmds := make([]models.EdgePushCommand, len(edges))
	for i, e := range edges {
		cmds[i] = models.EdgePushCommand{
			OperationID: op.OperationID,
			EdgeID:      e.EdgeID,
			Namespace:   e.Namespace,
			Status:      models.PushCommandPending,
			MaxAttempts: c.opts.MaxAttempts,
			DeadlineAt:  op.DeadlineAt,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&op).Error; err != nil {
			return err
		}
		return tx.Create(&cmds).Error
	}); err != nil {
		return nil, fmt.Errorf("pushes: record push: %w", err)
	}
	c.notify()

	res := &Result{Operation: op, SkippedTotal: int(skippedTotal)}
	live := c.liveNodes()
	unreachable := 0
	for _, e := range edges {
		t := c.target(e, live)
		if !t.Reachable {
			unreachable++
		}
		res.Targets = append(res.Targets, t)
	}
	for _, e := range skipped {
		res.Skipped = append(res.Skipped, Target{EdgeID: e.EdgeID, Namespace: e.Namespace, Reason: offlineReason(e)})
	}
	if unreachable > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%d of %d edge(s) are not connected to the control plane; the push waits up to %s for them to reconnect.",
			unreachable, len(edges), c.opts.Deadline.Round(time.Second)))
	}
	if skippedTotal > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("%d edge(s) were left out because they have been offline for more than %s.", skippedTotal, c.opts.RecentlyOffline))
	}
	logger.Infof("Edge push %s recorded by %s: %d target(s), %d not connected yet", op.OperationID, req.InitiatedBy, len(edges), unreachable)
	return res, nil
}

// maxSkippedListed caps how many left-out edges a push result lists (the
// warning gives the full count).
var maxSkippedListed = 100

// targetCondition is the SQL for "an edge a namespace push targets", given
// the cutoff for recently disconnected edges. Heartbeats are written by
// every replica in its local zone; on SQLite, which compares timestamps as
// text, both sides go through datetime() so zones compare correctly.
func (c *Coordinator) targetCondition(cutoff time.Time) (string, []interface{}) {
	recent := "last_heartbeat > ?"
	if c.db.Dialector.Name() == "sqlite" {
		recent = "COALESCE(datetime(last_heartbeat) > datetime(?), 0)"
	}
	return "(status IN ? OR (status = ? AND last_heartbeat IS NOT NULL AND " + recent + "))",
		[]interface{}{[]string{models.EdgeStatusConnected, models.EdgeStatusRegistered}, models.EdgeStatusDisconnected, cutoff.UTC()}
}

// liveNodes returns the live replicas, or nil if they cannot be listed.
func (c *Coordinator) liveNodes() map[string]bool {
	nodes, err := cluster.LiveNodes(c.db)
	if err != nil {
		logger.Warnf("Edge pushes: listing live replicas failed: %v", err)
		return nil
	}
	live := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		live[n.NodeID] = true
	}
	return live
}

// target reports whether e can be reached now: its stream's replica is
// live (live is from liveNodes; nil means unknown) and its last heartbeat
// is recent.
func (c *Coordinator) target(e models.EdgeInstance, live map[string]bool) Target {
	t := Target{EdgeID: e.EdgeID, Namespace: e.Namespace}
	if e.OwnerNodeID == "" || e.Status != models.EdgeStatusConnected {
		t.Reason = "not connected to any control-plane replica"
		return t
	}
	switch {
	case live == nil:
		t.Reason = "could not check its control-plane replica"
	case !live[e.OwnerNodeID]:
		t.Reason = "its control-plane replica has stopped; waiting for it to reconnect"
	case e.LastHeartbeat == nil || time.Since(*e.LastHeartbeat) > c.opts.ReachableHeartbeat:
		t.Reason = "no recent heartbeat"
	default:
		t.Reachable = true
	}
	return t
}

func offlineReason(e models.EdgeInstance) string {
	if e.LastHeartbeat == nil {
		return "never connected"
	}
	return "offline since " + e.LastHeartbeat.UTC().Format(time.RFC3339)
}

func scopeLabel(req Request) string {
	if req.Scope == ScopeNamespace {
		return fmt.Sprintf("in namespace %q", req.Namespace)
	}
	return "in any namespace"
}
