package grpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// cancelOnHeartbeat ends the stream at the moment a heartbeat is received,
// as an edge that stops right after sending one does: the server is still
// handling the heartbeat while its stream-end cleanup runs.
type cancelOnHeartbeat struct {
	*fakeEdgeStream
}

func (s cancelOnHeartbeat) Recv() (*pb.EdgeMessage, error) {
	m, err := s.fakeEdgeStream.Recv()
	if err == nil && m.GetHeartbeat() != nil {
		s.cancel()
	}
	return m, err
}

// A heartbeat handled while the stream is closing must not bring the edge
// back to "connected" after the close marked it disconnected: the edge is
// gone, and nothing would release it again until the stale sweep. (The
// microgateway cluster E2E saw this as an edge that never showed as
// disconnected after Stop.)
func TestStreamEndAfterAHeartbeatLeavesTheEdgeDisconnected(t *testing.T) {
	db := setupTestDB(t)
	server := replicaServer(t, db, "node-a")
	const edgeID = "edge-race"
	registerEdge(t, server, edgeID, "default")

	for i := 0; i < 25; i++ {
		stream := newFakeEdgeStream()
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = server.SubscribeToChanges(cancelOnHeartbeat{stream})
		}()
		stream.in <- &pb.EdgeMessage{Message: &pb.EdgeMessage_Registration{Registration: &pb.EdgeRegistrationRequest{EdgeId: edgeID, EdgeNamespace: "default", Version: "test"}}}
		require.Eventually(t, func() bool {
			for _, m := range stream.sent() {
				if m.GetRegistrationResponse() != nil {
					return true
				}
			}
			return false
		}, 5*time.Second, 5*time.Millisecond, "stream registration")

		heartbeat(stream, edgeID)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("stream handler did not return")
		}

		row := streamEdgeRow(t, db, edgeID)
		require.Equal(t, models.EdgeStatusDisconnected, row.Status, "iteration %d: the edge came back after its stream closed (owner %q, session %q)", i, row.OwnerNodeID, row.StreamSessionID)
		require.Empty(t, row.StreamSessionID, "iteration %d", i)
	}
}
