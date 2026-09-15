package chat_session

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/uuid"
)

// OutputMode selects how a session publishes its output on the queue.
//
// OutputModeRaw is the v1 behaviour: text deltas are published as raw bytes on
// the stream channel, status lines as ":::system ...:::" messages and errors on
// the error channel. OutputModeEvents (v2) publishes every observable thing as a
// ChatEvent envelope on the stream channel, tagged with the run it belongs to,
// so a per-turn reader can follow exactly one turn.
//
// A session's mode is fixed before Start() and a reader of the other mode must
// never be attached: the v1 SSE reader silently drops envelope JSON, and the v2
// reader cannot interpret raw chunks.
type OutputMode int

const (
	OutputModeRaw OutputMode = iota
	OutputModeEvents
)

// Event kinds. Where the AI SDK "UI message stream" has a native chunk type the
// same name is used so the browser mapping is mechanical; Studio-specific
// payloads use the "data-" prefix, which that protocol reserves for custom parts.
const (
	EventStart          = "start"
	EventTextDelta      = "text-delta"
	EventTextEnd        = "text-end"
	EventReasoningDelta = "reasoning-delta"
	EventToolCallStart  = "tool-call-start"
	EventToolCallDelta  = "tool-call-delta"
	EventToolCallEnd    = "tool-call-end"
	EventToolResult     = "tool-result"
	EventStatus         = "data-status"
	EventContext        = "data-context"
	EventError          = "data-error"
	EventFinish         = "finish"
)

// Finish reasons.
const (
	FinishStop      = "stop"
	FinishToolCalls = "tool-calls" // the turn is waiting on client-side tool results
	FinishError     = "error"
	FinishCancelled = "cancelled"
	FinishFiltered  = "filtered"
)

// Error codes carried in EventError payloads. They replace the string sniffing
// the v1 frontend did on error text.
const (
	ErrCodeLLMConfig  = "llm_config"
	ErrCodeAPI        = "api"
	ErrCodeConnection = "connection"
	ErrCodeAuth       = "auth"
	ErrCodeFilter     = "filter"
	ErrCodeSession    = "session"
	ErrCodeTool       = "tool"
	ErrCodeInternal   = "internal"
)

// Context sources carried in EventContext payloads.
const (
	ContextSourceRAG      = "rag"
	ContextSourceToolDocs = "tool_docs"
	ContextSourceFile     = "file"
)

const eventEnvelopeVersion = "aui/v1"

// ChatEvent is the envelope published on the stream channel in OutputModeEvents.
type ChatEvent struct {
	Version string          `json:"v"`
	RunID   string          `json:"run_id"`
	Seq     uint64          `json:"seq"`
	Kind    string          `json:"kind"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Event payloads.
type StartData struct {
	RunID string `json:"run_id"`
}

type TextDeltaData struct {
	Delta string `json:"delta"`
}

type ToolCallStartData struct {
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
}

type ToolCallDeltaData struct {
	ToolCallID string `json:"toolCallId"`
	ArgsText   string `json:"argsText"`
}

type ToolCallEndData struct {
	ToolCallID string `json:"toolCallId"`
}

type ToolResultData struct {
	ToolCallID string `json:"toolCallId"`
	Result     string `json:"result"`
	IsError    bool   `json:"isError,omitempty"`
	Bytes      int    `json:"bytes,omitempty"`
}

type StatusData struct {
	Text  string `json:"text"`
	Level string `json:"level,omitempty"`
}

type ContextData struct {
	Text   string `json:"text"`
	Source string `json:"source"`
}

type ErrorData struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Detail    string `json:"detail,omitempty"`
	Retryable bool   `json:"retryable"`
}

type FinishData struct {
	Reason string `json:"reason"`
	// Database ids of the rows this turn produced, so a client can address
	// them later (edit truncation, regenerate) without reloading history.
	UserMessageID      uint `json:"user_message_id,omitempty"`
	AssistantMessageID uint `json:"assistant_message_id,omitempty"`
}

// DecodeChatEvent reports whether b is an event envelope and decodes it.
func DecodeChatEvent(b []byte) (*ChatEvent, bool) {
	if len(b) == 0 || b[0] != '{' {
		return nil, false
	}
	var ev ChatEvent
	if err := json.Unmarshal(b, &ev); err != nil {
		return nil, false
	}
	if ev.Version != eventEnvelopeVersion || ev.Kind == "" {
		return nil, false
	}
	return &ev, true
}

// NewRunID returns a fresh identifier for one user turn.
func NewRunID() string {
	uid, _ := uuid.NewV4()
	return uid.String()
}

// ClassifyError maps an error to one of the ErrCode* constants. The heuristics
// mirror what the v1 browser code did (processErrorMessage / detectErrorType)
// so the two UIs categorise the same failures the same way.
func ClassifyError(err error) string {
	if err == nil {
		return ErrCodeInternal
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "blocked by filter"), strings.Contains(s, "response blocked"),
		strings.Contains(s, "content guideline"), strings.Contains(s, "blocked by policy"):
		return ErrCodeFilter
	case strings.Contains(s, "unauthorized"), strings.Contains(s, "authentication failed"):
		return ErrCodeAuth
	case strings.Contains(s, "api returned unexpected status code"), strings.Contains(s, "status code:"):
		return ErrCodeAPI
	case strings.Contains(s, "connection"), strings.Contains(s, "network"), strings.Contains(s, "timeout"),
		strings.Contains(s, "context deadline exceeded"):
		return ErrCodeConnection
	case strings.Contains(s, "tool"):
		return ErrCodeTool
	case strings.Contains(s, "failed to create message"), strings.Contains(s, "anthropic"),
		strings.Contains(s, "openai"), strings.Contains(s, "llm"), strings.Contains(s, "model"):
		return ErrCodeLLMConfig
	default:
		return ErrCodeInternal
	}
}

// runState tracks the turn currently being processed by the session goroutine.
type runState struct {
	id       string
	cancel   context.CancelFunc
	ctx      context.Context
	streamed bool // at least one text delta was emitted for the current LLM reply
	seq      uint64

	userMsgID      uint // row id of the user message of this turn (0 for regenerate)
	assistantMsgID uint // row id of the first assistant row of this turn
}

// noteUserMessageID records the row id of the turn's user message.
func (cs *ChatSession) noteUserMessageID(id uint) {
	cs.stateMu.Lock()
	if cs.activeRun != nil {
		cs.activeRun.userMsgID = id
	}
	cs.stateMu.Unlock()
}

// noteAssistantMessageID records the first assistant row of the turn; later
// rows of the same turn (tool calls, follow-up text) fold into it in history.
func (cs *ChatSession) noteAssistantMessageID(id uint) {
	cs.stateMu.Lock()
	if cs.activeRun != nil && cs.activeRun.assistantMsgID == 0 {
		cs.activeRun.assistantMsgID = id
	}
	cs.stateMu.Unlock()
}

// eventSubscriber receives the envelopes of one run.
type eventSubscriber struct {
	ch chan ChatEvent
}

// SetOutputMode fixes the output mode. It must be called before Start().
func (cs *ChatSession) SetOutputMode(m OutputMode) {
	cs.outputMode = m
}

// OutputMode returns the session's output mode.
func (cs *ChatSession) OutputMode() OutputMode {
	return cs.outputMode
}

// TryLockRun claims the session for one turn. It returns false when another
// turn is in flight; the caller must UnlockRun when its turn has finished.
func (cs *ChatSession) TryLockRun() bool {
	return cs.runMu.TryLock()
}

// UnlockRun releases the claim taken by TryLockRun.
func (cs *ChatSession) UnlockRun() {
	cs.runMu.Unlock()
}

// ActiveRunID returns the id of the turn being processed, or "".
func (cs *ChatSession) ActiveRunID() string {
	cs.stateMu.Lock()
	defer cs.stateMu.Unlock()
	if cs.activeRun == nil {
		return ""
	}
	return cs.activeRun.id
}

// CancelRun aborts the in-flight LLM call of the active turn, if any. The turn
// still finishes normally (with FinishCancelled) once the call returns.
func (cs *ChatSession) CancelRun() bool {
	cs.stateMu.Lock()
	defer cs.stateMu.Unlock()
	if cs.activeRun == nil {
		return false
	}
	cs.activeRun.cancel()
	return true
}

// Subscribe returns a channel that receives the envelopes of runID until
// unsubscribe is called. Only meaningful in OutputModeEvents. Events are
// dropped (with a warning) if the subscriber falls more than buf behind.
func (cs *ChatSession) Subscribe(runID string, buf int) (<-chan ChatEvent, func()) {
	if buf <= 0 {
		buf = 256
	}
	sub := &eventSubscriber{ch: make(chan ChatEvent, buf)}
	cs.subMu.Lock()
	if cs.subs == nil {
		cs.subs = map[string]*eventSubscriber{}
	}
	cs.subs[runID] = sub
	cs.subMu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			cs.subMu.Lock()
			if cur, ok := cs.subs[runID]; ok && cur == sub {
				delete(cs.subs, runID)
			}
			cs.subMu.Unlock()
		})
	}
	return sub.ch, unsubscribe
}

type muteStreamKey struct{}

// mutedContext marks a request whose streamed chunks streamingFunc must
// drop. Side calls to the model (title generation) reuse the driver, and
// drivers stream through the driver-level callback regardless of call
// options; without this their chunks would leak into the user's reply.
//
// The mark travels with the request context rather than a session-wide
// flag because side calls run concurrently with the user's turn: a
// session-wide mute swallowed the opening of the next reply while the
// title was being generated.
func mutedContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, muteStreamKey{}, true)
}

// streamingMuted reports whether chunks for this request must be dropped.
func streamingMuted(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	muted, _ := ctx.Value(muteStreamKey{}).(bool)
	return muted
}

// runCtx is the context LLM calls should use: the active run's context when a
// turn is in flight (cancellable per run), otherwise the session context.
func (cs *ChatSession) runCtx() context.Context {
	cs.stateMu.Lock()
	defer cs.stateMu.Unlock()
	if cs.activeRun != nil {
		return cs.activeRun.ctx
	}
	return cs.ctx
}

// beginRun marks the start of a turn and emits EventStart.
func (cs *ChatSession) beginRun(runID string) {
	if runID == "" {
		runID = NewRunID()
	}
	ctx, cancel := context.WithCancel(cs.ctx)
	cs.stateMu.Lock()
	cs.activeRun = &runState{id: runID, ctx: ctx, cancel: cancel}
	cs.stateMu.Unlock()
	cs.emit(EventStart, StartData{RunID: runID})
}

// finishRun emits EventFinish for the active turn and clears it. It is safe to
// call when no turn is active.
func (cs *ChatSession) finishRun(reason string) {
	cs.stateMu.Lock()
	run := cs.activeRun
	cs.stateMu.Unlock()
	if run == nil {
		return
	}
	if reason == "" {
		reason = FinishStop
	}
	if run.ctx.Err() != nil && reason != FinishToolCalls {
		reason = FinishCancelled
	}
	cs.emit(EventFinish, FinishData{Reason: reason, UserMessageID: run.userMsgID, AssistantMessageID: run.assistantMsgID})
	cs.stateMu.Lock()
	if cs.activeRun == run {
		cs.activeRun = nil
	}
	cs.stateMu.Unlock()
	run.cancel()
}

// markStreamed records that text deltas have been emitted for the current reply.
func (cs *ChatSession) markStreamed() {
	cs.stateMu.Lock()
	if cs.activeRun != nil {
		cs.activeRun.streamed = true
	}
	cs.stateMu.Unlock()
}

// resetStreamed clears the streamed flag before a new LLM call within a turn.
func (cs *ChatSession) resetStreamed() {
	cs.stateMu.Lock()
	if cs.activeRun != nil {
		cs.activeRun.streamed = false
	}
	cs.stateMu.Unlock()
}

func (cs *ChatSession) hasStreamed() bool {
	cs.stateMu.Lock()
	defer cs.stateMu.Unlock()
	return cs.activeRun != nil && cs.activeRun.streamed
}

// emit publishes an event envelope for the active run. It is a no-op in
// OutputModeRaw so the v1 paths pay nothing.
func (cs *ChatSession) emit(kind string, data any) {
	if cs.outputMode != OutputModeEvents {
		return
	}
	var raw json.RawMessage
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			slog.Error("chat event marshal failed", "kind", kind, "error", err)
			return
		}
		raw = b
	}
	cs.stateMu.Lock()
	runID := ""
	var seq uint64
	if cs.activeRun != nil {
		runID = cs.activeRun.id
		cs.activeRun.seq++
		seq = cs.activeRun.seq
	}
	cs.stateMu.Unlock()

	ev := ChatEvent{Version: eventEnvelopeVersion, RunID: runID, Seq: seq, Kind: kind, Data: raw}
	b, err := json.Marshal(ev)
	if err != nil {
		slog.Error("chat event envelope marshal failed", "kind", kind, "error", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := cs.queue.PublishStream(ctx, b); err != nil {
		slog.Warn("chat event publish failed", "kind", kind, "run_id", runID, "error", err)
	}
}

// startEventFanout runs one reader over the queue's stream and error channels
// and dispatches envelopes to the subscriber of their run. It is started by
// Start() in OutputModeEvents and exits when the queue closes.
func (cs *ChatSession) startEventFanout() {
	stream := cs.queue.ConsumeStream(cs.ctx)
	errs := cs.queue.ConsumeErrors(cs.ctx)
	go func() {
		for {
			select {
			case <-cs.stop:
				return
			case b, ok := <-stream:
				if !ok {
					return
				}
				ev, isEvent := DecodeChatEvent(b)
				if !isEvent {
					// A raw chunk on an events-mode session is a programming
					// error somewhere; surface it as a delta rather than lose it.
					ev = &ChatEvent{Version: eventEnvelopeVersion, RunID: cs.ActiveRunID(), Kind: EventTextDelta}
					ev.Data, _ = json.Marshal(TextDeltaData{Delta: string(b)})
				}
				cs.dispatch(*ev)
			case err, ok := <-errs:
				if !ok {
					return
				}
				// Errors published outside emit() (e.g. queue-level) become error events.
				data, _ := json.Marshal(ErrorData{Code: ClassifyError(err), Message: err.Error()})
				cs.dispatch(ChatEvent{Version: eventEnvelopeVersion, RunID: cs.ActiveRunID(), Kind: EventError, Data: data})
			}
		}
	}()
}

func (cs *ChatSession) dispatch(ev ChatEvent) {
	cs.subMu.Lock()
	sub, ok := cs.subs[ev.RunID]
	cs.subMu.Unlock()
	if !ok {
		slog.Debug("chat event without subscriber dropped", "kind", ev.Kind, "run_id", ev.RunID, "session_id", cs.id)
		return
	}
	select {
	case sub.ch <- ev:
	default:
		slog.Warn("chat event subscriber overflow, event dropped", "kind", ev.Kind, "run_id", ev.RunID, "session_id", cs.id)
	}
}
