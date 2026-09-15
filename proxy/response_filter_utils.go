package proxy

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/tmc/langchaingo/llms"
)

// ExecuteResponseFilters executes response-side filters on LLM responses
// Returns whether the response should be blocked and an optional block message
func ExecuteResponseFilters(
	llm *models.LLM,
	service services.ServiceInterface,
	responseBody []byte,
	statusCode int,
	isStreaming bool,
	isChunk bool,
	chunkIndex int,
	currentBuffer string,
	r *http.Request,
) (blocked bool, blockMessage string, err error) {
	if !hasResponseFilterList(llm) {
		return false, "", nil
	}

	// Extract response text from the response body
	// For chunks, we use the chunk text; for full responses, we extract from JSON
	responseText := ""
	if isChunk {
		// For streaming chunks, extract text from the chunk
		responseText = switches.ExtractStreamingChunkText(llm.Vendor, responseBody)
	} else {
		// For full responses, extract complete text
		responseText, err = switches.ExtractResponseText(llm.Vendor, responseBody)
		if err != nil {
			slog.Warn("failed to extract response text for filtering, allowing response", "error", err)
			return false, "", nil
		}
	}

	return runResponseFilters(llm, service, responseText, statusCode, isChunk, false, chunkIndex, currentBuffer, r)
}

// ExecuteFinalResponseFilters runs the response filters once more when a
// streaming response has completed, over the whole accumulated text. Per-chunk
// evaluation can legitimately skip a response that never reaches a script's
// buffer threshold or a guardrail's cadence; this pass makes sure every
// streamed response is looked at in full at least once. The chunks are
// already with the client, so a block here ends the stream with an error
// event and is recorded, rather than withholding content.
func ExecuteFinalResponseFilters(
	llm *models.LLM,
	service services.ServiceInterface,
	statusCode int,
	chunkIndex int,
	fullText string,
	r *http.Request,
) (blocked bool, blockMessage string, err error) {
	if !hasResponseFilterList(llm) || fullText == "" {
		return false, "", nil
	}
	return runResponseFilters(llm, service, "", statusCode, true, true, chunkIndex, fullText, r)
}

func hasResponseFilterList(llm *models.LLM) bool {
	for _, filter := range llm.Filters {
		if filter.ResponseFilter {
			return true
		}
	}
	return false
}

func runResponseFilters(
	llm *models.LLM,
	service services.ServiceInterface,
	responseText string,
	statusCode int,
	isChunk bool,
	isFinal bool,
	chunkIndex int,
	currentBuffer string,
	r *http.Request,
) (blocked bool, blockMessage string, err error) {
	// Get model name from request context
	modelName := ""
	if modelFromCtx := r.Context().Value("model_name"); modelFromCtx != nil {
		if modelStr, ok := modelFromCtx.(string); ok {
			modelName = modelStr
		}
	}

	// Get app ID from request context
	appID := int64(0)
	if appIDFromCtx := r.Context().Value("app_id"); appIDFromCtx != nil {
		if appIDInt, ok := appIDFromCtx.(int); ok {
			appID = int64(appIDInt)
		} else if appIDInt64, ok := appIDFromCtx.(int64); ok {
			appID = appIDInt64
		}
	}

	// Build script input for response context
	scriptInput := &scripting.ScriptInput{
		RawInput:   responseText,
		Messages:   []llms.MessageContent{}, // Empty for response filters
		VendorName: string(llm.Vendor),
		ModelName:  modelName,
		Context: map[string]interface{}{
			"llm_id":     int64(llm.ID),
			"app_id":     appID,
			"request_id": r.Header.Get("X-Request-ID"),
		},
		IsChat:        false,
		IsResponse:    true,
		IsChunk:       isChunk,
		IsFinal:       isFinal,
		ChunkIndex:    chunkIndex,
		CurrentBuffer: currentBuffer,
		StatusCode:    statusCode,
		Ctx:           r.Context(),
	}

	// Execute response filters in chain
	for _, filter := range llm.Filters {
		if !filter.ResponseFilter {
			continue
		}
		slog.Debug("executing response filter", "filter_name", filter.Name, "is_chunk", isChunk, "is_final", isFinal, "chunk_index", chunkIndex)

		runner := scripting.NewFilterRunner(filter)
		output, err := runner.RunScript(scriptInput, service)
		if err != nil {
			slog.Error("response filter execution error", "filter_name", filter.Name, "error", err)
			// On error, allow response through (fail open for safety)
			return false, "", fmt.Errorf("filter '%s' error: %w", filter.Name, err)
		}

		// Record any compliance events reported by the script
		scripting.RecordComplianceEvents(r.Context(), output, filter.Name, "proxy_response", uint(appID), 0, llm.ID, string(llm.Vendor), modelName)

		// Check if filter blocks the response
		if output.Block {
			msg := output.Message
			if msg == "" {
				msg = fmt.Sprintf("Response blocked by filter: %s", filter.Name)
			}
			slog.Info("response blocked by filter", "filter_name", filter.Name, "message", msg, "is_chunk", isChunk, "is_final", isFinal)
			return true, msg, nil
		}

		// Note: Response filters are block-only, payload modifications are ignored
		if output.Payload != "" && output.Payload != scriptInput.RawInput {
			slog.Warn("response filter attempted to modify payload (not supported)", "filter_name", filter.Name)
		}
	}

	// All filters passed
	return false, "", nil
}
