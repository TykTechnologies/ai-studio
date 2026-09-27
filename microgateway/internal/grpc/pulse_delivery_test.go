package grpc

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// pulseControl is a fakeControl that also accepts analytics pulses and
// counts the analytics events they carry.
type pulseControl struct {
	fakeControl
	mu     sync.Mutex
	events int
}

func (f *pulseControl) SendAnalyticsPulse(ctx context.Context, pulse *pb.AnalyticsPulse) (*pb.AnalyticsPulseResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events += len(pulse.AnalyticsEvents)
	return &pb.AnalyticsPulseResponse{Success: true, ProcessedRecords: uint64(pulse.TotalRecords)}, nil
}

func (f *pulseControl) received() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.events
}

func startPulseControl(t *testing.T, addr string, pc *pulseControl) func() {
	t.Helper()
	var lis net.Listener
	var err error
	require.Eventually(t, func() bool {
		lis, err = net.Listen("tcp", addr)
		return err == nil
	}, 5*time.Second, 20*time.Millisecond, "listen on %s", addr)
	srv := grpc.NewServer()
	pb.RegisterConfigurationSyncServiceServer(srv, pc)
	go srv.Serve(lis)
	return srv.Stop
}

// pulseEdge starts an edge client against addr and loads the built-in
// analytics pulse into a plugin manager, the way main.go wires them. The
// pulse interval is an hour, so records only leave on an explicit flush.
func pulseEdge(t *testing.T, addr string) (*SimpleEdgeClient, *plugins.PluginManager) {
	t.Helper()
	client := NewSimpleEdgeClient(&config.Config{
		HubSpoke: config.HubSpokeConfig{
			EdgeID:            "pulse-edge",
			EdgeNamespace:     "test",
			ControlEndpoint:   addr,
			AllowInsecure:     true,
			HeartbeatInterval: time.Hour,
		},
	}, "test", "hash", "time")
	client.reconnectInterval = 20 * time.Millisecond
	require.NoError(t, client.Start())

	pm := plugins.NewPluginManager(nil)
	pm.SetEdgeClient(client)
	require.NoError(t, pm.LoadDeferredBuiltinPlugins([]plugins.DataCollectionPluginConfig{{
		Name:      "analytics_pulse",
		Enabled:   true,
		HookTypes: []string{"analytics", "budget", "proxy_log"},
		Config: map[string]interface{}{
			"interval_seconds": 3600,
			"timeout_seconds":  2,
			"max_retries":      1,
		},
	}}))
	return client, pm
}

func recordAnalytics(t *testing.T, pm *plugins.PluginManager, id string) {
	t.Helper()
	require.NoError(t, pm.ExecuteDataCollectionPlugins("analytics", &interfaces.AnalyticsData{
		AppID: 1, LLMID: 1, RequestID: id, Timestamp: time.Now(), StatusCode: 200,
	}))
}

func freeAddr(t *testing.T) string {
	t.Helper()
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := probe.Addr().String()
	probe.Close()
	return addr
}

// After the hub restarts the edge reconnects on a new gRPC connection. The
// pulse used to keep the client of the first connection, which the reconnect
// had closed, so every later pulse failed with "the client connection is
// closing" and no edge analytics reached the hub again.
func TestAnalyticsPulse_DeliversAfterHubRestart(t *testing.T) {
	addr := freeAddr(t)
	pc := &pulseControl{}
	stop := startPulseControl(t, addr, pc)

	client, pm := pulseEdge(t, addr)
	defer client.Stop()
	require.Eventually(t, func() bool { return pc.streams.Load() == 1 }, 5*time.Second, 10*time.Millisecond, "initial subscription")

	// Hub restart.
	stop()
	time.Sleep(200 * time.Millisecond)
	stop = startPulseControl(t, addr, pc)
	defer stop()
	require.Eventually(t, func() bool { return pc.streams.Load() >= 2 }, 10*time.Second, 20*time.Millisecond, "edge never re-subscribed")

	for i := 0; i < 3; i++ {
		recordAnalytics(t, pm, fmt.Sprintf("after-restart-%d", i))
	}
	// Shutdown sends what is buffered.
	require.NoError(t, pm.Shutdown(context.Background()))

	assert.Equal(t, 3, pc.received(), "analytics recorded after a hub restart must reach the hub")
}

// Shutting the plugin manager down must stop the built-in pulse, whose Stop
// sends what is still buffered. Shutdown used to stop only the per-LLM
// plugins, so the buffer was dropped on every graceful edge shutdown.
func TestPluginManagerShutdown_FlushesAnalyticsPulse(t *testing.T) {
	addr := freeAddr(t)
	pc := &pulseControl{}
	stop := startPulseControl(t, addr, pc)
	defer stop()

	client, pm := pulseEdge(t, addr)
	defer client.Stop()

	for i := 0; i < 5; i++ {
		recordAnalytics(t, pm, fmt.Sprintf("buffered-%d", i))
	}
	require.NoError(t, pm.Shutdown(context.Background()))

	assert.Equal(t, 5, pc.received(), "records buffered at shutdown must be sent to the hub")
}
