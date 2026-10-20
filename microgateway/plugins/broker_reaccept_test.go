package plugins

import (
	"context"
	"testing"
	"time"

	mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/microgateway_management"
	"github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// capturePlugin hands the test the plugin side of the broker.
type capturePlugin struct {
	PluginGRPC
	pluginBroker chan *plugin.GRPCBroker
}

func (c *capturePlugin) GRPCServer(b *plugin.GRPCBroker, _ *grpc.Server) error {
	c.pluginBroker <- b
	return nil
}

// brokerPair returns the host's client wrapper and the plugin side of the
// broker of one in-memory go-plugin connection.
func brokerPair(t *testing.T) (*MicrogatewayPluginClient, *plugin.GRPCBroker) {
	t.Helper()
	cp := &capturePlugin{pluginBroker: make(chan *plugin.GRPCBroker, 1)}
	client, server := plugin.TestPluginGRPCConn(t, false, map[string]plugin.Plugin{"plugin": cp})
	_ = server // client.Close shuts the server down
	t.Cleanup(func() { _ = client.Close() })
	raw, err := client.Dispense("plugin")
	require.NoError(t, err)
	host, ok := raw.(*MicrogatewayPluginClient)
	require.True(t, ok, "dispensed %T", raw)
	select {
	case b := <-cp.pluginBroker:
		return host, b
	case <-time.After(5 * time.Second):
		t.Fatal("plugin side never got its broker")
		return nil, nil
	}
}

type testMgmtServer struct {
	mgmtpb.UnimplementedMicrogatewayManagementServiceServer
}

// A plugin that never dialled its brokered server gets a fresh broker ID on
// the next session renewal, and can connect to it; the unused server stops.
// A plugin that did connect keeps its broker.
func TestRenewSessionBroker_ReacceptsOnlyWhenNeverConnected(t *testing.T) {
	host, pluginSide := brokerPair(t)
	pm := &PluginManager{managementServer: &testMgmtServer{}}

	first, err := host.SetupServiceBroker(7, pm.managementServer)
	require.NoError(t, err)
	assert.False(t, host.BrokerConnected(first))

	next := pm.renewSessionBroker(7, host, first, 1)
	require.NotEqual(t, first, next, "a never-connected plugin gets a fresh ID")

	conn, err := pluginSide.Dial(next)
	require.NoError(t, err, "the plugin can dial the fresh ID")
	t.Cleanup(func() { _ = conn.Close() })
	_, err = mgmtpb.NewMicrogatewayManagementServiceClient(conn).GetApp(context.Background(), &mgmtpb.GetAppRequest{AppId: 1})
	require.Error(t, err) // Unimplemented, from the brokered server: the call got there
	require.Eventually(t, func() bool { return host.BrokerConnected(next) }, 2*time.Second, 10*time.Millisecond)

	assert.Equal(t, next, pm.renewSessionBroker(7, host, next, 2), "a connected plugin keeps its broker")
	host.mu.Lock()
	_, oldKept := host.servers[first]
	host.mu.Unlock()
	assert.False(t, oldKept, "the unused server was stopped")
}

// The bug this works around: go-plugin keeps a broker ID's connection info
// for five seconds after the host accepts on it, so a plugin that dials
// later cannot connect to that ID at all, while a fresh accept works.
func TestBrokerConnectionInfoExpires(t *testing.T) {
	if testing.Short() {
		t.Skip("waits for go-plugin's five-second window")
	}
	host, pluginSide := brokerPair(t)
	pm := &PluginManager{managementServer: &testMgmtServer{}}
	first, err := host.SetupServiceBroker(7, pm.managementServer)
	require.NoError(t, err)

	time.Sleep(6 * time.Second)
	_, err = pluginSide.Dial(first)
	require.Error(t, err, "the connection info for the first ID is gone")

	next := pm.renewSessionBroker(7, host, first, 1)
	conn, err := pluginSide.Dial(next)
	require.NoError(t, err)
	_ = conn.Close()
}
