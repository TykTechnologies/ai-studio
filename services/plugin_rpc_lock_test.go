package services

import (
	"context"
	"testing"
	"time"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// blockingRPCPluginClient holds every UI and portal RPC until release closes.
type blockingRPCPluginClient struct {
	pb.PluginServiceClient
	entered chan struct{}
	release chan struct{}
}

func (f *blockingRPCPluginClient) Call(ctx context.Context, req *pb.CallRequest, opts ...grpc.CallOption) (*pb.CallResponse, error) {
	f.entered <- struct{}{}
	<-f.release
	return &pb.CallResponse{Success: true, Data: `{}`}, nil
}

func (f *blockingRPCPluginClient) PortalCall(ctx context.Context, req *pb.PortalCallRequest, opts ...grpc.CallOption) (*pb.PortalCallResponse, error) {
	f.entered <- struct{}{}
	<-f.release
	return &pb.PortalCallResponse{Success: true, Data: `{}`}, nil
}

// bqo: a plugin RPC in flight must not hold the manager's lock, or a
// session renewal (a writer) waits for the RPC's 30 s timeout.
func TestPluginRPC_DoesNotBlockWriters(t *testing.T) {
	for name, call := range map[string]func(m *AIStudioPluginManager) (interface{}, error){
		"ui":     func(m *AIStudioPluginManager) (interface{}, error) { return m.CallPluginRPCAs(7, "slow", nil, nil) },
		"portal": func(m *AIStudioPluginManager) (interface{}, error) { return m.CallPluginPortalRPC(7, "slow", nil, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			client := &blockingRPCPluginClient{entered: make(chan struct{}, 1), release: make(chan struct{})}
			m := &AIStudioPluginManager{loadedPlugins: map[uint]*LoadedAIStudioPlugin{
				7: {ID: 7, GRPCClient: client, IsHealthy: true, SessionBrokerID: 1}}}

			done := make(chan error, 1)
			go func() { _, err := call(m); done <- err }()
			<-client.entered

			// What renewSessionBroker does with the new broker ID.
			renewed := make(chan struct{})
			go func() {
				m.mu.Lock()
				m.loadedPlugins[7].SessionBrokerID = 2
				m.mu.Unlock()
				close(renewed)
			}()
			select {
			case <-renewed:
			case <-time.After(2 * time.Second):
				close(client.release)
				t.Fatal("the renewal waited for the plugin RPC")
			}

			close(client.release)
			require.NoError(t, <-done)
		})
	}
}
