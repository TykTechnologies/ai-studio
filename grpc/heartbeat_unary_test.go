package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// With several replicas an edge's stream is held by one of them, and a unary
// call can reach any. The unary heartbeat answers from the database, so it
// no longer fails with NotFound on a replica that does not hold the stream.
func TestSendHeartbeat_EdgeStreamingToAnotherReplica(t *testing.T) {
	server, db := setupTestServer(t, &Config{AuthToken: testAuthToken, NodeID: "replica-b"})

	before := time.Now().Add(-time.Hour)
	edge := models.EdgeInstance{
		EdgeID:        "edge-elsewhere",
		Namespace:     "test",
		Status:        models.EdgeStatusConnected,
		OwnerNodeID:   "replica-a",
		LastHeartbeat: &before,
	}
	require.NoError(t, db.Create(&edge).Error)

	resp, err := server.SendHeartbeat(context.Background(), &pb.HeartbeatRequest{EdgeId: "edge-elsewhere"})
	require.NoError(t, err)
	assert.True(t, resp.Acknowledged)

	var got models.EdgeInstance
	require.NoError(t, got.GetByEdgeID(db, "edge-elsewhere"))
	require.NotNil(t, got.LastHeartbeat)
	assert.True(t, got.LastHeartbeat.After(before.Add(time.Minute)), "the heartbeat is recorded")
	assert.Equal(t, "replica-a", got.OwnerNodeID, "a unary heartbeat does not move stream ownership")
}

// An edge this replica holds the stream of also has its in-memory heartbeat
// refreshed; an edge with no record at all is NotFound.
func TestSendHeartbeat_LocalStreamAndUnknownEdge(t *testing.T) {
	server, db := setupTestServer(t, nil)

	conn := &EdgeInstanceConnection{EdgeID: "edge-here", Namespace: "test", Status: "connected"}
	server.edgeMutex.Lock()
	server.edgeConnections["edge-here"] = conn
	server.edgeMutex.Unlock()
	require.NoError(t, db.Create(&models.EdgeInstance{EdgeID: "edge-here", Namespace: "test", Status: models.EdgeStatusConnected}).Error)

	_, err := server.SendHeartbeat(context.Background(), &pb.HeartbeatRequest{EdgeId: "edge-here"})
	require.NoError(t, err)
	conn.mu.Lock()
	assert.WithinDuration(t, time.Now(), conn.LastHeartbeat, 5*time.Second)
	conn.mu.Unlock()

	_, err = server.SendHeartbeat(context.Background(), &pb.HeartbeatRequest{EdgeId: "never-registered"})
	assert.Equal(t, codes.NotFound, status.Code(err))
}
