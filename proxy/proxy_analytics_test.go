package proxy

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

func TestAnalyzeResponse(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := services.NewBudgetService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetService)
	require.NotNil(t, proxy)

	// Create test user
	user := &models.User{
		ID:    1,
		Email: "test@example.com",
	}
	err := db.Create(user).Error
	require.NoError(t, err)

	// Create test LLM
	llm := &models.LLM{
		Model:        gorm.Model{ID: 1},
		Name:         "TestLLM",
		Vendor:       models.MOCK_VENDOR,
		DefaultModel: "test-model",
		Active:       true,
		APIEndpoint:  "http://mock-api.example.com",
	}
	err = db.Create(llm).Error
	require.NoError(t, err)

	// Create test app
	app := &models.App{
		Model:  gorm.Model{ID: 1},
		Name:   "TestApp",
		UserID: user.ID,
	}
	err = db.Create(app).Error
	require.NoError(t, err)

	// Create test request
	reqBody := []byte(`{"prompt": "Hello"}`)
	req, _ := http.NewRequest("POST", "http://example.com/test", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	// Test successful response
	respBody := []byte(`{
		"id": "mock-123",
		"object": "chat.completion",
		"model": "test-model",
		"choices": [{"message": {"content": "Hello"}}],
		"usage": {"prompt_tokens":5,"completion_tokens":10,"total_tokens":15}
	}`)

	// Create a mock price
	price := &models.ModelPrice{
		Model:     gorm.Model{ID: 1},
		ModelName: "test-model",
		Vendor:    string(models.MOCK_VENDOR),
		CPT:       0.002,
		CPIT:      0.001,
		Currency:  "USD",
	}
	err = db.Create(price).Error
	require.NoError(t, err)

	// Test response analysis
	proxy.analyzeResponse(llm, app, http.StatusOK, respBody, reqBody, req)

	// Wait for analytics to process the request
	waitForAnalytics(t, db, 1)
	waitUntilIdle(t, db)
	record := waitForRecordWithCost(t, db)
	require.NotNil(t, record)
	assert.Equal(t, app.ID, record.AppID)
	assert.Equal(t, llm.ID, record.LLMID)
	assert.Equal(t, string(models.MOCK_VENDOR), record.Vendor)
	assert.Equal(t, 5, record.PromptTokens)
	assert.Equal(t, 10, record.ResponseTokens)
	assert.Equal(t, 15, record.TotalTokens)
	assert.Equal(t, 1, record.Choices)
	assert.Equal(t, 0.025, record.Cost) // (10 * 0.002) + (5 * 0.001)
}

func TestAnalyzeStreamingResponse(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := services.NewBudgetService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetService)
	require.NotNil(t, proxy)

	// Create test user
	user := &models.User{
		ID:    1,
		Email: "test@example.com",
	}
	err := db.Create(user).Error
	require.NoError(t, err)

	// Create test LLM
	llm := &models.LLM{
		Model:        gorm.Model{ID: 1},
		Name:         "TestLLM",
		Vendor:       models.MOCK_VENDOR,
		DefaultModel: "test-model",
		Active:       true,
		APIEndpoint:  "http://mock-api.example.com",
	}
	err = db.Create(llm).Error
	require.NoError(t, err)

	// Create test app
	app := &models.App{
		Model:  gorm.Model{ID: 1},
		Name:   "TestApp",
		UserID: user.ID,
	}
	err = db.Create(app).Error
	require.NoError(t, err)

	// Create test request
	reqBody := []byte(`{"prompt": "Hello"}`)
	req, _ := http.NewRequest("POST", "http://example.com/test", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	// Test streaming response with chunks in the same format as the mock server
	chunks := [][]byte{
		[]byte(`{"id":"mock-123","object":"chat.completion.chunk","choices":[{"delta":{"content":"Hello"}}]}`),
		[]byte(`{"id":"mock-123","object":"chat.completion.chunk","choices":[{"delta":{"content":" World"}}]}`),
		[]byte(`{"id":"mock-123","object":"chat.completion","model":"test-model","choices":[{"message":{"content":"Hello World"}}],"usage":{"prompt_tokens":5,"completion_tokens":10,"total_tokens":15}}`),
	}
	// Create the full response that would be sent
	fullResponse := []byte(`{"id":"mock-123","object":"chat.completion","model":"test-model","choices":[{"message":{"content":"Hello World"}}],"usage":{"prompt_tokens":5,"completion_tokens":10,"total_tokens":15}}`)

	// Create a mock price
	price := &models.ModelPrice{
		Model:     gorm.Model{ID: 1},
		ModelName: "test-model",
		Vendor:    string(models.MOCK_VENDOR),
		CPT:       0.002,
		CPIT:      0.001,
		Currency:  "USD",
	}
	err = db.Create(price).Error
	require.NoError(t, err)

	// Test streaming response analysis
	now := time.Now()
	proxy.analyzeStreamingResponse(llm, app, req, http.StatusOK, fullResponse, reqBody, chunks, now)

	// Wait for analytics to process the request
	waitForAnalytics(t, db, 1)
	waitUntilIdle(t, db)
	record := waitForRecordWithCost(t, db)
	require.NotNil(t, record)

	// Verify record fields
	assert.Equal(t, app.ID, record.AppID)
	assert.Equal(t, llm.ID, record.LLMID)
	assert.Equal(t, string(models.MOCK_VENDOR), record.Vendor)
	assert.Equal(t, 5, record.PromptTokens)
	assert.Equal(t, 10, record.ResponseTokens)
	assert.Equal(t, 15, record.TotalTokens)
	assert.Equal(t, 0.025, record.Cost) // (10 * 0.002) + (5 * 0.001)
}

type mockTokenResponse struct {
	model      string
	prompt     int
	resp       int
	total      int
	choices    int
	tools      int
	cacheWrite int
	cacheRead  int
}

func (m *mockTokenResponse) GetModel() string {
	return m.model
}

func (m *mockTokenResponse) GetPromptTokens() int {
	return m.prompt
}

func (m *mockTokenResponse) GetResponseTokens() int {
	return m.resp
}

func (m *mockTokenResponse) GetTotalTokens() int {
	return m.total
}

func (m *mockTokenResponse) GetChoiceCount() int {
	return m.choices
}

func (m *mockTokenResponse) GetToolCount() int {
	return m.tools
}

func (m *mockTokenResponse) GetCacheWritePromptTokens() int {
	return m.cacheWrite
}

func (m *mockTokenResponse) GetCacheReadPromptTokens() int {
	return m.cacheRead
}

func TestAnalyzeCompletionResponseWithCache(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := services.NewBudgetService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetService)
	require.NotNil(t, proxy)

	// Create test user
	user := &models.User{
		ID:    1,
		Email: "test@example.com",
	}
	err := db.Create(user).Error
	require.NoError(t, err)

	// Create test LLM
	llm := &models.LLM{
		Model:        gorm.Model{ID: 1},
		Name:         "TestLLM",
		Vendor:       models.MOCK_VENDOR,
		DefaultModel: "test-model",
		Active:       true,
		APIEndpoint:  "http://mock-api.example.com",
	}
	err = db.Create(llm).Error
	require.NoError(t, err)

	// Create test app
	app := &models.App{
		Model:  gorm.Model{ID: 1},
		Name:   "TestApp",
		UserID: user.ID,
	}
	err = db.Create(app).Error
	require.NoError(t, err)

	// Create a mock price
	price := &models.ModelPrice{
		Model:     gorm.Model{ID: 1},
		ModelName: "test-model",
		Vendor:    string(models.MOCK_VENDOR),
		CPT:       0.002,
		CPIT:      0.001,
		Currency:  "USD",
	}
	err = db.Create(price).Error
	require.NoError(t, err)

	// Create mock response with cache tokens
	mockResp := &mockTokenResponse{
		model:      "test-model",
		prompt:     5,
		resp:       10,
		choices:    1,
		tools:      0,
		cacheWrite: 3, // Cache write tokens
		cacheRead:  2, // Cache read tokens
	}

	// Test response analysis
	AnalyzeCompletionResponse(service, llm, app, mockResp, time.Now())

	// Wait for analytics to process the request
	waitForAnalytics(t, db, 1)
	waitUntilIdle(t, db)
	record := waitForRecordWithCost(t, db)
	require.NotNil(t, record)
	assert.Equal(t, app.ID, record.AppID)
	assert.Equal(t, llm.ID, record.LLMID)
	assert.Equal(t, string(models.MOCK_VENDOR), record.Vendor)
	assert.Equal(t, 5, record.PromptTokens)
	assert.Equal(t, 10, record.ResponseTokens)
	assert.Equal(t, 20, record.TotalTokens) // 5 prompt + 10 response + 3 cache write + 2 cache read
	assert.Equal(t, 1, record.Choices)
	assert.Equal(t, 0.025, record.Cost) // (10 * 0.002) + (5 * 0.001)
}

func TestAnalyzeCompletionResponse(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := services.NewBudgetService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetService)
	require.NotNil(t, proxy)

	// Create test user
	user := &models.User{
		ID:    1,
		Email: "test@example.com",
	}
	err := db.Create(user).Error
	require.NoError(t, err)

	// Create test LLM
	llm := &models.LLM{
		Model:        gorm.Model{ID: 1},
		Name:         "TestLLM",
		Vendor:       models.MOCK_VENDOR,
		DefaultModel: "test-model",
		Active:       true,
		APIEndpoint:  "http://mock-api.example.com",
	}
	err = db.Create(llm).Error
	require.NoError(t, err)

	// Create test app
	app := &models.App{
		Model:  gorm.Model{ID: 1},
		Name:   "TestApp",
		UserID: user.ID,
	}
	err = db.Create(app).Error
	require.NoError(t, err)

	// Create a mock price
	price := &models.ModelPrice{
		Model:     gorm.Model{ID: 1},
		ModelName: "test-model",
		Vendor:    string(models.MOCK_VENDOR),
		CPT:       0.002,
		CPIT:      0.001,
		Currency:  "USD",
	}
	err = db.Create(price).Error
	require.NoError(t, err)

	// Create test request and response
	reqBody := []byte(`{"model": "test-model", "prompt": "Hello"}`)
	req, _ := http.NewRequest("POST", "http://example.com/test", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	respBody := []byte(`{
		"id": "mock-123",
		"object": "chat.completion",
		"model": "test-model",
		"choices": [{"message": {"content": "Hello"}}],
		"usage": {"prompt_tokens":5,"completion_tokens":10,"total_tokens":15}
	}`)

	// Test response analysis
	AnalyzeResponse(service, llm, app, http.StatusOK, respBody, reqBody, req)

	// Wait for analytics to process the request
	waitForAnalytics(t, db, 1)
	waitUntilIdle(t, db)
	record := waitForRecordWithCost(t, db)
	require.NotNil(t, record)
	assert.Equal(t, app.ID, record.AppID)
	assert.Equal(t, llm.ID, record.LLMID)
	assert.Equal(t, string(models.MOCK_VENDOR), record.Vendor)
	assert.Equal(t, 5, record.PromptTokens)
	assert.Equal(t, 10, record.ResponseTokens)
	assert.Equal(t, 15, record.TotalTokens)
	assert.Equal(t, 1, record.Choices)
	assert.Equal(t, 0.025, record.Cost) // (10 * 0.002) + (5 * 0.001)
}
