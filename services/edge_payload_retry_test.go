package services

import (
	"context"
	"errors"
	"testing"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// edgePayloadPluginClient answers AcceptEdgePayload with resp, or err.
type edgePayloadPluginClient struct {
	pb.PluginServiceClient
	resp *pb.EdgePayloadResponse
	err  error
}

func (f *edgePayloadPluginClient) AcceptEdgePayload(context.Context, *pb.EdgePayloadRequest, ...grpc.CallOption) (*pb.EdgePayloadResponse, error) {
	return f.resp, f.err
}

// rlk: an edge payload a plugin could not take for a reason that may pass
// (a service API call refused while its scopes await approval, the plugin
// unavailable or unhealthy) is ErrEdgePayloadRetry, so the edge keeps it
// queued; the plugin's own refusal of the payload is final.
func TestRouteEdgePayload_RetryableFailures(t *testing.T) {
	kvDenied := status.Error(codes.PermissionDenied, "service access not authorized for plugin smoke-prompt-compressor")
	cases := []struct {
		name    string
		client  *edgePayloadPluginClient
		healthy bool
		retry   bool
	}{
		{"scope awaiting approval", &edgePayloadPluginClient{resp: &pb.EdgePayloadResponse{
			ErrorMessage: "failed to read KV data: " + kvDenied.Error()}}, true, true},
		{"host service unavailable", &edgePayloadPluginClient{resp: &pb.EdgePayloadResponse{
			ErrorMessage: "failed to write KV data: " + status.Error(codes.Unavailable, "connection refused").Error()}}, true, true},
		{"plugin unreachable", &edgePayloadPluginClient{err: status.Error(codes.Unavailable, "plugin exited")}, true, true},
		{"plugin unhealthy", &edgePayloadPluginClient{}, false, true},
		{"bad payload", &edgePayloadPluginClient{resp: &pb.EdgePayloadResponse{ErrorMessage: "invalid savings record"}}, true, false},
		{"not a receiver", &edgePayloadPluginClient{resp: &pb.EdgePayloadResponse{
			ErrorMessage: "plugin does not implement EdgePayloadReceiver"}}, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &AIStudioPluginManager{loadedPlugins: map[uint]*LoadedAIStudioPlugin{
				7: {ID: 7, GRPCClient: tc.client, IsHealthy: tc.healthy}}}
			err := m.RouteEdgePayload(context.Background(), &pb.PluginControlPayload{PluginId: 7, Payload: []byte("x")})
			if assert.Error(t, err) {
				assert.Equal(t, tc.retry, errors.Is(err, ErrEdgePayloadRetry), err.Error())
			}
		})
	}

	m := &AIStudioPluginManager{loadedPlugins: map[uint]*LoadedAIStudioPlugin{
		7: {ID: 7, GRPCClient: &edgePayloadPluginClient{resp: &pb.EdgePayloadResponse{Success: true, Handled: true}}, IsHealthy: true}}}
	assert.NoError(t, m.RouteEdgePayload(context.Background(), &pb.PluginControlPayload{PluginId: 7, Payload: []byte("x")}))
}
