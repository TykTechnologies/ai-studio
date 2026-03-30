package bedrockVendor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/responses"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	v4signer "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/sirupsen/logrus"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/schema"
)

type Bedrock struct{}

func New() models.LLMVendorProvider {
	return &Bedrock{}
}

// --- Client Cache ---
// AWS SDK clients are safe for concurrent use and expensive to create (LoadDefaultConfig
// performs filesystem I/O). Cache them keyed by LLM ID + credential fingerprint.

type clientCacheEntry struct {
	client      *bedrockruntime.Client
	fingerprint string // hash of credentials + region to detect config changes
}

var (
	clientCache sync.Map // map[uint]*clientCacheEntry keyed by LLM ID
)

// credentialFingerprint produces a short hash of the credential fields to detect changes.
func credentialFingerprint(accessKeyID, secretAccessKey, sessionToken, region string) string {
	h := sha256.Sum256([]byte(accessKeyID + "|" + secretAccessKey + "|" + sessionToken + "|" + region))
	return hex.EncodeToString(h[:8])
}

// --- Exported Helpers (used by proxy handlers) ---

// NewBedrockClient returns a cached bedrockruntime.Client for the given LLM config,
// creating a new one only if the LLM ID is unseen or credentials have changed.
func NewBedrockClient(llm *models.LLM) (*bedrockruntime.Client, error) {
	region, err := ParseRegionFromEndpoint(llm.APIEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to parse region from endpoint: %w", err)
	}

	accessKeyID, secretAccessKey, sessionToken, err := extractAWSCredentials(llm)
	if err != nil {
		return nil, err
	}

	fp := credentialFingerprint(accessKeyID, secretAccessKey, sessionToken, region)

	// Check cache
	if cached, ok := clientCache.Load(llm.ID); ok {
		entry := cached.(*clientCacheEntry)
		if entry.fingerprint == fp {
			return entry.client, nil
		}
		// Credentials changed — fall through to create new client
	}

	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			accessKeyID, secretAccessKey, sessionToken,
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := bedrockruntime.NewFromConfig(cfg)
	clientCache.Store(llm.ID, &clientCacheEntry{client: client, fingerprint: fp})
	return client, nil
}

// InvalidateClientCache removes a cached client for the given LLM ID.
// Useful when credentials are updated.
func InvalidateClientCache(llmID uint) {
	clientCache.Delete(llmID)
}

// ParseRegionFromEndpoint extracts the AWS region from a Bedrock endpoint URL.
// Expected format: https://bedrock-runtime.{region}.amazonaws.com
// Also accepts a plain region string like "us-east-1".
func ParseRegionFromEndpoint(endpoint string) (string, error) {
	if endpoint == "" {
		return "", fmt.Errorf("bedrock endpoint is empty")
	}

	// Plain region string (no dots, no slashes)
	if !strings.Contains(endpoint, ".") && !strings.Contains(endpoint, "/") {
		return endpoint, nil
	}

	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("invalid bedrock endpoint URL: %s", endpoint)
	}

	// Parse region from hostname: bedrock-runtime.{region}.amazonaws.com
	parts := strings.Split(parsed.Hostname(), ".")
	if len(parts) >= 3 && parts[0] == "bedrock-runtime" {
		return parts[1], nil
	}

	return "", fmt.Errorf("cannot extract region from endpoint: %s (expected bedrock-runtime.{region}.amazonaws.com)", endpoint)
}

// GetModelID returns the model ID to use, preferring the override if non-empty.
func GetModelID(llm *models.LLM, override string) string {
	if override != "" {
		return override
	}
	return llm.DefaultModel
}

// --- Format Conversion Helpers ---

// ChatMessage is a simplified message struct for format conversion.
type ChatMessage struct {
	Role    string
	Content string
}

// ConvertChatMessagesToConverse converts ChatMessage slices to Converse types.
// System messages are extracted into a separate slice as required by the Converse API.
func ConvertChatMessagesToConverse(messages []ChatMessage) ([]types.Message, []types.SystemContentBlock) {
	var converseMsgs []types.Message
	var systemBlocks []types.SystemContentBlock

	for _, msg := range messages {
		if msg.Role == "system" {
			systemBlocks = append(systemBlocks, &types.SystemContentBlockMemberText{
				Value: msg.Content,
			})
			continue
		}

		role := convertToConverseRole(msg.Role)
		converseMsgs = append(converseMsgs, types.Message{
			Role: role,
			Content: []types.ContentBlock{
				&types.ContentBlockMemberText{Value: msg.Content},
			},
		})
	}

	return converseMsgs, systemBlocks
}

// BuildInferenceConfig builds a Converse InferenceConfiguration from OpenAI parameters.
func BuildInferenceConfig(maxTokens *int, temperature *float64, topP *float64, stop any) *types.InferenceConfiguration {
	config := &types.InferenceConfiguration{}
	hasValues := false

	if maxTokens != nil {
		v := int32(*maxTokens)
		config.MaxTokens = &v
		hasValues = true
	}

	if temperature != nil {
		v := float32(*temperature)
		config.Temperature = &v
		hasValues = true
	}

	if topP != nil {
		v := float32(*topP)
		config.TopP = &v
		hasValues = true
	}

	if stop != nil {
		switch v := stop.(type) {
		case string:
			config.StopSequences = []string{v}
			hasValues = true
		case []string:
			config.StopSequences = v
			hasValues = true
		case []interface{}:
			seqs := make([]string, 0, len(v))
			for _, s := range v {
				if str, ok := s.(string); ok {
					seqs = append(seqs, str)
				}
			}
			if len(seqs) > 0 {
				config.StopSequences = seqs
				hasValues = true
			}
		}
	}

	if !hasValues {
		return nil
	}
	return config
}

// BuildToolConfig converts tool definitions to Bedrock ToolConfiguration.
func BuildToolConfig(tools []ToolDef) *types.ToolConfiguration {
	if len(tools) == 0 {
		return nil
	}

	bedrockTools := make([]types.Tool, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != "function" {
			continue
		}

		bedrockTools = append(bedrockTools, &types.ToolMemberToolSpec{
			Value: types.ToolSpecification{
				Name:        aws.String(tool.Function.Name),
				Description: aws.String(tool.Function.Description),
				InputSchema: &types.ToolInputSchemaMemberJson{
					Value: document.NewLazyDocument(tool.Function.Parameters),
				},
			},
		})
	}

	if len(bedrockTools) == 0 {
		return nil
	}

	return &types.ToolConfiguration{
		Tools: bedrockTools,
	}
}

// ToolDef is a simplified tool definition for format conversion.
type ToolDef struct {
	Type     string
	Function ToolFuncDef
}

// ToolFuncDef is a simplified function definition.
type ToolFuncDef struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// ConvertConverseStopReason maps Bedrock stop reasons to OpenAI finish_reason values.
func ConvertConverseStopReason(reason types.StopReason) string {
	switch reason {
	case types.StopReasonEndTurn:
		return "stop"
	case types.StopReasonToolUse:
		return "tool_calls"
	case types.StopReasonMaxTokens:
		return "length"
	case types.StopReasonStopSequence:
		return "stop"
	case types.StopReasonContentFiltered:
		return "content_filter"
	case types.StopReasonGuardrailIntervened:
		return "content_filter"
	default:
		return "stop"
	}
}

// ExtractTextFromContentBlocks extracts concatenated text from Converse ContentBlocks.
func ExtractTextFromContentBlocks(blocks []types.ContentBlock) string {
	var sb strings.Builder
	for _, block := range blocks {
		if textBlock, ok := block.(*types.ContentBlockMemberText); ok {
			sb.WriteString(textBlock.Value)
		}
	}
	return sb.String()
}

// ExtractToolCallsFromContentBlocks extracts tool calls from Converse ContentBlocks as OpenAI format.
func ExtractToolCallsFromContentBlocks(blocks []types.ContentBlock) []map[string]interface{} {
	var toolCalls []map[string]interface{}
	for _, block := range blocks {
		if toolBlock, ok := block.(*types.ContentBlockMemberToolUse); ok {
			argsJSON, _ := json.Marshal(toolBlock.Value.Input)
			toolCalls = append(toolCalls, map[string]interface{}{
				"id":   aws.ToString(toolBlock.Value.ToolUseId),
				"type": "function",
				"function": map[string]interface{}{
					"name":      aws.ToString(toolBlock.Value.Name),
					"arguments": string(argsJSON),
				},
			})
		}
	}
	return toolCalls
}

// --- LLMVendorProvider Interface Implementation ---

func (v *Bedrock) GetTokenCounts(choice *llms.ContentChoice) (int, int, int) {
	// Token counts for Bedrock come from ConverseOutput.Usage directly,
	// not from langchaingo GenerationInfo.
	return 0, 0, 0
}

func (v *Bedrock) GetDriver(_ *models.LLM, _ *models.LLMSettings, _ schema.Memory, _ func(ctx context.Context, chunk []byte) error) (llms.Model, error) {
	return nil, fmt.Errorf("bedrock uses direct AWS SDK calls via Converse API; use the Bedrock-specific proxy handlers instead")
}

func (v *Bedrock) GetEmbedder(_ *models.Datasource) (*embeddings.EmbedderImpl, error) {
	return nil, nil
}

func (v *Bedrock) AnalyzeResponse(llm *models.LLM, app *models.App, _ int, body []byte, _ *http.Request) (*models.LLM, *models.App, models.ITokenResponse, error) {
	var response responses.BedrockConverseResponse
	if err := json.Unmarshal(body, &response); err != nil {
		logrus.WithError(err).Warn("Failed to parse Bedrock Converse response for analytics")
		return llm, app, &responses.GenericResponse{}, nil
	}

	return llm, app, &response, nil
}

func (v *Bedrock) AnalyzeStreamingResponse(llm *models.LLM, app *models.App, _ int, resps []byte, _ *http.Request, _ [][]byte) (*models.LLM, *models.App, models.ITokenResponse, error) {
	aggregate := &responses.GenericResponse{
		Choices: 1,
	}

	// Parse SSE-formatted events from the SDK-mediated streaming proxy
	lines := strings.Split(string(resps), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" || data == "" {
			continue
		}

		// Try to parse as metadata event (contains usage)
		var metadata responses.BedrockConverseStreamMetadataEvent
		if err := json.Unmarshal([]byte(data), &metadata); err == nil && metadata.Usage.TotalTokens > 0 {
			aggregate.PromptTokens = metadata.Usage.InputTokens
			aggregate.CompletionTokens = metadata.Usage.OutputTokens
		}
	}

	return llm, app, aggregate, nil
}

func (v *Bedrock) ProxySetAuthHeader(r *http.Request, llm *models.LLM) error {
	region, err := ParseRegionFromEndpoint(llm.APIEndpoint)
	if err != nil {
		return fmt.Errorf("failed to parse region: %w", err)
	}

	accessKeyID, secretAccessKey, sessionToken, err := extractAWSCredentials(llm)
	if err != nil {
		return err
	}

	// Read and hash the request body for SigV4 signing
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body for signing: %w", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	payloadHash := sha256Hex(body)

	creds := aws.Credentials{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		SessionToken:    sessionToken,
	}

	signer := v4signer.NewSigner()
	err = signer.SignHTTP(r.Context(), creds, r, payloadHash, "bedrock", region, time.Now())
	if err != nil {
		return fmt.Errorf("failed to sign request with SigV4: %w", err)
	}

	return nil
}

func (v *Bedrock) ProxyScreenRequest(_ *models.LLM, _ *http.Request, _ bool) error {
	return nil
}

func (v *Bedrock) ProvidesEmbedder() bool {
	return false
}

// --- Internal helpers ---

func extractAWSCredentials(llm *models.LLM) (accessKeyID, secretAccessKey, sessionToken string, err error) {
	if llm.Metadata == nil {
		return "", "", "", fmt.Errorf("bedrock: metadata is required with aws_access_key_id and aws_secret_access_key")
	}

	accessKeyID = metadataString(llm.Metadata, "aws_access_key_id")
	secretAccessKey = metadataString(llm.Metadata, "aws_secret_access_key")
	sessionToken = metadataString(llm.Metadata, "aws_session_token") // optional

	if accessKeyID == "" {
		return "", "", "", fmt.Errorf("bedrock: AWS Access Key ID is required (set metadata.aws_access_key_id)")
	}
	if secretAccessKey == "" {
		return "", "", "", fmt.Errorf("bedrock: AWS Secret Access Key is required (set metadata.aws_secret_access_key)")
	}

	return accessKeyID, secretAccessKey, sessionToken, nil
}

func metadataString(m models.JSONMap, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func convertToConverseRole(role string) types.ConversationRole {
	switch role {
	case "user", "human":
		return types.ConversationRoleUser
	case "assistant", "ai":
		return types.ConversationRoleAssistant
	default:
		return types.ConversationRoleUser
	}
}
