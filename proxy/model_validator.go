package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/gorilla/mux"
)

type ModelValidator struct {
	allowedModels []string
}

type ValidationError struct {
	message string
}

func (e *ValidationError) Error() string {
	return e.message
}

type BadRequestError struct {
	message string
}

func (e *BadRequestError) Error() string {
	return e.message
}

func NewModelValidator(allowedModels []string) *ModelValidator {
	return &ModelValidator{
		allowedModels: allowedModels,
	}
}

func (mv *ModelValidator) IsModelAllowed(modelName string) bool {
	if len(mv.allowedModels) == 0 {
		return true // If no models specified, allow all
	}

	for _, pattern := range mv.allowedModels {
		matched, err := regexp.MatchString(pattern, modelName)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func (mv *ModelValidator) ValidateRequest(body []byte) error {
	// Try to extract model from different request formats
	var genericReq map[string]interface{}
	if err := json.Unmarshal(body, &genericReq); err != nil {
		return &BadRequestError{"invalid JSON body"}
	}

	modelInterface, ok := genericReq["model"]
	if !ok {
		return &BadRequestError{"model field is required"}
	}

	model, ok := modelInterface.(string)
	if !ok {
		return &BadRequestError{"model must be a string"}
	}

	if !mv.IsModelAllowed(model) {
		return &ValidationError{fmt.Sprintf("model '%s' is not allowed", model)}
	}

	return nil
}

func (p *Proxy) modelValidationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		llmSlug := vars["llmSlug"]

		p.mu.RLock()
		llm, ok := p.llms[llmSlug]
		p.mu.RUnlock()

		if !ok {
			slog.Error("LLM not found in middleware", "slug", llmSlug, "available_llms", p.llms)
			respondWithError(w, http.StatusNotFound, "LLM not found", nil)
			return
		}

		// Create validator
		validator := NewModelValidator(llm.AllowedModels)

		// Read and validate body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "Failed to read request body", err)
			return
		}
		r.Body.Close()
		r.Body = io.NopCloser(strings.NewReader(string(body)))

		if err := validator.ValidateRequest(body); err != nil {
			switch e := err.(type) {
			case *ValidationError:
				respondWithError(w, http.StatusForbidden, fmt.Sprintf("Model validation failed: %s", e.Error()), nil)
			case *BadRequestError:
				respondWithError(w, http.StatusBadRequest, fmt.Sprintf("Bad request: %s", e.Error()), nil)
			default:
				respondWithError(w, http.StatusInternalServerError, "Internal server error", err)
			}
			return
		}

		next.ServeHTTP(w, r)
	})
}
