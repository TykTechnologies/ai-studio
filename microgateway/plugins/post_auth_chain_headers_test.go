package plugins

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// chainService attaches the given plugins to every LLM. Every other method
// panics through the nil embedded interface, which these tests never reach.
type chainService struct {
	PluginServiceInterface
	plugins []PluginData
}

func (s chainService) GetPluginsForLLM(uint) ([]PluginData, error) { return s.plugins, nil }

// postAuthClient answers ProcessPostAuth with resp and records the request
// headers it was sent.
type postAuthClient struct {
	pb.PluginServiceClient
	resp *pb.PluginResponse
	got  map[string]string
}

func (c *postAuthClient) ProcessPostAuth(ctx context.Context, in *pb.EnrichedRequest, _ ...grpc.CallOption) (*pb.PluginResponse, error) {
	if _, ok := ctx.Deadline(); !ok {
		return nil, context.DeadlineExceeded
	}
	c.got = in.Request.Headers
	return c.resp, nil
}

func postAuthChainManager(clients ...*postAuthClient) *PluginManager {
	var data []PluginData
	pm := NewPluginManager(nil)
	for i, c := range clients {
		id := uint(i + 1)
		data = append(data, PluginData{ID: id, Name: "p", HookType: string(interfaces.HookTypePostAuth), IsActive: true})
		pm.loadedPlugins[id] = &LoadedPlugin{ID: id, Name: "p", GRPCClient: c, IsHealthy: true}
	}
	pm.service = chainService{plugins: data}
	return pm
}

// A plugin that returns only the header it sets must not hide the rest of the
// request's headers (Authorization among them) from the next plugin.
func TestExecutePluginChain_PostAuthHeadersMergeForNextPlugin(t *testing.T) {
	first := &postAuthClient{resp: &pb.PluginResponse{Modified: true, Headers: map[string]string{"X-First": "1"}}}
	second := &postAuthClient{resp: &pb.PluginResponse{}}
	pm := postAuthChainManager(first, second)

	in := &interfaces.EnrichedRequest{PluginRequest: &interfaces.PluginRequest{
		Headers: map[string]string{"Authorization": "Bearer app", "Anthropic-Beta": "a"},
	}}
	out, err := pm.ExecutePluginChain(1, interfaces.HookTypePostAuth, in, &interfaces.PluginContext{})
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"Authorization": "Bearer app", "Anthropic-Beta": "a", "X-First": "1"}, second.got)
	got := out.(*interfaces.EnrichedRequest).PluginRequest.Headers
	assert.Equal(t, second.got, got)
}

// A chain of plugins that change nothing hands back the headers it was sent.
func TestExecutePluginChain_PostAuthUnmodifiedKeepsHeaders(t *testing.T) {
	pm := postAuthChainManager(&postAuthClient{resp: &pb.PluginResponse{}}, &postAuthClient{resp: &pb.PluginResponse{}})
	sent := map[string]string{"Anthropic-Beta": "a"}
	in := &interfaces.EnrichedRequest{PluginRequest: &interfaces.PluginRequest{Headers: sent}}

	out, err := pm.ExecutePluginChain(1, interfaces.HookTypePostAuth, in, &interfaces.PluginContext{})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"Anthropic-Beta": "a"}, out.(*interfaces.EnrichedRequest).PluginRequest.Headers)
}
