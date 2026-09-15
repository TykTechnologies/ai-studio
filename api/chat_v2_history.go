package api

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/chat_session"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/tmc/langchaingo/llms"
)

// V2Message is one thread message as the v2 chat API exposes it: the shape
// assistant-ui's ThreadMessageLike expects, built server-side from the stored
// llms.MessageContent rows so no client has to re-derive tool calls from text.
type V2Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"` // "user" | "assistant"
	Parts     []V2Part  `json:"parts"`
	CreatedAt time.Time `json:"created_at"`
}

// V2Part is a message content part. Type is one of "text", "tool-call" or
// "data"; only the fields relevant to the type are set.
type V2Part struct {
	Type string `json:"type"`

	// text
	Text string `json:"text,omitempty"`

	// tool-call
	ToolCallID string          `json:"toolCallId,omitempty"`
	ToolName   string          `json:"toolName,omitempty"`
	ArgsText   string          `json:"argsText,omitempty"`
	Args       json.RawMessage `json:"args,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
	IsError    bool            `json:"isError,omitempty"`

	// data
	Name string          `json:"name,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

// materialiseHistory converts stored rows into thread messages. Consecutive
// assistant and tool rows are folded into one assistant message (an LLM turn
// is stored as separate rows for text, tool-call request and tool results),
// tool results are attached to their tool-call part, the [CONTEXT] block in
// front of a human message becomes a context data part, and system rows (the
// system prompt) are omitted.
func materialiseHistory(rows []models.CMessage) []V2Message {
	out := make([]V2Message, 0, len(rows))
	var cur *V2Message // open assistant message being folded into

	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}

	for _, row := range rows {
		var mc llms.MessageContent
		if err := json.Unmarshal(row.Content, &mc); err != nil {
			// Not a MessageContent: expose it as assistant text so nothing is lost.
			flush()
			out = append(out, V2Message{
				ID: strconv.FormatUint(uint64(row.ID), 10), Role: "assistant", CreatedAt: row.CreatedAt,
				Parts: []V2Part{{Type: "text", Text: string(row.Content)}},
			})
			continue
		}

		switch mc.Role {
		case llms.ChatMessageTypeSystem:
			continue

		case llms.ChatMessageTypeHuman, llms.ChatMessageTypeGeneric:
			flush()
			msg := V2Message{ID: strconv.FormatUint(uint64(row.ID), 10), Role: "user", CreatedAt: row.CreatedAt}
			for _, p := range mc.Parts {
				tc, ok := p.(llms.TextContent)
				if !ok {
					continue
				}
				ctxText, userText, has := models.SplitContext(tc.Text)
				if has && ctxText != "" {
					msg.Parts = append(msg.Parts, contextPart(ctxText, contextSourceFor(ctxText)))
				}
				if userText != "" {
					msg.Parts = append(msg.Parts, V2Part{Type: "text", Text: userText})
				}
			}
			out = append(out, msg)

		case llms.ChatMessageTypeAI:
			if cur == nil {
				cur = &V2Message{ID: strconv.FormatUint(uint64(row.ID), 10), Role: "assistant", CreatedAt: row.CreatedAt}
			}
			for _, p := range mc.Parts {
				switch v := p.(type) {
				case llms.TextContent:
					if strings.TrimSpace(v.Text) != "" {
						cur.Parts = append(cur.Parts, V2Part{Type: "text", Text: v.Text})
					}
				case llms.ToolCall:
					part := V2Part{Type: "tool-call", ToolCallID: v.ID}
					if v.FunctionCall != nil {
						part.ToolName = v.FunctionCall.Name
						part.ArgsText = v.FunctionCall.Arguments
						part.Args = jsonObjectOrWrap(v.FunctionCall.Arguments)
					}
					cur.Parts = append(cur.Parts, part)
				}
			}

		case llms.ChatMessageTypeTool:
			if cur == nil {
				cur = &V2Message{ID: strconv.FormatUint(uint64(row.ID), 10), Role: "assistant", CreatedAt: row.CreatedAt}
			}
			for _, p := range mc.Parts {
				resp, ok := p.(llms.ToolCallResponse)
				if !ok {
					continue
				}
				attachToolResult(cur, resp)
			}
		}
	}
	flush()
	return out
}

// attachToolResult sets the result on the matching tool-call part of msg, or
// appends a synthetic tool-call part when the request row is missing.
func attachToolResult(msg *V2Message, resp llms.ToolCallResponse) {
	isErr := strings.HasPrefix(resp.Content, "ERROR: ")
	var result json.RawMessage
	if answer, ok := chat_session.UnwrapClientAnswer(resp.Content); ok {
		// A client tool answer is stored in a labelled envelope for the
		// model; the person sees what they entered.
		result, _ = json.Marshal(answer)
	} else {
		result = jsonValueOrString(resp.Content)
	}
	for i := range msg.Parts {
		if msg.Parts[i].Type == "tool-call" && msg.Parts[i].ToolCallID == resp.ToolCallID {
			msg.Parts[i].Result = result
			msg.Parts[i].IsError = isErr
			return
		}
	}
	msg.Parts = append(msg.Parts, V2Part{
		Type: "tool-call", ToolCallID: resp.ToolCallID, ToolName: resp.Name,
		Args: json.RawMessage(`{}`), Result: result, IsError: isErr,
	})
}

func contextPart(text, source string) V2Part {
	data, _ := json.Marshal(map[string]string{"text": text, "source": source})
	return V2Part{Type: "data", Name: "context", Data: data}
}

// contextSourceFor guesses where a stored context block came from, using the
// fixed phrasing the session writes.
func contextSourceFor(text string) string {
	switch {
	case strings.HasPrefix(text, "The following additional documentation file"):
		return "tool_docs"
	case strings.HasPrefix(text, "File: "):
		return "file"
	default:
		return "rag"
	}
}

// jsonObjectOrWrap returns s when it is a JSON object, otherwise wraps the raw
// text so the client always receives an object for tool arguments.
func jsonObjectOrWrap(s string) json.RawMessage {
	t := strings.TrimSpace(s)
	if strings.HasPrefix(t, "{") && json.Valid([]byte(t)) {
		return json.RawMessage(t)
	}
	if t == "" {
		return json.RawMessage(`{}`)
	}
	b, _ := json.Marshal(map[string]string{"raw": s})
	return b
}

// jsonValueOrString returns s when it is valid JSON, otherwise s as a JSON string.
func jsonValueOrString(s string) json.RawMessage {
	t := strings.TrimSpace(s)
	if t != "" && json.Valid([]byte(t)) && (t[0] == '{' || t[0] == '[') {
		return json.RawMessage(t)
	}
	b, _ := json.Marshal(s)
	return b
}
