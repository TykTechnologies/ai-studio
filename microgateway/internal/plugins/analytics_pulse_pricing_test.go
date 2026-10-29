package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The requested model and price multiplier survive the plugin hand-off and
// reach the pulse, so the hub's price recalculation can reach and reprice
// edge rows. Usage without a multiplier sends none.
func TestAnalyticsPulsePlugin_RequestedModelAndMultiplierReachPulse(t *testing.T) {
	plugin := &AnalyticsPulsePlugin{
		config:        &PulsePluginConfig{MaxBufferSize: 100},
		edgeID:        "test-edge",
		edgeNamespace: "test",
		lastPulseTime: time.Now().Add(-time.Minute),
	}
	now := time.Now()
	half := 0.5
	for _, d := range []*interfaces.AnalyticsData{
		{LLMID: 7, AppID: 5, ModelName: "gemini-2.0-flash-001", Vendor: "google_ai", RequestID: "req-1", StatusCode: 200,
			Timestamp: now, RequestedModel: "gemini-2.0-flash", PriceMultiplier: &half},
		{LLMID: 7, AppID: 5, ModelName: "gemini-2.0-flash-001", Vendor: "google_ai", RequestID: "req-2", StatusCode: 200, Timestamp: now},
	} {
		_, err := plugin.HandleAnalytics(context.Background(), d, nil)
		require.NoError(t, err)
	}
	pulse := plugin.buildPulseMessage(plugin.analyticsBuffer, plugin.analyticsMetadata, nil, nil, nil, nil, 1)
	require.Len(t, pulse.AnalyticsEvents, 2)
	assert.Equal(t, "gemini-2.0-flash", pulse.AnalyticsEvents[0].RequestedModel)
	require.NotNil(t, pulse.AnalyticsEvents[0].PriceMultiplier)
	assert.Equal(t, 0.5, *pulse.AnalyticsEvents[0].PriceMultiplier)
	assert.Empty(t, pulse.AnalyticsEvents[1].RequestedModel)
	assert.Nil(t, pulse.AnalyticsEvents[1].PriceMultiplier)
}
