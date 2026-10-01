//go:build !enterprise

package studio

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	grpclib "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	grpcmetadata "google.golang.org/grpc/metadata"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/cluster"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/pkg/replicas"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/services/pushes"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

const (
	cpTestAuthToken     = "test-token"
	cpTestEncryptionKey = "0123456789abcdef0123456789abcdef"
	// peerEnv makes TestHelperFullStudioPeer run a full Studio: a Studio and
	// a control plane cannot share a process, so the full Studio runs in a
	// child test binary.
	peerEnv = "STUDIO_TEST_FULL_STUDIO_PEER"
)

// controlModeConfig makes conf serve edges: the settings a full Studio in
// control mode and a headless control plane share.
func controlModeConfig(conf *config.AppConf) {
	conf.GatewayMode = "control"
	conf.MicrogatewayEncryptionKey = cpTestEncryptionKey
	conf.GRPCAuthToken = cpTestAuthToken
	conf.GRPCTLSEnabled = false
	// Several leader budget syncs in a few seconds.
	conf.BudgetSyncInterval = 300 * time.Millisecond
	conf.LogLevel = "warn"
}

func headlessOptions(t *testing.T, db *gorm.DB) ControlPlaneOptions {
	t.Helper()
	conf := newTestOptions(t).Config
	controlModeConfig(conf)
	return ControlPlaneOptions{
		Config:  conf,
		DB:      db,
		Version: "test",
		NodeID:  fmt.Sprintf("headless-%d", time.Now().UnixNano()),
	}
}

func stopControlPlane(t *testing.T, c *ControlPlane) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, c.Stop(ctx))
}

// migratedSchema returns a Postgres schema a full Studio has migrated (and
// stopped on), for the control plane to start on.
func migratedSchema(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := schemaTestDSN(t)
	schema := fmt.Sprintf("studio_headless_%d", time.Now().UnixNano())
	dropSchemaAfter(t, dsn, schema)
	db := openSchema(t, dsn, schema)
	opts := newTestOptions(t)
	opts.DB = db
	s := newWithin(t, opts, 2*time.Minute)
	stopStudio(t, s)
	return db
}

// The control plane never migrates: it starts only on a schema a full
// Studio has recorded as one this build reads.
func TestControlPlaneChecksTheSchema_Postgres(t *testing.T) {
	dsn := schemaTestDSN(t)
	schema := fmt.Sprintf("studio_headless_%d", time.Now().UnixNano())
	dropSchemaAfter(t, dsn, schema)
	_, err := NewControlPlane(headlessOptions(t, openSchema(t, dsn, schema)))
	assert.ErrorIs(t, err, ErrSchemaMissing, "nothing has migrated it")
	assert.False(t, running.Load())

	db := migratedSchema(t)
	require.NoError(t, db.Model(&models.StudioSchema{}).Where("1 = 1").Update("version", models.SchemaVersion-1).Error)
	_, err = NewControlPlane(headlessOptions(t, db))
	assert.ErrorIs(t, err, ErrSchemaTooOld)

	require.NoError(t, db.Model(&models.StudioSchema{}).Where("1 = 1").
		Updates(map[string]interface{}{"version": models.SchemaVersion + 1, "min_reader_version": models.SchemaVersion + 1}).Error)
	_, err = NewControlPlane(headlessOptions(t, db))
	assert.ErrorIs(t, err, ErrSchemaTooNew)

	// Newer, but still readable by this build.
	require.NoError(t, db.Model(&models.StudioSchema{}).Where("1 = 1").
		Updates(map[string]interface{}{"version": models.SchemaVersion + 1, "min_reader_version": models.SchemaVersion}).Error)
	c, err := NewControlPlane(headlessOptions(t, db))
	require.NoError(t, err)
	stopControlPlane(t, c)
}

// One instance per process, a Studio or a control plane; a stopped (or
// refused) one frees the process for the next.
func TestControlPlaneSharesTheOneInstanceGuard_Postgres(t *testing.T) {
	db := migratedSchema(t)

	s, err := New(newTestOptions(t))
	require.NoError(t, err)
	_, err = NewControlPlane(headlessOptions(t, db))
	assert.ErrorIs(t, err, ErrAlreadyRunning)
	stopStudio(t, s)

	c, err := NewControlPlane(headlessOptions(t, db))
	require.NoError(t, err)
	_, err = New(newTestOptions(t))
	assert.ErrorIs(t, err, ErrAlreadyRunning)
	stopControlPlane(t, c)
	require.NoError(t, c.Stop(context.Background()), "Stop is idempotent")

	s, err = New(newTestOptions(t))
	require.NoError(t, err)
	stopStudio(t, s)
}

// A headless control plane next to a full Studio on one database: an edge
// streaming to it gets its configuration, pushes made on the full Studio
// and the leader's budget sync reach it through the cluster, it never takes
// the leader lease (not even with the full Studio gone), it leaves the
// registry on Stop, and it never changes the schema.
func TestControlPlaneNextToAFullStudio_Postgres(t *testing.T) {
	dsn := schemaTestDSN(t)
	schema := fmt.Sprintf("studio_headless_%d", time.Now().UnixNano())
	dropSchemaAfter(t, dsn, schema)
	db := openSchema(t, dsn, schema)
	// Before any goroutine uses the handle: registering a callback races
	// with queries in flight.
	ddl := watchDDL(t, db)

	peer := startFullStudioPeer(t, dsn, schema)

	// Spend for an App, so the leader's budget sync has something to send.
	app := models.App{Name: "headless budget app"}
	require.NoError(t, app.Create(db))
	require.NoError(t, db.Create(&models.LLMChatRecord{Name: "m", AppID: app.ID, Cost: 12345, TimeStamp: time.Now()}).Error)

	opts := headlessOptions(t, db)
	c, err := NewControlPlane(opts)
	require.NoError(t, err)
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			stopControlPlane(t, c)
		}
	})
	assert.Equal(t, opts.NodeID, c.NodeID())
	assert.False(t, replicas.IsLeader(), "a headless replica never leads")

	lis, err := listenLocal()
	require.NoError(t, err)
	serveDone := make(chan error, 1)
	go func() { serveDone <- c.Serve(lis) }()

	conn, err := grpclib.NewClient(lis.Addr().String(), grpclib.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	client := pb.NewConfigurationSyncServiceClient(conn)

	const edgeID = "edge-on-headless"
	reg, err := client.RegisterEdge(edgeContext(context.Background()), &pb.EdgeRegistrationRequest{EdgeId: edgeID, Version: "test"})
	require.NoError(t, err)
	require.True(t, reg.Success, reg.Message)
	require.NotNil(t, reg.InitialConfig, "the edge gets its configuration snapshot")
	assert.NotEmpty(t, reg.InitialConfig.Checksum)

	edge := openEdgeStream(t, client, edgeID)
	require.Eventually(t, func() bool {
		var e models.EdgeInstance
		return db.Where("edge_id = ?", edgeID).Take(&e).Error == nil && e.OwnerNodeID == c.NodeID()
	}, 10*time.Second, 50*time.Millisecond, "the headless replica holds the edge's stream")

	// The full Studio leads; its budget sync reaches this replica's edge
	// through the relay.
	require.Eventually(t, func() bool {
		holder, _, err := cluster.Holder(context.Background(), db, cluster.LeaderLease)
		return err == nil && holder == peer.node
	}, 30*time.Second, 100*time.Millisecond, "the full Studio holds the leader lease")
	edge.waitFor(t, "the leader's budget sync", 0, isBudgetSync, 30*time.Second)

	// A push made on the full Studio is delivered by the replica holding the
	// edge's stream: this one.
	peer.push(t, edgeID)
	edge.waitFor(t, "the full Studio's push", 0, func(m *pb.ControlMessage) bool { return m.GetReloadRequest() != nil }, 30*time.Second)

	// The full Studio goes away. Nothing takes over the leader's work here:
	// no lease, no budget sync from this replica's own budget service,
	// although the App's spend would give it something to send.
	peer.stop(t)
	mark := edge.count()
	time.Sleep(3 * time.Second) // ten of its budget sync intervals, a renewal period
	assert.False(t, replicas.IsLeader())
	holder, _, err := cluster.Holder(context.Background(), db, cluster.LeaderLease)
	require.NoError(t, err)
	assert.NotEqual(t, c.NodeID(), holder, "the headless replica never takes the leader lease")
	assert.False(t, edge.sawSince(mark, isBudgetSync), "the headless replica's budget sync never runs the leader's part")

	stopControlPlane(t, c)
	stopped = true
	select {
	case <-serveDone:
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not return after Stop")
	}
	var nodes int64
	require.NoError(t, db.Model(&models.ClusterNode{}).Where("node_id = ?", c.NodeID()).Count(&nodes).Error)
	assert.Zero(t, nodes, "Stop removes this replica from the registry")

	assert.Empty(t, ddl.statements(), "the headless control plane changed the schema")
}

// The DDL recorder the tests above rely on records what migrations run.
func TestWatchDDLRecordsMigrations_Postgres(t *testing.T) {
	dsn := schemaTestDSN(t)
	schema := fmt.Sprintf("studio_ddl_probe_%d", time.Now().UnixNano())
	dropSchemaAfter(t, dsn, schema)
	db := openSchema(t, dsn, schema)
	ddl := watchDDL(t, db)
	require.NoError(t, db.AutoMigrate(&models.StudioSchema{}))
	require.NotEmpty(t, ddl.statements())
	assert.Regexp(t, `(?i)^CREATE TABLE`, ddl.statements()[0])
}

// A schema that is not exactly what this build would migrate to (a newer
// full Studio's, which this build can still read) is left as it is: here
// the newer schema no longer has an index this build's analytics models
// declare. Migrating would put it back.
func TestControlPlaneLeavesANewerSchemaAlone_Postgres(t *testing.T) {
	db := migratedSchema(t)
	require.NoError(t, db.Exec(`DROP INDEX idx_proxy_logs_code`).Error)
	require.NoError(t, db.Model(&models.StudioSchema{}).Where("1 = 1").Update("version", models.SchemaVersion+1).Error)
	ddl := watchDDL(t, db)

	c, err := NewControlPlane(headlessOptions(t, db))
	require.NoError(t, err)
	stopControlPlane(t, c)

	assert.Empty(t, ddl.statements(), "the headless control plane changed the schema")
	assert.False(t, db.Migrator().HasIndex(&models.ProxyLog{}, "idx_proxy_logs_code"))
}

func isBudgetSync(m *pb.ControlMessage) bool {
	return m.GetEvent() != nil && m.GetEvent().Topic == eventbridge.BudgetSyncTopic
}

func edgeContext(ctx context.Context) context.Context {
	return grpcmetadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+cpTestAuthToken)
}

// edgeStream is a test edge's SubscribeToChanges stream: it registers,
// heartbeats once (so it takes pushes at once) and records what it receives.
type edgeStream struct {
	mu  sync.Mutex
	got []*pb.ControlMessage
}

func openEdgeStream(t *testing.T, client pb.ConfigurationSyncServiceClient, edgeID string) *edgeStream {
	t.Helper()
	ctx, cancel := context.WithCancel(edgeContext(context.Background()))
	t.Cleanup(cancel)
	st, err := client.SubscribeToChanges(ctx)
	require.NoError(t, err)
	es := &edgeStream{}
	go func() {
		for {
			m, err := st.Recv()
			if err != nil {
				return
			}
			es.mu.Lock()
			es.got = append(es.got, m)
			es.mu.Unlock()
		}
	}()
	require.NoError(t, st.Send(&pb.EdgeMessage{Message: &pb.EdgeMessage_Registration{Registration: &pb.EdgeRegistrationRequest{EdgeId: edgeID, Version: "test"}}}))
	es.waitFor(t, "stream registration", 0, func(m *pb.ControlMessage) bool { return m.GetRegistrationResponse() != nil }, 10*time.Second)
	require.NoError(t, st.Send(&pb.EdgeMessage{Message: &pb.EdgeMessage_Heartbeat{Heartbeat: &pb.HeartbeatRequest{EdgeId: edgeID}}}))
	return es
}

func (es *edgeStream) count() int {
	es.mu.Lock()
	defer es.mu.Unlock()
	return len(es.got)
}

func (es *edgeStream) sawSince(from int, match func(*pb.ControlMessage) bool) bool {
	es.mu.Lock()
	defer es.mu.Unlock()
	for _, m := range es.got[from:] {
		if match(m) {
			return true
		}
	}
	return false
}

func (es *edgeStream) waitFor(t *testing.T, what string, from int, match func(*pb.ControlMessage) bool, d time.Duration) {
	t.Helper()
	require.Eventually(t, func() bool { return es.sawSince(from, match) }, d, 20*time.Millisecond, what)
}

// ddlRecorder records the DDL statements run through a gorm handle.
type ddlRecorder struct {
	mu    sync.Mutex
	stmts []string
}

var ddlStatement = regexp.MustCompile(`(?i)^\s*(CREATE|ALTER|DROP|TRUNCATE|COMMENT|RENAME)\b`)

func watchDDL(t *testing.T, db *gorm.DB) *ddlRecorder {
	t.Helper()
	r := &ddlRecorder{}
	require.NoError(t, db.Callback().Raw().After("gorm:raw").Register("test:record_ddl", func(tx *gorm.DB) {
		if sql := tx.Statement.SQL.String(); ddlStatement.MatchString(sql) {
			r.mu.Lock()
			r.stmts = append(r.stmts, sql)
			r.mu.Unlock()
		}
	}))
	return r
}

func (r *ddlRecorder) statements() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.stmts...)
}

// fullStudioPeer is a full Studio (New, control mode) running in a child
// process on the same database: it migrates, leads, and makes pushes on
// request.
type fullStudioPeer struct {
	node   string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan string
	stderr *bytes.Buffer
	once   sync.Once
	exited chan struct{}
}

func startFullStudioPeer(t *testing.T, dsn, schema string) *fullStudioPeer {
	t.Helper()
	p := &fullStudioPeer{
		node:   fmt.Sprintf("full-studio-%d", time.Now().UnixNano()),
		lines:  make(chan string, 64),
		stderr: &bytes.Buffer{},
		exited: make(chan struct{}),
	}
	p.cmd = exec.Command(os.Args[0], "-test.run=^TestHelperFullStudioPeer$", "-test.count=1")
	p.cmd.Env = append(os.Environ(), peerEnv+"=1", "PEER_DSN="+dsn, "PEER_SCHEMA="+schema, "PEER_NODE="+p.node)
	p.cmd.Stderr = &lockedWriter{w: p.stderr}
	var err error
	p.stdin, err = p.cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := p.cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, p.cmd.Start())
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "PEER ") {
				p.lines <- sc.Text()
			}
		}
		close(p.lines)
	}()
	go func() {
		_ = p.cmd.Wait()
		close(p.exited)
	}()
	t.Cleanup(func() {
		p.close()
		select {
		case <-p.exited:
		case <-time.After(30 * time.Second):
			_ = p.cmd.Process.Kill()
			<-p.exited
		}
		if t.Failed() {
			t.Logf("full Studio peer stderr:\n%s", p.stderr.String())
		}
	})
	p.expect(t, "PEER READY", 3*time.Minute)
	return p
}

func (p *fullStudioPeer) close() { p.once.Do(func() { p.stdin.Close() }) }

func (p *fullStudioPeer) expect(t *testing.T, want string, d time.Duration) {
	t.Helper()
	timeout := time.After(d)
	for {
		select {
		case line, ok := <-p.lines:
			if !ok {
				t.Fatalf("full Studio peer exited before %q", want)
			}
			if strings.HasPrefix(line, "PEER ERR") {
				t.Fatalf("full Studio peer: %s", line)
			}
			if line == want {
				return
			}
		case <-timeout:
			t.Fatalf("full Studio peer: no %q within %s", want, d)
		}
	}
}

func (p *fullStudioPeer) push(t *testing.T, edgeID string) {
	t.Helper()
	_, err := fmt.Fprintf(p.stdin, "push %s\n", edgeID)
	require.NoError(t, err)
	p.expect(t, "PEER PUSHED", 30*time.Second)
}

// stop stops the full Studio (it hands the lease back) and waits for its
// process to end.
func (p *fullStudioPeer) stop(t *testing.T) {
	t.Helper()
	p.close()
	p.expect(t, "PEER STOPPED", time.Minute)
	select {
	case <-p.exited:
	case <-time.After(30 * time.Second):
		t.Fatal("full Studio peer did not exit")
	}
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(b)
}

// TestHelperFullStudioPeer is not a test of its own: startFullStudioPeer runs
// it in a child process as the full Studio next to a control plane. It reads
// commands on stdin ("push <edge>") until stdin closes, then stops.
func TestHelperFullStudioPeer(t *testing.T) {
	if os.Getenv(peerEnv) == "" {
		t.Skip("runs only as the full Studio peer of TestControlPlaneNextToAFullStudio_Postgres")
	}
	opts := newTestOptions(t)
	opts.DB = openSchema(t, os.Getenv("PEER_DSN"), os.Getenv("PEER_SCHEMA"))
	opts.NodeID = os.Getenv("PEER_NODE")
	controlModeConfig(opts.Config)
	s, err := New(opts)
	if err != nil {
		fmt.Println("PEER ERR", err)
		t.Fatal(err)
	}
	fmt.Println("PEER READY")

	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && f[0] == "push" {
			_, err := s.pushes.Push(context.Background(), pushes.Request{Scope: pushes.ScopeEdge, EdgeIDs: []string{f[1]}, InitiatedBy: "test"})
			if err != nil {
				fmt.Println("PEER ERR", err)
				continue
			}
			fmt.Println("PEER PUSHED")
		}
	}
	stopStudio(t, s)
	fmt.Println("PEER STOPPED")
}
