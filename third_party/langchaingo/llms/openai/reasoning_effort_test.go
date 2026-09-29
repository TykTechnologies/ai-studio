package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reasoning_effort is sent verbatim when the caller set it and is absent from
// the request otherwise: values and defaults differ per model, so the client
// never supplies one of its own.
func TestReasoningEffort(t *testing.T) {
	t.Parallel()

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got = nil
		_ = json.Unmarshal(raw, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","object":"chat.completion","created":1,"model":"gpt-5",` +
			`"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],` +
			`"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	llm, err := New(WithToken("test-key"), WithBaseURL(server.URL), WithModel("gpt-5"))
	require.NoError(t, err)
	msgs := []llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "hi")}

	for _, effort := range []string{"none", "minimal", "high", "xhigh"} {
		_, err = llm.GenerateContent(context.Background(), msgs, WithReasoningEffort(effort))
		require.NoError(t, err)
		assert.Equal(t, effort, got["reasoning_effort"])
	}

	_, err = llm.GenerateContent(context.Background(), msgs)
	require.NoError(t, err)
	assert.NotContains(t, got, "reasoning_effort")

	_, err = llm.GenerateContent(context.Background(), msgs, WithReasoningEffort(""))
	require.NoError(t, err)
	assert.NotContains(t, got, "reasoning_effort")
	assert.NotContains(t, got, "metadata", "the option never leaks into the request metadata")
}
