package tykmcp

import (
	"strings"
	"time"
)

// TykAnalyticsRecord is a Tyk Gateway analytics record as Tyk Pump
// serialises it (tyk-pump analytics.AnalyticsRecord): the body the ingest
// endpoint accepts is a JSON array of them. Only the fields MCP analytics
// uses are declared; the decoder ignores the rest (raw request and response
// bodies, IP address, geo data), so they are never held, let alone stored.
// It mirrors the pump's JSON tags rather than importing tyk-pump, whose
// module carries its own gorm fork.
type TykAnalyticsRecord struct {
	Method       string `json:"method"`
	Path         string `json:"path"`
	ResponseCode int    `json:"response_code"`
	// APIKey is the key's hash when the Gateway runs with hash_keys (the
	// default), and the raw key otherwise. It is used to attribute the call
	// and never persisted.
	APIKey    string          `json:"api_key"`
	TimeStamp time.Time       `json:"timestamp"`
	APIName   string          `json:"api_name"`
	APIID     string          `json:"api_id"`
	OrgID     string          `json:"org_id"`
	Network   TykNetworkStats `json:"network"`
	Latency   TykLatency      `json:"latency"`
	Tags      []string        `json:"tags"`
	Alias     string          `json:"alias"`
	MCPStats  TykMCPStats     `json:"mcp_stats"`
}

// TykNetworkStats is tyk-pump analytics.NetworkStats.
type TykNetworkStats struct {
	OpenConnections   int64 `json:"open_connections"`
	ClosedConnections int64 `json:"closed_connections"`
	BytesIn           int64 `json:"bytes_in"`
	BytesOut          int64 `json:"bytes_out"`
}

// TykLatency is tyk-pump analytics.Latency, in milliseconds.
type TykLatency struct {
	Total    int64 `json:"total"`
	Upstream int64 `json:"upstream"`
	Gateway  int64 `json:"gateway"`
}

// TykMCPStats is tyk-pump analytics.MCPStats (Gateway 5.14+, Pump 1.16+;
// the error code and protocol versions arrived later, and decode as zero
// from older producers).
type TykMCPStats struct {
	IsMCP                    bool   `json:"is_mcp"`
	JSONRPCMethod            string `json:"jsonrpc_method"`
	PrimitiveType            string `json:"primitive_type"`
	PrimitiveName            string `json:"primitive_name"`
	JSONRPCErrorCode         int64  `json:"jsonrpc_error_code"`
	EffectiveProtocolVersion string `json:"effective_protocol_version"`
	DeclaredProtocolVersion  string `json:"declared_protocol_version"`
	ProtocolVersionSource    string `json:"protocol_version_source"`
}

// Tag prefixes the Gateway writes into a record's tags.
const (
	// tagTraceIDPrefix carries the OpenTelemetry trace id.
	tagTraceIDPrefix = "trace-id-"
	// tagMCPSessionPrefix is the mcp-session-id request header when the API
	// lists it in tag_headers (the Gateway lower-cases the header name).
	tagMCPSessionPrefix = "mcp-session-id-"
)

// TraceID returns the trace id tag, or "".
func (r *TykAnalyticsRecord) TraceID() string {
	return tagValue(r.Tags, tagTraceIDPrefix)
}

// SessionID returns the MCP session id tag, or "".
func (r *TykAnalyticsRecord) SessionID() string {
	return tagValue(r.Tags, tagMCPSessionPrefix)
}

// ProtocolVersion is the effective MCP protocol version, falling back to the
// one the client declared.
func (r *TykAnalyticsRecord) ProtocolVersion() string {
	if v := r.MCPStats.EffectiveProtocolVersion; v != "" {
		return v
	}
	return r.MCPStats.DeclaredProtocolVersion
}

func tagValue(tags []string, prefix string) string {
	for _, t := range tags {
		if strings.HasPrefix(t, prefix) && len(t) > len(prefix) {
			return t[len(prefix):]
		}
	}
	return ""
}
