package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The service tier survives the plugin hand-off and reaches the pulse, so
// the hub can break edge spend down by tier.
func TestAnalyticsPulsePlugin_ServiceTierReachesPulse(t *testing.T) {
	plugin := &AnalyticsPulsePlugin{
		config:        &PulsePluginConfig{MaxBufferSize: 100},
		edgeID:        "test-edge",
		edgeNamespace: "test",
		lastPulseTime: time.Now().Add(-time.Minute),
	}
	now := time.Now()
	for _, tier := range []string{"batch", ""} {
		_, err := plugin.HandleAnalytics(context.Background(), &interfaces.AnalyticsData{
			LLMID: 7, AppID: 5, ModelName: "claude-luna", Vendor: "anthropic",
			RequestID: "req-" + tier, StatusCode: 200, Timestamp: now, ServiceTier: tier,
		}, nil)
		require.NoError(t, err)
	}
	pulse := plugin.buildPulseMessage(plugin.analyticsBuffer, plugin.analyticsMetadata, nil, nil, nil, nil, 1)
	require.Len(t, pulse.AnalyticsEvents, 2)
	assert.Equal(t, "batch", pulse.AnalyticsEvents[0].ServiceTier)
	assert.Equal(t, "", pulse.AnalyticsEvents[1].ServiceTier)
}
