package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/TykTechnologies/midsommar/v2/analytics"
	bedrockVendor "github.com/TykTechnologies/midsommar/v2/vendors/bedrock"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/rs/zerolog/log"
)

// handleBedrockStreamingProxy handles streaming requests for Bedrock on the /llm/stream/ path.
// Instead of raw HTTP forwarding (which can't handle Bedrock's binary event stream protocol),
// this uses the AWS SDK's ConverseStream API to decode events and re-serialize them as SSE.
//
// Analytics data (token usage) is extracted on-the-fly from the metadata event rather than
// buffering the entire response in memory.
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

	// Stream events to client, extracting analytics on-the-fly.
	// We do NOT buffer the full response — token usage comes from the metadata event.
	hasResponseFilters := p.hasResponseFilters(llm)
	// accumulatedText is the running text buffer for response filters. It is passed as
	// currentBuffer so filter scripts can do cross-chunk pattern matching. We use a plain
	// string instead of strings.Builder to avoid the Builder.String() copy on each iteration.
	var accumulatedText string
	chunkIndex := 0
	isErr := false
	var inputTokens, outputTokens int32

	for event := range stream.Events() {
		if err := stream.Err(); err != nil {
			log.Error().Err(err).Msg("Bedrock stream error")
			isErr = true
			break
		}

		// Extract token usage from metadata event (no buffering needed)
		if metadata, ok := event.(*types.ConverseStreamOutputMemberMetadata); ok {
			if metadata.Value.Usage != nil {
				inputTokens = aws.ToInt32(metadata.Value.Usage.InputTokens)
				outputTokens = aws.ToInt32(metadata.Value.Usage.OutputTokens)
			}
		}

		eventJSON, err := marshalStreamEvent(event)
		if err != nil {
			log.Error().Err(err).Msg("Failed to marshal Bedrock stream event")
			continue
		}

		chunk := fmt.Appendf(nil, "data: %s\n\n", eventJSON)

		// Execute response filters per-chunk if configured
		if hasResponseFilters {
			if text := extractTextFromStreamEvent(event); text != "" {
				accumulatedText += text
			}

			blocked, blockMsg, filterErr := ExecuteResponseFilters(
				llm, p.gatewayService, chunk, http.StatusOK,
				true, true, chunkIndex, accumulatedText, r,
			)
			if filterErr != nil {
				log.Error().Err(filterErr).Int("chunk", chunkIndex).Msg("Response filter error on Bedrock stream chunk")
			} else if blocked {
				log.Warn().Int("chunk", chunkIndex).Str("reason", blockMsg).Msg("Bedrock stream blocked by filter")
				errorChunk := fmt.Appendf(nil, `{"error":"Response blocked by filter: %s"}`, blockMsg)
				w.Write(errorChunk)
				flusher.Flush()
				isErr = true
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
	w.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()

	// Record analytics using token counts captured on-the-fly from the metadata event
	if !isErr {
		go recordBedrockStreamProxyAnalytics(p, llm, app, modelID, int(inputTokens), int(outputTokens), reqBody, startTime)
	}
}

// recordBedrockStreamProxyAnalytics records analytics for the /llm/stream/ Bedrock path
// using token counts extracted on-the-fly from the stream metadata event.
func recordBedrockStreamProxyAnalytics(p *Proxy, llm *models.LLM, app *models.App, modelID string, promptTokens int, responseTokens int, reqBody []byte, timestamp time.Time) {
	if promptTokens == 0 && responseTokens == 0 {
		return
	}

	// Use actual model ID for correct billing
	price, err := p.gatewayService.GetModelPriceByModelNameAndVendor(modelID, string(llm.Vendor))
	if err != nil {
		log.Debug().Str("model", modelID).Str("vendor", string(llm.Vendor)).Msg("No pricing found for Bedrock model")
		price = &models.ModelPrice{}
	}

	cost := ((price.CPT * float64(responseTokens)) + (price.CPIT * float64(promptTokens))) * 10000

	record := &models.LLMChatRecord{
		LLMID:           llm.ID,
		Name:            modelID,
		Vendor:          string(llm.Vendor),
		PromptTokens:    promptTokens,
		ResponseTokens:  responseTokens,
		TotalTokens:     promptTokens + responseTokens,
		Cost:            cost,
		Currency:        price.Currency,
		Choices:         1,
		TimeStamp:       timestamp,
		AppID:           app.ID,
		UserID:          app.UserID,
		InteractionType: models.ProxyInteraction,
	}
	analytics.RecordChatRecord(record)
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
	switch v := event.(type) {
	case *types.ConverseStreamOutputMemberMessageStart:
		return json.Marshal(map[string]any{
			"type":         "messageStart",
			"messageStart": map[string]any{"role": string(v.Value.Role)},
		})
	case *types.ConverseStreamOutputMemberContentBlockStart:
		return json.Marshal(map[string]any{
			"type":              "contentBlockStart",
			"contentBlockIndex": v.Value.ContentBlockIndex,
		})
	case *types.ConverseStreamOutputMemberContentBlockDelta:
		delta := map[string]any{}
		if textDelta, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberText); ok {
			delta["text"] = textDelta.Value
		}
		return json.Marshal(map[string]any{
			"type":              "contentBlockDelta",
			"contentBlockIndex": v.Value.ContentBlockIndex,
			"delta":             delta,
		})
	case *types.ConverseStreamOutputMemberContentBlockStop:
		return json.Marshal(map[string]any{
			"type":              "contentBlockStop",
			"contentBlockIndex": v.Value.ContentBlockIndex,
		})
	case *types.ConverseStreamOutputMemberMessageStop:
		return json.Marshal(map[string]any{
			"type":       "messageStop",
			"stopReason": string(v.Value.StopReason),
		})
	case *types.ConverseStreamOutputMemberMetadata:
		result := map[string]any{
			"type": "metadata",
		}
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
		return json.Marshal(result)
	default:
		return json.Marshal(map[string]any{"type": "unknown"})
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
