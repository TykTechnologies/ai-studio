package pushes

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/cluster"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

// forEachDB runs body on SQLite, and on Postgres (a fresh schema) when
// DATABASE_URL is set, so every rule is checked against both dialects' SQL.
func forEachDB(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "p.db")+"?_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		migrate(t, db)
		body(t, db)
	})
	base := os.Getenv("DATABASE_URL")
	if base == "" {
		return
	}
	t.Run("postgres", func(t *testing.T) {
		admin, err := gorm.Open(postgres.Open(base), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		schema := fmt.Sprintf("pushes_test_%d", time.Now().UnixNano())
		require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
		t.Cleanup(func() {
			admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`)
			if s, err := admin.DB(); err == nil {
				s.Close()
			}
		})
		u, err := url.Parse(base)
		require.NoError(t, err)
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		db, err := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		t.Cleanup(func() {
			if s, err := db.DB(); err == nil {
				s.Close()
			}
		})
		migrate(t, db)
		body(t, db)
	})
}

func migrate(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&models.EdgeInstance{}, &models.NamespaceSyncStatus{}, &models.ClusterNode{},
		&models.PushOperation{}, &models.EdgePushCommand{}))
}

// fakeStreams stands in for one replica's control server.
type fakeStreams struct {
	mu      sync.Mutex
	streams map[string]string // edge -> session
	sent    []sentReload
	failing error
	// onSend runs after a successful send, outside the lock: what happens
	// on the wire while the sender is about to record the send.
	onSend func(edgeID, session string)
}

type sentReload struct {
	edge, session string
	req           *pb.ConfigurationReloadRequest
}

func newFakeStreams() *fakeStreams { return &fakeStreams{streams: map[string]string{}} }

func (f *fakeStreams) LocalStreams() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]string{}
	for k, v := range f.streams {
		out[k] = v
	}
	return out
}

func (f *fakeStreams) SendReload(edgeID, session string, req *pb.ConfigurationReloadRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failing != nil {
		return f.failing
	}
	if f.streams[edgeID] != session {
		return fmt.Errorf("%w: stream changed", ErrNoStream)
	}
	f.sent = append(f.sent, sentReload{edgeID, session, req})
	if hook := f.onSend; hook != nil {
		f.mu.Unlock()
		hook(edgeID, session)
		f.mu.Lock()
	}
	return nil
}

func (f *fakeStreams) open(edge, session string) {
	f.mu.Lock()
	f.streams[edge] = session
	f.mu.Unlock()
}

func (f *fakeStreams) close(edge string) {
	f.mu.Lock()
	delete(f.streams, edge)
	f.mu.Unlock()
}

func (f *fakeStreams) sends() []sentReload {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sentReload(nil), f.sent...)
}

// replica is one Studio replica: a registered node, its streams and its
// coordinator (driven by hand: dispatch and janitor are called directly).
type replica struct {
	id      string
	node    *cluster.Node
	streams *fakeStreams
	c       *Coordinator
}

func newReplica(t *testing.T, db *gorm.DB, id string, opts Options) *replica {
	t.Helper()
	n, err := cluster.StartNode(db, id, "test")
	require.NoError(t, err)
	t.Cleanup(func() { n.Stop(context.Background()) })
	fs := newFakeStreams()
	return &replica{id: id, node: n, streams: fs, c: New(db, id, fs, opts)}
}

func addEdge(t *testing.T, db *gorm.DB, id, ns, status string, owner string) {
	t.Helper()
	now := time.Now()
	e := models.EdgeInstance{EdgeID: id, Namespace: ns, Status: status, OwnerNodeID: owner, LastHeartbeat: &now}
	require.NoError(t, db.Create(&e).Error)
}

func setChecksums(t *testing.T, db *gorm.DB, ns, expected string, edgeLoaded map[string]string) {
	t.Helper()
	st := models.NamespaceSyncStatus{Namespace: models.CanonicalNamespace(ns), ExpectedChecksum: expected}
	require.NoError(t, st.Upsert(db))
	for edge, loaded := range edgeLoaded {
		require.NoError(t, db.Model(&models.EdgeInstance{}).Where("edge_id = ?", edge).Update("loaded_checksum", loaded).Error)
	}
}

func command(t *testing.T, db *gorm.DB, op, edge string) models.EdgePushCommand {
	t.Helper()
	var cmd models.EdgePushCommand
	require.NoError(t, db.Where("operation_id = ? AND edge_id = ?", op, edge).First(&cmd).Error)
	return cmd
}

func operation(t *testing.T, db *gorm.DB, op string) models.PushOperation {
	t.Helper()
	var o models.PushOperation
	require.NoError(t, db.Where("operation_id = ?", op).First(&o).Error)
	return o
}

func respond(c *Coordinator, op, edge string, phase pb.ReloadPhase, msg string) {
	c.HandleReloadResponse(&pb.ConfigurationReloadResponse{OperationId: op, EdgeId: edge, Phase: phase, Success: phase != pb.ReloadPhase_FAILED, Message: msg})
}

func push(t *testing.T, c *Coordinator, req Request) *Result {
	t.Helper()
	if req.InitiatedBy == "" {
		req.InitiatedBy = "admin@example.com"
	}
	res, err := c.Push(context.Background(), req)
	require.NoError(t, err)
	return res
}

// A push issued on replica A for an edge whose stream is on B: only B
// delivers; the edge's READY (matching checksum) settles it; A reports it.
func TestPushIsDeliveredByTheReplicaHoldingTheStream(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		a := newReplica(t, db, "node-a", Options{})
		b := newReplica(t, db, "node-b", Options{})
		addEdge(t, db, "edge-1", "default", models.EdgeStatusConnected, "node-b")
		b.streams.open("edge-1", "s1")

		res := push(t, a.c, Request{Scope: ScopeEdge, EdgeIDs: []string{"edge-1"}})
		op := res.Operation.OperationID
		require.Len(t, res.Targets, 1)
		assert.True(t, res.Targets[0].Reachable)
		assert.Empty(t, res.Warnings)

		a.c.dispatch()
		assert.Empty(t, a.streams.sends(), "A does not hold the stream")
		b.c.dispatch()
		require.Len(t, b.streams.sends(), 1)
		sent := b.streams.sends()[0]
		assert.Equal(t, op, sent.req.OperationId)
		assert.Equal(t, []string{"edge-1"}, sent.req.TargetEdges)
		assert.Equal(t, models.PushCommandSent, command(t, db, op, "edge-1").Status)

		setChecksums(t, db, "default", "abc", map[string]string{"edge-1": "abc"})
		respond(b.c, op, "edge-1", pb.ReloadPhase_PONG, "ack")
		respond(b.c, op, "edge-1", pb.ReloadPhase_READY, "ready")

		cmd := command(t, db, op, "edge-1")
		assert.Equal(t, models.PushCommandSucceeded, cmd.Status)
		assert.Equal(t, 1, cmd.Attempts)
		assert.NotNil(t, cmd.CompletedAt)

		st, err := a.c.Status(context.Background(), op) // from the other replica
		require.NoError(t, err)
		assert.Equal(t, models.PushOperationSucceeded, st.Operation.Status)
		assert.Equal(t, 100, st.Progress)
		assert.Contains(t, st.Message, "All 1 edge(s)")
	})
}

// An edge that is not connected when the push is issued gets it as soon as
// it connects to any replica; one that never connects expires with the
// reason.
func TestOfflineEdgeWaitsForTheDeadline(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		a := newReplica(t, db, "node-a", Options{})
		addEdge(t, db, "edge-late", "default", models.EdgeStatusDisconnected, "")
		addEdge(t, db, "edge-gone", "default", models.EdgeStatusDisconnected, "")

		res := push(t, a.c, Request{Scope: ScopeNamespace, Namespace: "default"})
		op := res.Operation.OperationID
		require.Len(t, res.Targets, 2, "recently disconnected edges are waited for")
		assert.False(t, res.Targets[0].Reachable)
		require.NotEmpty(t, res.Warnings)
		assert.Contains(t, res.Warnings[0], "2 of 2 edge(s) are not connected")

		a.c.dispatch()
		assert.Empty(t, a.streams.sends())

		// edge-late connects to A.
		a.streams.open("edge-late", "s1")
		a.c.StreamOpened("edge-late")
		a.c.dispatch()
		require.Len(t, a.streams.sends(), 1)
		respond(a.c, op, "edge-late", pb.ReloadPhase_READY, "")
		assert.Equal(t, models.PushCommandSucceeded, command(t, db, op, "edge-late").Status)

		// edge-gone never comes back: past the deadline, it expires.
		require.NoError(t, db.Model(&models.EdgePushCommand{}).Where("operation_id = ?", op).Update("deadline_at", utcNow().Add(-time.Second)).Error)
		a.c.janitor()
		gone := command(t, db, op, "edge-gone")
		assert.Equal(t, models.PushCommandExpired, gone.Status)
		assert.Contains(t, gone.Message, "not connected to any control-plane replica")
		assert.Equal(t, models.PushOperationPartiallyFailed, operation(t, db, op).Status)
	})
}

// The stream breaks after the push was sent and before the edge answered;
// the edge reconnects to another replica, which delivers attempt 2.
func TestStreamLossMidReloadIsRetriedOnTheNewReplica(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		a := newReplica(t, db, "node-a", Options{})
		b := newReplica(t, db, "node-b", Options{})
		addEdge(t, db, "edge-1", "default", models.EdgeStatusConnected, "node-a")
		a.streams.open("edge-1", "sa")

		op := push(t, a.c, Request{Scope: ScopeEdge, EdgeIDs: []string{"edge-1"}}).Operation.OperationID
		a.c.dispatch()
		respond(a.c, op, "edge-1", pb.ReloadPhase_PULL_STARTED, "pulling")

		a.streams.close("edge-1")
		a.c.StreamClosed("edge-1", "sa")
		cmd := command(t, db, op, "edge-1")
		assert.Equal(t, models.PushCommandPending, cmd.Status)
		assert.Equal(t, 1, cmd.Attempts)
		assert.Contains(t, cmd.Message, "closed before it answered")

		b.streams.open("edge-1", "sb")
		b.c.dispatch()
		require.Len(t, b.streams.sends(), 1)
		assert.Equal(t, "sb", b.streams.sends()[0].session)
		respond(b.c, op, "edge-1", pb.ReloadPhase_READY, "")

		cmd = command(t, db, op, "edge-1")
		assert.Equal(t, models.PushCommandSucceeded, cmd.Status)
		assert.Equal(t, 2, cmd.Attempts)
		require.Len(t, cmd.History, 4, "sent, closed, sent, succeeded: %+v", cmd.History)
		assert.Contains(t, cmd.History[1].Outcome, "closed before it answered")
		assert.Equal(t, "node-b", cmd.History[2].Node)
		assert.Equal(t, "node-a", cmd.History[0].Node)
	})
}

// A replica that dies holding a sent command: another replica's janitor
// returns it to pending, and the edge's new replica delivers it.
func TestDeadReplicasCommandsAreTakenOver(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		a := newReplica(t, db, "node-a", Options{})
		b := newReplica(t, db, "node-b", Options{})
		addEdge(t, db, "edge-1", "default", models.EdgeStatusConnected, "node-b")
		b.streams.open("edge-1", "sb")

		op := push(t, a.c, Request{Scope: ScopeEdge, EdgeIDs: []string{"edge-1"}}).Operation.OperationID
		b.c.dispatch()
		require.Equal(t, models.PushCommandSent, command(t, db, op, "edge-1").Status)

		// B crashes (its registration is removed, as expiry would).
		b.node.Stop(context.Background())
		a.c.janitor()
		cmd := command(t, db, op, "edge-1")
		assert.Equal(t, models.PushCommandPending, cmd.Status)
		assert.Contains(t, cmd.Message, "replica node-b stopped")

		a.streams.open("edge-1", "sa")
		a.c.dispatch()
		require.Len(t, a.streams.sends(), 1)
		respond(a.c, op, "edge-1", pb.ReloadPhase_READY, "")
		assert.Equal(t, models.PushCommandSucceeded, command(t, db, op, "edge-1").Status)
	})
}

// A replica that claimed a command and never sent it (stuck, or crashed
// between claim and send while still registered) loses the claim.
func TestLapsedClaimIsRequeued(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		a := newReplica(t, db, "node-a", Options{})
		addEdge(t, db, "edge-1", "default", models.EdgeStatusConnected, "node-a")
		op := push(t, a.c, Request{Scope: ScopeEdge, EdgeIDs: []string{"edge-1"}}).Operation.OperationID
		past := utcNow().Add(-time.Minute)
		require.NoError(t, db.Model(&models.EdgePushCommand{}).Where("operation_id = ?", op).Updates(map[string]interface{}{
			"status": models.PushCommandClaimed, "claimed_by": "node-a", "claim_expires_at": past, "attempts": 1}).Error)
		a.c.janitor()
		cmd := command(t, db, op, "edge-1")
		assert.Equal(t, models.PushCommandPending, cmd.Status)
		assert.Contains(t, cmd.Message, "did not send it")
	})
}

// Two replicas both hold a stream for the edge (it reconnected and the old
// stream has not been noticed as closed): exactly one delivers.
func TestOnlyOneReplicaClaims(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		a := newReplica(t, db, "node-a", Options{})
		b := newReplica(t, db, "node-b", Options{})
		addEdge(t, db, "edge-1", "default", models.EdgeStatusConnected, "node-b")
		a.streams.open("edge-1", "old")
		b.streams.open("edge-1", "new")
		op := push(t, a.c, Request{Scope: ScopeEdge, EdgeIDs: []string{"edge-1"}}).Operation.OperationID

		var wg sync.WaitGroup
		for _, r := range []*replica{a, b, a, b} {
			wg.Add(1)
			go func(r *replica) { defer wg.Done(); r.c.dispatch() }(r)
		}
		wg.Wait()
		assert.Equal(t, 1, len(a.streams.sends())+len(b.streams.sends()), "exactly one send")
		assert.Equal(t, 1, command(t, db, op, "edge-1").Attempts)
	})
}

// An edge that reports FAILED is not retried; its message is the outcome.
func TestEdgeFailureIsFinal(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		a := newReplica(t, db, "node-a", Options{})
		addEdge(t, db, "edge-1", "default", models.EdgeStatusConnected, "node-a")
		a.streams.open("edge-1", "s")
		op := push(t, a.c, Request{Scope: ScopeEdge, EdgeIDs: []string{"edge-1"}}).Operation.OperationID
		a.c.dispatch()
		respond(a.c, op, "edge-1", pb.ReloadPhase_FAILED, "Failed to update SQLite: constraint violation")

		cmd := command(t, db, op, "edge-1")
		assert.Equal(t, models.PushCommandFailed, cmd.Status)
		assert.Equal(t, "Failed to update SQLite: constraint violation", cmd.Message)
		a.c.dispatch()
		a.c.janitor()
		assert.Len(t, a.streams.sends(), 1, "not retried")
		assert.Equal(t, models.PushOperationFailed, operation(t, db, op).Status)
	})
}

// READY whose loaded checksum no longer matches the namespace: a warning.
func TestReadyWithStaleChecksumIsAWarning(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		a := newReplica(t, db, "node-a", Options{})
		addEdge(t, db, "edge-1", "default", models.EdgeStatusConnected, "node-a")
		a.streams.open("edge-1", "s")
		op := push(t, a.c, Request{Scope: ScopeEdge, EdgeIDs: []string{"edge-1"}}).Operation.OperationID
		a.c.dispatch()
		setChecksums(t, db, "default", "newer-checksum", map[string]string{"edge-1": "older-checksum"})
		respond(a.c, op, "edge-1", pb.ReloadPhase_READY, "")

		cmd := command(t, db, op, "edge-1")
		assert.Equal(t, models.PushCommandSucceededWarning, cmd.Status)
		assert.Contains(t, cmd.Warning, "configuration changed during the push")
		assert.Equal(t, "newer-checksum", cmd.ExpectedChecksum)
		assert.Equal(t, "older-checksum", cmd.LoadedChecksum)
		assert.Equal(t, models.PushOperationSucceededWarning, operation(t, db, op).Status)
	})
}

// An edge that never answers is retried up to the attempt limit, then
// failed with the history of every attempt. A progress report resets the
// answer timeout.
func TestSilentEdgeIsRetriedThenFailed(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		a := newReplica(t, db, "node-a", Options{AnswerTimeout: time.Minute})
		addEdge(t, db, "edge-1", "default", models.EdgeStatusConnected, "node-a")
		a.streams.open("edge-1", "s")
		op := push(t, a.c, Request{Scope: ScopeEdge, EdgeIDs: []string{"edge-1"}}).Operation.OperationID
		ago := func(d time.Duration) time.Time { return utcNow().Add(-d) }

		for attempt := 1; attempt <= 3; attempt++ {
			a.c.dispatch()
			require.Len(t, a.streams.sends(), attempt)
			// A recent progress report keeps it alive past the send time.
			require.NoError(t, db.Model(&models.EdgePushCommand{}).Where("operation_id = ?", op).
				Updates(map[string]interface{}{"sent_at": ago(2 * time.Minute), "phase_at": ago(time.Second)}).Error)
			a.c.janitor()
			require.Equal(t, models.PushCommandSent, command(t, db, op, "edge-1").Status, "progress resets the timeout")
			// Then silence.
			require.NoError(t, db.Model(&models.EdgePushCommand{}).Where("operation_id = ?", op).
				Update("phase_at", ago(2*time.Minute)).Error)
			a.c.janitor()
		}
		cmd := command(t, db, op, "edge-1")
		assert.Equal(t, models.PushCommandFailed, cmd.Status)
		assert.Contains(t, cmd.Message, "did not answer within 1m0s; gave up after 3 attempts")
		assert.Equal(t, 3, cmd.Attempts)
		a.c.dispatch()
		assert.Len(t, a.streams.sends(), 3, "no fourth attempt")
	})
}

// A failed send uses an attempt and is retried on the next dispatch.
func TestSendFailureIsRetried(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		a := newReplica(t, db, "node-a", Options{})
		addEdge(t, db, "edge-1", "default", models.EdgeStatusConnected, "node-a")
		a.streams.open("edge-1", "s")
		op := push(t, a.c, Request{Scope: ScopeEdge, EdgeIDs: []string{"edge-1"}}).Operation.OperationID
		a.streams.failing = errors.New("transport is closing")
		a.c.dispatch()
		cmd := command(t, db, op, "edge-1")
		assert.Equal(t, models.PushCommandPending, cmd.Status)
		assert.Equal(t, 1, cmd.Attempts)
		assert.Contains(t, cmd.Message, "transport is closing")

		a.streams.failing = nil
		a.c.dispatch()
		assert.Len(t, a.streams.sends(), 1)
		assert.Equal(t, 2, command(t, db, op, "edge-1").Attempts)
	})
}

// Reports that arrive late, twice, or for pushes that do not exist change
// nothing.
func TestDuplicateLateAndUnknownResponsesAreIgnored(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		a := newReplica(t, db, "node-a", Options{})
		addEdge(t, db, "edge-1", "default", models.EdgeStatusConnected, "node-a")
		a.streams.open("edge-1", "s")
		op := push(t, a.c, Request{Scope: ScopeEdge, EdgeIDs: []string{"edge-1"}}).Operation.OperationID
		a.c.dispatch()
		respond(a.c, op, "edge-1", pb.ReloadPhase_READY, "first")
		first := command(t, db, op, "edge-1")

		respond(a.c, op, "edge-1", pb.ReloadPhase_FAILED, "late failure")
		respond(a.c, op, "edge-1", pb.ReloadPhase_READY, "again")
		respond(a.c, op, "edge-9", pb.ReloadPhase_READY, "wrong edge")
		respond(a.c, "push-unknown", "edge-1", pb.ReloadPhase_READY, "unknown op")
		a.c.HandleReloadResponse(nil)

		after := command(t, db, op, "edge-1")
		assert.Equal(t, models.PushCommandSucceeded, after.Status)
		assert.Equal(t, "first", after.Message)
		assert.Equal(t, first.CompletedAt.Unix(), after.CompletedAt.Unix())
	})
}

// Which edges a namespace push targets: connected and registered ones and
// recently disconnected ones; long-offline ones are skipped with a warning;
// nothing to push to is an error that says why.
func TestNamespacePushTargets(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		a := newReplica(t, db, "node-a", Options{RecentlyOffline: time.Hour})
		addEdge(t, db, "connected", "ns1", models.EdgeStatusConnected, "node-a")
		addEdge(t, db, "registered", "ns1", models.EdgeStatusRegistered, "")
		addEdge(t, db, "blip", "ns1", models.EdgeStatusDisconnected, "")
		addEdge(t, db, "gone", "ns1", models.EdgeStatusDisconnected, "")
		addEdge(t, db, "other-ns", "ns2", models.EdgeStatusConnected, "node-a")
		old := utcNow().Add(-48 * time.Hour)
		require.NoError(t, db.Model(&models.EdgeInstance{}).Where("edge_id = ?", "gone").Update("last_heartbeat", old).Error)

		res := push(t, a.c, Request{Scope: ScopeNamespace, Namespace: "ns1"})
		var ids []string
		for _, tg := range res.Targets {
			ids = append(ids, tg.EdgeID)
		}
		assert.ElementsMatch(t, []string{"connected", "registered", "blip"}, ids)
		require.Len(t, res.Skipped, 1)
		assert.Equal(t, "gone", res.Skipped[0].EdgeID)
		assert.Contains(t, res.Skipped[0].Reason, "offline since")
		assert.Len(t, res.Warnings, 2)

		all := push(t, a.c, Request{Scope: ScopeAll})
		assert.Len(t, all.Targets, 4)

		_, err := a.c.Push(context.Background(), Request{Scope: ScopeNamespace, Namespace: "empty"})
		assert.ErrorIs(t, err, ErrNoTargets)
		_, err = a.c.Push(context.Background(), Request{Scope: ScopeEdge, EdgeIDs: []string{"connected", "nope"}})
		assert.ErrorIs(t, err, ErrEdgeNotFound)
		assert.Contains(t, err.Error(), "nope")
	})
}

// Operation status from mixed outcomes, and the status report's counts.
func TestOperationStatusFromMixedOutcomes(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		a := newReplica(t, db, "node-a", Options{})
		for _, id := range []string{"e1", "e2", "e3"} {
			addEdge(t, db, id, "default", models.EdgeStatusConnected, "node-a")
			a.streams.open(id, "s-"+id)
		}
		op := push(t, a.c, Request{Scope: ScopeNamespace, Namespace: "default"}).Operation.OperationID
		a.c.dispatch()
		respond(a.c, op, "e1", pb.ReloadPhase_READY, "")
		respond(a.c, op, "e2", pb.ReloadPhase_FAILED, "boom")

		st, err := a.c.Status(context.Background(), op)
		require.NoError(t, err)
		assert.Equal(t, models.PushOperationInProgress, st.Operation.Status)
		assert.Equal(t, 66, st.Progress)
		assert.Equal(t, 1, st.Counts[models.PushCommandSent])
		assert.Contains(t, st.Message, "1 updated, 1 failed, 1 reloading")

		respond(a.c, op, "e3", pb.ReloadPhase_READY, "")
		assert.Equal(t, models.PushOperationPartiallyFailed, operation(t, db, op).Status)

		_, err = a.c.Status(context.Background(), "push-nope")
		assert.ErrorIs(t, err, ErrOperationNotFound)
	})
}

func TestOperationStatusRules(t *testing.T) {
	cases := []struct {
		counts map[string]int
		want   string
	}{
		{map[string]int{models.PushCommandSucceeded: 2}, models.PushOperationSucceeded},
		{map[string]int{models.PushCommandSucceeded: 1, models.PushCommandSucceededWarning: 1}, models.PushOperationSucceededWarning},
		{map[string]int{models.PushCommandSucceeded: 1, models.PushCommandPending: 1}, models.PushOperationInProgress},
		{map[string]int{models.PushCommandSucceeded: 1, models.PushCommandExpired: 1}, models.PushOperationPartiallyFailed},
		{map[string]int{models.PushCommandExpired: 2}, models.PushOperationExpired},
		{map[string]int{models.PushCommandExpired: 1, models.PushCommandFailed: 1}, models.PushOperationFailed},
		{map[string]int{}, models.PushOperationFailed},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, operationStatus(tc.counts), "%v", tc.counts)
	}
}

// utcNow is how every push timestamp is written: SQLite compares the stored
// text, so a timestamp in another zone would compare wrongly.
func utcNow() time.Time { return time.Now().UTC() }

// The stream closes between the send and the record of it: the close's
// requeue wins, the late "sent" record must not undo it, and the history
// keeps every step.
func TestStreamClosingWhileTheSendIsRecorded(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		a := newReplica(t, db, "node-a", Options{})
		addEdge(t, db, "edge-1", "default", models.EdgeStatusConnected, "node-a")
		a.streams.open("edge-1", "s1")
		op := push(t, a.c, Request{Scope: ScopeEdge, EdgeIDs: []string{"edge-1"}}).Operation.OperationID
		a.streams.onSend = func(edge, session string) {
			a.streams.close(edge)
			a.c.StreamClosed(edge, session)
		}
		a.c.dispatch()

		cmd := command(t, db, op, "edge-1")
		assert.Equal(t, models.PushCommandPending, cmd.Status, "requeued, not left as sent on a dead stream")
		assert.Empty(t, cmd.StreamSessionID)
		assert.Equal(t, 1, cmd.Attempts)
		require.Len(t, cmd.History, 1)
		assert.Contains(t, cmd.History[0].Outcome, "closed before it answered")

		// And the other order: recorded as sent, then the close.
		a.streams.onSend = nil
		a.streams.open("edge-1", "s2")
		a.c.dispatch()
		require.Equal(t, models.PushCommandSent, command(t, db, op, "edge-1").Status)
		a.streams.close("edge-1")
		a.c.StreamClosed("edge-1", "s2")
		cmd = command(t, db, op, "edge-1")
		assert.Equal(t, models.PushCommandPending, cmd.Status)
		require.Len(t, cmd.History, 3, "%+v", cmd.History)
		assert.Equal(t, "sent", cmd.History[1].Outcome)
	})
}

// A concurrent change between a read and a write is never lost: many
// progress reports and a READY racing a stream close all land in order.
func TestConcurrentChangesKeepEveryHistoryEntry(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		a := newReplica(t, db, "node-a", Options{})
		b := newReplica(t, db, "node-b", Options{})
		addEdge(t, db, "edge-1", "default", models.EdgeStatusConnected, "node-a")
		a.streams.open("edge-1", "s1")
		op := push(t, a.c, Request{Scope: ScopeEdge, EdgeIDs: []string{"edge-1"}}).Operation.OperationID
		a.c.dispatch()

		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(2)
			go func() { defer wg.Done(); respond(a.c, op, "edge-1", pb.ReloadPhase_UPDATING, "") }()
			go func() { defer wg.Done(); respond(b.c, op, "edge-1", pb.ReloadPhase_PULL_STARTED, "") }()
		}
		wg.Add(2)
		go func() { defer wg.Done(); respond(b.c, op, "edge-1", pb.ReloadPhase_READY, "") }()
		go func() { defer wg.Done(); a.c.StreamClosed("edge-1", "s1") }()
		wg.Wait()

		cmd := command(t, db, op, "edge-1")
		var outcomes []string
		for _, h := range cmd.History {
			outcomes = append(outcomes, h.Outcome)
		}
		assert.Equal(t, "sent", outcomes[0])
		switch cmd.Status {
		case models.PushCommandSucceeded:
			// READY first; the close found nothing in flight, or came
			// after it requeued and READY still settled it.
			assert.Contains(t, outcomes, models.PushCommandSucceeded)
		case models.PushCommandPending:
			t.Fatalf("READY was lost: %v", outcomes)
		default:
			t.Fatalf("unexpected status %s: %v", cmd.Status, outcomes)
		}
		assert.Greater(t, cmd.Version, int64(1))
	})
}

// A replica whose view of its streams is a moment old claims a push for a
// stream that has just closed: nothing is sent, and no attempt is used.
func TestSendOnAStreamThatJustClosedUsesNoAttempt(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		a := newReplica(t, db, "node-a", Options{MaxAttempts: 1})
		addEdge(t, db, "edge-1", "default", models.EdgeStatusConnected, "node-a")
		a.streams.open("edge-1", "old")
		op := push(t, a.c, Request{Scope: ScopeEdge, EdgeIDs: []string{"edge-1"}}).Operation.OperationID

		// The dispatcher's snapshot says "old"; the edge is on "new" by the
		// time it sends.
		a.streams.open("edge-1", "new")
		var cmd models.EdgePushCommand
		require.NoError(t, db.Where("operation_id = ?", op).First(&cmd).Error)
		a.c.deliver(cmd.ID, "old", a.c.now())

		cmd = command(t, db, op, "edge-1")
		assert.Equal(t, models.PushCommandPending, cmd.Status)
		assert.Equal(t, 0, cmd.Attempts, "nothing was sent")
		assert.Empty(t, cmd.History)
		assert.Empty(t, a.streams.sends())

		// Even with a single attempt allowed, the current stream gets it.
		a.c.dispatch()
		require.Len(t, a.streams.sends(), 1)
		assert.Equal(t, "new", a.streams.sends()[0].session)
		respond(a.c, op, "edge-1", pb.ReloadPhase_READY, "")
		assert.Equal(t, models.PushCommandSucceeded, command(t, db, op, "edge-1").Status)
	})
}
