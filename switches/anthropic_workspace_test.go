package switches

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Chat sessions, the translation bridges and semantic routing call Anthropic
// through the langchaingo driver; it must run in the LLM's pinned workspace
// too, on the direct path and on the internal-client (loopback) path.
func TestFetchDriver_AnthropicWorkspacePin(t *testing.T) {
	var mu sync.Mutex
	var seen []http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Clone())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-luna",` +
			`"content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer upstream.Close()

	call := func(t *testing.T, ws string, opts ...FetchDriverOption) http.Header {
		t.Helper()
		llm := &models.LLM{Vendor: models.ANTHROPIC, APIKey: "k", APIEndpoint: upstream.URL, VendorWorkspaceID: ws}
		driver, err := FetchDriver(llm, &models.LLMSettings{ModelName: "claude-luna"}, nil, nil, opts...)
		require.NoError(t, err)
		_, err = driver.GenerateContent(context.Background(),
			[]llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "hello")}, llms.WithMaxTokens(10))
		require.NoError(t, err)
		mu.Lock()
		defer mu.Unlock()
		require.NotEmpty(t, seen)
		return seen[len(seen)-1]
	}

	for name, opts := range map[string][]FetchDriverOption{
		"direct":          nil,
		"internal client": {WithHTTPClient(&http.Client{})},
	} {
		t.Run(name, func(t *testing.T) {
			h := call(t, "wrkspc_pinned", opts...)
			assert.Equal(t, []string{"wrkspc_pinned"}, h.Values("anthropic-workspace-id"))
			assert.Equal(t, "k", h.Get("x-api-key"))
			assert.Empty(t, call(t, "", opts...).Values("anthropic-workspace-id"), "no pin, no header")
		})
	}
}
