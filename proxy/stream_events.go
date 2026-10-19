package proxy

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream/eventstreamapi"
)

// Streamed responses reach the gateway in reads of any size, which cut
// events anywhere. When an LLM has response filters, the streaming loop
// frames the body into whole events (eventFramer), reads each event's text
// for the filters, and releases only whole events to the client: the
// filters judge an event before its bytes leave, and a block ends the
// stream with the vendor's own error event (streamDialect.blockEvent),
// which the vendors' SDKs raise. Without response filters the loop relays
// reads untouched, as it always has.

// bedrockInvokeStream reports a Bedrock InvokeModelWithResponseStream path,
// whose binary event stream the gateway does not read.
func bedrockInvokeStream(path string) bool {
	return strings.Contains(strings.ToLower(path), "invoke-with-response-stream")
}

// streamWire is how a streamed body is framed.
type streamWire int

const (
	wireUnknown streamWire = iota
	// wireSSE: server-sent events, separated by a blank line (OpenAI,
	// Anthropic, Responses, Google with alt=sse).
	wireSSE
	// wireJSONArray: one JSON array whose elements arrive one by one
	// (Google streamGenerateContent without alt=sse).
	wireJSONArray
	// wireNDJSON: one JSON object per line (Ollama).
	wireNDJSON
	// wireEventStream: AWS binary event stream messages (Bedrock
	// converse-stream), each led by its total length.
	wireEventStream
)

// maxEventStreamMessage bounds one event stream message; a larger length
// prefix is a corrupt stream, relayed as it is without further framing.
const maxEventStreamMessage = 16 << 20

// streamEvent is one whole event: its bytes as received, and its payload
// (an SSE event's data, or a JSON element or line).
type streamEvent struct {
	raw  []byte
	name string
	data []byte
}

// eventFramer splits a streamed body into whole events.
type eventFramer struct {
	wire streamWire
	buf  []byte

	// JSON array state: scan position, nesting depth, string state, where
	// the current element began, whether the array has opened, and whether
	// an element has been released.
	scan        int
	depth       int
	inString    bool
	escaped     bool
	elemStart   int
	opened      bool
	anyReleased bool
	// jsonBody: the Content-Type is JSON, so a body whose first byte has
	// not arrived is a JSON array (Google without alt=sse).
	jsonBody bool
}

// newEventFramer frames a body of the given Content-Type; an unknown or
// generic type is decided by the body's first byte.
func newEventFramer(contentType string) *eventFramer {
	f := &eventFramer{elemStart: -1}
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "vnd.amazon.eventstream"):
		f.wire = wireEventStream
	case strings.Contains(ct, "text/event-stream"):
		f.wire = wireSSE
	case strings.Contains(ct, "ndjson"):
		f.wire = wireNDJSON
	case strings.Contains(ct, "json"):
		f.jsonBody = true
	}
	return f
}

// feed adds b and returns the events it completed.
func (f *eventFramer) feed(b []byte) []streamEvent {
	f.buf = append(f.buf, b...)
	if f.wire == wireUnknown {
		trimmed := bytes.TrimLeft(f.buf, " \t\r\n")
		if len(trimmed) == 0 {
			return nil
		}
		switch trimmed[0] {
		case '[':
			f.wire = wireJSONArray
		case '{':
			f.wire = wireNDJSON
		default:
			f.wire = wireSSE
		}
	}
	switch f.wire {
	case wireSSE:
		return f.sseEvents()
	case wireNDJSON:
		return f.lineEvents()
	case wireEventStream:
		return f.eventStreamEvents()
	}
	return f.arrayEvents()
}

// eventStreamEvents releases every complete event stream message: its name
// is its :event-type header ("exception:<type>" for an exception), its data
// the payload.
func (f *eventFramer) eventStreamEvents() []streamEvent {
	var out []streamEvent
	for len(f.buf) >= 12 {
		total := int(binary.BigEndian.Uint32(f.buf[:4]))
		if total < 16 || total > maxEventStreamMessage {
			// Not a stream the gateway can frame: no more events; the rest
			// is relayed by flush.
			return out
		}
		if len(f.buf) < total {
			return out
		}
		raw := f.buf[:total]
		ev := streamEvent{raw: raw}
		if msg, err := eventstream.NewDecoder().Decode(bytes.NewReader(raw), nil); err == nil {
			ev.data = msg.Payload
			if h := msg.Headers.Get(":message-type"); h != nil && h.String() == "exception" {
				ev.name = "exception:"
				if t := msg.Headers.Get(":exception-type"); t != nil {
					ev.name += t.String()
				}
			} else if t := msg.Headers.Get(":event-type"); t != nil {
				ev.name = t.String()
			}
		}
		out = append(out, ev)
		f.buf = f.buf[total:]
	}
	return out
}

// flush returns what is left at the end of the body as one last event
// (a final event with no terminator), or nil.
func (f *eventFramer) flush() *streamEvent {
	if len(f.buf) == 0 {
		return nil
	}
	ev := &streamEvent{raw: f.buf}
	switch f.wire {
	case wireSSE:
		ev.name, ev.data = parseSSEFields(f.buf)
	case wireNDJSON:
		ev.data = bytes.TrimSpace(f.buf)
	}
	f.buf = nil
	return ev
}

// sseEvents releases every event ended by a blank line.
func (f *eventFramer) sseEvents() []streamEvent {
	var out []streamEvent
	for {
		end := sseEventEnd(f.buf)
		if end < 0 {
			return out
		}
		raw := f.buf[:end]
		name, data := parseSSEFields(raw)
		out = append(out, streamEvent{raw: raw, name: name, data: data})
		f.buf = f.buf[end:]
	}
}

// parseSSEFields reads an event's name and its data lines joined with "\n".
func parseSSEFields(raw []byte) (string, []byte) {
	var name string
	var data [][]byte
	for _, line := range bytes.Split(raw, []byte("\n")) {
		line = bytes.TrimRight(line, "\r")
		switch {
		case bytes.HasPrefix(line, []byte("data:")):
			data = append(data, bytes.TrimPrefix(bytes.TrimPrefix(line, []byte("data:")), []byte(" ")))
		case bytes.HasPrefix(line, []byte("event:")):
			name = strings.TrimSpace(string(bytes.TrimPrefix(line, []byte("event:"))))
		}
	}
	return name, bytes.Join(data, []byte("\n"))
}

// lineEvents releases every complete line.
func (f *eventFramer) lineEvents() []streamEvent {
	var out []streamEvent
	for {
		i := bytes.IndexByte(f.buf, '\n')
		if i < 0 {
			return out
		}
		raw := f.buf[:i+1]
		out = append(out, streamEvent{raw: raw, data: bytes.TrimSpace(raw)})
		f.buf = f.buf[i+1:]
	}
}

// arrayEvents releases every complete element of the array, each with the
// separators before it.
func (f *eventFramer) arrayEvents() []streamEvent {
	var out []streamEvent
	for ; f.scan < len(f.buf); f.scan++ {
		c := f.buf[f.scan]
		if f.inString {
			switch {
			case f.escaped:
				f.escaped = false
			case c == '\\':
				f.escaped = true
			case c == '"':
				f.inString = false
			}
			continue
		}
		switch c {
		case '"':
			if f.depth > 0 {
				f.inString = true
			}
		case '[', '{':
			if c == '[' && f.depth == 0 && !f.opened {
				f.opened = true
				continue
			}
			if f.depth == 0 {
				f.elemStart = f.scan
			}
			f.depth++
		case ']', '}':
			if f.depth == 0 {
				continue // the array's closing bracket stays in the tail
			}
			f.depth--
			if f.depth == 0 && f.elemStart >= 0 {
				end := f.scan + 1
				out = append(out, streamEvent{raw: f.buf[:end], data: f.buf[f.elemStart:end]})
				f.buf = f.buf[end:]
				f.scan, f.elemStart = -1, -1
				f.anyReleased = true
			}
		}
	}
	return out
}

// streamDialect is the response format a stream is read in: a request
// filter format name (openai, openai_responses, anthropic, google_ai,
// vertex, ollama).
type streamDialect string

// text is the response text an event carries.
func (d streamDialect) text(ev streamEvent) string {
	if len(ev.data) == 0 || ev.data[0] != '{' {
		return ""
	}
	if d == "bedrock" {
		if ev.name != "contentBlockDelta" {
			return ""
		}
		var e struct {
			Delta struct {
				Text string `json:"text"`
			} `json:"delta"`
		}
		_ = json.Unmarshal(ev.data, &e)
		return e.Delta.Text
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(ev.data, &m) != nil {
		return ""
	}
	str := func(raw json.RawMessage) string {
		var s string
		_ = json.Unmarshal(raw, &s)
		return s
	}
	// Responses events name themselves in type, whatever the vendor path.
	if t := str(m["type"]); strings.HasPrefix(t, "response.") {
		switch t {
		case "response.output_text.delta", "response.refusal.delta":
			return str(m["delta"])
		}
		return ""
	}
	switch d {
	case "anthropic":
		if str(m["type"]) != "content_block_delta" {
			return ""
		}
		var delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		_ = json.Unmarshal(m["delta"], &delta)
		if delta.Type == "text_delta" {
			return delta.Text
		}
		return ""
	case "google_ai", "vertex":
		var c struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text    string `json:"text"`
						Thought bool   `json:"thought"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		_ = json.Unmarshal(ev.data, &c)
		var b strings.Builder
		for _, cand := range c.Candidates {
			for _, p := range cand.Content.Parts {
				if !p.Thought {
					b.WriteString(p.Text)
				}
			}
		}
		return b.String()
	}
	// OpenAI chat completions, and Ollama (native: message.content or
	// response; its OpenAI-compatible endpoint streams chat chunks).
	if raw, ok := m["choices"]; ok {
		var choices []struct {
			Delta struct {
				Content string `json:"content"`
				Refusal string `json:"refusal"`
			} `json:"delta"`
		}
		_ = json.Unmarshal(raw, &choices)
		var b strings.Builder
		for _, ch := range choices {
			b.WriteString(ch.Delta.Content)
			b.WriteString(ch.Delta.Refusal)
		}
		return b.String()
	}
	if raw, ok := m["message"]; ok {
		var msg struct {
			Content string `json:"content"`
		}
		_ = json.Unmarshal(raw, &msg)
		return msg.Content
	}
	return str(m["response"])
}

// blockEvent is the event that ends a blocked stream, in the vendor's own
// error shape for the wire, so the vendor's SDK raises it: OpenAI an error
// object then [DONE], Anthropic an error event, Responses an error event,
// Google an error object (closing the array on the JSON array wire),
// Ollama an error line. The message is the filter's block message.
func (d streamDialect) blockEvent(f *eventFramer, msg string) []byte {
	text := "Response blocked by filter: " + msg
	enc := func(v any) []byte {
		b, _ := json.Marshal(v)
		return b
	}
	wire := f.wire
	if wire == wireUnknown && f.jsonBody {
		wire = wireJSONArray
	}
	switch wire {
	case wireEventStream:
		// A validationException, which the AWS SDKs raise.
		var b bytes.Buffer
		_ = eventstream.NewEncoder().Encode(&b, eventstream.Message{
			Headers: eventstream.Headers{
				{Name: eventstreamapi.MessageTypeHeader, Value: eventstream.StringValue(eventstreamapi.ExceptionMessageType)},
				{Name: eventstreamapi.ExceptionTypeHeader, Value: eventstream.StringValue("validationException")},
				{Name: eventstreamapi.ContentTypeHeader, Value: eventstream.StringValue("application/json")},
			},
			Payload: enc(map[string]string{"message": text}),
		})
		return b.Bytes()
	case wireNDJSON:
		return append(enc(map[string]string{"error": text}), '\n')
	case wireJSONArray:
		obj := enc(map[string]any{"error": map[string]any{"code": 400, "message": text, "status": "FAILED_PRECONDITION"}})
		// The closing bracket, once seen, is in the unreleased tail, which a
		// blocked stream never sends: the error closes the array instead.
		switch {
		case !f.opened:
			return append(append([]byte("["), obj...), ']')
		case f.anyReleased:
			return append(append([]byte(","), obj...), '\n', ']')
		}
		return append(obj, ']')
	}
	switch d {
	case "anthropic":
		return []byte("event: error\ndata: " + string(enc(map[string]any{
			"type": "error", "error": map[string]string{"type": "invalid_request_error", "message": text},
		})) + "\n\n")
	case "openai_responses":
		return []byte("event: error\ndata: " + string(enc(map[string]any{
			"type": "error", "code": "policy_violation", "message": text, "param": nil,
			"error": map[string]string{"type": "policy_violation", "code": "policy_violation", "message": text},
		})) + "\n\n")
	case "google_ai", "vertex":
		return []byte("data: " + string(enc(map[string]any{
			"error": map[string]any{"code": 400, "message": text, "status": "FAILED_PRECONDITION"},
		})) + "\n\n")
	}
	return []byte("data: " + string(enc(map[string]any{
		"error": map[string]string{"message": text, "type": "policy_violation", "code": "policy_violation"},
	})) + "\n\ndata: [DONE]\n\n")
}
