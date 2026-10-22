package proxy

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// postBridge posts to a Bedrock bridge. It used to retry a stream that
// failed before its first frame: midsommar-clean-q0z.26 found the SDK's
// connection closed under load by its own request-body handling
// (vendors/bedrock sdkHTTPClient) and events dropped when the stream's error
// was checked per event, both fixed.
func postBridge(t *testing.T, h *failoverHarness, path, body string, headers ...string) (*http.Response, []byte) {
	t.Helper()
	return h.post(path, body, headers...)
}

// The /ai/ and /anthropic/ bridges to Bedrock re-encode the Converse stream
// as OpenAI or Anthropic SSE. Response guardrails see its text per event and
// in a final pass, and hold its tool calls; they never ran there before.
func TestBedrockBridgeStreams_Guardrails(t *testing.T) {
	type bridge struct {
		name, path, body, auth, errMarker string
	}
	bridges := []bridge{
		{"ai", "/ai/primary/v1/chat/completions", `{"model":"anthropic.claude-3","stream":true,"messages":[{"role":"user","content":"tidy my inbox"}]}`, "Authorization", `"message":"Response blocked by filter`},
		{"anthropic", "/anthropic/primary/v1/messages", `{"model":"anthropic.claude-3","stream":true,"max_tokens":64,"messages":[{"role":"user","content":"tidy my inbox"}]}`, "x-api-key", "event: error"},
	}
	post := func(h *failoverHarness, b bridge) string {
		value := h.apiKey
		if b.auth == "Authorization" {
			value = "Bearer " + h.apiKey
		}
		resp, body := postBridge(t, h, b.path, b.body, b.auth, value)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		return string(body)
	}
	for _, b := range bridges {
		t.Run(b.name+" text block", func(t *testing.T) {
			rec := &bedrockRecorder{reply: bedrockStream(t, "FORBIDDEN text", `{"to":"x"}`)}
			got := post(bedrockHarness(t, rec, forbiddenResponseGuardrail()), b)
			assert.NotContains(t, got, "FORBIDDEN")
			assert.Contains(t, got, b.errMarker, got)
		})
		t.Run(b.name+" tool call block", func(t *testing.T) {
			rec := &bedrockRecorder{reply: bedrockStream(t, "Sure. ", `{"to":"EXFIL"}`)}
			got := post(bedrockHarness(t, rec, streamCallGuardrail(true)), b)
			assert.Contains(t, got, "Sure. ")
			assert.NotContains(t, got, "EXFIL")
			assert.NotContains(t, got, `"send"`)
			assert.Contains(t, got, b.errMarker, got)
		})
		t.Run(b.name+" clean call released", func(t *testing.T) {
			rec := &bedrockRecorder{reply: bedrockStream(t, "Sure. ", `{"to":"x"}`)}
			got := post(bedrockHarness(t, rec, streamCallGuardrail(true)), b)
			assert.Contains(t, got, `"send"`)
			assert.Contains(t, got, `to`)
			assert.NotContains(t, got, "blocked by filter")
		})
		t.Run(b.name+" final pass", func(t *testing.T) {
			// Split across two text deltas, each innocent alone at the
			// default cadence of a remote provider (1000 characters).
			f := forbiddenResponseGuardrail()
			f.Config["stream"] = map[string]any{"evaluate_every_chars": 100000}
			rec := &bedrockRecorder{reply: bedrockStream(t, "FORBIDDEN", "{}")}
			got := post(bedrockHarness(t, rec, f), b)
			assert.Contains(t, got, b.errMarker, "the end-of-stream pass must block: %s", got)
		})
	}
}

// A Converse stream that breaks (here a corrupt frame after the first text
// event) ends the bridged stream with the vendor's error event, not a clean
// [DONE] or message_stop that reads as a complete, empty answer.
func TestBedrockBridgeStreams_StreamErrorReachesTheClient(t *testing.T) {
	good := bedrockStream(t, "Sure. ", `{"to":"x"}`)
	first := bedrockFrame(t, "messageStart", map[string]any{"role": "assistant"})
	first = append(first, bedrockFrame(t, "contentBlockDelta", map[string]any{"contentBlockIndex": 0, "delta": map[string]any{"text": "Sure. "}})...)
	broken := append(append([]byte{}, first...), []byte("\x00\x00\x00\x30\x00\x00\x00\x10garbage-not-a-frame-at-all-xxxxxxxxxxxxxxxxxxxx")...)
	_ = good
	for _, filtered := range []bool{false, true} {
		var filters []*models.Filter
		if filtered {
			filters = []*models.Filter{forbiddenResponseGuardrail()}
		}
		rec := &bedrockRecorder{reply: broken}
		h := bedrockHarness(t, rec, filters...)
		resp, body := postBridge(t, h, "/ai/primary/v1/chat/completions", `{"model":"anthropic.claude-3","stream":true,"messages":[{"role":"user","content":"hi"}]}`, "Authorization", "Bearer "+h.apiKey)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		got := string(body)
		assert.Contains(t, got, "Sure. ", "filtered=%v: %s", filtered, got)
		assert.Contains(t, got, `"error":{`, "filtered=%v: %s", filtered, got)
		assert.NotContains(t, got, `"finish_reason":"stop"`, got)

		rec = &bedrockRecorder{reply: broken}
		h = bedrockHarness(t, rec, filters...)
		resp, body = postBridge(t, h, "/anthropic/primary/v1/messages", `{"model":"anthropic.claude-3","stream":true,"max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`, "x-api-key", h.apiKey)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		got = string(body)
		assert.Contains(t, got, "event: error", "filtered=%v: %s", filtered, got)
		assert.NotContains(t, got, "event: message_stop", got)
	}
}

// Without response filters the bridges stream as before.
func TestBedrockBridgeStreams_NoFilters(t *testing.T) {
	rec := &bedrockRecorder{reply: bedrockStream(t, "Sure. ", `{"to":"x"}`)}
	h := bedrockHarness(t, rec)
	resp, body := postBridge(t, h, "/ai/primary/v1/chat/completions", `{"model":"anthropic.claude-3","stream":true,"messages":[{"role":"user","content":"hi"}]}`, "Authorization", "Bearer "+h.apiKey)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	got := string(body)
	assert.Contains(t, got, "Sure. ")
	assert.Contains(t, got, `"send"`)
	assert.True(t, strings.HasSuffix(got, "data: [DONE]\n\n"))
	_ = models.BEDROCK
}
