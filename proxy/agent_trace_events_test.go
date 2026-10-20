package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gatewayEvents(ev *agenttrace.Event, typ string) []agenttrace.GatewayEvent {
	var out []agenttrace.GatewayEvent
	for _, ge := range ev.GatewayEvents {
		if ge.Type == typ {
			out = append(out, ge)
		}
	}
	return out
}

// An /ai/ failover rung the planner skips is recorded as a fallback skip.
func TestAgentTraceEvents_FailoverSkip(t *testing.T) {
	withAgentTracing(t, true)
	h := newFailoverHarness(t, serveStatus(503), serveOpenAIText("never"), func(primary, fallback *models.LLM) {
		fallback.AllowedModels = []string{"other-model"}
	})
	req := httptest.NewRequest(http.MethodPost, "/ai/primary/v1/chat/completions", nil)
	ctx, rec := agenttrace.Begin(context.Background())
	h.proxy.planFailover(h.primary, "gpt-4o", req.WithContext(ctx))
	var got []agenttrace.GatewayEvent
	rec.Update(func(ev *agenttrace.Event) { got = ev.GatewayEvents })
	require.Len(t, got, 1)
	assert.Equal(t, agenttrace.GatewayEventFallback, got[0].Type)
	assert.Equal(t, agenttrace.OutcomeSkip, got[0].Outcome)
	assert.Contains(t, got[0].Detail, "allowed models")
}

func TestRecordPolicyBlockAndDetailBound(t *testing.T) {
	withAgentTracing(t, true)
	ctx, rec := agenttrace.Begin(context.Background())
	recordPolicyBlock(ctx, "budget", "budget", string(make([]byte, 1000)))
	rec.Update(func(ev *agenttrace.Event) {
		require.Len(t, ev.GatewayEvents, 1)
		assert.Equal(t, agenttrace.GatewayEventBudget, ev.GatewayEvents[0].Type)
		assert.Len(t, ev.GatewayEvents[0].Detail, maxEventDetail)
		assert.Equal(t, agenttrace.StatusError, ev.Status)
		assert.Equal(t, "policy_budget", ev.ErrorType)
	})
}

// With tracing off a policy block allocates nothing beyond today's metric.
func TestRecordPolicyBlockOffAllocatesNothing(t *testing.T) {
	withAgentTracing(t, false)
	ctx := context.Background()
	if n := testing.AllocsPerRun(100, func() {
		recordGatewayEvent(ctx, agenttrace.GatewayEvent{Type: agenttrace.GatewayEventRouter})
		captureApp(ctx, nil)
	}); n != 0 {
		t.Fatalf("allocates %.0f", n)
	}
}
