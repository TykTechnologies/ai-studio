package mockVendor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/responses"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/schema"
)

type MockResponse struct {
	Model string `json:"model"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (o *MockResponse) GetPromptTokens() int {
	return o.Usage.PromptTokens
}

func (o *MockResponse) GetResponseTokens() int {
	return o.Usage.CompletionTokens
}

func (o *MockResponse) GetChoiceCount() int {
	return 1
}

func (o *MockResponse) GetToolCount() int {
	return 0
}

func (o *MockResponse) GetModel() string {
	return o.Model
}

type Mock struct{}

func New() models.LLMVendorProvider {
	return &Mock{}
}

func (v *Mock) GetTokenCounts(choice *llms.ContentChoice) (int, int, int) {
	return 0, 0, 0
}

func (v *Mock) GetDriver(LLMConfig *models.LLM, settings *models.LLMSettings, mem schema.Memory, streamingFunc func(ctx context.Context, chunk []byte) error) (llms.Model, error) {
	llm := &DummyDriver{
		StreamingFunc: streamingFunc,
		Memory:        mem,
	}

	return llm, nil
}

func (v *Mock) GetEmbedder(d *models.Datasource) (*embeddings.EmbedderImpl, error) {
	return nil, nil
}

func (v *Mock) AnalyzeResponse(llm *models.LLM, app *models.App, statusCode int, body []byte, r *http.Request) (*models.LLM, *models.App, models.ITokenResponse, error) {
	response := &MockResponse{Model: "test-vendor"}
	err := json.Unmarshal(body, response)
	if err != nil {
		return nil, nil, nil, err
	}
	return llm, app, response, nil
}

func (v *Mock) AnalyzeStreamingResponse(llm *models.LLM, app *models.App, statusCode int, resps []byte, r *http.Request, chunks [][]byte) (*models.LLM, *models.App, models.ITokenResponse, error) {
	response := &responses.DummyResponse{Model: "test-vendor"}
	return llm, app, response, nil
}

func (v *Mock) ProxySetAuthHeader(r *http.Request, llm *models.LLM) error {
	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", llm.APIKey))
	return nil
}

func (v *Mock) ProxyScreenRequest(llm *models.LLM, r *http.Request, isStreamingChannel bool) error {
	return nil
}

func (v *Mock) ProvidesEmbedder() bool {
	return false
}
