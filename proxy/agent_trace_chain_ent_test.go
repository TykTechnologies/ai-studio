//go:build enterprise

package proxy

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// The capture keeps what the request filters let through, never what a
// redacting filter removed, and still reads the chain's ids the way the
// client sent them, so R3 links (jyx.154).
func TestAgentTraceChain_FilteredBodyClientIDs(t *testing.T) {
	sink := withCapture(t)
	h := newFailoverHarness(t, serveResponsesChain, serveOpenAIText("never"), func(primary, _ *models.LLM) {
		primary.Failover = models.LLMFailover{}
		primary.Filters = []*models.Filter{{Name: "redact", Script: []byte(redactingFilterScript)}}
	})

	resp, body := h.post("/llm/rest/primary/v1/responses", `{"model":"gpt-4o","input":"summarise"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	var first struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(body, &first))

	resp, body = h.post("/llm/rest/primary/v1/responses",
		`{"model":"gpt-4o","previous_response_id":"`+first.ID+`","input":"my SECRET-123 please"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	calls := h.primaryVendor.calls()
	require.Len(t, calls, 2)
	require.Contains(t, string(calls[1].Body), "[REDACTED]", "the filter ran")

	var evs []*agenttrace.Event
	require.Eventually(t, func() bool { evs = sink.llmEvents(); return len(evs) == 2 }, 5*time.Second, 10*time.Millisecond)
	var chained *agenttrace.Event
	for _, e := range evs {
		if e.Extracts.PreviousResponseID != "" {
			chained = e
		}
	}
	require.NotNil(t, chained)
	assert.Equal(t, first.ID, chained.Extracts.PreviousResponseID, "R3 keys match")
	var req []byte
	for _, p := range chained.Raw {
		if p.Part == agenttrace.PartRequest {
			req = p.Data
		}
	}
	require.NotEmpty(t, req)
	assert.NotContains(t, string(req), "SECRET-123", "what the filter removed is not captured")
	assert.Contains(t, string(req), "[REDACTED]")
	assert.Contains(t, string(req), first.ID, "the client's wrapped id")
	assert.NotContains(t, string(req), `"resp_chain_1"`, "never the vendor's id")
}
