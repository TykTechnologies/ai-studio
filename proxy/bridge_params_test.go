package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// The OpenAI-shaped bridges (/v1, /ai/) dropped stop (as a list), top_p,
// service_tier, user, store, parallel_tool_calls, logprobs, top_logprobs,
// prompt_cache_key and safety_identifier on the way to the vendor
// (midsommar-clean-6bg5.23). OpenAI now receives all ten as sent; the other
// vendors receive stop and top_p in their own shape, and the parameters only
// OpenAI has are not sent to them.

const bridgeAllParams = `"service_tier":"flex","user":"u1","store":true,"parallel_tool_calls":false,` +
	`"prompt_cache_key":"pk","safety_identifier":"sid","logprobs":true,"top_logprobs":2,` +
	`"top_p":0.9,"stop":["x","y"]`

func bridgeParamBody(model, params string) string {
	body := `{"model":"` + model + `","max_tokens":20,"temperature":0.2,"messages":[{"role":"user","content":"hi"}]`
	if params != "" {
		body += "," + params
	}
	return body + `}`
}

func keysOfRaw(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestBridgeParams_OpenAIReceivesThemUnchanged(t *testing.T) {
	capture := &capturingVendor{serve: serveOpenAIText("ok")}
	h := newFailoverHarness(t, capture.handler, serveOpenAIText("never"), noFailover)

	// Without the parameters the vendor request is what it was.
	resp, out := h.post("/ai/primary/v1/chat/completions", bridgeParamBody("gpt-4o", ""))
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	baseline := keysOfRaw(capture.last(t))
	assert.Equal(t, []string{"max_completion_tokens", "messages", "model", "stream", "stream_options", "temperature"}, baseline)

	resp, out = h.post("/ai/primary/v1/chat/completions", bridgeParamBody("gpt-4o", bridgeAllParams))
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	sent := capture.last(t)
	for field, want := range map[string]string{
		"service_tier": `"flex"`, "user": `"u1"`, "store": `true`, "parallel_tool_calls": `false`,
		"prompt_cache_key": `"pk"`, "safety_identifier": `"sid"`, "logprobs": `true`, "top_logprobs": `2`,
		"top_p": `0.9`, "stop": `["x","y"]`,
	} {
		if assert.Contains(t, sent, field, "%s was dropped", field) {
			assert.JSONEq(t, want, string(sent[field]), field)
		}
	}
	// And nothing else changed.
	assert.Len(t, sent, len(baseline)+10)

	// A single stop string still works.
	resp, out = h.post("/ai/primary/v1/chat/completions", bridgeParamBody("gpt-4o", `"stop":"END"`))
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	assert.JSONEq(t, `["END"]`, string(capture.last(t)["stop"]))
}

// serveOpenAITier answers a streamed chat completion whose chunks carry the
// service tier the call was served at, as OpenAI does.
func serveOpenAITier(tier string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, chunk := range []string{
			`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-4o","service_tier":"` + tier + `","choices":[{"index":0,"delta":{"role":"assistant","content":"ok"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-4o","service_tier":"` + tier + `","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"gpt-4o","service_tier":"` + tier + `","choices":[],"usage":{"prompt_tokens":6,"completion_tokens":5,"total_tokens":11}}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}
}

// service_tier reaches OpenAI, the tier it answers with is reported to the
// client, and the call is recorded with it.
func TestBridgeParams_ServiceTierIsServedReportedAndRecorded(t *testing.T) {
	capture := &capturingVendor{serve: serveOpenAITier("flex")}
	h := newFailoverHarness(t, capture.handler, serveOpenAIText("never"), noFailover)

	resp, out := h.post("/ai/primary/v1/chat/completions", bridgeParamBody("gpt-4o", `"service_tier":"flex"`))
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	assert.JSONEq(t, `"flex"`, string(capture.last(t)["service_tier"]))
	var answer struct {
		ServiceTier string `json:"service_tier"`
	}
	require.NoError(t, json.Unmarshal(out, &answer), string(out))
	assert.Equal(t, "flex", answer.ServiceTier, "the client is told the tier it was served at")

	var recs []models.LLMChatRecord
	require.Eventually(t, func() bool { recs = h.chatRecords(); return len(recs) == 1 }, 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, "flex", recs[0].ServiceTier, "the call is recorded with its tier")
}

// The other vendors get stop and top_p in their own shape, and none of the
// parameters only OpenAI has.
func TestBridgeParams_OtherVendors(t *testing.T) {
	openAIOnly := []string{"service_tier", "user", "store", "parallel_tool_calls", "prompt_cache_key", "safety_identifier", "logprobs", "top_logprobs"}

	t.Run("anthropic", func(t *testing.T) {
		capture := &capturingVendor{serve: serveAnthropic(t)}
		h := newFailoverHarness(t, capture.handler, serveOpenAIText("never"), func(primary, _ *models.LLM) {
			primary.Vendor = models.ANTHROPIC
			primary.Failover = models.LLMFailover{}
			primary.DefaultModel = "claude-sonnet-5"
		})
		resp, out := h.post("/ai/primary/v1/chat/completions", bridgeParamBody("claude-sonnet-5", bridgeAllParams))
		require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
		sent := capture.last(t)
		assert.JSONEq(t, `["x","y"]`, string(sent["stop_sequences"]))
		assert.JSONEq(t, `0.9`, string(sent["top_p"]))
		for _, field := range openAIOnly {
			assert.NotContains(t, sent, field)
		}
	})

	t.Run("google", func(t *testing.T) {
		capture := &capturingVendor{serve: serveGeminiAnyShape}
		h := newFailoverHarness(t, capture.handler, serveOpenAIText("never"), func(primary, _ *models.LLM) {
			primary.Vendor = models.GOOGLEAI
			primary.Failover = models.LLMFailover{}
			primary.DefaultModel = "gemini-2.5-flash"
		})
		resp, out := h.post("/ai/primary/v1/chat/completions", bridgeParamBody("gemini-2.5-flash", bridgeAllParams))
		require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
		var sent struct {
			GenerationConfig struct {
				StopSequences []string `json:"stopSequences"`
				TopP          float64  `json:"topP"`
			} `json:"generationConfig"`
		}
		capture.mu.Lock()
		body := capture.bodies[len(capture.bodies)-1]
		capture.mu.Unlock()
		require.NoError(t, json.Unmarshal(body, &sent), string(body))
		assert.Equal(t, []string{"x", "y"}, sent.GenerationConfig.StopSequences)
		assert.InDelta(t, 0.9, sent.GenerationConfig.TopP, 1e-6)
		for _, field := range openAIOnly {
			assert.NotContains(t, string(body), `"`+field+`":`)
		}
	})

	t.Run("bedrock", func(t *testing.T) {
		h, v := converseHarness(t, false)
		resp, out := h.post("/ai/primary/v1/chat/completions", bridgeParamBody("anthropic.claude-3", bridgeAllParams))
		require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
		calls := v.calls()
		require.Len(t, calls, 1)
		var sent struct {
			InferenceConfig struct {
				StopSequences []string `json:"stopSequences"`
				TopP          float64  `json:"topP"`
			} `json:"inferenceConfig"`
		}
		require.NoError(t, json.Unmarshal(calls[0].Body, &sent), string(calls[0].Body))
		assert.Equal(t, []string{"x", "y"}, sent.InferenceConfig.StopSequences)
		assert.InDelta(t, 0.9, sent.InferenceConfig.TopP, 1e-6)
		for _, field := range openAIOnly {
			assert.NotContains(t, string(calls[0].Body), `"`+field+`":`)
		}
	})
}
