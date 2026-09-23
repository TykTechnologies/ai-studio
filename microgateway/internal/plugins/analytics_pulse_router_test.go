package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every field of a routing decision must survive the plugin hand-off and
// reach the pulse, or the hub's ProxyLog loses it. The Semantic Router's
// score and shadow route were once dropped here while the edge had them.
func TestAnalyticsPulsePlugin_RoutingDecisionReachesPulse(t *testing.T) {
	plugin := &AnalyticsPulsePlugin{
		config:        &PulsePluginConfig{MaxBufferSize: 100},
		edgeID:        "test-edge",
		edgeNamespace: "test",
		lastPulseTime: time.Now().Add(-time.Minute),
	}
	_, err := plugin.HandleAnalytics(context.Background(), &interfaces.AnalyticsData{
		LLMID: 7, AppID: 5, ModelName: "gpt-4o-mini", Vendor: "openai", RequestID: "req-routed",
		StatusCode: 200, Timestamp: time.Now(),
		RouterKind: "semantic_router", RouterSlug: "smart", RouterPool: "cheap",
		Route: "chat", RouteReason: "embedding",
		RouterSourceModel: "auto", RouterTargetModel: "gpt-4o-mini", RouterSelectionAlgo: "round_robin",
		RouteScore: 0.91, ShadowRoute: "complex",
	}, nil)
	require.NoError(t, err)

	pulse := plugin.buildPulseMessage(plugin.analyticsBuffer, plugin.analyticsMetadata, nil, nil, nil, nil, 1)
	require.Len(t, pulse.AnalyticsEvents, 1)
	ev := pulse.AnalyticsEvents[0]
	assert.Equal(t, "semantic_router", ev.RouterKind)
	assert.Equal(t, "smart", ev.RouterSlug)
	assert.Equal(t, "cheap", ev.RouterPool)
	assert.Equal(t, "chat", ev.Route)
	assert.Equal(t, "embedding", ev.RouteReason)
	assert.Equal(t, "auto", ev.RouteSourceModel)
	assert.Equal(t, "gpt-4o-mini", ev.RouteTargetModel)
	assert.Equal(t, "round_robin", ev.RouteSelection)
	assert.InDelta(t, 0.91, ev.RouteScore, 1e-9)
	assert.Equal(t, "complex", ev.ShadowRoute)
}
