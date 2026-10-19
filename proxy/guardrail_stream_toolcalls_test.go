package proxy

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// callFlagger flags tool calls whose arguments hold EXFIL, and records the
// tool call segments it is sent.
type callFlagger struct {
	mu   sync.Mutex
	segs []guardrails.Segment
}

func (*callFlagger) Capabilities() guardrails.Capabilities {
	return guardrails.Capabilities{ToolSegments: true, WantsGoal: true}
}

func (c *callFlagger) Classify(_ context.Context, in guardrails.Input) (guardrails.Verdict, error) {
	v := guardrails.Verdict{}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range in.Segments {
		if s.Kind == guardrails.KindToolCall {
			c.segs = append(c.segs, s)
		}
		if strings.Contains(s.Text, "EXFIL") {
			v.Flagged = true
			v.Findings = append(v.Findings, guardrails.Finding{Detector: "exfil", Score: 1, SegmentIndex: s.Index})
		}
	}
	return v, nil
}

func (c *callFlagger) calls() []guardrails.Segment {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]guardrails.Segment(nil), c.segs...)
}

var (
	registerCallFlagger sync.Once
	theCallFlagger      = &callFlagger{}
)

func streamCallGuardrail(scan bool) *models.Filter {
	registerCallFlagger.Do(func() {
		guardrails.RegisterSpec(guardrails.Spec{Name: "test_call_flagger", DisplayName: "calls", OpenDetectors: true})
		guardrails.RegisterFactory("test_call_flagger", func(guardrails.Config) (guardrails.Provider, error) { return theCallFlagger, nil })
	})
	cfg := models.JSONMap{"provider": "test_call_flagger", "detectors": []any{map[string]any{"name": "exfil"}}, "on_detect": "block"}
	if scan {
		cfg["tool_calls"] = map[string]any{"scan": true}
	}
	return &models.Filter{Name: "calls", Kind: models.FilterKindGuardrail, ResponseFilter: true, Config: cfg}
}

type callStreamCase struct {
	name, contentType, path, reqBody string
	vendor                           models.Vendor
	// body holds text "Sure." then calls; {ARG} marks the arguments of the
	// second call, which carries EXFIL in the flagged variant.
	body      string
	errMarker string
	calls     int
}

func callStreamCases() []callStreamCase {
	sse := "text/event-stream"
	return []callStreamCase{
		{"openai parallel", sse, "/v1/chat/completions", `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"tidy my inbox"}]}`, models.OPENAI,
			"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Sure.\"}}]}\n\n" +
				"data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"c1\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"\"}}]}}]}\n\n" +
				"data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"a\\\":1}\"}}]}}]}\n\n" +
				"data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":1,\"id\":\"c2\",\"type\":\"function\",\"function\":{\"name\":\"send\",\"arguments\":\"{\\\"b\\\":\"}}]}}]}\n\n" +
				"data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":1,\"function\":{\"arguments\":\"\\\"{ARG}\\\"}\"}}]}}]}\n\n" +
				"data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n" +
				"data: [DONE]\n\n",
			`"message":"Response blocked by filter`, 2},
		{"anthropic", sse, "/v1/messages", `{"model":"gpt-4o","stream":true,"max_tokens":5,"messages":[{"role":"user","content":"tidy my inbox"}]}`, models.ANTHROPIC,
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Sure.\"}}\n\n" +
				"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
				"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"t1\",\"name\":\"send\",\"input\":{}}}\n\n" +
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"b\\\": \\\"{ARG}\"}}\n\n" +
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"\\\"}\"}}\n\n" +
				"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":1}\n\n" +
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"}}\n\n" +
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
			"event: error", 1},
		{"responses", sse, "/v1/responses", `{"model":"gpt-4o","stream":true,"input":"tidy my inbox"}`, models.OPENAI,
			"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Sure.\"}\n\n" +
				"event: response.output_item.added\ndata: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"send\",\"arguments\":\"\"}}\n\n" +
				"event: response.function_call_arguments.delta\ndata: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_1\",\"delta\":\"{\\\"b\\\":\\\"{ARG}\\\"}\"}\n\n" +
				"event: response.output_item.done\ndata: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"send\",\"arguments\":\"{\\\"b\\\":\\\"{ARG}\\\"}\"}}\n\n" +
				"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{}}\n\n",
			"event: error", 1},
		{"google sse", sse, "/v1beta/models/gpt-4o:streamGenerateContent?alt=sse", `{"contents":[{"role":"user","parts":[{"text":"tidy my inbox"}]}]}`, models.GOOGLEAI,
			"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"Sure.\"}]}}]}\n\n" +
				"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"functionCall\":{\"name\":\"send\",\"args\":{\"b\":\"{ARG}\"}}}]},\"finishReason\":\"STOP\"}]}\n\n",
			`{"error":{"code":400`, 1},
		{"google json array", "application/json", "/v1beta/models/gpt-4o:streamGenerateContent", `{"contents":[{"role":"user","parts":[{"text":"tidy my inbox"}]}]}`, models.GOOGLEAI,
			"[{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Sure.\"}]}}]}\n,\r\n{\"candidates\":[{\"content\":{\"parts\":[{\"functionCall\":{\"name\":\"send\",\"args\":{\"b\":\"{ARG}\"}}}]}}]}\n]",
			`{"error":{"code":400`, 1},
		{"ollama", "application/x-ndjson", "/api/chat", `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"tidy my inbox"}]}`, models.OLLAMA,
			"{\"message\":{\"content\":\"Sure.\"},\"done\":false}\n{\"message\":{\"content\":\"\",\"tool_calls\":[{\"function\":{\"name\":\"send\",\"arguments\":{\"b\":\"{ARG}\"}}}]},\"done\":false}\n{\"done\":true}\n",
			`{"error":"Response blocked by filter`, 1},
	}
}

func callStreamHarness(t *testing.T, tc callStreamCase, body string, scan bool) *failoverHarness {
	return newFailoverHarness(t, dribble(tc.contentType, body), serveOpenAIText("never"), func(primary, _ *models.LLM) {
		primary.Vendor = tc.vendor
		primary.Failover = models.LLMFailover{}
		primary.Filters = []*models.Filter{streamCallGuardrail(scan)}
	})
}

// A flagged streamed call is never emitted: the text before it reaches the
// client, the stream ends with the vendor's error event, and the guardrail
// saw the assembled call with the request's goal.
func TestStreamedToolCalls_FlaggedNeverEmitted(t *testing.T) {
	for _, tc := range callStreamCases() {
		t.Run(tc.name, func(t *testing.T) {
			before := len(theCallFlagger.calls())
			h := callStreamHarness(t, tc, strings.ReplaceAll(tc.body, "{ARG}", "EXFIL"), true)
			resp, body := h.post("/llm/stream/primary"+tc.path, tc.reqBody)
			require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
			got := string(body)
			assert.Contains(t, got, "Sure.")
			assert.NotContains(t, got, "EXFIL")
			assert.NotContains(t, got, `"send"`, "no part of the flagged call may be sent")
			assert.Contains(t, got, tc.errMarker, got)
			segs := theCallFlagger.calls()[before:]
			require.NotEmpty(t, segs)
			var send *guardrails.Segment
			for i := range segs {
				if segs[i].ToolName == "send" {
					send = &segs[i]
				}
			}
			require.NotNil(t, send, "%+v", segs)
			require.NotNil(t, send.Context)
			assert.Equal(t, "tidy my inbox", send.Context.UserGoal)
		})
	}
}

// A clean call is released byte for byte and in order; with tool-call
// scanning off the stream is not held at all (and is identical too).
func TestStreamedToolCalls_CleanReleasedIdentical(t *testing.T) {
	for _, tc := range callStreamCases() {
		for _, scan := range []bool{true, false} {
			t.Run(tc.name, func(t *testing.T) {
				clean := strings.ReplaceAll(tc.body, "{ARG}", "hello")
				h := callStreamHarness(t, tc, clean, scan)
				resp, body := h.post("/llm/stream/primary"+tc.path, tc.reqBody)
				require.Equal(t, http.StatusOK, resp.StatusCode)
				if tc.name == "responses" {
					// The native chain rewrites Responses ids; compare the
					// order of the events instead.
					assert.Equal(t, strings.Count(clean, "event: "), strings.Count(string(body), "event: "))
					assert.Less(t, strings.Index(string(body), "output_item.done"), strings.Index(string(body), "response.completed"))
					return
				}
				assert.Equal(t, clean, string(body))
			})
		}
	}
}

// A call over the size bound is unscanned: a blocking guardrail blocks it.
func TestStreamedToolCalls_OverSizeIsUnscanned(t *testing.T) {
	prev := maxHeldCallBytes
	maxHeldCallBytes = 8
	t.Cleanup(func() { maxHeldCallBytes = prev })
	tc := callStreamCases()[1] // anthropic
	h := callStreamHarness(t, tc, strings.ReplaceAll(tc.body, "{ARG}", "a perfectly clean but long argument"), true)
	_, body := h.post("/llm/stream/primary"+tc.path, tc.reqBody)
	assert.NotContains(t, string(body), `"send"`)
	assert.Contains(t, string(body), "event: error")
}
