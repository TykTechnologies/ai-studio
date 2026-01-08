package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelValidationMiddleware(t *testing.T) {
	t.Run("stores model from request body in context", func(t *testing.T) {
		// Create a test proxy with a mock LLM
		p := &Proxy{
			llms: map[string]*models.LLM{
				"test-llm": {
					Name:   "Test LLM",
					Vendor: models.ANTHROPIC,
				},
			},
		}

		// Register the Anthropic model extractor
		p.modelValidator = NewModelValidator(nil)
		p.modelValidator.RegisterExtractor(string(models.ANTHROPIC), AnthropicModelExtractor)

		// Create a test handler that checks the context
		var capturedModel string
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if model := r.Context().Value("model_name"); model != nil {
				capturedModel = model.(string)
			}
			w.WriteHeader(http.StatusOK)
		})

		// Create the middleware handler
		handler := p.modelValidationMiddleware(nextHandler)

		// Create a test request with a model in the body
		reqBody := map[string]interface{}{
			"model": "claude-3-opus-20240229",
			"messages": []map[string]string{
				{"role": "user", "content": "Hello"},
			},
		}
		reqBodyBytes, _ := json.Marshal(reqBody)

		req := httptest.NewRequest("POST", "/llm/rest/test-llm/v1/messages", strings.NewReader(string(reqBodyBytes)))
		req = mux.SetURLVars(req, map[string]string{"llmSlug": "test-llm"})

		// Execute the request
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		// Verify the model was stored in context
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "claude-3-opus-20240229", capturedModel)
	})
}

func TestIsModelAllowed(t *testing.T) {
	tests := []struct {
		name           string
		allowedModels  []string
		modelName      string
		expectedResult bool
	}{
		{
			name:           "empty allowed list permits all models",
			allowedModels:  []string{},
			modelName:      "claude-opus-4-5-20251101",
			expectedResult: true,
		},
		{
			name:           "nil allowed list permits all models",
			allowedModels:  nil,
			modelName:      "claude-opus-4-5-20251101",
			expectedResult: true,
		},
		{
			name:           "exact match pattern allows model",
			allowedModels:  []string{"claude-opus-4-5-20251101"},
			modelName:      "claude-opus-4-5-20251101",
			expectedResult: true,
		},
		{
			name:           "sonnet pattern should NOT match opus model",
			allowedModels:  []string{"sonnet.*", "haiku.*"},
			modelName:      "claude-opus-4-5-20251101",
			expectedResult: false,
		},
		{
			name:           "sonnet pattern SHOULD match sonnet model",
			allowedModels:  []string{"sonnet.*", "haiku.*"},
			modelName:      "claude-3-5-sonnet-20241022",
			expectedResult: true,
		},
		{
			name:           "haiku pattern SHOULD match haiku model",
			allowedModels:  []string{"sonnet.*", "haiku.*"},
			modelName:      "claude-3-haiku-20240307",
			expectedResult: true,
		},
		{
			name:           "partial regex match - sonnet.* matches substring",
			allowedModels:  []string{"sonnet.*"},
			modelName:      "claude-3-5-sonnet-20241022",
			expectedResult: true,
		},
		{
			name:           "anchored pattern - ^claude-sonnet should not match mid-string",
			allowedModels:  []string{"^claude-sonnet.*"},
			modelName:      "claude-3-5-sonnet-20241022",
			expectedResult: false,
		},
		{
			name:           "anchored pattern - ^claude-3-5-sonnet should match",
			allowedModels:  []string{"^claude-3-5-sonnet.*"},
			modelName:      "claude-3-5-sonnet-20241022",
			expectedResult: true,
		},
		{
			name:           "wildcard for all claude models",
			allowedModels:  []string{"claude-.*"},
			modelName:      "claude-opus-4-5-20251101",
			expectedResult: true,
		},
		{
			name:           "multiple patterns - first matches",
			allowedModels:  []string{"opus.*", "sonnet.*"},
			modelName:      "claude-opus-4-5-20251101",
			expectedResult: true,
		},
		{
			name:           "gpt model should not match claude patterns",
			allowedModels:  []string{"claude-.*"},
			modelName:      "gpt-4-turbo",
			expectedResult: false,
		},
		{
			name:           "full model name patterns",
			allowedModels:  []string{"^claude-3-5-sonnet-.*$", "^claude-3-haiku-.*$"},
			modelName:      "claude-3-5-sonnet-20241022",
			expectedResult: true,
		},
		{
			name:           "full model name patterns reject opus",
			allowedModels:  []string{"^claude-3-5-sonnet-.*$", "^claude-3-haiku-.*$"},
			modelName:      "claude-opus-4-5-20251101",
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewModelValidator(tt.allowedModels)
			result := validator.IsModelAllowed(tt.modelName)
			require.Equal(t, tt.expectedResult, result,
				"IsModelAllowed(%q) with patterns %v = %v, want %v",
				tt.modelName, tt.allowedModels, result, tt.expectedResult)
		})
	}
}

func TestModelValidationMiddleware_AllowedModels(t *testing.T) {
	t.Run("rejects model not in allowed list", func(t *testing.T) {
		// Create a test proxy with an LLM that only allows sonnet and haiku
		p := &Proxy{
			llms: map[string]*models.LLM{
				"test-llm": {
					Name:          "Test LLM",
					Vendor:        models.ANTHROPIC,
					AllowedModels: []string{"sonnet.*", "haiku.*"},
				},
			},
		}

		// Register the Anthropic model extractor
		p.modelValidator = NewModelValidator(nil)
		p.modelValidator.RegisterExtractor(string(models.ANTHROPIC), AnthropicModelExtractor)

		// Create a test handler that should NOT be called
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("next handler should not be called when model is rejected")
			w.WriteHeader(http.StatusOK)
		})

		// Create the middleware handler
		handler := p.modelValidationMiddleware(nextHandler)

		// Create a test request with an opus model (should be rejected)
		reqBody := map[string]interface{}{
			"model": "claude-opus-4-5-20251101",
			"messages": []map[string]string{
				{"role": "user", "content": "Hello"},
			},
		}
		reqBodyBytes, _ := json.Marshal(reqBody)

		req := httptest.NewRequest("POST", "/llm/rest/test-llm/v1/messages", strings.NewReader(string(reqBodyBytes)))
		req = mux.SetURLVars(req, map[string]string{"llmSlug": "test-llm"})

		// Execute the request
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		// Verify the request was rejected with 403 Forbidden
		require.Equal(t, http.StatusForbidden, rr.Code, "opus model should be rejected")
		assert.Contains(t, rr.Body.String(), "not allowed")
	})

	t.Run("allows model in allowed list", func(t *testing.T) {
		// Create a test proxy with an LLM that only allows sonnet and haiku
		p := &Proxy{
			llms: map[string]*models.LLM{
				"test-llm": {
					Name:          "Test LLM",
					Vendor:        models.ANTHROPIC,
					AllowedModels: []string{"sonnet.*", "haiku.*"},
				},
			},
		}

		// Register the Anthropic model extractor
		p.modelValidator = NewModelValidator(nil)
		p.modelValidator.RegisterExtractor(string(models.ANTHROPIC), AnthropicModelExtractor)

		// Create a test handler
		var capturedModel string
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if model := r.Context().Value("model_name"); model != nil {
				capturedModel = model.(string)
			}
			w.WriteHeader(http.StatusOK)
		})

		// Create the middleware handler
		handler := p.modelValidationMiddleware(nextHandler)

		// Create a test request with a sonnet model (should be allowed)
		reqBody := map[string]interface{}{
			"model": "claude-3-5-sonnet-20241022",
			"messages": []map[string]string{
				{"role": "user", "content": "Hello"},
			},
		}
		reqBodyBytes, _ := json.Marshal(reqBody)

		req := httptest.NewRequest("POST", "/llm/rest/test-llm/v1/messages", strings.NewReader(string(reqBodyBytes)))
		req = mux.SetURLVars(req, map[string]string{"llmSlug": "test-llm"})

		// Execute the request
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		// Verify the request was allowed
		require.Equal(t, http.StatusOK, rr.Code, "sonnet model should be allowed")
		assert.Equal(t, "claude-3-5-sonnet-20241022", capturedModel)
	})
}
