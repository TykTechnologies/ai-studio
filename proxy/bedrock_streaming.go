package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	bedrockVendor "github.com/TykTechnologies/midsommar/v2/vendors/bedrock"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/rs/zerolog/log"
)

// handleBedrockStreamingProxy handles streaming requests for Bedrock on the /llm/stream/ path.
// Instead of raw HTTP forwarding (which can't handle Bedrock's binary event stream protocol),
// this uses the AWS SDK's ConverseStream API to decode events and re-serialize them as SSE.
func (p *Proxy) handleBedrockStreamingProxy(w http.ResponseWriter, r *http.Request, llm *models.LLM, app *models.App, reqBody []byte) {
	startTime := time.Now()

	// Parse the Converse API request from the body
	var converseReq bedrockConverseStreamRequest
	if err := json.Unmarshal(reqBody, &converseReq); err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid Bedrock Converse request body", err, false)
		return
	}

	// Determine model ID
	modelID := converseReq.ModelId
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

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		respondWithError(w, http.StatusInternalServerError, "streaming not supported", nil, false)
		return
	}
	flusher.Flush()

	// Iterate over decoded stream events
	var fullResponse bytes.Buffer
	var responses [][]byte
	var textBuffer strings.Builder
	hasResponseFilters := p.hasResponseFilters(llm)
	chunkIndex := 0
	isErr := false

	for event := range stream.Events() {
		if err := stream.Err(); err != nil {
			log.Error().Err(err).Msg("Bedrock stream error")
			isErr = true
			break
		}

		eventJSON, err := marshalStreamEvent(event)
		if err != nil {
			log.Error().Err(err).Msg("Failed to marshal Bedrock stream event")
			continue
		}

		chunk := []byte(fmt.Sprintf("data: %s\n\n", eventJSON))
		responses = append(responses, chunk)
		fullResponse.Write(chunk)

		// Extract text for response filters
		if hasResponseFilters {
			if text := extractTextFromStreamEvent(event); text != "" {
				textBuffer.WriteString(text)
			}

			blocked, blockMsg, filterErr := ExecuteResponseFilters(
				llm, p.gatewayService, chunk, http.StatusOK,
				true, true, chunkIndex, textBuffer.String(), r,
			)
			if filterErr != nil {
				log.Error().Err(filterErr).Int("chunk", chunkIndex).Msg("Response filter error on Bedrock stream chunk")
			} else if blocked {
				log.Warn().Int("chunk", chunkIndex).Str("reason", blockMsg).Msg("Bedrock stream blocked by filter")
				errorChunk := []byte(fmt.Sprintf(`{"error":"Response blocked by filter: %s"}`, blockMsg))
				w.Write(errorChunk)
				flusher.Flush()
				isErr = true
				go p.analyzeStreamingResponse(llm, app, r, http.StatusBadRequest, fullResponse.Bytes(), reqBody, responses, startTime, "")
				return
			}
		}

		// Write to client
		if _, werr := w.Write(chunk); werr != nil {
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

	// Send [DONE] marker
	doneChunk := []byte("data: [DONE]\n\n")
	w.Write(doneChunk)
	fullResponse.Write(doneChunk)
	flusher.Flush()

	if !isErr {
		go p.analyzeStreamingResponse(llm, app, r, http.StatusOK, fullResponse.Bytes(), reqBody, responses, startTime, "")

		if p.responseHookManager != nil && p.hasResponseHooks() {
			go p.executeOnStreamComplete(r, nil, llm, app, fullResponse.Bytes(), reqBody, chunkIndex)
		}
	}
}

// --- Helper types and functions ---

// bedrockConverseStreamRequest is a JSON-parseable subset of the Converse API request
// that users send to the /llm/stream/ endpoint.
type bedrockConverseStreamRequest struct {
	ModelId         string                   `json:"modelId,omitempty"`
	Messages        []bedrockMessage         `json:"messages"`
	System          []bedrockSystemBlock     `json:"system,omitempty"`
	InferenceConfig *bedrockInferenceConfig  `json:"inferenceConfig,omitempty"`
	ToolConfig      *bedrockToolConfig       `json:"toolConfig,omitempty"`
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

// marshalStreamEvent converts a ConverseStream event to JSON for SSE output.
func marshalStreamEvent(event types.ConverseStreamOutput) ([]byte, error) {
	// Create a typed wrapper for JSON serialization
	switch v := event.(type) {
	case *types.ConverseStreamOutputMemberMessageStart:
		return json.Marshal(map[string]interface{}{
			"type":         "messageStart",
			"messageStart": map[string]interface{}{"role": string(v.Value.Role)},
		})
	case *types.ConverseStreamOutputMemberContentBlockStart:
		return json.Marshal(map[string]interface{}{
			"type":              "contentBlockStart",
			"contentBlockIndex": v.Value.ContentBlockIndex,
		})
	case *types.ConverseStreamOutputMemberContentBlockDelta:
		delta := map[string]interface{}{}
		if textDelta, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberText); ok {
			delta["text"] = textDelta.Value
		}
		return json.Marshal(map[string]interface{}{
			"type":              "contentBlockDelta",
			"contentBlockIndex": v.Value.ContentBlockIndex,
			"delta":             delta,
		})
	case *types.ConverseStreamOutputMemberContentBlockStop:
		return json.Marshal(map[string]interface{}{
			"type":              "contentBlockStop",
			"contentBlockIndex": v.Value.ContentBlockIndex,
		})
	case *types.ConverseStreamOutputMemberMessageStop:
		return json.Marshal(map[string]interface{}{
			"type":       "messageStop",
			"stopReason": string(v.Value.StopReason),
		})
	case *types.ConverseStreamOutputMemberMetadata:
		result := map[string]interface{}{
			"type": "metadata",
		}
		if v.Value.Usage != nil {
			result["usage"] = map[string]interface{}{
				"inputTokens":  aws.ToInt32(v.Value.Usage.InputTokens),
				"outputTokens": aws.ToInt32(v.Value.Usage.OutputTokens),
				"totalTokens":  aws.ToInt32(v.Value.Usage.TotalTokens),
			}
		}
		if v.Value.Metrics != nil {
			result["metrics"] = map[string]interface{}{
				"latencyMs": aws.ToInt64(v.Value.Metrics.LatencyMs),
			}
		}
		return json.Marshal(result)
	default:
		return json.Marshal(map[string]interface{}{"type": "unknown"})
	}
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

