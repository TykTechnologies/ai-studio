package proxy

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace/extract"
)

// Agent tracing reads a streamed response's correlation extracts (spec §4.5
// streaming): the tool calls with their ids, names and argument
// fingerprints, the response id and the finish reasons, and whether the
// stream reached its terminal event.
//
// The streaming loop already keeps every byte it relays (fullResponse) for
// analytics, so the capture runs on that copy from the analytics goroutine,
// after the client has the stream: it adds nothing to forwarding or to the
// time to first token, and a slow parse can never hold a chunk back. The
// capturer is incremental (Feed, then Finish) so it frames events across any
// read boundary.
//
// The result matches extract.ParseResponse on the equivalent non-streamed
// body: the same calls, ids, names and argument fingerprints, with provider
// server tools apart.

// streamCapture reassembles one streamed response.
type streamCapture struct {
	dialect streamDialect
	framer  *eventFramer
	fp      agenttrace.Fingerprinter

	maxArgs  int // capture.max_stream_buffer_bytes: argument bytes kept per response
	argBytes int

	id       string
	finishes []string
	complete bool

	calls []*streamCall
	byKey map[string]*streamCall
}

// streamCall is one tool call being assembled.
type streamCall struct {
	id, name string
	args     strings.Builder
	server   bool
	open     bool
	// gotDelta: argument deltas replace an initial placeholder input.
	gotDelta bool
	// unavailable: the argument bytes went over the cap, or the stream ended
	// with the call still open; the call keeps its id and name.
	unavailable bool
}

// captureStreamedResponse captures a relayed stream in one call, for the
// analytics goroutine: body is the stream as relayed (the loop's
// fullResponse), contentType the upstream's Content-Type (needed for
// Bedrock's binary event stream; SSE and NDJSON are told by their first
// byte), maxArgs AGENT_TRACING_MAX_STREAM_BUFFER_BYTES.
func captureStreamedResponse(r *http.Request, llm *models.LLM, contentType string, body []byte, maxArgs int, fp agenttrace.Fingerprinter) streamCaptureResult {
	c := newStreamCapture(contentType, streamDialect(requestFilterFormatFor(r, llm)), maxArgs, fp)
	c.Feed(body)
	return c.Finish()
}

// newStreamCapture starts a capture of a body of contentType in dialect (a
// request filter format: openai, openai_responses, anthropic, google_ai,
// vertex, ollama, bedrock).
func newStreamCapture(contentType string, d streamDialect, maxArgs int, fp agenttrace.Fingerprinter) *streamCapture {
	if maxArgs <= 0 {
		maxArgs = 1 << 20
	}
	return &streamCapture{dialect: d, framer: newEventFramer(contentType), fp: fp, maxArgs: maxArgs, byKey: map[string]*streamCall{}}
}

// Feed takes the next bytes of the body, cut anywhere.
func (c *streamCapture) Feed(b []byte) {
	for _, ev := range c.framer.feed(b) {
		c.event(ev)
	}
}

// streamCaptureResult is a finished capture.
type streamCaptureResult struct {
	// Extracts has Stream set, StreamIncomplete when no terminal event
	// arrived, the client tool calls, the response id and finish reasons.
	Extracts agenttrace.Extracts
	// ServerToolCalls are provider-hosted tools (span events, §4.4 R2).
	ServerToolCalls []extract.ToolCall
	// ArgsOverflow: some argument bytes were over the cap.
	ArgsOverflow bool
}

// Finish ends the body and returns what was captured.
func (c *streamCapture) Finish() streamCaptureResult {
	if ev := c.framer.flush(); ev != nil {
		c.event(*ev)
	}
	var res streamCaptureResult
	x := &res.Extracts
	x.Stream = true
	x.StreamIncomplete = !c.complete
	x.ResponseID = boundStreamID(c.id)
	x.FinishReasons = c.finishes
	if len(x.FinishReasons) > extract.MaxFinishReasons {
		x.FinishReasons = x.FinishReasons[:extract.MaxFinishReasons]
	}
	for i, f := range x.FinishReasons {
		x.FinishReasons[i] = boundStreamText(f, 64)
	}
	if !c.complete {
		// §4.5: a stream that ends without its terminal event is an error,
		// with the partial output kept.
		x.FinishReasons = append(x.FinishReasons, "error")
	}
	calls := c.calls
	if len(calls) > extract.MaxToolCalls {
		calls = calls[:extract.MaxToolCalls]
	}
	for _, sc := range calls {
		sc.id, sc.name = boundStreamID(sc.id), boundStreamText(sc.name, extract.MaxNameBytes)
		unavailable := sc.unavailable || (!c.complete && sc.open)
		var fp agenttrace.Fingerprint
		if !unavailable {
			args := sc.args.String()
			if args == "" {
				args = "{}"
			}
			if f, err := extract.ArgsFingerprint(c.fp, []byte(args)); err == nil {
				fp = f
			}
		}
		if sc.server {
			res.ServerToolCalls = append(res.ServerToolCalls, extract.ToolCall{ID: sc.id, Name: sc.name, ArgsFP: fp, Server: true})
			continue
		}
		x.ToolCalls = append(x.ToolCalls, agenttrace.ToolCallRequest{ID: sc.id, Name: sc.name, ArgsFP: fp, ArgsUnavailable: unavailable})
		res.ArgsOverflow = res.ArgsOverflow || sc.unavailable
	}
	return res
}

// boundStreamID and boundStreamText apply extract's bounds (jyx.62) to what a
// stream contributes: an over-long id is dropped, not cut (two cut ids could
// collide into a false link); text is cut on a rune boundary.
func boundStreamID(s string) string {
	if len(s) > extract.MaxIDBytes {
		return ""
	}
	return s
}

func boundStreamText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// open starts (or finds) the call under key.
func (c *streamCapture) open(key, id, name string, server bool) *streamCall {
	sc := c.byKey[key]
	if sc == nil {
		sc = &streamCall{open: true, server: server}
		c.byKey[key] = sc
		c.calls = append(c.calls, sc)
	}
	if id != "" && sc.id == "" {
		sc.id = id
	}
	if name != "" && sc.name == "" {
		sc.name = name
	}
	return sc
}

// appendArgs adds argument bytes to a call, within the response's cap.
func (c *streamCapture) appendArgs(sc *streamCall, s string) {
	if sc == nil || s == "" || sc.unavailable {
		return
	}
	if c.argBytes+len(s) > c.maxArgs {
		sc.unavailable = true
		sc.args.Reset()
		return
	}
	c.argBytes += len(s)
	sc.args.WriteString(s)
}

// setArgs replaces a call's arguments with its complete arguments.
func (c *streamCapture) setArgs(sc *streamCall, s string) {
	if sc == nil || sc.unavailable {
		return
	}
	c.argBytes -= sc.args.Len()
	sc.args.Reset()
	c.appendArgs(sc, s)
}

func (c *streamCapture) finish(reason string) {
	if reason != "" {
		c.finishes = append(c.finishes, reason)
	}
}

func (c *streamCapture) setID(id string) {
	if c.id == "" {
		c.id = id
	}
}

func (c *streamCapture) event(ev streamEvent) {
	if c.dialect == "bedrock" {
		c.bedrock(ev)
		return
	}
	data := strings.TrimSpace(string(ev.data))
	if data == "[DONE]" {
		c.complete = true
		return
	}
	if !strings.HasPrefix(data, "{") || c.skippable(data) {
		return
	}
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(data), &m) != nil {
		return
	}
	typ := jsonStr(m["type"])
	switch {
	case strings.HasPrefix(typ, "response."):
		c.responses(typ, m)
	case c.dialect == "anthropic":
		c.anthropic(typ, m)
	case c.dialect == "google_ai" || c.dialect == "vertex":
		c.google(m)
	default:
		c.openAI(m)
	}
}

// skippable reports, without decoding, an event that carries only text:
// most of a stream is text deltas, and decoding each costs more than the
// rest of the capture together.
func (c *streamCapture) skippable(data string) bool {
	// Anthropic and Responses events lead with their type (compact JSON);
	// any other spelling is simply decoded.
	if rest, ok := strings.CutPrefix(data, `{"type":"`); ok {
		i := strings.IndexByte(rest, '"')
		if i < 0 {
			return false
		}
		switch t := rest[:i]; {
		case t == "content_block_delta":
			return !strings.Contains(rest, `"input_json_delta"`) // a text, thinking or signature delta
		case strings.HasPrefix(t, "response.") && strings.HasSuffix(t, ".delta"):
			return t != "response.function_call_arguments.delta" && t != "response.custom_tool_call_input.delta" &&
				t != "response.mcp_call_arguments.delta"
		}
		return false
	}
	if c.dialect == "openai" || c.dialect == "ollama" {
		// A Chat Completions text chunk: no call, no finish reason, and the
		// id is already known.
		if c.id == "" || strings.Contains(data, `"tool_calls"`) || strings.Contains(data, `"function_call"`) ||
			strings.Contains(data, `"done":true`) || strings.Contains(data, `"done": true`) {
			return false
		}
		i := strings.Index(data, `"finish_reason"`)
		return i < 0 || strings.HasPrefix(strings.TrimLeft(data[i+len(`"finish_reason"`):], ": "), "null")
	}
	return false
}

// openAI reads a Chat Completions chunk, or an Ollama native line.
func (c *streamCapture) openAI(m map[string]json.RawMessage) {
	c.setID(jsonStr(m["id"]))
	if raw, ok := m["choices"]; ok {
		var choices []struct {
			Index int `json:"index"`
			Delta struct {
				ToolCalls []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
				FunctionCall *struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function_call"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		}
		_ = json.Unmarshal(raw, &choices)
		for _, ch := range choices {
			for _, tc := range ch.Delta.ToolCalls {
				sc := c.open("c"+strconv.Itoa(ch.Index)+"."+strconv.Itoa(tc.Index), tc.ID, tc.Function.Name, false)
				c.appendArgs(sc, tc.Function.Arguments)
			}
			if fc := ch.Delta.FunctionCall; fc != nil {
				sc := c.open("c"+strconv.Itoa(ch.Index)+".fn", "", fc.Name, false)
				c.appendArgs(sc, fc.Arguments)
			}
			if ch.FinishReason != nil && *ch.FinishReason != "" {
				c.finish(*ch.FinishReason)
				c.closeAll()
			}
		}
		return
	}
	// Ollama: message.tool_calls are whole calls; done ends the stream.
	if raw, ok := m["message"]; ok {
		var msg struct {
			ToolCalls []struct {
				Function struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		}
		_ = json.Unmarshal(raw, &msg)
		for _, tc := range msg.ToolCalls {
			sc := c.open("o"+strconv.Itoa(len(c.calls)), "", tc.Function.Name, false)
			c.setArgs(sc, string(tc.Function.Arguments))
			sc.open = false
		}
	}
	if done, _ := strconv.ParseBool(string(m["done"])); done {
		c.finish(jsonStr(m["done_reason"]))
		c.complete = true
	}
}

// responsesServerItems are Responses output items the provider runs itself
// (as extract classifies them).
var responsesServerItems = map[string]bool{
	"web_search_call": true, "file_search_call": true, "code_interpreter_call": true,
	"image_generation_call": true, "mcp_call": true, "mcp_list_tools": true, "mcp_approval_request": true,
}

// responses reads an OpenAI Responses stream event.
func (c *streamCapture) responses(typ string, m map[string]json.RawMessage) {
	type item struct {
		Type      string          `json:"type"`
		ID        string          `json:"id"`
		CallID    string          `json:"call_id"`
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
		Input     json.RawMessage `json:"input"`
		Action    json.RawMessage `json:"action"`
	}
	readItem := func() (item, *streamCall) {
		var it item
		_ = json.Unmarshal(m["item"], &it)
		id := firstNonEmpty(it.CallID, it.ID)
		switch {
		case it.Type == "function_call" || it.Type == "custom_tool_call":
			return it, c.open(it.ID, id, it.Name, false)
		case it.Type == "computer_call" || it.Type == "local_shell_call":
			return it, c.open(it.ID, id, it.Type, false)
		case responsesServerItems[it.Type]:
			name := it.Name
			if name == "" {
				name = strings.TrimSuffix(it.Type, "_call")
			}
			return it, c.open(it.ID, id, name, true)
		}
		return it, nil
	}
	switch typ {
	case "response.created", "response.in_progress":
		c.responseHeader(m["response"], false)
	case "response.output_item.added":
		readItem()
	case "response.function_call_arguments.delta", "response.custom_tool_call_input.delta", "response.mcp_call_arguments.delta":
		if sc := c.byKey[jsonStr(m["item_id"])]; sc != nil {
			sc.gotDelta = true
			c.appendArgs(sc, jsonStr(m["delta"]))
		}
	case "response.output_item.done":
		it, sc := readItem()
		if sc == nil {
			return
		}
		var whole json.RawMessage
		switch it.Type {
		case "custom_tool_call":
			whole = it.Input
		case "computer_call", "local_shell_call":
			whole = it.Action
		default:
			whole = it.Arguments
		}
		if present := len(whole) > 0 && string(whole) != "null"; present {
			if s, ok := jsonString(whole); ok {
				c.setArgs(sc, s)
			} else {
				c.setArgs(sc, string(whole))
			}
		}
		sc.open = false
	case "response.completed", "response.incomplete", "response.failed":
		c.responseHeader(m["response"], true)
		c.complete = true
		c.closeAll()
	}
}

// responseHeader reads a Responses response object's id and, at the end,
// its status and incomplete reason (as extract does for the whole body).
func (c *streamCapture) responseHeader(raw json.RawMessage, final bool) {
	var r struct {
		ID                string `json:"id"`
		Status            string `json:"status"`
		IncompleteDetails *struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return
	}
	c.setID(r.ID)
	if final {
		c.finish(r.Status)
		if r.IncompleteDetails != nil {
			c.finish(r.IncompleteDetails.Reason)
		}
	}
}

// anthropic reads an Anthropic Messages stream event.
func (c *streamCapture) anthropic(typ string, m map[string]json.RawMessage) {
	key := func() string { return "b" + strings.TrimSpace(string(m["index"])) }
	switch typ {
	case "message_start":
		var msg struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(m["message"], &msg)
		c.setID(msg.ID)
	case "content_block_start":
		var b struct {
			Type  string          `json:"type"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		}
		_ = json.Unmarshal(m["content_block"], &b)
		switch b.Type {
		case "tool_use", "server_tool_use", "mcp_tool_use":
			sc := c.open(key(), b.ID, b.Name, b.Type != "tool_use")
			// The start carries a placeholder input ({}); deltas replace it.
			if len(b.Input) > 0 && string(b.Input) != "null" {
				c.setArgs(sc, string(b.Input))
			}
		}
	case "content_block_delta":
		var dl struct {
			Type        string `json:"type"`
			PartialJSON string `json:"partial_json"`
		}
		_ = json.Unmarshal(m["delta"], &dl)
		if dl.Type != "input_json_delta" {
			return
		}
		if sc := c.byKey[key()]; sc != nil {
			if !sc.gotDelta {
				sc.gotDelta = true
				c.setArgs(sc, "")
			}
			c.appendArgs(sc, dl.PartialJSON)
		}
	case "content_block_stop":
		if sc := c.byKey[key()]; sc != nil {
			sc.open = false
		}
	case "message_delta":
		var d struct {
			StopReason string `json:"stop_reason"`
		}
		_ = json.Unmarshal(m["delta"], &d)
		c.finish(d.StopReason)
	case "message_stop":
		c.complete = true
		c.closeAll()
	}
}

// google reads a Gemini / Vertex chunk: function calls arrive whole. The
// stream has no terminal event; a finish reason marks the end.
func (c *streamCapture) google(m map[string]json.RawMessage) {
	type functionCall struct {
		ID   string          `json:"id"`
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
	}
	var b struct {
		Candidates []struct {
			FinishReason string `json:"finishReason"`
			Content      struct {
				Parts []struct {
					FunctionCall      *functionCall `json:"functionCall"`
					FunctionCallSnake *functionCall `json:"function_call"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	c.setID(jsonStr(m["responseId"]))
	_ = json.Unmarshal(m["candidates"], &b.Candidates)
	for _, cand := range b.Candidates {
		for _, p := range cand.Content.Parts {
			fc := p.FunctionCall
			if fc == nil {
				fc = p.FunctionCallSnake
			}
			if fc != nil {
				sc := c.open("g"+strconv.Itoa(len(c.calls)), fc.ID, fc.Name, false)
				c.setArgs(sc, string(fc.Args))
				sc.open = false
			}
		}
		if cand.FinishReason != "" {
			c.finish(cand.FinishReason)
			c.complete = true
		}
	}
}

// bedrock reads a Converse stream event (named by its event type).
func (c *streamCapture) bedrock(ev streamEvent) {
	ops, end := bedrockCallOps(ev)
	for _, op := range ops {
		if op.start {
			c.open(op.key, op.id, op.name, false)
			continue
		}
		sc := c.byKey[op.key]
		c.appendArgs(sc, op.args)
		if op.stop && sc != nil {
			sc.open = false
		}
	}
	if end {
		var s struct {
			StopReason string `json:"stopReason"`
		}
		_ = json.Unmarshal(ev.data, &s)
		c.finish(s.StopReason)
		c.complete = true
	}
}

func (c *streamCapture) closeAll() {
	for _, sc := range c.calls {
		sc.open = false
	}
}

func jsonStr(raw json.RawMessage) string {
	s, _ := jsonString(raw)
	return s
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
