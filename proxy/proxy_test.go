package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockService is a mock implementation of the ServiceInterface
type MockService struct {
	mock.Mock
}

func (m *MockService) GetActiveLLMs() ([]models.LLM, error) {
	args := m.Called()
	return args.Get(0).([]models.LLM), args.Error(1)
}

func (m *MockService) GetLLMByID(id uint) (*models.LLM, error) {
	args := m.Called(id)
	return args.Get(0).(*models.LLM), args.Error(1)
}

func (m *MockService) GetActiveDatasources() ([]models.Datasource, error) {
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
	args := m.Called(modelName)
	return args.Get(0).(*models.ModelPrice), args.Error(1)
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
		{ID: 1, Name: "DummyLLM", Vendor: "DUMMY", APIEndpoint: upstream.URL, APIKey: "dummyapikey"},
	}, nil)
	mockService.On("GetActiveDatasources").Return([]models.Datasource{}, nil)
	mockService.On("GetCredentialBySecret", "valid-token").Return(&models.Credential{ID: 1, Active: true}, nil)
	mockService.On("GetCredentialBySecret", "invalid-token").Return((*models.Credential)(nil), fmt.Errorf("invalid credential"))
	mockService.On("GetAppByCredentialID", uint(1)).Return(&models.App{ID: 1, LLMs: []models.LLM{{ID: 1}}}, nil)

	config := &Config{Port: 8080}
	proxy := NewProxy(mockService, config)
	proxy.credValidator.RegisterValidator("dummy", DummyValidator)

	// Explicitly load resources
	err := proxy.loadResources()
	assert.NoError(t, err)

	// Create a test server
	router := mux.NewRouter()
	router.HandleFunc("/llm/{llmSlug}/{rest:.*}", proxy.handleLLMRequest).Methods("POST")
	testServer := httptest.NewServer(proxy.credValidator.Middleware(router))
	defer testServer.Close()

	// Test valid request
	reqBody := []byte(`{"prompt": "Hello, world!"}`)
	req, _ := http.NewRequest("POST", testServer.URL+"/llm/dummyllm/v1/chat/completions", bytes.NewBuffer(reqBody))
	req.Header.Set("Dummy-Authorization", "valid-token")
	resp, err := http.DefaultClient.Do(req)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Check the response from the upstream server
	body, err := ioutil.ReadAll(resp.Body)
	assert.NoError(t, err)
	var responseBody map[string]interface{}
	err = json.Unmarshal(body, &responseBody)
	assert.NoError(t, err)
	assert.Equal(t, "chatcmpl-123", responseBody["id"])
	assert.Equal(t, "Hello, how can I assist you today?", responseBody["choices"].([]interface{})[0].(map[string]interface{})["message"].(map[string]interface{})["content"])

	// Test invalid credential
	req, _ = http.NewRequest("POST", testServer.URL+"/llm/dummyllm/v1/chat/completions", bytes.NewBuffer(reqBody))
	req.Header.Set("Dummy-Authorization", "invalid-token")
	resp, err = http.DefaultClient.Do(req)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Test missing credential
	req, _ = http.NewRequest("POST", testServer.URL+"/llm/dummyllm/v1/chat/completions", bytes.NewBuffer(reqBody))
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
	assert.True(t, proxy.credValidator.CheckCredential("valid-token", "", "dummyllm", r))

	// Test valid Datasource credential
	assert.True(t, proxy.credValidator.CheckCredential("valid-token", "dummyds", "", r))

	// Test invalid credential for LLM
	mockService.On("GetCredentialBySecret", "invalid-token").Return(&models.Credential{}, fmt.Errorf("invalid credential"))
	assert.False(t, proxy.credValidator.CheckCredential("invalid-token", "", "dummyllm", r))

	// Test invalid credential for Datasource
	assert.False(t, proxy.credValidator.CheckCredential("invalid-token", "dummyds", "", r))

	// Test non-existent LLM
	assert.False(t, proxy.credValidator.CheckCredential("valid-token", "", "nonexistentllm", r))

	// Test non-existent Datasource
	assert.False(t, proxy.credValidator.CheckCredential("valid-token", "nonexistentds", "", r))
}
