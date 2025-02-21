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
