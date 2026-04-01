package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream/eventstreamapi"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/TykTechnologies/midsommar/v2/models"
	bedrockVendor "github.com/TykTechnologies/midsommar/v2/vendors/bedrock"
	"github.com/rs/zerolog/log"
)

// handleBedrockStreamingProxy handles streaming requests for Bedrock on the /llm/stream/ path.
// It decodes the AWS binary event stream via the SDK, applies filters/middleware on the decoded
// events, then re-encodes them back to binary event stream format so native AWS SDK clients
// (boto3, etc.) work transparently.
func (p *Proxy) handleBedrockStreamingProxy(w http.ResponseWriter, r *http.Request, llm *models.LLM, app *models.App, reqBody []byte) {
	startTime := time.Now()

	// Parse the Converse API request from the body
	var converseReq bedrockConverseStreamRequest
	if err := json.Unmarshal(reqBody, &converseReq); err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid Bedrock Converse request body", err, false)
		return
	}

	// Determine model ID: check URL path first (native Bedrock sends /model/{modelId}/converse-stream),
	// then request body, then fall back to LLM default.
	modelID := extractModelIDFromPath(r.URL.Path)
	if modelID == "" {
		modelID = converseReq.ModelId
	}
	if modelID == "" {
		modelID = llm.DefaultModel
	}
	if modelID == "" {
		respondWithError(w, http.StatusBadRequest, "model ID is required (set modelId in request body or configure DefaultModel)", nil, false)
		return
	}

	// Create Bedrock client
	client, err := bedrockVendor.NewBedrockClient(llm)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to create Bedrock client", err, false)
		return
	}

	// Build ConverseStream input from the request
	input := buildConverseStreamInput(modelID, &converseReq)

	// Call ConverseStream
	output, err := client.ConverseStream(r.Context(), input)
	if err != nil {
		respondWithError(w, http.StatusBadGateway, fmt.Sprintf("Bedrock ConverseStream failed: %s", err.Error()), err, false)
		return
	}

	stream := output.GetStream()
	if stream == nil {
		respondWithError(w, http.StatusBadGateway, "Bedrock returned no stream", nil, false)
		return
	}
	defer stream.Close()

	// Set binary event stream headers (matching what AWS Bedrock returns)
	w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		respondWithError(w, http.StatusInternalServerError, "streaming not supported", nil, false)
		return
	}
	flusher.Flush()

	encoder := eventstream.NewEncoder()

	// Stream events to client, extracting analytics on-the-fly.
	hasResponseFilters := p.hasResponseFilters(llm)
	var textBuffer strings.Builder
	chunkIndex := 0
	isErr := false
	var inputTokens, outputTokens int32

	for event := range stream.Events() {
		if err := stream.Err(); err != nil {
			log.Error().Err(err).Msg("Bedrock stream error")
			isErr = true
			break
		}

		// Extract token usage from metadata event
		if metadata, ok := event.(*types.ConverseStreamOutputMemberMetadata); ok {
			if metadata.Value.Usage != nil {
				inputTokens = aws.ToInt32(metadata.Value.Usage.InputTokens)
				outputTokens = aws.ToInt32(metadata.Value.Usage.OutputTokens)
			}
		}

		// Execute response filters per-chunk if configured
		if hasResponseFilters {
			if text := extractTextFromStreamEvent(event); text != "" {
				textBuffer.WriteString(text)
			}

			// For filter evaluation, serialize to JSON
			filterJSON, _ := marshalEventPayload(event)
			blocked, blockMsg, filterErr := ExecuteResponseFilters(
				llm, p.gatewayService, filterJSON, http.StatusOK,
				true, true, chunkIndex, textBuffer.String(), r,
			)
			if filterErr != nil {
				log.Error().Err(filterErr).Int("chunk", chunkIndex).Msg("Response filter error on Bedrock stream chunk")
			} else if blocked {
				log.Warn().Int("chunk", chunkIndex).Str("reason", blockMsg).Msg("Bedrock stream blocked by filter")
				// Send error as a binary event stream exception
				writeEventStreamError(encoder, w, fmt.Sprintf("Response blocked by filter: %s", blockMsg))
				flusher.Flush()
				isErr = true
				return
			}
		}

		// Re-encode event as binary event stream message
		if err := encodeBedrockEventStreamMessage(encoder, w, event); err != nil {
			log.Error().Err(err).Msg("Failed to encode Bedrock event stream message")
			isErr = true
			break
		}
		flusher.Flush()
		chunkIndex++
	}

	if err := stream.Err(); err != nil {
		log.Error().Err(err).Msg("Bedrock stream ended with error")
		isErr = true
	}

	// Record analytics using token counts captured on-the-fly from the metadata event
	if !isErr {
		go recordBedrockChatRecord(p, llm, app, modelID, int(inputTokens), int(outputTokens), r, startTime)
	}
}

// encodeBedrockEventStreamMessage encodes a ConverseStream SDK event back to
// the AWS binary event stream format that native clients (boto3, etc.) expect.
func encodeBedrockEventStreamMessage(encoder *eventstream.Encoder, w io.Writer, event types.ConverseStreamOutput) error {
	eventType, payload, err := marshalEventForBinaryStream(event)
	if err != nil {
		return fmt.Errorf("marshal event %T: %w", event, err)
	}

	msg := eventstream.Message{
		Headers: eventstream.Headers{
			{Name: eventstreamapi.MessageTypeHeader, Value: eventstream.StringValue(eventstreamapi.EventMessageType)},
			{Name: eventstreamapi.EventTypeHeader, Value: eventstream.StringValue(eventType)},
			{Name: eventstreamapi.ContentTypeHeader, Value: eventstream.StringValue("application/json")},
		},
		Payload: payload,
	}

	return encoder.Encode(w, msg)
}

// marshalEventForBinaryStream returns the event type string and JSON payload
// matching the exact wire format that AWS Bedrock sends in the binary event stream.
// We cannot use json.Marshal on SDK structs directly because they don't have json tags
// matching the wire format — the SDK uses custom serializers internally.
func marshalEventForBinaryStream(event types.ConverseStreamOutput) (string, []byte, error) {
	switch v := event.(type) {
	case *types.ConverseStreamOutputMemberMessageStart:
		payload, err := json.Marshal(map[string]any{
			"role": string(v.Value.Role),
		})
		return "messageStart", payload, err
	case *types.ConverseStreamOutputMemberContentBlockStart:
		startMap := map[string]any{
			"contentBlockIndex": aws.ToInt32(v.Value.ContentBlockIndex),
			"start":             map[string]any{},
		}
		if v.Value.Start != nil {
			if toolUse, ok := v.Value.Start.(*types.ContentBlockStartMemberToolUse); ok {
				startMap["start"] = map[string]any{
					"toolUse": map[string]any{
						"toolUseId": toolUse.Value.ToolUseId,
						"name":      toolUse.Value.Name,
					},
				}
			}
		}
		payload, err := json.Marshal(startMap)
		return "contentBlockStart", payload, err
	case *types.ConverseStreamOutputMemberContentBlockDelta:
		delta := map[string]any{}
		if v.Value.Delta != nil {
			if textDelta, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberText); ok {
				delta["text"] = textDelta.Value
			} else if toolDelta, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberToolUse); ok {
				delta["toolUse"] = map[string]any{"input": toolDelta.Value.Input}
			}
		}
		payload, err := json.Marshal(map[string]any{
			"contentBlockIndex": aws.ToInt32(v.Value.ContentBlockIndex),
			"delta":             delta,
		})
		return "contentBlockDelta", payload, err
	case *types.ConverseStreamOutputMemberContentBlockStop:
		payload, err := json.Marshal(map[string]any{
			"contentBlockIndex": aws.ToInt32(v.Value.ContentBlockIndex),
		})
		return "contentBlockStop", payload, err
	case *types.ConverseStreamOutputMemberMessageStop:
		payload, err := json.Marshal(map[string]any{
			"stopReason": string(v.Value.StopReason),
		})
		return "messageStop", payload, err
	case *types.ConverseStreamOutputMemberMetadata:
		result := map[string]any{}
		if v.Value.Usage != nil {
			result["usage"] = map[string]any{
				"inputTokens":  aws.ToInt32(v.Value.Usage.InputTokens),
				"outputTokens": aws.ToInt32(v.Value.Usage.OutputTokens),
				"totalTokens":  aws.ToInt32(v.Value.Usage.TotalTokens),
			}
		}
		if v.Value.Metrics != nil {
			result["metrics"] = map[string]any{
				"latencyMs": aws.ToInt64(v.Value.Metrics.LatencyMs),
			}
		}
		payload, err := json.Marshal(result)
		return "metadata", payload, err
	default:
		return "unknown", []byte("{}"), nil
	}
}

// marshalEventPayload serializes an event to JSON for filter evaluation.
func marshalEventPayload(event types.ConverseStreamOutput) ([]byte, error) {
	_, payload, err := marshalEventForBinaryStream(event)
	return payload, err
}

// writeEventStreamError writes an error message as a binary event stream exception.
func writeEventStreamError(encoder *eventstream.Encoder, w io.Writer, errMsg string) {
	payload, _ := json.Marshal(map[string]string{"message": errMsg})
	msg := eventstream.Message{
		Headers: eventstream.Headers{
			{Name: eventstreamapi.MessageTypeHeader, Value: eventstream.StringValue(eventstreamapi.ExceptionMessageType)},
			{Name: eventstreamapi.ExceptionTypeHeader, Value: eventstream.StringValue("validationException")},
			{Name: eventstreamapi.ContentTypeHeader, Value: eventstream.StringValue("application/json")},
		},
		Payload: payload,
	}
	encoder.Encode(w, msg)
}

// --- Helper types and functions ---

// bedrockConverseStreamRequest is a JSON-parseable subset of the Converse API request
// that users send to the /llm/stream/ endpoint.
type bedrockConverseStreamRequest struct {
	ModelId         string                  `json:"modelId,omitempty"`
	Messages        []bedrockMessage        `json:"messages"`
	System          []bedrockSystemBlock    `json:"system,omitempty"`
	InferenceConfig *bedrockInferenceConfig `json:"inferenceConfig,omitempty"`
	ToolConfig      *bedrockToolConfig      `json:"toolConfig,omitempty"`
}

type bedrockMessage struct {
	Role    string                `json:"role"`
	Content []bedrockContentBlock `json:"content"`
}

type bedrockContentBlock struct {
	Text string `json:"text,omitempty"`
}

type bedrockSystemBlock struct {
	Text string `json:"text,omitempty"`
}

type bedrockInferenceConfig struct {
	MaxTokens     *int32   `json:"maxTokens,omitempty"`
	Temperature   *float32 `json:"temperature,omitempty"`
	TopP          *float32 `json:"topP,omitempty"`
	StopSequences []string `json:"stopSequences,omitempty"`
}

type bedrockToolConfig struct {
	Tools []json.RawMessage `json:"tools,omitempty"`
}

// extractModelIDFromPath extracts the model ID from a Bedrock URL path.
// Native Bedrock API paths contain /model/{modelId}/converse or /model/{modelId}/converse-stream.
func extractModelIDFromPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "model" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func buildConverseStreamInput(modelID string, req *bedrockConverseStreamRequest) *bedrockruntime.ConverseStreamInput {
	input := &bedrockruntime.ConverseStreamInput{
		ModelId: aws.String(modelID),
	}

	// Convert messages
	for _, msg := range req.Messages {
		var contentBlocks []types.ContentBlock
		for _, c := range msg.Content {
			if c.Text != "" {
				contentBlocks = append(contentBlocks, &types.ContentBlockMemberText{Value: c.Text})
			}
		}
		input.Messages = append(input.Messages, types.Message{
			Role:    types.ConversationRole(msg.Role),
			Content: contentBlocks,
		})
	}

	// Convert system blocks
	for _, sys := range req.System {
		if sys.Text != "" {
			input.System = append(input.System, &types.SystemContentBlockMemberText{Value: sys.Text})
		}
	}

	// Convert inference config
	if req.InferenceConfig != nil {
		input.InferenceConfig = &types.InferenceConfiguration{
			MaxTokens:     req.InferenceConfig.MaxTokens,
			Temperature:   req.InferenceConfig.Temperature,
			TopP:          req.InferenceConfig.TopP,
			StopSequences: req.InferenceConfig.StopSequences,
		}
	}

	return input
}

// extractTextFromStreamEvent extracts text content from a ConverseStream event for response filtering.
func extractTextFromStreamEvent(event types.ConverseStreamOutput) string {
	if delta, ok := event.(*types.ConverseStreamOutputMemberContentBlockDelta); ok {
		if textDelta, ok := delta.Value.Delta.(*types.ContentBlockDeltaMemberText); ok {
			return textDelta.Value
		}
	}
	return ""
}
