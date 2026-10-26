package proxy

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// forbiddenFlagger flags any text holding FORBIDDEN.
type forbiddenFlagger struct{}

func (forbiddenFlagger) Capabilities() guardrails.Capabilities { return guardrails.Capabilities{} }

func (forbiddenFlagger) Classify(_ context.Context, in guardrails.Input) (guardrails.Verdict, error) {
	v := guardrails.Verdict{}
	for _, s := range in.Segments {
		if strings.Contains(s.Text, "FORBIDDEN") {
			v.Flagged = true
			v.Findings = append(v.Findings, guardrails.Finding{Detector: "forbidden", Score: 1, SegmentIndex: s.Index})
		}
	}
	return v, nil
}

var registerForbidden sync.Once

func forbiddenResponseGuardrail() *models.Filter {
	registerForbidden.Do(func() {
		guardrails.RegisterSpec(guardrails.Spec{Name: "test_forbidden", DisplayName: "forbidden", OpenDetectors: true})
		guardrails.RegisterFactory("test_forbidden", func(guardrails.Config) (guardrails.Provider, error) { return forbiddenFlagger{}, nil })
	})
	return &models.Filter{Name: "forbidden", Kind: models.FilterKindGuardrail, ResponseFilter: true, Config: models.JSONMap{
		"provider": "test_forbidden", "detectors": []any{map[string]any{"name": "forbidden"}}, "on_detect": "block",
		"stream": map[string]any{"evaluate_every_chars": 1},
	}}
}

// dribble serves body in 7-byte writes, flushing each, so events arrive cut
// at arbitrary places.
func dribble(contentType, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		for i := 0; i < len(body); i += 7 {
			w.Write([]byte(body[i:min(i+7, len(body))]))
			w.(http.Flusher).Flush()
			time.Sleep(time.Millisecond)
		}
	}
}

type streamCase struct {
	name, contentType, path, reqBody string
	vendor                           models.Vendor
	body                             string
	// errMarker is in the vendor's error event.
	errMarker string
}

func streamCases() []streamCase {
	sse := "text/event-stream"
	return []streamCase{
		{"openai", sse, "/v1/chat/completions", `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`, models.OPENAI,
			"data: {\"choices\":[{\"delta\":{\"content\":\"safe \"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"FORB\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"IDDEN text\"}}]}\n\ndata: [DONE]\n\n",
			`"message":"Response blocked by filter`},
		{"responses", sse, "/v1/responses", `{"model":"gpt-4o","stream":true,"input":"hi"}`, models.OPENAI,
			"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"safe \"}\n\nevent: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"FORBIDDEN\"}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{}}\n\n",
			"event: error\ndata: {"},
		{"anthropic", sse, "/v1/messages", `{"model":"gpt-4o","stream":true,"max_tokens":5,"messages":[{"role":"user","content":"hi"}]}`, models.ANTHROPIC,
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"safe \"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"FORBIDDEN\"}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
			"event: error\ndata: {\"error\":"},
		{"google json array", "application/json", "/v1beta/models/gpt-4o:streamGenerateContent", `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`, models.GOOGLEAI,
			"[{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"safe \"}]}}]}\n,\r\n{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"FORBIDDEN\"}]}}]}\n]",
			`{"error":{"code":400`},
		{"ollama", "application/x-ndjson", "/api/chat", `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`, models.OLLAMA,
			"{\"message\":{\"content\":\"safe \"},\"done\":false}\n{\"message\":{\"content\":\"FORBIDDEN\"},\"done\":false}\n{\"done\":true}\n",
			`{"error":"Response blocked by filter`},
	}
}

// A response guardrail sees the text of every streamed format, however the
// events are cut, and a block ends the stream with the vendor's error
// event; the event that tripped it never reaches the client.
func TestStreamingGuardrail_BlocksEveryFormat(t *testing.T) {
	t.Parallel()
	for _, tc := range streamCases() {
		t.Run(tc.name, func(t *testing.T) {
			h := newFailoverHarness(t, dribble(tc.contentType, tc.body), serveOpenAIText("never"), func(primary, _ *models.LLM) {
				primary.Vendor = tc.vendor
				primary.Failover = models.LLMFailover{}
				primary.Filters = []*models.Filter{forbiddenResponseGuardrail()}
			})
			for _, route := range []string{"/llm/stream/primary", "/llm/call/primary", "/llm/rest/primary"} {
				resp, body := h.post(route+tc.path, tc.reqBody)
				require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", route, body)
				got := string(body)
				assert.NotContains(t, got, "FORBIDDEN", route)
				assert.Contains(t, got, "safe ", route)
				assert.Contains(t, got, tc.errMarker, "%s: %s", route, got)
			}
		})
	}
}

// A buffered Responses API answer's text reaches response guardrails (it has
// output items, not choices, and was let through unread).
func TestResponseGuardrail_BufferedResponsesText(t *testing.T) {
	t.Parallel()
	for _, text := range []string{"safe text", "FORBIDDEN text"} {
		body := `{"id":"resp_1","object":"response","status":"completed","model":"gpt-4o","output":[{"type":"message","id":"m1","role":"assistant","content":[{"type":"output_text","text":"` + text + `","annotations":[]}]}],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}`
		serve := func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}
		h := newFailoverHarness(t, serve, serveOpenAIText("never"), func(primary, _ *models.LLM) {
			primary.Failover = models.LLMFailover{}
			primary.Filters = []*models.Filter{forbiddenResponseGuardrail()}
		})
		resp, got := h.post("/llm/rest/primary/v1/responses", `{"model":"gpt-4o","input":"hi"}`)
		if strings.Contains(text, "FORBIDDEN") {
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, string(got))
			assert.NotContains(t, string(got), "FORBIDDEN")
		} else {
			assert.Equal(t, http.StatusOK, resp.StatusCode, string(got))
			assert.Contains(t, string(got), text)
		}
	}
}

// An upstream may send a stream with a Content-Length (a Gemini JSON array
// through some proxies). A block changes the body's length, so the client
// must not get the upstream's length: it would read a truncated body (or
// lose the error event) instead of the block.
func TestStreamingGuardrail_UpstreamContentLength(t *testing.T) {
	t.Parallel()
	for _, tc := range streamCases() {
		t.Run(tc.name, func(t *testing.T) {
			sized := func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.Header().Set("Content-Length", strconv.Itoa(len(tc.body)))
				_, _ = w.Write([]byte(tc.body))
			}
			h := newFailoverHarness(t, sized, serveOpenAIText("never"), func(primary, _ *models.LLM) {
				primary.Vendor = tc.vendor
				primary.Failover = models.LLMFailover{}
				primary.Filters = []*models.Filter{forbiddenResponseGuardrail()}
			})
			resp, body := h.post("/llm/stream/primary"+tc.path, tc.reqBody)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			assert.NotContains(t, string(body), "FORBIDDEN")
			assert.Contains(t, string(body), tc.errMarker, string(body))
		})
	}
}

// Without response filters a stream is relayed as before, byte for byte;
// with them and nothing to block, the client gets the same bytes, whole
// events at a time. (Responses streams are rewritten by the native chain,
// which changes their bytes with or without filters.)
func TestStreamingGuardrail_CleanStreamUnchanged(t *testing.T) {
	t.Parallel()
	for _, tc := range streamCases() {
		t.Run(tc.name, func(t *testing.T) {
			clean := strings.ReplaceAll(tc.body, "FORB", "PERM")
			run := func(filters []*models.Filter) string {
				h := newFailoverHarness(t, dribble(tc.contentType, clean), serveOpenAIText("never"), func(primary, _ *models.LLM) {
					primary.Vendor = tc.vendor
					primary.Failover = models.LLMFailover{}
					primary.Filters = filters
				})
				resp, body := h.post("/llm/stream/primary"+tc.path, tc.reqBody)
				require.Equal(t, http.StatusOK, resp.StatusCode)
				return string(body)
			}
			plain := run(nil)
			if tc.name != "responses" {
				assert.Equal(t, clean, plain)
			}
			assert.Equal(t, plain, run([]*models.Filter{forbiddenResponseGuardrail()}))
		})
	}
}

// Bedrock invoke-with-response-stream cannot be read, so with response
// filters it is refused rather than passed unfiltered.
func TestStreamingGuardrail_BedrockInvokeStreamRefused(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, dribble("application/vnd.amazon.eventstream", "x"), serveOpenAIText("never"), func(primary, _ *models.LLM) {
		primary.Vendor = models.BEDROCK
		primary.Failover = models.LLMFailover{}
		primary.Filters = []*models.Filter{forbiddenResponseGuardrail()}
	})
	resp, body := h.post("/llm/rest/primary/model/m/invoke-with-response-stream", `{"prompt":"hi"}`)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
	assert.Contains(t, string(body), "invoke-with-response-stream")
}
