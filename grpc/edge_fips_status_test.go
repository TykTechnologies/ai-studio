package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/fipsinfo"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The hub records each edge's FIPS 140-3 status from its heartbeats; an edge
// that does not report one (released before the field) shows as unknown.
func TestEdgeFIPSStatusFromHeartbeats(t *testing.T) {
	db := setupTestDB(t)
	server := replicaServer(t, db, "node-a")

	registerEdge(t, server, "edge-fips", "")
	stream, stop := connect(t, server, "edge-fips", "")
	defer stop()
	assert.Nil(t, streamEdgeRow(t, db, "edge-fips").FIPS, "an edge that has reported nothing is unknown")

	st := fipsinfo.Status{Enabled: true, Version: "v1.0.0-c2097c7c", Enforced: false}
	stream.in <- &pb.EdgeMessage{Message: &pb.EdgeMessage_Heartbeat{Heartbeat: &pb.HeartbeatRequest{
		EdgeId: "edge-fips",
		Health: &pb.HealthStatus{Status: pb.HealthStatus_HEALTHY, Metrics: st.Metrics()},
	}}}
	require.Eventually(t, func() bool {
		f := streamEdgeRow(t, db, "edge-fips").FIPS
		return f != nil && *f == st
	}, 5*time.Second, 10*time.Millisecond, "FIPS status recorded from the heartbeat")

	// A heartbeat without the field leaves the recorded status alone.
	heartbeat(stream, "edge-fips")
	time.Sleep(100 * time.Millisecond)
	got := streamEdgeRow(t, db, "edge-fips").FIPS
	require.NotNil(t, got)
	assert.Equal(t, st, *got)

	// The same edge re-registering as an older release (no status) is
	// unknown again.
	_, err := server.RegisterEdge(context.Background(), &pb.EdgeRegistrationRequest{EdgeId: "edge-fips", Version: "old"})
	require.NoError(t, err)
	assert.Nil(t, streamEdgeRow(t, db, "edge-fips").FIPS)

	// Registration carries it too.
	off := fipsinfo.Status{Version: "latest"}
	_, err = server.RegisterEdge(context.Background(), &pb.EdgeRegistrationRequest{
		EdgeId: "edge-fips", Version: "new", Health: &pb.HealthStatus{Metrics: off.Metrics()},
	})
	require.NoError(t, err)
	got = streamEdgeRow(t, db, "edge-fips").FIPS
	require.NotNil(t, got)
	assert.Equal(t, off, *got)
}
