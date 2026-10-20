//go:build enterprise

package proxy

import (
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A request filter block is recorded as a filter gateway event; the denial
// still answers exactly as before, and the request's span is an errored LLM
// call of the authenticated App.
func TestAgentTraceEvents_RequestFilterBlock(t *testing.T) {
	sink := withCapture(t)
	h := newFailoverHarness(t, serveOpenAIText("never"), serveOpenAIText("never"), func(primary, _ *models.LLM) {
		primary.Failover = models.LLMFailover{}
		primary.Filters = []*models.Filter{{Name: "guard", Script: []byte(`output := {block: true, message: "stop"}`)}}
	})
	resp, body := h.post("/llm/rest/primary/v1/chat/completions", failoverChatBody)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
	assert.Empty(t, h.primaryVendor.calls(), "blocked before the vendor")

	ev := waitLLMEvent(t, sink)
	assert.Equal(t, agenttrace.StatusError, ev.Status)
	assert.Equal(t, http.StatusBadRequest, ev.HTTPStatus)
	assert.Equal(t, h.app.ID, ev.Identity.AppID, "the denial is attributed to its App")
	blocks := gatewayEvents(ev, agenttrace.GatewayEventFilter)
	require.Len(t, blocks, 1)
	assert.Equal(t, agenttrace.OutcomeBlock, blocks[0].Outcome)
	assert.Equal(t, "request_filter", blocks[0].Name)
}

