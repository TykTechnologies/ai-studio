package ollamaVendor

import (
	"context"
	"fmt"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	openaiVendor "github.com/TykTechnologies/midsommar/v2/vendors/openai"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/schema"
)

type Ollama struct{}

const (
	OllamaChatCompletionsEndpoint     = "/api/chat"
	OllamaGenerateCompletionsEndpoint = "/api/generate"
)

func New() models.LLMVendorProvider {
	return &Ollama{}
}

func (v *Ollama) GetTokenCounts(choice *llms.ContentChoice) (int, int, int) {
	promptTokens := helpers.KeyValueOrZero(choice.GenerationInfo, "PromptTokens")
	responseTokens := helpers.KeyValueOrZero(choice.GenerationInfo, "CompletionTokens")
	totalTokens := promptTokens + responseTokens

	return totalTokens, promptTokens, responseTokens
}

func (v *Ollama) GetDriver(LLMConfig *models.LLM, settings *models.LLMSettings, mem schema.Memory, streamingFunc func(ctx context.Context, chunk []byte) error) (llms.Model, error) {
	llm, err := setupOllamaDriver(LLMConfig, settings)
	if err != nil {
		return nil, err
	}

	return llm, nil
}

func (v *Ollama) GetEmbedder(d *models.Datasource) (*embeddings.EmbedderImpl, error) {
	var llm embeddings.EmbedderClient
	var err error

	opts := []ollama.Option{}
	if d.EmbedUrl != "" {
		opts = append(opts, ollama.WithServerURL(d.EmbedUrl))
	}
	if d.EmbedModel == "" {
		return nil, fmt.Errorf("missing embed model")
	}

	opts = append(opts, ollama.WithModel(d.EmbedModel))
	llm, err = ollama.New(opts...)

	if err != nil {
		return nil, err
	}

	e, err := embeddings.NewEmbedder(llm)
	if err != nil {
		return nil, err
	}

	return e, nil
}

func (v *Ollama) AnalyzeResponse(llm *models.LLM, app *models.App, statusCode int, body []byte, r *http.Request) (*models.LLM, *models.App, models.ITokenResponse, error) {
	oai := openaiVendor.New()
	return oai.AnalyzeResponse(llm, app, statusCode, body, r)
}

func (v *Ollama) AnalyzeStreamingResponse(llm *models.LLM, app *models.App, statusCode int, resps []byte, r *http.Request, chunks [][]byte) (*models.LLM, *models.App, models.ITokenResponse, error) {
	oai := openaiVendor.New()
	return oai.AnalyzeStreamingResponse(llm, app, statusCode, resps, r, chunks)
}

func (v *Ollama) ProxySetAuthHeader(r *http.Request, llm *models.LLM) error {
	r.Header.Set("Authorization", llm.APIKey)
	return nil
}

func (v *Ollama) ProxyScreenRequest(llm *models.LLM, r *http.Request, isStreamingChannel bool) error {
	oai := openaiVendor.New()
	return oai.ProxyScreenRequest(llm, r, isStreamingChannel)
}

func (v *Ollama) ProvidesEmbedder() bool {
	return true
}

func setupOllamaDriver(connDef *models.LLM, llmSettings *models.LLMSettings) (llms.Model, error) {
	var opts = make([]ollama.Option, 0)

	if connDef.APIEndpoint != "" {
		opts = append(opts, ollama.WithServerURL(connDef.APIEndpoint))
	}

	if llmSettings != nil {
		opts = append(opts, ollama.WithModel(llmSettings.ModelName))
	}

	llm, err := ollama.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create ollama driver: %v", err)
	}

	return llm, nil
}
