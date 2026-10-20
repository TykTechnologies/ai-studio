package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// serveResponsesChain answers OpenAI's POST /v1/responses: the first call
// creates resp_chain_1, a call naming previous_response_id creates
// resp_chain_2.
func serveResponsesChain(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	id := "resp_chain_1"
	if strings.Contains(string(body), "previous_response_id") {
		id = "resp_chain_2"
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"id":%q,"object":"response","created_at":1,"status":"completed","model":"gpt-4o",`+
		`"output":[{"type":"message","id":"msg_%s","status":"completed","role":"assistant","content":[{"type":"output_text","text":"noted","annotations":[]}]}],`+
		`"usage":{"input_tokens":20,"output_tokens":8,"total_tokens":28}}`, id, id)
}

// The capture reads a Responses chain in one id space, the client's: the
// response id as the client got it (wrapped by the native chain hook) and
// the next request's previous_response_id as the client sent it, so R3 can
// link them (jyx.147).
func TestAgentTraceChain_ResponseIDsInTheClientsSpace(t *testing.T) {
	sink := withCapture(t)
	h := newFailoverHarness(t, serveResponsesChain, serveOpenAIText("never"), nil)

	resp, body := h.post("/llm/rest/primary/v1/responses", `{"model":"gpt-4o","input":"summarise"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	var first struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(body, &first))
	require.NotEqual(t, "resp_chain_1", first.ID, "the client gets a wrapped id")

	resp, body = h.post("/llm/rest/primary/v1/responses", `{"model":"gpt-4o","previous_response_id":"`+first.ID+`","input":"shorter"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	calls := h.primaryVendor.calls()
	require.Len(t, calls, 2)
	assert.Contains(t, string(calls[1].Body), `"resp_chain_1"`, "the vendor gets its own id")

	var evs []*agenttrace.Event
	require.Eventually(t, func() bool { evs = sink.llmEvents(); return len(evs) == 2 }, 5*time.Second, 10*time.Millisecond)
	byPrev := map[bool]*agenttrace.Event{}
	for _, e := range evs {
		byPrev[e.Extracts.PreviousResponseID != ""] = e
	}
	require.NotNil(t, byPrev[false])
	require.NotNil(t, byPrev[true])
	assert.Equal(t, first.ID, byPrev[false].Extracts.ResponseID)
	assert.Equal(t, first.ID, byPrev[true].Extracts.PreviousResponseID, "R3 keys match")
}
