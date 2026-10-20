package models

import "time"

// Agent Call Tracing tables (Enterprise feature; spec
// enterprise/features/agenttracing/SPEC.md §3.4 as amended by decisions D1
// and D4). The tables live in core so the schema goldens cover them, but only
// the Enterprise assembler writes them: in Community Edition they stay empty.
//
// Portability (D1): plain tables that work on SQLite and Postgres. No
// partitioning, jsonb or GIN; list filters use the small join tables below
// and B-tree indexes, other lists are JSON in text columns. Retention is a
// chunked delete (DeleteAgentTraceRowsBefore).
//
// Scope (D4): there is no tenant entity, so every row is scoped by app_id;
// team_id is denormalised onto sessions for filtering.
//
// Ids: trace ids are 16 bytes and span ids 8 bytes, stored binary (bytea on
// Postgres, blob on SQLite) and rendered as lowercase hex by the API. Session
// and agent ids are UUIDv7 strings. Fingerprint columns hold an HMAC-SHA-256
// prefixed by the 1-byte D4 key id: fp[0] is the key id, fp[1:] the MAC.
//
// None of these use gorm.Model: there is no soft delete (a tombstone would
// block re-creation under the unique indexes, and retention deletes rows).
// No bool has a default:true tag (gorm would turn an explicit false into
// true); the writer always sets them.

// Session status values.
const (
	AgentTraceSessionActive = "active"
	AgentTraceSessionClosed = "closed"
	AgentTraceSessionError  = "error"
)

// AgentTraceSession is one agent conversation or job (declared or inferred).
// The counters and token/cost totals are rollups the assembler upserts with
// UpsertAgentTraceSessionRollup.
type AgentTraceSession struct {
	ID     string `gorm:"primaryKey;size:36" json:"id"` // UUIDv7, Tyk-minted
	AppID  uint   `gorm:"not null;index:idx_agent_trace_sessions_app_started,priority:1;index:idx_agent_trace_sessions_app_user_started,priority:1;index:idx_agent_trace_sessions_app_external,priority:1;index:idx_agent_trace_sessions_app_cost,priority:1" json:"app_id"`
	TeamID *uint  `gorm:"index:idx_agent_trace_sessions_team_started,priority:1" json:"team_id,omitempty"`
	// UserID is the Studio user the credential belongs to (as on proxy_logs);
	// EndUser is the end user the client declared (header or IdP subject).
	UserID         uint   `gorm:"index:idx_agent_trace_sessions_app_user_started,priority:2" json:"user_id"`
	EndUser        string `gorm:"size:255" json:"end_user,omitempty"`
	ExternalID     string `gorm:"size:255;index:idx_agent_trace_sessions_app_external,priority:2" json:"external_id,omitempty"` // client-supplied session id
	ExternalSource string `gorm:"size:64" json:"external_source,omitempty"`                                                     // x-tyk-session-id, helicone, ...
	Source         string `gorm:"size:16;not null" json:"source"`                                                               // declared | inferred
	Name           string `gorm:"size:255" json:"name,omitempty"`
	Status         string `gorm:"size:16;not null;index:idx_agent_trace_sessions_status_activity,priority:1" json:"status"`

	StartedAt      time.Time  `gorm:"not null;index:idx_agent_trace_sessions_app_started,priority:2;index:idx_agent_trace_sessions_app_user_started,priority:3;index:idx_agent_trace_sessions_team_started,priority:2" json:"started_at"`
	LastActivityAt time.Time  `gorm:"not null;index:idx_agent_trace_sessions_status_activity,priority:2" json:"last_activity_at"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`

	TurnCount     int    `gorm:"not null;default:0" json:"turn_count"`
	SpanCount     int    `gorm:"not null;default:0" json:"span_count"`
	LLMCallCount  int    `gorm:"column:llm_call_count;not null;default:0" json:"llm_call_count"`
	ToolCallCount int    `gorm:"not null;default:0" json:"tool_call_count"`
	ErrorCount    int    `gorm:"not null;default:0" json:"error_count"`
	DroppedCount  int    `gorm:"not null;default:0" json:"dropped_count"` // captures dropped under load; the UI shows a banner
	InputTokens   int64  `gorm:"not null;default:0" json:"input_tokens"`
	OutputTokens  int64  `gorm:"not null;default:0" json:"output_tokens"`
	CostMicros    int64  `gorm:"not null;default:0;index:idx_agent_trace_sessions_app_cost,priority:2" json:"cost_micros"` // cost * 1e6 in Currency
	Currency      string `gorm:"size:3" json:"currency,omitempty"`

	MinConfidence  string    `gorm:"size:16" json:"min_confidence,omitempty"` // weakest link used in this session
	Sampled        bool      `gorm:"not null" json:"sampled"`
	ContentMode    string    `gorm:"size:16;not null" json:"content_mode"` // effective max mode in the session
	RetentionUntil time.Time `gorm:"not null;index:idx_agent_trace_sessions_retention" json:"retention_until"`
}

func (AgentTraceSession) TableName() string { return "agent_trace_sessions" }

// AgentTraceSessionModel, AgentTraceSessionTool and AgentTraceSessionAgent
// are the list filters of spec §6.1 (model, tool and agent names) as join
// tables, in place of Postgres arrays with GIN indexes. The primary key
// serves "names in this session"; the (name, session_id) index serves "sessions
// with this name".
type AgentTraceSessionModel struct {
	SessionID string `gorm:"primaryKey;size:36" json:"session_id"`
	Name      string `gorm:"primaryKey;size:255;index:idx_agent_trace_session_models_name,priority:1" json:"name"`
}

func (AgentTraceSessionModel) TableName() string { return "agent_trace_session_models" }

type AgentTraceSessionTool struct {
	SessionID string `gorm:"primaryKey;size:36" json:"session_id"`
	Name      string `gorm:"primaryKey;size:255;index:idx_agent_trace_session_tools_name,priority:1" json:"name"`
}

func (AgentTraceSessionTool) TableName() string { return "agent_trace_session_tools" }

type AgentTraceSessionAgent struct {
	SessionID string `gorm:"primaryKey;size:36" json:"session_id"`
	Name      string `gorm:"primaryKey;size:255;index:idx_agent_trace_session_agents_name,priority:1" json:"name"`
}

func (AgentTraceSessionAgent) TableName() string { return "agent_trace_session_agents" }

// AgentTraceTrace is one turn (or one client-defined trace) of a session.
type AgentTraceTrace struct {
	TraceID    []byte     `gorm:"primaryKey" json:"trace_id"` // 16 bytes
	SessionID  string     `gorm:"size:36;not null;index:idx_agent_trace_traces_session_turn,priority:1" json:"session_id"`
	AppID      uint       `gorm:"not null;index:idx_agent_trace_traces_app_started,priority:1" json:"app_id"`
	TurnIndex  int        `gorm:"not null;default:0;index:idx_agent_trace_traces_session_turn,priority:2" json:"turn_index"`
	Source     string     `gorm:"size:32;not null" json:"source"` // client_traceparent | tyk
	RootSpanID []byte     `json:"root_span_id,omitempty"`
	StartedAt  time.Time  `gorm:"not null;index:idx_agent_trace_traces_app_started,priority:2" json:"started_at"`
	EndedAt    *time.Time `json:"ended_at,omitempty"`
	Status     string     `gorm:"size:16;not null" json:"status"` // in_progress | ok | error | abandoned

	SpanCount     int   `gorm:"not null;default:0" json:"span_count"`
	LLMCallCount  int   `gorm:"column:llm_call_count;not null;default:0" json:"llm_call_count"`
	ToolCallCount int   `gorm:"not null;default:0" json:"tool_call_count"`
	ErrorCount    int   `gorm:"not null;default:0" json:"error_count"`
	InputTokens   int64 `gorm:"not null;default:0" json:"input_tokens"`
	OutputTokens  int64 `gorm:"not null;default:0" json:"output_tokens"`
	CostMicros    int64 `gorm:"not null;default:0" json:"cost_micros"`
}

func (AgentTraceTrace) TableName() string { return "agent_trace_traces" }

// AgentTraceSpan is one span: an LLM inference, tool/MCP call, datasource
// call, gateway event, agent grouping or inferred client tool (spec §3.2).
// Usage and cost are computed once by the existing chat-record computation
// and copied here for rollups; AnalyticsRef points at that per-request record.
type AgentTraceSpan struct {
	SpanID           []byte  `gorm:"primaryKey" json:"span_id"` // 8 bytes
	TraceID          []byte  `gorm:"not null;index:idx_agent_trace_spans_trace_start,priority:1" json:"trace_id"`
	SessionID        string  `gorm:"size:36;not null;index:idx_agent_trace_spans_session_start,priority:1" json:"session_id"`
	AppID            uint    `gorm:"not null;index:idx_agent_trace_spans_app_kind_start,priority:1;index:idx_agent_trace_spans_app_tool_start,priority:1" json:"app_id"`
	ParentSpanID     []byte  `json:"parent_span_id,omitempty"`
	ParentConfidence string  `gorm:"size:16" json:"parent_confidence,omitempty"` // declared | exact | strong | weak
	ParentRule       string  `gorm:"size:64" json:"parent_rule,omitempty"`       // e.g. tool_call_id, mcp_name_args
	AgentRef         *string `gorm:"size:36" json:"agent_ref,omitempty"`         // agent_trace_agents.id
	Kind             string  `gorm:"size:32;not null;index:idx_agent_trace_spans_app_kind_start,priority:2" json:"kind"`
	Name             string  `gorm:"size:255;not null" json:"name"` // OTel span name

	StartTime    time.Time  `gorm:"not null;index:idx_agent_trace_spans_session_start,priority:2;index:idx_agent_trace_spans_trace_start,priority:2;index:idx_agent_trace_spans_app_kind_start,priority:3;index:idx_agent_trace_spans_app_tool_start,priority:3" json:"start_time"`
	EndTime      *time.Time `json:"end_time,omitempty"`
	Status       string     `gorm:"size:16;not null" json:"status"` // in_progress | ok | error
	ErrorType    string     `gorm:"size:128" json:"error_type,omitempty"`
	ErrorMessage string     `gorm:"type:text" json:"error_message,omitempty"` // redacted per policy

	CredentialID  string `gorm:"size:128" json:"credential_id,omitempty"` // key/client id, never a secret
	UserID        uint   `json:"user_id,omitempty"`
	ResourceType  string `gorm:"size:16" json:"resource_type,omitempty"` // llm | tool | datasource | plugin
	ResourceID    string `gorm:"size:64" json:"resource_id,omitempty"`   // Studio catalogue id
	Provider      string `gorm:"size:64" json:"provider,omitempty"`      // gen_ai.provider.name
	Operation     string `gorm:"size:64" json:"operation,omitempty"`     // gen_ai.operation.name / mcp.method.name
	RequestModel  string `gorm:"size:255" json:"request_model,omitempty"`
	ResponseModel string `gorm:"size:255" json:"response_model,omitempty"`
	ToolName      string `gorm:"size:255;index:idx_agent_trace_spans_app_tool_start,priority:2" json:"tool_name,omitempty"`
	// ToolCallIDs are the ids requested (llm) or served (tool); FinishReasons
	// the response's finish reasons. JSON arrays in text columns (D1).
	ToolCallIDs   []string `gorm:"column:tool_call_ids;type:text;serializer:json" json:"tool_call_ids,omitempty"`
	FinishReasons []string `gorm:"type:text;serializer:json" json:"finish_reasons,omitempty"`

	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	CacheReadTokens  int     `json:"cache_read_tokens"`
	CacheWriteTokens int     `json:"cache_write_tokens"`
	ReasoningTokens  int     `json:"reasoning_tokens"`
	CostMicros       int64   `json:"cost_micros"`
	TTFTMs           *int    `gorm:"column:ttft_ms" json:"ttft_ms,omitempty"`
	HTTPStatus       int     `gorm:"column:http_status" json:"http_status,omitempty"`
	GatewayNode      string  `gorm:"size:128" json:"gateway_node,omitempty"`
	AnalyticsRef     string  `gorm:"size:64" json:"analytics_ref,omitempty"` // per-request analytics record (proxy_logs id)
	Inferred         bool    `gorm:"not null" json:"inferred"`
	ContentState     string  `gorm:"size:16;not null" json:"content_state"` // not_captured | metadata | hashed | redacted | full | truncated | dropped
	Attributes       RawJSON `gorm:"type:text" json:"attributes,omitempty"` // extra semconv / tyk.* attributes
}

func (AgentTraceSpan) TableName() string { return "agent_trace_spans" }

// AgentTraceSpanLink is a non-parent causal edge between spans.
type AgentTraceSpanLink struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	SpanID        []byte    `gorm:"not null;index:idx_agent_trace_span_links_span" json:"span_id"`
	SpanStart     time.Time `gorm:"not null;index:idx_agent_trace_span_links_span_start" json:"span_start"`
	LinkedTraceID []byte    `gorm:"not null" json:"linked_trace_id"`
	LinkedSpanID  []byte    `gorm:"not null;index:idx_agent_trace_span_links_linked" json:"linked_span_id"`
	LinkType      string    `gorm:"size:32;not null" json:"link_type"` // caused_by | continues | subagent_of | retry_of | provisional_context | candidate
	Confidence    string    `gorm:"size:16;not null" json:"confidence"`
	Rule          string    `gorm:"size:64;not null" json:"rule"`
	Detail        RawJSON   `gorm:"type:text" json:"detail,omitempty"`
}

func (AgentTraceSpanLink) TableName() string { return "agent_trace_span_links" }

// AgentTraceAgent is one agent (declared or inferred) within a session.
type AgentTraceAgent struct {
	ID              string  `gorm:"primaryKey;size:36" json:"id"`
	SessionID       string  `gorm:"size:36;not null;index:idx_agent_trace_agents_session" json:"session_id"`
	Name            string  `gorm:"size:255;not null" json:"name"` // declared name or generated "Agent 2 (inferred)"
	Declared        bool    `gorm:"not null" json:"declared"`
	ParentAgentID   *string `gorm:"size:36" json:"parent_agent_id,omitempty"`
	SpawnedBySpanID []byte  `json:"spawned_by_span_id,omitempty"`
	Confidence      string  `gorm:"size:16;not null" json:"confidence"`
	// Fingerprints: key id byte + HMAC-SHA-256 (see the package comment above).
	SystemPromptFP  []byte    `gorm:"column:system_prompt_fp" json:"system_prompt_fp,omitempty"`
	ToolsetFP       []byte    `gorm:"column:toolset_fp" json:"toolset_fp,omitempty"`
	FirstSeen       time.Time `gorm:"not null" json:"first_seen"`
	LastSeen        time.Time `gorm:"not null;index:idx_agent_trace_agents_last_seen" json:"last_seen"`
	SelfCostMicros  int64     `json:"self_cost_micros"`
	TotalCostMicros int64     `json:"total_cost_micros"`
	SelfTokens      int64     `json:"self_tokens"`
	TotalTokens     int64     `json:"total_tokens"`
}

func (AgentTraceAgent) TableName() string { return "agent_trace_agents" }

// AgentTracePayload is captured content, kept apart from the span, size-capped,
// compressed and (at the hub) encrypted with AES-256-GCM. ExpiresAt is the
// payload retention, which never outlives the session's metadata retention.
type AgentTracePayload struct {
	ID           string `gorm:"primaryKey;size:36" json:"id"`
	SpanID       []byte `gorm:"not null;index:idx_agent_trace_payloads_span" json:"span_id"`
	AppID        uint   `gorm:"not null" json:"app_id"`
	Part         string `gorm:"size:32;not null" json:"part"`          // request | response | system_instructions | tool_definitions | tool_args | tool_result | error_body
	ContentState string `gorm:"size:16;not null" json:"content_state"` // hashed | redacted | full | truncated
	MediaType    string `gorm:"size:128;not null" json:"media_type"`
	Encoding     string `gorm:"size:16;not null" json:"encoding"` // zstd | gzip | identity
	Encrypted    bool   `gorm:"not null" json:"encrypted"`
	// KeyID is the D4 payload key id the data was encrypted with (0 when not encrypted).
	KeyID          int       `gorm:"not null;default:0" json:"key_id"`
	Data           []byte    `json:"-"`                                  // NULL when BlobRef is used
	BlobRef        string    `gorm:"size:512" json:"blob_ref,omitempty"` // object-store key (optional backend)
	OriginalBytes  int       `gorm:"not null;default:0" json:"original_bytes"`
	StoredBytes    int       `gorm:"not null;default:0" json:"stored_bytes"`
	Truncated      bool      `gorm:"not null" json:"truncated"`
	RedactionCount int       `gorm:"not null;default:0" json:"redaction_count"`
	Fingerprint    []byte    `json:"fingerprint,omitempty"` // key id byte + HMAC-SHA-256 of the canonical content
	CreatedAt      time.Time `gorm:"not null" json:"created_at"`
	ExpiresAt      time.Time `gorm:"not null;index:idx_agent_trace_payloads_expires" json:"expires_at"`
}

func (AgentTracePayload) TableName() string { return "agent_trace_payloads" }

// AgentTraceCorrelationKey is the short-lived correlation index the assembler
// looks keys up in (tool_call_id, response_id, args_fp, ...). KeyHash is the
// key id byte + HMAC-SHA-256 of the key.
type AgentTraceCorrelationKey struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	AppID     uint      `gorm:"not null;index:idx_agent_trace_correlation_keys_lookup,priority:1" json:"app_id"`
	KeyType   string    `gorm:"size:32;not null;index:idx_agent_trace_correlation_keys_lookup,priority:2" json:"key_type"` // tool_call_id | response_id | prefix_out | prefix_in | args_fp | result_fp | text_fp | ext_session
	KeyHash   []byte    `gorm:"not null;index:idx_agent_trace_correlation_keys_lookup,priority:3" json:"key_hash"`
	SpanID    []byte    `gorm:"not null" json:"span_id"`
	TraceID   []byte    `gorm:"not null" json:"trace_id"`
	SessionID string    `gorm:"size:36;not null" json:"session_id"`
	AgentRef  *string   `gorm:"size:36" json:"agent_ref,omitempty"`
	Attrs     RawJSON   `gorm:"type:text" json:"attrs,omitempty"` // e.g. {tool_name, pending:true}
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
	ExpiresAt time.Time `gorm:"not null;index:idx_agent_trace_correlation_keys_expires" json:"expires_at"`
}

func (AgentTraceCorrelationKey) TableName() string { return "agent_trace_correlation_keys" }

// AgentTraceCaptureEvent is the append-only staging table between the
// capture transports and the assembler (D3). The embedded gateway writes it
// directly; edge events arrive in AnalyticsPulse field 15 and are written by
// the hub's ingest. IdempotencyKey ("<source>:<span id hex>:<seq>", built by
// the producer) is unique, so a redelivered pulse inserts nothing. The
// assembler consumes rows in ID order with a cursor (ListAgentTraceCaptureEventsAfter).
type AgentTraceCaptureEvent struct {
	ID             uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	IdempotencyKey string    `gorm:"size:255;not null;uniqueIndex:idx_agent_trace_capture_events_idem" json:"idempotency_key"`
	Source         string    `gorm:"size:128;not null" json:"source"` // "embedded" or the edge id
	Kind           string    `gorm:"size:32;not null" json:"kind"`
	AppID          uint      `json:"app_id"`
	SpanID         []byte    `json:"span_id"`
	ReceivedAt     time.Time `gorm:"not null;index:idx_agent_trace_capture_events_received" json:"received_at"`
	Event          []byte    `gorm:"not null" json:"-"` // the marshalled capture event; opaque here
	Size           int       `gorm:"not null;default:0" json:"size"`
}

func (AgentTraceCaptureEvent) TableName() string { return "agent_trace_capture_events" }

// AgentTraceModels are the agent tracing tables, migrated with the analytics
// tables (AnalyticsModels).
func AgentTraceModels() []interface{} {
	return []interface{}{
		&AgentTraceSession{},
		&AgentTraceSessionModel{},
		&AgentTraceSessionTool{},
		&AgentTraceSessionAgent{},
		&AgentTraceTrace{},
		&AgentTraceSpan{},
		&AgentTraceSpanLink{},
		&AgentTraceAgent{},
		&AgentTracePayload{},
		&AgentTraceCorrelationKey{},
		&AgentTraceCaptureEvent{},
	}
}
