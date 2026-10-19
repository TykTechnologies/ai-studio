package chat_session

import (
	"context"
	"errors"
	"log/slog"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
)

// errTurnFailed is what the user is told when a turn ended on an internal
// error (a recovered panic). It carries nothing of the panic.
var errTurnFailed = errors.New("the request could not be processed")

// errToolResultBlocked is what the user is told when a chat guardrail blocks
// a tool result. Like scripting.ToolBlockedMessage for the model, it names
// neither the filter nor the content.
var errToolResultBlocked = errors.New("a tool result was blocked by policy; the turn was stopped")

// filterPass is one use of the chat's request filters: which of them run
// and how the text is presented to them.
type filterPass struct {
	// keep selects among the request filters; nil keeps them all.
	keep func(*models.Filter) bool
	role llms.ChatMessageType
	// format is ScriptInput.Format; extra is added to the context.
	format string
	extra  map[string]interface{}
}

// filterStop is the filter that stopped a pass: it blocked (out) or failed
// (err).
type filterStop struct {
	filter *models.Filter
	out    *scripting.ScriptOutput
	err    error
}

// runRequestFilters runs the chat's request filters that pass keeps over
// text, in order, each seeing the text the one before it returned, and
// records their compliance events as chat_request events. It stops at the
// first filter that blocks or fails.
func (cs *ChatSession) runRequestFilters(ctx context.Context, text string, pass filterPass) (string, *filterStop) {
	if ctx == nil {
		ctx = context.Background()
	}
	for _, filter := range cs.filters {
		if filter.ResponseFilter || (pass.keep != nil && !pass.keep(filter)) {
			continue
		}
		input := &scripting.ScriptInput{
			RawInput: text,
			Messages: []llms.MessageContent{{
				Role:  pass.role,
				Parts: []llms.ContentPart{llms.TextPart(text)},
			}},
			VendorName: string(cs.chatRef.LLM.Vendor),
			ModelName:  cs.chatRef.LLMSettings.ModelName,
			Context: map[string]interface{}{
				"session_id": cs.ID(),
				"user_id":    int64(cs.userID),
				"chat_id":    int64(cs.chatRef.ID),
			},
			IsChat: true,
			Format: pass.format,
			Ctx:    ctx,
			RecordEvents: scripting.EventRecorder("chat_request", 0, cs.userID, cs.chatRef.LLM.ID,
				string(cs.chatRef.LLM.Vendor), cs.chatRef.LLMSettings.ModelName),
		}
		for k, v := range pass.extra {
			input.Context[k] = v
		}
		out, err := scripting.NewFilterRunner(filter).RunScript(input, cs.service)
		if err != nil {
			return "", &filterStop{filter: filter, err: err}
		}
		scripting.RecordComplianceEvents(ctx, out, filter.Name, "chat_request", 0, cs.userID, cs.chatRef.LLM.ID,
			string(cs.chatRef.LLM.Vendor), cs.chatRef.LLMSettings.ModelName)
		if out.Block {
			return "", &filterStop{filter: filter, out: out}
		}
		if out.Payload != "" {
			text = out.Payload
		}
	}
	return text, nil
}

// runScriptRequestFilters runs the chat's script (Tengo) request filters over
// text the user supplied outside a typed message (an answer to a client
// tool card), with the same input a typed message gets, so a script DLP
// filter cannot be routed around through a tool card. blocked is true when a
// filter blocks or errors, as on the typed-message path.
func (cs *ChatSession) runScriptRequestFilters(ctx context.Context, text string) (filtered string, blocked bool) {
	filtered, stop := cs.runRequestFilters(ctx, text, filterPass{
		keep: func(f *models.Filter) bool { return !f.IsGuardrail() },
		role: llms.ChatMessageTypeHuman,
	})
	if stop == nil {
		return filtered, false
	}
	if stop.err != nil {
		slog.Error("chat request filter on a client tool answer failed; blocking it",
			"filter_name", stop.filter.Name, "session_id", cs.ID(), "error", stop.err)
	}
	return "", true
}

// screenToolResult runs the chat's request-side guardrail filters over the
// output of a tool before the model, the client or the stored history sees
// it: tool output (a fetched page, an API response, a user's answer to a
// client tool) is how instructions reach the model without the user typing
// them. Each guardrail that scans tool results (the default) applies its
// tool-result action: quarantine replaces the output with its placeholder,
// redact rewrites it, block stops the turn (blocked is true). Script chat
// filters are not run here; they keep seeing only typed messages. A
// guardrail that errors blocks, as chat request filters do.
func (cs *ChatSession) screenToolResult(ctx context.Context, toolName, toolCallID, text string) (screened string, blocked bool) {
	screened, stop := cs.runRequestFilters(ctx, text, filterPass{
		keep:   (*models.Filter).IsGuardrail,
		role:   llms.ChatMessageTypeTool,
		format: scripting.FormatToolResult,
		extra:  map[string]interface{}{"tool_name": toolName, "tool_call_id": toolCallID},
	})
	switch {
	case stop == nil:
		return screened, false
	case stop.err != nil:
		slog.Error("chat guardrail on a tool result failed; blocking it",
			"filter_name", stop.filter.Name, "session_id", cs.ID(), "error", stop.err)
	default:
		slog.Info("tool result blocked by a chat guardrail",
			"filter_name", stop.filter.Name, "tool", toolName, "session_id", cs.ID())
	}
	return "", true
}
