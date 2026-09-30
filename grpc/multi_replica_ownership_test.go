package grpc

import (
	"context"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	grpclib "google.golang.org/grpc"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// fakeEdgeStream is the server side of one edge's SubscribeToChanges stream:
// the test pushes EdgeMessages in and reads the ControlMessages sent back.
type fakeEdgeStream struct {
	grpclib.ServerStream
	ctx    context.Context
	cancel context.CancelFunc
	in     chan *pb.EdgeMessage

	mu  sync.Mutex
	out []*pb.ControlMessage
}

func newFakeEdgeStream() *fakeEdgeStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &fakeEdgeStream{ctx: ctx, cancel: cancel, in: make(chan *pb.EdgeMessage, 16)}
}

func (f *fakeEdgeStream) Context() context.Context { return f.ctx }

func (f *fakeEdgeStream) Recv() (*pb.EdgeMessage, error) {
	select {
	case m := <-f.in:
		return m, nil
	case <-f.ctx.Done():
		return nil, io.EOF
	}
}

func (f *fakeEdgeStream) Send(m *pb.ControlMessage) error {
	if f.ctx.Err() != nil {
		return io.EOF
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.out = append(f.out, m)
	return nil
}

func (f *fakeEdgeStream) sent() []*pb.ControlMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*pb.ControlMessage(nil), f.out...)
}

// replicaServer is one Studio replica's control server on a shared database.
func replicaServer(t *testing.T, db *gorm.DB, nodeID string) *ControlServer {
	t.Helper()
	os.Setenv("MICROGATEWAY_ENCRYPTION_KEY", testEncryptionKey)
	t.Cleanup(func() { os.Unsetenv("MICROGATEWAY_ENCRYPTION_KEY") })
	s, err := NewControlServer(&Config{AuthToken: testAuthToken, MaxConcurrentStreams: 100, NodeID: nodeID}, db)
	require.NoError(t, err)
	// The fake edges here send no heartbeats: their streams take pushes at
	// once (TestControlServer_PushWaitsForReadyStream covers the wait).
	s.pushReadyGrace = 0
	t.Cleanup(s.Stop)
	return s
}

// connect opens a stream for edgeID on server and waits until the server
// has registered it. The returned stop closes the stream (as a dropped
// connection would) and waits for the server's handler to finish.
func connect(t *testing.T, server *ControlServer, edgeID, namespace string) (*fakeEdgeStream, func()) {
	t.Helper()
	stream := newFakeEdgeStream()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.SubscribeToChanges(stream)
	}()
	stream.in <- &pb.EdgeMessage{Message: &pb.EdgeMessage_Registration{Registration: &pb.EdgeRegistrationRequest{EdgeId: edgeID, EdgeNamespace: namespace, Version: "test"}}}
	require.Eventually(t, func() bool {
		for _, m := range stream.sent() {
			if m.GetRegistrationResponse() != nil {
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond, "stream registration")
	return stream, func() {
		stream.cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("stream handler did not return")
		}
	}
}

func heartbeat(stream *fakeEdgeStream, edgeID string) {
	stream.in <- &pb.EdgeMessage{Message: &pb.EdgeMessage_Heartbeat{Heartbeat: &pb.HeartbeatRequest{EdgeId: edgeID}}}
}

func streamEdgeRow(t *testing.T, db *gorm.DB, edgeID string) models.EdgeInstance {
	t.Helper()
	var e models.EdgeInstance
	require.NoError(t, db.Where("edge_id = ?", edgeID).First(&e).Error)
	return e
}

func registerEdge(t *testing.T, server *ControlServer, edgeID, namespace string) {
	t.Helper()
	_, err := server.RegisterEdge(context.Background(), &pb.EdgeRegistrationRequest{EdgeId: edgeID, EdgeNamespace: namespace, Version: "test"})
	require.NoError(t, err)
}

// An edge that moves from replica A to replica B stays connected, owned by
// B, whatever A does afterwards with its dead stream.
func TestEdgeMovingBetweenReplicasKeepsItsNewOwner(t *testing.T) {
	db := setupTestDB(t)
	a := replicaServer(t, db, "node-a")
	b := replicaServer(t, db, "node-b")

	registerEdge(t, a, "edge-1", "default")
	_, closeOnA := connect(t, a, "edge-1", "default")
	row := streamEdgeRow(t, db, "edge-1")
	assert.Equal(t, "node-a", row.OwnerNodeID)
	assert.Equal(t, models.EdgeStatusConnected, row.Status)
	sessionA := row.StreamSessionID
	require.NotEmpty(t, sessionA)

	// The edge's connection to A breaks; it registers and streams to B
	// before A notices.
	registerEdge(t, b, "edge-1", "default")
	streamB, closeOnB := connect(t, b, "edge-1", "default")
	row = streamEdgeRow(t, db, "edge-1")
	assert.Equal(t, "node-b", row.OwnerNodeID)
	assert.NotEqual(t, sessionA, row.StreamSessionID)

	// Now A's stream ends, and A's stale sweep runs.
	closeOnA()
	a.cleanupStaleConnections()
	row = streamEdgeRow(t, db, "edge-1")
	assert.Equal(t, "node-b", row.OwnerNodeID, "A must not overwrite B's ownership")
	assert.Equal(t, models.EdgeStatusConnected, row.Status)

	// A unary re-registration handled by A does not clear B's ownership.
	registerEdge(t, a, "edge-1", "default")
	row = streamEdgeRow(t, db, "edge-1")
	assert.Equal(t, "node-b", row.OwnerNodeID)
	assert.NotEmpty(t, row.StreamSessionID)

	// Heartbeats on B keep it (and restore it if the row was cleared).
	require.NoError(t, db.Model(&models.EdgeInstance{}).Where("edge_id = ?", "edge-1").
		Updates(map[string]interface{}{"owner_node_id": "", "stream_session_id": "", "status": models.EdgeStatusDisconnected}).Error)
	heartbeat(streamB, "edge-1")
	require.Eventually(t, func() bool {
		r := streamEdgeRow(t, db, "edge-1")
		return r.OwnerNodeID == "node-b" && r.Status == models.EdgeStatusConnected
	}, 5*time.Second, 20*time.Millisecond, "heartbeat restores ownership")

	// B's stream ending is the real disconnect.
	closeOnB()
	row = streamEdgeRow(t, db, "edge-1")
	assert.Equal(t, models.EdgeStatusDisconnected, row.Status)
	assert.Empty(t, row.OwnerNodeID)
}

// A configuration change on a replica that holds no edge streams still
// marks the edges on other replicas (and offline ones) pending: namespaces
// come from the database.
func TestConfigChangeOnAnyReplicaMarksAllNamespacesPending(t *testing.T) {
	db := setupTestDB(t)
	a := replicaServer(t, db, "node-a")
	b := replicaServer(t, db, "node-b")

	// Edges in two namespaces, streaming to B; one more, offline.
	for _, e := range []struct{ id, ns string }{{"edge-ns1", "ns1"}, {"edge-default", "default"}, {"edge-offline", "ns2"}} {
		require.NoError(t, db.Create(&models.EdgeInstance{EdgeID: e.id, Namespace: e.ns, Status: models.EdgeStatusRegistered, SyncStatus: models.EdgeSyncStatusInSync}).Error)
	}
	_, closeNS1 := connect(t, b, "edge-ns1", "ns1")
	defer closeNS1()
	_, closeDefault := connect(t, b, "edge-default", "default")
	defer closeDefault()
	require.NoError(t, db.Model(&models.EdgeInstance{}).Where("1 = 1").Update("sync_status", models.EdgeSyncStatusInSync).Error)

	// The change happens on A, which has no edges at all.
	createTestLLMs(db, "")
	a.onConfigurationChanged(topicLLMCreated, eventbridge.Event{ID: "change-on-a"})

	for _, id := range []string{"edge-ns1", "edge-default"} {
		assert.Equal(t, models.EdgeSyncStatusPending, streamEdgeRow(t, db, id).SyncStatus, id)
	}
	for _, ns := range []string{"ns1", "ns2", "default"} {
		var st models.NamespaceSyncStatus
		require.NoError(t, st.GetByNamespace(db, ns), ns)
		assert.NotEmpty(t, st.ExpectedChecksum, "expected checksum recorded for %s", ns)
	}
	assert.ElementsMatch(t, []string{"default", "ns1", "ns2"}, a.namespacesToRecompute())
}
