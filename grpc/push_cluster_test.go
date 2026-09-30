package grpc

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/cluster"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/services/pushes"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

// These tests run the real control server and push coordinator of two or
// three replicas on one database, with fake edges on the server side of
// each stream. On Postgres (DATABASE_URL) replicas wake each other with
// NOTIFY; on SQLite they find work by polling.

func forEachClusterDB(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "c.db")+"?_busy_timeout=10000&_journal_mode=WAL"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		require.NoError(t, models.InitModels(db))
		body(t, db)
	})
	base := os.Getenv("DATABASE_URL")
	if base == "" {
		return
	}
	t.Run("postgres", func(t *testing.T) {
		admin, err := gorm.Open(postgres.Open(base), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		schema := fmt.Sprintf("push_cluster_%d", time.Now().UnixNano())
		require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
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
			admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`)
			if s, err := admin.DB(); err == nil {
				s.Close()
			}
		})
		require.NoError(t, models.InitModels(db))
		body(t, db)
	})
}

type pushReplica struct {
	id     string
	server *ControlServer
	pushes *pushes.Coordinator
	node   *cluster.Node
}

func fastPushOptions() pushes.Options {
	return pushes.Options{PollInterval: 25 * time.Millisecond, JanitorInterval: 50 * time.Millisecond, Deadline: 30 * time.Second}
}

func startPushReplica(t *testing.T, db *gorm.DB, id string, opts pushes.Options) *pushReplica {
	t.Helper()
	return startPushReplicaWithGrace(t, db, id, opts, 0)
}

// startPushReplicaWithGrace starts a replica whose streams take pushes only
// after a heartbeat or the grace period (see ControlServer.pushReady).
func startPushReplicaWithGrace(t *testing.T, db *gorm.DB, id string, opts pushes.Options, grace time.Duration) *pushReplica {
	t.Helper()
	node, err := cluster.StartNode(db, id, "test")
	require.NoError(t, err)
	server := replicaServer(t, db, id)
	server.pushReadyGrace = grace
	c := pushes.New(db, id, server, opts)
	server.SetPushDelivery(c)
	require.NoError(t, c.Start(context.Background()))
	r := &pushReplica{id: id, server: server, pushes: c, node: node}
	t.Cleanup(r.stop)
	return r
}

func (r *pushReplica) stop() {
	r.pushes.Stop()
	r.node.Stop(context.Background())
}

// fakeEdge answers the pushes sent on its stream the way the microgateway's
// EdgeReloadHandler does (PONG, PULL_STARTED, then READY or FAILED), after
// optionally reporting its loaded checksum.
type fakeEdge struct {
	t      *testing.T
	id     string
	stream *fakeEdgeStream
	close  func()

	mu       sync.Mutex
	answered map[int]bool
	answer   func(req *pb.ConfigurationReloadRequest) (phase pb.ReloadPhase, message string, silent bool)
	stopCh   chan struct{}
	stopOnce sync.Once
}

func startFakeEdge(t *testing.T, r *pushReplica, edgeID, ns string) *fakeEdge {
	t.Helper()
	registerEdge(t, r.server, edgeID, ns)
	stream, closeStream := connect(t, r.server, edgeID, ns)
	e := &fakeEdge{t: t, id: edgeID, stream: stream, close: closeStream, answered: map[int]bool{}, stopCh: make(chan struct{})}
	e.answer = func(*pb.ConfigurationReloadRequest) (pb.ReloadPhase, string, bool) {
		return pb.ReloadPhase_READY, "Configuration up to date, no changes needed", false
	}
	go e.run()
	t.Cleanup(e.disconnect)
	return e
}

func (e *fakeEdge) run() {
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-e.stopCh:
			return
		case <-tick.C:
		}
		msgs := e.stream.sent()
		for i, m := range msgs {
			req := m.GetReloadRequest()
			e.mu.Lock()
			seen := e.answered[i]
			e.answered[i] = true
			answer := e.answer
			e.mu.Unlock()
			if req == nil || seen {
				continue
			}
			phase, msg, silent := answer(req)
			if silent {
				continue
			}
			for _, p := range []pb.ReloadPhase{pb.ReloadPhase_PONG, pb.ReloadPhase_PULL_STARTED, phase} {
				m := msg
				if p != phase {
					m = p.String()
				}
				select {
				case e.stream.in <- &pb.EdgeMessage{Message: &pb.EdgeMessage_ReloadResponse{ReloadResponse: &pb.ConfigurationReloadResponse{
					OperationId: req.OperationId, EdgeId: e.id, Phase: p, Success: p != pb.ReloadPhase_FAILED, Message: m}}}:
				case <-e.stopCh:
					return
				}
			}
		}
	}
}

func (e *fakeEdge) setAnswer(fn func(req *pb.ConfigurationReloadRequest) (pb.ReloadPhase, string, bool)) {
	e.mu.Lock()
	e.answer = fn
	e.mu.Unlock()
}

// disconnect drops the edge's stream, as a network failure would.
func (e *fakeEdge) disconnect() {
	e.stopOnce.Do(func() {
		close(e.stopCh)
		e.close()
	})
}

func (e *fakeEdge) requests() []*pb.ConfigurationReloadRequest { return reloadRequests(e.stream) }

func waitForPush(t *testing.T, c *pushes.Coordinator, op string, want string) *pushes.OperationStatus {
	t.Helper()
	var st *pushes.OperationStatus
	require.Eventually(t, func() bool {
		var err error
		st, err = c.Status(context.Background(), op)
		require.NoError(t, err)
		return st.Operation.Status == want
	}, 20*time.Second, 25*time.Millisecond, "push %s never reached %s", op, want)
	return st
}

func lastStatus(t *testing.T, c *pushes.Coordinator, op string) string {
	st, err := c.Status(context.Background(), op)
	require.NoError(t, err)
	return fmt.Sprintf("%s: %s %+v", st.Operation.Status, st.Message, st.Commands)
}

func pushTo(t *testing.T, c *pushes.Coordinator, req pushes.Request) string {
	t.Helper()
	req.InitiatedBy = "admin@example.com"
	res, err := c.Push(context.Background(), req)
	require.NoError(t, err)
	return res.Operation.OperationID
}

// A push issued on A for an edge whose stream is on B is delivered by B,
// once, and both replicas report the same outcome.
func TestPushCluster_DeliveredByTheOwningReplica(t *testing.T) {
	forEachClusterDB(t, func(t *testing.T, db *gorm.DB) {
		a := startPushReplica(t, db, "node-a", fastPushOptions())
		b := startPushReplica(t, db, "node-b", fastPushOptions())
		edge := startFakeEdge(t, b, "edge-1", "default")

		op := pushTo(t, a.pushes, pushes.Request{Scope: pushes.ScopeEdge, EdgeIDs: []string{"edge-1"}})
		st := waitForPush(t, a.pushes, op, models.PushOperationSucceeded)
		require.Len(t, st.Commands, 1)
		assert.Equal(t, 1, st.Commands[0].Attempts)
		assert.Equal(t, "node-b", st.Commands[0].History[0].Node)

		fromB, err := b.pushes.Status(context.Background(), op)
		require.NoError(t, err)
		assert.Equal(t, models.PushOperationSucceeded, fromB.Operation.Status)

		reqs := edge.requests()
		require.Len(t, reqs, 1, "delivered exactly once")
		assert.Equal(t, op, reqs[0].OperationId)
		assert.Equal(t, []string{"edge-1"}, reqs[0].TargetEdges)
		assert.Equal(t, "admin@example.com", reqs[0].InitiatedBy)
		assert.Greater(t, reqs[0].TimeoutSeconds, int64(0))
	})
}

// A namespace push reaches edges spread over three replicas, each once;
// edges in other namespaces get nothing. (Community builds register every
// edge in "default", so the other namespace's edge is written directly.)
func TestPushCluster_NamespaceAcrossThreeReplicas(t *testing.T) {
	forEachClusterDB(t, func(t *testing.T, db *gorm.DB) {
		reps := []*pushReplica{
			startPushReplica(t, db, "node-a", fastPushOptions()),
			startPushReplica(t, db, "node-b", fastPushOptions()),
			startPushReplica(t, db, "node-c", fastPushOptions()),
		}
		var edges []*fakeEdge
		for i, r := range reps {
			edges = append(edges, startFakeEdge(t, r, fmt.Sprintf("edge-%d", i), "default"))
		}
		now := time.Now()
		require.NoError(t, db.Create(&models.EdgeInstance{EdgeID: "edge-other", Namespace: "ns2", Status: models.EdgeStatusConnected, OwnerNodeID: "node-a", LastHeartbeat: &now}).Error)

		op := pushTo(t, reps[1].pushes, pushes.Request{Scope: pushes.ScopeNamespace, Namespace: "default"})
		st := waitForPush(t, reps[2].pushes, op, models.PushOperationSucceeded)
		require.Len(t, st.Commands, 3)
		for _, c := range st.Commands {
			assert.NotEqual(t, "edge-other", c.EdgeID)
		}
		for i, e := range edges {
			assert.Len(t, e.requests(), 1, e.id)
			assert.Equal(t, reps[i].id, st.Commands[i].History[0].Node, "delivered by the replica holding its stream")
		}
	})
}

// An edge that is offline when the push is issued gets it when it
// connects, to whichever replica.
func TestPushCluster_OfflineEdgeReceivesItOnConnect(t *testing.T) {
	forEachClusterDB(t, func(t *testing.T, db *gorm.DB) {
		a := startPushReplica(t, db, "node-a", fastPushOptions())
		b := startPushReplica(t, db, "node-b", fastPushOptions())
		now := time.Now()
		require.NoError(t, db.Create(&models.EdgeInstance{EdgeID: "edge-1", Namespace: "default", Status: models.EdgeStatusDisconnected, LastHeartbeat: &now}).Error)

		res, err := a.pushes.Push(context.Background(), pushes.Request{Scope: pushes.ScopeEdge, EdgeIDs: []string{"edge-1"}, InitiatedBy: "admin"})
		require.NoError(t, err)
		require.Len(t, res.Targets, 1)
		assert.False(t, res.Targets[0].Reachable)
		require.NotEmpty(t, res.Warnings)
		op := res.Operation.OperationID

		time.Sleep(200 * time.Millisecond)
		st, err := a.pushes.Status(context.Background(), op)
		require.NoError(t, err)
		assert.Equal(t, models.PushCommandPending, st.Commands[0].Status, "waiting, not failed")
		require.Len(t, st.Targets, 1)
		assert.False(t, st.Targets[0].Reachable)

		startFakeEdge(t, b, "edge-1", "default")
		waitForPush(t, a.pushes, op, models.PushOperationSucceeded)
	})
}

// The edge's connection drops after the push reached it and before it
// answered; it reconnects to another replica, which delivers it again.
func TestPushCluster_StreamLostBeforeAnswer(t *testing.T) {
	forEachClusterDB(t, func(t *testing.T, db *gorm.DB) {
		a := startPushReplica(t, db, "node-a", fastPushOptions())
		b := startPushReplica(t, db, "node-b", fastPushOptions())
		first := startFakeEdge(t, a, "edge-1", "default")
		first.setAnswer(func(*pb.ConfigurationReloadRequest) (pb.ReloadPhase, string, bool) { return 0, "", true })

		op := pushTo(t, a.pushes, pushes.Request{Scope: pushes.ScopeEdge, EdgeIDs: []string{"edge-1"}})
		require.Eventually(t, func() bool { return len(first.requests()) == 1 }, 10*time.Second, 10*time.Millisecond)
		first.disconnect()

		second := startFakeEdge(t, b, "edge-1", "default")
		st := waitForPush(t, a.pushes, op, models.PushOperationSucceeded)
		cmd := st.Commands[0]
		assert.Equal(t, 2, cmd.Attempts)
		assert.Len(t, second.requests(), 1)
		var outcomes []string
		for _, h := range cmd.History {
			outcomes = append(outcomes, h.Node+": "+h.Outcome)
		}
		assert.Contains(t, outcomes, "node-a: the edge's connection to replica node-a closed before it answered")
	})
}

// A replica crashes after sending a push (no stream-close handling, its
// registration gone): another replica's janitor requeues it and the edge's
// new replica delivers it.
func TestPushCluster_ReplicaCrashAfterSending(t *testing.T) {
	forEachClusterDB(t, func(t *testing.T, db *gorm.DB) {
		a := startPushReplica(t, db, "node-a", fastPushOptions())
		b := startPushReplica(t, db, "node-b", fastPushOptions())
		first := startFakeEdge(t, a, "edge-1", "default")
		first.setAnswer(func(*pb.ConfigurationReloadRequest) (pb.ReloadPhase, string, bool) { return 0, "", true })

		op := pushTo(t, b.pushes, pushes.Request{Scope: pushes.ScopeEdge, EdgeIDs: []string{"edge-1"}})
		require.Eventually(t, func() bool { return len(first.requests()) == 1 }, 10*time.Second, 10*time.Millisecond)

		// A dies: nothing on A runs any more.
		a.server.SetPushDelivery(nil)
		a.stop()
		first.disconnect()

		startFakeEdge(t, b, "edge-1", "default")
		st := waitForPush(t, b.pushes, op, models.PushOperationSucceeded)
		assert.Equal(t, 2, st.Commands[0].Attempts)
		found := false
		for _, h := range st.Commands[0].History {
			if h.Outcome == "replica node-a stopped before the edge answered" {
				found = true
			}
		}
		assert.True(t, found, "history explains the retry: %+v", st.Commands[0].History)
	})
}

// FAILED from the edge is final: surfaced with the edge's message, and the
// push is not sent again.
func TestPushCluster_EdgeFailureIsSurfaced(t *testing.T) {
	forEachClusterDB(t, func(t *testing.T, db *gorm.DB) {
		a := startPushReplica(t, db, "node-a", fastPushOptions())
		ok := startFakeEdge(t, a, "edge-ok", "default")
		bad := startFakeEdge(t, a, "edge-bad", "default")
		bad.setAnswer(func(*pb.ConfigurationReloadRequest) (pb.ReloadPhase, string, bool) {
			return pb.ReloadPhase_FAILED, "Failed to update SQLite: disk full", false
		})

		op := pushTo(t, a.pushes, pushes.Request{Scope: pushes.ScopeNamespace, Namespace: "default"})
		st := waitForPush(t, a.pushes, op, models.PushOperationPartiallyFailed)
		byEdge := map[string]models.EdgePushCommand{}
		for _, c := range st.Commands {
			byEdge[c.EdgeID] = c
		}
		assert.Equal(t, models.PushCommandSucceeded, byEdge["edge-ok"].Status)
		assert.Equal(t, models.PushCommandFailed, byEdge["edge-bad"].Status)
		assert.Equal(t, "Failed to update SQLite: disk full", byEdge["edge-bad"].Message)
		assert.Contains(t, st.Message, "1 updated, 1 failed")

		time.Sleep(300 * time.Millisecond)
		assert.Len(t, bad.requests(), 1, "not retried")
		assert.Len(t, ok.requests(), 1)
	})
}

// Every push to a reconnecting, flapping edge: it gets the push on each new
// stream until one answers, and the attempts limit ends it otherwise.
func TestPushCluster_FlappingEdgeExhaustsAttempts(t *testing.T) {
	forEachClusterDB(t, func(t *testing.T, db *gorm.DB) {
		a := startPushReplica(t, db, "node-a", fastPushOptions())
		b := startPushReplica(t, db, "node-b", fastPushOptions())
		silent := func(*pb.ConfigurationReloadRequest) (pb.ReloadPhase, string, bool) { return 0, "", true }

		e := startFakeEdge(t, a, "edge-1", "default")
		e.setAnswer(silent)
		op := pushTo(t, a.pushes, pushes.Request{Scope: pushes.ScopeEdge, EdgeIDs: []string{"edge-1"}})
		for i, r := range []*pushReplica{b, a, b} {
			require.Eventually(t, func() bool { return len(e.requests()) == 1 }, 10*time.Second, 10*time.Millisecond, "attempt %d: %s", i+1, lastStatus(t, a.pushes, op))
			e.disconnect()
			if i < 2 {
				e = startFakeEdge(t, r, "edge-1", "default")
				e.setAnswer(silent)
			}
		}
		st := waitForPush(t, a.pushes, op, models.PushOperationFailed)
		cmd := st.Commands[0]
		assert.Equal(t, 3, cmd.Attempts)
		assert.Contains(t, cmd.Message, "gave up after 3 attempts")
		// Each attempt ended with its stream closing. (A "sent" entry is
		// missing when the stream closed before the send was recorded; the
		// close is recorded either way, see TestStreamClosingWhileTheSendIsRecorded.)
		closed := map[int]bool{}
		for _, h := range cmd.History {
			require.LessOrEqual(t, h.Attempt, 3, "no fourth attempt: %+v", cmd.History)
			if strings.Contains(h.Outcome, "closed before it answered") {
				closed[h.Attempt] = true
			}
		}
		assert.Equal(t, map[int]bool{1: true, 2: true, 3: true}, closed, "%+v", cmd.History)
	})
}

// An edge from before early heartbeats (v2.2) drops a push that reaches it
// while it is still starting up. Control waits for the stream's first
// heartbeat or the grace period before delivering, so the push waiting for
// it arrives once it can take it: one attempt, not a minute's timeout and a
// second attempt (M3).
func TestPushCluster_OldEdgeStartingUpGetsThePushAfterTheGrace(t *testing.T) {
	forEachClusterDB(t, func(t *testing.T, db *gorm.DB) {
		a := startPushReplicaWithGrace(t, db, "node-a", fastPushOptions(), 400*time.Millisecond)
		now := time.Now()
		require.NoError(t, db.Create(&models.EdgeInstance{EdgeID: "edge-1", Namespace: "default", Status: models.EdgeStatusDisconnected, LastHeartbeat: &now}).Error)
		op := pushTo(t, a.pushes, pushes.Request{Scope: pushes.ScopeEdge, EdgeIDs: []string{"edge-1"}})

		connectedAt := time.Now()
		e := startFakeEdge(t, a, "edge-1", "default")
		e.setAnswer(func(*pb.ConfigurationReloadRequest) (pb.ReloadPhase, string, bool) {
			if time.Since(connectedAt) < 300*time.Millisecond {
				return 0, "", true // no reload handler yet: dropped
			}
			return pb.ReloadPhase_READY, "Configuration up to date, no changes needed", false
		})
		st := waitForPush(t, a.pushes, op, models.PushOperationSucceeded)
		assert.Equal(t, 1, st.Commands[0].Attempts, "history: %+v", st.Commands[0].History)
	})
}
