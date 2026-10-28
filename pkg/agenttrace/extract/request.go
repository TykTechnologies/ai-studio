package extract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/guardrails/segments"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
)

// ToolResult is one trailing tool result in an LLM request: a result the
// client sends back after the model's last message (§4.4 R1). The content
// appears only as a fingerprint.
type ToolResult struct {
	// ID is the tool call id the result answers; empty where the format
	// carries none (Gemini without ids, Ollama). R1 needs it.
	ID string
	// Name is the called tool's, when the body names it. A result answering
	// a call made before the model's last message may have none (jyx.57).
	Name string
	// CallArgsFP is set on a result without an id: the fingerprint of the
	// arguments of the call it answers, from the request's history
	// (pairResults). Nil without a Fingerprinter.
	CallArgsFP agenttrace.Fingerprint
	// ResultFP is nil without a Fingerprinter or when the result has no text.
	ResultFP agenttrace.Fingerprint
	IsError  bool
	// Server marks the replayed output of a provider-hosted tool (an
	// OpenAI Responses mcp_call item), not a client tool result.
	Server bool
}

// Request is what correlation needs from one LLM request body.
type Request struct {
	PreviousResponseID string
	// ToolResults are the trailing tool results, in body order. Earlier
	// ones are history and are not returned.
	ToolResults []ToolResult
	// OfferedTools are the names of the tools the request offers the model
	// (its tool definitions), in body order: the "tools available" of the
	// LLM span, and the set a call to an unoffered tool is flagged against
	// (jyx.51).
	OfferedTools []string
	// LastRole is the role of the last input message (system, user,
	// assistant or tool), for turn boundaries (§3.1): a request ending in
	// user text opens a turn, one ending in tool results continues it.
	LastRole string
}

// Apply copies the request extracts into x. Replayed server tool outputs
// are left out. A result without an id is kept, with its call's name and
// argument fingerprint: it still continues the turn, and the assembler
// matches it by name and arguments.
func (r Request) Apply(x *agenttrace.Extracts) {
	x.PreviousResponseID = r.PreviousResponseID
	x.OfferedTools = r.OfferedTools
	for _, t := range r.ToolResults {
		if t.Server {
			continue
		}
		tr := agenttrace.ToolResult{ID: t.ID, ResultFP: t.ResultFP, IsError: t.IsError}
		if t.ID == "" {
			tr.Name, tr.CallArgsFP = t.Name, t.CallArgsFP
		}
		x.ToolResults = append(x.ToolResults, tr)
	}
}

// ParseRequest extracts correlation material from an LLM request body. fp
// may be nil: ids and names are still extracted, fingerprints are not.
func ParseRequest(format string, body []byte, fp agenttrace.Fingerprinter) (Request, error) {
	r, err := parseRequest(format, body, fp)
	r.bound()
	return r, err
}

func parseRequest(format string, body []byte, fp agenttrace.Fingerprinter) (Request, error) {
	format = strings.ToLower(format)
	if format == FormatOpenAIResponses {
		return parseResponsesRequest(body, fp)
	}
	if !segments.Supports(format) {
		return Request{}, fmt.Errorf("%w: %q", ErrUnsupportedFormat, format)
	}
	// Only the units after the last assistant message are returned, so the
	// history before it need not be segmented (jyx.57). The trimmed body
	// gives the same result when it still holds a non-assistant unit after
	// that message; otherwise, or when it fails, the whole body is parsed.
	if trimmed, ok := trimHistory(format, body); ok {
		if r, settled, err := parseUnits(format, trimmed, fp); err == nil && settled {
			return r, nil
		}
	}
	r, _, err := parseUnits(format, body, fp)
	return r, err
}

// parseUnits segments a supported body. settled reports a non-assistant
// unit after the first message, which makes a trimmed body's result the
// whole body's (trimHistory).
func parseUnits(format string, body []byte, fp agenttrace.Fingerprinter) (Request, bool, error) {
	// Tool definitions come in the same pass. A body whose definitions the
	// segmenter refuses still yields its tool results.
	all, err := segments.ExtractUnitsWith(format, body, segments.Options{ToolDefinitions: true})
	if err != nil {
		if all, err = segments.ExtractUnitsWith(format, body, segments.Options{}); err != nil {
			return Request{}, false, err
		}
	}
	var r Request
	settled := false
	units := all[:0:0]
	for _, u := range all {
		if u.IsToolDefinition() {
			r.OfferedTools = append(r.OfferedTools, u.ToolName)
			continue
		}
		units = append(units, u)
	}
	current := -1
	for _, u := range units {
		if u.Role != guardrails.RoleAssistant && u.Turn > current {
			current = u.Turn
		}
		if u.Role != guardrails.RoleAssistant && u.Message >= 1 && inHistory(format, u.Path) {
			settled = true
		}
	}
	if n := len(units); n > 0 {
		r.LastRole = units[n-1].Role
		if units[n-1].IsToolResult() {
			r.LastRole = guardrails.RoleTool
		}
	}
	var msgs []int
	for _, u := range units {
		if !u.IsToolResult() || u.Turn != current {
			continue
		}
		texts := make([]string, 0, len(u.Leaves))
		for _, l := range u.Leaves {
			texts = append(texts, l.Text)
		}
		r.ToolResults = append(r.ToolResults, ToolResult{
			ID:       u.ToolCallID,
			Name:     u.ToolName,
			ResultFP: ResultFingerprint(fp, texts...),
		})
		msgs = append(msgs, u.Message)
	}
	if len(r.ToolResults) > 0 {
		pairResults(format, body, current, r.ToolResults, fp)
	}
	// A second decode costs as much as the segmenter; most requests carry no
	// error result, so look for the marker first.
	if len(r.ToolResults) > 0 &&
		(format == FormatAnthropic && bytes.Contains(body, []byte(`"is_error"`)) ||
			format == FormatBedrock && bytes.Contains(body, []byte(`"status"`))) {
		markErrors(body, msgs, r.ToolResults)
	}
	return r, settled, nil
}

// pairResults completes the trailing results sent back without the id of
// the call they answer (§4.4 R1 fallback): Gemini makes the id optional,
// Ollama has none, and some clients drop it on both the call and the
// result (the AI SDK on Vertex). Each such result is paired with an
// unclaimed call of the model's last message: the first of the same name,
// or the next in order when the result names none. It takes the call's id
// when the call kept one; otherwise the call's name and argument
// fingerprint, for the assembler's name and arguments match. Only bodies
// with such a result pay for the second pass.
func pairResults(format string, body []byte, turn int, results []ToolResult, fp agenttrace.Fingerprinter) {
	missing := false
	for _, t := range results {
		missing = missing || t.ID == ""
	}
	if !missing {
		return
	}
	units, err := segments.ExtractUnitsWith(format, body, segments.Options{ToolCalls: true})
	if err != nil {
		return
	}
	answered := map[string]bool{}
	for _, t := range results {
		answered[t.ID] = t.ID != ""
	}
	var calls []segments.Unit
	for _, u := range units {
		// The model's last message is the one assistant message before the
		// results' turn (Unit.Turn counts the assistant messages before).
		if u.IsToolCall() && u.Turn == turn-1 && !answered[u.ToolCallID] {
			calls = append(calls, u)
		}
	}
	for i := range results {
		if results[i].ID != "" {
			continue
		}
		j := -1
		for k, c := range calls {
			if results[i].Name == "" || c.ToolName == results[i].Name {
				j = k
				break
			}
		}
		if j < 0 {
			continue
		}
		c := calls[j]
		calls = append(calls[:j], calls[j+1:]...)
		if c.ToolCallID != "" {
			results[i].ID = c.ToolCallID
			continue
		}
		if results[i].Name == "" {
			results[i].Name = c.ToolName
		}
		// The arguments' raw bytes, read in place; missing ones fingerprint
		// as {} on both sides.
		args, _ := rawAt(body, c.Path)
		if f, err := ArgsFingerprint(fp, args); err == nil {
			results[i].CallArgsFP = f
		}
	}
}

// rawAt returns the raw bytes of the value at JSON Pointer ptr (RFC 6901)
// in body, scanning in place (trim.go's scanner) rather than decoding the
// body. Keys are compared as written, so a key spelled with escapes is not
// found.
func rawAt(body []byte, ptr string) ([]byte, bool) {
	i := skipSpace(body, 0)
	end := skipValue(body, i)
	if end < 0 {
		return nil, false
	}
	if ptr == "" {
		return body[i:end], true
	}
	for _, tok := range strings.Split(strings.TrimPrefix(ptr, "/"), "/") {
		tok = strings.ReplaceAll(strings.ReplaceAll(tok, "~1", "/"), "~0", "~")
		found := false
		switch {
		case i < len(body) && body[i] == '{':
			members(body, i, func(k []byte, v, e int) bool {
				if string(k) == tok {
					i, end, found = v, e, true
					return false
				}
				return true
			})
		case i < len(body) && body[i] == '[':
			n, err := strconv.Atoi(tok)
			if err != nil || n < 0 {
				return nil, false
			}
			j := skipSpace(body, i+1)
			for idx := 0; j < len(body) && body[j] != ']'; idx++ {
				e := skipValue(body, j)
				if e < 0 {
					return nil, false
				}
				if idx == n {
					i, end, found = j, e, true
					break
				}
				if j = skipSpace(body, e); j < len(body) && body[j] == ',' {
					j = skipSpace(body, j+1)
				}
			}
		}
		if !found {
			return nil, false
		}
	}
	return body[i:end], true
}

// markErrors sets IsError from Anthropic tool_result is_error and Bedrock
// toolResult status "error", reading only the messages that hold the
// trailing results.
func markErrors(body []byte, msgs []int, results []ToolResult) {
	var b struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &b) != nil {
		return
	}
	failed := map[string]bool{}
	seen := map[int]bool{}
	for _, m := range msgs {
		if m < 0 || m >= len(b.Messages) || seen[m] {
			continue
		}
		seen[m] = true
		var blocks []struct {
			ToolUseID  string `json:"tool_use_id"`
			IsError    bool   `json:"is_error"`
			ToolResult *struct {
				ToolUseID string `json:"toolUseId"`
				Status    string `json:"status"`
			} `json:"toolResult"`
		}
		if json.Unmarshal(b.Messages[m].Content, &blocks) != nil {
			continue
		}
		for _, bl := range blocks {
			if bl.IsError && bl.ToolUseID != "" {
				failed[bl.ToolUseID] = true
			}
			if tr := bl.ToolResult; tr != nil && strings.EqualFold(tr.Status, "error") {
				failed[tr.ToolUseID] = true
			}
		}
	}
	for i := range results {
		if failed[results[i].ID] {
			results[i].IsError = true
		}
	}
}

type responsesItem struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	ID      string          `json:"id"`
	CallID  string          `json:"call_id"`
	Name    string          `json:"name"`
	Output  json.RawMessage `json:"output"`
	Error   json.RawMessage `json:"error"`
	Content json.RawMessage `json:"content"`
}

// parseResponsesRequest reads an OpenAI Responses API request: input is a
// string (one user message) or an array of items.
func parseResponsesRequest(body []byte, fp agenttrace.Fingerprinter) (Request, error) {
	var b struct {
		PreviousResponseID string          `json:"previous_response_id"`
		Input              json.RawMessage `json:"input"`
		Tools              []struct {
			Type        string `json:"type"`
			Name        string `json:"name"`
			ServerLabel string `json:"server_label"`
		} `json:"tools"`
	}
	if err := unmarshal(body, &b); err != nil {
		return Request{}, err
	}
	r := Request{PreviousResponseID: b.PreviousResponseID}
	for _, t := range b.Tools {
		// Function and custom tools are named; hosted tools (web_search,
		// mcp, ...) by their type, an MCP server by its label.
		switch {
		case t.Name != "":
			r.OfferedTools = append(r.OfferedTools, t.Name)
		case t.Type == "mcp" && t.ServerLabel != "":
			r.OfferedTools = append(r.OfferedTools, "mcp:"+t.ServerLabel)
		case t.Type != "":
			r.OfferedTools = append(r.OfferedTools, t.Type)
		}
	}
	in := strings.TrimSpace(string(b.Input))
	if in == "" || in == "null" {
		return r, nil
	}
	if in[0] == '"' {
		r.LastRole = guardrails.RoleUser
		return r, nil
	}
	var items []responsesItem
	if err := json.Unmarshal(b.Input, &items); err != nil {
		return Request{}, fmt.Errorf("%w: input: %v", ErrUnrecognisedBody, err)
	}
	// Trailing results are the outputs after the last item the model produced
	// (a call, reasoning or an assistant message): a tool loop of only
	// function_call items resends every earlier output as history. The
	// standard loop appends a response's calls, then their outputs, so
	// parallel calls are all trailing; outputs interleaved with their calls
	// read like a sequential loop and only the last is trailing: the input
	// alone cannot tell the two apart, and re-linking history on every
	// request is the worse error.
	names := map[string]string{}
	lastModel := -1
	for i, it := range items {
		switch it.Type {
		case "function_call", "custom_tool_call":
			names[it.CallID] = it.Name
			lastModel = i
		case "computer_call", "local_shell_call":
			// Named like the response side names these calls.
			names[it.CallID] = it.Type
			lastModel = i
		case "reasoning":
			lastModel = i
		case "", "message":
			if it.Role == guardrails.RoleAssistant {
				lastModel = i
			}
		}
	}
	for i := lastModel + 1; i < len(items); i++ {
		it := items[i]
		switch it.Type {
		case "function_call_output", "custom_tool_call_output", "computer_call_output", "local_shell_call_output":
			r.ToolResults = append(r.ToolResults, ToolResult{
				ID:       it.CallID,
				Name:     names[it.CallID],
				ResultFP: ResultFingerprint(fp, outputTexts(it.Output)...),
			})
		case "mcp_call":
			r.ToolResults = append(r.ToolResults, ToolResult{
				ID:       it.ID,
				Name:     it.Name,
				ResultFP: ResultFingerprint(fp, outputTexts(it.Output)...),
				IsError:  present(it.Error),
				Server:   true,
			})
		}
	}
	if n := len(items); n > 0 {
		last := items[n-1]
		switch last.Type {
		case "", "message":
			r.LastRole = last.Role
			if r.LastRole == "developer" {
				r.LastRole = guardrails.RoleSystem
			}
		case "function_call_output", "custom_tool_call_output", "computer_call_output", "local_shell_call_output", "mcp_call":
			r.LastRole = guardrails.RoleTool
		default:
			r.LastRole = guardrails.RoleAssistant
		}
	}
	return r, nil
}

// outputTexts returns the text of a Responses tool output: a string, or the
// text of each input_text / output_text part of an array.
func outputTexts(raw json.RawMessage) []string {
	if !present(raw) {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []string{s}
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return nil
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p.Text != "" && (p.Type == "input_text" || p.Type == "output_text" || p.Type == "text") {
			out = append(out, p.Text)
		}
	}
	return out
}
