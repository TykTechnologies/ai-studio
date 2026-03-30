package proxy

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
)

// createBodySuppressionFixtures sets up common test data for body suppression tests.
// Returns service, llm, app, and the raw request/response bodies used.
func createBodySuppressionFixtures(t *testing.T, db *gorm.DB, dontLogBodies bool) (*services.Service, *models.LLM, *models.App, []byte, []byte) {
	t.Helper()

	service := services.NewService(db)

	user := &models.User{ID: 1, Email: "test@example.com"}
	require.NoError(t, db.Create(user).Error)

	llm := &models.LLM{
		Model:         gorm.Model{ID: 1},
		Name:          "TestLLM",
		Vendor:        models.MOCK_VENDOR,
		DefaultModel:  "test-model",
		Active:        true,
		APIEndpoint:   "http://mock-api.example.com",
		DontLogBodies: dontLogBodies,
	}
	require.NoError(t, db.Create(llm).Error)

	app := &models.App{
		Model:  gorm.Model{ID: 1},
		Name:   "TestApp",
		UserID: user.ID,
	}
	require.NoError(t, db.Create(app).Error)

	price := &models.ModelPrice{
		Model:     gorm.Model{ID: 1},
		ModelName: "test-model",
		Vendor:    string(models.MOCK_VENDOR),
		CPT:       0.002,
		CPIT:      0.001,
		Currency:  "USD",
	}
	require.NoError(t, db.Create(price).Error)

	reqBody := []byte(`{"model":"test-model","messages":[{"role":"user","content":"sensitive prompt data"}]}`)
	respBody := []byte(`{
		"id":"mock-123",
		"object":"chat.completion",
		"model":"test-model",
		"choices":[{"message":{"content":"sensitive response data"}}],
		"usage":{"prompt_tokens":5,"completion_tokens":10,"total_tokens":15}
	}`)

	return service, llm, app, reqBody, respBody
}

func makeTestRequest(reqBody []byte) *http.Request {
	req, _ := http.NewRequest("POST", "http://example.com/test", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), "model_name", "test-model")
	return req.WithContext(ctx)
}

// TestAnalyzeResponse_DontLogBodies_Enabled verifies that when DontLogBodies is true,
// the ProxyLog record has empty request and response bodies.
func TestAnalyzeResponse_DontLogBodies_Enabled(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service, llm, app, reqBody, respBody := createBodySuppressionFixtures(t, db, true)

	notificationSvc := services.NewTestNotificationService(db)
	budgetService := budget.NewService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetService)
	require.NotNil(t, proxy)

	req := makeTestRequest(reqBody)
	proxy.analyzeResponse(llm, app, http.StatusOK, respBody, reqBody, req)

	waitForAnalytics(t, db, 1)
	waitUntilIdle(t, db)
	waitForProxyLog(t, db, app.ID, http.StatusOK)

	var proxyLog models.ProxyLog
	require.NoError(t, db.Where("app_id = ?", app.ID).First(&proxyLog).Error)

	assert.Empty(t, proxyLog.RequestBody, "request body should be empty when DontLogBodies is true")
	assert.Empty(t, proxyLog.ResponseBody, "response body should be empty when DontLogBodies is true")
	assert.Equal(t, http.StatusOK, proxyLog.ResponseCode, "response code should still be recorded")
	assert.Equal(t, string(models.MOCK_VENDOR), proxyLog.Vendor, "vendor should still be recorded")

	// Token analytics should still be recorded
	record := waitForRecordWithCost(t, db)
	require.NotNil(t, record)
	assert.Equal(t, 5, record.PromptTokens, "token counts should still be recorded")
	assert.Equal(t, 10, record.ResponseTokens, "token counts should still be recorded")
}

// TestAnalyzeResponse_DontLogBodies_Disabled verifies that when DontLogBodies is false (default),
// the ProxyLog record contains the full request and response bodies.
func TestAnalyzeResponse_DontLogBodies_Disabled(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service, llm, app, reqBody, respBody := createBodySuppressionFixtures(t, db, false)

	notificationSvc := services.NewTestNotificationService(db)
	budgetService := budget.NewService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetService)
	require.NotNil(t, proxy)

	req := makeTestRequest(reqBody)
	proxy.analyzeResponse(llm, app, http.StatusOK, respBody, reqBody, req)

	waitForAnalytics(t, db, 1)
	waitUntilIdle(t, db)
	waitForProxyLog(t, db, app.ID, http.StatusOK)

	var proxyLog models.ProxyLog
	require.NoError(t, db.Where("app_id = ?", app.ID).First(&proxyLog).Error)

	assert.Equal(t, string(reqBody), proxyLog.RequestBody, "request body should be stored when DontLogBodies is false")
	assert.Contains(t, proxyLog.ResponseBody, "sensitive response data", "response body should be stored when DontLogBodies is false")
}

// TestAnalyzeStreamingResponse_DontLogBodies_Enabled verifies body suppression for streaming responses.
func TestAnalyzeStreamingResponse_DontLogBodies_Enabled(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service, llm, app, reqBody, _ := createBodySuppressionFixtures(t, db, true)

	notificationSvc := services.NewTestNotificationService(db)
	budgetService := budget.NewService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetService)
	require.NotNil(t, proxy)

	chunks := [][]byte{
		[]byte(`{"id":"mock-123","object":"chat.completion.chunk","choices":[{"delta":{"content":"Hello"}}]}`),
		[]byte(`{"id":"mock-123","object":"chat.completion","model":"test-model","choices":[{"message":{"content":"Hello"}}],"usage":{"prompt_tokens":5,"completion_tokens":10,"total_tokens":15}}`),
	}
	fullResponse := []byte(`{"id":"mock-123","object":"chat.completion","model":"test-model","choices":[{"message":{"content":"Hello"}}],"usage":{"prompt_tokens":5,"completion_tokens":10,"total_tokens":15}}`)

	req := makeTestRequest(reqBody)
	proxy.analyzeStreamingResponse(llm, app, req, http.StatusOK, fullResponse, reqBody, chunks, time.Now(), "")

	waitForAnalytics(t, db, 1)
	waitUntilIdle(t, db)
	waitForProxyLog(t, db, app.ID, http.StatusOK)

	var proxyLog models.ProxyLog
	require.NoError(t, db.Where("app_id = ?", app.ID).First(&proxyLog).Error)

	assert.Empty(t, proxyLog.RequestBody, "request body should be empty for streaming when DontLogBodies is true")
	assert.Empty(t, proxyLog.ResponseBody, "response body should be empty for streaming when DontLogBodies is true")

	// Token analytics should still be recorded
	record := waitForRecordWithCost(t, db)
	require.NotNil(t, record)
	assert.Equal(t, 5, record.PromptTokens)
	assert.Equal(t, 10, record.ResponseTokens)
}

// TestAnalyzeStreamingResponse_DontLogBodies_Disabled verifies bodies are stored for streaming when flag is false.
func TestAnalyzeStreamingResponse_DontLogBodies_Disabled(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service, llm, app, reqBody, _ := createBodySuppressionFixtures(t, db, false)

	notificationSvc := services.NewTestNotificationService(db)
	budgetService := budget.NewService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetService)
	require.NotNil(t, proxy)

	fullResponse := []byte(`{"id":"mock-123","object":"chat.completion","model":"test-model","choices":[{"message":{"content":"Hello"}}],"usage":{"prompt_tokens":5,"completion_tokens":10,"total_tokens":15}}`)
	chunks := [][]byte{fullResponse}

	req := makeTestRequest(reqBody)
	proxy.analyzeStreamingResponse(llm, app, req, http.StatusOK, fullResponse, reqBody, chunks, time.Now(), "")

	waitForAnalytics(t, db, 1)
	waitUntilIdle(t, db)
	waitForProxyLog(t, db, app.ID, http.StatusOK)

	var proxyLog models.ProxyLog
	require.NoError(t, db.Where("app_id = ?", app.ID).First(&proxyLog).Error)

	assert.Equal(t, string(reqBody), proxyLog.RequestBody, "request body should be stored for streaming when DontLogBodies is false")
	assert.NotEmpty(t, proxyLog.ResponseBody, "response body should be stored for streaming when DontLogBodies is false")
}
