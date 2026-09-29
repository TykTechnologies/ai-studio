package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The request's latency reaches the pulse. The plugin wrote 0 ("not
// available") on every event, so the hub never had one for edge traffic.
func TestAnalyticsPulsePlugin_LatencyReachesPulse(t *testing.T) {
	plugin := &AnalyticsPulsePlugin{
		config:        &PulsePluginConfig{MaxBufferSize: 100},
		edgeID:        "test-edge",
		edgeNamespace: "test",
		lastPulseTime: time.Now().Add(-time.Minute),
	}

	_, err := plugin.HandleAnalytics(context.Background(), &interfaces.AnalyticsData{
		LLMID: 7, AppID: 5, ModelName: "gpt-4o", Vendor: "openai",
		RequestID: "req-1", StatusCode: 200, Timestamp: time.Now(), TotalTimeMS: 321,
	}, nil)
	require.NoError(t, err)

	require.Len(t, plugin.analyticsBuffer, 1)
	assert.Equal(t, 321, plugin.analyticsBuffer[0].TotalTimeMS)

	pulse := plugin.buildPulseMessage(plugin.analyticsBuffer, plugin.analyticsMetadata, nil, nil, nil, nil, 1)
	require.NotNil(t, pulse)
	require.Len(t, pulse.AnalyticsEvents, 1)
	assert.Equal(t, uint32(321), pulse.AnalyticsEvents[0].LatencyMs)
}
