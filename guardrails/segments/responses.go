package segments

import (
	"strings"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
)

// The fields the Responses segmenter reads, per object.
var (
	responsesRootFields    = []string{"input", "instructions", "prompt"}
	responsesItemFields    = []string{"type", "role", "content", "call_id", "name", "output", "arguments", "input"}
	responsesPartFields    = []string{"type", "text", "refusal"}
	responsesTextPartTypes = map[string]bool{"input_text": true, "output_text": true, "text": true}
)

// responsesSilentItems carry no text at all: the opaque compaction of
// earlier turns, references to stored items, and generated images.
var responsesSilentItems = map[string]bool{
	"compaction": true, "item_reference": true, "image_generation_call": true,
}

// responsesBinary names members of a tool output or unknown item that hold
// images, files or ids rather than text.
func responsesBinary(key string) bool {
	switch key {
	case "image_url", "file_data", "file_url", "file_id", "detail", "status":
		return true
	}
	return opaque(key)
}

// OpenAI Responses (POST /v1/responses): instructions is the system prompt;
// input is a string (one user message) or an array of items. Each message
// item is a unit of its role (input_text and output_text parts; images and
// files are not text), and each *_call_output item a tool result named by
// the call with its call_id. The built-in tools' results (mcp_call output,
// file_search results, code_interpreter logs) are tool results named by the
// tool. A stored prompt's variables are the current turn's user text.
//
// The model's own items (reasoning summaries, function and custom tool call
// arguments, MCP and code interpreter calls, web search and file search
// queries, computer and shell actions, and *_call items added later) are
// assistant units of detection text only: the model reads them as earlier
// turns, and a client can forge them, so a guardrail sees them, but they
// have no leaves and are never rewritten. With Options.ToolCalls, function,
// custom, MCP and code interpreter calls are tool call units instead.
// encrypted_content and signatures are never read.
//
// Every item is one message: Message is its position in input, and Turn
// counts the runs of model items (assistant messages, calls, reasoning)
// before it.
func extractResponses(x *extraction, root *node) error {
	if err := x.strict(root, "", responsesRootFields...); err != nil {
		return err
	}
	input := root.get("input")
	if input == nil && root.get("instructions") == nil && root.get("prompt") == nil && root.get("previous_response_id") == nil {
		_, err := x.required(root, "input")
		return err
	}
	items := input.elems()
	if input.is(nodeArray) {
		var err error
		if items, err = x.objects(input, "/input"); err != nil {
			return err
		}
	}
	for i, item := range items {
		path := pointerIndex("/input", i)
		if err := x.strict(item, path, responsesItemFields...); err != nil {
			return err
		}
		switch typ := x.str(item.get("type")); typ {
		case "function_call", "custom_tool_call", "local_shell_call", "computer_call", "shell_call", "apply_patch_call":
			x.noteCall(item.get("call_id"), item.get("name"))
		}
	}

	if ins := root.get("instructions"); !x.isNull(ins) {
		if !ins.is(nodeString) {
			return shapeError("/instructions", "a string")
		}
		u := x.open("/instructions", wholeString, guardrails.Segment{Role: guardrails.RoleSystem})
		x.leaf(u, ins, "/instructions")
	}

	switch {
	case x.isNull(input):
	case input.is(nodeString):
		x.enter(0)
		u := x.open("/input", wholeString, guardrails.Segment{Role: guardrails.RoleUser})
		x.leaf(u, input, "/input")
	case input.is(nodeArray):
		model := false
		for i, item := range items {
			x.enter(i)
			itemModel := responsesModelItem(x, item)
			if model && !itemModel {
				x.turn++
			}
			model = itemModel
			if err := x.responsesItem(item, pointerIndex("/input", i)); err != nil {
				return err
			}
		}
	default:
		return shapeError("/input", "a string or an array")
	}

	// A stored prompt's variables are substituted into the prompt the
	// model reads, in this turn.
	if vars := root.get("prompt").get("variables"); vars.is(nodeObject) {
		x.enter(len(items))
		u := x.open("/prompt/variables", wholeLeaves, guardrails.Segment{Role: guardrails.RoleUser})
		x.walk(u, vars, "/prompt/variables", false, responsesBinary)
	}
	return nil
}

// responsesModelItem reports whether an input item is something the model
// produced (an assistant message, a call, reasoning), as opposed to input to
// the model (a user, system or developer message, a tool's output).
func responsesModelItem(x *extraction, item *node) bool {
	typ := x.str(item.get("type"))
	switch {
	case typ == "message" || typ == "" && item.get("role") != nil:
		return x.str(item.get("role")) == "assistant"
	case strings.HasSuffix(typ, "_output"):
		return false
	case typ == "mcp_approval_response":
		return false
	}
	return true
}

// responsesItem adds one input item's units.
func (x *extraction) responsesItem(item *node, path string) error {
	typ := x.str(item.get("type"))
	switch {
	case typ == "message" || typ == "" && item.get("role") != nil:
		return x.responsesMessage(item, path)
	case strings.HasSuffix(typ, "_call_output"):
		id := x.str(item.get("call_id"))
		outPath := pointer(path, "output")
		u := x.open(outPath, wholeString, x.toolResult(id, ""))
		u.adjacent = true
		return x.responsesOutput(u, item.get("output"), outPath)
	case typ == "function_call":
		x.responsesCall(item.get("arguments"), pointer(path, "arguments"), item.get("call_id"), item.get("name"))
	case typ == "custom_tool_call":
		x.responsesCall(item.get("input"), pointer(path, "input"), item.get("call_id"), item.get("name"))
	case typ == "mcp_call":
		x.responsesCall(item.get("arguments"), pointer(path, "arguments"), item.get("id"), item.get("name"))
		result := x.open(path, wholeLeaves, x.toolResult(x.str(item.get("id")), x.str(item.get("name"))))
		x.leaf(result, item.get("output"), pointer(path, "output"))
		x.leaf(result, item.get("error"), pointer(path, "error"))
	case typ == "mcp_list_tools":
		// Each listed tool's description is a result of its own, named by
		// the tool (a poisoned description names the tool it poisons).
		id := x.str(item.get("id"))
		for k, t := range item.get("tools").elems() {
			descPath := pointer(pointerIndex(pointer(path, "tools"), k), "description")
			result := x.open(descPath, wholeLeaves, x.toolResult(id, x.str(t.get("name"))))
			x.leaf(result, t.get("description"), descPath)
		}
		result := x.open(path, wholeLeaves, x.toolResult(id, x.str(item.get("server_label"))))
		x.leaf(result, item.get("error"), pointer(path, "error"))
	case typ == "mcp_approval_request":
		x.responsesCall(item.get("arguments"), pointer(path, "arguments"), item.get("id"), item.get("name"))
	case typ == "mcp_approval_response":
		u := x.open(path, wholeLeaves, guardrails.Segment{Role: guardrails.RoleUser})
		x.leaf(u, item.get("reason"), pointer(path, "reason"))
	case typ == "file_search_call":
		x.modelText(path, guardrails.KindToolCall, x.str(item.get("id")), "file_search", item.get("queries"))
		result := x.open(path, wholeLeaves, x.toolResult(x.str(item.get("id")), "file_search"))
		for k, r := range item.get("results").elems() {
			x.leaf(result, r.get("text"), pointer(pointerIndex(pointer(path, "results"), k), "text"))
		}
	case typ == "code_interpreter_call":
		x.responsesCall(item.get("code"), pointer(path, "code"), item.get("id"), nil)
		result := x.open(path, wholeLeaves, x.toolResult(x.str(item.get("id")), "code_interpreter"))
		for k, o := range item.get("outputs").elems() {
			x.leaf(result, o.get("logs"), pointer(pointerIndex(pointer(path, "outputs"), k), "logs"))
		}
	case typ == "reasoning":
		// The summaries and reasoning text; encrypted_content is opaque.
		var texts []*node
		for _, s := range item.get("summary").elems() {
			texts = append(texts, s.get("text"))
		}
		for _, c := range item.get("content").elems() {
			texts = append(texts, c.get("text"))
		}
		x.modelText(path, guardrails.KindText, "", "", texts...)
	case typ == "web_search_call":
		action := item.get("action")
		x.modelText(path, guardrails.KindToolCall, x.str(item.get("id")), "web_search", action.get("query"), action.get("queries"))
	case typ == "computer_call":
		x.modelText(path, guardrails.KindToolCall, x.str(item.get("call_id")), "computer", item.get("action").get("text"))
	case typ == "local_shell_call":
		x.modelText(path, guardrails.KindToolCall, x.str(item.get("call_id")), "local_shell", item.get("action").get("command"))
	case responsesSilentItems[typ]:
	case strings.HasSuffix(typ, "_call"):
		// A call type added later (shell_call, apply_patch_call...): every
		// string but ids, signatures and encrypted fields, as detection text.
		u := x.open(path, wholeNone, guardrails.Segment{
			Role: guardrails.RoleAssistant, Kind: guardrails.KindToolCall,
			ToolName: strings.TrimSuffix(typ, "_call"), ToolCallID: x.str(item.get("call_id")),
		})
		x.detectAll(u, item, responsesBinary)
	default:
		// An item type added to the API later: its strings join a unit of
		// their own rather than be skipped.
		u := x.open(path, wholeLeaves, guardrails.Segment{Role: guardrails.RoleUser})
		x.walk(u, item, path, false, responsesBinary)
	}
	return nil
}

// responsesCall is the model's call with its arguments args: a tool call
// unit with Options.ToolCalls, else an assistant unit of detection text.
func (x *extraction) responsesCall(args *node, path string, id, name *node) {
	if x.opts.ToolCalls {
		x.toolCall(args, path, id, name)
		return
	}
	u := x.open(path, wholeNone, guardrails.Segment{
		Role: guardrails.RoleAssistant, Kind: guardrails.KindToolCall,
		ToolName: x.str(name), ToolCallID: x.str(id),
	})
	if args.is(nodeString) {
		x.detect(u, x.str(args))
	} else if args.is(nodeObject) || args.is(nodeArray) {
		x.detect(u, string(x.body[args.start:args.end]))
	}
}

// modelText opens an assistant unit of detection text for the strings in
// nodes (strings, or arrays of strings): model output that is read but
// never rewritten.
func (x *extraction) modelText(path, kind, callID, tool string, nodes ...*node) {
	u := x.open(path, wholeNone, guardrails.Segment{Role: guardrails.RoleAssistant, Kind: kind, ToolName: tool, ToolCallID: callID})
	for _, n := range nodes {
		if n.is(nodeArray) {
			for _, e := range n.vals {
				x.detect(u, x.str(e))
			}
			continue
		}
		x.detect(u, x.str(n))
	}
}

// detectAll adds every string under n to u as detection text, skipping
// members skip names.
func (x *extraction) detectAll(u *Unit, n *node, skip func(string) bool) {
	switch {
	case n.is(nodeString):
		x.detect(u, x.str(n))
	case n.is(nodeObject):
		for i, k := range n.keys {
			if !skip(k) {
				x.detectAll(u, n.vals[i], skip)
			}
		}
	case n.is(nodeArray):
		for _, v := range n.vals {
			x.detectAll(u, v, skip)
		}
	}
}

// responsesMessage adds a message item: content is a string or an array of
// parts.
func (x *extraction) responsesMessage(item *node, path string) error {
	// developer is scanned as user, like OpenAI chat's (messageRole).
	role := messageRole(x.str(item.get("role")))
	contentPath := pointer(path, "content")
	u := x.open(contentPath, wholeString, guardrails.Segment{Role: role})
	u.adjacent = true
	content := item.get("content")
	switch {
	case x.isNull(content):
	case content.is(nodeString):
		x.leaf(u, content, contentPath)
	case content.is(nodeArray):
		parts, err := x.objects(content, contentPath)
		if err != nil {
			return err
		}
		for k, part := range parts {
			if err := x.responsesPart(u, part, pointerIndex(contentPath, k)); err != nil {
				return err
			}
		}
	default:
		return shapeError(contentPath, "a string or an array")
	}
	return nil
}

// responsesPart adds a content part: text parts and refusals are leaves;
// images, files and audio are not text; another type joins u with its
// strings.
func (x *extraction) responsesPart(u *Unit, part *node, path string) error {
	if err := x.strict(part, path, responsesPartFields...); err != nil {
		return err
	}
	switch typ := x.str(part.get("type")); {
	case responsesTextPartTypes[typ]:
		x.leaf(u, part.get("text"), pointer(path, "text"))
	case typ == "refusal":
		x.leaf(u, part.get("refusal"), pointer(path, "refusal"))
	case typ == "input_image" || typ == "input_file" || typ == "input_audio":
	default:
		x.walk(u, part, path, false, responsesBinary)
	}
	return nil
}

// responsesOutput adds a tool output: a string, an array of content parts,
// or (from older clients) any other JSON, read with its keys like a Google
// functionResponse.
func (x *extraction) responsesOutput(u *Unit, out *node, path string) error {
	switch {
	case x.isNull(out):
	case out.is(nodeString):
		x.leaf(u, out, path)
	case out.is(nodeArray):
		parts, err := x.objects(out, path)
		if err != nil {
			return err
		}
		for k, part := range parts {
			if err := x.responsesPart(u, part, pointerIndex(path, k)); err != nil {
				return err
			}
		}
	default:
		x.walk(u, out, path, true, responsesBinary)
	}
	return nil
}
