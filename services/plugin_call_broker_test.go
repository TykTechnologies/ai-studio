package services

import (
	"context"
	"runtime"
	"testing"
	"time"

	mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// capturePluginGRPC is AIStudioPluginGRPC that hands the test the plugin
// side of the broker.
type capturePluginGRPC struct {
	AIStudioPluginGRPC
	pluginBroker chan *goplugin.GRPCBroker
}

func (c *capturePluginGRPC) GRPCServer(b *goplugin.GRPCBroker, _ *grpc.Server) error {
	c.pluginBroker <- b
	return nil
}

// studioBrokerPair returns the host's client wrapper and the plugin side of
// the broker of one in-memory go-plugin connection.
func studioBrokerPair(t *testing.T) (*AIStudioPluginClient, *goplugin.GRPCBroker) {
	t.Helper()
	cp := &capturePluginGRPC{pluginBroker: make(chan *goplugin.GRPCBroker, 1)}
	client, _ := goplugin.TestPluginGRPCConn(t, false, map[string]goplugin.Plugin{"plugin": cp})
	t.Cleanup(func() { _ = client.Close() })
	raw, err := client.Dispense("plugin")
	require.NoError(t, err)
	host, ok := raw.(*AIStudioPluginClient)
	require.True(t, ok, "dispensed %T", raw)
	host.service = &Service{} // startCallBroker needs a service to serve
	select {
	case b := <-cp.pluginBroker:
		return host, b
	case <-time.After(5 * time.Second):
		t.Fatal("plugin side never got its broker")
		return nil, nil
	}
}

func withCallBrokerGrace(t *testing.T, d time.Duration) {
	t.Helper()
	orig := callBrokerGrace
	callBrokerGrace = d
	t.Cleanup(func() { callBrokerGrace = orig })
}

// Per-call brokered servers the plugin never connects to are stopped: many
// scheduled tasks or edge payloads leave no goroutines behind. They used to
// run for good, one goroutine and listener per call.
func TestCallBroker_UnusedServersStop(t *testing.T) {
	withCallBrokerGrace(t, 100*time.Millisecond)
	host, _ := studioBrokerPair(t)

	time.Sleep(100 * time.Millisecond)
	before := runtime.NumGoroutine()
	for range 20 {
		host.startCallBroker(7, "edge payload")
	}
	assert.Greater(t, runtime.NumGoroutine(), before+10, "the calls started servers")
	require.Eventually(t, func() bool { return runtime.NumGoroutine() <= before+2 },
		10*time.Second, 50*time.Millisecond, "servers still running: %d goroutines, %d before", runtime.NumGoroutine(), before)
}

// A plugin that dials the per-call broker during the call (a plugin built
// before 2.3, which dials lazily and keeps its first connection) keeps a
// working connection after the grace period.
func TestCallBroker_ConnectedServerStays(t *testing.T) {
	withCallBrokerGrace(t, 100*time.Millisecond)
	host, pluginSide := studioBrokerPair(t)

	id := host.startCallBroker(7, "scheduled task")
	conn, err := pluginSide.Dial(id)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client := mgmtpb.NewAIStudioManagementServiceClient(conn)
	_, err = client.ListPlugins(context.Background(), &mgmtpb.ListPluginsRequest{})
	require.Error(t, err)

	time.Sleep(400 * time.Millisecond) // well past the grace
	_, err = client.ListPlugins(context.Background(), &mgmtpb.ListPluginsRequest{})
	require.Error(t, err)
	assert.NotEqual(t, codes.Unavailable, status.Code(err), "the server the plugin uses was stopped: %v", err)
}

// SetupServiceBroker (agent sessions) goes through the same per-call path.
func TestSetupServiceBroker_StopsWhenUnused(t *testing.T) {
	withCallBrokerGrace(t, 100*time.Millisecond)
	host, _ := studioBrokerPair(t)
	time.Sleep(100 * time.Millisecond)
	before := runtime.NumGoroutine()
	for range 10 {
		_, err := host.SetupServiceBroker(7)
		require.NoError(t, err)
	}
	require.Eventually(t, func() bool { return runtime.NumGoroutine() <= before+2 }, 10*time.Second, 50*time.Millisecond)
}
