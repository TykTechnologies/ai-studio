package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/tmc/langchaingo/llms"
)

// Handlers
func (p *Proxy) CreateCompletionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	routeID := vars["routeId"]

	// get the route ID from the DB to find out what back-end LLM to use
	conf, ok := p.llms[routeID]
	if !ok {
		http.Error(w, "Route not found", http.StatusNotFound)
		return
	}

	var req CreateCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate model
	validator := NewModelValidator(conf.AllowedModels)
	if !validator.IsModelAllowed(req.Model) {
		respondWithOAIError(w, http.StatusForbidden, fmt.Sprintf("Model '%s' is not allowed", req.Model), nil)
		return
	}

	// create a standard llangchain completion request based on the input
	llm, err := switches.FetchDriver(conf, nil, nil, func(ctx context.Context, chunk []byte) error { return nil })
	if err != nil {
		http.Error(w, "Failed to create LLM client", http.StatusInternalServerError)
		return
	}

	// send the request to the LLM
	ctx := context.Background()
	// 1. create the options
	opts := handleOptions(&req)

	if req.Stream != nil {
		http.Error(w, "Streaming is not supported", http.StatusBadRequest)
		return
	}

	// 3. call the LLM
	slog.Warn(" completions API is deprecated, use the /v1/chat/completions API instead (no analytics stored)")
	resp, err := llm.Call(ctx, req.Prompt, opts...)

	// convert the response to OpenAI format
	response := CompletionResponse{
		ID: "completion-" + uuid.New().String(),
		Choices: []CompletionChoice{
			{
				Text:         resp,
				FinishReason: "stop",
			},
		},
		Model: req.Model,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (p *Proxy) CreateChatCompletionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	routeID := vars["routeId"]

	// get the route ID from the DB to find out what back-end LLM to use
	conf, ok := p.llms[routeID]
	if !ok {
		respondWithOAIError(w, http.StatusNotFound, "vendor not found", nil)
		return
	}

	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithOAIError(w, http.StatusBadRequest, "Invalid request body", err)
		return
	}

	// Validate model
	validator := NewModelValidator(conf.AllowedModels)
	if !validator.IsModelAllowed(req.Model) {
		respondWithOAIError(w, http.StatusForbidden, fmt.Sprintf("Model '%s' is not allowed", req.Model), nil)
		return
	}

	// Validate required fields
	if len(req.Messages) == 0 || req.Model == "" {
		respondWithOAIError(w, http.StatusBadRequest, "missing required fields", nil)
		return
	}

	// Handle streaming if requested
	if req.Stream != nil && *req.Stream {
		respondWithOAIError(w, http.StatusBadRequest, "streaming not supported", nil)
		return
	}

	// create a standard llangchain completion request based on the input
	llm, err := switches.FetchDriver(conf, nil, nil, func(ctx context.Context, chunk []byte) error { return nil })
	if err != nil {
		respondWithOAIError(w, http.StatusInternalServerError, "Failed to create LLM client", err)
		return
	}

	ctx := context.Background()
	opts := req.ToLangchainOptions(conf)
	messages := req.GetMessages()

	resp, err := llm.GenerateContent(ctx, messages, opts...)
	if err != nil {
		respondWithOAIError(w, http.StatusInternalServerError, "failed to generate content", err)
		return
	}

	// Create response
	response := NewChatCompletionResponse(resp, req.Model)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleOptions(req *CreateCompletionRequest) []llms.CallOption {
	opts := make([]llms.CallOption, 0)
	if req.MaxTokens != nil {
		opts = append(opts, llms.WithMaxTokens(*req.MaxTokens))
	}

	if req.Temperature != nil {
		opts = append(opts, llms.WithTemperature(*req.Temperature))
	}

	if req.TopP != nil {
		opts = append(opts, llms.WithTopP(*req.TopP))
	}

	if req.PresencePenalty != nil {
		opts = append(opts, llms.WithPresencePenalty(*req.PresencePenalty))
	}

	if req.FrequencyPenalty != nil {
		opts = append(opts, llms.WithFrequencyPenalty(*req.FrequencyPenalty))
	}

	if req.Model != "" {
		opts = append(opts, llms.WithModel(req.Model))
	}

	if req.Stop != "" {
		stopWords := []string{}
		switch req.Stop.(type) {
		case string:
			stopWords = append(stopWords, req.Stop.(string))
		case []string:
			stopWords = req.Stop.([]string)
		}

		opts = append(opts, llms.WithStopWords(stopWords))
	}

	return opts
}
