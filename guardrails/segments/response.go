package segments

import (
	"fmt"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

// FormatOpenAIResponses is the OpenAI Responses API (POST /v1/responses),
// whose requests the proxy reads with an extractor of its own; this package
// reads only its tool calls and its goal.
const FormatOpenAIResponses = "openai_responses"

// ResponseToolCalls returns the tool calls in a buffered (non-streamed)
// response body in format, one unit each (Role assistant, Kind tool_call,
// the tool's name and call id, the arguments as Text), in body order:
//
//   - OpenAI chat completions: choices[].message.tool_calls and the legacy
//     function_call; Ollama: message.tool_calls (arguments an object);
//   - Anthropic (and an Anthropic body on Bedrock InvokeModel): content[]
//     tool_use, server_tool_use and mcp_tool_use blocks;
//   - Google AI / Vertex: candidates[].content.parts[].functionCall;
//   - Bedrock Converse: output.message.content[].toolUse;
//   - OpenAI Responses: output[] function_call, custom_tool_call, mcp_call
//     and mcp_approval_request items.
//
// A body that is not a JSON object is ErrUnrecognisedBody; a format with no
// tool calls (or one this package does not know) returns none.
func ResponseToolCalls(format string, body []byte) ([]Unit, error) {
	root, err := parseTree(body)
	if err != nil {
		return nil, err
	}
	if root.kind != nodeObject {
		return nil, fmt.Errorf("%w: not a JSON object", ErrUnrecognisedBody)
	}
	x := newExtraction(body)
	x.opts.ToolCalls = true
	x.enter(0)
	format = strings.ToLower(format)
	if format == FormatOpenAI && root.get("choices") == nil && root.get("output").is(nodeArray) {
		// A Responses API body reached under the vendor's name (a batch
		// results line).
		format = FormatOpenAIResponses
	}
	switch format {
	case FormatOpenAI, FormatOllama:
		for i, c := range root.get("choices").elems() {
			x.openAIResponseMessage(c.get("message"), pointer(pointerIndex("/choices", i), "message"))
		}
		x.openAIResponseMessage(root.get("message"), "/message")
	case FormatAnthropic:
		x.anthropicResponseBlocks(root.get("content"), "/content")
	case FormatBedrock:
		if root.get("content") != nil {
			x.anthropicResponseBlocks(root.get("content"), "/content")
			break
		}
		for k, b := range root.get("output").get("message").get("content").elems() {
			if use := b.get("toolUse"); use.is(nodeObject) {
				path := pointer(pointerIndex("/output/message/content", k), "toolUse")
				x.toolCall(use.get("input"), pointer(path, "input"), use.get("toolUseId"), use.get("name"))
			}
		}
	case FormatGoogleAI, FormatVertex:
		for i, c := range root.get("candidates").elems() {
			partsPath := pointer(pointer(pointerIndex("/candidates", i), "content"), "parts")
			for k, part := range c.get("content").get("parts").elems() {
				fc, key, err := x.alias(part, pointerIndex(partsPath, k), "functionCall", "function_call")
				if err != nil {
					return nil, err
				}
				if fc.is(nodeObject) {
					path := pointer(pointerIndex(partsPath, k), key)
					x.toolCall(fc.get("args"), pointer(path, "args"), fc.get("id"), fc.get("name"))
				}
			}
		}
	case FormatOpenAIResponses:
		for i, item := range root.get("output").elems() {
			path := pointerIndex("/output", i)
			switch x.str(item.get("type")) {
			case "function_call", "mcp_call", "mcp_approval_request":
				x.toolCall(item.get("arguments"), pointer(path, "arguments"), responsesCallID(item), item.get("name"))
			case "custom_tool_call":
				x.toolCall(item.get("input"), pointer(path, "input"), responsesCallID(item), item.get("name"))
			}
		}
	}
	x.finish()
	return x.result(), nil
}

// responsesCallID is the id a Responses tool call is answered by: call_id
// for function and custom tool calls, the item's id otherwise.
func responsesCallID(item *node) *node {
	if id := item.get("call_id"); id != nil {
		return id
	}
	return item.get("id")
}

func (x *extraction) openAIResponseMessage(m *node, path string) {
	if !m.is(nodeObject) {
		return
	}
	for j, call := range m.get("tool_calls").elems() {
		fnPath := pointer(pointerIndex(pointer(path, "tool_calls"), j), "function")
		fn := call.get("function")
		x.toolCall(fn.get("arguments"), pointer(fnPath, "arguments"), call.get("id"), fn.get("name"))
	}
	if fc := m.get("function_call"); fc.is(nodeObject) {
		x.toolCall(fc.get("arguments"), pointer(pointer(path, "function_call"), "arguments"), nil, fc.get("name"))
	}
}

func (x *extraction) anthropicResponseBlocks(n *node, path string) {
	for k, b := range n.elems() {
		switch x.str(b.get("type")) {
		case "tool_use", "server_tool_use", "mcp_tool_use":
			bp := pointerIndex(path, k)
			x.toolCall(b.get("input"), pointer(bp, "input"), b.get("id"), b.get("name"))
		}
	}
}

// Goal is the context of the tool calls a model makes in answer to the
// request body in format: its latest user text and its system prompt, each
// clipped (guardrails.ClipContext). Formats this package reads give their
// units' text; a Responses request its instructions and the text of its
// user messages. An unreadable body has no goal.
func Goal(format string, body []byte) guardrails.SegmentContext {
	if strings.ToLower(format) == FormatOpenAIResponses {
		return responsesGoal(body)
	}
	units, err := ExtractUnits(format, body)
	if err != nil {
		return guardrails.SegmentContext{}
	}
	var system []string
	user := ""
	for _, u := range units {
		switch {
		case u.Role == guardrails.RoleSystem && u.Text != "":
			system = append(system, u.Text)
		case u.Role == guardrails.RoleUser && u.Kind == guardrails.KindText && strings.TrimSpace(u.Text) != "":
			user = u.Text
		}
	}
	return guardrails.SegmentContext{UserGoal: guardrails.ClipContext(user), System: guardrails.ClipContext(strings.Join(system, "\n\n"))}
}

// responsesGoal reads a Responses request's instructions and its latest user
// text: input as a string, or the last user message of input[] (a string, or
// its input_text parts).
func responsesGoal(body []byte) guardrails.SegmentContext {
	root, err := parseTree(body)
	if err != nil || root.kind != nodeObject {
		return guardrails.SegmentContext{}
	}
	x := newExtraction(body)
	goal := guardrails.SegmentContext{System: guardrails.ClipContext(x.str(root.get("instructions")))}
	in := root.get("input")
	if in.is(nodeString) {
		goal.UserGoal = guardrails.ClipContext(x.str(in))
		return goal
	}
	for _, item := range in.elems() {
		role := x.str(item.get("role"))
		typ := x.str(item.get("type"))
		if (typ != "" && typ != "message") || (role != "user" && role != "") {
			if role == "system" || role == "developer" {
				goal.System = guardrails.ClipContext(strings.TrimSpace(goal.System + "\n\n" + responsesText(x, item.get("content"))))
			}
			continue
		}
		if text := responsesText(x, item.get("content")); strings.TrimSpace(text) != "" {
			goal.UserGoal = guardrails.ClipContext(text)
		}
	}
	return goal
}

// responsesText is a Responses message content's text: a string, or its
// text parts joined.
func responsesText(x *extraction, content *node) string {
	if content.is(nodeString) {
		return x.str(content)
	}
	var parts []string
	for _, p := range content.elems() {
		if t := x.str(p.get("text")); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n")
}
