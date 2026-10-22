package tracing

import (
	"context"
	"net/http"
)

// MCPMetaInjector carries trace context into an outbound MCP request's
// params._meta (SEP-414: traceparent, tracestate and baggage as _meta keys).
//
// Studio does not proxy MCP to MCP today: the MCP shim serves OpenAPI tools
// and calls their upstreams over HTTP, which carry the context as headers
// (InjectOutgoing). A future MCP-to-MCP proxy injects through this interface,
// so outbound _meta follows the same rules as the headers.
type MCPMetaInjector interface {
	InjectMeta(ctx context.Context, meta map[string]any) map[string]any
}

// MCPMeta is the default MCPMetaInjector (InjectMCPMeta).
var MCPMeta MCPMetaInjector = mcpMeta{}

type mcpMeta struct{}

func (mcpMeta) InjectMeta(ctx context.Context, meta map[string]any) map[string]any {
	return InjectMCPMeta(ctx, meta)
}

// InjectMCPMeta writes the current span context into meta and returns it
// (a new map when meta is nil). Trace keys the client sent are replaced by
// the gateway span context, whose parent is the client's, so the chain stays
// intact; baggage is reduced to the allowlist as on HTTP. With
// AGENT_TRACING_PROPAGATE_MCP off the trace keys are removed instead.
func InjectMCPMeta(ctx context.Context, meta map[string]any) map[string]any {
	h := http.Header{}
	if PropagateToMCP() {
		InjectOutgoing(ctx, h)
	}
	if meta == nil {
		meta = map[string]any{}
	}
	for key, header := range map[string]string{
		"traceparent": headerTraceparent,
		"tracestate":  headerTracestate,
		"baggage":     headerBaggage,
	} {
		delete(meta, key)
		if v := h.Get(header); v != "" {
			meta[key] = v
		}
	}
	return meta
}
