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
	"github.com/TykTechnologies/midsommar/v2/models"
	bedrockVendor "github.com/TykTechnologies/midsommar/v2/vendors/bedrock"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// handleBedrockChatCompletion handles non-streaming chat completion requests for Bedrock
// via the /ai/v1/chat/completions endpoint. Translates OpenAI format ↔ Converse API format.
func (p *Proxy) handleBedrockChatCompletion(w http.ResponseWriter, r *http.Request, conf *models.LLM, req *ChatCompletionRequest, reqBody []byte) {
	timestamp := time.Now()

	// Get app from context (auth already ran via middleware)
	app, err := p.getAppFromContext(r, conf)
	if err != nil {
		respondWithOAIError(w, http.StatusUnauthorized, "authentication required", err, false)
		return
	}

	// Check budget
	if _, _, err := p.budgetService.CheckBudget(app, conf); err != nil {
		respondWithOAIError(w, http.StatusForbidden, fmt.Sprintf("budget exceeded: %s", err.Error()), err, false)
		return
	}

	// Create Bedrock client
	client, err := bedrockVendor.NewBedrockClient(conf)
	if err != nil {
		respondWithOAIError(w, http.StatusInternalServerError, "failed to create Bedrock client", err, false)
		return
	}

	// Determine model ID
	modelID := bedrockVendor.GetModelID(conf, req.Model)
	if modelID == "" {
		respondWithOAIError(w, http.StatusBadRequest, "model ID is required", nil, false)
		return
	}

	// Convert OpenAI messages to Converse format
	chatMessages := openAIMessagesToChatMessages(req.Messages)
	converseMsgs, systemBlocks := bedrockVendor.ConvertChatMessagesToConverse(chatMessages)

	// Build inference config
	inferenceConfig := bedrockVendor.BuildInferenceConfig(req.MaxCompletionTokens, req.Temperature, req.TopP, req.Stop)

	// Build tool config
	var toolConfig *types.ToolConfiguration
	if len(req.Tools) > 0 {
		toolDefs := openAIToolsToToolDefs(req.Tools)
		toolConfig = bedrockVendor.BuildToolConfig(toolDefs)
	}

	// Call Converse API
	input := &bedrockruntime.ConverseInput{
		ModelId:         aws.String(modelID),
		Messages:        converseMsgs,
		System:          systemBlocks,
		InferenceConfig: inferenceConfig,
		ToolConfig:      toolConfig,
	}

	output, err := client.Converse(r.Context(), input)
	if err != nil {
		respondWithOAIError(w, http.StatusBadGateway, fmt.Sprintf("Bedrock Converse failed: %s", err.Error()), err, false)
		return
	}

	// Convert Converse response to OpenAI format
	response := converseOutputToOpenAI(output, req.Model)

	// Marshal response
	respBody, err := json.Marshal(response)
	if err != nil {
		respondWithOAIError(w, http.StatusInternalServerError, "failed to marshal response", err, false)
		return
	}

	// Record analytics
	go recordBedrockAnalytics(p, conf, app, output, reqBody, respBody, r, timestamp)

	// Send response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respBody)
}

// handleBedrockChatCompletionStream handles streaming chat completion requests for Bedrock
// via the /ai/v1/chat/completions endpoint with stream=true.
func (p *Proxy) handleBedrockChatCompletionStream(
	w http.ResponseWriter,
	r *http.Request,
	conf *models.LLM,
	req *ChatCompletionRequest,
	reqBody []byte,
) {
	timestamp := time.Now()

	// Get app from context
	app, err := p.getAppFromContext(r, conf)
	if err != nil {
		respondWithOAIError(w, http.StatusUnauthorized, "authentication required", err, false)
		return
	}

	// Check budget
	if _, _, err := p.budgetService.CheckBudget(app, conf); err != nil {
		respondWithOAIError(w, http.StatusForbidden, fmt.Sprintf("budget exceeded: %s", err.Error()), err, false)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		respondWithOAIError(w, http.StatusInternalServerError, "streaming not supported", nil, false)
		return
	}

	// Create Bedrock client
	client, err := bedrockVendor.NewBedrockClient(conf)
	if err != nil {
		p.sendStreamError(w, flusher, "failed to create Bedrock client", "server_error")
		return
	}

	// Determine model ID
	modelID := bedrockVendor.GetModelID(conf, req.Model)
	if modelID == "" {
		p.sendStreamError(w, flusher, "model ID is required", "invalid_request_error")
		return
	}

	// Convert messages
	chatMessages := openAIMessagesToChatMessages(req.Messages)
	converseMsgs, systemBlocks := bedrockVendor.ConvertChatMessagesToConverse(chatMessages)

	// Build configs
	inferenceConfig := bedrockVendor.BuildInferenceConfig(req.MaxCompletionTokens, req.Temperature, req.TopP, req.Stop)
	var toolConfig *types.ToolConfiguration
	if len(req.Tools) > 0 {
		toolDefs := openAIToolsToToolDefs(req.Tools)
		toolConfig = bedrockVendor.BuildToolConfig(toolDefs)
	}

	// Call ConverseStream
	input := &bedrockruntime.ConverseStreamInput{
		ModelId:         aws.String(modelID),
		Messages:        converseMsgs,
		System:          systemBlocks,
		InferenceConfig: inferenceConfig,
		ToolConfig:      toolConfig,
	}

	output, err := client.ConverseStream(r.Context(), input)
	if err != nil {
		p.sendStreamError(w, flusher, fmt.Sprintf("Bedrock ConverseStream failed: %s", err.Error()), "server_error")
		return
	}

	stream := output.GetStream()
	if stream == nil {
		p.sendStreamError(w, flusher, "Bedrock returned no stream", "server_error")
		return
	}
	defer stream.Close()

	completionID := "chatcmpl-" + uuid.New().String()
	created := time.Now().Unix()
	isFirstChunk := true
	var inputTokens, outputTokens int32

	for event := range stream.Events() {
		if err := stream.Err(); err != nil {
			log.Error().Err(err).Msg("Bedrock ConverseStream error")
			break
		}

		switch v := event.(type) {
		case *types.ConverseStreamOutputMemberMessageStart:
			// Send first chunk with role
			if isFirstChunk {
				chunkResp := ChatCompletionChunk{
					ID:      completionID,
					Object:  "chat.completion.chunk",
					Created: created,
					Model:   req.Model,
					Choices: []ChatCompletionChunkChoice{{
						Index: 0,
						Delta: ChatCompletionDelta{Role: "assistant"},
					}},
				}
				sendSSEChunk(w, flusher, chunkResp)
				isFirstChunk = false
			}
			_ = v

		case *types.ConverseStreamOutputMemberContentBlockDelta:
			if textDelta, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberText); ok {
				chunkResp := ChatCompletionChunk{
					ID:      completionID,
					Object:  "chat.completion.chunk",
					Created: created,
					Model:   req.Model,
					Choices: []ChatCompletionChunkChoice{{
						Index: 0,
						Delta: ChatCompletionDelta{Content: textDelta.Value},
					}},
				}
				sendSSEChunk(w, flusher, chunkResp)
			}

		case *types.ConverseStreamOutputMemberMessageStop:
			finishReason := bedrockVendor.ConvertConverseStopReason(v.Value.StopReason)
			chunkResp := ChatCompletionChunk{
				ID:      completionID,
				Object:  "chat.completion.chunk",
				Created: created,
				Model:   req.Model,
				Choices: []ChatCompletionChunkChoice{{
					Index:        0,
					Delta:        ChatCompletionDelta{},
					FinishReason: &finishReason,
				}},
			}
			sendSSEChunk(w, flusher, chunkResp)

		case *types.ConverseStreamOutputMemberMetadata:
			if v.Value.Usage != nil {
				inputTokens = aws.ToInt32(v.Value.Usage.InputTokens)
				outputTokens = aws.ToInt32(v.Value.Usage.OutputTokens)

				// Send final chunk with usage
				usage := CompletionUsage{
					PromptTokens:     int(inputTokens),
					CompletionTokens: int(outputTokens),
					TotalTokens:      int(inputTokens + outputTokens),
				}
				chunkResp := ChatCompletionChunk{
					ID:      completionID,
					Object:  "chat.completion.chunk",
					Created: created,
					Model:   req.Model,
					Choices: []ChatCompletionChunkChoice{{
						Index: 0,
						Delta: ChatCompletionDelta{},
					}},
					Usage: &usage,
				}
				sendSSEChunk(w, flusher, chunkResp)
			}
		}
	}

	// Send [DONE] marker
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()

	// Record analytics
	go recordBedrockStreamAnalytics(p, conf, app, int(inputTokens), int(outputTokens), reqBody, r, timestamp)
}

// --- Helper functions ---

func openAIMessagesToChatMessages(messages []Message) []bedrockVendor.ChatMessage {
	result := make([]bedrockVendor.ChatMessage, 0, len(messages))
	for _, msg := range messages {
		content := ""
		switch v := msg.Content.(type) {
		case string:
			content = v
		case []interface{}:
			// Multi-part content: extract text parts
			for _, part := range v {
				if partMap, ok := part.(map[string]interface{}); ok {
					if partMap["type"] == "text" {
						if text, ok := partMap["text"].(string); ok {
							content += text
						}
					}
				}
			}
		}
		result = append(result, bedrockVendor.ChatMessage{
			Role:    msg.Role,
			Content: content,
		})
	}
	return result
}

func openAIToolsToToolDefs(tools []Tool) []bedrockVendor.ToolDef {
	result := make([]bedrockVendor.ToolDef, 0, len(tools))
	for _, tool := range tools {
		result = append(result, bedrockVendor.ToolDef{
			Type: tool.Type,
			Function: bedrockVendor.ToolFuncDef{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
		})
	}
	return result
}

func converseOutputToOpenAI(output *bedrockruntime.ConverseOutput, model string) *ChatCompletionResponse {
	response := &ChatCompletionResponse{
		ID:      "chatcmpl-" + uuid.New().String(),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: make([]ChatCompletionChoice, 0),
	}

	// Extract content from output
	if msgOutput, ok := output.Output.(*types.ConverseOutputMemberMessage); ok {
		text := bedrockVendor.ExtractTextFromContentBlocks(msgOutput.Value.Content)
		toolCalls := bedrockVendor.ExtractToolCallsFromContentBlocks(msgOutput.Value.Content)

		choice := ChatCompletionChoice{
			Index: 0,
			Message: ChatCompletionMessage{
				Role:    "assistant",
				Content: text,
			},
			FinishReason: bedrockVendor.ConvertConverseStopReason(output.StopReason),
		}

		if len(toolCalls) > 0 {
			choice.Message.ToolCalls = toolCalls
			choice.Message.Content = ""
		}

		response.Choices = append(response.Choices, choice)
	}

	// Set usage
	if output.Usage != nil {
		response.Usage = CompletionUsage{
			PromptTokens:     int(aws.ToInt32(output.Usage.InputTokens)),
			CompletionTokens: int(aws.ToInt32(output.Usage.OutputTokens)),
			TotalTokens:      int(aws.ToInt32(output.Usage.TotalTokens)),
		}
	}

	return response
}

func sendSSEChunk(w http.ResponseWriter, flusher http.Flusher, chunk ChatCompletionChunk) {
	jsonBytes, err := json.Marshal(chunk)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", jsonBytes)
	flusher.Flush()
}

func recordBedrockAnalytics(p *Proxy, llm *models.LLM, app *models.App, output *bedrockruntime.ConverseOutput, reqBody []byte, respBody []byte, r *http.Request, timestamp time.Time) {
	const maxBodySize = 65535

	// Record proxy log
	proxyLog := &models.ProxyLog{
		AppID:        app.ID,
		UserID:       app.UserID,
		TimeStamp:    timestamp,
		Vendor:       string(llm.Vendor),
		RequestBody:  truncateString(string(reqBody), maxBodySize),
		ResponseBody: truncateString(string(respBody), maxBodySize),
		ResponseCode: http.StatusOK,
	}
	analytics.RecordProxyLog(proxyLog)

	// Record chat analytics
	if output.Usage != nil {
		promptTokens := int(aws.ToInt32(output.Usage.InputTokens))
		responseTokens := int(aws.ToInt32(output.Usage.OutputTokens))

		modelName := llm.DefaultModel
		price, err := p.gatewayService.GetModelPriceByModelNameAndVendor(modelName, string(llm.Vendor))
		if err != nil {
			log.Debug().Str("model", modelName).Str("vendor", string(llm.Vendor)).Msg("No pricing found for Bedrock model")
			price = &models.ModelPrice{}
		}

		cost := ((price.CPT * float64(responseTokens)) + (price.CPIT * float64(promptTokens))) * 10000

		record := &models.LLMChatRecord{
			LLMID:           llm.ID,
			Name:            modelName,
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
}

func recordBedrockStreamAnalytics(p *Proxy, llm *models.LLM, app *models.App, promptTokens int, responseTokens int, reqBody []byte, r *http.Request, timestamp time.Time) {
	if promptTokens == 0 && responseTokens == 0 {
		return
	}

	modelName := llm.DefaultModel
	price, err := p.gatewayService.GetModelPriceByModelNameAndVendor(modelName, string(llm.Vendor))
	if err != nil {
		log.Debug().Str("model", modelName).Str("vendor", string(llm.Vendor)).Msg("No pricing found for Bedrock model")
		price = &models.ModelPrice{}
	}

	cost := ((price.CPT * float64(responseTokens)) + (price.CPIT * float64(promptTokens))) * 10000

	record := &models.LLMChatRecord{
		LLMID:           llm.ID,
		Name:            modelName,
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

