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

	// Force reload
	err = proxy.loadResources()
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
