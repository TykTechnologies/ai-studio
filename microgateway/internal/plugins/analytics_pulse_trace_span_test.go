package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The agent tracing span id and the edge's request id ride on the pulse, so
// the hub's proxy log can link to the trace.
func TestAnalyticsPulse_CarriesTraceSpanIDAndRequestID(t *testing.T) {
	client := &fakeSyncClient{}
	p := newRetryPlugin(client, PulsePluginConfig{MaxRetries: 1})

	_, err := p.HandleAnalytics(context.Background(), &interfaces.AnalyticsData{
		LLMID: 1, AppID: 2, ModelName: "gpt-4o", Vendor: "openai", Timestamp: time.Now(),
		RequestID: "req_edge_1", TraceSpanID: "00f067aa0ba902b7", StatusCode: 200,
	}, nil)
	require.NoError(t, err)
	p.sendPulse()

	require.Equal(t, 1, client.callCount())
	events := client.pulses[0].AnalyticsEvents
	require.Len(t, events, 1)
	assert.Equal(t, "req_edge_1", events[0].RequestId)
	assert.Equal(t, "00f067aa0ba902b7", events[0].TraceSpanId)
}
