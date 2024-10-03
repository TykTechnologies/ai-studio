// pckage switches is the place where all vendor-dependent logic lives. This should make organising and extending the code easier.
package switches

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

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
