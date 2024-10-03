package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// MockService is a mock implementation of the ServiceInterface
type MockService struct {
	mock.Mock
}

func (m *MockService) GetActiveLLMs() (models.LLMs, error) {
	args := m.Called()
	return args.Get(0).([]models.LLM), args.Error(1)
}

func (m *MockService) GetLLMByID(id uint) (*models.LLM, error) {
	args := m.Called(id)
	return args.Get(0).(*models.LLM), args.Error(1)
}

func (m *MockService) GetActiveDatasources() (models.Datasources, error) {
	args := m.Called()
	return args.Get(0).([]models.Datasource), args.Error(1)
}

func (m *MockService) GetDatasourceByID(id uint) (*models.Datasource, error) {
	args := m.Called(id)
	return args.Get(0).(*models.Datasource), args.Error(1)
}

func (m *MockService) GetCredentialBySecret(secret string) (*models.Credential, error) {
	args := m.Called(secret)
	return args.Get(0).(*models.Credential), args.Error(1)
}

func (m *MockService) GetAppByCredentialID(credID uint) (*models.App, error) {
	args := m.Called(credID)
	return args.Get(0).(*models.App), args.Error(1)
}

func (m *MockService) GetModelPriceByModelNameAndVendor(modelName, vendor string) (*models.ModelPrice, error) {
	args := m.Called(modelName, vendor)
	return args.Get(0).(*models.ModelPrice), args.Error(1)
}

func setupDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&analytics.LLMChatRecord{})
	require.NoError(t, err)

	return db
}

// TestProxySetup tests the initial setup of the proxy
func TestProxySetup(t *testing.T) {
	mockService := new(MockService)
	mockService.On("GetActiveLLMs").Return([]models.LLM{
		{ID: 1, Name: "DummyLLM", Vendor: "DUMMY", APIEndpoint: "http://dummy-llm.com"},
	}, nil)
	mockService.On("GetActiveDatasources").Return([]models.Datasource{}, nil)

	config := &Config{Port: 8080}
	proxy := NewProxy(mockService, config)

	assert.NotNil(t, proxy)
	assert.Equal(t, 8080, proxy.config.Port)
	assert.NotNil(t, proxy.credValidator)
}

// TestLLMRequestHandling tests the handling of LLM requests
func TestLLMRequestHandling(t *testing.T) {
	// Setup mock upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if the request was properly forwarded
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer dummyapikey", r.Header.Get("Authorization"))

		// Read the request body
		body, err := ioutil.ReadAll(r.Body)
		assert.NoError(t, err)

		// Check if the body was forwarded correctly
		var requestBody map[string]interface{}
		err = json.Unmarshal(body, &requestBody)
		assert.NoError(t, err)
		assert.Equal(t, "Hello, world!", requestBody["prompt"])

		// Send a mock response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := map[string]interface{}{
			"id":      "chatcmpl-123",
			"object":  "chat.completion",
			"created": 1677652288,
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Hello, how can I assist you today?",
					},
					"finish_reason": "stop",
				},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer upstream.Close()

	mockService := new(MockService)
	mockService.On("GetActiveLLMs").Return([]models.LLM{
		{ID: 1, Name: "DummyLLM", Vendor: models.MOCK_VENDOR, APIEndpoint: upstream.URL, APIKey: "dummyapikey"},
	}, nil)
	mockService.On("GetActiveDatasources").Return([]models.Datasource{}, nil)
	mockService.On("GetCredentialBySecret", "valid-token").Return(&models.Credential{ID: 1, Active: true}, nil)
	mockService.On("GetCredentialBySecret", "invalid-token").Return((*models.Credential)(nil), fmt.Errorf("invalid credential"))
	mockService.On("GetAppByCredentialID", uint(1)).Return(&models.App{ID: 1, LLMs: []models.LLM{{ID: 1}}}, nil)
	mockService.On("GetModelPriceByModelNameAndVendor", mock.Anything, mock.Anything).Return(&models.ModelPrice{CPT: 0.001}, nil)

	config := &Config{Port: 8080}
	proxy := NewProxy(mockService, config)
	proxy.credValidator.RegisterValidator("dummy", DummyValidator)

	// Explicitly load resources
	err := proxy.loadResources()
	assert.NoError(t, err)

	// Create a test server
	router := mux.NewRouter()
	router.HandleFunc("/llm/rest/{llmSlug}/{rest:.*}", proxy.handleLLMRequest).Methods("POST")
	testServer := httptest.NewServer(proxy.credValidator.Middleware(router))
	defer testServer.Close()

	// Test valid request
	reqBody := []byte(`{"prompt": "Hello, world!"}`)
	req, _ := http.NewRequest("POST", testServer.URL+"/llm/rest/dummyllm/v1/chat/completions", bytes.NewBuffer(reqBody))
	req.Header.Set("Authorization", "valid-token")
	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Check the response from the upstream server
	body, err := ioutil.ReadAll(resp.Body)
	fmt.Println(string(body))
	assert.NoError(t, err)
	var responseBody map[string]interface{}
	err = json.Unmarshal(body, &responseBody)
	assert.NoError(t, err)
	assert.Equal(t, "chatcmpl-123", responseBody["id"])

	if responseBody != nil {
		choices, ok := responseBody["choices"].([]interface{})
		if ok {
			if len(choices) > 0 {
				assert.Equal(t, "Hello, how can I assist you today?", choices[0].(map[string]interface{})["message"].(map[string]interface{})["content"])
			}
		}
	}

	// Test invalid credential
	req, _ = http.NewRequest("POST", testServer.URL+"/llm/rest/dummyllm/v1/chat/completions", bytes.NewBuffer(reqBody))
	req.Header.Set("Authorization", "invalid-token")
	resp, err = http.DefaultClient.Do(req)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Test missing credential
	req, _ = http.NewRequest("POST", testServer.URL+"/llm/rest/dummyllm/v1/chat/completions", bytes.NewBuffer(reqBody))
	resp, err = http.DefaultClient.Do(req)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestDatasourceRequestHandling tests the handling of datasource requests
func TestDatasourceRequestHandling(t *testing.T) {
	mockService := new(MockService)
	mockService.On("GetActiveLLMs").Return([]models.LLM{}, nil)
	mockService.On("GetActiveDatasources").Return([]models.Datasource{
		{ID: 1, Name: "DummyDS"},
	}, nil)
	mockService.On("GetCredentialBySecret", "valid-token").Return(&models.Credential{ID: 1, Active: true}, nil)
	mockService.On("GetCredentialBySecret", "invalid-token").Return((*models.Credential)(nil), fmt.Errorf("invalid credential"))
	mockService.On("GetAppByCredentialID", uint(1)).Return(&models.App{ID: 1, Datasources: []models.Datasource{{ID: 1}}}, nil)

	config := &Config{Port: 8080}
	proxy := NewProxy(mockService, config)

	// Explicitly load resources
	err := proxy.loadResources()
	assert.NoError(t, err)

	// Create a test server
	router := mux.NewRouter()
	router.HandleFunc("/datasource/{dsSlug}", proxy.handleDatasourceRequest).Methods("POST")
	testServer := httptest.NewServer(proxy.credValidator.Middleware(router))
	defer testServer.Close()

	// Test valid request
	reqBody := []byte(`{"query": "test query", "n": 5}`)
	req, _ := http.NewRequest("POST", testServer.URL+"/datasource/dummyds", bytes.NewBuffer(reqBody))
	req.Header.Set("Authorization", "valid-token")
	resp, err := http.DefaultClient.Do(req)

	assert.NoError(t, err)

	// There is no mock embedding handler
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	// Test invalid credential
	req, _ = http.NewRequest("POST", testServer.URL+"/datasource/dummyds", bytes.NewBuffer(reqBody))
	req.Header.Set("Authorization", "invalid-token")
	resp, err = http.DefaultClient.Do(req)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestProxyReload tests the reloading of proxy resources
func TestProxyReload(t *testing.T) {
	mockService := new(MockService)
	mockService.On("GetActiveLLMs").Return([]models.LLM{
		{ID: 1, Name: "DummyLLM", Vendor: "DUMMY", APIEndpoint: "http://dummy-llm.com"},
	}, nil).Once()
	mockService.On("GetActiveDatasources").Return([]models.Datasource{}, nil).Once()

	config := &Config{Port: 8080}
	proxy := NewProxy(mockService, config)

	// Explicitly load resources
	err := proxy.loadResources()
	assert.NoError(t, err)

	assert.Len(t, proxy.llms, 1)
	assert.Len(t, proxy.datasources, 0)

	// Mock new data for reload
	mockService.On("GetActiveLLMs").Return([]models.LLM{
		{ID: 1, Name: "DummyLLM", Vendor: "DUMMY", APIEndpoint: "http://dummy-llm.com"},
		{ID: 2, Name: "NewLLM", Vendor: "DUMMY", APIEndpoint: "http://new-llm.com"},
	}, nil).Once()
	mockService.On("GetActiveDatasources").Return([]models.Datasource{
		{ID: 1, Name: "NewDS"},
	}, nil).Once()

	err = proxy.Reload()
	assert.NoError(t, err)

	assert.Len(t, proxy.llms, 2)
	assert.Len(t, proxy.datasources, 1)
}

// TestCredentialValidation tests the credential validation process
func TestCredentialValidation(t *testing.T) {
	mockService := new(MockService)
	mockService.On("GetActiveLLMs").Return([]models.LLM{
		{ID: 1, Name: "DummyLLM", Vendor: "DUMMY", APIEndpoint: "http://dummy-llm.com"},
	}, nil)
	mockService.On("GetActiveDatasources").Return([]models.Datasource{
		{ID: 1, Name: "DummyDS"},
	}, nil)
	mockService.On("GetCredentialBySecret", "valid-token").Return(&models.Credential{ID: 1, Active: true}, nil)
	mockService.On("GetAppByCredentialID", uint(1)).Return(&models.App{
		ID:          1,
		LLMs:        []models.LLM{{ID: 1}},
		Datasources: []models.Datasource{{ID: 1}},
	}, nil)

	config := &Config{Port: 8080}
	proxy := NewProxy(mockService, config)
	proxy.credValidator.RegisterValidator("dummy", DummyValidator)

	// Explicitly load resources
	err := proxy.loadResources()
	assert.NoError(t, err)

	r, err := http.NewRequest("POST", "http://goo.bar/baz", bytes.NewBuffer([]byte("")))
	assert.NoError(t, err)

	// Test valid LLM credential
	v, r := proxy.credValidator.CheckCredential("valid-token", "", "dummyllm", r)
	assert.True(t, v)

	// Test valid Datasource credential
	v, r = proxy.credValidator.CheckCredential("valid-token", "dummyds", "", r)
	assert.True(t, v)

	// Test invalid credential for LLM
	mockService.On("GetCredentialBySecret", "invalid-token").Return(&models.Credential{}, fmt.Errorf("invalid credential"))
	v, r = proxy.credValidator.CheckCredential("invalid-token", "", "dummyllm", r)
	assert.False(t, v)

	// Test invalid credential for Datasource
	v, r = proxy.credValidator.CheckCredential("invalid-token", "dummyds", "", r)
	assert.False(t, v)

	// Test non-existent LLM
	v, r = proxy.credValidator.CheckCredential("valid-token", "", "nonexistentllm", r)
	assert.False(t, v)

	// Test non-existent Datasource
	v, r = proxy.credValidator.CheckCredential("valid-token", "nonexistentds", "", r)
	assert.False(t, v)
}

func TestOutboundRequestMiddleware(t *testing.T) {
	mockService := new(MockService)
	mockService.On("GetActiveLLMs").Return([]models.LLM{}, nil)
	mockService.On("GetActiveDatasources").Return([]models.Datasource{}, nil)

	config := &Config{Port: 8080}
	proxy := NewProxy(mockService, config)

	blockedFilter := &models.Filter{
		Name: "BlockedWordFilter",
		Script: []byte(`
			text := import("text")
			fmt := import("fmt")
			filter := func(p) {
				if text.contains(p, "blocked") {
					return false
				}
				return true
			}
			result := filter(payload)
			`),
	}
	proxy.AddFilter(blockedFilter)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := proxy.outboundRequestMiddleware(testHandler)

	// Test passing request
	req := httptest.NewRequest("POST", "/test", strings.NewReader(`{"message": "Hello, world!"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Test blocked request
	req = httptest.NewRequest("POST", "/test", strings.NewReader(`{"message": "This should be blocked"}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestLoadResourcesError(t *testing.T) {
	mockService := new(MockService)
	mockService.On("GetActiveLLMs").Return([]models.LLM{}, fmt.Errorf("LLM error"))
	mockService.On("GetActiveDatasources").Return([]models.Datasource{}, fmt.Errorf("Datasource error"))

	config := &Config{Port: 8080}
	proxy := NewProxy(mockService, config)

	err := proxy.loadResources()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get LLMs")
}

func TestConcurrentAccess(t *testing.T) {
	mockService := new(MockService)
	mockService.On("GetActiveLLMs").Return([]models.LLM{{ID: 1, Name: "TestLLM"}}, nil)
	mockService.On("GetActiveDatasources").Return([]models.Datasource{{ID: 1, Name: "TestDS"}}, nil)

	config := &Config{Port: 8080}
	proxy := NewProxy(mockService, config)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Add(-1)
			_, _ = proxy.GetLLM("TestLLM")
			_, _ = proxy.GetDatasource("TestDS")
		}()
	}
	wg.Wait()
}

func TestEdgeCasesRequests(t *testing.T) {
	mockService := new(MockService)
	mockService.On("GetActiveLLMs").Return([]models.LLM{{ID: 1, Name: "TestLLM", APIEndpoint: "http://test-llm.com"}}, nil)
	mockService.On("GetActiveDatasources").Return([]models.Datasource{{ID: 1, Name: "TestDS"}}, nil)
	mockService.On("GetCredentialBySecret", mock.Anything).Return(&models.Credential{ID: 1, Active: true}, nil)
	mockService.On("GetAppByCredentialID", uint(1)).Return(&models.App{ID: 1, LLMs: []models.LLM{{ID: 1}}, Datasources: []models.Datasource{{ID: 1}}}, nil)

	config := &Config{Port: 8080}
	proxy := NewProxy(mockService, config)
	proxy.loadResources()

	router := mux.NewRouter()
	router.HandleFunc("/llm/{llmSlug}/{rest:.*}", proxy.handleLLMRequest).Methods("POST")
	router.HandleFunc("/datasource/{dsSlug}", proxy.handleDatasourceRequest).Methods("POST")
	testServer := httptest.NewServer(proxy.credValidator.Middleware(router))
	defer testServer.Close()

	// Test malformed LLM request body
	req, _ := http.NewRequest("POST", testServer.URL+"/llm/rest/testllm/test", strings.NewReader(`{"invalid json`))
	req.Header.Set("Authorization", "valid-token")
	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Test non-existent LLM
	req, _ = http.NewRequest("POST", testServer.URL+"/llm/rest/nonexistent/test", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "valid-token")
	resp, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// Test malformed datasource request body
	req, _ = http.NewRequest("POST", testServer.URL+"/datasource/testds", strings.NewReader(`{"invalid json`))
	req.Header.Set("Authorization", "valid-token")
	resp, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

}

func TestAnalyzeResponse(t *testing.T) {
	// Set up a test database
	db := setupDB(t)

	// Start recording analytics
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	analytics.StartRecording(ctx, db)

	mockService := new(MockService)
	mockService.On("GetModelPriceByModelNameAndVendor", mock.Anything, mock.Anything).Return(&models.ModelPrice{CPT: 0.001}, nil)

	config := &Config{Port: 8080}
	proxy := NewProxy(mockService, config)

	llm := &models.LLM{ID: 1, Name: "TestLLM", Vendor: models.MOCK_VENDOR}
	app := &models.App{ID: 1, UserID: 1}
	statusCode := 200
	body := []byte(`{"model": "test-model", "usage": {"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30}}`)
	r, _ := http.NewRequest("POST", "http://test.com", nil)

	proxy.analyzeResponse(llm, app, statusCode, body, r)

	// Wait a bit for the goroutine to process the record
	time.Sleep(100 * time.Millisecond)

	// Retrieve the recorded analytics
	var recordedAnalytics analytics.LLMChatRecord
	result := db.First(&recordedAnalytics)
	assert.NoError(t, result.Error)

	assert.Equal(t, "mock", recordedAnalytics.Vendor)
	assert.Equal(t, 10, recordedAnalytics.PromptTokens)
	assert.Equal(t, 20, recordedAnalytics.ResponseTokens)
	assert.Equal(t, 30, recordedAnalytics.TotalTokens)
	assert.Equal(t, uint(1), recordedAnalytics.AppID)
	assert.Equal(t, uint(1), recordedAnalytics.UserID)
	assert.InDelta(t, 0.03, recordedAnalytics.Cost, 0.001)
}

func TestSetVendorAuthHeader(t *testing.T) {
	mockService := new(MockService)
	config := &Config{Port: 8080}
	proxy := NewProxy(mockService, config)

	testCases := []struct {
		vendor   models.Vendor
		apiKey   string
		expected string
	}{
		{models.OPENAI, "test-openai-key", "Bearer test-openai-key"},
		{models.ANTHROPIC, "test-anthropic-key", "test-anthropic-key"},
		// Add more cases for other vendors
	}

	for _, tc := range testCases {
		llm := &models.LLM{Vendor: tc.vendor, APIKey: tc.apiKey}
		req, _ := http.NewRequest("POST", "http://test.com", nil)

		err := proxy.setVendorAuthHeader(req, llm)
		assert.NoError(t, err)

		switch tc.vendor {
		case models.OPENAI:
			assert.Equal(t, tc.expected, req.Header.Get("Authorization"))
		case models.ANTHROPIC:
			assert.Equal(t, tc.expected, req.Header.Get("x-api-key"))
			// Add more cases for other vendors
		}
	}
}

func TestErrorResponses(t *testing.T) {
	w := httptest.NewRecorder()

	testCases := []struct {
		status  int
		message string
		err     error
	}{
		{http.StatusBadRequest, "Bad Request", fmt.Errorf("invalid input")},
		{http.StatusUnauthorized, "Unauthorized", nil},
		{http.StatusForbidden, "Forbidden", fmt.Errorf("access denied")},
		{http.StatusInternalServerError, "Internal Server Error", fmt.Errorf("something went wrong")},
	}

	for _, tc := range testCases {
		w = httptest.NewRecorder()
		respondWithError(w, tc.status, tc.message, tc.err)

		resp := w.Result()
		body, _ := ioutil.ReadAll(resp.Body)

		assert.Equal(t, tc.status, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

		var errorResp ErrorResponse
		err := json.Unmarshal(body, &errorResp)
		assert.NoError(t, err)

		assert.Equal(t, tc.status, errorResp.Status)
		assert.Equal(t, tc.message, errorResp.Message)
		if tc.err != nil {
			assert.Equal(t, tc.err.Error(), errorResp.Error)
		} else {
			assert.Empty(t, errorResp.Error)
		}
	}
}

func TestGetDatasourceAndLLM(t *testing.T) {
	mockService := new(MockService)
	mockService.On("GetActiveLLMs").Return([]models.LLM{{ID: 1, Name: "TestLLM"}}, nil)
	mockService.On("GetActiveDatasources").Return([]models.Datasource{{ID: 1, Name: "TestDS"}}, nil)

	config := &Config{Port: 8080}
	proxy := NewProxy(mockService, config)
	proxy.loadResources()

	// Test GetDatasource
	ds, ok := proxy.GetDatasource("testds")
	assert.True(t, ok)
	assert.NotNil(t, ds)
	assert.Equal(t, "TestDS", ds.Name)

	ds, ok = proxy.GetDatasource("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, ds)

	// Test GetLLM
	llm, ok := proxy.GetLLM("testllm")
	assert.True(t, ok)
	assert.NotNil(t, llm)
	assert.Equal(t, "TestLLM", llm.Name)

	llm, ok = proxy.GetLLM("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, llm)
}

func TestFilterScriptExecution(t *testing.T) {
	mockService := new(MockService)
	config := &Config{Port: 8080}
	proxy := NewProxy(mockService, config)

	testCases := []struct {
		name     string
		script   string
		payload  string
		expected bool
	}{
		{
			name:     "Allow all",
			script:   `result := true`,
			payload:  `{"message": "Hello"}`,
			expected: true,
		},
		{
			name:     "Block all",
			script:   `result := false`,
			payload:  `{"message": "Hello"}`,
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filter := &models.Filter{
				Name:   tc.name,
				Script: []byte(tc.script),
			}
			proxy.AddFilter(filter)

			runner := scripting.NewScriptRunner(filter.Script)
			err := runner.RunFilter(tc.payload)

			if tc.expected {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestHandleStreamingLLMRequest(t *testing.T) {
	// Setup mock upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("Expected http.ResponseWriter to be an http.Flusher")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		events := []string{
			"data: {\"content\":\"Hello\"}\n\n",
			"data: {\"content\":\"World\"}\n\n",
			"data: {\"content\":\"!\"}\n\n",
		}

		for _, event := range events {
			_, err := w.Write([]byte(event))
			if err != nil {
				t.Fatalf("Error writing to response: %v", err)
			}
			flusher.Flush()
			time.Sleep(100 * time.Millisecond)
		}
	}))
	defer upstream.Close()

	mockService := new(MockService)
	mockService.On("GetActiveLLMs").Return([]models.LLM{
		{ID: 1, Name: "StreamingLLM", Vendor: models.MOCK_VENDOR, APIEndpoint: upstream.URL, APIKey: "dummyapikey"},
	}, nil)
	mockService.On("GetActiveDatasources").Return([]models.Datasource{}, nil)
	mockService.On("GetCredentialBySecret", "valid-token").Return(&models.Credential{ID: 1, Active: true}, nil)
	mockService.On("GetCredentialBySecret", "invalid-token").Return(&models.Credential{ID: 0, Active: false}, nil)
	mockService.On("GetAppByCredentialID", uint(1)).Return(&models.App{ID: 1, LLMs: []models.LLM{{ID: 1}}}, nil)
	mockService.On("GetModelPriceByModelNameAndVendor", mock.Anything, mock.Anything).Return(&models.ModelPrice{CPT: 0.001}, nil)

	config := &Config{Port: 8080}
	proxy := NewProxy(mockService, config)
	proxy.credValidator.RegisterValidator("dummy", DummyValidator)

	err := proxy.loadResources()
	assert.NoError(t, err)

	router := mux.NewRouter()
	router.HandleFunc("/llm/stream/{llmSlug}/{rest:.*}", proxy.handleStreamingLLMRequest).Methods("POST")
	testServer := httptest.NewServer(proxy.credValidator.Middleware(router))
	defer testServer.Close()

	db := setupDB(t)
	analytics.StartRecording(context.Background(), db)

	t.Run("Valid streaming request", func(t *testing.T) {
		reqBody := []byte(`{"prompt": "Tell me a story"}`)
		u := testServer.URL + "/llm/stream/streamingllm/v1/chat/completions"
		// fmt.Println(u)
		req, _ := http.NewRequest("POST", u, bytes.NewBuffer(reqBody))
		req.Header.Set("Authorization", "valid-token")
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

		scanner := bufio.NewScanner(resp.Body)
		var events []string
		for scanner.Scan() {
			event := scanner.Text()
			if event != "" {
				events = append(events, event)
			}
		}

		assert.NoError(t, scanner.Err())
		assert.Len(t, events, 3)
		fmt.Println(events)
		if len(events) == 3 {
			assert.Contains(t, events[0], "Hello")
			assert.Contains(t, events[1], "World")
			assert.Contains(t, events[2], "!")
		}

	})

	t.Run("Invalid LLM", func(t *testing.T) {
		reqBody := []byte(`{"prompt": "Tell me a story"}`)
		req, _ := http.NewRequest("POST", testServer.URL+"/llm/stream/nonexistentllm/v1/chat/completions", bytes.NewBuffer(reqBody))
		req.Header.Set("Authorization", "valid-token")
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("Invalid credential", func(t *testing.T) {
		reqBody := []byte(`{"prompt": "Tell me a story"}`)
		req, _ := http.NewRequest("POST", testServer.URL+"/llm/stream/streamingllm/v1/chat/completions", bytes.NewBuffer(reqBody))
		req.Header.Set("Authorization", "invalid-token")
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		assert.NotNil(t, resp)
		if resp == nil {
			return
		}
		assert.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}
