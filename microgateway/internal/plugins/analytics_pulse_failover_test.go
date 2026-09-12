package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The failover marker on an analytics event must survive the plugin hand-off
// and reach the pulse, or the hub's ProxyLog cannot say which primary the
// rung was failing over from. Before this test the interfaces.AnalyticsData
// struct had no failover fields and every pulsed event arrived unmarked.
func TestAnalyticsPulsePlugin_FailoverMarkerReachesPulse(t *testing.T) {
	plugin := &AnalyticsPulsePlugin{
		config:        &PulsePluginConfig{MaxBufferSize: 100},
		edgeID:        "test-edge",
		edgeNamespace: "test",
		lastPulseTime: time.Now().Add(-time.Minute),
	}

	from := uint(8)
	now := time.Now()
	_, err := plugin.HandleAnalytics(context.Background(), &interfaces.AnalyticsData{
		LLMID:             7,
		AppID:             5,
		ModelName:         "mock-gpt",
		Vendor:            "openai",
		RequestID:         "req-fallback",
		StatusCode:        200,
		Timestamp:         now,
		FailoverFromLLMID: &from,
		FailoverAttempt:   1,
	}, nil)
	require.NoError(t, err)

	// A primary attempt carries no marker.
	_, err = plugin.HandleAnalytics(context.Background(), &interfaces.AnalyticsData{
		LLMID: 8, AppID: 5, ModelName: "mock-fail503", Vendor: "openai",
		RequestID: "req-primary", StatusCode: 503, Timestamp: now,
	}, nil)
	require.NoError(t, err)

	require.Len(t, plugin.analyticsBuffer, 2)
	require.NotNil(t, plugin.analyticsBuffer[0].FailoverFromLLMID)
	assert.Equal(t, uint(8), *plugin.analyticsBuffer[0].FailoverFromLLMID)
	assert.Equal(t, 1, plugin.analyticsBuffer[0].FailoverAttempt)
	assert.Nil(t, plugin.analyticsBuffer[1].FailoverFromLLMID)

	pulse := plugin.buildPulseMessage(plugin.analyticsBuffer, plugin.analyticsMetadata, nil, nil, nil, nil, 1)
	require.NotNil(t, pulse)
	require.Len(t, pulse.AnalyticsEvents, 2)
	assert.Equal(t, uint32(8), pulse.AnalyticsEvents[0].FailoverFromLlmId)
	assert.Equal(t, uint32(1), pulse.AnalyticsEvents[0].FailoverAttempt)
	assert.Equal(t, uint32(0), pulse.AnalyticsEvents[1].FailoverFromLlmId)
	assert.Equal(t, uint32(0), pulse.AnalyticsEvents[1].FailoverAttempt)
}
