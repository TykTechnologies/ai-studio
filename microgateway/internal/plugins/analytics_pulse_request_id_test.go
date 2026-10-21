package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The gateway's request id, the caller's, and whether the request produced
// usage reach the pulse, so the hub can file a blocked request's log row
// under the id its compliance event names.
func TestAnalyticsPulsePlugin_RequestIDsAndNoUsageReachPulse(t *testing.T) {
	plugin := &AnalyticsPulsePlugin{
		config:        &PulsePluginConfig{MaxBufferSize: 100},
		edgeID:        "test-edge",
		edgeNamespace: "test",
		lastPulseTime: time.Now().Add(-time.Minute),
	}
	now := time.Now()
	_, err := plugin.HandleAnalytics(context.Background(), &interfaces.AnalyticsData{
		LLMID: 11, AppID: 408, Vendor: "openai", RequestID: "proxy_408_1", StatusCode: 400, Timestamp: now,
		GatewayRequestID: "tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1b", ClientRequestID: "lb-1", NoUsage: true,
	}, nil)
	require.NoError(t, err)
	_, err = plugin.HandleAnalytics(context.Background(), &interfaces.AnalyticsData{
		LLMID: 11, AppID: 408, Vendor: "openai", RequestID: "proxy_408_2", StatusCode: 200, Timestamp: now,
		TotalTokens: 10,
	}, nil)
	require.NoError(t, err)

	pulse := plugin.buildPulseMessage(plugin.analyticsBuffer, plugin.analyticsMetadata, nil, nil, nil, nil, 1)
	require.Len(t, pulse.AnalyticsEvents, 2)
	blocked := pulse.AnalyticsEvents[0]
	assert.Equal(t, "proxy_408_1", blocked.RequestId)
	assert.Equal(t, "tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1b", blocked.GatewayRequestId)
	assert.Equal(t, "lb-1", blocked.ClientRequestId)
	assert.True(t, blocked.NoUsage)
	assert.False(t, pulse.AnalyticsEvents[1].NoUsage)
}
