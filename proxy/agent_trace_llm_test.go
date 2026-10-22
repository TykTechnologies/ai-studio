package proxy

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const toolCallReply = `{"id":"chatcmpl-9","object":"chat.completion","created":1,"model":"gpt-4o",` +
	`"choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"search","arguments":"{\"q\":\"x\"}"}}]},"finish_reason":"tool_calls"}],` +
	`"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19}}`

func serveToolCall(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(toolCallReply))
}

func serveStreamedToolCall(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	f := w.(http.Flusher)
	for _, d := range []string{
		`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"search","arguments":""}}]}}]}`,
		`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q\":\"x\"}"}}]}}]}`,
		`{"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19}}`,
	} {
		fmt.Fprintf(w, "data: %s\n\n", d)
		f.Flush()
		time.Sleep(2 * time.Millisecond)
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
}

const toolResultRequest = `{"model":"gpt-4o","messages":[{"role":"user","content":"find x"},` +
	`{"role":"assistant","tool_calls":[{"id":"call_0","type":"function","function":{"name":"search","arguments":"{}"}}]},` +
	`{"role":"tool","tool_call_id":"call_0","content":"found"}]}`

func assertToolCallEvent(t *testing.T, ev *agenttrace.Event, h *failoverHarness) {
	t.Helper()
	assert.Equal(t, "openai", ev.Provider)
	assert.Equal(t, h.app.ID, ev.Identity.AppID)
	assert.Equal(t, http.StatusOK, ev.HTTPStatus)
	require.Len(t, ev.Extracts.ToolCalls, 1)
	assert.Equal(t, "call_1", ev.Extracts.ToolCalls[0].ID)
	assert.Equal(t, "search", ev.Extracts.ToolCalls[0].Name)
	assert.NotEmpty(t, ev.Extracts.ToolCalls[0].ArgsFP)
	require.Len(t, ev.Extracts.ToolResults, 1, "the request's trailing tool result")
	assert.Equal(t, "call_0", ev.Extracts.ToolResults[0].ID)
	assert.Contains(t, ev.Extracts.FinishReasons, "tool_calls")
	assert.NotEmpty(t, ev.Raw)

	logs := waitForLogs(t, h, 1)
	assert.Equal(t, ev.SpanID.String(), logs[0].TraceSpanID, "trace_span_id equals the ProxyLog's")
	var rec models.LLMChatRecord
	require.Eventually(t, func() bool { return h.db.Where("app_id = ?", h.app.ID).First(&rec).Error == nil }, 5*time.Second, 20*time.Millisecond)
	assert.EqualValues(t, rec.PromptTokens, ev.Usage.InputTokens, "usage is the chat record's")
	assert.EqualValues(t, rec.ResponseTokens, ev.Usage.OutputTokens)
	assert.EqualValues(t, int64(rec.Cost*100), ev.Usage.CostMicros)
}

func TestAgentTraceLLM_RESTCapture(t *testing.T) {
	sink := withCapture(t)
	h := newFailoverHarness(t, serveToolCall, serveOpenAIText("never"), nil)
	resp, body := h.post("/llm/rest/primary/v1/chat/completions", toolResultRequest)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assertToolCallEvent(t, waitLLMEvent(t, sink), h)
}

func TestAgentTraceLLM_StreamCapture(t *testing.T) {
	sink := withCapture(t)
	h := newFailoverHarness(t, serveStreamedToolCall, serveOpenAIText("never"), nil)
	resp, body := h.post("/llm/stream/primary/v1/chat/completions", `{"stream":true,`+toolResultRequest[1:])
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	ev := waitLLMEvent(t, sink)
	assertToolCallEvent(t, ev, h)
	assert.True(t, ev.Extracts.Stream)
	assert.Positive(t, ev.TTFT)
}

// An /ai/ call that fails over is one logical llm_inference event on the
// outer hop; the inner hops are events marked as its attempts.
func TestAgentTraceLLM_AIFailoverIsOneLogicalCall(t *testing.T) {
	sink := withCapture(t)
	h := newFailoverHarness(t, serveStatus(503), serveStreamedToolCall, nil) // the bridge's driver streams from the vendor
	withTools := `{"tools":[{"type":"function","function":{"name":"search","parameters":{"type":"object"}}}],` + toolResultRequest[1:]
	resp, body := h.post("/ai/primary/v1/chat/completions", withTools)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	var outer *agenttrace.Event
	var attempts []*agenttrace.Event
	require.Eventually(t, func() bool {
		outer, attempts = nil, nil
		for _, ev := range sink.llmEvents() {
			if ev.AttemptOf.IsZero() {
				outer = ev
			} else {
				attempts = append(attempts, ev)
			}
		}
		return outer != nil && len(attempts) == 2
	}, 5*time.Second, 20*time.Millisecond)
	assert.EqualValues(t, 2, outer.Attempt)
	assert.Equal(t, "gpt-4o", outer.ResponseModel)
	require.Len(t, outer.Extracts.ToolCalls, 1, "client body: %s", body)
	for _, a := range attempts {
		assert.Equal(t, outer.SpanID, a.AttemptOf)
	}
}

// The streamed /ai/ bridge records its logical call too.
func TestAgentTraceLLM_AIStreamFailover(t *testing.T) {
	sink := withCapture(t)
	h := newFailoverHarness(t, serveStatus(503), serveStreamedToolCall, nil)
	withTools := `{"stream":true,"tools":[{"type":"function","function":{"name":"search","parameters":{"type":"object"}}}],` + toolResultRequest[1:]
	resp, body := h.post("/ai/primary/v1/chat/completions", withTools)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	var outer *agenttrace.Event
	require.Eventually(t, func() bool {
		for _, ev := range sink.llmEvents() {
			if ev.AttemptOf.IsZero() {
				outer = ev
			}
		}
		return outer != nil
	}, 5*time.Second, 20*time.Millisecond)
	assert.EqualValues(t, 2, outer.Attempt)
	require.Len(t, outer.Extracts.ToolCalls, 1)
	assert.Equal(t, "call_1", outer.Extracts.ToolCalls[0].ID)
}

func TestAgentTraceLLM_OffCapturesNothing(t *testing.T) {
	withAgentTracing(t, false)
	sink := &captureSink{}
	agenttrace.SetSink(sink)
	t.Cleanup(func() { agenttrace.SetSink(nil) })
	h := newFailoverHarness(t, serveToolCall, serveOpenAIText("never"), nil)
	resp, _ := h.post("/llm/rest/primary/v1/chat/completions", toolResultRequest)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	waitForLogs(t, h, 1)
	assert.Empty(t, sink.llmEvents())
}
