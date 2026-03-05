package vertexVendor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/responses"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/llms/googleai/vertex"
	"github.com/tmc/langchaingo/schema"
)

type Vertex struct{}

func New() models.LLMVendorProvider {
	return &Vertex{}
}

func (v *Vertex) GetTokenCounts(choice *llms.ContentChoice) (int, int, int) {
	promptTokens := helpers.KeyValueInt32OrZero(choice.GenerationInfo, "input_tokens")
	responseTokens := helpers.KeyValueInt32OrZero(choice.GenerationInfo, "output_tokens")
	totalTokens := promptTokens + responseTokens

	return totalTokens, promptTokens, responseTokens
}

func (v *Vertex) GetDriver(LLMConfig *models.LLM, settings *models.LLMSettings, mem schema.Memory, streamingFunc func(ctx context.Context, chunk []byte) error) (llms.Model, error) {
	llm, err := setupVertexDriver(LLMConfig, settings)
	if err != nil {
		return nil, err
	}

	return llm, nil
}

func (v *Vertex) GetEmbedder(d *models.Datasource) (*embeddings.EmbedderImpl, error) {
	var llm embeddings.EmbedderClient
	var err error

	llm, err = setupVertexEmbedClient(d)

	if err != nil {
		return nil, err
	}

	e, err := embeddings.NewEmbedder(llm)
	if err != nil {
		return nil, err
	}

	return e, nil
}

func (v *Vertex) AnalyzeResponse(llm *models.LLM, app *models.App, statusCode int, body []byte, r *http.Request) (*models.LLM, *models.App, models.ITokenResponse, error) {
	response := &responses.GoogleAIChatResponse{}
	if err := json.Unmarshal(body, response); err != nil {
		return nil, nil, nil, err
	}

	// Prefer modelVersion from the response body (most accurate)
	modelName := response.ModelVersion
	if modelName == "" {
		modelName, _ = extractModelIDFromVertexURL(r.URL.Path)
	}
	if modelName == "" {
		modelName = "googleai-gemini-no-id"
	}

	response.SetModel(modelName)
	return llm, app, response, nil
}

func (v *Vertex) AnalyzeStreamingResponse(llm *models.LLM, app *models.App, statusCode int, resps []byte, r *http.Request, chunks [][]byte) (*models.LLM, *models.App, models.ITokenResponse, error) {
	return llm, app, &responses.DummyResponse{
		Model: "vertex"}, nil
}

func (v *Vertex) ProxySetAuthHeader(r *http.Request, llm *models.LLM) error {
	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", llm.APIKey))
	return nil
}

func (v *Vertex) ProxyScreenRequest(llm *models.LLM, r *http.Request, isStreamingChannel bool) error {
	return fmt.Errorf("vertex APIs are strongly tied to the gcloud client libraries and use gRPC, so they are not available in the proxy")
}

func (v *Vertex) ProvidesEmbedder() bool {
	return true
}

func setupVertexDriver(connDef *models.LLM, llmSettings *models.LLMSettings) (llms.Model, error) {
	// format for project and location is split with a colon
	split := strings.Split(connDef.APIEndpoint, ":")
	if len(split) != 2 {
		return nil, fmt.Errorf("invalid API endpoint format (must be project:location)")
	}

	project := split[0]
	location := split[1]

	ctx, _ := context.WithTimeout(context.Background(), 240*time.Second)

	llm, err := vertex.New(
		ctx,
		googleai.WithCloudProject(project),
		googleai.WithCloudLocation(location),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create vertex driver: %v", err)
	}

	return llm, nil
}

func setupVertexEmbedClient(d *models.Datasource) (embeddings.EmbedderClient, error) {
	// format for project and location is split with a colon
	split := strings.Split(d.DBConnString, ":")
	if len(split) != 2 {
		return nil, fmt.Errorf("Connection string endpoint format (must be project:location)")
	}

	project := split[0]
	location := split[1]
	ctx, _ := context.WithTimeout(context.Background(), 240*time.Second)

	llm, err := vertex.New(
		ctx,
		googleai.WithCloudProject(project),
		googleai.WithCloudLocation(location),
		googleai.WithAPIKey(d.DBConnAPIKey),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create vertex driver: %v", err)
	}

	return llm, nil
}

func extractModelIDFromVertexURL(url string) (string, error) {
	// Regular expression pattern to match the MODEL_ID at the end of the URL
	pattern := `/models/([^/]+)$`

	// Compile the regular expression
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("failed to compile regex: %v", err)
	}

	// Find the first match in the URL
	match := re.FindStringSubmatch(url)

	if len(match) > 1 {
		// If a match is found, return the captured group (MODEL_ID)
		return match[1], nil
	}

	// If no match is found, return an error
	return "", fmt.Errorf("model ID not found in URL")
}
