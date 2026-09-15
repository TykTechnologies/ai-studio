package agent_session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// Chunk types as the plugin proto names them (AgentMessageChunk_ChunkType.String()).
const (
	ChunkContent    = "CONTENT"
	ChunkToolCall   = "TOOL_CALL"
	ChunkToolResult = "TOOL_RESULT"
	ChunkThinking   = "THINKING"
	ChunkError      = "ERROR"
	ChunkDone       = "DONE"
)

// MetadataToolCallID is the metadata key the session adds to TOOL_CALL and
// TOOL_RESULT chunks so consumers can pair a result with its call.
const MetadataToolCallID = "tool_call_id"

// TranscriptPart is one part of an in-memory transcript message. It mirrors
// the parts the v2 chat history exposes so the API can convert it directly.
type TranscriptPart struct {
	Type       string         `json:"type"` // text | reasoning | tool-call | data
	Text       string         `json:"text,omitempty"`
	ToolCallID string         `json:"toolCallId,omitempty"`
	ToolName   string         `json:"toolName,omitempty"`
	Args       map[string]any `json:"args,omitempty"`
	ArgsText   string         `json:"argsText,omitempty"`
	Result     any            `json:"result,omitempty"`
	HasResult  bool           `json:"-"`
	IsError    bool           `json:"isError,omitempty"`
	Name       string         `json:"name,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
}

// TranscriptMessage is one turn of the session's in-memory transcript.
type TranscriptMessage struct {
	ID        string           `json:"id"`
	Role      string           `json:"role"` // user | assistant
	Parts     []TranscriptPart `json:"parts"`
	CreatedAt time.Time        `json:"created_at"`
}

// runState is the turn currently streaming from the plugin.
type runState struct {
	cancel context.CancelFunc
	done   chan error
}

// NewAgentSessionWithID is NewAgentSession with a caller-chosen id, so the
// queue and the session can share one identifier.
func NewAgentSessionWithID(
	id string,
	agentConfig *models.AgentConfig,
	pluginClient pb.PluginServiceClient,
	serviceBrokerID uint32,
	queue MessageQueue,
	db *gorm.DB,
) (*AgentSession, error) {
	as, err := NewAgentSession(agentConfig, pluginClient, serviceBrokerID, queue, db)
	if err != nil {
		return nil, err
	}
	if id != "" {
		as.id = id
	}
	return as, nil
}

// NewSessionID returns a fresh agent session id.
func NewSessionID() string {
	uid, _ := uuid.NewV4()
	return uid.String()
}

// ID returns the session id (HubSession).
func (as *AgentSession) ID() string { return as.id }

// Stop closes the session (HubSession). Idempotent.
func (as *AgentSession) Stop() {
	as.closeOnce.Do(func() {
		if err := as.Close(); err != nil {
			slog.Debug("agent session close", "session_id", as.id, "error", err)
		}
	})
}

// AgentConfig returns the configuration the session was created for.
func (as *AgentSession) AgentConfig() *models.AgentConfig { return as.agentConfig }

// SetOwnerID records which user created the session; OwnerID reads it.
func (as *AgentSession) SetOwnerID(id uint) {
	as.mu.Lock()
	as.ownerID = id
	as.mu.Unlock()
}

// OwnerID returns the user id the session was created for (0 if unset).
func (as *AgentSession) OwnerID() uint {
	as.mu.Lock()
	defer as.mu.Unlock()
	return as.ownerID
}

// TryLockRun claims the session for one turn; UnlockRun releases it.
func (as *AgentSession) TryLockRun() bool { return as.runMu.TryLock() }

// UnlockRun releases the claim taken by TryLockRun.
func (as *AgentSession) UnlockRun() { as.runMu.Unlock() }

// CancelRun aborts the plugin stream of the active turn, if any.
func (as *AgentSession) CancelRun() bool {
	as.mu.Lock()
	defer as.mu.Unlock()
	if as.activeRun == nil {
		return false
	}
	as.activeRun.cancel()
	return true
}

// Transcript returns a copy of the in-memory conversation.
func (as *AgentSession) Transcript() []TranscriptMessage {
	as.mu.Lock()
	defer as.mu.Unlock()
	out := make([]TranscriptMessage, 0, len(as.transcript)+1)
	for _, m := range as.transcript {
		out = append(out, cloneMessage(m))
	}
	if as.current != nil {
		out = append(out, cloneMessage(*as.current))
	}
	return out
}

func cloneMessage(m TranscriptMessage) TranscriptMessage {
	c := m
	c.Parts = append([]TranscriptPart(nil), m.Parts...)
	return c
}

// StartRun sends one user message to the plugin and streams the reply into
// the queue. The returned channel receives the stream's terminal error (nil on
// a clean end) exactly once. The history handed to the plugin is the
// transcript before this message.
func (as *AgentSession) StartRun(userMessage string) (<-chan error, error) {
	as.mu.Lock()
	history := as.historyForPlugin()
	as.nextID++
	as.transcript = append(as.transcript, TranscriptMessage{
		ID:        fmt.Sprintf("u%d", as.nextID),
		Role:      "user",
		Parts:     []TranscriptPart{{Type: "text", Text: userMessage}},
		CreatedAt: time.Now(),
	})
	as.nextID++
	as.current = &TranscriptMessage{ID: fmt.Sprintf("a%d", as.nextID), Role: "assistant", CreatedAt: time.Now()}
	as.mu.Unlock()

	req, err := as.buildAgentRequest(userMessage, history)
	if err != nil {
		as.discardCurrent()
		return nil, fmt.Errorf("failed to build agent request: %w", err)
	}

	runCtx, cancel := context.WithCancel(as.ctx)
	stream, err := as.pluginClient.HandleAgentMessage(runCtx, req)
	if err != nil {
		cancel()
		as.discardCurrent()
		return nil, fmt.Errorf("failed to call plugin HandleAgentMessage: %w", err)
	}

	done := make(chan error, 1)
	as.mu.Lock()
	as.activeRun = &runState{cancel: cancel, done: done}
	as.mu.Unlock()

	go func() {
		err := as.receiveChunks(stream)
		as.finishRun()
		cancel()
		done <- err
		close(done)
	}()
	return done, nil
}

func (as *AgentSession) discardCurrent() {
	as.mu.Lock()
	as.current = nil
	as.mu.Unlock()
}

// finishRun moves the streaming assistant message into the transcript.
func (as *AgentSession) finishRun() {
	as.mu.Lock()
	defer as.mu.Unlock()
	if as.current != nil {
		as.transcript = append(as.transcript, *as.current)
		as.current = nil
	}
	as.activeRun = nil
}

// historyForPlugin flattens the transcript into the role/content pairs the
// plugin proto carries. Caller holds as.mu.
func (as *AgentSession) historyForPlugin() []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(as.transcript))
	for _, m := range as.transcript {
		var sb strings.Builder
		for _, p := range m.Parts {
			if p.Type == "text" {
				sb.WriteString(p.Text)
			}
		}
		if sb.Len() == 0 {
			continue
		}
		out = append(out, map[string]interface{}{"role": m.Role, "content": sb.String()})
	}
	return out
}

// recordChunk folds one plugin chunk into the streaming assistant message and
// returns the chunk enriched with the metadata consumers need (tool call ids).
func (as *AgentSession) recordChunk(chunk *AgentMessageChunk) *AgentMessageChunk {
	as.mu.Lock()
	defer as.mu.Unlock()
	if as.current == nil {
		return chunk
	}
	if chunk.Metadata == nil {
		chunk.Metadata = map[string]interface{}{}
	}
	cur := as.current
	switch strings.ToUpper(chunk.Type) {
	case ChunkContent:
		if n := len(cur.Parts); n > 0 && cur.Parts[n-1].Type == "text" {
			cur.Parts[n-1].Text += chunk.Content
		} else if chunk.Content != "" {
			cur.Parts = append(cur.Parts, TranscriptPart{Type: "text", Text: chunk.Content})
		}
	case ChunkThinking:
		if n := len(cur.Parts); n > 0 && cur.Parts[n-1].Type == "reasoning" {
			cur.Parts[n-1].Text += chunk.Content
		} else if chunk.Content != "" {
			cur.Parts = append(cur.Parts, TranscriptPart{Type: "reasoning", Text: chunk.Content})
		}
	case ChunkToolCall:
		as.nextID++
		id := fmt.Sprintf("call_%d", as.nextID)
		name, _ := chunk.Metadata["tool_name"].(string)
		if name == "" {
			name = "tool"
		}
		args, _ := chunk.Metadata["parameters"].(map[string]interface{})
		argsText := ""
		if args != nil {
			if b, err := json.Marshal(args); err == nil {
				argsText = string(b)
			}
		} else if chunk.Content != "" {
			argsText = chunk.Content
		}
		chunk.Metadata[MetadataToolCallID] = id
		cur.Parts = append(cur.Parts, TranscriptPart{Type: "tool-call", ToolCallID: id, ToolName: name, Args: args, ArgsText: argsText})
	case ChunkToolResult:
		name, _ := chunk.Metadata["tool_name"].(string)
		idx := -1
		for i := len(cur.Parts) - 1; i >= 0; i-- {
			p := cur.Parts[i]
			if p.Type == "tool-call" && !p.HasResult && (name == "" || p.ToolName == name) {
				idx = i
				break
			}
		}
		var result any = chunk.Content
		var parsed any
		if err := json.Unmarshal([]byte(chunk.Content), &parsed); err == nil {
			if _, isObj := parsed.(map[string]any); isObj {
				result = parsed
			} else if _, isArr := parsed.([]any); isArr {
				result = parsed
			}
		}
		isErr, _ := chunk.Metadata["is_error"].(bool)
		if idx >= 0 {
			cur.Parts[idx].Result = result
			cur.Parts[idx].HasResult = true
			cur.Parts[idx].IsError = isErr
			chunk.Metadata[MetadataToolCallID] = cur.Parts[idx].ToolCallID
		} else {
			as.nextID++
			id := fmt.Sprintf("call_%d", as.nextID)
			if name == "" {
				name = "tool"
			}
			chunk.Metadata[MetadataToolCallID] = id
			cur.Parts = append(cur.Parts, TranscriptPart{Type: "tool-call", ToolCallID: id, ToolName: name, Args: map[string]any{}, Result: result, HasResult: true, IsError: isErr})
		}
	case ChunkError:
		cur.Parts = append(cur.Parts, TranscriptPart{Type: "data", Name: "error", Data: map[string]any{"code": "internal", "message": chunk.Content}})
	}
	return chunk
}
