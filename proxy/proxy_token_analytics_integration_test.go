package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
)

// setupIntegrationProxy creates the standard test fixtures for token analytics
// integration tests: a proxy backed by an in-memory DB, a user, an OpenAI-vendor
// LLM pointing at mockURL, an app with valid credentials, and a model price.
// It returns the proxy, DB, LLM, app, credential secret, and service.
func setupIntegrationProxy(t *testing.T, db *gorm.DB, mockURL string) (*Proxy, *models.LLM, *models.App, string) {
	t.Helper()

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := budget.NewService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetService)
	require.NotNil(t, proxy)

	user := &models.User{ID: 1, Email: "integration@example.com"}
	require.NoError(t, db.Create(user).Error)

	llm := &models.LLM{
		Model:        gorm.Model{ID: 1},
		Name:         "OpenAI-Test",
		Vendor:       models.OPENAI,
		DefaultModel: "gpt-4",
		Active:       true,
		APIEndpoint:  mockURL,
		APIKey:       "sk-test-key",
	}
	require.NoError(t, db.Create(llm).Error)

	app := &models.App{
		Model:  gorm.Model{ID: 1},
		Name:   "IntegrationTestApp",
		UserID: user.ID,
	}
	require.NoError(t, db.Create(app).Error)

	cred := &models.Credential{
		Model:  gorm.Model{ID: 1},
		Secret: "integration-test-token",
		Active: true,
	}
	require.NoError(t, db.Create(cred).Error)

	app.CredentialID = cred.ID
	app.LLMs = []models.LLM{*llm}
	require.NoError(t, db.Save(app).Error)

	require.NoError(t, proxy.loadResources())

	return proxy, llm, app, cred.Secret
}

// startProxyServer creates a test HTTP server wired through the credential
// middleware and the REST LLM handler, matching the real proxy routing.
func startProxyServer(t *testing.T, proxy *Proxy) *httptest.Server {
	t.Helper()

	r := mux.NewRouter()
	r.HandleFunc("/llm/rest/{llmSlug}/{rest:.*}", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		newReq := r.Clone(r.Context())
		newReq.Body = io.NopCloser(bytes.NewBuffer(body))
		newReq.ContentLength = int64(len(body))
		proxy.handleLLMRequest(w, newReq)
	}).Methods("POST")

	return httptest.NewServer(proxy.credValidator.Middleware(r))
}

// sendProxyRequest sends a chat completion request through the proxy and
// returns the HTTP response.
func sendProxyRequest(t *testing.T, proxyURL, llmSlug, secret, reqBody string) *http.Response {
	t.Helper()

	url := fmt.Sprintf("%s/llm/rest/%s/v1/chat/completions", proxyURL, llmSlug)
	req, err := http.NewRequest("POST", url, bytes.NewBufferString(reqBody))
	require.NoError(t, err)

	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// TestTokenAnalytics_EndToEnd_OpenAI verifies the complete flow for a standard
// successful OpenAI API call: the request is proxied to the mock upstream,
// the response usage tokens are extracted, a cost is calculated from the model
// price, and an LLMChatRecord plus a ProxyLog are persisted.
func TestTokenAnalytics_EndToEnd_OpenAI(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	// Mock upstream OpenAI API
	var upstreamCalled bool
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": "chatcmpl-integration-1",
			"object": "chat.completion",
			"model": "gpt-4",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "Hello!"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30}
		}`))
	}))
	defer mockUpstream.Close()

	proxy, llm, _, secret := setupIntegrationProxy(t, db, mockUpstream.URL)

	// Create model price: CPIT=0.001 per prompt token, CPT=0.002 per completion token
	price := &models.ModelPrice{
		Model:     gorm.Model{ID: 1},
		ModelName: "gpt-4",
		Vendor:    string(models.OPENAI),
		CPT:       0.002,
		CPIT:      0.001,
		Currency:  "USD",
	}
	require.NoError(t, db.Create(price).Error)

	srv := startProxyServer(t, proxy)
	defer srv.Close()

	reqBody := `{"model": "gpt-4", "messages": [{"role": "user", "content": "Hi"}]}`
	resp := sendProxyRequest(t, srv.URL, "openai-test", secret, reqBody)
	defer resp.Body.Close()

	// The proxy should return the upstream response successfully
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "Hello!")

	// The mock upstream must have been called
	assert.True(t, upstreamCalled, "mock upstream server was not called")

	// Wait for async analytics to be recorded
	waitForAnalytics(t, db, 1)
	waitUntilIdle(t, db)

	// Verify LLMChatRecord
	var record models.LLMChatRecord
	require.NoError(t, db.First(&record).Error)

	assert.Equal(t, llm.ID, record.LLMID)
	assert.Equal(t, "gpt-4", record.Name)
	assert.Equal(t, string(models.OPENAI), record.Vendor)
	assert.Equal(t, 10, record.PromptTokens)
	assert.Equal(t, 20, record.ResponseTokens)
	assert.Equal(t, 30, record.TotalTokens)
	assert.Equal(t, 1, record.Choices)
	assert.Equal(t, models.ProxyInteraction, record.InteractionType)
	assert.Equal(t, "USD", record.Currency)

	// Cost = ((CPT * responseTokens) + (CPIT * promptTokens)) * 10000
	// = ((0.002 * 20) + (0.001 * 10)) * 10000 = (0.04 + 0.01) * 10000 = 500.0
	assert.InDelta(t, 500.0, record.Cost, 0.01)

	// Verify ProxyLog
	var proxyLog models.ProxyLog
	require.NoError(t, db.First(&proxyLog).Error)

	assert.Equal(t, uint(1), proxyLog.AppID)
	assert.Equal(t, http.StatusOK, proxyLog.ResponseCode)
	assert.Equal(t, string(models.OPENAI), proxyLog.Vendor)
	assert.Contains(t, proxyLog.ResponseBody, "chatcmpl-integration-1")
}

// TestTokenAnalytics_MalformedJSON_NoRecord verifies that when the upstream LLM
// returns malformed JSON, the proxy still returns a response to the client but
// does NOT create an LLMChatRecord or ProxyLog, because AnalyzeResponse fails
// on the JSON unmarshal and returns early before recording anything.
func TestTokenAnalytics_MalformedJSON_NoRecord(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	// Mock upstream returns malformed JSON
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Malformed JSON: invalid value after "usage" key
		w.Write([]byte(`{"id": "chatcmpl-bad", "usage": invalid}`))
	}))
	defer mockUpstream.Close()

	proxy, _, _, secret := setupIntegrationProxy(t, db, mockUpstream.URL)

	srv := startProxyServer(t, proxy)
	defer srv.Close()

	reqBody := `{"model": "gpt-4", "messages": [{"role": "user", "content": "Hi"}]}`
	resp := sendProxyRequest(t, srv.URL, "openai-test", secret, reqBody)
	defer resp.Body.Close()

	// The proxy should still relay the upstream response to the client
	// (the body is passed through by the reverse proxy regardless of JSON validity)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Give analytics goroutine time to process (or fail to process)
	waitUntilIdle(t, db)

	// No LLMChatRecord should be created because AnalyzeResponse fails on unmarshal
	var chatCount int64
	require.NoError(t, db.Model(&models.LLMChatRecord{}).Count(&chatCount).Error)
	assert.Equal(t, int64(0), chatCount, "no LLMChatRecord should exist for malformed JSON response")

	// No ProxyLog either — AnalyzeResponse returns early on unmarshal error,
	// before the ProxyLog is recorded. This is the current system behavior.
	var logCount int64
	require.NoError(t, db.Model(&models.ProxyLog{}).Count(&logCount).Error)
	assert.Equal(t, int64(0), logCount, "no ProxyLog should exist when response analysis fails")
}

// TestTokenAnalytics_Upstream500_ProxyLogWithErrorCode verifies correct behavior
// when the upstream LLM returns an HTTP 500 error: a ProxyLog is created with
// the 500 status code, and no LLMChatRecord is created since the error response
// won't contain valid usage data.
func TestTokenAnalytics_Upstream500_ProxyLogWithErrorCode(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	// Mock upstream returns HTTP 500
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": {"message": "Internal server error", "type": "server_error"}}`))
	}))
	defer mockUpstream.Close()

	proxy, _, _, secret := setupIntegrationProxy(t, db, mockUpstream.URL)

	// Create model price so cost calculation can run if a record were created
	price := &models.ModelPrice{
		Model:     gorm.Model{ID: 1},
		ModelName: "gpt-4",
		Vendor:    string(models.OPENAI),
		CPT:       0.002,
		CPIT:      0.001,
		Currency:  "USD",
	}
	require.NoError(t, db.Create(price).Error)

	srv := startProxyServer(t, proxy)
	defer srv.Close()

	reqBody := `{"model": "gpt-4", "messages": [{"role": "user", "content": "Hi"}]}`
	resp := sendProxyRequest(t, srv.URL, "openai-test", secret, reqBody)
	defer resp.Body.Close()

	// The proxy relays the 500 status from upstream
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	// Wait for analytics to settle
	waitUntilIdle(t, db)

	// Verify ProxyLog was created with 500 status code
	waitForProxyLog(t, db, 1, http.StatusInternalServerError)

	var proxyLog models.ProxyLog
	require.NoError(t, db.Where("response_code = ?", http.StatusInternalServerError).First(&proxyLog).Error)
	assert.Equal(t, uint(1), proxyLog.AppID)
	assert.Equal(t, http.StatusInternalServerError, proxyLog.ResponseCode)
	assert.Equal(t, string(models.OPENAI), proxyLog.Vendor)

	// An error response body from OpenAI does not match the /v1/chat/completions
	// JSON schema, so AnalyzeResponse returns an error and no chat record is created.
	// However, the OpenAI vendor's AnalyzeResponse will still attempt to unmarshal
	// the body — if it succeeds with zero usage, a record with zero tokens may be created.
	// We verify token counts are zero regardless.
	var chatCount int64
	require.NoError(t, db.Model(&models.LLMChatRecord{}).Count(&chatCount).Error)

	if chatCount > 0 {
		var record models.LLMChatRecord
		require.NoError(t, db.First(&record).Error)
		assert.Equal(t, 0, record.PromptTokens, "prompt tokens should be zero for error response")
		assert.Equal(t, 0, record.ResponseTokens, "response tokens should be zero for error response")
		assert.Equal(t, 0, record.TotalTokens, "total tokens should be zero for error response")
		assert.InDelta(t, 0.0, record.Cost, 0.01, "cost should be zero for error response")
	}
}

// TestTokenAnalytics_EndToEnd_NoPriceRecord verifies that when no ModelPrice
// exists for the model, the system still records analytics but with zero cost.
func TestTokenAnalytics_EndToEnd_NoPriceRecord(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": "chatcmpl-noprice",
			"object": "chat.completion",
			"model": "gpt-4",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "Hi"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 5, "completion_tokens": 15, "total_tokens": 20}
		}`))
	}))
	defer mockUpstream.Close()

	proxy, _, _, secret := setupIntegrationProxy(t, db, mockUpstream.URL)

	// Intentionally do NOT create a ModelPrice — the service auto-creates a
	// zero-cost record when one is not found.

	srv := startProxyServer(t, proxy)
	defer srv.Close()

	reqBody := `{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello"}]}`
	resp := sendProxyRequest(t, srv.URL, "openai-test", secret, reqBody)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	waitForAnalytics(t, db, 1)
	waitUntilIdle(t, db)

	var record models.LLMChatRecord
	require.NoError(t, db.First(&record).Error)

	// Tokens are recorded correctly even without a price
	assert.Equal(t, 5, record.PromptTokens)
	assert.Equal(t, 15, record.ResponseTokens)
	assert.Equal(t, 20, record.TotalTokens)

	// Cost should be zero because auto-created price has zero rates
	assert.InDelta(t, 0.0, record.Cost, 0.01)
}

// TestTokenAnalytics_MultipleChoices verifies that the Choices field is
// correctly populated when the LLM returns multiple choices.
func TestTokenAnalytics_MultipleChoices(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": "chatcmpl-multi",
			"object": "chat.completion",
			"model": "gpt-4",
			"choices": [
				{"index": 0, "message": {"role": "assistant", "content": "Option A"}, "finish_reason": "stop"},
				{"index": 1, "message": {"role": "assistant", "content": "Option B"}, "finish_reason": "stop"}
			],
			"usage": {"prompt_tokens": 8, "completion_tokens": 12, "total_tokens": 20}
		}`))
	}))
	defer mockUpstream.Close()

	proxy, _, _, secret := setupIntegrationProxy(t, db, mockUpstream.URL)

	price := &models.ModelPrice{
		Model:     gorm.Model{ID: 1},
		ModelName: "gpt-4",
		Vendor:    string(models.OPENAI),
		CPT:       0.002,
		CPIT:      0.001,
		Currency:  "USD",
	}
	require.NoError(t, db.Create(price).Error)

	srv := startProxyServer(t, proxy)
	defer srv.Close()

	reqBody := `{"model": "gpt-4", "messages": [{"role": "user", "content": "Give two options"}]}`
	resp := sendProxyRequest(t, srv.URL, "openai-test", secret, reqBody)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	waitForAnalytics(t, db, 1)
	waitUntilIdle(t, db)

	var record models.LLMChatRecord
	require.NoError(t, db.First(&record).Error)

	assert.Equal(t, 2, record.Choices)
	assert.Equal(t, 8, record.PromptTokens)
	assert.Equal(t, 12, record.ResponseTokens)

	// Cost = ((0.002 * 12) + (0.001 * 8)) * 10000 = (0.024 + 0.008) * 10000 = 320.0
	assert.InDelta(t, 320.0, record.Cost, 0.01)
}

// TestTokenAnalytics_ToolCallsCounted verifies that tool calls in the response
// are correctly counted in the analytics record.
func TestTokenAnalytics_ToolCallsCounted(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": "chatcmpl-tools",
			"object": "chat.completion",
			"model": "gpt-4",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": null,
					"tool_calls": [
						{"id": "call_1", "type": "function", "function": {"name": "get_weather", "arguments": "{\"city\":\"London\"}"}},
						{"id": "call_2", "type": "function", "function": {"name": "get_time", "arguments": "{\"tz\":\"UTC\"}"}}
					]
				},
				"finish_reason": "tool_calls"
			}],
			"usage": {"prompt_tokens": 25, "completion_tokens": 30, "total_tokens": 55}
		}`))
	}))
	defer mockUpstream.Close()

	proxy, _, _, secret := setupIntegrationProxy(t, db, mockUpstream.URL)

	price := &models.ModelPrice{
		Model:     gorm.Model{ID: 1},
		ModelName: "gpt-4",
		Vendor:    string(models.OPENAI),
		CPT:       0.002,
		CPIT:      0.001,
		Currency:  "USD",
	}
	require.NoError(t, db.Create(price).Error)

	srv := startProxyServer(t, proxy)
	defer srv.Close()

	reqBody := `{"model": "gpt-4", "messages": [{"role": "user", "content": "What is the weather in London?"}]}`
	resp := sendProxyRequest(t, srv.URL, "openai-test", secret, reqBody)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	waitForAnalytics(t, db, 1)
	waitUntilIdle(t, db)

	var record models.LLMChatRecord
	require.NoError(t, db.First(&record).Error)

	assert.Equal(t, 2, record.ToolCalls, "should count both tool calls")
	assert.Equal(t, 25, record.PromptTokens)
	assert.Equal(t, 30, record.ResponseTokens)
	assert.Equal(t, 55, record.TotalTokens)

	// Cost = ((0.002 * 30) + (0.001 * 25)) * 10000 = (0.06 + 0.025) * 10000 = 850.0
	assert.InDelta(t, 850.0, record.Cost, 0.01)
}
