package anthropicVendor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
)

func TestDriverBaseURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"https://api.anthropic.com", "https://api.anthropic.com/v1"},
		{"https://api.anthropic.com/", "https://api.anthropic.com/v1"},
		{"https://api.anthropic.com/v1", "https://api.anthropic.com/v1"},
		{"https://api.anthropic.com/v1/", "https://api.anthropic.com/v1"},
		{"https://gw.example.com/anthropic", "https://gw.example.com/anthropic/v1"},
		{"https://gw.example.com/anthropic/v1", "https://gw.example.com/anthropic/v1"},
		{"https://gw.example.com/v1/acct/anthropic", "https://gw.example.com/v1/acct/anthropic/v1"},
		{"http://127.0.0.1:8080/llm/call/claude/v1", "http://127.0.0.1:8080/llm/call/claude/v1"},
		{"https://gw.example.com/v2beta", "https://gw.example.com/v2beta"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, DriverBaseURL(tc.in))
		})
	}
}

// The driver is what Studio chat, agents and the OpenAI-compatible bridge use.
// Whatever shape the operator configured, the vendor must receive /v1/messages.
func TestSetupAnthropicDriver_CallsMessagesUnderV1ForEveryEndpointShape(t *testing.T) {
	for _, suffix := range []string{"", "/", "/v1", "/v1/", "/anthropic", "/anthropic/v1"} {
		t.Run("endpoint suffix "+suffix, func(t *testing.T) {
			var gotPath string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant",` +
					`"content":[{"type":"text","text":"ok"}],"model":"claude-sonnet-5",` +
					`"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
			}))
			defer upstream.Close()

			driver, err := setupAnthropicDriver(
				&models.LLM{APIEndpoint: upstream.URL + suffix, APIKey: "sk-ant-test"},
				&models.LLMSettings{ModelName: "claude-sonnet-5"})
			require.NoError(t, err)

			_, err = driver.GenerateContent(context.Background(),
				[]llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "hi")})
			require.NoError(t, err)

			want := "/v1/messages"
			if len(suffix) > 0 && suffix[:min(len(suffix), 10)] == "/anthropic" {
				want = "/anthropic/v1/messages"
			}
			assert.Equal(t, want, gotPath)
		})
	}
}
