package scripting

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/tmc/langchaingo/llms"
)

// Filter scopes recorded against compliance events raised by tool filters.
const (
	ToolInputScope  = "tool_input"
	ToolOutputScope = "tool_output"
)

// ToolBlockedMessage is what a caller is told when a governance filter stops a
// tool call, on every transport. It is deliberately generic and identical for
// input and output blocks: the filter's own name and reason go to the logs and
// to the compliance event, so a caller cannot probe the filters to map the
// governance configuration or learn whether the downstream tool was reached.
const ToolBlockedMessage = "blocked by policy"

// Event types synthesised when a filter blocks without reporting an event of
// its own, so that every block is auditable.
const (
	ToolInputBlockedEvent  = "tool_input_blocked"
	ToolOutputBlockedEvent = "tool_output_blocked"
)

// ToolCallArgs is the request shape a tool input filter sees, and may rewrite.
// It mirrors the body of the REST tool endpoint so that REST, MCP and chat tool
// calls all present scripts with a single contract.
type ToolCallArgs struct {
	OperationID string                 `json:"operation_id"`
	Parameters  map[string][]string    `json:"parameters"`
	Payload     map[string]interface{} `json:"payload"`
	Headers     map[string][]string    `json:"headers"`
}

// ToolFilterIdentity carries the call context a script can inspect and that
// compliance events are attributed to. Every field is best effort: the MCP path
// has no app when the caller authenticated with an OAuth bearer token, and the
// REST path has no chat session.
type ToolFilterIdentity struct {
	AppID     uint
	UserID    uint
	ToolID    uint
	ToolName  string
	SessionID string
	CallID    string
	IsChat    bool
}

// ToolBlock describes a tool call halted by a filter. The reason is for logs
// and compliance records only - it is deliberately never returned to the
// caller, which sees a generic refusal.
type ToolBlock struct {
	FilterName string
	Reason     string
}

func (b *ToolBlock) Error() string {
	return fmt.Sprintf("blocked by filter '%s': %s", b.FilterName, b.Reason)
}

// RunToolInputFilters executes the request-side filters attached to a tool
// against the arguments bound for it, returning the arguments to actually call
// the tool with. A non-nil ToolBlock means the tool must not be called.
func RunToolInputFilters(
	ctx context.Context,
	filters []models.Filter,
	service services.ServiceInterface,
	args ToolCallArgs,
	id ToolFilterIdentity,
) (ToolCallArgs, *ToolBlock, error) {
	selected := selectToolFilters(filters, false)
	if len(selected) == 0 {
		return args, nil, nil
	}

	raw, err := json.Marshal(args)
	if err != nil {
		return args, nil, fmt.Errorf("failed to encode tool arguments for filtering: %w", err)
	}

	warnToolFilterDirection(selected, id)

	current := string(raw)
	for _, filter := range selected {
		output, block, err := runToolFilter(ctx, filter, service, current, ToolInputScope, false, id)
		if block != nil || err != nil {
			return args, block, err
		}

		if output.Payload == "" || output.Payload == current {
			continue
		}

		// Only the mutable parts of the envelope are taken back. A filter must
		// not be able to redirect the call to a different operation.
		var rewritten ToolCallArgs
		if err := json.Unmarshal([]byte(output.Payload), &rewritten); err != nil {
			slog.Error("tool input filter returned an unparsable payload",
				"filter_name", filter.Name, "tool_id", id.ToolID, "error", err)
			return args, nil, fmt.Errorf("filter '%s' returned an invalid tool argument payload: %w", filter.Name, err)
		}

		if rewritten.OperationID != "" && rewritten.OperationID != args.OperationID {
			slog.Warn("tool input filter attempted to change the operation (ignored)",
				"filter_name", filter.Name, "tool_id", id.ToolID,
				"requested_operation", rewritten.OperationID, "operation", args.OperationID)
		}

		args.Parameters = rewritten.Parameters
		args.Payload = rewritten.Payload
		args.Headers = rewritten.Headers

		// Re-encode from the authoritative struct so the next filter in the
		// chain sees exactly what will be sent, operation ID included.
		next, err := json.Marshal(args)
		if err != nil {
			return args, nil, fmt.Errorf("failed to re-encode tool arguments after filter '%s': %w", filter.Name, err)
		}
		current = string(next)
	}

	return args, nil, nil
}

// RunToolOutputFilters executes the response-side filters attached to a tool
// against the body it returned, returning the body to hand back to the caller.
// A non-nil ToolBlock means the body must not be returned.
func RunToolOutputFilters(
	ctx context.Context,
	filters []models.Filter,
	service services.ServiceInterface,
	body string,
	id ToolFilterIdentity,
) (string, *ToolBlock, error) {
	selected := selectToolFilters(filters, true)
	if len(selected) == 0 {
		return body, nil, nil
	}

	current := body
	for _, filter := range selected {
		output, block, err := runToolFilter(ctx, filter, service, current, ToolOutputScope, true, id)
		if block != nil || err != nil {
			return body, block, err
		}

		if output.Payload != "" && output.Payload != current {
			current = output.Payload
		}
	}

	return current, nil, nil
}

// selectToolFilters picks the filters facing the given direction. Filters carry
// one direction flag shared with the LLM paths: ResponseFilter false runs on
// the way in, true on the way out.
func selectToolFilters(filters []models.Filter, responseSide bool) []models.Filter {
	selected := make([]models.Filter, 0, len(filters))
	for i := range filters {
		if filters[i].ResponseFilter == responseSide {
			selected = append(selected, filters[i])
		}
	}
	return selected
}

// runToolFilter executes a single filter and records whatever it reported.
//
// Unlike LLM response filters, tool filters fail closed: a script that errored
// enforced nothing, and letting the call through would silently drop the
// governance control it exists to provide.
func runToolFilter(
	ctx context.Context,
	filter models.Filter,
	service services.ServiceInterface,
	raw string,
	scope string,
	isResponse bool,
	id ToolFilterIdentity,
) (*ScriptOutput, *ToolBlock, error) {
	slog.Debug("executing tool filter",
		"filter_name", filter.Name, "scope", scope, "tool_id", id.ToolID, "tool_name", id.ToolName)

	input := &ScriptInput{
		RawInput: raw,
		Messages: []llms.MessageContent{
			{
				Role:  llms.ChatMessageTypeTool,
				Parts: []llms.ContentPart{llms.TextPart(raw)},
			},
		},
		Context:    id.scriptContext(),
		IsChat:     id.IsChat,
		IsResponse: isResponse,
	}

	output, err := runToolScript(filter, service, input)
	if err != nil {
		slog.Error("tool filter execution error",
			"filter_name", filter.Name, "scope", scope, "tool_id", id.ToolID, "error", err)
		recordToolBlockEvent(ctx, filter.Name, scope, "filter execution error", id)
		return nil, nil, fmt.Errorf("filter '%s' error: %w", filter.Name, err)
	}

	RecordComplianceEvents(ctx, output, filter.Name, scope, id.AppID, id.UserID, 0, "", "")

	if output.Block {
		reason := output.Message
		if reason == "" {
			reason = "blocked by policy"
		}

		slog.Info("tool call blocked by filter",
			"filter_name", filter.Name, "scope", scope,
			"tool_id", id.ToolID, "tool_name", id.ToolName, "reason", reason)

		if len(output.ComplianceEvents) == 0 {
			recordToolBlockEvent(ctx, filter.Name, scope, reason, id)
		}

		return output, &ToolBlock{FilterName: filter.Name, Reason: reason}, nil
	}

	return output, nil, nil
}

// directionNoticeSeen tracks which tool/filter pairs have already had their
// direction reported, so the notice below is emitted once per pair per process.
var directionNoticeSeen sync.Map

// warnToolFilterDirection announces, once per tool and filter, that a filter is
// governing tool input.
//
// Before tool filters honoured their direction flag, every filter attached to a
// tool ran on the tool's response regardless of how it was configured. An
// existing request-direction attachment therefore changes what it governs on
// upgrade, and the change is otherwise invisible - so it is called out the
// first time each such filter runs.
func warnToolFilterDirection(filters []models.Filter, id ToolFilterIdentity) {
	for _, filter := range filters {
		key := fmt.Sprintf("%d:%d", id.ToolID, filter.ID)
		if _, seen := directionNoticeSeen.LoadOrStore(key, struct{}{}); seen {
			continue
		}

		slog.Warn("tool filter is governing tool input, not tool responses; "+
			"filters attached to tools before directions were honoured ran on responses regardless of this flag",
			"filter_name", filter.Name, "filter_id", filter.ID,
			"tool_id", id.ToolID, "tool_name", id.ToolName)
	}
}

// runToolScript executes one filter script, converting a panic into an error.
//
// The Tengo VM panics rather than returning an error on some runtime faults
// (integer divide by zero, for one), and a tool call is served on the caller's
// own goroutine, so an unrecovered panic in a filter would take the whole
// gateway process down. Recovering here turns a bad script into a fail-closed
// refusal of the one call that triggered it.
func runToolScript(filter models.Filter, service services.ServiceInterface, input *ScriptInput) (output *ScriptOutput, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			output = nil
			err = fmt.Errorf("filter script panicked: %v", recovered)
		}
	}()

	return NewScriptRunner(filter.Script).RunScript(input, service)
}

// recordToolBlockEvent synthesises a compliance event for a block the script
// did not report itself, so a block is never invisible to compliance reporting.
func recordToolBlockEvent(ctx context.Context, filterName, scope, reason string, id ToolFilterIdentity) {
	eventType := ToolInputBlockedEvent
	if scope == ToolOutputScope {
		eventType = ToolOutputBlockedEvent
	}

	synthetic := &ScriptOutput{
		ComplianceEvents: []ComplianceEventOutput{
			{
				EventType:   eventType,
				Severity:    "critical",
				Description: reason,
				Metadata: map[string]interface{}{
					"tool_id":   id.ToolID,
					"tool_name": id.ToolName,
				},
			},
		},
	}

	RecordComplianceEvents(ctx, synthetic, filterName, scope, id.AppID, id.UserID, 0, "", "")
}

// scriptContext builds the input.context map a tool filter script reads.
// Integers are widened to int64 because Tengo has no narrower integer type.
func (id ToolFilterIdentity) scriptContext() map[string]interface{} {
	scriptCtx := map[string]interface{}{
		"tool_id":   int64(id.ToolID),
		"tool_name": id.ToolName,
		"app_id":    int64(id.AppID),
		"user_id":   int64(id.UserID),
	}

	if id.SessionID != "" {
		scriptCtx["session_id"] = id.SessionID
	}

	if id.CallID != "" {
		scriptCtx["call_id"] = id.CallID
	}

	return scriptCtx
}
