package server

import (
	"context"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/plugins"
	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// countingControl stands in for the control server's pulse endpoint.
type countingControl struct {
	pb.ConfigurationSyncServiceClient
	mu     sync.Mutex
	events int
}

func (c *countingControl) SendAnalyticsPulse(ctx context.Context, pulse *pb.AnalyticsPulse, _ ...grpc.CallOption) (*pb.AnalyticsPulseResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events += len(pulse.AnalyticsEvents)
	return &pb.AnalyticsPulseResponse{Success: true}, nil
}

func (c *countingControl) received() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.events
}

// fakeEdge is the part of the edge client the pulse uses.
type fakeEdge struct{ control *countingControl }

func (e fakeEdge) GetGRPCClient() pb.ConfigurationSyncServiceClient { return e.control }
func (e fakeEdge) GetEdgeID() string                                { return "edge" }
func (e fakeEdge) GetEdgeNamespace() string                         { return "" }

// A request still being served when shutdown starts records its analytics
// when it finishes. Shutdown used to stop the plugins before draining the
// HTTP server, so those records never reached the pulse, and the pulse's
// buffer was never sent anyway.
func TestShutdown_PulsesRequestsServedDuringDrain(t *testing.T) {
	control := &countingControl{}
	pm := plugins.NewPluginManager(nil)
	pm.SetEdgeClient(fakeEdge{control})
	require.NoError(t, pm.LoadDeferredBuiltinPlugins([]plugins.DataCollectionPluginConfig{{
		Name: "analytics_pulse", Enabled: true, HookTypes: []string{"analytics"},
		Config: map[string]interface{}{"interval_seconds": 3600, "timeout_seconds": 2},
	}}))

	entered := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		time.Sleep(200 * time.Millisecond)
		_ = pm.ExecuteDataCollectionPlugins("analytics", &interfaces.AnalyticsData{
			AppID: 1, RequestID: "draining", Timestamp: time.Now(), StatusCode: 200,
		})
		w.WriteHeader(http.StatusOK)
	})

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	s := &Server{server: &http.Server{Handler: handler}, pluginManager: pm}
	go s.server.Serve(lis)

	respErr := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + lis.Addr().String() + "/")
		if err == nil {
			resp.Body.Close()
		}
		respErr <- err
	}()
	<-entered

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, s.Shutdown(ctx))
	require.NoError(t, <-respErr, "the request in flight is served")

	assert.Equal(t, 1, control.received(), "the request served during the drain must be pulsed")
}
