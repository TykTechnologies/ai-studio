package services

import (
	"context"
	"testing"
	"time"

	mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A plugin that never connected to its session broker is served on a fresh
// broker ID at the next renewal, the plugin's SessionBrokerID follows, and
// the unused server stops. A plugin that connected keeps its broker.
func TestRenewSessionBroker_ReacceptsUntilConnected(t *testing.T) {
	host, pluginSide := studioBrokerPair(t)
	m := &AIStudioPluginManager{loadedPlugins: map[uint]*LoadedAIStudioPlugin{7: {ID: 7}}}

	first, firstBroker := host.startTrackedBroker(7, "plugin services")
	m.loadedPlugins[7].SessionBrokerID = first

	next, nextBroker := m.renewSessionBroker(7, host, first, firstBroker, 1)
	require.NotEqual(t, first, next)
	assert.Equal(t, next, m.loadedPlugins[7].SessionBrokerID, "RPCs and payloads name the new ID")

	conn, err := pluginSide.Dial(next)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = mgmtpb.NewAIStudioManagementServiceClient(conn).ListPlugins(context.Background(), &mgmtpb.ListPluginsRequest{})
	require.Error(t, err)
	assert.NotEqual(t, codes.Unavailable, status.Code(err), "the call reached the new server")
	require.Eventually(t, nextBroker.connected.Load, 2*time.Second, 10*time.Millisecond)

	again, _ := m.renewSessionBroker(7, host, next, nextBroker, 2)
	assert.Equal(t, next, again, "a connected plugin keeps its broker")

	firstBroker.mu.Lock()
	stopped := firstBroker.stopped
	firstBroker.mu.Unlock()
	assert.True(t, stopped, "the unused server was stopped")
}

// The trap this works around, end to end: a lazily dialling plugin (an SDK
// before 2.3) cannot reach a session broker accepted more than five seconds
// earlier, and can reach the one a renewal offers.
func TestRenewSessionBroker_RecoversALateDialler(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for go-plugin's five-second window")
	}
	host, pluginSide := studioBrokerPair(t)
	m := &AIStudioPluginManager{loadedPlugins: map[uint]*LoadedAIStudioPlugin{7: {ID: 7}}}
	first, firstBroker := host.startTrackedBroker(7, "plugin services")

	time.Sleep(6 * time.Second)
	_, err := pluginSide.Dial(first)
	require.Error(t, err, "the connection info for the load-time broker is gone")

	next, _ := m.renewSessionBroker(7, host, first, firstBroker, 1)
	conn, err := pluginSide.Dial(next)
	require.NoError(t, err)
	_ = conn.Close()
}
