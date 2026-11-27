package vertexVendor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/responses"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/embeddings"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms/googleai"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms/googleai/vertex"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/schema"
	googleaiVendor "github.com/TykTechnologies/midsommar/v2/vendors/googleai"
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

func (v *Vertex) GetEmbedder(spec *models.EmbedderSpec) (*embeddings.EmbedderImpl, error) {
	var llm embeddings.EmbedderClient
	var err error

	llm, err = setupVertexEmbedClient(spec)

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

// AnalyzeStreamingResponse reads a streamGenerateContent response. Vertex
// streams the Gemini wire format (alt=sse or a JSON array), so the Google AI
// parser records the tokens and the model, which it takes from the response
// or else from the .../publishers/google/models/{model} path.
func (v *Vertex) AnalyzeStreamingResponse(llm *models.LLM, app *models.App, statusCode int, resps []byte, r *http.Request, chunks [][]byte) (*models.LLM, *models.App, models.ITokenResponse, error) {
	return googleaiVendor.New().AnalyzeStreamingResponse(llm, app, statusCode, resps, r, chunks)
}

// ProxySetAuthHeader sets a Google OAuth2 access token from the LLM's
// credential chain (see credentials.go). On error the header is left unset
// and the caller must not send the request.
func (v *Vertex) ProxySetAuthHeader(r *http.Request, llm *models.LLM) error {
	token, err := AccessToken(llm)
	if err != nil {
		return err
	}
	r.Header.Set("Authorization", "Bearer "+token)
	return nil
}

// ProxyScreenRequest checks the request suits the route: Vertex's REST API is
// JSON over HTTPS, and a :streamGenerateContent path is a stream, so it
// belongs on /llm/stream and every other path on /llm/rest, as for Google AI.
func (v *Vertex) ProxyScreenRequest(llm *models.LLM, r *http.Request, isStreamingChannel bool) error {
	isStream := strings.Contains(strings.ToLower(r.URL.Path), ":streamgeneratecontent")
	if isStreamingChannel && !isStream {
		return fmt.Errorf("streaming is required for this endpoint")
	}
	if !isStreamingChannel && isStream {
		return fmt.Errorf("streaming is not allowed for this endpoint")
	}
	return nil
}

func (v *Vertex) ProvidesEmbedder() bool {
	return true
}

func (v *Vertex) ProvidesChat() bool {
	return true
}

func (v *Vertex) DecisionEndpoint() string {
	return ""
}

func setupVertexDriver(connDef *models.LLM, llmSettings *models.LLMSettings) (llms.Model, error) {
	// format for project and location is split with a colon
	split := strings.Split(connDef.APIEndpoint, ":")
	if len(split) != 2 {
		return nil, fmt.Errorf("invalid API endpoint format (must be project:location)")
	}

	project := split[0]
	location := split[1]

	// The returned client is long-lived and its context is retained by the
	// underlying Google credential/token source for refreshes, so it must not
	// be bound to a short timeout.
	ctx := context.Background()

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

func setupVertexEmbedClient(spec *models.EmbedderSpec) (embeddings.EmbedderClient, error) {
	// The endpoint names the project and location, split with a colon.
	split := strings.Split(spec.Endpoint, ":")
	if len(split) != 2 {
		return nil, fmt.Errorf("Connection string endpoint format (must be project:location)")
	}

	project := split[0]
	location := split[1]
	// The returned client is long-lived and its context is retained by the
	// underlying Google credential/token source for refreshes, so it must not
	// be bound to a short timeout.
	ctx := context.Background()

	llm, err := vertex.New(
		ctx,
		googleai.WithCloudProject(project),
		googleai.WithCloudLocation(location),
		googleai.WithAPIKey(spec.APIKey),
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
