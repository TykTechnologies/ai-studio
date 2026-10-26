package proxy

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// A request's capture is finished after the response, once its analysis is
// done, and must go to the agent trace Sink that was installed when the
// request arrived, not to one installed in between. midsommar-clean-vrc.19:
// a serial test's late tool_call capture landed in the next test's sink
// (TestAgentTraceTool_MCPStreamableCapture saw two events). Here the Sink is
// replaced while the upstream answers.
func TestCaptureGoesToTheSinkInstalledAtRequestStart(t *testing.T) {
	atStart := withCapture(t)
	replacement := &captureSink{}
	answer := serveOpenAIText("hi")
	h := newFailoverHarness(t, func(w http.ResponseWriter, r *http.Request) {
		agenttrace.SetSink(replacement) // the next test installs its own
		answer(w, r)
	}, serveOpenAIText("never"), noFailover)

	resp, body := h.post("/llm/rest/primary/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	h.proxy.waitForAnalyzers()

	require.Len(t, atStart.llmEvents(), 1, "the capture goes to the Sink installed when the request arrived")
	replacement.mu.Lock()
	defer replacement.mu.Unlock()
	require.Empty(t, replacement.events, "nothing lands in the Sink installed afterwards")
}
