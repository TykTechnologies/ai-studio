package chat_session

import (
	"fmt"
	"log/slog"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/tmc/langchaingo/llms"
)

// ExecuteResponseFilters executes response-side filters on chat LLM responses
// Returns whether the response should be blocked and an optional block message
func ExecuteResponseFilters(
	filters []*models.Filter,
	service services.ServiceInterface,
	responseText string,
	vendor string,
	modelName string,
	isStreaming bool,
	isChunk bool,
	chunkIndex int,
	currentBuffer string,
	sessionID string,
	userID uint,
	chatID uint,
) (blocked bool, blockMessage string, err error) {
	// Filter to only response filters (ResponseFilter = true)
	responseFilters := []*models.Filter{}
	for _, filter := range filters {
		if filter.ResponseFilter {
			responseFilters = append(responseFilters, filter)
		}
	}

	// If no response filters, pass through
	if len(responseFilters) == 0 {
		return false, "", nil
	}

	// Build script input for response context
	scriptInput := &scripting.ScriptInput{
		RawInput:   responseText,
		Messages:   []llms.MessageContent{}, // Empty for response filters
		VendorName: vendor,
		ModelName:  modelName,
		Context: map[string]interface{}{
			"session_id": sessionID,
			"user_id":    int64(userID),
			"chat_id":    int64(chatID),
		},
		IsChat:        true,
		IsResponse:    true,
		IsChunk:       isChunk,
		ChunkIndex:    chunkIndex,
		CurrentBuffer: currentBuffer,
		StatusCode:    200, // Chat responses don't have HTTP status codes
	}

	// Execute response filters in chain
	for _, filter := range responseFilters {
		slog.Debug("executing chat response filter", "filter_name", filter.Name, "is_chunk", isChunk, "chunk_index", chunkIndex)

		runner := scripting.NewScriptRunner(filter.Script)
		output, err := runner.RunScript(scriptInput, service)
		if err != nil {
			slog.Error("chat response filter execution error", "filter_name", filter.Name, "error", err)
			// On error, allow response through (fail open for safety)
			return false, "", fmt.Errorf("filter '%s' error: %w", filter.Name, err)
		}

		// Check if filter blocks the response
		if output.Block {
			msg := output.Message
			if msg == "" {
				msg = fmt.Sprintf("Response blocked by filter: %s", filter.Name)
			}
			slog.Info("chat response blocked by filter", "filter_name", filter.Name, "message", msg, "is_chunk", isChunk)
			return true, msg, nil
		}

		// Note: Response filters are block-only, payload modifications are ignored
		if output.Payload != "" && output.Payload != scriptInput.RawInput {
			slog.Warn("chat response filter attempted to modify payload (not supported)", "filter_name", filter.Name)
		}
	}

	// All filters passed
	return false, "", nil
}
