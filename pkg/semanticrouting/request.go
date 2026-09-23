package semanticrouting

import (
	"encoding/json"
	"strings"
)

// MessagesFromOpenAIBody extracts the conversation from an OpenAI-shaped
// request body: the chat "messages" (string content, or the text parts of
// array content), or a completions "prompt" (a string or a list of strings)
// as one user message. Anything it cannot read yields no messages, which the
// router answers with its default route.
func MessagesFromOpenAIBody(body []byte) []Message {
	var req struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		Prompt json.RawMessage `json:"prompt"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil
	}
	var out []Message
	for _, m := range req.Messages {
		if text := contentText(m.Content); text != "" {
			out = append(out, Message{Role: m.Role, Content: text})
		}
	}
	if len(out) == 0 && len(req.Prompt) > 0 {
		if text := contentText(req.Prompt); text != "" {
			out = append(out, Message{Role: "user", Content: text})
		}
	}
	return out
}

// contentText reads a string, a list of strings, or a list of content parts
// ({"type": "text", "text": ...}; other parts are skipped).
func contentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return ""
	}
	var parts []string
	for _, item := range items {
		var str string
		if err := json.Unmarshal(item, &str); err == nil {
			parts = append(parts, str)
			continue
		}
		var part struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(item, &part); err == nil && (part.Type == "text" || part.Type == "input_text") && part.Text != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "\n")
}
