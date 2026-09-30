// Package cluster runs real microgateway edges against several AI Studio
// control-plane replicas sharing one database, and checks that configuration
// pushes land on the edges (in the edges' own SQLite) whatever happens to
// the connections and the replicas in between. See
// features/ClusterControlPlane.md in the repository root.
package cluster

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	edgegrpc "github.com/TykTechnologies/midsommar/microgateway/internal/grpc"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	studiogrpc "github.com/TykTechnologies/midsommar/v2/grpc"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/cluster"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/services/pushes"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

const (
	authToken     = "cluster-e2e-token"
	encryptionKey = "12345678901234567890123456789012"
)

// studioDB opens one handle on the shared Studio database per replica:
// SQLite in a temp file, or a fresh schema on DATABASE_URL.
type studioDB struct {
	open func(t *testing.T) *gorm.DB
}

func forEachStudioDB(t *testing.T, body func(t *testing.T, sdb studioDB)) {
	t.Run("sqlite", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "studio.db")
		var once sync.Once
		var shared *gorm.DB
		body(t, studioDB{open: func(t *testing.T) *gorm.DB {
			// One process, one SQLite file: the replicas share a handle.
			once.Do(func() {
				db, err := gorm.Open(sqlite.Open(path+"?_busy_timeout=10000&_journal_mode=WAL"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
				require.NoError(t, err)
				require.NoError(t, models.InitModels(db))
				shared = db
			})
			return shared
		}})
	})
	base := os.Getenv("DATABASE_URL")
	if base == "" {
		return
	}
	t.Run("postgres", func(t *testing.T) {
		admin, err := gorm.Open(postgres.Open(base), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		schema := fmt.Sprintf("push_e2e_%d", time.Now().UnixNano())
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
		var once sync.Once
		body(t, studioDB{open: func(t *testing.T) *gorm.DB {
			db, err := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			t.Cleanup(func() {
				if s, err := db.DB(); err == nil {
					s.Close()
				}
			})
			once.Do(func() { require.NoError(t, models.InitModels(db)) })
			return db
		}})
	})
}

// replica is one AI Studio control-plane replica: its gRPC control server
// and its push coordinator.
type replica struct {
	id       string
	db       *gorm.DB
	addr     string
	server   *studiogrpc.ControlServer
	pushes   *pushes.Coordinator
	node     *cluster.Node
	stopOnce sync.Once
}

func startReplica(t *testing.T, sdb studioDB, id string) *replica {
	t.Helper()
	t.Setenv("MICROGATEWAY_ENCRYPTION_KEY", encryptionKey)
	db := sdb.open(t)
	node, err := cluster.StartNode(db, id, "e2e")
	require.NoError(t, err)
	server, err := studiogrpc.NewControlServer(&studiogrpc.Config{AuthToken: authToken, MaxConcurrentStreams: 100, NodeID: id}, db)
	require.NoError(t, err)
	c := pushes.New(db, id, server, pushes.Options{PollInterval: 50 * time.Millisecond, JanitorInterval: 100 * time.Millisecond, Deadline: time.Minute})
	server.SetPushDelivery(c)
	require.NoError(t, c.Start(context.Background()))

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = server.Serve(lis) }()
	r := &replica{id: id, db: db, addr: lis.Addr().String(), server: server, pushes: c, node: node}
	t.Cleanup(r.stop)
	return r
}

// stop shuts the replica down in order, as Studio does.
func (r *replica) stop() {
	r.stopOnce.Do(func() {
		r.server.Stop()
		r.pushes.Stop()
		r.node.Stop(context.Background())
	})
}

// crash stops the replica as if its process died: nothing it would do on
// the way down happens, and its registration is gone (as expiry would do).
func (r *replica) crash() {
	r.stopOnce.Do(func() {
		r.server.SetPushDelivery(nil)
		r.pushes.Stop()
		r.node.Stop(context.Background())
		r.server.Stop()
	})
}

// forwarder is the network between an edge and the control plane (a load
// balancer, say): it sends new connections to the current target and can
// cut every connection it carries.
type forwarder struct {
	t      *testing.T
	lis    net.Listener
	mu     sync.Mutex
	target string
	down   bool
	conns  map[net.Conn]struct{}
}

func newForwarder(t *testing.T, target string) *forwarder {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	f := &forwarder{t: t, lis: lis, target: target, conns: map[net.Conn]struct{}{}}
	go f.accept()
	t.Cleanup(func() { lis.Close(); f.cut() })
	return f
}

func (f *forwarder) addr() string { return f.lis.Addr().String() }

func (f *forwarder) accept() {
	for {
		in, err := f.lis.Accept()
		if err != nil {
			return
		}
		f.mu.Lock()
		target, down := f.target, f.down
		f.mu.Unlock()
		if down {
			in.Close()
			continue
		}
		out, err := net.Dial("tcp", target)
		if err != nil {
			in.Close()
			continue
		}
		f.mu.Lock()
		f.conns[in], f.conns[out] = struct{}{}, struct{}{}
		f.mu.Unlock()
		go f.pipe(in, out)
		go f.pipe(out, in)
	}
}

func (f *forwarder) pipe(dst, src net.Conn) {
	_, _ = io.Copy(dst, src)
	dst.Close()
	src.Close()
	f.mu.Lock()
	delete(f.conns, dst)
	delete(f.conns, src)
	f.mu.Unlock()
}

// cut drops every connection, as a network failure would.
func (f *forwarder) cut() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for c := range f.conns {
		c.Close()
	}
}

// route sends new connections to target (and cuts none).
func (f *forwarder) route(target string) {
	f.mu.Lock()
	f.target = target
	f.down = false
	f.mu.Unlock()
}

// setDown refuses new connections until route is called.
func (f *forwarder) setDown() {
	f.mu.Lock()
	f.down = true
	f.mu.Unlock()
}

// edge is a real microgateway edge: the edge client, the reload handler and
// the edge's own SQLite, wired as cmd/microgateway does.
type edge struct {
	id     string
	db     *gorm.DB
	client *edgegrpc.SimpleEdgeClient
	fwd    *forwarder
}

func startEdge(t *testing.T, id, namespace string, fwd *forwarder) *edge {
	t.Helper()
	return startEdgeWith(t, id, namespace, fwd, edgeOptions{})
}

// edgeOptions vary how an edge starts.
type edgeOptions struct {
	// handlerAfter sets the reload handler this long after the client has
	// connected, as cmd/microgateway does (it sets it once its services
	// are up); zero sets it before the client starts.
	handlerAfter time.Duration
}

func startEdgeWith(t *testing.T, id, namespace string, fwd *forwarder, opts edgeOptions) *edge {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), id+".db")+"?_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, database.Migrate(db))

	cfg := &config.Config{HubSpoke: config.HubSpokeConfig{
		Mode:              "edge",
		ControlEndpoint:   fwd.addr(),
		EdgeID:            id,
		EdgeNamespace:     namespace,
		ReconnectInterval: 100 * time.Millisecond,
		HeartbeatInterval: 300 * time.Millisecond,
		SyncTimeout:       5 * time.Second,
		ClientToken:       authToken,
		ClientTLSEnabled:  false,
		AllowInsecure:     true,
	}}
	client := edgegrpc.NewSimpleEdgeClient(cfg, "e2e", "hash", "now")
	syncService := services.NewEdgeSyncService(db, namespace)
	client.SetOnConfigChange(func(snap *pb.ConfigurationSnapshot) {
		if err := syncService.SyncConfiguration(snap); err != nil {
			t.Logf("edge %s: sync failed: %v", id, err)
		}
	})
	handler := services.NewEdgeReloadHandler(client, syncService, db, id, func(resp *pb.ConfigurationReloadResponse) {
		_ = client.SendReloadStatus(resp)
	}, nil)
	if opts.handlerAfter == 0 {
		client.SetReloadHandler(handler)
	}
	require.NoError(t, client.Start())
	t.Cleanup(func() { _ = client.Stop() })
	if opts.handlerAfter > 0 {
		time.AfterFunc(opts.handlerAfter, func() { client.SetReloadHandler(handler) })
	}
	return &edge{id: id, db: db, client: client, fwd: fwd}
}

// hasLLM reports whether the edge's own database holds the LLM.
func (e *edge) hasLLM(name string) bool {
	var n int64
	e.db.Model(&database.LLM{}).Where("name = ?", name).Count(&n)
	return n > 0
}

func streamOwner(t *testing.T, db *gorm.DB, edgeID string) (owner string, status string) {
	var row models.EdgeInstance
	if err := db.Where("edge_id = ?", edgeID).First(&row).Error; err != nil {
		return "", ""
	}
	return row.OwnerNodeID, row.Status
}

func waitOwner(t *testing.T, db *gorm.DB, edgeID, owner string) {
	t.Helper()
	require.Eventually(t, func() bool {
		o, s := streamOwner(t, db, edgeID)
		return o == owner && s == models.EdgeStatusConnected
	}, 20*time.Second, 25*time.Millisecond, "edge %s never streamed to %s", edgeID, owner)
}

// addLLM creates an LLM in Studio: a configuration change edges must get.
func addLLM(t *testing.T, db *gorm.DB, name string) {
	t.Helper()
	require.NoError(t, db.Create(&models.LLM{Name: name, Vendor: models.OPENAI, Active: true, APIEndpoint: "https://api.openai.com/v1", DefaultModel: "gpt-4o-mini", Namespace: ""}).Error)
}

func push(t *testing.T, r *replica, req pushes.Request) string {
	t.Helper()
	req.InitiatedBy = "admin@example.com"
	res, err := r.pushes.Push(context.Background(), req)
	require.NoError(t, err)
	return res.Operation.OperationID
}

func waitPush(t *testing.T, r *replica, op, want string, within time.Duration) *pushes.OperationStatus {
	t.Helper()
	var st *pushes.OperationStatus
	ok := assert.Eventually(t, func() bool {
		var err error
		st, err = r.pushes.Status(context.Background(), op)
		return err == nil && st.Operation.Status == want
	}, within, 50*time.Millisecond)
	if !ok {
		t.Fatalf("push %s never reached %s; last: %s %s %+v", op, want, st.Operation.Status, st.Message, st.Commands)
	}
	return st
}

// A change made and pushed on replica A lands in the SQLite of an edge
// streaming to replica B; both replicas report it.
func TestE2E_PushLandsOnEdgeOfAnotherReplica(t *testing.T) {
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		a := startReplica(t, sdb, "node-a")
		b := startReplica(t, sdb, "node-b")
		e := startEdge(t, "edge-1", "", newForwarder(t, b.addr))
		waitOwner(t, a.db, "edge-1", "node-b")

		addLLM(t, a.db, "pushed-llm")
		require.False(t, e.hasLLM("pushed-llm"))
		op := push(t, a, pushes.Request{Scope: pushes.ScopeEdge, EdgeIDs: []string{"edge-1"}})
		st := waitPush(t, a, op, models.PushOperationSucceeded, 30*time.Second)
		assert.Equal(t, 1, st.Commands[0].Attempts)
		assert.Equal(t, "READY", st.Commands[0].Phase)
		assert.True(t, e.hasLLM("pushed-llm"), "the configuration is in the edge's database")

		fromB, err := b.pushes.Status(context.Background(), op)
		require.NoError(t, err)
		assert.Equal(t, models.PushOperationSucceeded, fromB.Operation.Status)
	})
}

// A namespace push reaches every edge, spread over both replicas.
func TestE2E_NamespacePushOverTwoReplicas(t *testing.T) {
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		a := startReplica(t, sdb, "node-a")
		b := startReplica(t, sdb, "node-b")
		edges := []*edge{
			startEdge(t, "edge-a1", "", newForwarder(t, a.addr)),
			startEdge(t, "edge-b1", "", newForwarder(t, b.addr)),
			startEdge(t, "edge-b2", "", newForwarder(t, b.addr)),
		}
		waitOwner(t, a.db, "edge-a1", "node-a")
		waitOwner(t, a.db, "edge-b1", "node-b")
		waitOwner(t, a.db, "edge-b2", "node-b")

		addLLM(t, b.db, "ns-llm")
		op := push(t, b, pushes.Request{Scope: pushes.ScopeNamespace, Namespace: "default"})
		st := waitPush(t, a, op, models.PushOperationSucceeded, 30*time.Second)
		assert.Len(t, st.Commands, 3)
		for _, e := range edges {
			assert.True(t, e.hasLLM("ns-llm"), e.id)
		}
	})
}

// The edge's connection drops while it is reloading, before it answers;
// it reconnects to the other replica, which delivers the push again. The
// push ends succeeded, and the configuration is on the edge.
func TestE2E_ConnectionLostMidReload(t *testing.T) {
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		a := startReplica(t, sdb, "node-a")
		b := startReplica(t, sdb, "node-b")
		fwd := newForwarder(t, a.addr)
		e := startEdge(t, "edge-1", "", fwd)
		waitOwner(t, a.db, "edge-1", "node-a")

		addLLM(t, a.db, "mid-reload-llm")
		op := push(t, b, pushes.Request{Scope: pushes.ScopeEdge, EdgeIDs: []string{"edge-1"}})
		// The edge acknowledged and is pulling (the handler waits two
		// seconds for the snapshot before it answers).
		require.Eventually(t, func() bool {
			st, err := a.pushes.Status(context.Background(), op)
			return err == nil && len(st.Commands) == 1 && st.Commands[0].Phase == "PULL_STARTED"
		}, 20*time.Second, 20*time.Millisecond)

		fwd.route(b.addr)
		fwd.cut()

		st := waitPush(t, a, op, models.PushOperationSucceeded, 40*time.Second)
		cmd := st.Commands[0]
		assert.Equal(t, 2, cmd.Attempts, "history: %+v", cmd.History)
		assert.Equal(t, "node-b", cmd.History[len(cmd.History)-1].Node)
		assert.True(t, e.hasLLM("mid-reload-llm"))
		waitOwner(t, a.db, "edge-1", "node-b")
	})
}

// The replica holding the edge's stream crashes; the push, issued on the
// surviving replica while the edge is reconnecting, is delivered when the
// edge arrives there.
func TestE2E_ReplicaCrashWhileEdgeReconnects(t *testing.T) {
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		a := startReplica(t, sdb, "node-a")
		b := startReplica(t, sdb, "node-b")
		fwd := newForwarder(t, a.addr)
		e := startEdge(t, "edge-1", "", fwd)
		waitOwner(t, b.db, "edge-1", "node-a")

		fwd.setDown()
		a.crash()
		fwd.cut()

		addLLM(t, b.db, "after-crash-llm")
		res, err := b.pushes.Push(context.Background(), pushes.Request{Scope: pushes.ScopeEdge, EdgeIDs: []string{"edge-1"}, InitiatedBy: "admin"})
		require.NoError(t, err)
		op := res.Operation.OperationID

		time.Sleep(500 * time.Millisecond)
		st, err := b.pushes.Status(context.Background(), op)
		require.NoError(t, err)
		assert.Equal(t, models.PushCommandPending, st.Commands[0].Status, "waiting for the edge, not failed")

		fwd.route(b.addr)
		st = waitPush(t, b, op, models.PushOperationSucceeded, 40*time.Second)
		assert.Equal(t, 1, st.Commands[0].Attempts)
		assert.True(t, e.hasLLM("after-crash-llm"))
	})
}

// A replica is stopped cleanly (a rolling restart) while the edge is
// reloading: its streams close, the push is requeued, and the edge's new
// replica finishes it.
func TestE2E_RollingRestartMidReload(t *testing.T) {
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		a := startReplica(t, sdb, "node-a")
		b := startReplica(t, sdb, "node-b")
		fwd := newForwarder(t, a.addr)
		e := startEdge(t, "edge-1", "", fwd)
		waitOwner(t, b.db, "edge-1", "node-a")

		addLLM(t, b.db, "rolling-llm")
		op := push(t, b, pushes.Request{Scope: pushes.ScopeEdge, EdgeIDs: []string{"edge-1"}})
		require.Eventually(t, func() bool {
			st, err := b.pushes.Status(context.Background(), op)
			return err == nil && len(st.Commands) == 1 && st.Commands[0].Status == models.PushCommandSent
		}, 20*time.Second, 20*time.Millisecond)

		fwd.route(b.addr)
		a.stop()
		fwd.cut()

		waitPush(t, b, op, models.PushOperationSucceeded, 40*time.Second)
		assert.True(t, e.hasLLM("rolling-llm"))
	})
}

// An edge that is down when the push is made gets it when it starts, from
// whichever replica it reaches; one that never comes back expires with the
// reason, and the operation reports the mix.
func TestE2E_OfflineEdges(t *testing.T) {
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		a := startReplica(t, sdb, "node-a")
		b := startReplica(t, sdb, "node-b")
		fwdLate := newForwarder(t, a.addr)
		late := startEdge(t, "edge-late", "", fwdLate)
		gone := startEdge(t, "edge-gone", "", newForwarder(t, a.addr))
		waitOwner(t, a.db, "edge-late", "node-a")
		waitOwner(t, a.db, "edge-gone", "node-a")
		require.NoError(t, late.client.Stop())
		require.NoError(t, gone.client.Stop())
		require.Eventually(t, func() bool {
			_, s1 := streamOwner(t, a.db, "edge-late")
			_, s2 := streamOwner(t, a.db, "edge-gone")
			return s1 == models.EdgeStatusDisconnected && s2 == models.EdgeStatusDisconnected
		}, 20*time.Second, 25*time.Millisecond)

		addLLM(t, a.db, "offline-llm")
		res, err := a.pushes.Push(context.Background(), pushes.Request{Scope: pushes.ScopeNamespace, Namespace: "default", InitiatedBy: "admin"})
		require.NoError(t, err)
		require.Len(t, res.Targets, 2)
		require.NotEmpty(t, res.Warnings)
		op := res.Operation.OperationID

		// edge-late restarts, and reaches B this time.
		again := startEdge(t, "edge-late", "", newForwarder(t, b.addr))
		require.Eventually(t, func() bool {
			st, err := a.pushes.Status(context.Background(), op)
			if err != nil {
				return false
			}
			for _, c := range st.Commands {
				if c.EdgeID == "edge-late" {
					return c.Status == models.PushCommandSucceeded
				}
			}
			return false
		}, 40*time.Second, 50*time.Millisecond)
		assert.True(t, again.hasLLM("offline-llm"))

		// edge-gone never returns: at the deadline it expires.
		require.NoError(t, a.db.Model(&models.EdgePushCommand{}).Where("operation_id = ? AND edge_id = ?", op, "edge-gone").
			Update("deadline_at", time.Now().UTC().Add(-time.Second)).Error)
		st := waitPush(t, a, op, models.PushOperationPartiallyFailed, 20*time.Second)
		for _, c := range st.Commands {
			if c.EdgeID == "edge-gone" {
				assert.Equal(t, models.PushCommandExpired, c.Status)
				assert.Contains(t, c.Message, "not connected to any control-plane replica")
			}
		}
	})
}

// An edge that restarts while a push waits for it gets the push as soon as
// it connects, although, like cmd/microgateway, it sets its reload handler
// only a moment later: the edge holds the push until the handler is set
// (and control waits for the edge's first heartbeat). Before, the edge
// dropped it and the push waited for control's one-minute answer timeout,
// then used a second attempt (M3).
func TestE2E_PushToEdgeStillStartingUp(t *testing.T) {
	forEachStudioDB(t, func(t *testing.T, sdb studioDB) {
		a := startReplica(t, sdb, "node-a")
		first := startEdge(t, "edge-1", "", newForwarder(t, a.addr))
		waitOwner(t, a.db, "edge-1", "node-a")
		require.NoError(t, first.client.Stop())
		require.Eventually(t, func() bool {
			_, s := streamOwner(t, a.db, "edge-1")
			return s == models.EdgeStatusDisconnected
		}, 20*time.Second, 25*time.Millisecond)

		addLLM(t, a.db, "startup-llm")
		op := push(t, a, pushes.Request{Scope: pushes.ScopeEdge, EdgeIDs: []string{"edge-1"}})

		start := time.Now()
		again := startEdgeWith(t, "edge-1", "", newForwarder(t, a.addr), edgeOptions{handlerAfter: time.Second})
		st := waitPush(t, a, op, models.PushOperationSucceeded, 20*time.Second)
		t.Logf("push settled %v after the edge started", time.Since(start).Round(time.Millisecond))
		assert.Equal(t, 1, st.Commands[0].Attempts, "history: %+v", st.Commands[0].History)
		assert.True(t, again.hasLLM("startup-llm"))
	})
}
