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

// The edge's post_auth hook runs before the model validation middleware.
// When it reads the body it decodes it into the map the model extractor
// then reuses, so a request with post_auth plugins is parsed no more often
// than v2.2.2 parsed it (53e.48): once by the hook, where v2.2.2 parsed it
// once in the extractor.
func TestExchangeMetadataFor_ExtractorReusesTheHooksDecode(t *testing.T) {
	for _, tc := range []struct {
		name      string
		vendor    models.Vendor
		extractor ModelNameExtractor
		path      string
	}{
		{"openai", models.OPENAI, OpenAIModelExtractor, "/llm/rest/llm/v1/chat/completions"},
		{"openai stream", models.OPENAI, OpenAIModelExtractor, "/llm/stream/llm/v1/chat/completions"},
		{"anthropic", models.ANTHROPIC, AnthropicModelExtractor, "/llm/rest/llm/v1/messages"},
		{"google openai-compatible", models.GOOGLEAI, GoogleAIModelExtractor, "/llm/rest/llm/v1beta/openai/chat/completions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := parityBody(t, map[string]any{"model": "m-1", "stream": true})
			req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(body))
			r, md := ExchangeMetadataFor(req, string(tc.vendor), body)
			assert.Equal(t, map[string]string{"model": "m-1", "stream": "true"}, md)

			// The middleware reads its own copy of the body.
			mwBody := append([]byte(nil), body...)
			model, err := tc.extractor(r, mwBody)
			require.NoError(t, err)
			assert.Equal(t, "m-1", model)

			oneParse := testing.AllocsPerRun(20, func() { requestformat.Model("openai", "/v1/chat/completions", mwBody) })
			got := testing.AllocsPerRun(20, func() { _, _ = tc.extractor(r, mwBody) })
			assert.Less(t, got, oneParse, "extractor allocs = %v after the hook decoded the body; one body parse is %v", got, oneParse)
		})
	}

	// A post_auth plugin that rewrote the body: the extractor decodes what it got.
	body := parityBody(t, map[string]any{"model": "m-1"})
	req := httptest.NewRequest(http.MethodPost, "/llm/rest/llm/v1/chat/completions", bytes.NewReader(body))
	r, _ := ExchangeMetadataFor(req, string(models.OPENAI), body)
	model, err := OpenAIModelExtractor(r, parityBody(t, map[string]any{"model": "m-2"}))
	require.NoError(t, err)
	assert.Equal(t, "m-2", model)
	_, err = OpenAIModelExtractor(r, []byte("not json"))
	assert.Error(t, err)
}

// On the edge, the post_auth hook decodes a Bedrock OpenAI-compatible body
// (its path names no model) into a map; BedrockModelExtractor reads
// "modelId" from that map rather than decode the body again (53e.50), and
// reads it as its own struct decode does.
func TestBedrockModelExtractor_ReusesTheHooksDecode(t *testing.T) {
	const path = "/llm/rest/llm/openai/v1/chat/completions"
	for _, fields := range []map[string]any{
		{"modelId": "amazon.nova-pro-v1:0"},
		{"ModelID": "amazon.nova-pro-v1:0"},
		{"modelId": 42},
		{"modelId": nil},
		{"modelId": ""},
		{"model": "openai.gpt-oss-20b-1:0"},
	} {
		body := parityBody(t, fields)
		plain := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		wantID, wantErr := BedrockModelExtractor(plain, body)

		r, _ := ExchangeMetadataFor(httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body)), string(models.BEDROCK), body)
		mwBody := append([]byte(nil), body...)
		id, err := BedrockModelExtractor(r, mwBody)
		assert.Equal(t, wantID, id, "%v", fields)
		assert.Equal(t, wantErr == nil, err == nil, "%v: %v vs %v", fields, err, wantErr)

		oneParse := testing.AllocsPerRun(20, func() { requestformat.Model("openai", "/v1/chat/completions", mwBody) })
		got := testing.AllocsPerRun(20, func() { _, _ = BedrockModelExtractor(r, mwBody) })
		assert.Less(t, got, oneParse, "%v: extractor allocs = %v after the hook decoded the body; one body parse is %v", fields, got, oneParse)
	}

	// Two keys that match "modelId" but for case: the struct keeps the later
	// one in the body, which the map cannot tell, so the body is decoded.
	body := []byte(`{"modelId":"first","MODELID":"second"}`)
	r, _ := ExchangeMetadataFor(httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body)), string(models.BEDROCK), body)
	id, err := BedrockModelExtractor(r, body)
	require.NoError(t, err)
	assert.Equal(t, "second", id)
}
