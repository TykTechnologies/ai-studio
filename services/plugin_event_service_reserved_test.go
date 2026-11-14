package services

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	pb "github.com/TykTechnologies/midsommar/v2/proto/plugin_events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Studio plugin cannot raise a guardrail alert or outcome event: only
// PublishGuardrailEvents does, after checking the event against the hub's
// configuration (midsommar-clean-9u1j.55, .66). Other topics still publish.
func TestPluginEventServer_GuardrailAlertTopicRefused(t *testing.T) {
	bus := eventbridge.NewBus()
	got := make(chan string, 4)
	sub := bus.SubscribeAll(func(ev eventbridge.Event) { got <- ev.Topic })
	defer bus.Unsubscribe(sub)
	server := NewPluginEventServer(bus, "hub")

	for _, topic := range []string{"guardrail.alert.visor-pr-flag", "guardrail.outcome.blocked", "guardrail.outcome.judge_softened", "credential.changed"} {
		resp, err := server.Publish(context.Background(), &pb.PublishRequest{Topic: topic, Payload: []byte(`{"alert":"visor-pr-flag"}`), PluginId: "3"})
		require.NoError(t, err)
		assert.False(t, resp.Success, topic)
		assert.Contains(t, resp.ErrorMessage, "AI Studio only")
	}

	resp, err := server.Publish(context.Background(), &pb.PublishRequest{Topic: "metrics.report", Payload: []byte(`{}`), PluginId: "3"})
	require.NoError(t, err)
	assert.True(t, resp.Success)
	select {
	case topic := <-got:
		assert.Equal(t, "metrics.report", topic)
	case <-time.After(time.Second):
		t.Fatal("the allowed topic was not published")
	}
	select {
	case topic := <-got:
		t.Fatalf("unexpected topic %s", topic)
	case <-time.After(50 * time.Millisecond):
	}
}
