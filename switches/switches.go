// pckage switches is the place where all vendor-dependent logic lives. This should make organising and extending the code easier.
package switches

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	anthropicVendor "github.com/TykTechnologies/midsommar/v2/vendors/anthropic"
	googleaiVendor "github.com/TykTechnologies/midsommar/v2/vendors/googleai"
	hfVendor "github.com/TykTechnologies/midsommar/v2/vendors/huggingface"
	mockVendor "github.com/TykTechnologies/midsommar/v2/vendors/mock"
	ollamaVendor "github.com/TykTechnologies/midsommar/v2/vendors/ollama"
	openaiVendor "github.com/TykTechnologies/midsommar/v2/vendors/openai"
	vertexVendor "github.com/TykTechnologies/midsommar/v2/vendors/vertex"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/schema"
)

var AVAILABLE_LLM_DRIVERS = []models.Vendor{
	models.ANTHROPIC,
	models.OPENAI,
	models.OLLAMA,
	models.VERTEX,
	models.GOOGLEAI,
}

var AVAILABLE_EMBEDDERS = []models.Vendor{
	models.OPENAI,
	models.OLLAMA,
	models.VERTEX,
	models.GOOGLEAI,
}

type newVendorFunc func() models.LLMVendorProvider

var VendorMap = map[models.Vendor]newVendorFunc{
	models.OPENAI:      openaiVendor.New,
	models.ANTHROPIC:   anthropicVendor.New,
	models.OLLAMA:      ollamaVendor.New,
	models.VERTEX:      vertexVendor.New,
	models.GOOGLEAI:    googleaiVendor.New,
	models.HUGGINGFACE: hfVendor.New,
	models.MOCK_VENDOR: mockVendor.New,
}

// Handles token count finding for analytics, different vendors have different response types and we nee dto handle all of them
func GetTokenCounts(choice *llms.ContentChoice, vendor models.Vendor) (int, int, int) {
	v, ok := VendorMap[vendor]
	if !ok {
		slog.Warn("vendor not in supported vendor map")
		return 0, 0, 0
	}

	return v().GetTokenCounts(choice)
}

func FetchDriver(LLMConfig *models.LLM, settings *models.LLMSettings, mem schema.Memory, streamingFunc func(ctx context.Context, chunk []byte) error) (llms.Model, error) {
	v, ok := VendorMap[LLMConfig.Vendor]
	if !ok {
		return nil, fmt.Errorf("unsupported vendor")
	}

	return v().GetDriver(LLMConfig, settings, mem, streamingFunc)
}

func GetEmbedder(d *models.Datasource) (*embeddings.EmbedderImpl, error) {
	v, ok := VendorMap[d.EmbedVendor]
	if !ok {
		return nil, fmt.Errorf("unsupported vendor")
	}

	vn := v()
	if !vn.ProvidesEmbedder() {
		return nil, fmt.Errorf("vendor does not provide an embedder")
	}

	return vn.GetEmbedder(d)
}

func AnalyzeResponse(llm *models.LLM, app *models.App, statusCode int, body []byte, r *http.Request) (*models.LLM, *models.App, models.ITokenResponse, error) {
	v, ok := VendorMap[llm.Vendor]
	if !ok {
		return nil, nil, nil, fmt.Errorf("unsupported vendor")
	}

	return v().AnalyzeResponse(llm, app, statusCode, body, r)
}

func AnalyzeStreamingResponse(llm *models.LLM, app *models.App, statusCode int, resps []byte, r *http.Request, chunks [][]byte) (*models.LLM, *models.App, models.ITokenResponse, error) {
	v, ok := VendorMap[llm.Vendor]
	if !ok {
		return nil, nil, nil, fmt.Errorf("unsupported vendor")
	}

	return v().AnalyzeStreamingResponse(llm, app, statusCode, resps, r, chunks)
}

func SetVendorAuthHeader(r *http.Request, llm *models.LLM) error {
	v, ok := VendorMap[llm.Vendor]
	if !ok {
		return fmt.Errorf("unsupported vendor")
	}

	return v().ProxySetAuthHeader(r, llm)
}

// DetectStreamingIntent inspects the request to determine if it's a streaming request
// based on vendor-specific patterns (body fields, URL parameters, path patterns, etc.)
func DetectStreamingIntent(vendor models.Vendor, r *http.Request) (bool, error) {
	switch vendor {
	case models.GOOGLEAI, models.VERTEX:
		// Google AI/Vertex uses URL path pattern for streaming detection
		// Path contains ":streamGenerateContent" for streaming requests
		if containsCaseInsensitive(r.URL.Path, ":streamgeneratecontent") {
			return true, nil
		}
		// Also check for alt=sse query parameter (alternative streaming indicator)
		if r.URL.Query().Get("alt") == "sse" {
			return true, nil
		}
		return false, nil

	case models.OPENAI, models.OLLAMA:
		// OpenAI and Ollama use the "stream" field in request body
		return detectStreamFromBody(r, func(data []byte) (bool, error) {
			var req struct {
				Stream bool `json:"stream"`
			}
			if err := unmarshalJSON(data, &req); err != nil {
				return false, err
			}
			return req.Stream, nil
		})

	case models.ANTHROPIC:
		// Anthropic uses the "stream" field in request body
		return detectStreamFromBody(r, func(data []byte) (bool, error) {
			var req struct {
				Stream bool `json:"stream"`
			}
			if err := unmarshalJSON(data, &req); err != nil {
				return false, err
			}
			return req.Stream, nil
		})

	case models.HUGGINGFACE:
		// HuggingFace uses the "stream" field in request body
		return detectStreamFromBody(r, func(data []byte) (bool, error) {
			var req struct {
				Stream bool `json:"stream"`
			}
			if err := unmarshalJSON(data, &req); err != nil {
				return false, err
			}
			return req.Stream, nil
		})

	case models.MOCK_VENDOR:
		// Mock vendor can check body for stream field as a default behavior
		return detectStreamFromBody(r, func(data []byte) (bool, error) {
			var req struct {
				Stream bool `json:"stream"`
			}
			if err := unmarshalJSON(data, &req); err != nil {
				// For mock vendor, if we can't parse, default to false
				return false, nil
			}
			return req.Stream, nil
		})

	default:
		return false, fmt.Errorf("unsupported vendor for streaming detection: %s", vendor)
	}
}

// Helper functions for stream detection

// detectStreamFromBody reads the request body, applies the detection function, and restores the body
func detectStreamFromBody(r *http.Request, detector func([]byte) (bool, error)) (bool, error) {
	// Read the body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read request body: %w", err)
	}

	// Restore the body for downstream handlers
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))

	// Apply the detector function
	return detector(body)
}

// unmarshalJSON is a helper to unmarshal JSON with better error handling
func unmarshalJSON(data []byte, v any) error {
	if len(data) == 0 {
		return fmt.Errorf("empty request body")
	}

	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	return nil
}

// containsCaseInsensitive checks if a string contains a substring (case-insensitive)
func containsCaseInsensitive(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
