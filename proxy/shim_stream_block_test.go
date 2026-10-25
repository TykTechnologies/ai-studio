package proxy

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// shimCase is an upstream vendor behind /ai/: its native stream, with a
// FORBIDDEN text event, and with a tool call whose arguments are {ARG}.
type shimCase struct {
	name        string
	vendor      models.Vendor
	contentType string
	text        string
	toolCall    string
}

func shimCases() []shimCase {
	sse := "text/event-stream"
	return []shimCase{
		{"openai", models.OPENAI, sse,
			"data: {\"id\":\"x\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"safe \"}}]}\n\n" +
				"data: {\"id\":\"x\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"FORBIDDEN\"}}]}\n\n" +
				"data: {\"id\":\"x\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n",
			"data: {\"id\":\"x\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"c1\",\"type\":\"function\",\"function\":{\"name\":\"send\",\"arguments\":\"{\\\"b\\\":\\\"{ARG}\\\"}\"}}]}}]}\n\n" +
				"data: {\"id\":\"x\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n"},
		{"anthropic", models.ANTHROPIC, sse,
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"gpt-4o\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n" +
				"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"safe \"}}\n\n" +
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"FORBIDDEN\"}}\n\n" +
				"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n" +
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"gpt-4o\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n" +
				"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"t1\",\"name\":\"send\",\"input\":{}}}\n\n" +
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"b\\\":\\\"{ARG}\\\"}\"}}\n\n" +
				"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":2}}\n\n" +
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"},
		{"google", models.GOOGLEAI, "application/json",
			"[{\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"safe \"}]}}]}\n,\r\n" +
				"{\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"FORBIDDEN\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":2}}\n]",
			"[{\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"functionCall\":{\"name\":\"send\",\"args\":{\"b\":\"{ARG}\"}}}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":2}}\n]"},
	}
}

const shimRequest = `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"tidy my inbox"}],"tools":[{"type":"function","function":{"name":"send","parameters":{"type":"object","properties":{"b":{"type":"string"}}}}}]}`

func shimHarness(t *testing.T, tc shimCase, body string, filters ...*models.Filter) *failoverHarness {
	return newFailoverHarness(t, dribble(tc.contentType, body), serveOpenAIText("never"), func(primary, _ *models.LLM) {
		primary.Vendor = tc.vendor
		primary.Failover = models.LLMFailover{}
		primary.Filters = filters
	})
}

func shimPost(t *testing.T, h *failoverHarness) (int, string) {
	t.Helper()
	resp, body := h.post("/ai/primary/v1/chat/completions", shimRequest)
	return resp.StatusCode, string(body)
}

// assertBlocked checks a blocked /ai/ answer: before any frame an HTTP 400
// policy error, after frames an error chunk and [DONE]; the block message,
// and never a clean finish.
func assertBlocked(t *testing.T, status int, got string) {
	t.Helper()
	assert.Contains(t, got, "blocked by policy", got)
	assert.Contains(t, got, `"policy_violation"`, got)
	assert.NotContains(t, got, `"finish_reason":"stop"`, got)
	assert.NotContains(t, got, `"finish_reason":"tool_calls"`, got)
	switch status {
	case http.StatusOK:
		assert.True(t, strings.HasSuffix(got, "data: [DONE]\n\n"), got)
	case http.StatusBadRequest:
		assert.True(t, strings.HasPrefix(got, `{"error":`), got)
	default:
		t.Errorf("status %d: %s", status, got)
	}
}

// A mid-stream block on the inner hop reaches the /ai/ client as an OpenAI
// error chunk and [DONE], never a clean finish.
func TestShimStream_InnerBlockReachesTheClient(t *testing.T) {
	t.Parallel()
	for _, tc := range shimCases() {
		t.Run(tc.name, func(t *testing.T) {
			status, got := shimPost(t, shimHarness(t, tc, tc.text, forbiddenResponseGuardrail()))
			assert.NotContains(t, got, "FORBIDDEN")
			assertBlocked(t, status, got)
		})
	}
}

// A block in the inner hop's final pass is visible on the outer stream.
func TestShimStream_FinalPassBlockReachesTheClient(t *testing.T) {
	t.Parallel()
	for _, tc := range shimCases() {
		t.Run(tc.name, func(t *testing.T) {
			f := forbiddenResponseGuardrail()
			f.Config["stream"] = map[string]any{"evaluate_every_chars": 100000}
			status, got := shimPost(t, shimHarness(t, tc, tc.text, f))
			assertBlocked(t, status, got)
		})
	}
}

// The unified /v1 route goes through the same bridge: a block there reaches
// the client too, and so does one on a non-streaming /ai/ request (the
// bridge streams its inner hop either way).
func TestShimStream_UnifiedRouteAndNonStreaming(t *testing.T) {
	t.Parallel()
	tc := shimCases()[0]
	h := shimHarness(t, tc, strings.ReplaceAll(tc.toolCall, "{ARG}", "EXFIL"), streamCallGuardrail(true))
	resp, body := h.post("/v1/chat/completions", strings.Replace(shimRequest, `"model":"gpt-4o"`, `"model":"primary/gpt-4o"`, 1))
	assert.NotContains(t, string(body), "EXFIL")
	assertBlocked(t, resp.StatusCode, string(body))

	h = shimHarness(t, tc, tc.text, forbiddenResponseGuardrail())
	resp, body = h.post("/ai/primary/v1/chat/completions", strings.Replace(shimRequest, `"stream":true`, `"stream":false`, 1))
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
	assert.NotContains(t, string(body), "FORBIDDEN")
	assert.Contains(t, string(body), "blocked by policy")
}

// A flagged tool call is never emitted on /ai/; a clean one is.
func TestShimStream_ToolCalls(t *testing.T) {
	t.Parallel()
	for _, tc := range shimCases() {
		t.Run(tc.name, func(t *testing.T) {
			status, got := shimPost(t, shimHarness(t, tc, strings.ReplaceAll(tc.toolCall, "{ARG}", "EXFIL"), streamCallGuardrail(true)))
			assert.NotContains(t, got, "EXFIL")
			assert.NotContains(t, got, `"send"`)
			assertBlocked(t, status, got)

			status, got = shimPost(t, shimHarness(t, tc, strings.ReplaceAll(tc.toolCall, "{ARG}", "hello"), streamCallGuardrail(true)))
			require.Equal(t, http.StatusOK, status, got)
			assert.Contains(t, got, `"send"`, got)
			assert.Contains(t, got, "hello", got)
			assert.NotContains(t, got, `"error":{`, got)
		})
	}
}
