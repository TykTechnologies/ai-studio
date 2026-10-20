package declared

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// Coding-harness extractors (§4.2 priority 5, bead jyx.52). Each reads only
// the signals verified against recorded traffic in
// testdata/agent_tracing/harness/ and only from the verified version
// onwards; an older version is ignored and counted. Harness ids are
// client-supplied, so they may become the conversation id.
//
// They never keep anything else the harness sends: Claude Code's
// metadata.user_id device_id and account_uuid, and Codex's installation_id,
// are stable per-machine identifiers.

var (
	uuidRe      = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	agentIDRe   = regexp.MustCompile(`^[0-9A-Za-z_-]{1,64}$`)
	openCodeSID = regexp.MustCompile(`^ses_[0-9A-Za-z]{8,64}$`)
)

// Minimum verified versions.
var (
	minClaudeCode = [3]int{2, 1, 0}   // verified 2.1.304
	minCodexCLI   = [3]int{0, 160, 0} // verified 0.160.1
	minOpenCode   = [3]int{1, 18, 0}  // verified 1.18.23
)

// maxTurnMetadata bounds the Codex turn-metadata header we parse.
const maxTurnMetadata = 4096

func (x *Extractor) harnesses(h http.Header, ua string, c *collector) {
	r := c.r
	switch {
	case x.harness[HarnessClaudeCode] && harnessUA(ua, "claude-cli/") != "":
		v := harnessUA(ua, "claude-cli/")
		if !x.versionOK(HarnessClaudeCode, v, minClaudeCode, c) {
			break
		}
		r.Harness.Name, r.Harness.Version = HarnessClaudeCode, v
		if s, ok := single(h, "X-Claude-Code-Session-Id", c); ok {
			if uuidRe.MatchString(s) {
				c.session(strings.ToLower(s), HarnessClaudeCode)
			} else {
				c.invalid("x-claude-code-session-id", "format")
			}
		}
		if a, ok := single(h, "X-Claude-Code-Agent-Id", c); ok {
			if agentIDRe.MatchString(a) {
				r.Harness.SubAgentID = a
			} else {
				c.invalid("x-claude-code-agent-id", "format")
			}
		}

	case x.harness[HarnessCodexCLI] && codexClient(h, ua):
		v := harnessVersionAfterSlash(ua)
		if !x.versionOK(HarnessCodexCLI, v, minCodexCLI, c) {
			break
		}
		r.Harness.Name, r.Harness.Version = HarnessCodexCLI, v
		for _, key := range []string{"Session-Id", "Thread-Id"} {
			if s, ok := single(h, key, c); ok {
				if uuidRe.MatchString(s) {
					c.session(strings.ToLower(s), HarnessCodexCLI)
				} else {
					c.invalid(strings.ToLower(key), "format")
				}
			}
		}
		if raw, ok := single(h, "X-Codex-Turn-Metadata", c); ok {
			session, turn, ok := codexTurnMetadata(raw)
			if !ok {
				c.invalid("x-codex-turn-metadata", "format")
				break
			}
			c.session(session, HarnessCodexCLI)
			r.Harness.TurnID = turn
		}

	case x.harness[HarnessOpenCode] && harnessUA(ua, "opencode/") != "":
		v := harnessUA(ua, "opencode/")
		if !x.versionOK(HarnessOpenCode, v, minOpenCode, c) {
			break
		}
		r.Harness.Name, r.Harness.Version = HarnessOpenCode, v
		for _, key := range []string{"X-Session-Id", "Session-Id", "X-Session-Affinity"} {
			if s, ok := single(h, key, c); ok {
				if openCodeSID.MatchString(s) {
					c.session(s, HarnessOpenCode)
				} else {
					c.invalid(strings.ToLower(key), "format")
				}
			}
		}
	}

	for _, key := range x.extraHeaders {
		if v, ok := single(h, key, c); ok {
			if s, ok := boundedASCII(v, maxSessionID); ok {
				c.session(s, "header:"+strings.ToLower(key))
			} else {
				c.invalid(strings.ToLower(key), "format")
			}
		}
	}
}

func (x *Extractor) versionOK(name, v string, min [3]int, c *collector) bool {
	got, ok := parseVersion(v)
	if !ok || less(got, min) {
		c.invalid(name, "unverified_version")
		return false
	}
	return true
}

// harnessUA returns the version after prefix when the User-Agent starts
// with it ("claude-cli/2.1.304 (external, sdk-cli)" -> "2.1.304").
func harnessUA(ua, prefix string) string {
	if !strings.HasPrefix(ua, prefix) {
		return ""
	}
	v := ua[len(prefix):]
	if i := strings.IndexAny(v, " ;("); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return "unknown"
	}
	return v
}

// codexClient recognises Codex CLI by its Originator header or User-Agent
// product (codex_exec, codex_cli_rs, ...).
func codexClient(h http.Header, ua string) bool {
	return strings.HasPrefix(h.Get("Originator"), "codex") || strings.HasPrefix(ua, "codex")
}

func harnessVersionAfterSlash(ua string) string {
	i := strings.IndexByte(ua, '/')
	if i < 0 {
		return ""
	}
	return harnessUA(ua, ua[:i+1])
}

func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return out, false
	}
	for i, p := range parts {
		// Drop a pre-release or build suffix: 2.1.304-beta.1
		if j := strings.IndexAny(p, "-+"); j >= 0 {
			p = p[:j]
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func less(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// claudeMetadataSession reads the session_id inside Claude Code's
// metadata.user_id, a JSON-encoded string
// {"device_id":...,"account_uuid":...,"session_id":"<uuid>"}. Only
// session_id is read; the other members are never kept.
func claudeMetadataSession(body []byte, keys int, c *collector) string {
	if keys&keyUserID == 0 {
		return ""
	}
	var b struct {
		Metadata struct {
			UserID json.RawMessage `json:"user_id"`
		} `json:"metadata"`
	}
	if json.Unmarshal(body, &b) != nil {
		return ""
	}
	inner, ok := jsonString(b.Metadata.UserID)
	if !ok || !strings.HasPrefix(strings.TrimSpace(inner), "{") {
		return "" // another shape (e.g. the legacy user_..._session_...): unverified, ignored
	}
	var u struct {
		SessionID string `json:"session_id"`
	}
	if json.Unmarshal([]byte(inner), &u) != nil || !uuidRe.MatchString(u.SessionID) {
		c.invalid("claude_code.metadata.user_id", "format")
		return ""
	}
	return strings.ToLower(u.SessionID)
}

// codexTurnMetadata reads session_id and turn_id from Codex's
// X-Codex-Turn-Metadata JSON. Nothing else in it is kept (installation_id
// is a per-install identifier).
func codexTurnMetadata(raw string) (session, turn string, ok bool) {
	if len(raw) > maxTurnMetadata {
		return "", "", false
	}
	var m struct {
		SessionID string `json:"session_id"`
		TurnID    string `json:"turn_id"`
	}
	if json.Unmarshal([]byte(raw), &m) != nil {
		return "", "", false
	}
	if m.SessionID != "" && !uuidRe.MatchString(m.SessionID) || m.TurnID != "" && !uuidRe.MatchString(m.TurnID) {
		return "", "", false
	}
	return strings.ToLower(m.SessionID), strings.ToLower(m.TurnID), true
}
