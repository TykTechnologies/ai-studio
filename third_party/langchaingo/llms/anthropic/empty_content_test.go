package anthropic

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

// Anthropic answers content: [] when a turn ends before the model writes
// anything: at a tiny max_tokens, or on an immediate end_turn. That is a
// successful response. It must come back as one empty choice that keeps the
// stop reason and the usage, not as ErrEmptyResponse.
func TestGenerateContent_EmptyContentIsAnEmptyChoice(t *testing.T) {
	const buffered = `{"id":"msg_1","type":"message","role":"assistant","content":[],` +
		`"model":"claude-sonnet-5","stop_reason":"max_tokens","stop_sequence":null,` +
		`"usage":{"input_tokens":8,"output_tokens":1}}`
	streamed := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5","content":[],"stop_reason":null,"usage":{"input_tokens":8,"output_tokens":1}}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"max_tokens","stop_sequence":null},"usage":{"output_tokens":1}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
		``,
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"stream":true`) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(streamed))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(buffered))
	}))
	defer srv.Close()

	llm, err := New(WithToken("test"), WithBaseURL(srv.URL+"/v1"), WithModel("claude-sonnet-5"))
	if err != nil {
		t.Fatal(err)
	}
	msgs := []llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "hi")}

	for _, tc := range []struct {
		name string
		opts []llms.CallOption
	}{
		{"buffered", []llms.CallOption{llms.WithMaxTokens(1)}},
		{"streaming", []llms.CallOption{llms.WithMaxTokens(1),
			llms.WithStreamingFunc(func(context.Context, []byte) error { return nil })}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := llm.GenerateContent(context.Background(), msgs, tc.opts...)
			if err != nil {
				t.Fatalf("GenerateContent: %v", err)
			}
			if len(resp.Choices) != 1 {
				t.Fatalf("want 1 choice, got %d", len(resp.Choices))
			}
			c := resp.Choices[0]
			if c.Content != "" || len(c.ToolCalls) != 0 {
				t.Errorf("want an empty choice, got content %q and %d tool calls", c.Content, len(c.ToolCalls))
			}
			if c.StopReason != "max_tokens" {
				t.Errorf("stop reason = %q, want max_tokens", c.StopReason)
			}
			if got := c.GenerationInfo["InputTokens"]; got != 8 {
				t.Errorf("InputTokens = %v, want 8", got)
			}
			if got := c.GenerationInfo["OutputTokens"]; got != 1 {
				t.Errorf("OutputTokens = %v, want 1", got)
			}
		})
	}
}
