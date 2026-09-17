package chat_session

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/tmc/langchaingo/llms"
)

// Client (human-in-the-loop) tools.
//
// A ToolTypeClient tool is offered to the model like any function. When the
// model calls it the session cannot execute it: it records the call as
// pending, persists the tool-call request row, ends the turn with
// FinishToolCalls and waits. The browser renders the call (form / approval),
// the user answers, and the answer arrives as a UserMessage with ToolResults,
// which the session persists as the tool-response row before calling the
// model again. A plain user message while calls are pending abandons them
// with an error result so the stored history stays well-formed.

// ClientToolInfo describes a client tool to the browser.
type ClientToolInfo struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Schema      map[string]interface{} `json:"schema"`
	UI          models.ClientToolUI    `json:"ui"`
}

// pendingClientCall is a parked tool call.
type pendingClientCall struct {
	id   string
	name string
	ui   models.ClientToolUI // how the answer is collected, hence its expected shape
}

// clientToolState is the session's view of one parked turn.
type clientToolState struct {
	pending []pendingClientCall
	partial llms.MessageContent // results of server-side tools from the same reply
}

// ClientTools lists the client tools attached to the session.
func (cs *ChatSession) ClientTools() []ClientToolInfo {
	out := []ClientToolInfo{}
	for _, t := range cs.snapshotTools() {
		if t.ToolType != models.ToolTypeClient {
			continue
		}
		def, err := t.ClientDefinition()
		if err != nil {
			slog.Warn("skipping client tool with invalid definition", "tool", t.Name, "error", err)
			continue
		}
		out = append(out, ClientToolInfo{Name: t.ClientOperation(), Description: t.Description, Schema: def.Parameters, UI: def.UI})
	}
	return out
}

// AwaitingClientTools reports whether the last turn is parked on client tools.
func (cs *ChatSession) AwaitingClientTools() bool {
	cs.stateMu.Lock()
	defer cs.stateMu.Unlock()
	return cs.clientState != nil && len(cs.clientState.pending) > 0
}

// PendingClientCallIDs returns the ids of the parked calls.
func (cs *ChatSession) PendingClientCallIDs() []string {
	cs.stateMu.Lock()
	defer cs.stateMu.Unlock()
	if cs.clientState == nil {
		return nil
	}
	ids := make([]string, 0, len(cs.clientState.pending))
	for _, p := range cs.clientState.pending {
		ids = append(ids, p.id)
	}
	return ids
}

// DecodeToolSpec replaces the tool's stored spec (base64, the API contract)
// with its plain text before the tool joins a session. Every path that
// attaches a tool goes through here: default tools, dependencies and tools
// picked in the chat window. A client tool parses its own definition and
// accepts either form (see Tool.ClientDefinition), so one stored as raw JSON
// is left as it is instead of failing the add. On error the spec is untouched.
func DecodeToolSpec(t *models.Tool) error {
	decoded, err := helpers.DecodeToUTF8(t.OASSpec)
	if err != nil {
		if t.ToolType == models.ToolTypeClient {
			if _, defErr := t.ClientDefinition(); defErr == nil {
				return nil
			}
		}
		return err
	}
	t.OASSpec = decoded
	return nil
}

// clientToolDefinition converts a client tool into the function definition
// the model sees.
func clientToolDefinition(t models.Tool) (llms.Tool, error) {
	def, err := t.ClientDefinition()
	if err != nil {
		return llms.Tool{}, err
	}
	desc := t.Description
	if def.UI.Description != "" {
		desc = strings.TrimSpace(desc + "\n" + def.UI.Description)
	}
	return llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        t.ClientOperation(),
			Description: desc,
			Parameters:  def.Parameters,
		},
	}, nil
}

// parkClientCall records a client tool call to be answered by the user.
func (cs *ChatSession) parkClientCall(t llms.ToolCall, tool models.Tool) {
	ui := models.ClientToolUI{Kind: models.ClientToolKindApproval}
	if def, err := tool.ClientDefinition(); err == nil {
		ui = def.UI
	}
	cs.stateMu.Lock()
	defer cs.stateMu.Unlock()
	if cs.clientState == nil {
		cs.clientState = &clientToolState{partial: llms.MessageContent{Role: llms.ChatMessageTypeTool}}
	}
	cs.clientState.pending = append(cs.clientState.pending, pendingClientCall{id: t.ID, name: t.FunctionCall.Name, ui: ui})
}

// parkTurn stores the partial (server-side) tool results of a reply that
// also contained client calls, so they are persisted together later.
func (cs *ChatSession) parkTurn(partial llms.MessageContent) {
	cs.stateMu.Lock()
	defer cs.stateMu.Unlock()
	if cs.clientState == nil {
		cs.clientState = &clientToolState{}
	}
	cs.clientState.partial = partial
}

// takeClientState clears and returns the parked state.
func (cs *ChatSession) takeClientState() *clientToolState {
	cs.stateMu.Lock()
	defer cs.stateMu.Unlock()
	st := cs.clientState
	cs.clientState = nil
	return st
}

// resumeWithToolResults persists the user's answers for the parked calls and
// calls the model again. It returns true when a model response was
// dispatched to the queue.
func (cs *ChatSession) resumeWithToolResults(results []models.ToolResult) bool {
	st := cs.takeClientState()
	if st == nil || len(st.pending) == 0 {
		cs.sendError(fmt.Errorf("no client tool call is waiting for a result"))
		return false
	}

	byID := map[string]models.ToolResult{}
	for _, r := range results {
		byID[r.ToolCallID] = r
	}

	toolResult := st.partial
	toolResult.Role = llms.ChatMessageTypeTool
	for _, p := range st.pending {
		r, ok := byID[p.id]
		// The answer is validated against the card's shape and stored in a
		// labelled envelope so the model reads it as user input, not as a
		// trusted tool response (see client_results.go).
		content, _ := wrapClientResult(p, r, ok)
		// No tool-result event here: the client supplied the answer and
		// already holds it; the resumed stream carries only what follows.
		toolResult.Parts = append(toolResult.Parts, llms.ToolCallResponse{ToolCallID: p.id, Name: p.name, Content: content})
	}

	if err := cs.chatHistory.AddMessage(context.Background(), toolResult); err != nil {
		cs.sendError(fmt.Errorf("error adding tool results to history: %v", err))
		return false
	}
	return cs.callModelAfterTools()
}

// abandonPendingClientCalls closes parked calls with an error result so the
// stored history remains valid when the user moves on without answering.
func (cs *ChatSession) abandonPendingClientCalls() {
	st := cs.takeClientState()
	if st == nil || len(st.pending) == 0 {
		return
	}
	toolResult := st.partial
	toolResult.Role = llms.ChatMessageTypeTool
	for _, p := range st.pending {
		toolResult.Parts = append(toolResult.Parts, llms.ToolCallResponse{ToolCallID: p.id, Name: p.name, Content: "ERROR: the user did not respond"})
	}
	if err := cs.chatHistory.AddMessage(context.Background(), toolResult); err != nil {
		slog.Warn("could not persist abandoned client tool results", "session_id", cs.id, "error", err)
	}
}

// callModelAfterTools re-calls the model with the stored history (which now
// ends in tool results) and queues the reply. Shared by the server-side tool
// loop and the client-tool resume path.
func (cs *ChatSession) callModelAfterTools() bool {
	history, err := cs.getMessages()
	if err != nil {
		cs.sendError(fmt.Errorf("error getting updated history: %v", err))
		return false
	}

	// Regenerate options from current session state instead of using w.Opts
	// This ensures we have the correct tools and settings after NATS deserialization
	tools := cs.prepareTools()
	currentOpts := cs.getOptions(cs.chatRef.LLMSettings, tools)

	// Check token length and get LLM response based on updated history
	history = cs.PreflightTokenLengthCheck(history)

	// Reset streaming buffer/index for new request
	cs.streamBuffer = ""
	cs.streamChunkIndex = 0
	cs.streamFilterBlocked = false
	cs.resetStreamed()

	resp, err := cs.caller.GenerateContent(cs.runCtx(), history, currentOpts...)
	if err != nil {
		cs.sendError(fmt.Errorf("error getting LLM response after tool call: %v", err))
		return false
	}

	// Send the new LLM response to continue the conversation
	// Note: We still use nil for Opts since they'll be regenerated when needed
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := cs.queue.PublishLLMResponse(ctx, &LLMResponseWrapper{Response: resp, Opts: nil}); err != nil {
		cs.sendError(fmt.Errorf("could not queue LLM response after tool call: %v", err))
		return false
	}
	return true
}
