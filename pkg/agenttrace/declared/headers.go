package declared

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// Limits (§4.3).
const (
	maxSessionID   = 200
	maxSessionName = 120
	maxAgentName   = 120
	maxAgentID     = 200
	maxUserID      = 200
	maxAgentPath   = 512
	maxPathDepth   = 16
	maxExternalID  = 200
	maxTraceState  = 512
	maxBaggageLen  = 8192 // W3C baggage limit
	maxBaggageKeys = 64
	maxBaggageVal  = 256
)

var captureModes = map[string]bool{"off": true, "metadata": true, "hashed": true, "redacted": true, "full": true}

// traceparent parses a W3C traceparent (version 00, or a later version read
// as 00 per the spec's forward-compatibility rule).
func (c collector) traceparent(v, tracestate, source string) {
	d := &c.r.Declared
	if !d.TraceID.IsZero() {
		return
	}
	parts := strings.Split(strings.TrimSpace(v), "-")
	if len(parts) < 4 || len(parts[0]) != 2 || len(parts[1]) != 32 || len(parts[2]) != 16 || len(parts[3]) != 2 {
		c.invalid(source, "format")
		return
	}
	if parts[0] == "ff" || !lowerHex(parts[0]) || !lowerHex(parts[3]) || (parts[0] == "00" && len(parts) != 4) {
		c.invalid(source, "format")
		return
	}
	tid, ok1 := traceID(parts[1])
	sid, ok2 := spanID(parts[2])
	if !ok1 || !ok2 {
		c.invalid(source, "format")
		return
	}
	flags, _ := hex.DecodeString(parts[3])
	d.TraceID, d.ParentSpanID, d.Sampled = tid, sid, flags[0]&1 == 1
	d.TraceSource = source
	if ts := strings.TrimSpace(tracestate); ts != "" {
		if len(ts) <= maxTraceState && printableASCII(ts) {
			d.TraceState = ts
		} else {
			c.invalid(source+".tracestate", "too_long")
		}
	}
}

func (x *Extractor) tykHeaders(h http.Header, c *collector) {
	d := &c.r.Declared
	if v, ok := single(h, "X-Tyk-Session-Id", c); ok {
		if s, ok := boundedASCII(v, maxSessionID); ok {
			c.session(s, "x-tyk-session-id")
		} else {
			c.invalid("x-tyk-session-id", "format")
		}
	}
	if v, ok := single(h, "X-Tyk-Session-Name", c); ok {
		if s, ok := boundedText(v, maxSessionName); ok {
			set(&d.SessionName, s)
		} else {
			c.invalid("x-tyk-session-name", "format")
		}
	}
	if v, ok := single(h, "X-Tyk-Trace-Id", c); ok {
		if t, ok := traceID(strings.ToLower(v)); ok {
			if d.TraceID.IsZero() {
				d.TraceID, d.TraceSource = t, "x-tyk"
			}
		} else {
			c.invalid("x-tyk-trace-id", "format")
		}
	}
	if v, ok := single(h, "X-Tyk-Span-Id", c); ok {
		if _, ok := spanID(strings.ToLower(v)); ok {
			set(&d.ClientSpanID, strings.ToLower(v))
		} else {
			c.invalid("x-tyk-span-id", "format")
		}
	}
	if v, ok := single(h, "X-Tyk-Parent-Span-Id", c); ok {
		if s, ok := spanID(strings.ToLower(v)); ok {
			if d.ParentSpanID.IsZero() {
				d.ParentSpanID = s
			}
		} else {
			c.invalid("x-tyk-parent-span-id", "format")
		}
	}
	if v, ok := single(h, "X-Tyk-Agent-Name", c); ok {
		if s, ok := boundedText(v, maxAgentName); ok {
			set(&d.AgentName, s)
		} else {
			c.invalid("x-tyk-agent-name", "format")
		}
	}
	if v, ok := single(h, "X-Tyk-Agent-Id", c); ok {
		if s, ok := boundedASCII(v, maxAgentID); ok {
			set(&d.AgentID, s)
		} else {
			c.invalid("x-tyk-agent-id", "format")
		}
	}
	if v, ok := single(h, "X-Tyk-Agent-Path", c); ok {
		if p, ok := agentPath(v); ok {
			set(&d.AgentPath, p)
		} else {
			c.invalid("x-tyk-agent-path", "format")
		}
	}
	if v, ok := single(h, "X-Tyk-User-Id", c); ok {
		if s, ok := boundedText(v, maxUserID); ok {
			set(&d.EndUserID, s)
		} else {
			c.invalid("x-tyk-user-id", "format")
		}
	}
	if v, ok := single(h, "X-Tyk-Trace-Capture", c); ok {
		if m := strings.ToLower(v); captureModes[m] {
			d.CaptureMode = m
		} else {
			c.invalid("x-tyk-trace-capture", "value")
		}
	}
}

func (x *Extractor) compat(h http.Header, ua string, c *collector) {
	d := &c.r.Declared
	sessionHeader := func(family, header string, source string) {
		if !x.families[family] {
			return
		}
		if v, ok := single(h, header, c); ok {
			if s, ok := boundedASCII(v, maxSessionID); ok {
				c.session(s, source)
			} else {
				c.invalid(strings.ToLower(header), "format")
			}
		}
	}

	if x.families[FamilyHelicone] {
		sessionHeader(FamilyHelicone, "Helicone-Session-Id", "helicone")
		if v, ok := single(h, "Helicone-Session-Path", c); ok {
			if p, ok := agentPath(v); ok {
				set(&d.AgentPath, p)
			} else {
				c.invalid("helicone-session-path", "format")
			}
		}
		if v, ok := single(h, "Helicone-Session-Name", c); ok {
			if s, ok := boundedText(v, maxSessionName); ok {
				set(&d.SessionName, s)
			} else {
				c.invalid("helicone-session-name", "format")
			}
		}
	}
	if x.families[FamilyPortkey] {
		if v, ok := single(h, "X-Portkey-Trace-Id", c); ok {
			if s, ok := boundedASCII(v, maxExternalID); ok {
				if d.TraceID.IsZero() {
					t, hashed := portkeyTraceID(s)
					d.TraceID, d.TraceSource = t, "portkey"
					if hashed {
						d.ExternalTraceID = s
					}
				}
			} else {
				c.invalid("x-portkey-trace-id", "format")
			}
		}
		if v, ok := single(h, "X-Portkey-Span-Id", c); ok {
			if s, ok := boundedASCII(v, maxExternalID); ok {
				set(&d.ClientSpanID, portkeySpanID(s).String())
			} else {
				c.invalid("x-portkey-span-id", "format")
			}
		}
		if v, ok := single(h, "X-Portkey-Parent-Span-Id", c); ok {
			if s, ok := boundedASCII(v, maxExternalID); ok {
				if d.ParentSpanID.IsZero() {
					d.ParentSpanID = portkeySpanID(s)
				}
			} else {
				c.invalid("x-portkey-parent-span-id", "format")
			}
		}
	}
	// OpenCode sends X-Session-Id, the OpenRouter header (bead jyx.59):
	// group it the same way but label it as OpenCode.
	openRouterSource := "openrouter"
	if harnessUA(ua, "opencode/") != "" {
		openRouterSource = HarnessOpenCode
	}
	sessionHeader(FamilyOpenRouter, "X-Session-Id", openRouterSource)
	sessionHeader(FamilyBifrost, "X-Bf-Session-Id", "bifrost")
	sessionHeader(FamilyAgentRouter, "Agent-Session-Id", "agent_router")
	sessionHeader(FamilyAgentCore, "X-Amzn-Bedrock-Agentcore-Runtime-Session-Id", "agentcore")
	if x.families[FamilyCloudflare] {
		if v, ok := single(h, "Cf-Aig-Otel-Trace-Id", c); ok {
			if t, ok := traceID(strings.ToLower(v)); ok {
				if d.TraceID.IsZero() {
					d.TraceID, d.TraceSource = t, "cloudflare"
				}
			} else {
				c.invalid("cf-aig-otel-trace-id", "format")
			}
		}
		if v, ok := single(h, "Cf-Aig-Otel-Parent-Span-Id", c); ok {
			if s, ok := spanID(strings.ToLower(v)); ok {
				if d.ParentSpanID.IsZero() {
					d.ParentSpanID = s
				}
			} else {
				c.invalid("cf-aig-otel-parent-span-id", "format")
			}
		}
	}
	if x.families[FamilyMCPSession] {
		if v, ok := single(h, "Mcp-Session-Id", c); ok {
			if s, ok := boundedASCII(v, maxSessionID); ok {
				d.MCPSessionID = s // a weak hint only, never the session (§4.3)
			} else {
				c.invalid("mcp-session-id", "format")
			}
		}
	}
}

// baggage applies the W3C baggage correlation keys, then keeps the
// allowlisted ones (§4.6).
func (x *Extractor) baggage(bag map[string]string, c *collector) {
	if len(bag) == 0 {
		return
	}
	d := &c.r.Declared
	if v := bag["session.id"]; v != "" {
		if s, ok := boundedASCII(v, maxSessionID); ok {
			c.session(s, "baggage")
		} else {
			c.invalid("baggage.session.id", "format")
		}
	}
	if v := bag["gen_ai.conversation.id"]; v != "" {
		if s, ok := boundedASCII(v, maxSessionID); ok {
			c.conversation(s)
			c.session(s, "baggage")
		} else {
			c.invalid("baggage.gen_ai.conversation.id", "format")
		}
	}
	if v := bag["user.id"]; v != "" {
		if s, ok := boundedText(v, maxUserID); ok {
			set(&d.EndUserID, s)
		} else {
			c.invalid("baggage.user.id", "format")
		}
	}
	if v := bag["gen_ai.agent.name"]; v != "" {
		if s, ok := boundedText(v, maxAgentName); ok {
			if set(&d.AgentName, s) && d.AgentPath == "" {
				// tyk.agent.parent names the parent agent: the pair is a
				// two-level declared path.
				if p := bag["tyk.agent.parent"]; p != "" {
					if path, ok := agentPath("/" + p + "/" + s); ok {
						d.AgentPath = path
					} else {
						c.invalid("baggage.tyk.agent.parent", "format")
					}
				}
			}
		} else {
			c.invalid("baggage.gen_ai.agent.name", "format")
		}
	}
	for k, v := range bag {
		if !x.allow[k] {
			continue
		}
		if d.Baggage == nil {
			d.Baggage = map[string]string{}
		}
		d.Baggage[k] = v
	}
}

// parseBaggage parses W3C baggage headers (members key=value;properties,
// values percent-encoded). An oversized header is dropped whole.
func parseBaggage(values []string, c *collector) map[string]string {
	if len(values) == 0 {
		return nil
	}
	total := 0
	for _, v := range values {
		total += len(v)
	}
	if total > maxBaggageLen {
		c.invalid("baggage", "too_long")
		return nil
	}
	out := map[string]string{}
	for _, v := range values {
		for _, member := range strings.Split(v, ",") {
			member = strings.TrimSpace(member)
			if member == "" {
				continue
			}
			if i := strings.IndexByte(member, ';'); i >= 0 {
				member = member[:i]
			}
			k, val, ok := strings.Cut(member, "=")
			k, val = strings.TrimSpace(k), strings.TrimSpace(val)
			if !ok || k == "" || !token(k) {
				c.invalid("baggage", "format")
				continue
			}
			dec, err := url.PathUnescape(val)
			if err != nil || len(dec) > maxBaggageVal || !utf8.ValidString(dec) {
				c.invalid("baggage", "value")
				continue
			}
			if len(out) >= maxBaggageKeys {
				c.invalid("baggage", "too_many")
				return out
			}
			if _, dup := out[k]; !dup {
				out[k] = dec
			}
		}
	}
	return out
}

// portkeyTraceID uses a W3C-length hex id as is and hashes anything else to
// 16 bytes deterministically, reporting whether it hashed.
func portkeyTraceID(s string) (agenttrace.TraceID, bool) {
	if t, ok := traceID(strings.ToLower(s)); ok {
		return t, false
	}
	sum := sha256.Sum256([]byte("tyk-declared/portkey-trace/v1\x00" + s))
	var t agenttrace.TraceID
	copy(t[:], sum[:16])
	if t.IsZero() {
		t[15] = 1
	}
	return t, true
}

func portkeySpanID(s string) agenttrace.SpanID {
	if id, ok := spanID(strings.ToLower(s)); ok {
		return id
	}
	sum := sha256.Sum256([]byte("tyk-declared/portkey-span/v1\x00" + s))
	var id agenttrace.SpanID
	copy(id[:], sum[:8])
	if id.IsZero() {
		id[7] = 1
	}
	return id
}

func traceID(s string) (agenttrace.TraceID, bool) {
	var t agenttrace.TraceID
	if len(s) != 32 || !lowerHex(s) {
		return t, false
	}
	hex.Decode(t[:], []byte(s))
	return t, !t.IsZero()
}

func spanID(s string) (agenttrace.SpanID, bool) {
	var id agenttrace.SpanID
	if len(s) != 16 || !lowerHex(s) {
		return id, false
	}
	hex.Decode(id[:], []byte(s))
	return id, !id.IsZero()
}

func lowerHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// boundedASCII accepts printable US-ASCII (spaces allowed inside) up to max
// bytes, trimmed.
func boundedASCII(s string, max int) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > max {
		return "", false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return "", false
		}
	}
	return s, true
}

func printableASCII(s string) bool {
	_, ok := boundedASCII(s, len(s))
	return ok
}

// boundedText accepts valid UTF-8 without control characters, up to max
// runes, trimmed.
func boundedText(s string, max int) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || !utf8.ValidString(s) || utf8.RuneCountInString(s) > max {
		return "", false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return s, true
}

// agentPath validates a path-style hierarchy (/root/researcher/summariser):
// 1 to maxPathDepth segments of bounded text without '/'.
func agentPath(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "/") || len(s) > maxAgentPath {
		return "", false
	}
	segs := strings.Split(strings.TrimSuffix(s[1:], "/"), "/")
	if len(segs) == 0 || len(segs) > maxPathDepth {
		return "", false
	}
	for _, seg := range segs {
		if _, ok := boundedText(seg, maxAgentName); !ok || seg != strings.TrimSpace(seg) {
			return "", false
		}
	}
	return "/" + strings.Join(segs, "/"), true
}

// token reports whether s is an RFC 7230 token (a baggage key).
func token(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c <= 0x20 || c >= 0x7f || strings.IndexByte(`"(),/:;<=>?@[\]{}`, c) >= 0 {
			return false
		}
	}
	return true
}
