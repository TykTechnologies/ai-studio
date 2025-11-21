package scripting

import (
	"github.com/tmc/langchaingo/llms"
)

// ScriptInput provides rich context to scripts including messages, metadata, and vendor info
type ScriptInput struct {
	RawInput   string                  // Full request/response JSON body
	Messages   []llms.MessageContent   // Normalized messages via extractors
	VendorName string                  // LLM vendor (e.g., "openai", "anthropic")
	ModelName  string                  // Model being called (e.g., "gpt-4")
	Context    map[string]interface{}  // Additional metadata (app_id, user_id, etc.)
	IsChat     bool                    // True if this is a chat session context
}

// ScriptOutput represents the result of script execution
type ScriptOutput struct {
	Block    bool                     // If true, stops the request/response chain
	Payload  string                   // Modified content (empty = no modification)
	Messages []map[string]interface{} // Modified messages array (alternative to Payload)
	Message  string                   // Optional blocking reason or log message
}
