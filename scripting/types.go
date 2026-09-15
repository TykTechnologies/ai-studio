package scripting

import (
	"context"

	"github.com/tmc/langchaingo/llms"
)

// ScriptInput provides rich context to scripts including messages, metadata, and vendor info
type ScriptInput struct {
	RawInput      string                 `json:"raw_input"`      // Full request/response JSON body
	Messages      []llms.MessageContent  `json:"messages"`       // Normalized messages via extractors
	VendorName    string                 `json:"vendor_name"`    // LLM vendor (e.g., "openai", "anthropic")
	ModelName     string                 `json:"model_name"`     // Model being called (e.g., "gpt-4")
	Context       map[string]interface{} `json:"context"`        // Additional metadata (app_id, user_id, etc.)
	IsChat        bool                   `json:"is_chat"`        // True if this is a chat session context
	IsResponse    bool                   `json:"is_response"`    // True if this is response-side filtering
	IsChunk       bool                   `json:"is_chunk"`       // True if this is a streaming chunk
	ChunkIndex    int                    `json:"chunk_index"`    // Current chunk number (for streaming)
	CurrentBuffer string                 `json:"current_buffer"` // Accumulated response text (for streaming)
	StatusCode    int                    `json:"status_code"`    // HTTP status code from LLM
	// IsFinal marks the evaluation run once when a streaming response has
	// completed: IsChunk is true, RawInput is empty and CurrentBuffer holds
	// the whole response. Guardrails always evaluate on it, so a response too
	// short to reach the streaming cadence is still checked; scripts see it
	// as one more chunk.
	IsFinal bool `json:"is_final"`

	// Ctx, when set, bounds the execution: the script is aborted when the
	// context is cancelled, in addition to the FILTER_SCRIPT_TIMEOUT limit.
	// Call sites pass the request or session context so a caller that has
	// gone away does not leave a script running on its behalf. Never
	// serialised; it is not part of the script-visible input.
	Ctx context.Context `json:"-"`
}

// ComplianceEventOutput represents a compliance event reported by a filter script.
// Script developers set these in the output object to flag compliance-relevant activity.
type ComplianceEventOutput struct {
	EventType   string                 // Free-form type: "pii_redacted", "content_rewritten", "silent_failure", etc.
	Severity    string                 // "info", "warning", "critical"
	Description string                 // Human-readable description of what happened
	Metadata    map[string]interface{} // Arbitrary key-value data
}

// ScriptOutput represents the result of script execution
type ScriptOutput struct {
	Block            bool                     // If true, stops the request/response chain
	Payload          string                   // Modified content (empty = no modification)
	Messages         []map[string]interface{} // Modified messages array (alternative to Payload)
	Message          string                   // Optional blocking reason or log message
	ComplianceEvents []ComplianceEventOutput  // Compliance events to record (enterprise only)
}
