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

func TestAuthFailures(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := budget.NewService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetService)
	require.NotNil(t, proxy)

	user := &models.User{ID: 1, Email: "test@example.com"}
	db.Create(user)

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("Upstream server should not be called in a failed auth test")
		w.WriteHeader(http.StatusOK)
	}))
	defer mockUpstream.Close()

	llm := &models.LLM{
		Model:        gorm.Model{ID: 1},
		Name:         "TestLLM",
		Vendor:       models.MOCK_VENDOR,
		DefaultModel: "test-model",
		Active:       true,
		APIEndpoint:  mockUpstream.URL,
	}
	db.Create(llm)

	app := &models.App{
		Model:  gorm.Model{ID: 1},
		LLMs:   []models.LLM{*llm},
		UserID: user.ID,
	}
	db.Create(app)

	validCred, err := service.CreateCredential()
	require.NoError(t, err)
	err = service.ActivateCredential(validCred.ID)
	require.NoError(t, err)

	inactiveCred, err := service.CreateCredential()
	require.NoError(t, err)

	credNotLinked, err := service.CreateCredential()
	require.NoError(t, err)
	err = service.ActivateCredential(credNotLinked.ID)
	require.NoError(t, err)

	app.CredentialID = validCred.ID
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

	tests := []struct {
		name               string
		setupRequest       func(r *http.Request)
		expectedStatusCode int
	}{
		{
			name: "Auth/Failure/NoToken",
			setupRequest: func(_ *http.Request) {
			},
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name: "Auth/Failure/InvalidToken",
			setupRequest: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer invalid-token")
			},
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name: "Auth/Failure/InactiveCredential",
			setupRequest: func(r *http.Request) {
				r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", inactiveCred.Secret))
			},
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name: "Auth/Failure/CredentialNotAssociatedWithApp",
			setupRequest: func(r *http.Request) {
				r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", credNotLinked.Secret))
			},
			expectedStatusCode: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := []byte(`{"messages": [{"role": "user", "content": "Hello"}]}`)
			url := fmt.Sprintf("%s/llm/call/%s/v1/chat/completions", proxySrv.URL, strings.ToLower(llm.Name))
			req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
			require.NoError(t, err)

			tt.setupRequest(req)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer func() {
				if resp != nil && resp.Body != nil {
					if err := resp.Body.Close(); err != nil {
						t.Logf("error closing response body: %v", err)
					}
				}
			}()

			assert.Equal(t, tt.expectedStatusCode, resp.StatusCode, "Expected status code to match")
		})
	}
}

func TestGoogleAIRequestHandling(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	mockGoogleAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Empty(t, r.Header.Get("Authorization"), "Authorization header must be omitted")
		require.Equal(t, "api-key", r.Header.Get("x-goog-api-key"))
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
		APIKey:       "api-key",
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
	url := fmt.Sprintf("%s/llm/call/%s/v1beta/models", proxySrv.URL, googleAILLM.Name)

	model := "gemini-2.5-flash"

	tests := []struct {
		name         string
		setupRequest func(r *http.Request)
		shouldFail   bool
		statusCode   int
	}{
		{
			name: "Auth/REST/WithAuthorizationHeader/OK",
			setupRequest: func(r *http.Request) {
				r.URL = r.URL.JoinPath(fmt.Sprintf("%s:%s", model, "generateContent"))
				r.Header.Set("Authorization", "Bearer valid-api-token")
			},
			statusCode: http.StatusOK,
		},
		{
			name: "Auth/Stream/WithAuthorizationHeader/OK",
			setupRequest: func(r *http.Request) {
				r.URL = r.URL.JoinPath(fmt.Sprintf("%s:%s", model, "streamGenerateContent"))
				r.Header.Set("Authorization", "Bearer valid-api-token")
			},
			statusCode: http.StatusOK,
		},
		{
			name: "Auth/REST/WithGoogAPIKeyHeader/OK",
			setupRequest: func(r *http.Request) {
				r.URL = r.URL.JoinPath(fmt.Sprintf("%s:%s", model, "generateContent"))
				r.Header.Set("x-goog-api-key", "valid-api-token")
			},
			statusCode: http.StatusOK,
		},
		{
			name: "Auth/Stream/WithGoogAPIKeyHeader/OK",
			setupRequest: func(r *http.Request) {
				r.URL = r.URL.JoinPath(fmt.Sprintf("%s:%s", model, "streamGenerateContent"))
				r.Header.Set("x-goog-api-key", "valid-api-token")
			},
			statusCode: http.StatusOK,
		},
		{
			name: "Auth/REST/WithQueryParamKey/OK",
			setupRequest: func(r *http.Request) {
				r.URL = r.URL.JoinPath(fmt.Sprintf("%s:%s", model, "generateContent"))

				q := r.URL.Query()
				q.Set("key", "valid-api-token")
				r.URL.RawQuery = q.Encode()
			},
			statusCode: http.StatusOK,
		},
		{
			name: "Auth/Strema/WithQueryParamKey/OK",
			setupRequest: func(r *http.Request) {
				r.URL = r.URL.JoinPath(fmt.Sprintf("%s:%s", model, "streamGenerateContent"))

				q := r.URL.Query()
				q.Set("key", "valid-api-token")
				r.URL.RawQuery = q.Encode()
			},
			statusCode: http.StatusOK,
		},
		{
			name: "Auth/Ambiguous/HeaderAndQueryKeyMatch/OK",
			setupRequest: func(r *http.Request) {
				r.URL = r.URL.JoinPath(fmt.Sprintf("%s:%s", model, "streamGenerateContent"))
				r.Header.Set("x-goog-api-key", "valid-api-token")

				q := r.URL.Query()
				q.Set("key", "valid-api-token")
				r.URL.RawQuery = q.Encode()
			},
			shouldFail: false,
			statusCode: http.StatusOK,
		},
		{
			name: "Auth/Ambiguous/HeaderAndQueryKeyMismatch/Unauthorized",
			setupRequest: func(r *http.Request) {
				r.URL = r.URL.JoinPath(fmt.Sprintf("%s:%s", model, "streamGenerateContent"))
				r.Header.Set("x-goog-api-key", "valid-api-token")

				q := r.URL.Query()
				q.Set("key", "somevalue")
				r.URL.RawQuery = q.Encode()
			},
			shouldFail: true,
			statusCode: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			tt.setupRequest(req)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer func() {
				if resp != nil && resp.Body != nil {
					if err := resp.Body.Close(); err != nil {
						t.Logf("error closing response body: %v", err)
					}
				}
			}()

			require.Equal(t, tt.statusCode, resp.StatusCode)
			if tt.shouldFail {
				return
			}

			respBody, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(respBody), "Hello! How can I help you today?")
		})
	}
}
