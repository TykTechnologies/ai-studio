//go:build enterprise

package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// redactingFilter rewrites SECRET-123 to [REDACTED] in the raw request body.
// It is deliberately a payload (not messages) rewrite so it works for any
// vendor, including the mock vendor that has no message extractor.
const redactingFilterScript = `
text := import("text")
output := {
    block: false,
    payload: text.replace(input.raw_input, "SECRET-123", "[REDACTED]", -1)
}
`

const blockingFilterScript = `output := {block: true, message: "no secrets"}`

// newFilterTestProxy builds a bare proxy with the OpenAI and Anthropic message
// registries, which is all runRequestFilters needs.
func newFilterTestProxy() *Proxy {
	p := &Proxy{}
	extractors := NewMessageExtractorRegistry()
	extractors.Register(&OpenAIMessageExtractor{})
	extractors.Register(&AnthropicMessageExtractor{})
	p.messageExtractorRegistry = extractors

	reconstructors := NewMessageReconstructorRegistry()
	reconstructors.Register(&OpenAIMessageReconstructor{})
	reconstructors.Register(&AnthropicMessageReconstructor{})
	p.messageReconstructorRegistry = reconstructors
	return p
}

// The proxy log must hold the prompt as the filters left it. Before this test
// the handler copied the body before screenProxyRequestByVendor rewrote it and
// logged that copy, so a redaction reached the vendor but not the database.
func TestHandleLLMRequest_ProxyLogStoresPostFilterBody(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	var upstreamSaw []byte
	proxy, closeUpstream, llm, app := setupProxyWithMockUpstream(t, db,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			upstreamSaw, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","model":"metrics-test-model","choices":[{"message":{"content":"Hello"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
		}),
	)
	defer closeUpstream()

	filter := &models.Filter{Name: "redact", Script: []byte(redactingFilterScript)}
	require.NoError(t, db.Create(filter).Error)
	require.NoError(t, db.Model(llm).Association("Filters").Append(filter))
	require.NoError(t, proxy.loadResources())

	r := mux.NewRouter()
	r.HandleFunc("/llm/rest/{llmSlug}/{rest:.*}", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		newReq := r.Clone(r.Context())
		newReq.Body = io.NopCloser(bytes.NewBuffer(body))
		newReq.ContentLength = int64(len(body))
		proxy.handleLLMRequest(w, newReq)
	}).Methods("POST")
	srv := httptest.NewServer(proxy.credValidator.Middleware(r))
	defer srv.Close()

	reqBody := []byte(`{"prompt": "the code is SECRET-123"}`)
	req, _ := http.NewRequest("POST", srv.URL+"/llm/rest/metricstestllm/v1/chat", bytes.NewBuffer(reqBody))
	req.Header.Set("Authorization", "Bearer metrics-test-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(reqBody)))
	req = req.WithContext(context.WithValue(req.Context(), "app", app))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	assert.Contains(t, string(upstreamSaw), "[REDACTED]", "the vendor must see the redacted prompt")
	assert.NotContains(t, string(upstreamSaw), "SECRET-123")

	waitForProxyLog(t, db, app.ID, http.StatusOK)
	var proxyLog models.ProxyLog
	require.NoError(t, db.Where("app_id = ? AND response_code = ?", app.ID, http.StatusOK).First(&proxyLog).Error)
	assert.Contains(t, proxyLog.RequestBody, "[REDACTED]", "the proxy log must store the redacted prompt")
	assert.NotContains(t, proxyLog.RequestBody, "SECRET-123", "the pre-filter prompt must not reach the database")
}
