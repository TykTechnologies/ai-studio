package openaiVendor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/responses"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/schema"
)

const (
	OpenAICompletionsEndpoint = "/v1/chat/completions"
	OpenAIEmbeddingsEndpoint  = "/v1/embeddings"
)

type OpenAI struct{}

func New() models.LLMVendorProvider {
	return &OpenAI{}
}

func (v *OpenAI) GetTokenCounts(choice *llms.ContentChoice) (int, int, int) {
	promptTokens := 0
	responseTokens := 0
	totalTokens := 0

	usage := choice.GenerationInfo
	promptTokens = helpers.KeyValueOrZero(usage, "PromptTokens")
	responseTokens = helpers.KeyValueOrZero(usage, "ResponseTokens")
	cacheWriteTokens := helpers.KeyValueOrZero(usage, "CacheCreationInputTokens")
	cacheReadTokens := helpers.KeyValueOrZero(usage, "CacheReadInputTokens")
	totalTokens = promptTokens + responseTokens + cacheWriteTokens + cacheReadTokens

	return totalTokens, promptTokens, responseTokens
}

func (v *OpenAI) GetDriver(
	LLMConfig *models.LLM,
	settings *models.LLMSettings,
	mem schema.Memory,
	streamingFunc func(ctx context.Context, chunk []byte) error) (llms.Model, error) {

	var llm llms.Model
	var err error

	llm, err = setupOpenAIDriver(LLMConfig, settings)
	return llm, err
}

func (v *OpenAI) GetEmbedder(d *models.Datasource) (*embeddings.EmbedderImpl, error) {
	var llm embeddings.EmbedderClient
	var err error

	opts := []openai.Option{}
	if d.EmbedAPIKey != "" {
		opts = append(opts, openai.WithToken(d.EmbedAPIKey))
	}
	if d.EmbedUrl != "" {
		opts = append(opts, openai.WithBaseURL(d.EmbedUrl))
	}
	if d.EmbedModel == "" {
		return nil, fmt.Errorf("missing embed model")
	}

	opts = append(opts, openai.WithEmbeddingModel(d.EmbedModel))
	llm, err = openai.New(opts...)

	if err != nil {
		return nil, err
	}

	e, err := embeddings.NewEmbedder(llm)
	if err != nil {
		return nil, err
	}

	return e, nil
}

func (v *OpenAI) AnalyzeResponse(llm *models.LLM, app *models.App, statusCode int, body []byte, r *http.Request) (*models.LLM, *models.App, models.ITokenResponse, error) {
	var response models.ITokenResponse
	// embedding replies have the same usage section
	if strings.Contains(strings.ToLower(r.URL.Path), OpenAICompletionsEndpoint) ||
		strings.Contains(strings.ToLower(r.URL.Path), OpenAIEmbeddingsEndpoint) {

		response = &responses.OpenAIResponse{}
		err := json.Unmarshal(body, response)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to unmarshal llm rest response: %w", err)
		}

		return llm, app, response, nil
	}

	return nil, nil, nil, fmt.Errorf("[analyse response] unknown completions endpoint: %s", llm.Vendor)
}

func (v *OpenAI) AnalyzeStreamingResponse(llm *models.LLM, app *models.App, statusCode int, resps []byte, r *http.Request, chunks [][]byte) (*models.LLM, *models.App, models.ITokenResponse, error) {
	aggregate := &responses.GenericResponse{}
	strBody := string(resps)
	parts := strings.Split(strBody, "\n")

	for _, p := range parts {
		pBody := strings.TrimPrefix(p, "data:")
		if pBody != "" && strings.Trim(pBody, " ") != "[DONE]" {
			tempResp := &responses.OpenAIStreamingResponse{}
			err := json.Unmarshal([]byte(pBody), tempResp)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("failed to unmarshal streaming chunk %v (%s)", err, pBody)
			}

			aggregate.Choices += tempResp.GetChoiceCount()
			aggregate.ToolCalls += tempResp.GetToolCount()

			if tempResp.Usage != nil {
				aggregate.PromptTokens = tempResp.Usage.PromptTokens
				aggregate.CompletionTokens = tempResp.Usage.CompletionTokens
			}
		}
	}

	return llm, app, aggregate, nil
}

func (v *OpenAI) ProxySetAuthHeader(r *http.Request, llm *models.LLM) error {
	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", llm.APIKey))
	return nil
}

func (v *OpenAI) ProxyScreenRequest(llm *models.LLM, r *http.Request, isStreamingChannel bool) error {
	b, err := helpers.CopyRequestBody(r)
	if err != nil {
		return err
	}

	var req responses.OpenAIRequest
	if err := json.Unmarshal(b, &req); err != nil {
		return err
	}

	if isStreamingChannel {
		if !req.Stream {
			return fmt.Errorf("streaming is required for this endpoint")
		}

		if !req.StreamOptions.IncludeUsage {
			return fmt.Errorf("streaming without usage is not allowed")
		}

		return nil
	}

	// not a streaming endpoint, but they are streaming
	if req.Stream {
		return fmt.Errorf("streaming is not allowed for this endpoint")
	}

	return nil
}

func (v *OpenAI) ProvidesEmbedder() bool {
	return true
}

func setupOpenAIDriver(connDef *models.LLM, llmSettings *models.LLMSettings) (llms.Model, error) {
	var opts = make([]openai.Option, 0)
	if connDef.APIEndpoint != "" {
		opts = append(opts, openai.WithBaseURL(connDef.APIEndpoint))
	}

	if connDef.APIKey != "" {
		opts = append(opts, openai.WithToken(connDef.APIKey))
	}

	if llmSettings != nil {
		opts = append(opts, openai.WithModel(llmSettings.ModelName))
	}

	llm, err := openai.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI driver: %v", err)
	}

	return llm, nil
}
