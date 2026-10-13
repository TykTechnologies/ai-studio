package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
	openaiVendor "github.com/TykTechnologies/midsommar/v2/vendors/openai"
)

// serveInputCalls answers embeddings, completions and moderations.
func (f *fakeResponsesAPI) serveInputCalls(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/v1/embeddings":
		_, _ = w.Write([]byte(`{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.1]}],"model":"text-embedding-3-small","usage":{"prompt_tokens":3,"total_tokens":3}}`))
	case "/v1/completions":
		_, _ = w.Write([]byte(`{"id":"cmpl-1","object":"text_completion","model":"gpt-3.5-turbo-instruct","choices":[{"text":"ok","index":0}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`))
	case "/v1/moderations":
		_, _ = w.Write([]byte(`{"id":"modr-1","model":"omni-moderation-latest","results":[{"flagged":false}]}`))
	default:
		return false
	}
	return true
}

func inputView(t *testing.T, body string) []string {
	t.Helper()
	msgs, err := (&OpenAIInputMessageExtractor{}).ExtractMessages(httptest.NewRequest("POST", "/v1/embeddings", nil), []byte(body))
	require.NoError(t, err)
	out := make([]string, len(msgs))
	for i, m := range msgs {
		assert.Equal(t, llms.ChatMessageTypeHuman, m.Role)
		out[i] = m.Parts[0].(llms.TextContent).Text
	}
	return out
}

func TestOpenAIInputExtractor(t *testing.T) {
	assert.Equal(t, requestFilterFormatOpenAIInput, openaiVendor.InputFilterFormat)
	assert.Equal(t, []string{"hello"}, inputView(t, `{"model":"m","input":"hello"}`))
	assert.Equal(t, []string{"a", "b"}, inputView(t, `{"model":"m","input":["a","b"]}`))
	assert.Equal(t, []string{"p", "s"}, inputView(t, `{"model":"m","prompt":"p","suffix":"s"}`))
	assert.Equal(t, []string{"p1", "p2"}, inputView(t, `{"model":"m","prompt":["p1","p2"]}`))
	assert.Equal(t, []string{"look"}, inputView(t, `{"model":"m","input":[{"type":"text","text":"look"},{"type":"image_url","image_url":{"url":"u"}}]}`))
	assert.Empty(t, inputView(t, `{"model":"m"}`))

	for name, b := range map[string]string{
		"token array":     `{"input":[1,2,3]}`,
		"token arrays":    `{"input":[[1,2],[3]]}`,
		"prompt tokens":   `{"prompt":[1,2]}`,
		"number":          `{"input":5}`,
		"unknown part":    `{"input":[{"type":"audio","data":"x"}]}`,
		"text not string": `{"input":[{"type":"text","text":5}]}`,
		"not json":        `nope`,
	} {
		_, err := (&OpenAIInputMessageExtractor{}).ExtractMessages(nil, []byte(b))
		assert.Error(t, err, name)
	}
	reg := NewMessageExtractorRegistry()
	reg.Register(&OpenAIInputMessageExtractor{})
	assert.True(t, reg.FailsClosed(requestFilterFormatOpenAIInput))

	out, err := (&OpenAIInputMessageReconstructor{}).Reconstruct([]map[string]interface{}{
		{"role": "user", "content": "a [REDACTED]"}, {"role": "user", "content": "b"},
	}, []byte(`{"model":"m","input":["a SECRET","b"],"dimensions":256}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"m","input":["a [REDACTED]","b"],"dimensions":256}`, string(out))
	out, err = (&OpenAIInputMessageReconstructor{}).Reconstruct([]map[string]interface{}{{"role": "user", "content": "x"}},
		[]byte(`{"input":[{"type":"text","text":"SECRET"},{"type":"image_url","image_url":{"url":"u"}}]}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"input":[{"type":"text","text":"x"},{"type":"image_url","image_url":{"url":"u"}}]}`, string(out))
	_, err = (&OpenAIInputMessageReconstructor{}).Reconstruct(nil, []byte(`{"input":"x"}`))
	assert.Error(t, err)
}

// Without filters, traffic on these endpoints is unchanged, token arrays
// included.
func TestOpenAIInputCalls_NoFiltersUnchanged(t *testing.T) {
	h := newResponsesHarness(t)
	for path, body := range map[string]string{
		"/llm/rest/primary/v1/embeddings":  `{"model":"text-embedding-3-small","input":[1,2,3]}`,
		"/llm/rest/primary/v1/completions": `{"model":"gpt-3.5-turbo-instruct","prompt":"hi"}`,
		"/llm/rest/primary/v1/moderations": `{"model":"omni-moderation-latest","input":"hi"}`,
	} {
		resp, out := h.do(h.keyC, http.MethodPost, path, body)
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", path, out)
	}
}
