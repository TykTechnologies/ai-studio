package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/requestformat"
)

// parityBody is an agent-sized request body: its parse is what matters.
func parityBody(t testing.TB, fields map[string]any) []byte {
	t.Helper()
	body := map[string]any{"messages": []map[string]string{{"role": "user", "content": strings.Repeat("lorem ipsum ", 4096)}}}
	for k, v := range fields {
		body[k] = v
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)
	return b
}

// validatedRequest runs r through the model validation middleware, as every
// /llm/rest/, /llm/stream/ and /llm/call/ request is, and returns the request
// the handler gets.
func validatedRequest(t *testing.T, vendor models.Vendor, extractor ModelNameExtractor, path string, body []byte) *http.Request {
	t.Helper()
	p := &Proxy{llms: map[string]*models.LLM{"llm": {Name: "LLM", Vendor: vendor}}}
	p.modelValidator = NewModelValidator(nil)
	p.modelValidator.RegisterExtractor(string(vendor), extractor)
	var got *http.Request
	h := p.modelValidationMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r }))
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"llmSlug": "llm"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.NotNil(t, got, "middleware refused: %d %s", rec.Code, rec.Body.String())
	return got
}

// TestExchangeMetadata_ParseParityWithV222 pins how often a request body is
// parsed for the exchange metadata (53e.45): never. v2.2.2 parsed a request
// in the model extractor and the vendor's screening only; the metadata reuses
// what the extractor decoded, and on Bedrock's and Gemini's native paths the
// path decides both, as their APIs do. One parse costs what
// requestformat.Model costs; without one, withExchangeMetadata allocates only
// its map and context.
func TestExchangeMetadata_ParseParityWithV222(t *testing.T) {
	for _, tc := range []struct {
		name      string
		vendor    models.Vendor
		extractor ModelNameExtractor
		path      string
		body      map[string]any
		want      map[string]string
	}{
		{"openai rest", models.OPENAI, OpenAIModelExtractor, "/llm/rest/llm/v1/chat/completions",
			map[string]any{"model": "gpt-4o"}, map[string]string{"model": "gpt-4o", "stream": "false"}},
		{"openai stream", models.OPENAI, OpenAIModelExtractor, "/llm/stream/llm/v1/chat/completions",
			map[string]any{"model": "gpt-4o", "stream": true}, map[string]string{"model": "gpt-4o", "stream": "true"}},
		{"ollama", models.OLLAMA, OpenAIModelExtractor, "/llm/rest/llm/api/chat",
			map[string]any{"model": "llama3", "stream": true}, map[string]string{"model": "llama3", "stream": "true"}},
		{"anthropic", models.ANTHROPIC, AnthropicModelExtractor, "/llm/rest/llm/v1/messages",
			map[string]any{"model": "claude-sonnet-4-5", "stream": true}, map[string]string{"model": "claude-sonnet-4-5", "stream": "true"}},
		{"google openai-compatible", models.GOOGLEAI, GoogleAIModelExtractor, "/llm/rest/llm/v1beta/openai/chat/completions",
			map[string]any{"model": "gemini-2.5-flash", "stream": true}, map[string]string{"model": "gemini-2.5-flash", "stream": "true"}},
		{"bedrock converse", models.BEDROCK, BedrockModelExtractor, "/llm/rest/llm/model/amazon.nova-pro-v1:0/converse",
			nil, map[string]string{"model": "amazon.nova-pro-v1:0", "stream": "false"}},
		{"bedrock converse-stream", models.BEDROCK, BedrockModelExtractor, "/llm/stream/llm/model/amazon.nova-pro-v1:0/converse-stream",
			nil, map[string]string{"model": "amazon.nova-pro-v1:0", "stream": "true"}},
		{"google native", models.GOOGLEAI, GoogleAIModelExtractor, "/llm/rest/llm/v1beta/models/gemini-2.5-flash:generateContent",
			nil, map[string]string{"model": "gemini-2.5-flash", "stream": "false"}},
		{"vertex native stream", models.VERTEX, VertexModelExtractor, "/llm/stream/llm/v1/projects/p/locations/l/publishers/google/models/gemini-2.0-flash:streamGenerateContent",
			nil, map[string]string{"model": "gemini-2.0-flash", "stream": "true"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := parityBody(t, tc.body)
			r := validatedRequest(t, tc.vendor, tc.extractor, tc.path, body)
			// The handler reads its own copy of the body after the filters.
			reqBody := append([]byte(nil), body...)

			md := addedExchangeMetadata(withExchangeMetadata(r, string(tc.vendor), reqBody))
			assert.Equal(t, tc.want, md)

			oneParse := testing.AllocsPerRun(20, func() { requestformat.Model("openai", "/v1/chat/completions", reqBody) })
			got := testing.AllocsPerRun(20, func() { withExchangeMetadata(r, string(tc.vendor), reqBody) })
			assert.Less(t, got, oneParse, "withExchangeMetadata allocs = %v; one body parse is %v", got, oneParse)
		})
	}
}

// A filter or plugin that rewrites the body between the extractor and the
// handler makes the kept fields miss: the metadata is of the body sent.
func TestExchangeMetadata_RewrittenBodyIsReadAgain(t *testing.T) {
	body := parityBody(t, map[string]any{"model": "gpt-4o"})
	r := validatedRequest(t, models.OPENAI, OpenAIModelExtractor, "/llm/rest/llm/v1/chat/completions", body)
	rewritten := parityBody(t, map[string]any{"model": "gpt-4o-mini", "stream": true})
	md := addedExchangeMetadata(withExchangeMetadata(r, string(models.OPENAI), rewritten))
	assert.Equal(t, map[string]string{"model": "gpt-4o-mini", "stream": "true"}, md)
}

// The edge's post_auth hook computes the metadata first (ExchangeMetadataFor);
// the handler reuses it for the same bytes instead of parsing again.
func TestExchangeMetadataFor_HandlerReusesIt(t *testing.T) {
	body := parityBody(t, map[string]any{"model": "gpt-4o", "stream": true})
	req := httptest.NewRequest(http.MethodPost, "/llm/rest/llm/v1/chat/completions", bytes.NewReader(body))
	r, md := ExchangeMetadataFor(req, string(models.OPENAI), body)
	assert.Equal(t, map[string]string{"model": "gpt-4o", "stream": "true"}, md)

	reqBody := append([]byte(nil), body...)
	oneParse := testing.AllocsPerRun(20, func() { requestformat.Model("openai", "/v1/chat/completions", reqBody) })
	got := testing.AllocsPerRun(20, func() { withExchangeMetadata(r, string(models.OPENAI), reqBody) })
	assert.Less(t, got, oneParse, "withExchangeMetadata allocs = %v; one body parse is %v", got, oneParse)
	assert.Equal(t, md, addedExchangeMetadata(withExchangeMetadata(r, string(models.OPENAI), reqBody)))
}

func addedExchangeMetadata(r *http.Request) map[string]string {
	md := map[string]string{}
	addExchangeMetadata(r.Context(), md)
	return md
}
