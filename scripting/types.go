package scripting

import (
	"github.com/tmc/langchaingo/llms"
)

// ScriptInput provides rich context to scripts including messages, metadata, and vendor info
type ScriptInput struct {
	RawInput      string                  `json:"raw_input"`      // Full request/response JSON body
	Messages      []llms.MessageContent   `json:"messages"`       // Normalized messages via extractors
	VendorName    string                  `json:"vendor_name"`    // LLM vendor (e.g., "openai", "anthropic")
	ModelName     string                  `json:"model_name"`     // Model being called (e.g., "gpt-4")
	Context       map[string]interface{}  `json:"context"`        // Additional metadata (app_id, user_id, etc.)
	IsChat        bool                    `json:"is_chat"`        // True if this is a chat session context
	IsResponse    bool                    `json:"is_response"`    // True if this is response-side filtering
	IsChunk       bool                    `json:"is_chunk"`       // True if this is a streaming chunk
	ChunkIndex    int                     `json:"chunk_index"`    // Current chunk number (for streaming)
	CurrentBuffer string                  `json:"current_buffer"` // Accumulated response text (for streaming)
	StatusCode    int                     `json:"status_code"`    // HTTP status code from LLM
}

// ScriptOutput represents the result of script execution
type ScriptOutput struct {
	Block    bool                     // If true, stops the request/response chain
	Payload  string                   // Modified content (empty = no modification)
	Messages []map[string]interface{} // Modified messages array (alternative to Payload)
	Message  string                   // Optional blocking reason or log message
}
