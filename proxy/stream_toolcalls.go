package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// Streamed tool calls are held until they are classified, when a response
// guardrail on the LLM scans tool calls (and only then: otherwise a stream
// is not touched). Text before a call streams as usual. From the moment a
// call opens, every event is held, in order, so the client never sees a
// later block, a finish reason or a usage event ahead of the call. When no
// call is open any more, the calls held are classified together, as tool
// call segments with the request's goal: clean, the held events are
// released byte for byte; flagged under block, they are never sent and the
// stream ends with the vendor's error event. A call larger than
// maxHeldCallBytes, or held longer than maxHoldDuration, is unscanned: the
// filter's tool-call action applies to it.

// The bounds of a hold; variables so tests can lower them.
var (
	maxHeldCallBytes = 1 << 20
	maxHoldDuration  = 30 * time.Second
)

// heldCall is one tool call assembled from its events.
type heldCall struct {
	id, name string
	args     strings.Builder
	open     bool
}

// toolCallHold assembles and holds a stream's tool calls.
type toolCallHold struct {
	dialect streamDialect
	calls   []*heldCall
	byKey   map[string]*heldCall
	queue   []byte
	holding bool
	since   time.Time
	// overflow is why the held calls cannot be scanned (size, time).
	overflow string
}

func newToolCallHold(d streamDialect) *toolCallHold {
	return &toolCallHold{dialect: d, byKey: map[string]*heldCall{}}
}

// callOp is what an event does to a call.
type callOp struct {
	key, id, name, args string
	start, stop         bool
	// whole: args are the call's complete arguments (replacing any).
	whole bool
}

// observe takes one whole event. It returns the bytes to send now (the
// event itself when nothing is held) and whether the held calls are ready
// to classify: no call is open any more, or a bound was exceeded.
func (h *toolCallHold) observe(ev streamEvent) (release []byte, ready bool) {
	ops, closeAll := h.dialect.callOps(ev)
	// Only a start opens a call; an op for a block that is not a held call
	// (a text block's stop) is not one.
	known := ops[:0]
	starts := false
	for _, op := range ops {
		if op.start || h.byKey[op.key] != nil {
			known = append(known, op)
			starts = starts || op.start
		}
	}
	ops = known
	if !h.holding && !starts {
		return ev.raw, false
	}
	if !h.holding {
		h.holding, h.since = true, time.Now()
	}
	h.queue = append(h.queue, ev.raw...)
	for _, op := range ops {
		c := h.byKey[op.key]
		if c == nil || (op.start && !c.open) {
			c = &heldCall{open: true}
			h.byKey[op.key] = c
			h.calls = append(h.calls, c)
		}
		if op.id != "" {
			c.id = op.id
		}
		if op.name != "" {
			c.name = op.name
		}
		if op.whole {
			c.args.Reset()
		}
		c.args.WriteString(op.args)
		if c.args.Len() > maxHeldCallBytes {
			h.overflow = "size"
		}
		if op.stop {
			c.open = false
		}
	}
	if closeAll {
		for _, c := range h.calls {
			c.open = false
		}
	}
	if time.Since(h.since) > maxHoldDuration {
		h.overflow = "time"
	}
	return nil, h.overflow != "" || !h.anyOpen()
}

func (h *toolCallHold) anyOpen() bool {
	for _, c := range h.calls {
		if c.open {
			return true
		}
	}
	return false
}

// take returns the held events and calls and starts holding afresh.
func (h *toolCallHold) take() ([]byte, []*heldCall, string) {
	q, calls, overflow := h.queue, h.calls, h.overflow
	h.queue, h.calls, h.overflow, h.holding = nil, nil, "", false
	h.byKey = map[string]*heldCall{}
	return q, calls, overflow
}

// callOps reads the tool call parts of an event. closeAll reports an event
// that ends every open call (a finish reason, the end of the stream).
func (d streamDialect) callOps(ev streamEvent) (ops []callOp, closeAll bool) {
	data := strings.TrimSpace(string(ev.data))
	if data == "[DONE]" {
		return nil, true
	}
	if d == "bedrock" {
		return bedrockCallOps(ev)
	}
	if !strings.HasPrefix(data, "{") {
		return nil, false
	}
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(data), &m) != nil {
		return nil, false
	}
	str := func(raw json.RawMessage) string {
		var s string
		_ = json.Unmarshal(raw, &s)
		return s
	}
	typ := str(m["type"])

	if strings.HasPrefix(typ, "response.") {
		// OpenAI Responses.
		var item struct {
			ID, CallID, Name, Type, Arguments, Input string
		}
		_ = json.Unmarshal(m["item"], &item)
		var it map[string]json.RawMessage
		_ = json.Unmarshal(m["item"], &it)
		item.CallID, item.Type = str(it["call_id"]), str(it["type"])
		isCall := item.Type == "function_call" || item.Type == "custom_tool_call" || item.Type == "mcp_call"
		switch typ {
		case "response.output_item.added":
			if isCall {
				ops = append(ops, callOp{key: item.ID, id: firstNonEmpty(item.CallID, item.ID), name: item.Name, start: true})
			}
		case "response.function_call_arguments.delta", "response.custom_tool_call_input.delta", "response.mcp_call_arguments.delta":
			ops = append(ops, callOp{key: str(m["item_id"]), args: str(m["delta"])})
		case "response.output_item.done":
			if isCall {
				args := item.Arguments
				if item.Type == "custom_tool_call" {
					args = item.Input
				}
				ops = append(ops, callOp{key: item.ID, id: firstNonEmpty(item.CallID, item.ID), name: item.Name, args: args, whole: true, stop: true})
			}
		case "response.completed", "response.incomplete", "response.failed":
			return ops, true
		}
		return ops, false
	}

	switch d {
	case "anthropic":
		var idx struct {
			Index int `json:"index"`
		}
		_ = json.Unmarshal([]byte(data), &idx)
		key := "b" + strconv.Itoa(idx.Index)
		switch typ {
		case "content_block_start":
			var b struct {
				Type, ID, Name string
				Input          json.RawMessage
			}
			_ = json.Unmarshal(m["content_block"], &b)
			if b.Type == "tool_use" || b.Type == "server_tool_use" || b.Type == "mcp_tool_use" {
				op := callOp{key: key, id: b.ID, name: b.Name, start: true}
				if in := strings.TrimSpace(string(b.Input)); in != "" && in != "{}" {
					op.args = in
				}
				ops = append(ops, op)
			}
		case "content_block_delta":
			var dl struct {
				Type        string `json:"type"`
				PartialJSON string `json:"partial_json"`
			}
			_ = json.Unmarshal(m["delta"], &dl)
			if dl.Type == "input_json_delta" {
				ops = append(ops, callOp{key: key, args: dl.PartialJSON})
			}
		case "content_block_stop":
			ops = append(ops, callOp{key: key, stop: true})
		case "message_stop":
			return ops, true
		}
		return ops, false
	case "google_ai", "vertex":
		var c struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						FunctionCall *struct {
							ID   string          `json:"id"`
							Name string          `json:"name"`
							Args json.RawMessage `json:"args"`
						} `json:"functionCall"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		_ = json.Unmarshal([]byte(data), &c)
		for ci, cand := range c.Candidates {
			for pi, p := range cand.Content.Parts {
				if fc := p.FunctionCall; fc != nil {
					args := strings.TrimSpace(string(fc.Args))
					if args == "" {
						args = "{}"
					}
					ops = append(ops, callOp{key: fmt.Sprintf("g%d.%d.%p", ci, pi, fc), id: fc.ID, name: fc.Name, args: args, start: true, whole: true, stop: true})
				}
			}
		}
		return ops, false
	}

	// OpenAI chat completions (and Ollama's OpenAI-compatible endpoint),
	// then Ollama's native message.tool_calls (whole calls).
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
			// A call's first delta carries its id and name.
			for _, tc := range ch.Delta.ToolCalls {
				ops = append(ops, callOp{key: fmt.Sprintf("c%d.%d", ch.Index, tc.Index), id: tc.ID, name: tc.Function.Name, args: tc.Function.Arguments,
					start: tc.ID != "" || tc.Function.Name != ""})
			}
			if fc := ch.Delta.FunctionCall; fc != nil {
				ops = append(ops, callOp{key: fmt.Sprintf("c%d.fn", ch.Index), name: fc.Name, args: fc.Arguments, start: fc.Name != ""})
			}
			if ch.FinishReason != nil && *ch.FinishReason != "" {
				closeAll = true
			}
		}
		return ops, closeAll
	}
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
		for i, tc := range msg.ToolCalls {
			ops = append(ops, callOp{key: fmt.Sprintf("o%p.%d", &msg, i), name: tc.Function.Name, args: string(tc.Function.Arguments), start: true, whole: true, stop: true})
		}
	}
	return ops, false
}

// bedrockCallOps reads a Converse stream event (named by its event type):
// contentBlockStart with a toolUse start opens a call, contentBlockDelta
// toolUse input adds to it, contentBlockStop closes it, messageStop ends
// every call.
func bedrockCallOps(ev streamEvent) ([]callOp, bool) {
	var e struct {
		Index int `json:"contentBlockIndex"`
		Start struct {
			ToolUse *struct {
				ToolUseID string `json:"toolUseId"`
				Name      string `json:"name"`
			} `json:"toolUse"`
		} `json:"start"`
		Delta struct {
			ToolUse *struct {
				Input string `json:"input"`
			} `json:"toolUse"`
		} `json:"delta"`
	}
	_ = json.Unmarshal(ev.data, &e)
	key := "b" + strconv.Itoa(e.Index)
	switch ev.name {
	case "contentBlockStart":
		if tu := e.Start.ToolUse; tu != nil {
			return []callOp{{key: key, id: tu.ToolUseID, name: tu.Name, start: true}}, false
		}
	case "contentBlockDelta":
		if tu := e.Delta.ToolUse; tu != nil {
			return []callOp{{key: key, args: tu.Input}}, false
		}
	case "contentBlockStop":
		return []callOp{{key: key, stop: true}}, false
	case "messageStop":
		return nil, true
	}
	return nil, false
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// heldCallsBody re-encodes held calls as an OpenAI chat completion, the
// shape the response guardrails read tool calls from.
func heldCallsBody(calls []*heldCall) []byte {
	type fn struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	type tc struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function fn     `json:"function"`
	}
	var tcs []tc
	for _, c := range calls {
		tcs = append(tcs, tc{ID: c.id, Type: "function", Function: fn{Name: c.name, Arguments: c.args.String()}})
	}
	b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "tool_calls": tcs}}}})
	return b
}

// classifyHeldCalls runs the LLM's response guardrails that scan tool calls
// over the held calls, with reqBody (in format) as their goal. overflow
// names a bound the calls exceeded; they are then unscanned, and blocked
// when a scanning guardrail's tool-call action is block.
func classifyHeldCalls(llm *models.LLM, svc services.ServiceInterface, calls []*heldCall, overflow string, reqBody []byte, format string, status int, r *http.Request) (bool, string, error) {
	if len(calls) == 0 {
		return false, "", nil
	}
	if overflow != "" {
		return unscannedHeldCalls(llm, calls, overflow, r)
	}
	return runResponseFilters(llm, svc, "", status, false, false, 0, "", r, &BufferedResponse{
		Body: heldCallsBody(calls), Format: "openai", RequestBody: reqBody, RequestFormat: format, ToolCallsOnly: true,
	})
}

// unscannedHeldCalls applies the scanning guardrails' tool-call action to
// calls that could not be scanned, and records it.
func unscannedHeldCalls(llm *models.LLM, calls []*heldCall, reason string, r *http.Request) (bool, string, error) {
	var names []string
	for _, c := range calls {
		names = append(names, c.name)
	}
	for _, f := range llm.Filters {
		if !f.ResponseFilter || !f.IsGuardrail() || !guardrails.ConfigScansToolCalls(f.Config) {
			continue
		}
		cfg, err := guardrails.ParseConfig(f.Config)
		if err == nil {
			cfg, err = guardrails.Normalize(cfg, true)
		}
		if err != nil {
			continue
		}
		action := cfg.ActionFor(guardrails.KindToolCall)
		scripting.RecordComplianceEvents(r.Context(), &scripting.ScriptOutput{ComplianceEvents: []scripting.ComplianceEventOutput{{
			EventType:   "guardrail.unscanned",
			Severity:    map[bool]string{true: "critical", false: "warning"}[action == guardrails.ActionBlock],
			Description: fmt.Sprintf("guardrail %q (%s): %d streamed tool call(s) not scanned (held_call_%s); action %s", f.Name, cfg.Provider, len(calls), reason, action),
			Metadata: map[string]interface{}{
				"provider": cfg.Provider, "detector": "unscanned", "reason": "held_call_" + reason, "count": len(calls),
				"action": action, "kinds": []string{guardrails.KindToolCall}, "tool_names": names,
			},
		}}}, f.Name, "proxy_response", 0, 0, llm.ID, string(llm.Vendor), "")
		if action == guardrails.ActionBlock {
			return true, cfg.BlockMessage, nil
		}
	}
	return false, "", nil
}
