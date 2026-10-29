package models

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

type InteractionType string

const (
	ChatInteraction  InteractionType = "chat"
	ProxyInteraction InteractionType = "proxy"
)

// RequestKind is the model class a call was made to. It is orthogonal to
// InteractionType, which says where the call came from (chat UI or proxy).
type RequestKind string

const (
	// GenerativeRequest is chat, completion and embedding traffic. It is the
	// empty string so rows written before request kinds existed match it.
	GenerativeRequest RequestKind = ""
	// DecisionRequest is a call to a decision model (/v1/decisions,
	// /v1/systemone): typed answers instead of generated text.
	DecisionRequest RequestKind = "decision"
)

// LLMChatRecord logs usage for cost and analytics.
//
// Rows are insert-only: never update or delete them. The hub budget sync
// (grpc budgetUsageTracker) and usage telemetry (TokenTotals) read only the
// rows added since their last pass, found by id, so a changed or removed row
// would stay counted as it was.
type LLMChatRecord struct {
	gorm.Model
	ID     uint `gorm:"primaryKey"`
	Name   string
	Vendor string
	// Add LLMID so we can track usage for that specific LLM object:
	LLMID                  uint      `gorm:"index:idx_llm_chat_records_llm_time,priority:1"`
	TotalTimeMS            int
	PromptTokens           int
	ResponseTokens         int
	TotalTokens            int
	TimeStamp              time.Time `gorm:"index:idx_llm_chat_records_time;index:idx_llm_chat_records_app_time,priority:2;index:idx_llm_chat_records_llm_time,priority:2;index:idx_llm_chat_records_team_time,priority:2;index:idx_llm_chat_records_kind_time,priority:2"`
	UserID                 uint      `gorm:"index"`
	Choices                int
	ToolCalls              int
	ChatID                 string
	AppID                  uint    `gorm:"index:idx_llm_chat_records_app_time,priority:1"`
	Cost                   float64
	Currency               string
	InteractionType        InteractionType `gorm:"type:string;default:'chat'"`
	CacheWritePromptTokens int
	CacheReadPromptTokens  int
	// TeamID is the team the spend is attributed to: the App's team for
	// proxy and edge traffic, the user's budget team for chat.
	TeamID *uint `gorm:"index:idx_llm_chat_records_team_time,priority:1"`
	// OnBehalfOf and ActingAgent are who the call was for and the agent that
	// made it, when an auth plugin said (a delegated token's sub, and its act
	// or azp). UserID stays the App owner. Audit only.
	OnBehalfOf  string `gorm:"size:255"`
	ActingAgent string `gorm:"size:255"`
	// RequestKind is the model class of the call ("" generative, "decision").
	RequestKind RequestKind `gorm:"size:16;default:'';index:idx_llm_chat_records_kind_time,priority:1"`
	// DecisionQuestions and DecisionRefusals count, for a decision call, the
	// questions asked and the ones the model declined to answer. Answered
	// questions are Choices.
	DecisionQuestions int
	DecisionRefusals  int
	// ServiceTier is the vendor service tier the usage was billed at, when
	// the response said (IPriceAdjustedResponse: "batch", "priority",
	// "flex"...); empty for list-price traffic and older rows (NULL).
	ServiceTier string `gorm:"size:32"`
	// RequestedModel is the model the caller asked for, when it said. Name
	// is the model the vendor reported, which can be a versioned name
	// (gemini-2.0-flash-001 for gemini-2.0-flash); the gateway prices such a
	// row by the requested name when the reported one is unpriced, and a
	// recalculation of the requested name's price reaches it the same way.
	// Empty on edge rows and older rows (NULL).
	RequestedModel string
	// PriceMultiplier is the multiple of the list price Cost was billed at
	// (IPriceAdjustedResponse, after the LLM's per-tier override): 0.5 for
	// a batch row, 1 at list price. A price recalculation reprices from it,
	// since it cannot be recovered from Cost once the list cost has been 0.
	// NULL on older and edge rows until a recalculation records the ratio.
	PriceMultiplier *float64
}

// LLMChatLogEntry for storing extra logs
type LLMChatLogEntry struct {
	gorm.Model
	ID        uint `gorm:"primaryKey"`
	Name      string
	Vendor    string
	TimeStamp time.Time
	Prompt    string
	Response  string
	Tokens    int
	UserID    uint
	ChatID    string
	SessionID string
}

// records tool usage
type ToolCallRecord struct {
	gorm.Model
	ID        uint `gorm:"primaryKey"`
	ToolID    uint
	Name      string
	ExecTime  int
	TimeStamp time.Time
}

// MCP call record sources.
const (
	// MCPCallSourceTykPump is a record a Tyk Pump delivered to the ingest
	// endpoint (the http pump, or the stdout pump through a log shipper).
	MCPCallSourceTykPump = "tyk_pump"
)

// MCPCallRecord is one MCP request a Tyk Gateway proxied for a server Studio
// brokers. Studio never sees that traffic, so the rows come from the
// Gateway's analytics records, delivered through Tyk Pump.
//
// By design there is no column for a raw API key, a request or response
// body, or a client IP: the ingester attributes the call to an App and
// credential and drops the rest.
type MCPCallRecord struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	// Timestamp is when the Gateway handled the request.
	Timestamp time.Time `gorm:"not null;index:idx_mcp_calls_time;index:idx_mcp_calls_server_time,priority:2;index:idx_mcp_calls_app_time,priority:2;index:idx_mcp_calls_conn_time,priority:2" json:"timestamp"`

	ConnectionID uint `gorm:"not null;index:idx_mcp_calls_conn_time,priority:1" json:"connection_id"`
	// MCPServerID is nil when the record could not be tied to a server.
	MCPServerID *uint  `gorm:"index:idx_mcp_calls_server_time,priority:1" json:"mcp_server_id"`
	TykAPIID    string `gorm:"size:64" json:"tyk_api_id"`
	OrgID       string `gorm:"size:64" json:"org_id"`
	// AppID and CredentialID are set when the call's key is a brokered
	// credential (or its alias names the App); nil otherwise.
	AppID        *uint   `gorm:"index:idx_mcp_calls_app_time,priority:1" json:"app_id"`
	CredentialID *string `gorm:"size:36" json:"credential_id"`

	JSONRPCMethod    string `gorm:"column:jsonrpc_method;size:128" json:"jsonrpc_method"`
	PrimitiveType    string `gorm:"size:32" json:"primitive_type"`
	PrimitiveName    string `gorm:"size:512" json:"primitive_name"`
	JSONRPCErrorCode int    `gorm:"column:jsonrpc_error_code" json:"jsonrpc_error_code"`
	ResponseCode     int    `json:"response_code"`

	LatencyTotalMs    int64 `json:"latency_total_ms"`
	LatencyUpstreamMs int64 `json:"latency_upstream_ms"`
	LatencyGatewayMs  int64 `json:"latency_gateway_ms"`
	BytesIn           int64 `json:"bytes_in"`
	BytesOut          int64 `json:"bytes_out"`

	ProtocolVersion string `gorm:"size:32" json:"protocol_version"`
	SessionID       string `gorm:"size:128" json:"session_id"`
	TraceID         string `gorm:"size:128" json:"trace_id"`
	Source          string `gorm:"size:32" json:"source"`
}

// ClampLengths cuts the string fields to their column sizes, so one
// oversized value from a Gateway cannot fail the insert of a whole batch
// (Postgres rejects a varchar overflow).
func (r *MCPCallRecord) ClampLengths() {
	r.TykAPIID = clampString(r.TykAPIID, 64)
	r.OrgID = clampString(r.OrgID, 64)
	if r.CredentialID != nil {
		c := clampString(*r.CredentialID, 36)
		r.CredentialID = &c
	}
	r.JSONRPCMethod = clampString(r.JSONRPCMethod, 128)
	r.PrimitiveType = clampString(r.PrimitiveType, 32)
	r.PrimitiveName = clampString(r.PrimitiveName, 512)
	r.ProtocolVersion = clampString(r.ProtocolVersion, 32)
	r.SessionID = clampString(r.SessionID, 128)
	r.TraceID = clampString(r.TraceID, 128)
	r.Source = clampString(r.Source, 32)
}

// clampString drops invalid UTF-8 and NUL bytes and cuts s to at most n
// bytes without splitting a character (Postgres rejects invalid UTF-8 and
// NUL in text as well as an overflow). varchar(n) counts characters, so n
// bytes always fits.
func clampString(s string, n int) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	if strings.IndexByte(s, 0) >= 0 {
		s = strings.ReplaceAll(s, "\x00", "")
	}
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 {
		r, size := utf8.DecodeLastRuneInString(s)
		if r != utf8.RuneError || size != 1 {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}

// ChartData represents data for charts
type ChartData struct {
	Labels []string  `json:"labels"`
	Data   []float64 `json:"data"`
}

// MultiAxisChartData represents data for a chart with multiple y-axes
type MultiAxisChartData struct {
	Labels   []string  `json:"labels"`
	Datasets []Dataset `json:"datasets"`
}

// Dataset represents a single dataset in a multi-axis chart
type Dataset struct {
	Label string    `json:"label"`
	Data  []float64 `json:"data"`
	Yaxis string    `json:"yAxisID"`
}

// VendorModelCost represents the total cost for a specific vendor and model
type VendorModelCost struct {
	Vendor    string  `json:"vendor"`
	Model     string  `json:"model"`
	TotalCost float64 `json:"totalCost"`
	Currency  string  `json:"currency"`
}

// AppBudgetUsageResponse represents the budget usage for a specific app
type AppBudgetUsageResponse struct {
	CurrentUsage  float64   `json:"current_usage"`
	MonthlyBudget *float64  `json:"monthly_budget"`
	Percentage    *float64  `json:"percentage"`
	StartDate     time.Time `json:"start_date"`
}

// ProxyLogResponse represents a proxy log response in JSON API format
type ProxyLogResponse struct {
	Type       string             `json:"type"`
	ID         string             `json:"id"`
	Attributes ProxyLogAttributes `json:"attributes"`
}

// ProxyLogAttributes is the serialised form of a ProxyLog row. The failover
// marker is carried so a fallback row can be told from a primary one:
// FailoverAttempt is the 1-based rung index (0 = primary) and
// FailoverFromLLMID the primary the request failed over from, absent for a
// primary attempt.
type ProxyLogAttributes struct {
	AppID             uint      `json:"app_id"`
	UserID            uint      `json:"user_id"`
	LLMID             uint      `json:"llm_id"`
	ModelName         string    `json:"model_name"`
	TimeStamp         time.Time `json:"time_stamp"`
	Vendor            string    `json:"vendor"`
	RequestBody       string    `json:"request_body"`
	ResponseBody      string    `json:"response_body"`
	ResponseCode      int       `json:"response_code"`
	FailoverAttempt   int       `json:"failover_attempt"`
	FailoverFromLLMID *uint     `json:"failover_from_llm_id,omitempty"`
	RouterKind        string    `json:"router_kind,omitempty"`
	RouterSlug        string    `json:"router_slug,omitempty"`
	RouterPool        string    `json:"router_pool,omitempty"`
	Route             string    `json:"route,omitempty"`
	RouteReason       string    `json:"route_reason,omitempty"`
	RouteSourceModel  string    `json:"route_source_model,omitempty"`
	RouteTargetModel  string    `json:"route_target_model,omitempty"`
	RouteSelection    string    `json:"route_selection,omitempty"`
	RouteScore        float64   `json:"route_score,omitempty"`
	ShadowRoute       string    `json:"shadow_route,omitempty"`
	OnBehalfOf        string    `json:"on_behalf_of,omitempty"`
	ActingAgent       string    `json:"acting_agent,omitempty"`
	RequestID         string    `json:"request_id,omitempty"`
	ClientRequestID   string    `json:"client_request_id,omitempty"`
}

// NewProxyLogResponse serialises one ProxyLog row for the proxy-log
// endpoints.
func NewProxyLogResponse(log ProxyLog) ProxyLogResponse {
	return ProxyLogResponse{
		Type: "proxy_log",
		ID:   strconv.FormatUint(uint64(log.ID), 10),
		Attributes: ProxyLogAttributes{
			AppID:             log.AppID,
			UserID:            log.UserID,
			LLMID:             log.LLMID,
			ModelName:         log.ModelName,
			TimeStamp:         log.TimeStamp,
			Vendor:            log.Vendor,
			RequestBody:       log.RequestBody,
			ResponseBody:      log.ResponseBody,
			ResponseCode:      log.ResponseCode,
			FailoverAttempt:   log.FailoverAttempt,
			FailoverFromLLMID: log.FailoverFromLLMID,
			RouterKind:        log.RouterKind,
			RouterSlug:        log.RouterSlug,
			RouterPool:        log.RouterPool,
			Route:             log.Route,
			RouteReason:       log.RouteReason,
			RouteSourceModel:  log.RouteSourceModel,
			RouteTargetModel:  log.RouteTargetModel,
			RouteSelection:    log.RouteSelection,
			RouteScore:        log.RouteScore,
			ShadowRoute:       log.ShadowRoute,
			OnBehalfOf:        log.OnBehalfOf,
			ActingAgent:       log.ActingAgent,
			RequestID:         log.RequestID,
			ClientRequestID:   log.ClientRequestID,
		},
	}
}

// PaginatedProxyLogs represents a paginated list of proxy logs
type PaginatedProxyLogs struct {
	Data []ProxyLogResponse `json:"data"`
	Meta struct {
		TotalCount int64 `json:"total_count"`
		TotalPages int   `json:"total_pages"`
		PageSize   int   `json:"page_size"`
		PageNumber int   `json:"page_number"`
	} `json:"meta"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Errors []struct {
		Title  string `json:"title"`
		Detail string `json:"detail"`
	} `json:"errors"`
}

type ProxyLog struct {
	gorm.Model
	ID           uint      `gorm:"primaryKey"`
	AppID        uint      `gorm:"index:idx_proxy_logs_app_time,priority:1;index:idx_proxy_logs_app_code_time,priority:1"`
	UserID       uint      `gorm:"index"`
	TimeStamp    time.Time `gorm:"index:idx_proxy_logs_time;index:idx_proxy_logs_app_time,priority:2;index:idx_proxy_logs_app_code_time,priority:3;index:idx_proxy_logs_llm_time,priority:2"`
	// LLMID is the specific LLM vendor entry that handled the request.
	// Required to disambiguate when several LLM entries share a Vendor type
	// (e.g. two Anthropic entries with different API keys).
	LLMID        uint `gorm:"index:idx_proxy_logs_llm_time,priority:1"`
	Vendor       string
	ModelName    string
	RequestBody  string
	ResponseBody string
	ResponseCode int `gorm:"index:idx_proxy_logs_code;index:idx_proxy_logs_app_code_time,priority:2"`
	// FailoverFromLLMID is set when this attempt was a rung of that LLM's
	// failover waterfall; nil for a primary attempt. FailoverAttempt is the
	// 1-based rung index (0 = primary). A request that failed over leaves one
	// row per attempt, so request counts should filter failover_attempt = 0.
	FailoverFromLLMID *uint `gorm:"index:idx_proxy_logs_failover_from"`
	FailoverAttempt   int   `gorm:"default:0"`
	// Router fields are set when the request was addressed to a router: its
	// kind ("model_router"), slug, the pool or route that matched, and why
	// (RouteReason, a small fixed set).
	RouterKind  string `gorm:"size:32"`
	RouterSlug  string `gorm:"index:idx_proxy_logs_router"`
	RouterPool  string
	Route       string
	RouteReason string `gorm:"size:64"`
	// The model the caller asked the router for, the model the chosen LLM
	// was asked for (after any mapping), and how the target was selected
	// ("round_robin", "weighted").
	RouteSourceModel string
	RouteTargetModel string
	RouteSelection   string `gorm:"size:32"`
	// Semantic Routers: the similarity that decided an embedding match, and,
	// in shadow mode, the route the classifier picked (the default served).
	RouteScore  float64
	ShadowRoute string
	// OnBehalfOf and ActingAgent are who the call was for and the agent that
	// made it, when an auth plugin said (a delegated token's sub, and its act
	// or azp). UserID stays the App owner. Audit only.
	OnBehalfOf  string `gorm:"size:255"`
	ActingAgent string `gorm:"size:255"`
	// RequestKind is the model class of the call ("" generative, "decision").
	RequestKind RequestKind `gorm:"size:16;default:'';index"`
	// ServiceTier is the vendor service tier of the call's usage, as on its
	// chat record.
	ServiceTier string `gorm:"size:32"`
	// TraceSpanID is the agent tracing gateway span id (16 hex) of the logical
	// call this row belongs to; every failover rung of one /ai/ call shares
	// it. Empty while agent tracing is off. The span row's analytics_ref
	// points the other way.
	TraceSpanID string `gorm:"size:16;index:idx_proxy_logs_trace_span"`
	// RequestID is the gateway's id for the request (pkg/requestid: the
	// X-Tyk-AI-Request-ID it answered with, and the request_id of the
	// request's compliance events). Every failover attempt of one request
	// shares it. An edge row from an edge older than that keeps the edge's
	// own request id. ClientRequestID is the caller's own X-Request-ID,
	// sanitised.
	RequestID       string `gorm:"size:64;index:idx_proxy_logs_request_id"`
	ClientRequestID string `gorm:"size:128"`
}
