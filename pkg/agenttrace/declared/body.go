package declared

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Body keys worth a decode, as bits for keysIn.
const (
	keyConversation = 1 << iota
	keySessionID
	keyUser
	keyUserID
)

var bodyKeys = []struct {
	name string
	bit  int
}{{"conversation", keyConversation}, {"session_id", keySessionID}, {"user", keyUser}, {"user_id", keyUserID}}

// keysIn reports which body keys occur as "key" followed by optional
// whitespace and a colon, in one pass that jumps between quotes: a cheap
// test before decoding a large body for a few fields. False positives (a
// key inside a string value) only cost a decode.
func keysIn(body []byte) int {
	found := 0
	for i := 0; i < len(body); {
		j := bytes.IndexByte(body[i:], '"')
		if j < 0 {
			break
		}
		q := i + j
		rest := body[q+1:]
		for _, k := range bodyKeys {
			if len(rest) > len(k.name) && rest[len(k.name)] == '"' && string(rest[:len(k.name)]) == k.name {
				p := q + 2 + len(k.name)
				for p < len(body) && (body[p] == ' ' || body[p] == '\t' || body[p] == '\n' || body[p] == '\r') {
					p++
				}
				if p < len(body) && body[p] == ':' {
					found |= k.bit
				}
			}
		}
		i = q + 1
	}
	return found
}

// parseLLMBody reads the §4.2 priority-4 body fields of an LLM request:
// OpenAI Responses conversation, metadata.session_id and user.
func parseLLMBody(body []byte, keys int, c *collector) {
	if keys&(keyConversation|keySessionID|keyUser) == 0 {
		return
	}
	var b struct {
		Conversation json.RawMessage `json:"conversation"`
		Metadata     json.RawMessage `json:"metadata"`
		User         json.RawMessage `json:"user"`
	}
	if json.Unmarshal(body, &b) != nil {
		return // not JSON or not an object: nothing declared
	}
	d := &c.r.Declared
	if conv := conversationID(b.Conversation); conv != "" {
		if s, ok := boundedASCII(conv, maxSessionID); ok {
			c.conversation(s)
			c.session(s, "body.conversation")
		} else {
			c.invalid("body.conversation", "format")
		}
	}
	if len(b.Metadata) > 0 && b.Metadata[0] == '{' {
		var m struct {
			SessionID json.RawMessage `json:"session_id"`
		}
		if json.Unmarshal(b.Metadata, &m) == nil {
			if v, ok := jsonString(m.SessionID); ok {
				if s, ok := boundedASCII(v, maxSessionID); ok {
					c.session(s, "body.metadata.session_id")
				} else {
					c.invalid("body.metadata.session_id", "format")
				}
			}
		}
	}
	if v, ok := jsonString(b.User); ok {
		if s, ok := boundedText(v, maxUserID); ok {
			set(&d.EndUserID, s)
		} else {
			c.invalid("body.user", "format")
		}
	}
}

// conversationID reads OpenAI's conversation: a string id or {"id": ...}.
func conversationID(raw json.RawMessage) string {
	if s, ok := jsonString(raw); ok {
		return s
	}
	var o struct {
		ID string `json:"id"`
	}
	if len(raw) > 0 && raw[0] == '{' && json.Unmarshal(raw, &o) == nil {
		return o.ID
	}
	return ""
}

func jsonString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || raw[0] != '"' {
		return "", false
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	return s, true
}

// mcpBody is what an MCP JSON-RPC request declares.
type mcpBody struct {
	method, name            string
	traceparent, tracestate string
	clientInfo              string
}

// parseMCPBody reads method, params.name and params._meta (traceparent,
// tracestate, io.modelcontextprotocol/clientInfo) of an MCP request.
// Batches are not classified.
func parseMCPBody(body []byte, c *collector) mcpBody {
	var out mcpBody
	if len(bytes.TrimSpace(body)) == 0 || bytes.TrimSpace(body)[0] != '{' {
		return out
	}
	var b struct {
		Method string `json:"method"`
		Params struct {
			Name string                     `json:"name"`
			URI  string                     `json:"uri"`
			Meta map[string]json.RawMessage `json:"_meta"`
		} `json:"params"`
	}
	if json.Unmarshal(body, &b) != nil {
		return out
	}
	if s, ok := boundedASCII(b.Method, 128); ok {
		out.method = s
	}
	name := b.Params.Name
	if name == "" {
		name = b.Params.URI // resources/read
	}
	if s, ok := boundedText(name, 255); ok {
		out.name = s
	}
	if tp, ok := jsonString(b.Params.Meta["traceparent"]); ok {
		out.traceparent = tp
		out.tracestate, _ = jsonString(b.Params.Meta["tracestate"])
	}
	if raw := b.Params.Meta["io.modelcontextprotocol/clientInfo"]; len(raw) > 0 {
		var ci struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		if json.Unmarshal(raw, &ci) == nil && ci.Name != "" {
			v := ci.Name
			if ci.Version != "" {
				v += "/" + ci.Version
			}
			if s, ok := boundedText(strings.TrimSpace(v), 200); ok {
				out.clientInfo = s
			} else {
				c.invalid("mcp._meta.clientInfo", "format")
			}
		}
	}
	return out
}
