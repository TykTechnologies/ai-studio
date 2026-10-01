package grpc

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	grpclib "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/TykTechnologies/midsommar/v2/pkg/safe"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// panickyHealth panics on its first Check and answers the ones after.
type panickyHealth struct {
	healthpb.UnimplementedHealthServer
	calls atomic.Int32
}

func (h *panickyHealth) Check(context.Context, *healthpb.HealthCheckRequest) (*healthpb.HealthCheckResponse, error) {
	if h.calls.Add(1) == 1 {
		panic("handler bug")
	}
	return &healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING}, nil
}

// A panicking RPC fails with Internal and the server goes on serving: the
// control server's interceptor chain recovers before grpc-go would let the
// panic end the process.
func TestControlServerInterceptorsRecoverHandlerPanic(t *testing.T) {
	control, _ := setupTestServer(t, nil)
	t.Cleanup(control.Stop)

	server := grpclib.NewServer(control.interceptors()...)
	health := &panickyHealth{}
	healthpb.RegisterHealthServer(server, health)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = server.Serve(lis) }()
	t.Cleanup(server.Stop)

	conn, err := grpclib.NewClient(lis.Addr().String(), grpclib.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	client := healthpb.NewHealthClient(conn)
	ctx, cancel := context.WithTimeout(metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+testAuthToken), 5*time.Second)
	defer cancel()

	before := safe.Panics()
	_, err = client.Check(ctx, &healthpb.HealthCheckRequest{})
	assert.Equal(t, codes.Internal, status.Code(err))
	assert.Equal(t, before+1, safe.Panics())

	resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err, "the server must keep serving after a recovered panic")
	assert.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.Status)
}

// Recovery runs before authentication, so a call that would panic still
// has to authenticate first: an unauthenticated one never reaches it.
func TestControlServerInterceptorsAuthenticateInsideRecovery(t *testing.T) {
	control, _ := setupTestServer(t, nil)
	t.Cleanup(control.Stop)
	var reached bool
	info := &grpclib.UnaryServerInfo{FullMethod: "/test/Method"}
	_, err := recoverUnary(context.Background(), nil, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return control.authInterceptor(ctx, req, info, func(context.Context, interface{}) (interface{}, error) {
			reached = true
			return nil, nil
		})
	})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.False(t, reached)
}

func TestRecoverStreamReturnsInternal(t *testing.T) {
	err := recoverStream(nil, nil, &grpclib.StreamServerInfo{FullMethod: "/test/Stream"}, func(interface{}, grpclib.ServerStream) error {
		panic("stream handler bug")
	})
	assert.Equal(t, codes.Internal, status.Code(err))
}

// A panic handling one edge's message ends that edge's stream with
// Internal (the edge reconnects); the server and the other edges carry on.
func TestEdgeStreamPanicEndsOnlyThatStream(t *testing.T) {
	control, db := setupTestServer(t, nil)
	t.Cleanup(control.Stop)
	control.pushReadyGrace = 0

	registerEdge(t, control, "edge-steady", "default")
	registerEdge(t, control, "edge-faulty", "default")
	other, stopOther := connect(t, control, "edge-steady", "default")
	defer stopOther()

	stream := newFakeEdgeStream()
	defer stream.cancel()
	result := make(chan error, 1)
	go func() { result <- control.SubscribeToChanges(stream) }()
	stream.in <- &pb.EdgeMessage{Message: &pb.EdgeMessage_Registration{Registration: &pb.EdgeRegistrationRequest{EdgeId: "edge-faulty", EdgeNamespace: "default", Version: "test"}}}
	require.Eventually(t, func() bool {
		for _, m := range stream.sent() {
			if m.GetRegistrationResponse() != nil {
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond, "stream registration")

	before := safe.Panics()
	// A nil message: the receive loop dereferences it and panics.
	stream.in <- nil

	select {
	case err := <-result:
		assert.Equal(t, codes.Internal, status.Code(err))
	case <-time.After(5 * time.Second):
		t.Fatal("the faulty edge's stream did not end after the panic")
	}
	assert.Equal(t, before+1, safe.Panics())
	assert.Equal(t, "disconnected", streamEdgeRow(t, db, "edge-faulty").Status)

	// The steady edge's stream still works, and the faulty edge can come back.
	heartbeat(other, "edge-steady")
	assert.Eventually(t, func() bool {
		for _, m := range other.sent() {
			if m.GetHeartbeatResponse() != nil {
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond, "heartbeat answered on the other stream")
	_, stopAgain := connect(t, control, "edge-faulty", "default")
	stopAgain()
}
