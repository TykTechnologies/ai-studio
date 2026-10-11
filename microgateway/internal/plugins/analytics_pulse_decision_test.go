package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/gatewayplugin/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The model class and decision counts survive the plugin hand-off and reach
// the pulse; without them the hub would file edge decision traffic as chat.
func TestAnalyticsPulsePlugin_DecisionFieldsReachPulse(t *testing.T) {
	plugin := &AnalyticsPulsePlugin{
		config:        &PulsePluginConfig{MaxBufferSize: 100},
		edgeID:        "test-edge",
		edgeNamespace: "test",
		lastPulseTime: time.Now().Add(-time.Minute),
	}

	now := time.Now()
	_, err := plugin.HandleAnalytics(context.Background(), &interfaces.AnalyticsData{
		LLMID: 7, AppID: 5, ModelName: "jev-1.13.0", Vendor: "systemone",
		RequestID: "req-decision", StatusCode: 200, Timestamp: now,
		PromptTokens: 200, Choices: 2,
		RequestKind: "decision", DecisionQuestions: 3, DecisionRefusals: 1,
	}, nil)
	require.NoError(t, err)
	_, err = plugin.HandleAnalytics(context.Background(), &interfaces.AnalyticsData{
		LLMID: 8, AppID: 5, ModelName: "gpt-4o", Vendor: "openai",
		RequestID: "req-chat", StatusCode: 200, Timestamp: now,
	}, nil)
	require.NoError(t, err)

	pulse := plugin.buildPulseMessage(plugin.analyticsBuffer, plugin.analyticsMetadata, nil, nil, nil, nil, 1)
	require.Len(t, pulse.AnalyticsEvents, 2)
	assert.Equal(t, "decision", pulse.AnalyticsEvents[0].RequestKind)
	assert.Equal(t, uint32(3), pulse.AnalyticsEvents[0].DecisionQuestions)
	assert.Equal(t, uint32(1), pulse.AnalyticsEvents[0].DecisionRefusals)
	assert.Equal(t, uint32(2), pulse.AnalyticsEvents[0].Choices)
	assert.Equal(t, "", pulse.AnalyticsEvents[1].RequestKind)
}
