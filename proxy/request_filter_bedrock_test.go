//go:build enterprise

package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

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

func TestBedrockScreenRequest(t *testing.T) {
	app := &models.App{Model: gorm.Model{ID: 7}, UserID: 1}
	body := []byte(`{"model":"claude","messages":[{"role":"user","content":"the code is SECRET-123"}]}`)

	decode := func(t *testing.T) *ChatCompletionRequest {
		var req ChatCompletionRequest
		require.NoError(t, json.Unmarshal(body, &req))
		return &req
	}

	t.Run("no request filters is a pass-through", func(t *testing.T) {
		p := newFilterTestProxy()
		conf := &models.LLM{Vendor: models.BEDROCK, Filters: []*models.Filter{{Name: "resp", ResponseFilter: true, Script: []byte(blockingFilterScript)}}}
		req := decode(t)
		r := httptest.NewRequest("POST", "/ai/bedrock/v1/chat/completions", bytes.NewReader(body))

		got, gotBody, fail := p.bedrockScreenRequest(r, conf, app, req, body, "claude", time.Now())
		require.NoError(t, fail.err)
		assert.Same(t, req, got)
		assert.Equal(t, body, gotBody)
	})

	t.Run("blocking filter is a 400 that does not fail over", func(t *testing.T) {
		p := newFilterTestProxy()
		conf := &models.LLM{Vendor: models.BEDROCK, Filters: []*models.Filter{{Name: "guard", Script: []byte(blockingFilterScript)}}}
		r := httptest.NewRequest("POST", "/ai/bedrock/v1/chat/completions", bytes.NewReader(body))

		_, _, fail := p.bedrockScreenRequest(r, conf, app, decode(t), body, "claude", time.Now())
		require.Error(t, fail.err)
		assert.Contains(t, fail.err.Error(), "no secrets")
		assert.Equal(t, http.StatusBadRequest, fail.status)
		assert.True(t, fail.hasStatus)

		ok, _ := shouldFailover(fail, models.ResolvedFailoverTriggers{OnTimeout: true, OnConnectionError: true, StatusCodes: map[int]bool{400: true}})
		assert.False(t, ok, "a policy block must never fail over to a rung that may lack the filter")
		p.analyzers.Wait()
	})

	t.Run("modifying filter reaches the translated request", func(t *testing.T) {
		p := newFilterTestProxy()
		conf := &models.LLM{Vendor: models.BEDROCK, Filters: []*models.Filter{{Name: "redact", Script: []byte(redactingFilterScript)}}}
		req := decode(t)
		r := httptest.NewRequest("POST", "/ai/bedrock/v1/chat/completions", bytes.NewReader(body))

		got, gotBody, fail := p.bedrockScreenRequest(r, conf, app, req, body, "claude", time.Now())
		require.NoError(t, fail.err)
		require.NotSame(t, req, got, "the caller's request must not be mutated: filters are per rung")
		assert.Equal(t, "the code is [REDACTED]", got.Messages[0].Content)
		assert.Equal(t, "the code is SECRET-123", req.Messages[0].Content)
		assert.Contains(t, string(gotBody), "[REDACTED]")
	})

	t.Run("messages rewrite goes through the OpenAI reconstructor", func(t *testing.T) {
		p := newFilterTestProxy()
		script := `output := {block: false, messages: [{role: "user", content: "rewritten"}]}`
		conf := &models.LLM{Vendor: models.BEDROCK, Filters: []*models.Filter{{Name: "rewrite", Script: []byte(script)}}}
		r := httptest.NewRequest("POST", "/ai/bedrock/v1/chat/completions", bytes.NewReader(body))

		got, _, fail := p.bedrockScreenRequest(r, conf, app, decode(t), body, "claude", time.Now())
		require.NoError(t, fail.err)
		require.Len(t, got.Messages, 1)
		assert.Equal(t, "rewritten", got.Messages[0].Content)
		assert.Equal(t, "claude", got.Model, "non-message fields survive the rewrite")
	})
}

func TestAnthropicScreenRequest(t *testing.T) {
	app := &models.App{Model: gorm.Model{ID: 7}, UserID: 1}
	body := []byte(`{"model":"claude","max_tokens":10,"system":"be terse","messages":[{"role":"user","content":"the code is SECRET-123"}]}`)

	decode := func(t *testing.T) *AnthropicMessagesRequest {
		var req AnthropicMessagesRequest
		require.NoError(t, json.Unmarshal(body, &req))
		return &req
	}

	t.Run("blocking filter writes an Anthropic 400 and stops the handler", func(t *testing.T) {
		p := newFilterTestProxy()
		conf := &models.LLM{Vendor: models.BEDROCK, Filters: []*models.Filter{{Name: "guard", Script: []byte(blockingFilterScript)}}}
		r := httptest.NewRequest("POST", "/anthropic/bedrock/v1/messages", bytes.NewReader(body))
		w := httptest.NewRecorder()

		_, _, ok := p.anthropicScreenRequest(w, r, conf, app, decode(t), body, "anthropic.claude", time.Now())
		assert.False(t, ok)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "invalid_request_error", anthropicErrorType(t, w.Body.Bytes()))
		assert.Contains(t, w.Body.String(), "no secrets")
		p.analyzers.Wait()
	})

	t.Run("modifying filter reaches the translated request", func(t *testing.T) {
		p := newFilterTestProxy()
		conf := &models.LLM{Vendor: models.BEDROCK, Filters: []*models.Filter{{Name: "redact", Script: []byte(redactingFilterScript)}}}
		req := decode(t)
		r := httptest.NewRequest("POST", "/anthropic/bedrock/v1/messages", bytes.NewReader(body))
		w := httptest.NewRecorder()

		got, gotBody, ok := p.anthropicScreenRequest(w, r, conf, app, req, body, "anthropic.claude", time.Now())
		require.True(t, ok)
		assert.Equal(t, http.StatusOK, w.Code, "nothing is written on the happy path")
		require.NotSame(t, req, got)
		assert.Contains(t, string(got.Messages[0].Content), "[REDACTED]")
		assert.Contains(t, string(gotBody), "[REDACTED]")
		assert.Equal(t, 10, got.MaxTokens)
	})

	t.Run("messages rewrite goes through the Anthropic reconstructor", func(t *testing.T) {
		p := newFilterTestProxy()
		script := `output := {block: false, messages: [{role: "system", content: "new system"}, {role: "user", content: "rewritten"}]}`
		conf := &models.LLM{Vendor: models.BEDROCK, Filters: []*models.Filter{{Name: "rewrite", Script: []byte(script)}}}
		r := httptest.NewRequest("POST", "/anthropic/bedrock/v1/messages", bytes.NewReader(body))
		w := httptest.NewRecorder()

		got, _, ok := p.anthropicScreenRequest(w, r, conf, app, decode(t), body, "anthropic.claude", time.Now())
		require.True(t, ok)
		require.Len(t, got.Messages, 1)
		assert.True(t, strings.Contains(string(got.Messages[0].Content), "rewritten"))
		assert.True(t, strings.Contains(string(got.System), "new system"))
	})
}
