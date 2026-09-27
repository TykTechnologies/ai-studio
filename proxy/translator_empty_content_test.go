package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serveAnthropicEmpty answers like api.anthropic.com does when a turn ends
// before the model wrote anything: content is an empty array. It happens at a
// tiny max_tokens (claude-sonnet-5 answers "hi" at max_tokens 1 this way) and
// on an immediate end_turn, and it is a successful response, not an error.
func serveAnthropicEmpty(stopReason string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Stream bool `json:"stream"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		if !req.Stream {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","content":[],` +
				`"model":"claude-sonnet-5","stop_reason":"` + stopReason + `","stop_sequence":null,` +
				`"usage":{"input_tokens":8,"output_tokens":1}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, ev := range []string{
			`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5","content":[],"stop_reason":null,"usage":{"input_tokens":8,"output_tokens":1}}}`,
			`{"type":"message_delta","delta":{"stop_reason":"` + stopReason + `","stop_sequence":null},"usage":{"output_tokens":1}}`,
			`{"type":"message_stop"}`,
		} {
			var typed struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal([]byte(ev), &typed)
			_, _ = w.Write([]byte("event: " + typed.Type + "\ndata: " + ev + "\n\n"))
			flusher.Flush()
		}
	}
}

// An empty Anthropic turn must reach an OpenAI client as a normal, empty
// completion with the mapped finish_reason, not as a 502 "no response".
func TestTranslator_AnthropicEmptyContent(t *testing.T) {
	for _, c := range []struct {
		stopReason string
		finish     string
	}{
		{"max_tokens", "length"},
		{"end_turn", "stop"},
	} {
		t.Run(c.stopReason, func(t *testing.T) {
			h := newEndpointShapeHarness(t, models.ANTHROPIC, "", serveAnthropicEmpty(c.stopReason))
			for _, surface := range []struct{ name, path, model string }{
				{"ai bridge", "/ai/shape-route/v1/chat/completions", "claude-sonnet-5"},
				{"unified", "/v1/chat/completions", "shape-route/claude-sonnet-5"},
			} {
				t.Run(surface.name+"/buffered", func(t *testing.T) {
					resp, raw := h.post(surface.path,
						`{"model":"`+surface.model+`","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`)
					body := string(raw)
					require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
					var got struct {
						Choices []struct {
							Message struct {
								Role    string  `json:"role"`
								Content *string `json:"content"`
							} `json:"message"`
							FinishReason string `json:"finish_reason"`
						} `json:"choices"`
						Usage struct {
							PromptTokens     int `json:"prompt_tokens"`
							CompletionTokens int `json:"completion_tokens"`
						} `json:"usage"`
					}
					require.NoError(t, json.Unmarshal([]byte(body), &got), body)
					require.Len(t, got.Choices, 1, body)
					assert.Equal(t, "assistant", got.Choices[0].Message.Role, body)
					if got.Choices[0].Message.Content != nil {
						assert.Empty(t, *got.Choices[0].Message.Content, body)
					}
					assert.Equal(t, c.finish, got.Choices[0].FinishReason, body)
					// The turn was billed: its usage must reach the client.
					assert.Equal(t, 8, got.Usage.PromptTokens, body)
					assert.Equal(t, 1, got.Usage.CompletionTokens, body)
				})
				t.Run(surface.name+"/stream", func(t *testing.T) {
					resp, raw := h.post(surface.path,
						`{"model":"`+surface.model+`","stream":true,"max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`)
					body := string(raw)
					require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
					assert.NotContains(t, body, `"error"`, body)
					assert.Contains(t, body, `"finish_reason":"`+c.finish+`"`, body)
					assert.True(t, strings.HasSuffix(strings.TrimSpace(body), "data: [DONE]"), body)
				})
			}
		})
	}
}
