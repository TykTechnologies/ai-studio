package extract

import (
	"bytes"
	"encoding/json"
	"strings"
)

// StreamText assembles the assistant's text from a streamed response body:
// SSE events of OpenAI Chat Completions (choices[0].delta.content), the
// Responses API (response.output_text.delta), Anthropic Messages
// (content_block_delta text_delta) and Gemini (candidates[0].content.parts),
// or NDJSON lines in Ollama's shape (message.content). ok reports that data
// is such a stream, even when it carries no text (a tool-call-only step).
// The transcript preview and the Markdown export use it for a part that is
// not one JSON document (jyx.162). It runs over the stored, already
// redacted part, so it never sees more than was kept.
func StreamText(data []byte) (text string, ok bool) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return "", false
	}
	var b strings.Builder
	events := 0
	sse := bytes.HasPrefix(data, []byte("data:")) || bytes.HasPrefix(data, []byte("event:")) ||
		bytes.HasPrefix(data, []byte("id:")) || bytes.HasPrefix(data, []byte(":"))
	for len(data) > 0 {
		var line []byte
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			line, data = data[:i], data[i+1:]
		} else {
			line, data = data, nil
		}
		line = bytes.TrimSpace(line)
		if sse {
			rest, found := bytes.CutPrefix(line, []byte("data:"))
			if !found {
				continue // event:, id:, comments and blank separators
			}
			line = bytes.TrimSpace(rest)
		}
		if len(line) == 0 || line[0] != '{' {
			continue // [DONE] and anything that is not an event object
		}
		var ev streamEvent
		if json.Unmarshal(line, &ev) != nil {
			continue // a frame cut by the capture limit
		}
		events++
		b.WriteString(ev.text())
	}
	if !sse && events < 2 {
		return "", false // one JSON document is not a stream
	}
	return b.String(), events > 0
}

// streamEvent holds the fields of every supported dialect's text event.
type streamEvent struct {
	Type    string `json:"type"`
	Choices []struct {
		Delta struct {
			Content *string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Delta      json.RawMessage `json:"delta"`
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Message *struct {
		Content string `json:"content"`
	} `json:"message"`
}

func (ev *streamEvent) text() string {
	switch {
	case len(ev.Choices) > 0: // OpenAI Chat Completions
		if c := ev.Choices[0].Delta.Content; c != nil {
			return *c
		}
	case ev.Type == "response.output_text.delta": // Responses API
		var s string
		if json.Unmarshal(ev.Delta, &s) == nil {
			return s
		}
	case ev.Type == "content_block_delta": // Anthropic Messages
		var d struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(ev.Delta, &d) == nil && d.Type == "text_delta" {
			return d.Text
		}
	case len(ev.Candidates) > 0: // Gemini
		var s strings.Builder
		for _, p := range ev.Candidates[0].Content.Parts {
			s.WriteString(p.Text)
		}
		return s.String()
	case ev.Message != nil: // Ollama NDJSON
		return ev.Message.Content
	}
	return ""
}
