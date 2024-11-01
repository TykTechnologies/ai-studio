package hfVendor

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/responses"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/huggingface"
	"github.com/tmc/langchaingo/schema"
)

type HuggingFace struct{}

func New() models.LLMVendorProvider {
	return &HuggingFace{}
}

func (v *HuggingFace) GetTokenCounts(choice *llms.ContentChoice) (int, int, int) {
	return 0, 0, 0
}

func (v *HuggingFace) GetDriver(LLMConfig *models.LLM, settings *models.LLMSettings, mem schema.Memory, streamingFunc func(ctx context.Context, chunk []byte) error) (llms.Model, error) {
	llm, err := setupHuggingFaceDriver(LLMConfig, settings)
	if err != nil {
		return nil, err
	}

	return llm, nil
}

func (v *HuggingFace) GetEmbedder(d *models.Datasource) (*embeddings.EmbedderImpl, error) {
	var llm embeddings.EmbedderClient
	var err error

	opts := []huggingface.Option{}
	if d.EmbedAPIKey != "" {
		opts = append(opts, huggingface.WithToken(d.EmbedAPIKey))
	}

	llm, err = NewHFWrapper(d.EmbedModel, opts...)

	e, err := embeddings.NewEmbedder(llm)
	if err != nil {
		return nil, err
	}

	return e, nil
}

func (v *HuggingFace) AnalyzeResponse(llm *models.LLM, app *models.App, statusCode int, body []byte, r *http.Request) (*models.LLM, *models.App, models.ITokenResponse, error) {
	mName := extractHuggingfaceModelID(r.URL.Path)
	out, err := helpers.CopyRequestBody(r)
	if err != nil {
		return llm, app, nil, err
	}

	outTokens := helpers.EstimateTokenCount(string(out))
	respTokens := helpers.EstimateTokenCount(string(body))

	return llm, app, &responses.DummyResponse{
		Usage: struct {
			PromptTokens   int `json:"prompt_tokens"`
			ResponseTokens int `json:"response_tokens"`
		}{
			PromptTokens:   outTokens,
			ResponseTokens: respTokens,
		},
		Model: mName}, nil
}

func (v *HuggingFace) AnalyzeStreamingResponse(llm *models.LLM, app *models.App, statusCode int, resps []byte, r *http.Request, chunks [][]byte) (*models.LLM, *models.App, models.ITokenResponse, error) {
	return llm, app, &responses.DummyResponse{
		Model: "huggingFace"}, nil
}

func (v *HuggingFace) ProxySetAuthHeader(r *http.Request, llm *models.LLM) error {
	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", llm.APIKey))
	return nil
}

func (v *HuggingFace) ProxyScreenRequest(llm *models.LLM, r *http.Request, isStreamingChannel bool) error {
	if isStreamingChannel {
		return fmt.Errorf("streaming is not supported for huggingface")
	}

	return nil
}

func (v *HuggingFace) ProvidesEmbedder() bool {
	return true
}

func setupHuggingFaceDriver(connDef *models.LLM, llmSettings *models.LLMSettings) (llms.Model, error) {
	var opts = make([]huggingface.Option, 0)

	if connDef.APIKey != "" {
		opts = append(opts, huggingface.WithToken(connDef.APIKey))
	}

	if llmSettings != nil {
		opts = append(opts, huggingface.WithModel(llmSettings.ModelName))
	}

	llm, err := huggingface.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create huggingface driver: %v", err)
	}

	return llm, nil
}

// The signature for the huggingface embedclient is wrong so it dos not implement
// the EmbedClient interface, this fixes that
func NewHFWrapper(modelName string, opts ...huggingface.Option) (*HFWrapper, error) {
	x, err := huggingface.New(opts...)
	if err != nil {
		return nil, err
	}

	return &HFWrapper{hdLLM: x}, nil
}

type HFWrapper struct {
	hdLLM     *huggingface.LLM
	ModelName string
}

func (o *HFWrapper) CreateEmbedding(ctx context.Context, inputTexts []string) ([][]float32, error) {
	return o.hdLLM.CreateEmbedding(ctx, inputTexts, o.ModelName, "embedding")
}

func extractHuggingfaceModelID(url string) string {
	patterns := []string{
		`/pipeline/feature-extraction/([^/]+)`,
		`/models/([^/]+)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(url)
		if len(matches) > 1 {
			return matches[1]
		}
	}

	return "huggingface-unspecified"
}
