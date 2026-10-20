// Package declared extracts the context a client declares about a gateway
// request (spec enterprise/features/agenttracing/SPEC.md §4.2 and §4.3):
// W3C trace context and baggage, the X-Tyk-* tracing headers, the
// compatibility header families of other gateways, a few body fields, and
// the session ids of verified coding harnesses.
//
// Everything here is untrusted client input. It labels and groups spans; it
// must never feed authentication, authorisation or App scoping.
//
// The package is pure and inert: callers run Extract only when agent
// tracing is enabled (agenttrace.Enabled). StripTracingHeaders is the one
// part that runs in every edition, so X-Tyk-* tracing headers never reach a
// vendor.
//
// # Precedence (field-wise, first source wins)
//
//   - Trace id, parent span id, sampled: traceparent (for MCP requests,
//     params._meta.traceparent first) > X-Tyk-Trace-Id / X-Tyk-Parent-Span-Id
//     > Portkey > Cloudflare AI Gateway. traceparent always wins these.
//   - Session, agent, path, end user: X-Tyk-* > compat families > baggage >
//     body fields > harness extractors. §4.2's note ("Tyk/compat headers
//     override traceparent for session/agent/path") is applied to the whole
//     W3C context, baggage included, so an explicit Tyk or compat header
//     beats an ambient baggage value an OTel SDK propagated.
package declared

import (
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// Compat family names (config AGENT_TRACING_COMPAT_HEADERS).
const (
	FamilyHelicone    = "helicone"
	FamilyPortkey     = "portkey"
	FamilyOpenRouter  = "openrouter"
	FamilyBifrost     = "bifrost"
	FamilyAgentRouter = "agent_router"
	FamilyCloudflare  = "cloudflare"
	FamilyAgentCore   = "agentcore"
	FamilyMCPSession  = "mcp_session"
)

// Harness extractor names (config AGENT_TRACING_HARNESS_SESSION_HEADERS).
// Other entries in that list are header names read as a session id.
const (
	HarnessClaudeCode = "claude_code"
	HarnessCodexCLI   = "codex_cli"
	HarnessOpenCode   = "opencode"
)

// Options configure an Extractor. Build one per configuration change, not
// per request.
type Options struct {
	// CompatFamilies are the enabled compat families (Family* names).
	CompatFamilies []string
	// Harness lists enabled harness extractors (Harness* names) and extra
	// header names read as a session id.
	Harness []string
	// BaggageAllowlist names the baggage keys kept in DeclaredContext.Baggage
	// (§4.6). The correlation keys are read whatever the allowlist says.
	BaggageAllowlist []string
}

// HarnessInfo is what a coding-harness extractor found (bead jyx.52).
type HarnessInfo struct {
	// Name is the harness (Harness* constant), set whenever the request is
	// recognised, even when another source supplied the session.
	Name    string
	Version string
	// TurnID is a client-declared turn id (Codex CLI X-Codex-Turn-Metadata
	// turn_id): one id for all requests of a user turn.
	TurnID string
	// SubAgentID is a client-declared sub-agent instance (Claude Code
	// X-Claude-Code-Agent-Id); empty on the main agent's requests.
	SubAgentID string
}

// Result is the extracted context of one request.
type Result struct {
	Declared agenttrace.DeclaredContext
	Harness  HarnessInfo
	// MCPMethod and MCPName classify an MCP request: the Mcp-Method /
	// Mcp-Name headers (MCP 2026-07-28), else the JSON-RPC body.
	MCPMethod string
	MCPName   string
	// Invalid lists the inputs that were present but rejected, as
	// "<source>:<reason>", for the tracing-health counters.
	Invalid []string

	sessionLevel int  // level of the source that set the session
	convDerived  bool // ConversationID was copied from SessionID
}

// Extractor applies one Options to requests. It is safe for concurrent use.
type Extractor struct {
	families     map[string]bool
	harness      map[string]bool
	extraHeaders []string
	allow        map[string]bool
}

// New builds an Extractor.
func New(o Options) *Extractor {
	x := &Extractor{families: map[string]bool{}, harness: map[string]bool{}, allow: map[string]bool{}}
	for _, f := range o.CompatFamilies {
		x.families[strings.ToLower(strings.TrimSpace(f))] = true
	}
	for _, h := range o.Harness {
		h = strings.TrimSpace(h)
		switch strings.ToLower(h) {
		case HarnessClaudeCode, HarnessCodexCLI, HarnessOpenCode:
			x.harness[strings.ToLower(h)] = true
		case "":
		default:
			x.extraHeaders = append(x.extraHeaders, http.CanonicalHeaderKey(h))
		}
	}
	for _, k := range o.BaggageAllowlist {
		x.allow[strings.TrimSpace(k)] = true
	}
	return x
}

// Body formats Extract understands. Any other value skips body fields.
const (
	BodyLLM = "llm" // an LLM request body (OpenAI, Anthropic, ...)
	BodyMCP = "mcp" // an MCP JSON-RPC request
)

// Session precedence levels (lower wins). Baggage sits below the compat
// families: see the package comment.
const (
	levelTyk     = 20
	levelCompat  = 30
	levelBaggage = 35
	levelBody    = 40
	levelHarness = 50
)

// Extract is Headers followed by ApplyBody: the whole declared context of a
// request. bodyKind is BodyLLM, BodyMCP or "" (no body fields); body is the
// already-buffered request body, never read here.
func (x *Extractor) Extract(h http.Header, bodyKind string, body []byte) Result {
	r := x.Headers(h, bodyKind, body)
	if bodyKind == BodyLLM {
		x.ApplyBody(&r, body)
	}
	return r
}

// Headers reads everything but the LLM body: the headers, and for an MCP
// request (bodyKind BodyMCP) its small JSON-RPC body, whose
// _meta.traceparent outbound propagation needs. It is what the request
// path runs. An LLM body can be large, so its fields are left to ApplyBody,
// which callers run once the response is on its way.
func (x *Extractor) Headers(h http.Header, bodyKind string, body []byte) Result {
	var r Result
	c := collector{r: &r}

	ua := h.Get("User-Agent")
	var mcp mcpBody
	if bodyKind == BodyMCP {
		mcp = parseMCPBody(body, &c)
	}

	// Trace id and parent span (traceparent always wins these).
	if bodyKind == BodyMCP && mcp.traceparent != "" {
		c.traceparent(mcp.traceparent, mcp.tracestate, "mcp_meta")
	}
	if tp, ok := single(h, "Traceparent", &c); ok {
		c.traceparent(tp, h.Get("Tracestate"), "traceparent")
	}

	// Session, agent, path, user: Tyk headers first.
	c.level = levelTyk
	x.tykHeaders(h, &c)
	c.level = levelCompat
	x.compat(h, ua, &c)
	c.level = levelBaggage
	bag := parseBaggage(h.Values("Baggage"), &c)
	x.baggage(bag, &c)
	c.level = levelHarness
	x.harnesses(h, ua, &c)

	if bodyKind == BodyMCP {
		r.MCPMethod, r.MCPName = mcp.method, mcp.name
		r.Declared.ClientInfo = mcp.clientInfo
	}
	if m := h.Get("Mcp-Method"); m != "" {
		if s, ok := boundedASCII(m, 128); ok {
			r.MCPMethod = s
		} else {
			c.invalid("mcp-method", "format")
		}
	}
	if n := h.Get("Mcp-Name"); n != "" {
		if s, ok := boundedText(n, 255); ok {
			r.MCPName = s
		} else {
			c.invalid("mcp-name", "format")
		}
	}

	r.Declared.Harness, r.Declared.TurnID, r.Declared.SubAgentID = r.Harness.Name, r.Harness.TurnID, r.Harness.SubAgentID
	r.finish()
	return r
}

// ApplyBody adds the priority-4 body fields of an LLM request (OpenAI
// conversation, metadata.session_id, user) to r, and Claude Code's
// metadata.user_id session when its header was missing. Precedence holds as
// if Extract had run: a body session beats a harness one, never a header.
func (x *Extractor) ApplyBody(r *Result, body []byte) {
	if len(body) == 0 {
		return
	}
	c := collector{r: r, level: levelBody}
	keys := keysIn(body)
	parseLLMBody(body, keys, &c)
	if r.Harness.Name == HarnessClaudeCode && r.Declared.SessionID == "" {
		c.level = levelHarness
		if s := claudeMetadataSession(body, keys, &c); s != "" {
			c.session(s, HarnessClaudeCode)
		}
	}
	r.finish()
}

// finish derives the conversation id: every declared session id is
// client-supplied, so it may be the conversation id (§5.3 conversation id
// rule) unless the client named one explicitly.
func (r *Result) finish() {
	d := &r.Declared
	if d.ConversationID == "" || r.convDerived {
		d.ConversationID = d.SessionID
		r.convDerived = d.ConversationID != ""
	}
}

// collector records each source's values; for most fields the first source
// to supply one wins, the session goes to the lowest level.
type collector struct {
	r     *Result
	level int
}

func (c collector) invalid(source, reason string) {
	c.r.Invalid = append(c.r.Invalid, source+":"+reason)
}

func set(dst *string, v string) bool {
	if *dst != "" || v == "" {
		return false
	}
	*dst = v
	return true
}

func (c collector) session(id, source string) {
	if id == "" || (c.r.sessionLevel != 0 && c.level >= c.r.sessionLevel) {
		return
	}
	c.r.Declared.SessionID, c.r.Declared.SessionSource = id, source
	c.r.sessionLevel = c.level
}

// conversation sets an explicitly declared conversation id. The first
// explicit one wins; it replaces one finish derived from the session.
func (c collector) conversation(id string) {
	if c.r.Declared.ConversationID == "" || c.r.convDerived {
		c.r.Declared.ConversationID = id
		c.r.convDerived = false
	}
}

// single returns a header's only value. Several values for a header that
// carries one id are ambiguous, so they are rejected.
func single(h http.Header, key string, c *collector) (string, bool) {
	vs := h.Values(key)
	switch len(vs) {
	case 0:
		return "", false
	case 1:
		v := strings.TrimSpace(vs[0])
		return v, v != ""
	default:
		c.invalid(strings.ToLower(key), "repeated")
		return "", false
	}
}
