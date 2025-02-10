package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/gorilla/mux"
)

type ModelNameExtractor func(r *http.Request, body []byte) (string, error)

type ModelValidator struct {
	allowedModels []string
	extractors    map[string]ModelNameExtractor
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
		extractors:    make(map[string]ModelNameExtractor),
	}
}

func (mv *ModelValidator) RegisterExtractor(vendor string, extractor ModelNameExtractor) {
	mv.extractors[strings.ToLower(vendor)] = extractor
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

		// fmt.Printf("Request URL: %s\n", r.URL.Path)
		// fmt.Printf("All vars: %+v\n", vars)
		// fmt.Printf("llmSlug: %s\n", llmSlug)

		llm, ok := p.GetLLM(llmSlug)
		if !ok {
			fmt.Println(r.URL.String())
			fmt.Println(vars)
			errMsg := fmt.Sprintf("[modelValidator] LLM '%s' not found", llmSlug)
			respondWithError(w, http.StatusNotFound, errMsg, nil)
			return
		}

		// Create validator with LLM-specific allowed models
		validator := NewModelValidator(llm.AllowedModels)
		// Copy extractors from proxy's modelValidator
		validator.extractors = p.modelValidator.extractors

		body, err := io.ReadAll(r.Body)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "Failed to read request body", err)
			return
		}
		r.Body.Close()
		r.Body = io.NopCloser(strings.NewReader(string(body)))

		extractor, ok := validator.extractors[strings.ToLower(string(llm.Vendor))]
		if !ok {
			respondWithError(w, http.StatusBadRequest, "no model extractor for this vendor", nil)
			return
		}

		model, err := extractor(r, body)
		if err != nil {
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

		if !validator.IsModelAllowed(model) {
			respondWithError(w, http.StatusForbidden, fmt.Sprintf("model '%s' is not allowed", model), nil)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func OpenAIModelExtractor(r *http.Request, body []byte) (string, error) {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", &BadRequestError{"invalid JSON body"}
	}

	modelInterface, ok := req["model"]
	if !ok {
		return "", &BadRequestError{"model field is required"}
	}

	model, ok := modelInterface.(string)
	if !ok {
		return "", &BadRequestError{"model must be a string"}
	}

	return model, nil
}

func AzureModelExtractor(r *http.Request, body []byte) (string, error) {
	// Extract from URL path
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 2 {
		return "", &BadRequestError{"invalid URL path"}
	}
	return parts[len(parts)-2], nil
}

func AnthropicModelExtractor(r *http.Request, body []byte) (string, error) {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", &BadRequestError{"invalid JSON body"}
	}

	modelInterface, ok := req["model"]
	if !ok {
		return "", &BadRequestError{"model field is required"}
	}

	model, ok := modelInterface.(string)
	if !ok {
		return "", &BadRequestError{"model must be a string"}
	}

	return model, nil
}

func GoogleAIModelExtractor(r *http.Request, body []byte) (string, error) {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", &BadRequestError{"invalid JSON body"}
	}

	// Google AI can have model in different places depending on the API version
	// First try the standard location
	if modelInterface, ok := req["model"]; ok {
		if model, ok := modelInterface.(string); ok {
			return model, nil
		}
	}

	// Try the configuration block used in some APIs
	if config, ok := req["configuration"].(map[string]interface{}); ok {
		if modelInterface, ok := config["model"]; ok {
			if model, ok := modelInterface.(string); ok {
				return model, nil
			}
		}
	}

	return "", &BadRequestError{"model field not found in expected locations"}
}

func VertexModelExtractor(r *http.Request, body []byte) (string, error) {
	// Vertex AI typically includes model in the URL path
	// Format: .../projects/{project}/locations/{location}/publishers/google/models/{model}
	parts := strings.Split(r.URL.Path, "/")
	for i, part := range parts {
		if part == "models" && i+1 < len(parts) {
			return parts[i+1], nil
		}
	}

	// Fallback to body check
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", &BadRequestError{"invalid JSON body"}
	}

	if modelInterface, ok := req["model"]; ok {
		if model, ok := modelInterface.(string); ok {
			return model, nil
		}
	}

	return "", &BadRequestError{"model not found in URL path or request body"}
}

func HuggingFaceModelExtractor(r *http.Request, body []byte) (string, error) {
	// First check URL path
	parts := strings.Split(r.URL.Path, "/")
	for i, part := range parts {
		if part == "models" && i+1 < len(parts) {
			return parts[i+1], nil
		}
	}

	// Check request body
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", &BadRequestError{"invalid JSON body"}
	}

	// Try "model" field
	if modelInterface, ok := req["model"]; ok {
		if model, ok := modelInterface.(string); ok {
			return model, nil
		}
	}

	// Try "model_id" field
	if modelInterface, ok := req["model_id"]; ok {
		if model, ok := modelInterface.(string); ok {
			return model, nil
		}
	}

	return "", &BadRequestError{"model not found in URL path or request body"}
}
