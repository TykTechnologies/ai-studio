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

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
)

func TestLLMRequestHandling(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := budget.NewService(db, notificationSvc)
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

	// Mock upstream
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.Body = io.NopCloser(bytes.NewBuffer(body))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "mock-123",
			"object": "chat.completion",
			"model": "test-model",
			"choices": [{"message": {"content": "Hello"}}],
			"usage": {"prompt_tokens":5,"completion_tokens":10,"total_tokens":15}
		}`))
	}))
	defer mockUpstream.Close()

	llm.APIEndpoint = mockUpstream.URL
	err = db.Save(llm).Error
	require.NoError(t, err)

	// Force reload with mockUpstream URL added
	err = proxy.loadResources()
	require.NoError(t, err)

	proxy.credValidator.RegisterValidator(string(models.MOCK_VENDOR), func(r *http.Request) (string, error) {
		token := r.Header.Get("Authorization")
		return strings.TrimPrefix(token, "Bearer "), nil
	})

	app := &models.App{
		Model:  gorm.Model{ID: 1},
		Name:   "TestApp",
		UserID: user.ID,
	}
	err = db.Create(app).Error
	require.NoError(t, err)

	cred := &models.Credential{
		Model:  gorm.Model{ID: 1},
		Secret: "valid-token",
		Active: true,
	}
	err = db.Create(cred).Error
	require.NoError(t, err)

	app.CredentialID = cred.ID
	app.LLMs = []models.LLM{*llm}
	err = db.Save(app).Error
	require.NoError(t, err)

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
	srv := httptest.NewServer(proxy.credValidator.Middleware(r))
	defer srv.Close()

	reqBody := []byte(`{"prompt": "Hello"}`)
	req, _ := http.NewRequest("POST", srv.URL+"/llm/rest/testllm/v1/test", bytes.NewBuffer(reqBody))
	req.Header.Set("Authorization", "Bearer valid-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(reqBody)))
	req = req.WithContext(context.WithValue(req.Context(), "app", app))

	resp, err := http.DefaultClient.Do(req)
	assert.Nil(t, err)
	assert.NotNil(t, resp)
}

func TestDatasourceRequestHandling(t *testing.T) {
	// ...
}

// TODO:
// 1. Add case with processing `key` query param and x-goog-api-key header
// 2. Make sure that valid Authorization header is enough to proxy Google AI requests
// 3. Test all proxy routes including /ai/
// 4. Add assertion to check that mockGAPIServer doesn't receive Authorization header if provided

func TestGoogleAIRequestHandling(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	mockGoogleAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := `
		{
				"candidates": [
						{
								"content": {
										"parts": [
												{
														"text": "Hello! How can I help you today?"
												}
										],
										"role": "model"
								},
								"finishReason": "STOP",
								"index": 0
						}
				],
				"usageMetadata": {
						"promptTokenCount": 3,
						"candidatesTokenCount": 9,
						"totalTokenCount": 227,
						"promptTokensDetails": [
								{
										"modality": "TEXT",
										"tokenCount": 3
								}
						],
						"thoughtsTokenCount": 215
				},
				"modelVersion": "gemini-2.5-flash",
				"responseId": "tfONacGUIq-D28oPjPSgsAs"
		}
		`

		_, err := w.Write([]byte(response))
		require.NoError(t, err)
	}))
	defer mockGoogleAIServer.Close()

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := budget.NewService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetService)
	require.NotNil(t, proxy)

	user := &models.User{ID: 1, Email: "test@example.com"}
	db.Create(user)

	googleAILLM := &models.LLM{
		Model:        gorm.Model{ID: 1},
		Name:         "gemini-api",
		Vendor:       models.GOOGLEAI,
		DefaultModel: "gemini-1.5-flash",
		Active:       true,
		APIEndpoint:  mockGoogleAIServer.URL,
	}
	err := db.Create(googleAILLM).Error
	require.NoError(t, err)

	app := &models.App{
		Model:  gorm.Model{ID: 1},
		LLMs:   []models.LLM{*googleAILLM},
		UserID: user.ID,
	}
	db.Create(app)

	cred := &models.Credential{Model: gorm.Model{ID: 1}, Secret: "valid-api-token", Active: true}
	db.Create(cred)

	app.CredentialID = cred.ID
	app.LLMs = []models.LLM{*googleAILLM}
	db.Save(app)

	err = proxy.loadResources()
	require.NoError(t, err)

	r := mux.NewRouter()
	finalHandler := proxy.credValidator.Middleware(
		proxy.streamDetectionMiddleware(
			http.HandlerFunc(proxy.handleUnifiedLLMRequest),
		),
	)
	r.Handle("/llm/call/{llmSlug}/{rest:.*}", finalHandler)
	proxySrv := httptest.NewServer(r)
	defer proxySrv.Close()

	reqBody := []byte(`{"model": "gemini-1.5-flash", "messages": [{"role": "user", "content": "Hello, world!"}]}`)
	url := fmt.Sprintf("%s/llm/call/%s/v1beta/models/gemini-1.5-flash:generateContent", proxySrv.URL, googleAILLM.Name)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	assert.NoError(t, err)
	req.Header.Set("Authorization", "Bearer valid-api-token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() {
		err := resp.Body.Close()
		assert.NoError(t, err)
	}()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	respBody, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(respBody), "Hello! How can I help you today?")
	// TODO: Add more assertions
}
