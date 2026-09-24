package grpc

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// fakeControl is a minimal control server: it accepts registrations and
// holds subscription streams open until the server stops.
type fakeControl struct {
	pb.UnimplementedConfigurationSyncServiceServer
	registrations atomic.Int32
	streams       atomic.Int32
}

func (f *fakeControl) RegisterEdge(ctx context.Context, req *pb.EdgeRegistrationRequest) (*pb.EdgeRegistrationResponse, error) {
	f.registrations.Add(1)
	return &pb.EdgeRegistrationResponse{Success: true, SessionId: "s"}, nil
}

func (f *fakeControl) SubscribeToChanges(stream grpc.BidiStreamingServer[pb.EdgeMessage, pb.ControlMessage]) error {
	f.streams.Add(1)
	for {
		if _, err := stream.Recv(); err != nil {
			return err
		}
	}
}

func startFakeControl(t *testing.T, addr string, fc *fakeControl) *grpc.Server {
	t.Helper()
	var lis net.Listener
	var err error
	// The previous server may still hold the port for a moment.
	require.Eventually(t, func() bool {
		lis, err = net.Listen("tcp", addr)
		return err == nil
	}, 5*time.Second, 20*time.Millisecond, "listen on %s", addr)
	srv := grpc.NewServer()
	pb.RegisterConfigurationSyncServiceServer(srv, fc)
	go srv.Serve(lis)
	return srv
}

// An edge must reconnect after a control outage that outlasts more than
// one reconnect attempt. A failed attempt used to clear c.conn, which the
// retry loop read as "manually closed" and gave up for good.
func TestSimpleEdgeClient_ReconnectsAfterLongOutage(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := probe.Addr().String()
	probe.Close()

	fc := &fakeControl{}
	srv := startFakeControl(t, addr, fc)

	client := NewSimpleEdgeClient(&config.Config{
		HubSpoke: config.HubSpokeConfig{
			EdgeID:            "outage-edge",
			EdgeNamespace:     "test",
			ControlEndpoint:   addr,
			AllowInsecure:     true,
			HeartbeatInterval: time.Hour,
		},
	}, "test", "hash", "time")
	client.reconnectInterval = 20 * time.Millisecond
	require.NoError(t, client.Start())
	defer client.Stop()
	// Start returns once the stream is opened client-side; the server handler
	// may not have run yet.
	require.Eventually(t, func() bool {
		return fc.streams.Load() == 1
	}, 5*time.Second, 10*time.Millisecond, "initial subscription")

	// Outage: long enough for several reconnect attempts to fail.
	srv.Stop()
	time.Sleep(400 * time.Millisecond)

	srv = startFakeControl(t, addr, fc)
	defer srv.Stop()

	require.Eventually(t, func() bool {
		return fc.streams.Load() >= 2
	}, 10*time.Second, 50*time.Millisecond, "edge never re-subscribed after the control server came back")
}

// Stop must still end the reconnect loop.
func TestSimpleEdgeClient_StopEndsReconnection(t *testing.T) {
	client := NewSimpleEdgeClient(&config.Config{
		HubSpoke: config.HubSpokeConfig{
			EdgeID:          "stop-edge",
			EdgeNamespace:   "test",
			ControlEndpoint: "127.0.0.1:1",
			AllowInsecure:   true,
		},
	}, "test", "hash", "time")
	client.reconnectInterval = 10 * time.Millisecond

	done := make(chan struct{})
	go func() {
		client.attemptReconnection()
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	// Close stopCh the way Stop does, without Stop's unsynchronised c.conn read.
	close(client.stopCh)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reconnect loop kept running after Stop")
	}
}

// A stream that drops again right after a reconnect starts a second
// reconnection while the first is still unwinding. Only one may run: the
// guard is a single atomic step, so concurrent callers do not race on the
// client's reconnection state (run with -race).
func TestSimpleEdgeClient_ConcurrentReconnectionsRunOnce(t *testing.T) {
	client := NewSimpleEdgeClient(&config.Config{
		HubSpoke: config.HubSpokeConfig{
			EdgeID:          "concurrent-edge",
			EdgeNamespace:   "test",
			ControlEndpoint: "127.0.0.1:1",
			AllowInsecure:   true,
		},
	}, "test", "hash", "time")
	client.reconnectInterval = 5 * time.Millisecond

	const callers = 8
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client.attemptReconnection()
		}()
	}
	require.Eventually(t, client.reconnecting.Load, time.Second, time.Millisecond, "one reconnection is running")
	time.Sleep(30 * time.Millisecond)
	close(client.stopCh)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reconnection did not stop")
	}
	assert.False(t, client.reconnecting.Load())
}
