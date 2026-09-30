package grpc

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/services/pushes"
)

// recordingPushes is a PushDelivery that records what the server told it.
type recordingPushes struct {
	mu        sync.Mutex
	opened    []string
	closed    [][2]string
	responses []*pb.ConfigurationReloadResponse
}

func (r *recordingPushes) StreamOpened(edgeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.opened = append(r.opened, edgeID)
}

func (r *recordingPushes) StreamClosed(edgeID, session string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = append(r.closed, [2]string{edgeID, session})
}

func (r *recordingPushes) HandleReloadResponse(resp *pb.ConfigurationReloadResponse) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.responses = append(r.responses, resp)
}

func (r *recordingPushes) snapshot() (opened []string, closed [][2]string, responses []*pb.ConfigurationReloadResponse) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.opened...), append([][2]string(nil), r.closed...), append([]*pb.ConfigurationReloadResponse(nil), r.responses...)
}

func reloadRequests(stream *fakeEdgeStream) []*pb.ConfigurationReloadRequest {
	var out []*pb.ConfigurationReloadRequest
	for _, m := range stream.sent() {
		if r := m.GetReloadRequest(); r != nil {
			out = append(out, r)
		}
	}
	return out
}

// A push goes out only on the stream (session) it was claimed for; the
// coordinator hears about the stream opening and closing, with the session.
func TestControlServer_PushStreamLifecycle(t *testing.T) {
	db := setupTestDB(t)
	server := replicaServer(t, db, "node-a")
	rec := &recordingPushes{}
	server.SetPushDelivery(rec)
	registerEdge(t, server, "edge-1", "default")

	stream, stop := connect(t, server, "edge-1", "default")
	opened, _, _ := rec.snapshot()
	assert.Equal(t, []string{"edge-1"}, opened)

	local := server.LocalStreams()
	require.Contains(t, local, "edge-1")
	session := local["edge-1"]
	assert.Equal(t, session, streamEdgeRow(t, db, "edge-1").StreamSessionID)

	req := &pb.ConfigurationReloadRequest{OperationId: "push-1", TargetEdges: []string{"edge-1"}}
	require.NoError(t, server.SendReload("edge-1", session, req))
	require.Len(t, reloadRequests(stream), 1)
	assert.Equal(t, "push-1", reloadRequests(stream)[0].OperationId)

	err := server.SendReload("edge-1", "another-session", req)
	require.ErrorIs(t, err, pushes.ErrNoStream, "nothing sent: no attempt used")
	assert.Contains(t, err.Error(), "reconnected")
	assert.Len(t, reloadRequests(stream), 1, "not sent on a stream it was not claimed for")

	err = server.SendReload("edge-2", session, req)
	require.ErrorIs(t, err, pushes.ErrNoStream)

	stop()
	_, closed, _ := rec.snapshot()
	assert.Equal(t, [][2]string{{"edge-1", session}}, closed)
	assert.NotContains(t, server.LocalStreams(), "edge-1")
	assert.ErrorIs(t, server.SendReload("edge-1", session, req), pushes.ErrNoStream)
}

// An edge that reconnects on the same replica: the old stream's close
// reports the old session, so only pushes sent on it are retried.
func TestControlServer_PushReconnectSameReplica(t *testing.T) {
	db := setupTestDB(t)
	server := replicaServer(t, db, "node-a")
	rec := &recordingPushes{}
	server.SetPushDelivery(rec)
	registerEdge(t, server, "edge-1", "default")

	_, stopOld := connect(t, server, "edge-1", "default")
	oldSession := server.LocalStreams()["edge-1"]
	newStream, stopNew := connect(t, server, "edge-1", "default")
	defer stopNew()
	newSession := server.LocalStreams()["edge-1"]
	require.NotEqual(t, oldSession, newSession)

	stopOld()
	_, closed, _ := rec.snapshot()
	assert.Equal(t, [][2]string{{"edge-1", oldSession}}, closed)
	assert.Equal(t, newSession, server.LocalStreams()["edge-1"], "the new stream is still this edge's")
	require.NoError(t, server.SendReload("edge-1", newSession, &pb.ConfigurationReloadRequest{OperationId: "push-1"}))
	assert.Len(t, reloadRequests(newStream), 1)
}

// Reload reports are attributed to the edge the stream registered as,
// whatever edge the message names; a report before registration is dropped.
func TestControlServer_ReloadResponseRouting(t *testing.T) {
	db := setupTestDB(t)
	server := replicaServer(t, db, "node-a")
	rec := &recordingPushes{}
	server.SetPushDelivery(rec)
	registerEdge(t, server, "edge-1", "default")

	// Before registration.
	early := newFakeEdgeStream()
	done := make(chan struct{})
	go func() { defer close(done); _ = server.SubscribeToChanges(early) }()
	early.in <- &pb.EdgeMessage{Message: &pb.EdgeMessage_ReloadResponse{ReloadResponse: &pb.ConfigurationReloadResponse{OperationId: "push-1", EdgeId: "edge-1", Phase: pb.ReloadPhase_READY}}}

	stream, stop := connect(t, server, "edge-1", "default")
	defer stop()
	stream.in <- &pb.EdgeMessage{Message: &pb.EdgeMessage_ReloadResponse{ReloadResponse: &pb.ConfigurationReloadResponse{OperationId: "push-1", EdgeId: "edge-other", Phase: pb.ReloadPhase_READY, Message: "ok"}}}

	require.Eventually(t, func() bool { _, _, r := rec.snapshot(); return len(r) == 1 }, 5*time.Second, 10*time.Millisecond)
	early.cancel()
	<-done
	_, _, responses := rec.snapshot()
	require.Len(t, responses, 1, "the unregistered stream's report is dropped")
	assert.Equal(t, "edge-1", responses[0].EdgeId, "attributed to the stream's edge")
	assert.Equal(t, "push-1", responses[0].OperationId)
	assert.Equal(t, "ok", responses[0].Message)
}

// The stale-connection sweep reports the streams it drops.
func TestControlServer_StaleSweepReportsClosedStreams(t *testing.T) {
	db := setupTestDB(t)
	server := replicaServer(t, db, "node-a")
	rec := &recordingPushes{}
	server.SetPushDelivery(rec)

	server.edgeMutex.Lock()
	server.edgeConnections["edge-stale"] = &EdgeInstanceConnection{EdgeID: "edge-stale", SessionID: "s-stale", Status: "connected", LastHeartbeat: time.Now().Add(-time.Hour)}
	server.edgeMutex.Unlock()

	server.cleanupStaleConnections()
	_, closed, _ := rec.snapshot()
	assert.Equal(t, [][2]string{{"edge-stale", "s-stale"}}, closed)
}

// overlapStream fails the test if two Sends ever run at once.
type overlapStream struct {
	*fakeEdgeStream
	inSend  atomic.Int32
	overlap atomic.Bool
}

func (o *overlapStream) Send(m *pb.ControlMessage) error {
	if o.inSend.Add(1) > 1 {
		o.overlap.Store(true)
	}
	time.Sleep(100 * time.Microsecond)
	o.inSend.Add(-1)
	return o.fakeEdgeStream.Send(m)
}

// Pushes, heartbeat responses and events share one stream; gRPC allows one
// Send at a time, so the server must serialise them.
func TestControlServer_StreamSendsAreSerialised(t *testing.T) {
	db := setupTestDB(t)
	server := replicaServer(t, db, "node-a")
	registerEdge(t, server, "edge-1", "default")

	stream := &overlapStream{fakeEdgeStream: newFakeEdgeStream()}
	done := make(chan struct{})
	go func() { defer close(done); _ = server.SubscribeToChanges(stream) }()
	stream.in <- &pb.EdgeMessage{Message: &pb.EdgeMessage_Registration{Registration: &pb.EdgeRegistrationRequest{EdgeId: "edge-1", EdgeNamespace: "default"}}}
	require.Eventually(t, func() bool { return server.LocalStreams()["edge-1"] != "" }, 5*time.Second, 10*time.Millisecond)
	session := server.LocalStreams()["edge-1"]

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				require.NoError(t, server.SendReload("edge-1", session, &pb.ConfigurationReloadRequest{OperationId: "push"}))
			}
		}()
	}
	for j := 0; j < 20; j++ {
		heartbeat(stream.fakeEdgeStream, "edge-1")
	}
	wg.Wait()
	require.Eventually(t, func() bool {
		n := 0
		for _, m := range stream.sent() {
			if m.GetHeartbeatResponse() != nil {
				n++
			}
		}
		return n == 20
	}, 10*time.Second, 10*time.Millisecond)
	stream.cancel()
	<-done
	assert.False(t, stream.overlap.Load(), "two Sends overlapped on one stream")
	assert.Len(t, reloadRequests(stream.fakeEdgeStream), 120)
}

// Stopping a replica ends its edge streams itself: waiting for the edges
// to leave would hang the shutdown, and pushes in flight on them must be
// requeued for the replica each edge reconnects to.
func TestControlServer_StopEndsEdgeStreams(t *testing.T) {
	db := setupTestDB(t)
	server := replicaServer(t, db, "node-a")
	rec := &recordingPushes{}
	server.SetPushDelivery(rec)
	registerEdge(t, server, "edge-1", "default")

	stream := newFakeEdgeStream()
	done := make(chan struct{})
	go func() { defer close(done); _ = server.SubscribeToChanges(stream) }()
	stream.in <- &pb.EdgeMessage{Message: &pb.EdgeMessage_Registration{Registration: &pb.EdgeRegistrationRequest{EdgeId: "edge-1", EdgeNamespace: "default"}}}
	require.Eventually(t, func() bool { return server.LocalStreams()["edge-1"] != "" }, 5*time.Second, 10*time.Millisecond)
	session := server.LocalStreams()["edge-1"]

	stopped := make(chan struct{})
	go func() { server.Stop(); close(stopped) }()
	for _, ch := range []chan struct{}{done, stopped} {
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatal("Stop waited for the edge to disconnect")
		}
	}
	_, closed, _ := rec.snapshot()
	assert.Equal(t, [][2]string{{"edge-1", session}}, closed)
	assert.Equal(t, models.EdgeStatusDisconnected, streamEdgeRow(t, db, "edge-1").Status)
	for _, m := range stream.sent() {
		if hb := m.GetHeartbeatResponse(); hb != nil {
			assert.False(t, hb.ShutdownRequested, "the edge client stops for good on ShutdownRequested")
		}
	}
}

// A stream is given pushes only once the edge is ready for them: after its
// first heartbeat (current edges send one as soon as the stream is open),
// which also wakes the coordinator, or, for edges that send no early
// heartbeat (v2.2 and before), after the grace period. Until then an edge
// that is still starting up would drop the push (M3).
func TestControlServer_PushWaitsForReadyStream(t *testing.T) {
	db := setupTestDB(t)
	server := replicaServer(t, db, "node-a")
	server.pushReadyGrace = time.Hour
	rec := &recordingPushes{}
	server.SetPushDelivery(rec)
	registerEdge(t, server, "edge-1", "default")

	stream, stop := connect(t, server, "edge-1", "default")
	defer stop()
	assert.NotContains(t, server.LocalStreams(), "edge-1", "no pushes before the edge's first heartbeat")
	opened, _, _ := rec.snapshot()
	assert.Equal(t, []string{"edge-1"}, opened)

	heartbeat(stream, "edge-1")
	require.Eventually(t, func() bool { return server.LocalStreams()["edge-1"] != "" }, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { o, _, _ := rec.snapshot(); return len(o) == 2 }, 5*time.Second, 10*time.Millisecond,
		"the first heartbeat wakes the coordinator")

	heartbeat(stream, "edge-1")
	require.Eventually(t, func() bool {
		n := 0
		for _, m := range stream.sent() {
			if m.GetHeartbeatResponse() != nil {
				n++
			}
		}
		return n == 2
	}, 5*time.Second, 10*time.Millisecond)
	opened, _, _ = rec.snapshot()
	assert.Len(t, opened, 2, "later heartbeats do not")
}

// An edge that sends no heartbeat early gets pushes after the grace period.
func TestControlServer_PushReadyAfterGrace(t *testing.T) {
	db := setupTestDB(t)
	server := replicaServer(t, db, "node-a")
	server.pushReadyGrace = 200 * time.Millisecond
	registerEdge(t, server, "edge-1", "default")

	start := time.Now()
	_, stop := connect(t, server, "edge-1", "default")
	defer stop()
	assert.NotContains(t, server.LocalStreams(), "edge-1")
	require.Eventually(t, func() bool { return server.LocalStreams()["edge-1"] != "" }, 5*time.Second, 10*time.Millisecond)
	assert.GreaterOrEqual(t, time.Since(start), 200*time.Millisecond)
}
