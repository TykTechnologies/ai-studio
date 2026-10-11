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

// capturingAnalyticsClient is a gRPC plugin client that records the analytics
// request it is sent. Every other method panics through the nil embedded
// interface, which this test never reaches.
type capturingAnalyticsClient struct {
	pb.PluginServiceClient
	got *pb.AnalyticsRequest
}

func (c *capturingAnalyticsClient) HandleAnalytics(ctx context.Context, in *pb.AnalyticsRequest, _ ...grpc.CallOption) (*pb.DataCollectionResponse, error) {
	c.got = in
	return &pb.DataCollectionResponse{Success: true}, nil
}

// An external data-collector plugin on the edge receives a decision call's
// model class and question counts, and the context's vendor, which it used
// to be sent without.
func TestExecuteAnalyticsPlugin_SendsDecisionFields(t *testing.T) {
	client := &capturingAnalyticsClient{}
	pm := &PluginManager{}
	plugin := &GlobalPlugin{Config: DataCollectionPluginConfig{Name: "collector"}, GRPCClient: client}

	require.NoError(t, pm.executeAnalyticsPlugin(context.Background(), plugin, &interfaces.AnalyticsData{
		LLMID: 5, ModelName: "jev-1.13.0", Vendor: "systemone", PromptTokens: 200, Choices: 2,
		RequestID: "req-1", RequestKind: "decision", DecisionQuestions: 3, DecisionRefusals: 1,
	}))
	require.NotNil(t, client.got)
	assert.Equal(t, "decision", client.got.RequestKind)
	assert.Equal(t, int32(3), client.got.DecisionQuestions)
	assert.Equal(t, int32(1), client.got.DecisionRefusals)
	assert.Equal(t, int32(2), client.got.Choices)
	assert.Equal(t, "systemone", client.got.Context.Vendor)
	assert.Equal(t, "decision", client.got.Context.RequestKind)

	// Chat traffic carries no decision fields.
	require.NoError(t, pm.executeAnalyticsPlugin(context.Background(), plugin, &interfaces.AnalyticsData{
		LLMID: 6, ModelName: "gpt-4o", Vendor: "openai", RequestID: "req-2",
	}))
	assert.Equal(t, "", client.got.RequestKind)
	assert.Zero(t, client.got.DecisionQuestions)
}
