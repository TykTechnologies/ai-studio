package proxy

import (
	"bytes"
	"context"
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

	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
)

func scrapePrometheusMetrics(t *testing.T) string {
	t.Helper()
	h := metrics.Handler()
	if h == nil {
		t.Fatal("metrics handler is nil")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	b, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	return string(b)
}

// setupProxyWithMockUpstream creates a test proxy with a mock upstream LLM server.
// Returns the proxy, mock server closer func, the LLM, and the app.
func setupProxyWithMockUpstream(t *testing.T, db *gorm.DB, upstreamHandler http.HandlerFunc) (*Proxy, func(), *models.LLM, *models.App) {
	t.Helper()

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetSvc := budget.NewService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetSvc)
	require.NotNil(t, proxy)

	user := &models.User{ID: 1, Email: "metrics-test@example.com"}
	err := db.Create(user).Error
	require.NoError(t, err)

	mockUpstream := httptest.NewServer(upstreamHandler)

	llm := &models.LLM{
		Model:        gorm.Model{ID: 1},
		Name:         "MetricsTestLLM",
		Vendor:       models.MOCK_VENDOR,
		DefaultModel: "metrics-test-model",
		Active:       true,
		APIEndpoint:  mockUpstream.URL,
	}
	err = db.Create(llm).Error
	require.NoError(t, err)

	app := &models.App{
		Model:  gorm.Model{ID: 1},
		Name:   "MetricsTestApp",
		UserID: user.ID,
	}
	err = db.Create(app).Error
	require.NoError(t, err)

	cred := &models.Credential{
		Model:  gorm.Model{ID: 1},
		Secret: "metrics-test-token",
		Active: true,
	}
	err = db.Create(cred).Error
	require.NoError(t, err)

	app.CredentialID = cred.ID
	app.LLMs = []models.LLM{*llm}
	err = db.Save(app).Error
	require.NoError(t, err)

	err = proxy.loadResources()
	require.NoError(t, err)

	proxy.credValidator.RegisterValidator(string(models.MOCK_VENDOR), func(r *http.Request) (string, error) {
		token := r.Header.Get("Authorization")
		return strings.TrimPrefix(token, "Bearer "), nil
	})

	return proxy, mockUpstream.Close, llm, app
}

func TestHandleLLMRequest_EmitsInflightAndDurationMetrics(t *testing.T) {
	metrics.Init()

	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	proxy, closeUpstream, _, app := setupProxyWithMockUpstream(t, db,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simulate some processing time
			time.Sleep(10 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"id": "test-123",
				"object": "chat.completion",
				"model": "metrics-test-model",
				"choices": [{"message": {"content": "Hello"}}],
				"usage": {"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
			}`))
		}),
	)
	defer closeUpstream()

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

	reqBody := []byte(`{"prompt": "Hello"}`)
	req, _ := http.NewRequest("POST", srv.URL+"/llm/rest/metricstestllm/v1/chat", bytes.NewBuffer(reqBody))
	req.Header.Set("Authorization", "Bearer metrics-test-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(reqBody)))
	req = req.WithContext(context.WithValue(req.Context(), "app", app))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Give a moment for async metrics to settle
	time.Sleep(50 * time.Millisecond)

	body := scrapePrometheusMetrics(t)

	// Duration histogram should have at least one observation
	if !strings.Contains(body, "aistudio_llm_request_duration_seconds_count") {
		t.Error("missing request duration histogram after proxy request")
	}

	// After the request completes, inflight should be back to 0.
	// The metric line for our vendor should exist (it was incremented then decremented).
	if !strings.Contains(body, "aistudio_llm_inflight_requests") {
		t.Error("missing inflight requests metric")
	}
}

func TestPolicyBlockMetric_DirectEmission(t *testing.T) {
	// Budget enforcement is an enterprise feature (CE CheckBudget is a no-op),
	// so we test the metrics emission directly rather than through the full handler.
	// This verifies the metrics.RecordPolicyBlock call works correctly.
	metrics.Init()
	ctx := context.Background()

	metrics.RecordPolicyBlock(ctx, "budget", "rate_limit")
	metrics.RecordPolicyBlock(ctx, "request_filter", "firewall")
	metrics.RecordPolicyBlock(ctx, "response_filter", "filter")

	body := scrapePrometheusMetrics(t)

	if !strings.Contains(body, "aistudio_policy_blocks_total") {
		t.Fatal("missing aistudio_policy_blocks_total metric")
	}

	expectedLabels := []struct {
		ruleName  string
		blockType string
	}{
		{"budget", "rate_limit"},
		{"request_filter", "firewall"},
		{"response_filter", "filter"},
	}

	for _, exp := range expectedLabels {
		if !strings.Contains(body, fmt.Sprintf(`rule_name="%s"`, exp.ruleName)) {
			t.Errorf("missing rule_name=%q label", exp.ruleName)
		}
		if !strings.Contains(body, fmt.Sprintf(`block_type="%s"`, exp.blockType)) {
			t.Errorf("missing block_type=%q label", exp.blockType)
		}
	}
}

func TestHandleLLMRequest_NotFound_NoPanic(t *testing.T) {
	// Requesting a non-existent LLM should return 404 without recording
	// inflight or duration (since the LLM lookup fails before metrics).
	metrics.Init()

	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetSvc := budget.NewService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetSvc)

	r := mux.NewRouter()
	r.HandleFunc("/llm/rest/{llmSlug}/{rest:.*}", proxy.handleLLMRequest).Methods("POST")
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/llm/rest/nonexistent/v1/chat", bytes.NewBuffer([]byte(`{}`)))
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

func TestAnalyzeResponse_SetsModelNameOnProxyLog(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)

	llm := &models.LLM{
		Model:        gorm.Model{ID: 1},
		Name:         "ModelNameTestLLM",
		Vendor:       models.OPENAI,
		DefaultModel: "gpt-4o-mini",
		Active:       true,
	}
	db.Create(llm)

	app := &models.App{
		Model:  gorm.Model{ID: 1},
		Name:   "ModelNameTestApp",
		UserID: 1,
	}
	db.Create(app)

	// Create a model price so AnalyzeCompletionResponse doesn't error
	service.CreateModelPrice("gpt-4o-mini", string(models.OPENAI), 0.001, 0.002, 0, 0, "USD")

	// Build a minimal valid OpenAI response
	responseBody := []byte(`{
		"id": "chatcmpl-test",
		"object": "chat.completion",
		"model": "gpt-4o-mini",
		"choices": [{"index":0,"message":{"role":"assistant","content":"Hi"},"finish_reason":"stop"}],
		"usage": {"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
	}`)

	reqBody := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hi"}]}`)
	req, _ := http.NewRequest("POST", "/llm/rest/test/v1/chat/completions", bytes.NewBuffer(reqBody))
	req = req.WithContext(context.WithValue(req.Context(), "model_name", "gpt-4o-mini"))

	AnalyzeResponse(service, llm, app, http.StatusOK, responseBody, reqBody, req)

	// Wait for the async ProxyLog to be written
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var log models.ProxyLog
		err := db.First(&log).Error
		if err == nil {
			assert.Equal(t, "ModelNameTestLLM", log.ModelName, "ProxyLog.ModelName should be set to llm.Name")
			assert.Equal(t, string(models.OPENAI), log.Vendor)
			assert.Equal(t, app.ID, log.AppID)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("Timeout waiting for ProxyLog to be written")
}

func TestAnalyzeStreamingResponse_SetsModelNameOnProxyLog(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)

	llm := &models.LLM{
		Model:        gorm.Model{ID: 1},
		Name:         "StreamModelNameLLM",
		Vendor:       models.OPENAI,
		DefaultModel: "gpt-4o",
		Active:       true,
	}
	db.Create(llm)

	app := &models.App{
		Model:  gorm.Model{ID: 1},
		Name:   "StreamModelNameApp",
		UserID: 1,
	}
	db.Create(app)

	service.CreateModelPrice("gpt-4o", string(models.OPENAI), 0.001, 0.002, 0, 0, "USD")

	// Minimal streaming response
	chunk := `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"delta":{"content":"Hi"},"index":0}]}`
	doneChunk := `data: [DONE]`
	streamBody := []byte(chunk + "\n\n" + doneChunk + "\n\n")
	reqBody := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"Hi"}],"stream":true}`)

	req, _ := http.NewRequest("POST", "/llm/stream/test/v1/chat/completions", bytes.NewBuffer(reqBody))
	req = req.WithContext(context.WithValue(req.Context(), "model_name", "gpt-4o"))

	chunks := [][]byte{[]byte(chunk)}
	AnalyzeStreamingResponse(service, llm, app, http.StatusOK, streamBody, reqBody, req, chunks, time.Now(), "")

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var log models.ProxyLog
		err := db.First(&log).Error
		if err == nil {
			assert.Equal(t, "StreamModelNameLLM", log.ModelName, "ProxyLog.ModelName should be set for streaming responses")
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("Timeout waiting for streaming ProxyLog to be written")
}
