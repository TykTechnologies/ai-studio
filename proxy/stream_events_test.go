package proxy

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// Recorded streams, one per wire format and dialect, each saying
// "Hello world" across several events.
var recordedStreams = []struct {
	name, contentType string
	dialect           streamDialect
	body              string
	text              string
}{
	{"openai chat", "text/event-stream", "openai",
		"data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"Hello \"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"world\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
			"data: [DONE]\n\n",
		"Hello world"},
	{"anthropic", "text/event-stream", "anthropic",
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\"}}\n\n" +
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello \"}}\n\n" +
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"world\"}}\n\n" +
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		"Hello world"},
	{"responses", "text/event-stream", "openai_responses",
		"event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"r\"}}\n\n" +
			"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello \"}\n\n" +
			"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"world\"}\n\n" +
			"event: response.output_text.done\ndata: {\"type\":\"response.output_text.done\",\"text\":\"Hello world\"}\n\n" +
			"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\"}}\n\n",
		"Hello world"},
	{"google sse with CRLF", "text/event-stream", "google_ai",
		"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"Hello \"}]}}]}\r\n\r\n" +
			"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"wor\"},{\"text\":\"ld\"}]}}]}\r\n\r\n",
		"Hello world"},
	{"google json array", "application/json; charset=UTF-8", "google_ai",
		"[{\"candidates\": [{\"content\": {\"parts\": [{\"text\": \"Hello \"}],\"role\": \"model\"}}]}\n" +
			",\r\n{\"candidates\": [{\"content\": {\"parts\": [{\"text\": \"wor]ld\\\"}\"}],\"role\": \"model\"}}]}\n" +
			"]",
		"Hello wor]ld\"}"},
	{"ollama ndjson", "application/x-ndjson", "ollama",
		"{\"model\":\"m\",\"message\":{\"role\":\"assistant\",\"content\":\"Hello \"},\"done\":false}\n" +
			"{\"model\":\"m\",\"message\":{\"role\":\"assistant\",\"content\":\"world\"},\"done\":false}\n" +
			"{\"model\":\"m\",\"message\":{\"role\":\"assistant\",\"content\":\"\"},\"done\":true}\n",
		"Hello world"},
	{"ollama generate, no trailing newline", "application/x-ndjson", "ollama",
		"{\"response\":\"Hello \",\"done\":false}\n{\"response\":\"world\",\"done\":true}",
		"Hello world"},
}

// frame feeds body in pieces of the given sizes and returns the events and
// the bytes released (events plus the flushed tail).
func frame(contentType, body string, cut func(i int) int) ([]streamEvent, []byte) {
	f := newEventFramer(contentType)
	var events []streamEvent
	var released []byte
	for i := 0; i < len(body); {
		n := min(cut(i), len(body)-i)
		for _, ev := range f.feed([]byte(body[i : i+n])) {
			events = append(events, ev)
			released = append(released, ev.raw...)
		}
		i += n
	}
	if ev := f.flush(); ev != nil {
		events = append(events, *ev)
		released = append(released, ev.raw...)
	}
	return events, released
}

// Events split at every byte offset frame the same, the text is read in
// full, and the bytes released are the bytes received.
func TestEventFramerEverySplit(t *testing.T) {
	for _, tc := range recordedStreams {
		t.Run(tc.name, func(t *testing.T) {
			whole, _ := frame(tc.contentType, tc.body, func(int) int { return len(tc.body) })
			for split := 1; split < len(tc.body); split++ {
				events, released := frame(tc.contentType, tc.body, func(i int) int {
					if i == 0 {
						return split
					}
					return len(tc.body)
				})
				if string(released) != tc.body {
					t.Fatalf("split %d: released bytes differ", split)
				}
				if len(events) != len(whole) {
					t.Fatalf("split %d: %d events, whole %d", split, len(events), len(whole))
				}
				var text strings.Builder
				for _, ev := range events {
					text.WriteString(tc.dialect.text(ev))
				}
				if text.String() != tc.text {
					t.Fatalf("split %d: text %q, want %q", split, text.String(), tc.text)
				}
			}
			// One byte at a time.
			events, released := frame(tc.contentType, tc.body, func(int) int { return 1 })
			if string(released) != tc.body || len(events) != len(whole) {
				t.Fatalf("byte by byte: %d events, %q", len(events), released)
			}
		})
	}
}

// The wire is read from the first byte when the Content-Type does not say.
func TestEventFramerSniffsTheWire(t *testing.T) {
	for body, want := range map[string]streamWire{
		"  [{\"a\":1}]": wireJSONArray,
		"{\"a\":1}\n":   wireNDJSON,
		"data: x\n\n":   wireSSE,
	} {
		f := newEventFramer("application/octet-stream")
		f.feed([]byte(body))
		if f.wire != want {
			t.Errorf("%q: wire %d, want %d", body, f.wire, want)
		}
	}
}

// A block ends the stream with the vendor's error, in a shape its SDK
// raises: OpenAI an error object then [DONE], Anthropic and Responses an
// error event, Google an error object (closing the array), Ollama an error
// line.
func TestBlockEventShapes(t *testing.T) {
	sse := newEventFramer("text/event-stream")
	sse.feed([]byte("data: {}\n\n"))

	openai := string(streamDialect("openai").blockEvent(sse, "no"))
	if !strings.HasPrefix(openai, "data: {\"error\":") || !strings.HasSuffix(openai, "\n\ndata: [DONE]\n\n") {
		t.Errorf("openai: %q", openai)
	}
	anth := string(streamDialect("anthropic").blockEvent(sse, "no"))
	if !strings.HasPrefix(anth, "event: error\ndata: ") {
		t.Errorf("anthropic: %q", anth)
	}
	_, data := parseSSEFields([]byte(anth))
	var a struct {
		Type  string
		Error struct{ Type, Message string }
	}
	if json.Unmarshal(data, &a) != nil || a.Type != "error" || !strings.Contains(a.Error.Message, "no") {
		t.Errorf("anthropic data %s", data)
	}
	resp := string(streamDialect("openai_responses").blockEvent(sse, "no"))
	_, data = parseSSEFields([]byte(resp))
	var r map[string]any
	if json.Unmarshal(data, &r) != nil || r["type"] != "error" || r["error"] == nil || r["message"] == nil {
		t.Errorf("responses: %q", resp)
	}

	arr := newEventFramer("application/json")
	arr.feed([]byte("[{\"candidates\":[]}\n"))
	closed := append([]byte("[{\"candidates\":[]}\n"), streamDialect("google_ai").blockEvent(arr, "no")...)
	var elems []map[string]any
	if err := json.Unmarshal(closed, &elems); err != nil || len(elems) != 2 || elems[1]["error"] == nil {
		t.Errorf("google array: %s (%v)", closed, err)
	}
	fresh := newEventFramer("application/json")
	if b := streamDialect("google_ai").blockEvent(fresh, "no"); !json.Valid(b) || b[0] != '[' {
		t.Errorf("google array before any byte: %s", b)
	}

	nd := newEventFramer("application/x-ndjson")
	nd.feed([]byte("{}\n"))
	line := streamDialect("ollama").blockEvent(nd, "no")
	if !bytes.HasSuffix(line, []byte("\n")) || !json.Valid(bytes.TrimSpace(line)) || !bytes.Contains(line, []byte(`"error"`)) {
		t.Errorf("ollama: %q", line)
	}
}
