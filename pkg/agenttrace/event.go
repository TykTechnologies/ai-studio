package agenttrace

import (
	"encoding/hex"
	"strconv"
	"time"
)

// Kind is the kind of an observed span (spec §3.2). The assembler adds the
// synthetic kinds (agent, inferred_client_tool); the gateway never sends them.
type Kind string

const (
	KindLLMInference   Kind = "llm_inference"   // one logical LLM call, covering all attempts
	KindToolCall       Kind = "tool_call"       // MCP tools/call, resources/read, prompts/get
	KindDatasourceCall Kind = "datasource_call" // REST datasource, OpenAPI-to-MCP upstream
	KindPluginEndpoint Kind = "plugin_endpoint" // /plugins/{slug}/ custom endpoint (opaque body)
	KindGatewayEvent   Kind = "gateway_event"   // a policy outcome with no parent span of its own
)

// Status is a span outcome (spec §3.4 spans.status).
type Status string

const (
	StatusOK    Status = "ok"
	StatusError Status = "error"
)

// Resource types (spec §3.4 spans.resource_type).
const (
	ResourceLLM        = "llm"
	ResourceTool       = "tool"
	ResourceDatasource = "datasource"
	ResourcePlugin     = "plugin"
)

// Content modes in disclosure order (spec §5.5): off < metadata < hashed <
// redacted < full.
const (
	ContentOff      = "off"
	ContentMetadata = "metadata"
	ContentHashed   = "hashed"
	ContentRedacted = "redacted"
	ContentFull     = "full"
)

// Content states of a payload part or span (spec §3.4, §5.5). The UI and
// export distinguish all of them.
const (
	StateNotCaptured = "not_captured"
	StateMetadata    = "metadata"
	StateHashed      = "hashed"
	StateRedacted    = "redacted"
	StateFull        = "full"
	StateTruncated   = "truncated"
	StateDropped     = "dropped" // queue overflow (spec §10 drop policy)
)

// Payload parts (spec §3.4 agent_trace_payloads.part).
const (
	PartRequest            = "request"
	PartResponse           = "response"
	PartSystemInstructions = "system_instructions"
	PartToolDefinitions    = "tool_definitions"
	PartToolArgs           = "tool_args"
	PartToolResult         = "tool_result"
	PartErrorBody          = "error_body"
)

// Gateway event types (spec §3.2 gateway_event, §4.5 retries and fallbacks).
const (
	GatewayEventAttempt   = "attempt"
	GatewayEventFallback  = "fallback"
	GatewayEventGuardrail = "guardrail"
	GatewayEventFilter    = "filter"
	GatewayEventBudget    = "budget"
	GatewayEventRateLimit = "rate_limit"
	GatewayEventAuth      = "auth"
	GatewayEventPolicy    = "policy"
	GatewayEventRouter    = "router"
	GatewayEventOverload  = "overload"
	GatewayEventCache     = "cache"
)

// Gateway event outcomes.
const (
	OutcomeAllow   = "allow"
	OutcomeBlock   = "block"
	OutcomeModify  = "modify"
	OutcomeSkip    = "skip"
	OutcomeError   = "error"
	OutcomeSuccess = "success"
	OutcomeShadow  = "shadow" // a guardrail in shadow mode that would have blocked
)

// SourceEmbedded is Event.Source for the hub's embedded gateway. Edges use
// their edge id.
const SourceEmbedded = "embedded"

// Event is one observed span as the gateway saw it. The gateway fills it on
// the request path; the transports carry it; the assembler resolves it into
// rows. Nothing here is authoritative about trace or session membership:
// those are the assembler's (spec §4.1).
type Event struct {
	// Kind of span (§3.2).
	Kind Kind
	// SpanID is minted by the gateway (crypto/rand) when the recorder begins
	// (§3.4 spans.span_id; also proxy_logs.trace_span_id, request-identity bead).
	SpanID SpanID
	// TraceID is the provisional trace id: the declared one when the client
	// sent traceparent, otherwise a best-effort local choice used for outbound
	// propagation. The assembler may replace it and then links back with
	// provisional_context (§4.1).
	TraceID TraceID
	// ParentSpanID is the declared parent span (traceparent parent, or
	// X-Tyk-Parent-Span-Id) (§4.2 priority 1-2). Zero when none was declared.
	ParentSpanID SpanID
	// Seq orders several events for one span id (0 for the final event;
	// Phase 2 span-started events use their own value). Part of the
	// idempotency key (D3).
	Seq uint32
	// IdempotencyKey is "<source>:<span hex>:<seq>" (D3), set by the
	// transport with SetIdempotencyKey; the staging table is unique on it.
	IdempotencyKey string
	// Source is SourceEmbedded or the edge id (D3), set by the transport.
	Source string

	// Identity of the caller (§3.4 app_id, credential_id, user_id; D4 scope).
	Identity Identity
	// Resource is the Studio catalogue entry served (§3.4 resource_type/id).
	Resource Resource
	// Provider is the gen_ai.provider.name value, e.g. "openai" (§3.4, §5.3).
	Provider string
	// Operation is gen_ai.operation.name ("chat", "embeddings") or
	// mcp.method.name ("tools/call") (§3.4, §5.3).
	Operation string
	// RequestModel and ResponseModel; ResponseModel is the model that answered
	// after any fallback (§4.5).
	RequestModel  string
	ResponseModel string
	// ToolName is the tool for tool_call spans (§3.4 tool_name, §4.4 R2).
	ToolName string

	// StartTime and EndTime are gateway wall-clock times (§3.4). TTFT is the
	// time to first token on streams (§4.5, §5.7), zero when not streamed.
	StartTime time.Time
	EndTime   time.Time
	TTFT      time.Duration
	// Status, ErrorType and ErrorMessage (§4.5 errors). ErrorMessage is
	// redacted per policy before persistence (content pipeline bead).
	Status       Status
	ErrorType    string
	ErrorMessage string
	// HTTPStatus returned to the client.
	HTTPStatus int
	// GatewayNode is the hub node or edge id that served the request.
	GatewayNode string
	// Attempt is the number of upstream attempts the logical call made
	// (1 when there was no retry or fallback; §4.5).
	Attempt uint32
	// AttemptOf is set on the inner hop of the /ai/ loopback (and on each of
	// its failover rungs): the span id of the outer logical call this request
	// is one attempt of. The assembler folds such events into the outer
	// llm_inference span as attempt gateway events (§4.5).
	AttemptOf SpanID

	// Usage and cost, copied from the existing chat-record computation
	// (buildChatRecordFor) and never recomputed (epic deviation 6, §3.4).
	Usage Usage
	// AnalyticsRef links the existing per-request record (proxy_logs id)
	// instead of duplicating it (§3.4 analytics_ref).
	AnalyticsRef string

	// Declared is the client-declared context (§4.2, §4.3), filled by the
	// declared-context bead.
	Declared DeclaredContext
	// Extracts are the correlation fingerprints and ids (§4.4 R1-R3, §4.5).
	// They are computed in every content mode, including off (§4.1).
	Extracts Extracts
	// GatewayEvents are policy outcomes, attempts and fallbacks on this span
	// (§3.2 gateway_event, §4.5). The assembler renders each as a child.
	GatewayEvents []GatewayEvent
	// gatewayEventsDropped counts the gateway events FromProto dropped as
	// out of bounds (GatewayEventsDropped).
	gatewayEventsDropped int
	// Parts are the captured payloads, already through the content pipeline
	// (§3.4 payloads, §5.5). Empty in off mode.
	Parts []PayloadPart
	// Raw is content captured on the request path, before the content
	// pipeline (CaptureRaw). It is never serialised: ProcessContent turns it
	// into Parts in the capture worker, and the converters ignore it.
	Raw []RawPart
	// contentProcessed is set once the content pipeline ran (ProcessContent,
	// or FromProto for an event an edge processed), so it never runs twice.
	contentProcessed bool
	// ContentMode is the effective mode for this span (§5.5).
	ContentMode string
	// Attributes are extra semconv / tyk.* attributes (§3.4 spans.attributes),
	// e.g. tyk.upstream.request_id (§4.6).
	Attributes map[string]string
}

// Identity is who made the call (§3.4; D4: scoped by App, team denormalised).
type Identity struct {
	AppID        uint
	TeamID       uint
	CredentialID string // key or client id, never a secret
	UserID       uint   // Studio user owning the credential
	EndUserID    string // declared end user (X-Tyk-User-Id, baggage user.id), §4.3
	OnBehalfOf   string // authidentity: who the call was for (auth plugin)
	ActingAgent  string // authidentity: the agent that made it (auth plugin)
}

// Resource is the Studio catalogue entry a span used (§3.4).
type Resource struct {
	Type string // ResourceLLM, ResourceTool, ResourceDatasource, ResourcePlugin
	ID   string // catalogue id
	Name string
}

// Usage is token usage and cost (§3.4). CostMicros is cost * 1e6.
type Usage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	CostMicros       int64
	Currency         string
}

// DeclaredContext is what the client declared, after precedence (§4.2) and
// the header set (§4.3). Empty fields were not declared.
type DeclaredContext struct {
	// TraceID and ParentSpanID from traceparent (or X-Tyk-Trace-Id /
	// compat trace headers when traceparent is absent).
	TraceID      TraceID
	ParentSpanID SpanID
	TraceState   string
	Sampled      bool
	// TraceSource names where the trace id came from: "traceparent",
	// "mcp_meta", "x-tyk", or a compat family (§4.3).
	TraceSource string

	// SessionID is the client session id (§3.4 sessions.external_id) and
	// SessionSource the header family that supplied it
	// (sessions.external_source).
	SessionID     string
	SessionSource string
	SessionName   string

	// Agent identity (§4.3 X-Tyk-Agent-*; baggage gen_ai.agent.name).
	AgentName string
	AgentID   string
	AgentPath string
	// ClientSpanID is X-Tyk-Span-Id: the client's id for the agent step.
	ClientSpanID string

	// EndUserID is X-Tyk-User-Id or baggage user.id.
	EndUserID string
	// CaptureMode is X-Tyk-Trace-Capture as sent; the content pipeline
	// decides what it may change (§5.5).
	CaptureMode string
	// ExternalTraceID keeps a non-W3C trace id (Portkey) that was hashed to
	// W3C length (§4.3, tyk.external.trace_id).
	ExternalTraceID string
	// ConversationID is gen_ai.conversation.id, client-supplied only (Q3).
	ConversationID string
	// MCPSessionID is Mcp-Session-Id: a weak session hint only (§4.3).
	MCPSessionID string
	// ClientInfo is the MCP clientInfo discriminator (§4.2 priority 4).
	ClientInfo string
	// Baggage holds the allowlisted W3C baggage keys (§4.6).
	Baggage map[string]string

	// Coding-harness declarations (bead jyx.52), client-supplied like the
	// rest: Harness names the harness recognised (claude_code, codex_cli,
	// opencode); TurnID is a turn id it declares (Codex CLI), the same for
	// every request of one user turn; SubAgentID is a sub-agent instance it
	// declares (Claude Code), empty on the main agent's requests.
	Harness    string
	TurnID     string
	SubAgentID string
}

// Extracts are the correlation material for one span (§4.4). Ids appear in
// clear (they are not content); arguments and results only as fingerprints.
type Extracts struct {
	// ToolCalls the model requested in this response (R1), in order. A
	// streaming overflow keeps ids and names and sets ArgsUnavailable (§4.5).
	ToolCalls []ToolCallRequest
	// ToolResults are the trailing tool results in this request, after the
	// last assistant message (R1); earlier ones are history.
	ToolResults []ToolResult
	// ResponseID and PreviousResponseID for the Responses API chain (R3).
	ResponseID         string
	PreviousResponseID string
	// FinishReasons as the provider reported them (§3.1 turn boundary).
	FinishReasons []string
	// OfferedTools are the tool names the request offered the model (its
	// tool definitions): the span's "tools available" (jyx.51).
	OfferedTools []string

	// For tool_call / datasource_call spans (R2): the canonical argument and
	// result fingerprints of the executed call, the tool call id when the
	// request carried one (gen_ai.tool.call.id), and MCP isError.
	ArgsFP     Fingerprint
	ResultFP   Fingerprint
	ToolCallID string
	ToolError  bool

	// Stream is true for streamed responses; StreamIncomplete when the stream
	// ended without a terminal event (§4.5 streaming).
	Stream           bool
	StreamIncomplete bool
}

// ToolCallRequest is one tool call in an LLM response (R1).
type ToolCallRequest struct {
	ID              string
	Name            string
	ArgsFP          Fingerprint
	ArgsUnavailable bool
}

// ToolResult is one trailing tool result in an LLM request (R1). A result
// sent back without its call's id (Gemini makes it optional, Ollama has
// none, some clients drop it) carries instead the call's Name and
// CallArgsFP, the fingerprint of the call's arguments as the request's
// history repeats them, for the name and arguments fallback.
type ToolResult struct {
	ID         string
	Name       string
	CallArgsFP Fingerprint
	ResultFP   Fingerprint
	IsError    bool
}

// GatewayEvent is one policy outcome, attempt or fallback (§3.2, §4.5).
// Detail is a short, already-redacted string (e.g. "attempt=2 status=503"
// or a guardrail name), never content.
type GatewayEvent struct {
	Type    string
	Outcome string
	Name    string // the filter, guardrail, budget or model involved
	Detail  string
	At      time.Time
}

// PayloadPart is one captured payload (§3.4 payloads, §5.5). Data holds the
// content after the content pipeline (stripped, redacted, truncated); the
// hub encrypts it at rest. Bytes is the original size before truncation.
type PayloadPart struct {
	Part           string
	ContentState   string
	MediaType      string
	Bytes          int64
	Truncated      bool
	RedactionCount int32
	Fingerprint    Fingerprint
	Data           []byte
}

// SetIdempotencyKey stamps Source and the "<source>:<span hex>:<seq>"
// idempotency key (D3). Transports call it before enqueueing.
func (e *Event) SetIdempotencyKey(source string) {
	e.Source = source
	buf := make([]byte, 0, len(source)+1+16+1+10)
	buf = append(buf, source...)
	buf = append(buf, ':')
	buf = hex.AppendEncode(buf, e.SpanID[:])
	buf = append(buf, ':')
	buf = strconv.AppendUint(buf, uint64(e.Seq), 10)
	e.IdempotencyKey = string(buf)
}

// ApproxSize estimates the event's encoded size in bytes, for the queue's
// byte cap (§10 memory budget). It counts payload data and variable strings.
func (e *Event) ApproxSize() int {
	n := 256 + len(e.ErrorMessage) + len(e.RequestModel) + len(e.ResponseModel) +
		len(e.ToolName) + len(e.Declared.SessionID) + len(e.Declared.AgentPath)
	for i := range e.Parts {
		n += len(e.Parts[i].Data) + 64
	}
	for i := range e.Raw {
		n += len(e.Raw[i].Data) + 64
	}
	for i := range e.Extracts.ToolCalls {
		n += len(e.Extracts.ToolCalls[i].ID) + len(e.Extracts.ToolCalls[i].Name) + 40
	}
	for i := range e.Extracts.ToolResults {
		n += len(e.Extracts.ToolResults[i].ID) + len(e.Extracts.ToolResults[i].Name) + 40
	}
	for i := range e.GatewayEvents {
		n += len(e.GatewayEvents[i].Detail) + len(e.GatewayEvents[i].Name) + 32
	}
	for k, v := range e.Attributes {
		n += len(k) + len(v)
	}
	return n
}
