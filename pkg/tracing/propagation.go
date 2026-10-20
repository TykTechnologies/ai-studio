package tracing

import "sync/atomic"

// Outbound propagation switches (AGENT_TRACING_PROPAGATE_LLM_PROVIDERS,
// _HTTP_UPSTREAMS, _MCP; spec §4.6 as amended by Q11). All default on, in
// every edition; an LLM can still opt out on its own.
var noPropagateLLM, noPropagateHTTP, noPropagateMCP atomic.Bool

// SetOutboundPropagation sets which upstreams the gateway continues the
// trace into: LLM providers, HTTP upstreams (tools, datasources), and MCP
// servers (params._meta).
func SetOutboundPropagation(llmProviders, httpUpstreams, mcp bool) {
	noPropagateLLM.Store(!llmProviders)
	noPropagateHTTP.Store(!httpUpstreams)
	noPropagateMCP.Store(!mcp)
}

// PropagateToLLMProviders reports whether LLM upstreams get the trace context.
func PropagateToLLMProviders() bool { return !noPropagateLLM.Load() }

// PropagateToHTTPUpstreams reports whether tool and other HTTP upstreams get
// the trace context.
func PropagateToHTTPUpstreams() bool { return !noPropagateHTTP.Load() }

// PropagateToMCP reports whether MCP servers get the trace context in _meta.
func PropagateToMCP() bool { return !noPropagateMCP.Load() }
