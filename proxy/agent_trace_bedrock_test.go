package proxy

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	bedrockVendor "github.com/TykTechnologies/midsommar/v2/vendors/bedrock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureSink collects finished capture events.
type captureSink struct {
	mu     sync.Mutex
	events []*agenttrace.Event
}

func (s *captureSink) Enqueue(ev *agenttrace.Event) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
	return true
}

func (s *captureSink) llmEvents() []*agenttrace.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*agenttrace.Event
	for _, e := range s.events {
		if e.Kind == agenttrace.KindLLMInference {
			out = append(out, e)
		}
	}
	return out
}

func withCapture(t *testing.T) *captureSink {
	t.Helper()
	withAgentTracing(t, true)
	sink := &captureSink{}
	agenttrace.SetSink(sink)
	agenttrace.SetFingerprinter(agenttrace.StaticFingerprinter{ID: 1, Key: []byte("k")})
	t.Cleanup(func() { agenttrace.SetSink(nil); agenttrace.SetFingerprinter(nil) })
	return sink
}

func waitLLMEvent(t *testing.T, sink *captureSink) *agenttrace.Event {
	t.Helper()
	var evs []*agenttrace.Event
	require.Eventually(t, func() bool { evs = sink.llmEvents(); return len(evs) == 1 }, 5*time.Second, 10*time.Millisecond)
	return evs[0]
}

// assertBedrockToolCapture checks the llm_inference event of a Bedrock call
// that asked for the "send" tool.
func assertBedrockToolCapture(t *testing.T, ev *agenttrace.Event, stream bool) {
	t.Helper()
	assert.Equal(t, "aws.bedrock", ev.Provider)
	assert.Equal(t, "anthropic.claude-3", ev.RequestModel)
	assert.Equal(t, http.StatusOK, ev.HTTPStatus)
	require.Len(t, ev.Extracts.ToolCalls, 1, "the tool call is kept")
	tc := ev.Extracts.ToolCalls[0]
	assert.Equal(t, "tu1", tc.ID)
	assert.Equal(t, "send", tc.Name)
	assert.NotEmpty(t, tc.ArgsFP, "arguments are fingerprinted")
	assert.False(t, tc.ArgsUnavailable)
	assert.Contains(t, ev.Extracts.FinishReasons, "tool_use")
	assert.EqualValues(t, 10, ev.Usage.InputTokens)
	assert.EqualValues(t, 5, ev.Usage.OutputTokens)
	assert.NotZero(t, ev.Identity.AppID)
	if stream {
		assert.Positive(t, ev.TTFT, "time to first token")
		assert.True(t, ev.Extracts.Stream)
	}
	assert.NotEmpty(t, ev.Raw, "bodies go to the content pipeline")
}

// The native Bedrock stream keeps tool calls for capture and still logs
// the text exactly as before.
func TestAgentTraceBedrock_NativeStreamKeepsToolCalls(t *testing.T) {
	sink := withCapture(t)
	rec := &bedrockRecorder{reply: bedrockStream(t, "Sure. ", `{"to":"x"}`)}
	h := bedrockHarness(t, rec)
	resp, body := h.postBedrock("/llm/stream/primary/model/anthropic.claude-3/converse-stream", richConverse)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Equal(t, rec.reply, body, "frames still relayed byte for byte")

	ev := waitLLMEvent(t, sink)
	assertBedrockToolCapture(t, ev, true)
	require.Len(t, ev.Extracts.ToolResults, 1, "the request's trailing tool result is extracted")
	assert.Equal(t, "tu0", ev.Extracts.ToolResults[0].ID)

	logs := waitForLogs(t, h, 1)
	assert.Equal(t, "Sure. ", logs[0].ResponseBody, "the ProxyLog keeps today's text")
	assert.Len(t, logs[0].TraceSpanID, 16)
}

// waitBridgeCall waits for a bridge call's two llm_inference events: the
// logical call on the outer hop, and the inner /llm/call/ hop that answered,
// recorded as its attempt. Bedrock loops back as every other vendor does
// (8xf.12); the inner hop's event has the usage.
func waitBridgeCall(t *testing.T, sink *captureSink) (outer, inner *agenttrace.Event) {
	t.Helper()
	require.Eventually(t, func() bool {
		outer, inner = nil, nil
		for _, ev := range sink.llmEvents() {
			if ev.AttemptOf.IsZero() {
				outer = ev
			} else {
				inner = ev
			}
		}
		return outer != nil && inner != nil
	}, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, outer.SpanID, inner.AttemptOf)
	assert.Equal(t, "aws.bedrock", outer.Provider)
	assert.EqualValues(t, 1, outer.Attempt)
	return outer, inner
}

// The /ai/ bridge's Bedrock stream keeps tool calls too, on the inner hop
// and on the outer call.
func TestAgentTraceBedrock_AIStreamKeepsToolCalls(t *testing.T) {
	sink := withCapture(t)
	rec := &bedrockRecorder{reply: bedrockStream(t, "Sure. ", `{"to":"x"}`)}
	h := bedrockHarness(t, rec)
	resp, body := h.post("/ai/primary/v1/chat/completions",
		`{"model":"anthropic.claude-3","stream":true,"messages":[{"role":"user","content":"send it"}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Contains(t, string(body), `"tu1"`, "the client still gets the tool call")

	outer, inner := waitBridgeCall(t, sink)
	assertBedrockToolCapture(t, inner, true)
	require.Len(t, outer.Extracts.ToolCalls, 1)
	assert.Equal(t, "tu1", outer.Extracts.ToolCalls[0].ID)
	logs := waitForLogs(t, h, 1)
	assert.Equal(t, "Sure. ", logs[0].ResponseBody)
}

// The /anthropic/ bridge (Claude Code on Bedrock) keeps tool calls.
func TestAgentTraceBedrock_AnthropicStreamKeepsToolCalls(t *testing.T) {
	sink := withCapture(t)
	rec := &bedrockRecorder{reply: bedrockStream(t, "Sure. ", `{"to":"x"}`)}
	h := bedrockHarness(t, rec)
	resp, body := h.post("/anthropic/primary/v1/messages",
		`{"model":"claude-sonnet-4","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"send it"}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Contains(t, string(body), `"tu1"`)

	_, inner := waitBridgeCall(t, sink)
	assertBedrockToolCapture(t, inner, true)
}

// A non-streamed Converse answer through the /ai/ bridge: the tool call is
// read from the Converse output.
func TestAgentTraceBedrock_AIConverseKeepsToolCalls(t *testing.T) {
	sink := withCapture(t)
	converse := `{"output":{"message":{"role":"assistant","content":[{"text":"Sure. "},{"toolUse":{"toolUseId":"tu1","name":"send","input":{"to":"x"}}}]}},"stopReason":"tool_use","usage":{"inputTokens":10,"outputTokens":5,"totalTokens":15},"metrics":{"latencyMs":1}}`
	h := newFailoverHarness(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(converse))
	}, serveOpenAIText("never"), func(primary, _ *models.LLM) {
		primary.Vendor = models.BEDROCK
		primary.Failover = models.LLMFailover{}
		primary.DefaultModel = "anthropic.claude-3"
		primary.Metadata = models.JSONMap{"aws_access_key_id": "AKIDTEST", "aws_secret_access_key": "secret", "aws_region": "us-east-1"}
	})
	bedrockVendor.InvalidateClientCache(h.primary.ID)
	t.Cleanup(func() { bedrockVendor.InvalidateClientCache(h.primary.ID) })

	resp, body := h.post("/ai/primary/v1/chat/completions",
		`{"model":"anthropic.claude-3","messages":[{"role":"user","content":"send it"}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	outer, inner := waitBridgeCall(t, sink)
	assertBedrockToolCapture(t, inner, false)
	require.Len(t, outer.Extracts.ToolCalls, 1)
}

// With tracing off nothing is captured and the paths behave as before.
func TestAgentTraceBedrock_OffCapturesNothing(t *testing.T) {
	withAgentTracing(t, false)
	sink := &captureSink{}
	agenttrace.SetSink(sink)
	t.Cleanup(func() { agenttrace.SetSink(nil) })
	rec := &bedrockRecorder{reply: bedrockStream(t, "Sure. ", `{"to":"x"}`)}
	h := bedrockHarness(t, rec)
	resp, body := h.postBedrock("/llm/stream/primary/model/anthropic.claude-3/converse-stream", richConverse)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	waitForLogs(t, h, 1)
	assert.Empty(t, sink.llmEvents())
}
