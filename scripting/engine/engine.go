// Package engine is the hook through which the enterprise edition supplies
// the filter script runtime, and the types scripts exchange with it.
//
// It is a leaf package, apart from services, so that the enterprise
// registration (github.com/TykTechnologies/ai-studio-enterprise/v2/features/filters)
// can import it without importing package scripting, whose own tests
// register the engine. Package scripting aliases these types.
package engine

import (
	"context"
	"strings"
	"sync"

	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
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

	// Format names the wire shape of RawInput on a proxy request: the
	// request filter format (openai, anthropic, google_ai, ...). Guardrails
	// use it to segment the body block by block and rewrite it in place
	// (guardrails/segments). On a chat tool result it is FormatToolResult
	// (scripting); empty on other chat, tool and response inputs.
	Format string `json:"format,omitempty"`

	// MessageTools names, for each entry of Messages that is a tool result,
	// the tool and the call it answers, where the format's extractor knows
	// them: the OpenAI Responses API, whose tool outputs reach filters as
	// messages. Same length as Messages when set, nil otherwise. Guardrails
	// pass it to tool-aware providers and name the tools in compliance
	// events and quarantine placeholders. Not script-visible.
	MessageTools []MessageTool `json:"-"`

	// ResponseBody is the raw body of a buffered (not streamed) LLM
	// response, in ResponseFormat (a request filter format name), on
	// response filters; RequestBody is the request it answers, as forwarded.
	// Guardrails that scan tool calls read the response's tool calls from
	// the one and their goal from the other. Not script-visible: scripts
	// see the response's text in RawInput, as before.
	ResponseBody   string `json:"-"`
	ResponseFormat string `json:"-"`
	RequestBody    string `json:"-"`
	// RequestFormat is RequestBody's format when it is not ResponseFormat
	// (a stream's held tool calls, re-encoded as an OpenAI response).
	RequestFormat string `json:"-"`

	// RecordEvents, when set, records a filter's compliance events with the
	// call site's attribution (app, user, LLM, vendor, model, scope), as the
	// call site does with the output it is handed. A guardrail in shadow
	// mode returns at once and records its events through it when its
	// background evaluation finishes; with none set (the filter Test panel)
	// a shadow guardrail runs inline. Not script-visible.
	RecordEvents func(ctx context.Context, filterName string, out *ScriptOutput) `json:"-"`

	// Ctx, when set, bounds the execution: the script is aborted when the
	// context is cancelled, in addition to the FILTER_SCRIPT_TIMEOUT limit.
	// Call sites pass the request or session context so a caller that has
	// gone away does not leave a script running on its behalf. Never
	// serialised; it is not part of the script-visible input.
	Ctx context.Context `json:"-"`
}

// MessageTool is the tool behind one tool-result message: its name and the
// id of the call it answers. Zero for a message that is not a tool result.
// With Call set, the message is instead a tool call's arguments (an
// assistant message), and Name and CallID are the call's.
type MessageTool struct {
	Name   string
	CallID string
	Call   bool
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
	// ReasonCode and Reason explain a guardrail block to the caller: a
	// stable code (scripting.Reason*) and a sentence naming the cause and
	// where it was, never what was matched. Empty for script filters.
	ReasonCode string
	Reason     string
}

// Explain appends a guardrail block's reason to msg: "msg. Reason: <reason>
// (<code>)." It returns msg unchanged for a script filter's block.
func (o *ScriptOutput) Explain(msg string) string {
	if o == nil || o.ReasonCode == "" {
		return msg
	}
	msg = strings.TrimSpace(msg)
	if msg != "" && !strings.HasSuffix(msg, ".") {
		msg += "."
	}
	return strings.TrimSpace(msg + " Reason: " + o.Reason + " (" + o.ReasonCode + ").")
}

// Func runs one filter script: source against input. svc gives the script
// access to Studio's services and may be nil.
type Func func(source []byte, input *ScriptInput, svc services.ServiceInterface) (*ScriptOutput, error)

var (
	mu         sync.RWMutex
	registered Func
)

// Register installs the script runtime. The enterprise edition calls it from
// an init function; the Community Edition registers none, and scripts pass
// through unchanged.
func Register(f Func) {
	mu.Lock()
	defer mu.Unlock()
	registered = f
}

// Registered returns the installed runtime, or nil.
func Registered() Func {
	mu.RLock()
	defer mu.RUnlock()
	return registered
}

// Available reports whether a runtime is installed.
func Available() bool {
	return Registered() != nil
}
